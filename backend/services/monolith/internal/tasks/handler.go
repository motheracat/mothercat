package tasks

import (
	"bytes"
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

var (
	ErrUnauthenticated = errors.New("unauthenticated")
	errNotFound        = errors.New("task not found")
	errConflict        = errors.New("task conflict")
	errPrecondition    = errors.New("task version precondition failed")
	errForbidden       = errors.New("task operation forbidden")
)

type Principal struct {
	ID          string
	Permissions []string
}

func notifyTaskRecipient(ctx context.Context, tx *sql.Tx, recipient, actor, taskID, eventType, title, body string) error {
	if recipient == actor {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO notifications.notifications (
			recipient_account_id, event_type, title, body, resource_type, resource_id, source_event_id
		)
		SELECT $1::uuid, $3, $4, $5, 'task', $2::uuid, gen_random_uuid()
		WHERE coalesce((
			SELECT enabled FROM notifications.notification_preferences
			WHERE account_id=$1::uuid AND event_type=$3
		), true)
		ON CONFLICT (source_event_id, recipient_account_id) DO NOTHING
	`, recipient, taskID, eventType, title, body)
	if err != nil {
		return fmt.Errorf("create task notification: %w", err)
	}
	return nil
}

func notifyTaskCreator(ctx context.Context, tx *sql.Tx, taskID, actor, eventType, title, body string) error {
	var creator string
	if err := tx.QueryRowContext(ctx, `SELECT created_by::text FROM tasks.tasks WHERE id=$1::uuid`,
		taskID).Scan(&creator); err != nil {
		return fmt.Errorf("load task creator for notification: %w", err)
	}
	return notifyTaskRecipient(ctx, tx, creator, actor, taskID, eventType, title, body)
}

func notifyTaskAssignee(ctx context.Context, tx *sql.Tx, taskID, actor, eventType, title, body string) error {
	var assignee sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT assignee_account_id::text FROM tasks.task_assignments
		WHERE task_id=$1::uuid AND unassigned_at IS NULL
	`, taskID).Scan(&assignee); err != nil {
		return fmt.Errorf("load task assignee for notification: %w", err)
	}
	if !assignee.Valid {
		return nil
	}
	return notifyTaskRecipient(ctx, tx, assignee.String, actor, taskID, eventType, title, body)
}

func notifyTaskParticipants(ctx context.Context, tx *sql.Tx, taskID, actor, eventType, title, body string) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT created_by::text FROM tasks.tasks WHERE id=$1::uuid
		UNION
		SELECT assignee_account_id::text FROM tasks.task_assignments
		WHERE task_id=$1::uuid AND unassigned_at IS NULL
	`, taskID)
	if err != nil {
		return fmt.Errorf("load task participants for notification: %w", err)
	}
	var recipients []string
	for rows.Next() {
		var recipient string
		if err := rows.Scan(&recipient); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan task notification recipient: %w", err)
		}
		recipients = append(recipients, recipient)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("read task notification recipients: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close task notification recipients: %w", err)
	}
	for _, recipient := range recipients {
		if err := notifyTaskRecipient(ctx, tx, recipient, actor, taskID, eventType, title, body); err != nil {
			return err
		}
	}
	return nil
}

func parsePage(values url.Values) (int, int, error) {
	limit, offset := defaultPageSize, 0
	for key, entries := range values {
		if (key != "limit" && key != "offset") || len(entries) != 1 {
			return 0, 0, fmt.Errorf("invalid query parameter %q", key)
		}
		number, err := parseInteger(strings.TrimSpace(entries[0]))
		if err != nil {
			return 0, 0, err
		}
		if key == "limit" {
			if number < 1 || number > maxPageSize {
				return 0, 0, errors.New("limit out of range")
			}
			limit = number
		} else {
			if number > maxPageOffset {
				return 0, 0, errors.New("offset out of range")
			}
			offset = number
		}
	}
	return limit, offset, nil
}

type Authenticator interface {
	Authenticate(context.Context, string) (Principal, error)
}

type Handler struct {
	db            *sql.DB
	authenticator Authenticator
	logger        *slog.Logger
}

func NewHandler(db *sql.DB, authenticator Authenticator, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{db: db, authenticator: authenticator, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/tasks", h.list)
	mux.HandleFunc("POST /api/v1/tasks", h.create)
	mux.HandleFunc("GET /api/v1/me/tasks", h.myTasks)
	mux.HandleFunc("GET /api/v1/tasks/{taskID}", h.get)
	mux.HandleFunc("PATCH /api/v1/tasks/{taskID}", h.update)
	mux.HandleFunc("POST /api/v1/tasks/{taskID}/assign", h.assign)
	mux.HandleFunc("POST /api/v1/tasks/{taskID}/start", h.start)
	mux.HandleFunc("POST /api/v1/tasks/{taskID}/submit", h.submit)
	mux.HandleFunc("POST /api/v1/tasks/{taskID}/approve", h.approve)
	mux.HandleFunc("POST /api/v1/tasks/{taskID}/request-changes", h.requestChanges)
	mux.HandleFunc("POST /api/v1/tasks/{taskID}/reject", h.reject)
	mux.HandleFunc("POST /api/v1/tasks/{taskID}/cancel", h.cancel)
	mux.HandleFunc("GET /api/v1/tasks/{taskID}/history", h.history)
	mux.HandleFunc("GET /api/v1/tasks/{taskID}/comments", h.listComments)
	mux.HandleFunc("POST /api/v1/tasks/{taskID}/comments", h.addComment)
	return h.requestLoggingMiddleware(h.csrfMiddleware(mux))
}

type task struct {
	ID                string      `json:"id"`
	Title             string      `json:"title"`
	Description       string      `json:"description"`
	Priority          string      `json:"priority"`
	Status            string      `json:"status"`
	CreatedBy         string      `json:"created_by"`
	DepartmentID      string      `json:"department_id"`
	AssigneeAccountID *string     `json:"assignee_account_id,omitempty"`
	DueAt             *time.Time  `json:"due_at,omitempty"`
	Version           int         `json:"version"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
	LatestSubmission  *submission `json:"latest_submission,omitempty"`
}

