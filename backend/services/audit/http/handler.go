package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mothercat-enterprise/mothercat/backend/services/audit/internal/domain"
)

const (
	sessionCookieName = "mothercat_session"
	defaultPageSize   = 50
	maxPageSize       = 100
	maxPageOffset     = 1_000_000
)

var eventIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

var ErrUnauthenticated = errors.New("unauthenticated")

type Principal struct {
	Permissions []string
}

type Authenticator interface {
	Authenticate(context.Context, string) (Principal, error)
}

type Reader interface {
	List(context.Context, domain.EventFilter) ([]domain.EventRecord, int, error)
	Get(context.Context, string) (domain.EventRecord, error)
}

type Handler struct {
	reader        Reader
	authenticator Authenticator
	logger        *slog.Logger
}

func NewHandler(reader Reader, authenticator Authenticator, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	handler := &Handler{reader: reader, authenticator: authenticator, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/audit-events", handler.list)
	mux.HandleFunc("GET /api/v1/audit-events/{eventID}", handler.get)
	return handler.requestLoggingMiddleware(mux)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorize(w, r)
	if !ok {
		return
	}
	defer cancel()

	filter, err := parseFilter(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_query", requestID(ctx))
		return
	}
	events, total, err := h.reader.List(ctx, filter)
	if err != nil {
		h.fail(w, ctx, "list audit events", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"page": map[string]int{
			"limit": filter.Limit, "offset": filter.Offset, "total": total,
		},
	})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorize(w, r)
	if !ok {
		return
	}
	defer cancel()
	id := r.PathValue("eventID")
	if !eventIDPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "invalid_event_id", requestID(ctx))
		return
	}
	event, err := h.reader.Get(ctx, id)
	if err != nil {
		h.fail(w, ctx, "get audit event", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"event": event})
}

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) (context.Context, context.CancelFunc, bool) {
	if h.reader == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(r.Context()))
		return nil, nil, false
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", requestID(r.Context()))
		return nil, nil, false
	}
	if h.authenticator == nil {
		writeError(w, http.StatusServiceUnavailable, "identity_unavailable", requestID(r.Context()))
		return nil, nil, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	ctx = context.WithValue(ctx, requestIDContextKey{}, requestID(r.Context()))
	principal, err := h.authenticator.Authenticate(ctx, cookie.Value)
	if err != nil {
		cancel()
		if errors.Is(err, ErrUnauthenticated) {
			writeError(w, http.StatusUnauthorized, "unauthorized", requestID(r.Context()))
		} else {
			h.logger.ErrorContext(ctx, "authenticate audit request",
				"request_id", requestID(ctx), "error", err)
			writeError(w, http.StatusServiceUnavailable, "identity_unavailable", requestID(r.Context()))
		}
		return nil, nil, false
	}
	if !hasPermission(principal.Permissions, "audit.read") {
		cancel()
		writeError(w, http.StatusForbidden, "permission_denied", requestID(r.Context()))
		return nil, nil, false
	}
	return ctx, cancel, true
}

func parseFilter(values url.Values) (domain.EventFilter, error) {
	filter := domain.EventFilter{Limit: defaultPageSize}
	allowed := map[string]bool{
		"actor_id": true, "action": true, "resource_type": true, "resource_id": true,
		"from": true, "to": true, "limit": true, "offset": true,
	}
	for key, entries := range values {
		if !allowed[key] || len(entries) != 1 {
			return domain.EventFilter{}, fmt.Errorf("invalid query parameter %q", key)
		}
		value := strings.TrimSpace(entries[0])
		if len(value) > queryFieldMaxLength(key) {
			return domain.EventFilter{}, fmt.Errorf("query parameter %q is too long", key)
		}
		switch key {
		case "actor_id":
			filter.ActorID = value
		case "action":
			filter.Action = value
		case "resource_type":
			filter.ResourceType = value
		case "resource_id":
			filter.ResourceID = value
		case "from", "to":
			if value == "" {
				return domain.EventFilter{}, fmt.Errorf("query parameter %q is empty", key)
			}
			timestamp, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return domain.EventFilter{}, fmt.Errorf("query parameter %q must be RFC3339", key)
			}
			if key == "from" {
				filter.From = &timestamp
			} else {
				filter.To = &timestamp
			}
		case "limit":
			number, err := parseNonNegativeInt(value)
			if err != nil || number < 1 || number > maxPageSize {
				return domain.EventFilter{}, errors.New("limit is out of range")
			}
			filter.Limit = number
		case "offset":
			number, err := parseNonNegativeInt(value)
			if err != nil || number > maxPageOffset {
				return domain.EventFilter{}, errors.New("offset is out of range")
			}
			filter.Offset = number
		}
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return domain.EventFilter{}, errors.New("from must not be after to")
	}
	return filter, nil
}

func queryFieldMaxLength(key string) int {
	switch key {
	case "action":
		return 160
	case "resource_type":
		return 100
	case "limit", "offset":
		return 10
	case "from", "to":
		return 40
	default:
		return 200
	}
}

func parseNonNegativeInt(value string) (int, error) {
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

func (h *Handler) fail(w http.ResponseWriter, ctx context.Context, operation string, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", requestID(ctx))
		return
	}
	h.logger.ErrorContext(ctx, operation, "request_id", requestID(ctx), "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
}

func (h *Handler) requestLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			h.logger.ErrorContext(r.Context(), "generate audit request ID", "error", err)
			http.Error(w, "request ID unavailable", http.StatusInternalServerError)
			return
		}
		idString := hex.EncodeToString(id[:])
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, idString)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", idString)
		writer := &statusWriter{ResponseWriter: w}
		started := time.Now()
		next.ServeHTTP(writer, r)
		h.logger.InfoContext(ctx, "Audit HTTP request",
			"request_id", idString, "method", r.Method, "route", r.Pattern,
			"status", writer.status, "duration_ms", time.Since(started).Milliseconds())
	})
}

type requestIDContextKey struct{}

func requestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}

func hasPermission(permissions []string, required string) bool {
	for _, permission := range permissions {
		if permission == required {
			return true
		}
	}
	return false
}

func writeError(w http.ResponseWriter, status int, code, id string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{
		"code": code, "request_id": id,
	}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "response encoding failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(encoded, '\n'))
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}
