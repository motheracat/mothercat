package notifications

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	sessionCookieName = "mothercat_session"
	defaultPageSize   = 50
	maxPageSize       = 100
	maxPageOffset     = 1_000_000
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

var ErrUnauthenticated = errors.New("unauthenticated")

var supportedEventTypes = []string{
	"task.assigned", "task.submitted", "task.approved", "task.changes_requested",
	"task.rejected", "task.comment_added", "task.due_soon", "task.overdue",
}

type Principal struct {
	ID string
}

type Authenticator interface {
	Authenticate(context.Context, string) (Principal, error)
}

type Handler struct {
	db            *sql.DB
	authenticator Authenticator
	logger        *slog.Logger
}

type notification struct {
	ID        string     `json:"id"`
	EventType string     `json:"event_type"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Resource  *resource  `json:"resource,omitempty"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type resource struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type preference struct {
	EventType string `json:"event_type"`
	Enabled   bool   `json:"enabled"`
}

type page struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

func NewHandler(db *sql.DB, authenticator Authenticator, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{db: db, authenticator: authenticator, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/notifications", h.list)
	mux.HandleFunc("GET /api/v1/notifications/unread-count", h.unreadCount)
	mux.HandleFunc("POST /api/v1/notifications/{notificationID}/read", h.markRead)
	mux.HandleFunc("POST /api/v1/notifications/read-all", h.markAllRead)
	mux.HandleFunc("GET /api/v1/notifications/preferences", h.getPreferences)
	mux.HandleFunc("PUT /api/v1/notifications/preferences", h.replacePreferences)
	return h.requestIDMiddleware(h.csrfMiddleware(mux))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	limit, offset, unread, err := parseListQuery(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_query", requestID(ctx))
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	where := "recipient_account_id=$1::uuid"
	args := []any{principal.ID}
	if unread {
		where += " AND read_at IS NULL"
	}
	var total int
	if err := h.db.QueryRowContext(ctx,
		"SELECT count(*) FROM notifications.notifications WHERE "+where, args...,
	).Scan(&total); err != nil {
		h.fail(w, ctx, "count notifications", err)
		return
	}
	rows, err := h.db.QueryContext(ctx, `
		SELECT id::text, event_type, title, body, resource_type, resource_id::text, read_at, created_at
		FROM notifications.notifications WHERE `+where+`
		ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3
	`, principal.ID, limit, offset)
	if err != nil {
		h.fail(w, ctx, "list notifications", err)
		return
	}
	defer rows.Close()
	items := make([]notification, 0)
	for rows.Next() {
		item, scanErr := scanNotification(rows)
		if scanErr != nil {
			h.fail(w, ctx, "scan notification", scanErr)
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		h.fail(w, ctx, "read notifications", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"notifications": items, "page": page{Limit: limit, Offset: offset, Total: total},
	})
}

func (h *Handler) unreadCount(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	var count int
	if err := h.db.QueryRowContext(ctx, `
		SELECT count(*) FROM notifications.notifications
		WHERE recipient_account_id=$1::uuid AND read_at IS NULL
	`, principal.ID).Scan(&count); err != nil {
		h.fail(w, ctx, "count unread notifications", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"unread_count": count})
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	id := r.PathValue("notificationID")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid_notification_id", requestID(ctx))
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	result, err := h.db.ExecContext(ctx, `
		UPDATE notifications.notifications SET read_at=coalesce(read_at, now())
		WHERE id=$1::uuid AND recipient_account_id=$2::uuid
	`, id, principal.ID)
	if err != nil {
		h.fail(w, ctx, "mark notification read", err)
		return
	}
	changed, err := result.RowsAffected()
	if err != nil {
		h.fail(w, ctx, "check notification update", err)
		return
	}
	if changed == 0 {
		writeError(w, http.StatusNotFound, "not_found", requestID(ctx))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) markAllRead(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	result, err := h.db.ExecContext(ctx, `
		UPDATE notifications.notifications SET read_at=now()
		WHERE recipient_account_id=$1::uuid AND read_at IS NULL
	`, principal.ID)
	if err != nil {
		h.fail(w, ctx, "mark all notifications read", err)
		return
	}
	changed, err := result.RowsAffected()
	if err != nil {
		h.fail(w, ctx, "count notifications marked read", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"marked_read": changed})
}

func (h *Handler) getPreferences(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	rows, err := h.db.QueryContext(ctx, `
		SELECT supported.event_type, coalesce(preference.enabled, true)
		FROM (VALUES
			('task.assigned'), ('task.submitted'), ('task.approved'), ('task.changes_requested'),
			('task.rejected'), ('task.comment_added'), ('task.due_soon'), ('task.overdue')
		) AS supported(event_type)
		LEFT JOIN notifications.notification_preferences preference
		  ON preference.account_id=$1::uuid AND preference.event_type=supported.event_type
		ORDER BY supported.event_type
	`, principal.ID)
	if err != nil {
		h.fail(w, ctx, "get notification preferences", err)
		return
	}
	defer rows.Close()
	items := make([]preference, 0)
	for rows.Next() {
		var item preference
		if err := rows.Scan(&item.EventType, &item.Enabled); err != nil {
			h.fail(w, ctx, "scan notification preference", err)
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		h.fail(w, ctx, "read notification preferences", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"preferences": items})
}

func (h *Handler) replacePreferences(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var input struct {
		Preferences []preference `json:"preferences"`
	}
	if !decodeJSON(w, r, &input) || input.Preferences == nil || len(input.Preferences) > 50 {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID(ctx))
		return
	}
	seen := make(map[string]bool, len(input.Preferences))
	for _, item := range input.Preferences {
		if !validEventType(item.EventType) || seen[item.EventType] {
			writeError(w, http.StatusBadRequest, "invalid_preferences", requestID(ctx))
			return
		}
		seen[item.EventType] = true
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		h.fail(w, ctx, "begin notification preferences update", err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM notifications.notification_preferences WHERE account_id=$1::uuid`, principal.ID,
	); err != nil {
		h.fail(w, ctx, "replace notification preferences", err)
		return
	}
	for _, item := range input.Preferences {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO notifications.notification_preferences (account_id, event_type, enabled)
			VALUES ($1::uuid, $2, $3)
		`, principal.ID, item.EventType, item.Enabled); err != nil {
			h.fail(w, ctx, "save notification preference", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, ctx, "commit notification preferences", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"preferences": input.Preferences})
}

func scanNotification(row interface{ Scan(...any) error }) (notification, error) {
	var item notification
	var resourceType, resourceID sql.NullString
	var readAt sql.NullTime
	if err := row.Scan(&item.ID, &item.EventType, &item.Title, &item.Body,
		&resourceType, &resourceID, &readAt, &item.CreatedAt); err != nil {
		return notification{}, err
	}
	if resourceType.Valid && resourceID.Valid {
		item.Resource = &resource{Type: resourceType.String, ID: resourceID.String}
	}
	if readAt.Valid {
		item.ReadAt = &readAt.Time
	}
	return item, nil
}

func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (Principal, context.Context, bool) {
	ctx := r.Context()
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", requestID(ctx))
		return Principal{}, nil, false
	}
	if h.authenticator == nil {
		writeError(w, http.StatusServiceUnavailable, "identity_unavailable", requestID(ctx))
		return Principal{}, nil, false
	}
	principal, err := h.authenticator.Authenticate(ctx, cookie.Value)
	if err != nil {
		if errors.Is(err, ErrUnauthenticated) {
			writeError(w, http.StatusUnauthorized, "unauthorized", requestID(ctx))
		} else {
			h.logger.ErrorContext(ctx, "authenticate notification request", "request_id", requestID(ctx), "error", err)
			writeError(w, http.StatusServiceUnavailable, "identity_unavailable", requestID(ctx))
		}
		return Principal{}, nil, false
	}
	if !validUUID(principal.ID) {
		writeError(w, http.StatusUnauthorized, "unauthorized", requestID(ctx))
		return Principal{}, nil, false
	}
	return principal, ctx, true
}

func (h *Handler) fail(w http.ResponseWriter, ctx context.Context, operation string, err error) {
	h.logger.ErrorContext(ctx, operation, "request_id", requestID(ctx), "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
}

func parseListQuery(values url.Values) (int, int, bool, error) {
	limit, offset, unread := defaultPageSize, 0, false
	for key, entries := range values {
		if len(entries) != 1 {
			return 0, 0, false, fmt.Errorf("duplicate query parameter %s", key)
		}
		value := strings.TrimSpace(entries[0])
		switch key {
		case "limit", "offset":
			number, err := parseInteger(value)
			if err != nil {
				return 0, 0, false, err
			}
			if key == "limit" {
				if number < 1 || number > maxPageSize {
					return 0, 0, false, errors.New("limit out of range")
				}
				limit = number
			} else {
				if number > maxPageOffset {
					return 0, 0, false, errors.New("offset out of range")
				}
				offset = number
			}
		case "unread":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return 0, 0, false, errors.New("unread must be boolean")
			}
			unread = parsed
		default:
			return 0, 0, false, fmt.Errorf("unknown query parameter %s", key)
		}
	}
	return limit, offset, unread, nil
}

func parseInteger(value string) (int, error) {
	if value == "" {
		return 0, errors.New("empty integer")
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, errors.New("invalid integer")
		}
	}
	return strconv.Atoi(value)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return false
	}
	var extra any
	return errors.Is(decoder.Decode(&extra), io.EOF)
}

func validEventType(value string) bool {
	for _, supported := range supportedEventTypes {
		if value == supported {
			return true
		}
	}
	return false
}

func validUUID(value string) bool { return uuidPattern.MatchString(value) }

func (h *Handler) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			h.logger.ErrorContext(r.Context(), "generate notification request ID", "error", err)
			http.Error(w, "request ID unavailable", http.StatusInternalServerError)
			return
		}
		requestID := hex.EncodeToString(id[:])
		ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		origin, err := url.Parse(r.Header.Get("Origin"))
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
			scheme = "https"
		}
		if err != nil || origin == nil || !strings.EqualFold(origin.Host, r.Host) || origin.Scheme != scheme ||
			origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
			writeError(w, http.StatusForbidden, "origin_not_allowed", requestID(r.Context()))
			return
		}
		next.ServeHTTP(w, r)
	})
}

type requestIDKey struct{}

func requestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func writeError(w http.ResponseWriter, status int, code, id string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "request_id": id}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "response encoding failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(append(encoded, '\n'))
}