type submission struct {
	ID                 string    `json:"id"`
	SubmissionNumber   int       `json:"submission_number"`
	Summary            string    `json:"summary"`
	Links              []string  `json:"links"`
	SubmitterAccountID string    `json:"submitter_account_id"`
	SubmittedAt        time.Time `json:"submitted_at"`
}

type historyEntry struct {
	ID         string    `json:"id"`
	ActorID    string    `json:"actor_account_id"`
	FromStatus *string   `json:"from_status,omitempty"`
	ToStatus   string    `json:"to_status"`
	Reason     string    `json:"reason"`
	OccurredAt time.Time `json:"occurred_at"`
}

type comment struct {
	ID        string    `json:"id"`
	AuthorID  string    `json:"author_account_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type listFilter struct {
	Status       string
	AssigneeID   string
	DepartmentID string
	DueFrom      *time.Time
	DueTo        *time.Time
	Limit        int
	Offset       int
	Mine         bool
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	filter, err := parseFilter(r.URL.Query(), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_query", requestID(ctx))
		return
	}
	h.listTasks(w, ctx, principal, filter)
}

func (h *Handler) myTasks(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	filter, err := parseFilter(r.URL.Query(), true)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_query", requestID(ctx))
		return
	}
	h.listTasks(w, ctx, principal, filter)
}

func (h *Handler) listTasks(w http.ResponseWriter, ctx context.Context, principal Principal, filter listFilter) {
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	where, args := visibilityWhere(principal, filter.Mine)
	addFilter := func(condition string, value any) {
		args = append(args, value)
		where += fmt.Sprintf(" AND "+condition, len(args))
	}
	if filter.Status != "" {
		addFilter("t.status = $%d", filter.Status)
	}
	if filter.AssigneeID != "" {
		addFilter(`EXISTS (SELECT 1 FROM tasks.task_assignments fa
			WHERE fa.task_id=t.id AND fa.unassigned_at IS NULL AND fa.assignee_account_id=$%d::uuid)`, filter.AssigneeID)
	}
	if filter.DepartmentID != "" {
		addFilter("t.department_id = $%d::uuid", filter.DepartmentID)
	}
	if filter.DueFrom != nil {
		addFilter("t.due_at >= $%d", *filter.DueFrom)
	}
	if filter.DueTo != nil {
		addFilter("t.due_at <= $%d", *filter.DueTo)
	}

	var total int
	if err := h.db.QueryRowContext(ctx, `SELECT count(*) FROM tasks.tasks t WHERE `+where, args...).Scan(&total); err != nil {
		h.fail(w, ctx, "count tasks", err)
		return
	}
	pageArgs := append(append([]any(nil), args...), filter.Limit, filter.Offset)
	rows, err := h.db.QueryContext(ctx, `
		SELECT `+taskColumns+`
		FROM tasks.tasks t
		LEFT JOIN LATERAL (
			SELECT ta.assignee_account_id FROM tasks.task_assignments ta
			WHERE ta.task_id=t.id AND ta.unassigned_at IS NULL
		) active_assignment ON true
		LEFT JOIN LATERAL (
			SELECT ts.id, ts.submission_number, ts.summary, ts.links,
			       ts.submitter_account_id, ts.submitted_at
			FROM tasks.task_submissions ts
			WHERE ts.task_id=t.id ORDER BY ts.submission_number DESC LIMIT 1
		) latest_submission ON true
		WHERE `+where+fmt.Sprintf(" ORDER BY t.created_at DESC, t.id LIMIT $%d OFFSET $%d",
		len(pageArgs)-1, len(pageArgs)), pageArgs...)
	if err != nil {
		h.fail(w, ctx, "list tasks", err)
		return
	}
	defer rows.Close()
	tasks := make([]task, 0)
	for rows.Next() {
		item, scanErr := scanTask(rows)
		if scanErr != nil {
			h.fail(w, ctx, "scan task", scanErr)
			return
		}
		tasks = append(tasks, item)
	}
	if err := rows.Err(); err != nil {
		h.fail(w, ctx, "read tasks", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tasks": tasks, "page": map[string]int{"limit": filter.Limit, "offset": filter.Offset, "total": total},
	})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	id, ok := h.taskID(w, r, ctx)
	if !ok {
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	item, err := h.getVisibleTask(ctx, principal, id)
	if err != nil {
		h.fail(w, ctx, "get task", err)
		return
	}
	w.Header().Set("ETag", etag(item.Version))
	writeJSON(w, http.StatusOK, map[string]any{"task": item})
}

type createRequest struct {
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Priority     string     `json:"priority"`
	DepartmentID string     `json:"department_id"`
	DueAt        *time.Time `json:"due_at"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authenticatePermission(w, r, "tasks.create")
	if !ok {
		return
	}
	var input createRequest
	if !decodeJSON(w, r, &input) {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID(ctx))
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.Priority = strings.TrimSpace(input.Priority)
	input.DepartmentID = strings.TrimSpace(input.DepartmentID)
	if input.Priority == "" {
		input.Priority = "normal"
	}
	if len(input.Title) == 0 || len(input.Title) > 200 ||
		len(input.Description) > 20000 || !validUUID(input.DepartmentID) ||
		!validPriority(input.Priority) || (input.DueAt != nil && input.DueAt.IsZero()) {
		writeError(w, http.StatusBadRequest, "invalid_task", requestID(ctx))
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		h.fail(w, ctx, "begin task creation", err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var member bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM organization.department_memberships
			WHERE department_id=$1::uuid AND account_id=$2::uuid)
	`, input.DepartmentID, principal.ID).Scan(&member); err != nil {
		h.fail(w, ctx, "check task department scope", err)
		return
	}
	if !member {
		writeError(w, http.StatusForbidden, "permission_denied", requestID(ctx))
		return
	}
	item, err := scanTask(tx.QueryRowContext(ctx, `
		INSERT INTO tasks.tasks (title, description, priority, created_by, department_id, due_at)
		VALUES ($1, $2, $3, $4::uuid, $5::uuid, $6)
		RETURNING `+taskColumnsBare, input.Title, input.Description, input.Priority,
		principal.ID, input.DepartmentID, input.DueAt))
	if err != nil {
		h.fail(w, ctx, "create task", err)
		return
	}
	if err := recordStatusAndAudit(ctx, tx, principal.ID, item.ID, "", "draft", "task.created", ""); err != nil {
		h.fail(w, ctx, "record task creation", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, ctx, "commit task creation", err)
		return
	}
	w.Header().Set("ETag", etag(item.Version))
	writeJSON(w, http.StatusCreated, map[string]any{"task": item})
}

type updateRequest struct {
	Title       *string    `json:"title"`
	Description *string    `json:"description"`
	Priority    *string    `json:"priority"`
	DueAt       *time.Time `json:"-"`
	DueAtSet    bool       `json:"-"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	id, ok := h.taskID(w, r, ctx)
	if !ok {
		return
	}
	expected, valid := parseETag(r.Header.Get("If-Match"))
	if !valid {
		writeError(w, http.StatusPreconditionRequired, "if_match_required", requestID(ctx))
		return
	}
	var input updateRequest
	var rawUpdate struct {
		Title       *string         `json:"title"`
		Description *string         `json:"description"`
		Priority    *string         `json:"priority"`
		DueAt       json.RawMessage `json:"due_at"`
	}
	if !decodeJSON(w, r, &rawUpdate) || (rawUpdate.Title == nil && rawUpdate.Description == nil &&
		rawUpdate.Priority == nil && rawUpdate.DueAt == nil) {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID(ctx))
		return
	}
	input.Title, input.Description, input.Priority = rawUpdate.Title, rawUpdate.Description, rawUpdate.Priority
	if rawUpdate.DueAt != nil {
		input.DueAtSet = true
		if !bytes.Equal(bytes.TrimSpace(rawUpdate.DueAt), []byte("null")) {
			var dueAt time.Time
			if err := json.Unmarshal(rawUpdate.DueAt, &dueAt); err != nil || dueAt.IsZero() {
				writeError(w, http.StatusBadRequest, "invalid_task", requestID(ctx))
				return
			}
			input.DueAt = &dueAt
		}
	}
	if input.Title != nil {
		*input.Title = strings.TrimSpace(*input.Title)
		if len(*input.Title) == 0 || len(*input.Title) > 200 {
			writeError(w, http.StatusBadRequest, "invalid_task", requestID(ctx))
			return
		}
	}
	if input.Description != nil {
		*input.Description = strings.TrimSpace(*input.Description)
		if len(*input.Description) > 20000 {
			writeError(w, http.StatusBadRequest, "invalid_task", requestID(ctx))
			return
		}
	}
	if input.Priority != nil {
		*input.Priority = strings.TrimSpace(*input.Priority)
		if !validPriority(*input.Priority) {
			writeError(w, http.StatusBadRequest, "invalid_task", requestID(ctx))
			return
		}
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	item, err := h.updateTask(ctx, principal, id, expected, input)
	if err != nil {
		h.fail(w, ctx, "update task", err)
		return
	}
	w.Header().Set("ETag", etag(item.Version))
	writeJSON(w, http.StatusOK, map[string]any{"task": item})
}

type assignRequest struct {
	AssigneeAccountID string `json:"assignee_account_id"`
}

func (h *Handler) assign(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authenticatePermission(w, r, "tasks.assign")
	if !ok {
		return
	}
	id, ok := h.taskID(w, r, ctx)
	if !ok {
		return
	}
	var input assignRequest
	if !decodeJSON(w, r, &input) || !validUUID(strings.TrimSpace(input.AssigneeAccountID)) {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID(ctx))
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	item, err := h.assignTask(ctx, principal, id, strings.TrimSpace(input.AssigneeAccountID))
	if err != nil {
		h.fail(w, ctx, "assign task", err)
		return
	}
	w.Header().Set("ETag", etag(item.Version))
	writeJSON(w, http.StatusOK, map[string]any{"task": item})
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, "in_progress")
}

type submitRequest struct {
	Summary string   `json:"summary"`
	Links   []string `json:"links"`
}

func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	id, ok := h.taskID(w, r, ctx)
	if !ok {
		return
	}
	var input submitRequest
	if !decodeJSON(w, r, &input) {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID(ctx))
		return
	}
	input.Summary = strings.TrimSpace(input.Summary)
	if len(input.Summary) == 0 || len(input.Summary) > 20000 || len(input.Links) > 10 {
		writeError(w, http.StatusBadRequest, "invalid_submission", requestID(ctx))
		return
	}
	for _, link := range input.Links {
		link = strings.TrimSpace(link)
		parsed, err := url.ParseRequestURI(link)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || len(link) > 2048 {
			writeError(w, http.StatusBadRequest, "invalid_submission", requestID(ctx))
			return
		}
	}
	for i := range input.Links {
		input.Links[i] = strings.TrimSpace(input.Links[i])
	}
	if input.Links == nil {
		input.Links = []string{}
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	item, err := h.submitTask(ctx, principal, id, input)
	if err != nil {
		h.fail(w, ctx, "submit task", err)
		return
	}
	w.Header().Set("ETag", etag(item.Version))
	writeJSON(w, http.StatusOK, map[string]any{"task": item})
}

type reviewRequest struct {
	Feedback string `json:"feedback"`
}

func (h *Handler) approve(w http.ResponseWriter, r *http.Request) {
	h.review(w, r, "approved")
}

func (h *Handler) requestChanges(w http.ResponseWriter, r *http.Request) {
	h.review(w, r, "changes_requested")
}

func (h *Handler) reject(w http.ResponseWriter, r *http.Request) {
	h.review(w, r, "rejected")
}

func (h *Handler) review(w http.ResponseWriter, r *http.Request, decision string) {
	principal, ctx, ok := h.authenticatePermission(w, r, "tasks.review")
	if !ok {
		return
	}
	id, ok := h.taskID(w, r, ctx)
	if !ok {
		return
	}
	var input reviewRequest
	if !decodeJSON(w, r, &input) {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID(ctx))
		return
	}
	input.Feedback = strings.TrimSpace(input.Feedback)
	if len(input.Feedback) > 2000 ||
		((decision == "rejected" || decision == "changes_requested") && input.Feedback == "") {
		writeError(w, http.StatusBadRequest, "invalid_feedback", requestID(ctx))
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	item, err := h.reviewTask(ctx, principal, id, decision, input.Feedback)
	if err != nil {
		h.fail(w, ctx, "review task", err)
		return
	}
	w.Header().Set("ETag", etag(item.Version))
	writeJSON(w, http.StatusOK, map[string]any{"task": item})
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authenticatePermission(w, r, "tasks.cancel")
	if !ok {
		return
	}
	id, ok := h.taskID(w, r, ctx)
	if !ok {
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	item, err := h.transitionTask(ctx, principal, id, "cancelled")
	if err != nil {
		h.fail(w, ctx, "cancel task", err)
		return
	}
	w.Header().Set("ETag", etag(item.Version))
	writeJSON(w, http.StatusOK, map[string]any{"task": item})
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	h.listTaskChildren(w, r, "history")
}

func (h *Handler) listComments(w http.ResponseWriter, r *http.Request) {
	h.listTaskChildren(w, r, "comments")
}

func (h *Handler) listTaskChildren(w http.ResponseWriter, r *http.Request, kind string) {
	principal, ctx, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	id, ok := h.taskID(w, r, ctx)
	if !ok {
		return
	}
	limit, offset, err := parsePage(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_query", requestID(ctx))
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	if _, err := h.getVisibleTask(ctx, principal, id); err != nil {
		h.fail(w, ctx, "authorize task details", err)
		return
	}
	if kind == "history" {
		var total int
		if err := h.db.QueryRowContext(ctx,
			`SELECT count(*) FROM tasks.task_status_history WHERE task_id=$1::uuid`, id,
		).Scan(&total); err != nil {
			h.fail(w, ctx, "count task history", err)
			return
		}
		rows, err := h.db.QueryContext(ctx, `
			SELECT id::text, actor_account_id::text, from_status, to_status, reason, occurred_at
			FROM tasks.task_status_history WHERE task_id=$1::uuid
			ORDER BY occurred_at, id LIMIT $2 OFFSET $3
		`, id, limit, offset)
		if err != nil {
			h.fail(w, ctx, "list task history", err)
			return
		}
		defer rows.Close()
		result := make([]historyEntry, 0)
		for rows.Next() {
			var item historyEntry
			var from sql.NullString
			if err := rows.Scan(&item.ID, &item.ActorID, &from, &item.ToStatus, &item.Reason, &item.OccurredAt); err != nil {
				h.fail(w, ctx, "scan task history", err)
				return
			}
			if from.Valid {
				item.FromStatus = &from.String
			}
			result = append(result, item)
		}
		if err := rows.Err(); err != nil {
			h.fail(w, ctx, "read task history", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"history": result, "page": map[string]int{"limit": limit, "offset": offset, "total": total},
		})
		return
	}
	var total int
	if err := h.db.QueryRowContext(ctx,
		`SELECT count(*) FROM tasks.task_comments WHERE task_id=$1::uuid`, id,
	).Scan(&total); err != nil {
		h.fail(w, ctx, "count task comments", err)
		return
	}
	rows, err := h.db.QueryContext(ctx, `
		SELECT id::text, author_account_id::text, body, created_at
		FROM tasks.task_comments WHERE task_id=$1::uuid
		ORDER BY created_at, id LIMIT $2 OFFSET $3
	`, id, limit, offset)
	if err != nil {
		h.fail(w, ctx, "list task comments", err)
		return
	}
	defer rows.Close()
	result := make([]comment, 0)
	for rows.Next() {
		var item comment
		if err := rows.Scan(&item.ID, &item.AuthorID, &item.Body, &item.CreatedAt); err != nil {
			h.fail(w, ctx, "scan task comments", err)
			return
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		h.fail(w, ctx, "read task comments", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"comments": result, "page": map[string]int{"limit": limit, "offset": offset, "total": total},
	})
}

type addCommentRequest struct {
	Body string `json:"body"`
}

func (h *Handler) addComment(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	id, ok := h.taskID(w, r, ctx)
	if !ok {
		return
	}
	var input addCommentRequest
	if !decodeJSON(w, r, &input) {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID(ctx))
		return
	}
	input.Body = strings.TrimSpace(input.Body)
	if len(input.Body) == 0 || len(input.Body) > 5000 {
		writeError(w, http.StatusBadRequest, "invalid_comment", requestID(ctx))
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		h.fail(w, ctx, "begin task comment", err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := lockVisibleTask(ctx, tx, principal, id); err != nil {
		h.fail(w, ctx, "authorize task comment", err)
		return
	}
	var item comment
	err = tx.QueryRowContext(ctx, `
		INSERT INTO tasks.task_comments (task_id, author_account_id, body)
		VALUES ($1::uuid, $2::uuid, $3)
		RETURNING id::text, author_account_id::text, body, created_at
	`, id, principal.ID, input.Body).Scan(&item.ID, &item.AuthorID, &item.Body, &item.CreatedAt)
	if err != nil {
		h.fail(w, ctx, "create task comment", err)
		return
	}
	if err := recordAudit(ctx, tx, principal.ID, id, "task.comment_added"); err != nil {
		h.fail(w, ctx, "audit task comment", err)
		return
	}
	if err := notifyTaskParticipants(ctx, tx, id, principal.ID,
		"task.comment_added", "New task comment", "A new comment was added to a task."); err != nil {
		h.fail(w, ctx, "notify task comment", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, ctx, "commit task comment", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"comment": item})
}

func (h *Handler) transition(w http.ResponseWriter, r *http.Request, to string) {
	principal, ctx, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	id, ok := h.taskID(w, r, ctx)
	if !ok {
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	item, err := h.transitionTask(ctx, principal, id, to)
	if err != nil {
		h.fail(w, ctx, "transition task", err)
		return
	}
	w.Header().Set("ETag", etag(item.Version))
	writeJSON(w, http.StatusOK, map[string]any{"task": item})
}

func (h *Handler) taskID(w http.ResponseWriter, r *http.Request, ctx context.Context) (string, bool) {
	id := r.PathValue("taskID")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid_task_id", requestID(ctx))
		return "", false
	}
	return id, true
}

func (h *Handler) authenticatePermission(w http.ResponseWriter, r *http.Request, permission string) (Principal, context.Context, bool) {
	principal, ctx, ok := h.authenticate(w, r)
	if !ok {
		return Principal{}, nil, false
	}
	if !hasPermission(principal, permission) {
		writeError(w, http.StatusForbidden, "permission_denied", requestID(ctx))
		return Principal{}, nil, false
	}
	return principal, ctx, true
}

func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (Principal, context.Context, bool) {
	ctx := r.Context()
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", requestID(r.Context()))
		return Principal{}, nil, false
	}
	if h.authenticator == nil {
		writeError(w, http.StatusServiceUnavailable, "identity_unavailable", requestID(r.Context()))
		return Principal{}, nil, false
	}
	principal, err := h.authenticator.Authenticate(ctx, cookie.Value)
	if err != nil {
		if errors.Is(err, ErrUnauthenticated) {
			writeError(w, http.StatusUnauthorized, "unauthorized", requestID(r.Context()))
		} else {
			h.logger.ErrorContext(ctx, "authenticate task request", "error", err, "request_id", requestID(r.Context()))
			writeError(w, http.StatusServiceUnavailable, "identity_unavailable", requestID(r.Context()))
		}
		return Principal{}, nil, false
	}
	if !validUUID(principal.ID) {
		writeError(w, http.StatusUnauthorized, "unauthorized", requestID(r.Context()))
		return Principal{}, nil, false
	}
	return principal, ctx, true
}

func (h *Handler) getVisibleTask(ctx context.Context, principal Principal, id string) (task, error) {
	where, args := visibilityWhere(principal, false)
	args = append(args, id)
	item, err := scanTask(h.db.QueryRowContext(ctx, `
		SELECT `+taskColumns+`
		FROM tasks.tasks t
		LEFT JOIN LATERAL (
			SELECT ta.assignee_account_id FROM tasks.task_assignments ta
			WHERE ta.task_id=t.id AND ta.unassigned_at IS NULL
		) active_assignment ON true
		LEFT JOIN LATERAL (
			SELECT ts.id, ts.submission_number, ts.summary, ts.links,
			       ts.submitter_account_id, ts.submitted_at
			FROM tasks.task_submissions ts
			WHERE ts.task_id=t.id ORDER BY ts.submission_number DESC LIMIT 1
		) latest_submission ON true
		WHERE `+where+fmt.Sprintf(" AND t.id=$%d::uuid", len(args)), args...))
	if errors.Is(err, sql.ErrNoRows) {
		return task{}, errNotFound
	}
	if err != nil {
		return task{}, fmt.Errorf("query task: %w", err)
	}
	return item, nil
}

func (h *Handler) updateTask(ctx context.Context, principal Principal, id string, expected int, input updateRequest) (task, error) {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return task{}, fmt.Errorf("begin task update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := lockVisibleTask(ctx, tx, principal, id)
	if err != nil {
		return task{}, err
	}
	if current.CreatedBy != principal.ID || current.Status != "draft" {
		return task{}, errForbidden
	}
	if current.Version != expected {
		return task{}, errPrecondition
	}
	item, err := scanTask(tx.QueryRowContext(ctx, `
		UPDATE tasks.tasks SET
			title=coalesce($2, title),
			description=coalesce($3, description),
			priority=coalesce($4, priority),
			due_at=CASE WHEN $5 THEN $6::timestamptz ELSE due_at END,
			version=version+1, updated_at=now()
		WHERE id=$1::uuid AND version=$7
		RETURNING `+taskColumnsBare, id, input.Title, input.Description, input.Priority,
		input.DueAtSet, input.DueAt, expected))
	if err != nil {
		return task{}, fmt.Errorf("update task: %w", err)
	}
	if err := recordAudit(ctx, tx, principal.ID, id, "task.updated"); err != nil {
		return task{}, err
	}
	if err := tx.Commit(); err != nil {
		return task{}, fmt.Errorf("commit task update: %w", err)
	}
	return item, nil
}

func (h *Handler) assignTask(ctx context.Context, principal Principal, id, assigneeID string) (task, error) {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return task{}, fmt.Errorf("begin task assignment: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := lockVisibleTask(ctx, tx, principal, id)
	if err != nil {
		return task{}, err
	}
	if current.Status != "draft" && current.Status != "assigned" &&
		current.Status != "in_progress" && current.Status != "changes_requested" {
		return task{}, errConflict
	}
	var valid bool
	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM organization.staff_members sm
			JOIN identity.user_accounts ua ON ua.id=sm.account_id
			WHERE sm.account_id=$1::uuid AND sm.department_id=$2::uuid
			  AND sm.employment_status='active' AND ua.is_active
		) AND (
			EXISTS (SELECT 1 FROM organization.department_memberships dm
				WHERE dm.department_id=$2::uuid AND dm.account_id=$3::uuid)
		)
	`, assigneeID, current.DepartmentID, principal.ID).Scan(&valid)
	if err != nil {
		return task{}, fmt.Errorf("validate task assignee and department: %w", err)
	}
	if !valid {
		return task{}, errForbidden
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE tasks.task_assignments SET unassigned_at=now()
		WHERE task_id=$1::uuid AND unassigned_at IS NULL
	`, id); err != nil {
		return task{}, fmt.Errorf("close prior task assignment: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tasks.task_assignments (task_id, assignee_account_id, assigned_by)
		VALUES ($1::uuid, $2::uuid, $3::uuid)
	`, id, assigneeID, principal.ID); err != nil {
		return task{}, fmt.Errorf("assign task: %w", err)
	}
	item, err := scanTask(tx.QueryRowContext(ctx, `
		UPDATE tasks.tasks SET status='assigned', version=version+1, updated_at=now()
		WHERE id=$1::uuid RETURNING `+taskColumnsBare, id))
	if err != nil {
		return task{}, fmt.Errorf("update assigned task: %w", err)
	}
	if err := recordStatusAndAudit(ctx, tx, principal.ID, id, current.Status, "assigned", "task.assigned", ""); err != nil {
		return task{}, err
	}
	if err := notifyTaskRecipient(ctx, tx, assigneeID, principal.ID, id,
		"task.assigned", "Task assigned", "You have been assigned a task."); err != nil {
		return task{}, err
	}
	if err := tx.Commit(); err != nil {
		return task{}, fmt.Errorf("commit task assignment: %w", err)
	}
	return item, nil
}

func (h *Handler) transitionTask(ctx context.Context, principal Principal, id, to string) (task, error) {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return task{}, fmt.Errorf("begin task transition: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := lockVisibleTask(ctx, tx, principal, id)
	if err != nil {
		return task{}, err
	}
	if to == "in_progress" {
		if current.AssigneeAccountID == nil || *current.AssigneeAccountID != principal.ID ||
			(current.Status != "assigned" && current.Status != "changes_requested") {
			return task{}, errForbidden
		}
	} else {
		if current.Status == "approved" || current.Status == "rejected" || current.Status == "cancelled" {
			return task{}, errConflict
		}
	}
	item, err := scanTask(tx.QueryRowContext(ctx, `
		UPDATE tasks.tasks SET status=$2, version=version+1, updated_at=now()
		WHERE id=$1::uuid RETURNING `+taskColumnsBare, id, to))
	if err != nil {
		return task{}, fmt.Errorf("update task status: %w", err)
	}
	if err := recordStatusAndAudit(ctx, tx, principal.ID, id, current.Status, to, "task."+to, ""); err != nil {
		return task{}, err
	}
	if err := tx.Commit(); err != nil {
		return task{}, fmt.Errorf("commit task transition: %w", err)
	}
	return item, nil
}

func (h *Handler) submitTask(ctx context.Context, principal Principal, id string, input submitRequest) (task, error) {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return task{}, fmt.Errorf("begin task submission: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := lockVisibleTask(ctx, tx, principal, id)
	if err != nil {
		return task{}, err
	}
	if current.AssigneeAccountID == nil || *current.AssigneeAccountID != principal.ID || current.Status != "in_progress" {
		return task{}, errForbidden
	}
	links, err := json.Marshal(input.Links)
	if err != nil {
		return task{}, fmt.Errorf("encode submission links: %w", err)
	}
	var createdSubmission submission
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO tasks.task_submissions (task_id, submission_number, summary, links, submitter_account_id)
		SELECT $1::uuid, coalesce(max(submission_number), 0)+1, $2, $3::jsonb, $4::uuid
		FROM tasks.task_submissions WHERE task_id=$1::uuid
		RETURNING id::text, submission_number, submitted_at
	`, id, input.Summary, links, principal.ID).Scan(
		&createdSubmission.ID, &createdSubmission.SubmissionNumber, &createdSubmission.SubmittedAt,
	); err != nil {
		return task{}, fmt.Errorf("insert task submission: %w", err)
	}
	item, err := scanTask(tx.QueryRowContext(ctx, `
		UPDATE tasks.tasks SET status='submitted', version=version+1, updated_at=now()
		WHERE id=$1::uuid RETURNING `+taskColumnsBare, id))
	if err != nil {
		return task{}, fmt.Errorf("update submitted task: %w", err)
	}
	if err := recordStatusAndAudit(ctx, tx, principal.ID, id, current.Status, "submitted", "task.submitted", ""); err != nil {
		return task{}, err
	}
	if err := notifyTaskCreator(ctx, tx, id, principal.ID,
		"task.submitted", "Task submitted", "An assigned task has been submitted for review."); err != nil {
		return task{}, err
	}
	if err := tx.Commit(); err != nil {
		return task{}, fmt.Errorf("commit task submission: %w", err)
	}
	createdSubmission.Summary = input.Summary
	createdSubmission.Links = input.Links
	createdSubmission.SubmitterAccountID = principal.ID
	item.LatestSubmission = &createdSubmission
	return item, nil
}

func (h *Handler) reviewTask(ctx context.Context, principal Principal, id, decision, feedback string) (task, error) {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return task{}, fmt.Errorf("begin task review: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := lockVisibleTask(ctx, tx, principal, id)
	if err != nil {
		return task{}, err
	}
	if current.Status != "submitted" {
		return task{}, errConflict
	}
	if current.AssigneeAccountID != nil && *current.AssigneeAccountID == principal.ID {
		return task{}, errForbidden
	}
	var submissionID string
	if err := tx.QueryRowContext(ctx, `
		SELECT id::text FROM tasks.task_submissions
		WHERE task_id=$1::uuid ORDER BY submission_number DESC LIMIT 1
	`, id).Scan(&submissionID); err != nil {
		return task{}, fmt.Errorf("load current task submission: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tasks.task_reviews (task_id, submission_id, reviewer_account_id, decision, feedback)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5)
	`, id, submissionID, principal.ID, decision, feedback); err != nil {
		return task{}, fmt.Errorf("record task review: %w", err)
	}
	item, err := scanTask(tx.QueryRowContext(ctx, `
		UPDATE tasks.tasks SET status=$2, version=version+1, updated_at=now()
		WHERE id=$1::uuid RETURNING `+taskColumnsBare, id, decision))
	if err != nil {
		return task{}, fmt.Errorf("update reviewed task: %w", err)
	}
	if err := recordStatusAndAudit(ctx, tx, principal.ID, id, current.Status, decision, "task."+decision, feedback); err != nil {
		return task{}, err
	}
	title := map[string]string{
		"approved": "Task approved", "changes_requested": "Changes requested", "rejected": "Task rejected",
	}[decision]
	if err := notifyTaskAssignee(ctx, tx, id, principal.ID, "task."+decision, title, feedback); err != nil {
		return task{}, err
	}
	if err := tx.Commit(); err != nil {
		return task{}, fmt.Errorf("commit task review: %w", err)
	}
	return item, nil
}

const taskColumnsBare = `id::text, title, description, priority, status, created_by::text,
	department_id::text, NULL::text, due_at, version, created_at, updated_at,
	NULL::text, NULL::integer, NULL::text, NULL::jsonb, NULL::text, NULL::timestamptz`

const taskColumns = `t.id::text, t.title, t.description, t.priority, t.status, t.created_by::text,
	t.department_id::text, active_assignment.assignee_account_id::text, t.due_at, t.version,
	t.created_at, t.updated_at, latest_submission.id::text, latest_submission.submission_number,
	latest_submission.summary, latest_submission.links, latest_submission.submitter_account_id::text,
	latest_submission.submitted_at`

type rowScanner interface {
	Scan(...any) error
}

func scanTask(row rowScanner) (task, error) {
	var item task
	var assignee, submissionID, submissionSummary, submitter sql.NullString
	var dueAt, submittedAt sql.NullTime
	var submissionNumber sql.NullInt64
	var links []byte
	err := row.Scan(&item.ID, &item.Title, &item.Description, &item.Priority, &item.Status,
		&item.CreatedBy, &item.DepartmentID, &assignee, &dueAt, &item.Version, &item.CreatedAt,
		&item.UpdatedAt, &submissionID, &submissionNumber, &submissionSummary, &links,
		&submitter, &submittedAt)
	if err != nil {
		return task{}, err
	}
	if assignee.Valid {
		item.AssigneeAccountID = &assignee.String
	}
	if dueAt.Valid {
		item.DueAt = &dueAt.Time
	}
	if submissionID.Valid {
		var decodedLinks []string
		if err := json.Unmarshal(links, &decodedLinks); err != nil {
			return task{}, fmt.Errorf("decode task submission links: %w", err)
		}
		if decodedLinks == nil {
			decodedLinks = []string{}
		}
		item.LatestSubmission = &submission{
			ID: submissionID.String, SubmissionNumber: int(submissionNumber.Int64),
			Summary: submissionSummary.String, Links: decodedLinks,
			SubmitterAccountID: submitter.String, SubmittedAt: submittedAt.Time,
		}
	}
	return item, nil
}

func lockVisibleTask(ctx context.Context, tx *sql.Tx, principal Principal, id string) (task, error) {
	where, args := visibilityWhere(principal, false)
	args = append(args, id)
	item, err := scanTask(tx.QueryRowContext(ctx, `
		SELECT `+taskColumns+`
		FROM tasks.tasks t
		LEFT JOIN LATERAL (
			SELECT ta.assignee_account_id FROM tasks.task_assignments ta
			WHERE ta.task_id=t.id AND ta.unassigned_at IS NULL
		) active_assignment ON true
		LEFT JOIN LATERAL (
			SELECT ts.id, ts.submission_number, ts.summary, ts.links,
			       ts.submitter_account_id, ts.submitted_at
			FROM tasks.task_submissions ts
			WHERE ts.task_id=t.id ORDER BY ts.submission_number DESC LIMIT 1
		) latest_submission ON true
		WHERE `+where+fmt.Sprintf(" AND t.id=$%d::uuid FOR UPDATE OF t", len(args)), args...))
	if errors.Is(err, sql.ErrNoRows) {
		return task{}, errNotFound
	}
	if err != nil {
		return task{}, fmt.Errorf("lock task: %w", err)
	}
	return item, nil
}

func visibilityWhere(principal Principal, mine bool) (string, []any) {
	permissions := principal.Permissions
	canReadAll := contains(permissions, "tasks.read_all")
	canReadDepartment := contains(permissions, "tasks.read_department")
	where := `(t.created_by=$1::uuid OR EXISTS (
		SELECT 1 FROM tasks.task_assignments own_assignment
		WHERE own_assignment.task_id=t.id AND own_assignment.assignee_account_id=$1::uuid
		  AND own_assignment.unassigned_at IS NULL
	))`
	if !mine {
		where = "(" + where + ` OR $2::boolean OR ($3::boolean AND EXISTS (
			SELECT 1 FROM organization.department_memberships own_department
			WHERE own_department.department_id=t.department_id AND own_department.account_id=$1::uuid
		)))`
		return where, []any{principal.ID, canReadAll, canReadDepartment}
	}
	return where, []any{principal.ID}
}

func recordStatusAndAudit(ctx context.Context, tx *sql.Tx, actor, taskID, from, to, action, reason string) error {
	var previous any
	if from != "" {
		previous = from
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tasks.task_status_history (task_id, actor_account_id, from_status, to_status, reason)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5)
	`, taskID, actor, previous, to, reason); err != nil {
		return fmt.Errorf("record task status history: %w", err)
	}
	return recordAudit(ctx, tx, actor, taskID, action)
}

func recordAudit(ctx context.Context, tx *sql.Tx, actor, taskID, action string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit.audit_events (
			id, event_key, actor_type, actor_id, action, resource_type, resource_id, details
		) VALUES (gen_random_uuid(), gen_random_uuid()::text, 'user', $1, $2, 'task', $3, '{}'::jsonb)
	`, actor, action, taskID); err != nil {
		return fmt.Errorf("record task audit event: %w", err)
	}
	return nil
}

func (h *Handler) fail(w http.ResponseWriter, ctx context.Context, operation string, err error) {
	switch {
	case errors.Is(err, errNotFound):
		writeError(w, http.StatusNotFound, "not_found", requestID(ctx))
	case errors.Is(err, errForbidden):
		writeError(w, http.StatusForbidden, "permission_denied", requestID(ctx))
	case errors.Is(err, errConflict):
		writeError(w, http.StatusConflict, "task_conflict", requestID(ctx))
	case errors.Is(err, errPrecondition):
		writeError(w, http.StatusPreconditionFailed, "version_conflict", requestID(ctx))
	default:
		h.logger.ErrorContext(ctx, operation, "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
	}
}

func parseFilter(values url.Values, mine bool) (listFilter, error) {
	filter := listFilter{Limit: defaultPageSize, Mine: mine}
	allowed := map[string]bool{"status": true, "assignee": true, "department": true,
		"due_from": true, "due_to": true, "limit": true, "offset": true}
	for key, entries := range values {
		if !allowed[key] || len(entries) != 1 {
			return listFilter{}, fmt.Errorf("invalid query parameter %q", key)
		}
		value := strings.TrimSpace(entries[0])
		switch key {
		case "status":
			if !validStatus(value) {
				return listFilter{}, errors.New("invalid status")
			}
			filter.Status = value
		case "assignee":
			if !validUUID(value) {
				return listFilter{}, errors.New("invalid assignee")
			}
			filter.AssigneeID = value
		case "department":
			if !validUUID(value) {
				return listFilter{}, errors.New("invalid department")
			}
			filter.DepartmentID = value
		case "due_from", "due_to":
			parsed, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return listFilter{}, errors.New("invalid due date")
			}
			if key == "due_from" {
				filter.DueFrom = &parsed
			} else {
				filter.DueTo = &parsed
			}
		case "limit", "offset":
			number, err := parseInteger(value)
			if err != nil {
				return listFilter{}, err
			}
			if key == "limit" {
				if number < 1 || number > maxPageSize {
					return listFilter{}, errors.New("limit out of range")
				}
				filter.Limit = number
			} else {
				if number > maxPageOffset {
					return listFilter{}, errors.New("offset out of range")
				}
				filter.Offset = number
			}
		}
	}
	if filter.DueFrom != nil && filter.DueTo != nil && filter.DueFrom.After(*filter.DueTo) {
		return listFilter{}, errors.New("due_from after due_to")
	}
	return filter, nil
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
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return false
	}
	var extra any
	return errors.Is(decoder.Decode(&extra), io.EOF)
}

func (h *Handler) requestLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			h.logger.ErrorContext(r.Context(), "generate task request ID", "error", err)
			http.Error(w, "request ID unavailable", http.StatusInternalServerError)
			return
		}
		requestID := hex.EncodeToString(id[:])
		ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
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

func hasPermission(principal Principal, permission string) bool {
	return contains(principal.Permissions, permission)
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func validUUID(value string) bool { return uuidPattern.MatchString(value) }

func validPriority(value string) bool {
	return value == "low" || value == "normal" || value == "high" || value == "urgent"
}

func validStatus(value string) bool {
	switch value {
	case "draft", "assigned", "in_progress", "submitted", "changes_requested", "approved", "rejected", "cancelled":
		return true
	default:
		return false
	}
}

func etag(version int) string { return fmt.Sprintf(`"%d"`, version) }

func parseETag(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' {
		return 0, false
	}
	version, err := parseInteger(value[1 : len(value)-1])
	return version, err == nil && version > 0
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
	w.WriteHeader(status)
	_, _ = w.Write(append(encoded, '\n'))
}
