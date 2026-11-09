package opportunities

import (
	"archive/zip"
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
	"mime/multipart"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	sessionCookieName = "mothercat_session"
	maxDocumentSize   = 10 << 20
	maxUploadBodySize = maxDocumentSize + 64<<10
	defaultPageSize   = 50
	maxPageSize       = 100
	maxPageOffset     = 1_000_000
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

var (
	ErrUnauthenticated = errors.New("unauthenticated")
	errNotFound        = errors.New("not found")
)

type Principal struct {
	ID          string
	Permissions []string
}

type Authenticator interface {
	Authenticate(context.Context, string) (Principal, error)
}

type MalwareScanner interface {
	Scan(context.Context, []byte) error
}

type Handler struct {
	db            *sql.DB
	authenticator Authenticator
	scanner       MalwareScanner
	logger        *slog.Logger
	submissions   *submissionLimiter
}

type opportunity struct {
	ID                 string    `json:"id"`
	Type               string    `json:"type"`
	Title              string    `json:"title"`
	Location           string    `json:"location"`
	WorkMode           string    `json:"work_mode"`
	Summary            string    `json:"summary"`
	Description        string    `json:"description"`
	Requirements       string    `json:"requirements"`
	ApplicationDeadline *string   `json:"application_deadline,omitempty"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type documentMetadata struct {
	FileName         string `json:"file_name"`
	ContentType      string `json:"content_type"`
	SizeBytes        int64  `json:"size_bytes"`
	DownloadAvailable bool   `json:"download_available"`
}

type siwesApplication struct {
	ID            string           `json:"id"`
	OpportunityID string           `json:"opportunity_id"`
	Type          string           `json:"type"`
	FullName      string           `json:"full_name"`
	Email         string           `json:"email"`
	Phone         string           `json:"phone"`
	Institution   string           `json:"institution"`
	Course        string           `json:"course"`
	MatricNumber  string           `json:"matric_number"`
	Message       string           `json:"message"`
	Status        string           `json:"status"`
	ReviewerNote  string           `json:"reviewer_note,omitempty"`
	Document      documentMetadata `json:"document"`
	SubmittedAt   time.Time        `json:"submitted_at"`
	ReviewedAt    *time.Time       `json:"reviewed_at,omitempty"`
}

type page struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

type opportunityRequest struct {
	Type                string  `json:"type"`
	Title               string  `json:"title"`
	Location            string  `json:"location"`
	WorkMode            string  `json:"work_mode"`
	Summary             string  `json:"summary"`
	Description         string  `json:"description"`
	Requirements        string  `json:"requirements"`
	ApplicationDeadline *string `json:"application_deadline"`
}

type opportunityPatch struct {
	Type                   *string         `json:"type"`
	Title                  *string         `json:"title"`
	Location               *string         `json:"location"`
	WorkMode               *string         `json:"work_mode"`
	Summary                *string         `json:"summary"`
	Description            *string         `json:"description"`
	Requirements           *string         `json:"requirements"`
	ApplicationDeadline    json.RawMessage `json:"application_deadline"`
	deadlineSet            bool
	parsedApplicationDate  *string
}

type reviewerPatch struct {
	Status       *string `json:"status"`
	ReviewerNote *string `json:"reviewer_note"`
}

func NewHandler(db *sql.DB, authenticator Authenticator, scanner MalwareScanner, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{
		db: db, authenticator: authenticator, scanner: scanner,
		logger: logger, submissions: newSubmissionLimiter(),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/opportunities", h.publicList)
	mux.HandleFunc("GET /api/v1/opportunities/manage", h.manageList)
	mux.HandleFunc("POST /api/v1/opportunities", h.createOpportunity)
	mux.HandleFunc("GET /api/v1/opportunities/{opportunityID}", h.getOpportunity)
	mux.HandleFunc("PATCH /api/v1/opportunities/{opportunityID}", h.patchOpportunity)
	mux.HandleFunc("POST /api/v1/opportunities/{opportunityID}/publish", h.publishOpportunity)
	mux.HandleFunc("POST /api/v1/opportunities/{opportunityID}/unpublish", h.unpublishOpportunity)
	mux.HandleFunc("POST /api/v1/opportunities/{opportunityID}/close", h.closeOpportunity)
	mux.HandleFunc("POST /api/v1/opportunities/{opportunityID}/siwes-applications", h.submitApplication)
	mux.HandleFunc("GET /api/v1/siwes-applications", h.listApplications)
	mux.HandleFunc("GET /api/v1/siwes-applications/{applicationID}", h.getApplication)
	mux.HandleFunc("PATCH /api/v1/siwes-applications/{applicationID}", h.patchApplication)
	mux.HandleFunc("GET /api/v1/siwes-applications/{applicationID}/document", h.downloadDocument)
	return h.requestIDMiddleware(h.csrfMiddleware(mux))
}

func (h *Handler) publicList(w http.ResponseWriter, r *http.Request) {
	query, err := parseListQuery(r.URL.Query(), false)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_query")
		return
	}
	if h.db == nil {
		h.writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	where := "status='published' AND (application_deadline IS NULL OR application_deadline >= current_date)"
	args := []any{}
	if query.Type != "" {
		args = append(args, query.Type)
		where += fmt.Sprintf(" AND type=$%d", len(args))
	}
	h.listOpportunities(w, r.Context(), where, args, query)
}

func (h *Handler) manageList(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authorize(w, r, "", false)
	if !ok {
		return
	}
	if !canManage(principal) {
		h.writeError(w, http.StatusForbidden, "permission_denied")
		return
	}
	query, err := parseListQuery(r.URL.Query(), true)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_query")
		return
	}
	if h.db == nil {
		h.writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	where := `($1::boolean OR ($2::boolean AND type='job') OR ($3::boolean AND type='siwes'))`
	args := []any{
		hasPermission(principal, "opportunities.publish"),
		hasPermission(principal, "applications.review"),
		hasPermission(principal, "it_applications.review"),
	}
	if query.Type != "" {
		args = append(args, query.Type)
		where += fmt.Sprintf(" AND type=$%d", len(args))
	}
	if query.Status != "" {
		args = append(args, query.Status)
		where += fmt.Sprintf(" AND status=$%d", len(args))
	}
	h.listOpportunities(w, ctx, where, args, query)
}

func (h *Handler) listOpportunities(w http.ResponseWriter, ctx context.Context, where string, args []any, query listQuery) {
	var total int
	if err := h.db.QueryRowContext(ctx, "SELECT count(*) FROM opportunities.opportunities WHERE "+where, args...).Scan(&total); err != nil {
		h.fail(w, ctx, "count opportunities", err)
		return
	}
	pageArgs := append(append([]any(nil), args...), query.Limit, query.Offset)
	rows, err := h.db.QueryContext(ctx, `
		SELECT id::text,type,title,location,work_mode,summary,description,requirements,
		       application_deadline::text,status,created_at,updated_at
		FROM opportunities.opportunities WHERE `+where+
		fmt.Sprintf(" ORDER BY created_at DESC,id LIMIT $%d OFFSET $%d", len(pageArgs)-1, len(pageArgs)),
		pageArgs...)
	if err != nil {
		h.fail(w, ctx, "list opportunities", err)
		return
	}
	defer rows.Close()
	items := make([]opportunity, 0)
	for rows.Next() {
		item, scanErr := scanOpportunity(rows)
		if scanErr != nil {
			h.fail(w, ctx, "scan opportunity", scanErr)
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		h.fail(w, ctx, "read opportunities", err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"opportunities": items, "page": page{Limit: query.Limit, Offset: query.Offset, Total: total},
	})
}

func (h *Handler) getOpportunity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("opportunityID")
	if !validUUID(id) {
		h.writeError(w, http.StatusBadRequest, "invalid_opportunity_id")
		return
	}
	if h.db == nil {
		h.writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	item, err := h.readOpportunity(r.Context(), id, false)
	if err == nil {
		h.writeJSON(w, http.StatusOK, map[string]any{"opportunity": item})
		return
	}
	if !errors.Is(err, errNotFound) {
		h.fail(w, r.Context(), "get opportunity", err)
		return
	}
	principal, _, ok := h.authorize(w, r, "", false)
	if !ok {
		return
	}
	if !canViewOpportunity(principal) {
		h.writeError(w, http.StatusNotFound, "not_found")
		return
	}
	item, err = h.readOpportunity(r.Context(), id, true)
	if errors.Is(err, errNotFound) {
		h.writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		h.fail(w, r.Context(), "get managed opportunity", err)
		return
	}
	if !canViewType(principal, item.Type) {
		h.writeError(w, http.StatusNotFound, "not_found")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"opportunity": item})
}

func (h *Handler) readOpportunity(ctx context.Context, id string, managed bool) (opportunity, error) {
	query := `
		SELECT id::text,type,title,location,work_mode,summary,description,requirements,
		       application_deadline::text,status,created_at,updated_at
		FROM opportunities.opportunities WHERE id=$1::uuid`
	if !managed {
		query += " AND status='published' AND (application_deadline IS NULL OR application_deadline >= current_date)"
	}
	item, err := scanOpportunity(h.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return opportunity{}, errNotFound
	}
	if err != nil {
		return opportunity{}, fmt.Errorf("query opportunity: %w", err)
	}
	return item, nil
}

func (h *Handler) createOpportunity(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authorize(w, r, "opportunities.publish", true)
	if !ok {
		return
	}
	var input opportunityRequest
	if !h.decodeJSON(w, r, &input) {
		h.writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	normalizeOpportunity(&input)
	if err := validateOpportunity(input, false); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_opportunity")
		return
	}
	if h.db == nil {
		h.writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		h.fail(w, ctx, "begin opportunity creation", err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	item, err := scanOpportunity(tx.QueryRowContext(ctx, `
		INSERT INTO opportunities.opportunities
			(type,title,location,work_mode,summary,description,requirements,application_deadline,status,created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::date,'draft',$9::uuid)
		RETURNING id::text,type,title,location,work_mode,summary,description,requirements,
		          application_deadline::text,status,created_at,updated_at
	`, input.Type, input.Title, input.Location, input.WorkMode, input.Summary,
		input.Description, input.Requirements, nullableDate(input.ApplicationDeadline), principal.ID))
	if err != nil {
		h.fail(w, ctx, "create opportunity", err)
		return
	}
	if err := recordAudit(ctx, tx, principal.ID, "opportunity.create", "opportunity", item.ID, nil); err != nil {
		h.fail(w, ctx, "audit opportunity creation", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, ctx, "commit opportunity creation", err)
		return
	}
	h.writeJSON(w, http.StatusCreated, map[string]any{"opportunity": item})
}

func (h *Handler) patchOpportunity(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authorize(w, r, "opportunities.publish", true)
	if !ok {
		return
	}
	id := r.PathValue("opportunityID")
	if !validUUID(id) {
		h.writeError(w, http.StatusBadRequest, "invalid_opportunity_id")
		return
	}
	var patch opportunityPatch
	if !h.decodeJSON(w, r, &patch) || !hasOpportunityPatch(patch) {
		h.writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if patch.Type != nil {
		value := strings.TrimSpace(*patch.Type)
		patch.Type = &value
	}
	if patch.Title != nil {
		value := strings.TrimSpace(*patch.Title)
		patch.Title = &value
	}
	if patch.Location != nil {
		value := strings.TrimSpace(*patch.Location)
		patch.Location = &value
	}
	if patch.WorkMode != nil {
		value := strings.TrimSpace(*patch.WorkMode)
		patch.WorkMode = &value
	}
	if patch.Summary != nil {
		value := strings.TrimSpace(*patch.Summary)
		patch.Summary = &value
	}
	if patch.Description != nil {
		value := strings.TrimSpace(*patch.Description)
		patch.Description = &value
	}
	if patch.Requirements != nil {
		value := strings.TrimSpace(*patch.Requirements)
		patch.Requirements = &value
	}
	if patch.ApplicationDeadline != nil {
		patch.deadlineSet = true
		if !bytes.Equal(bytes.TrimSpace(patch.ApplicationDeadline), []byte("null")) {
			var value string
			if err := json.Unmarshal(patch.ApplicationDeadline, &value); err != nil {
				h.writeError(w, http.StatusBadRequest, "invalid_opportunity")
				return
			}
			value = strings.TrimSpace(value)
			patch.parsedApplicationDate = &value
		}
	}
	if err := validatePatch(patch); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_opportunity")
		return
	}
	if h.db == nil {
		h.writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		h.fail(w, ctx, "begin opportunity update", err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM opportunities.opportunities WHERE id=$1::uuid FOR UPDATE`, id).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		h.writeError(w, http.StatusNotFound, "not_found")
		return
	} else if err != nil {
		h.fail(w, ctx, "lock opportunity for update", err)
		return
	}
	if status != "draft" {
		h.writeError(w, http.StatusConflict, "opportunity_not_editable")
		return
	}
	item, err := scanOpportunity(tx.QueryRowContext(ctx, `
		UPDATE opportunities.opportunities SET
			type=coalesce($2,type), title=coalesce($3,title), location=coalesce($4,location),
			work_mode=coalesce($5,work_mode), summary=coalesce($6,summary),
			description=coalesce($7,description), requirements=coalesce($8,requirements),
			application_deadline=CASE WHEN $9::boolean THEN $10::date ELSE application_deadline END,
			updated_at=now()
		WHERE id=$1::uuid
		RETURNING id::text,type,title,location,work_mode,summary,description,requirements,
		          application_deadline::text,status,created_at,updated_at
	`, id, patch.Type, patch.Title, patch.Location, patch.WorkMode, patch.Summary,
		patch.Description, patch.Requirements, patch.ApplicationDeadline != nil,
		nullableDate(patch.parsedApplicationDate)))
	if err != nil {
		h.fail(w, ctx, "update opportunity", err)
		return
	}
	if err := recordAudit(ctx, tx, principal.ID, "opportunity.update", "opportunity", id, nil); err != nil {
		h.fail(w, ctx, "audit opportunity update", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, ctx, "commit opportunity update", err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"opportunity": item})
}

func (h *Handler) publishOpportunity(w http.ResponseWriter, r *http.Request) {
	h.changeOpportunityStatus(w, r, "published")
}

func (h *Handler) unpublishOpportunity(w http.ResponseWriter, r *http.Request) {
	h.changeOpportunityStatus(w, r, "draft")
}

func (h *Handler) closeOpportunity(w http.ResponseWriter, r *http.Request) {
	h.changeOpportunityStatus(w, r, "closed")
}

func (h *Handler) changeOpportunityStatus(w http.ResponseWriter, r *http.Request, target string) {
	principal, ctx, ok := h.authorize(w, r, "opportunities.publish", true)
	if !ok {
		return
	}
	id := r.PathValue("opportunityID")
	if !validUUID(id) {
		h.writeError(w, http.StatusBadRequest, "invalid_opportunity_id")
		return
	}
	if h.db == nil {
		h.writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		h.fail(w, ctx, "begin opportunity status update", err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	current, err := scanOpportunity(tx.QueryRowContext(ctx, `
		SELECT id::text,type,title,location,work_mode,summary,description,requirements,
		       application_deadline::text,status,created_at,updated_at
		FROM opportunities.opportunities WHERE id=$1::uuid FOR UPDATE
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		h.writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		h.fail(w, ctx, "lock opportunity status", err)
		return
	}
	allowed := (target == "published" && current.Status == "draft") ||
		(target == "draft" && current.Status == "published") ||
		(target == "closed" && current.Status == "published")
	if !allowed {
		h.writeError(w, http.StatusConflict, "opportunity_status_conflict")
		return
	}
	if target == "published" {
		input := opportunityRequest{
			Type: current.Type, Title: current.Title, Location: current.Location,
			WorkMode: current.WorkMode, Summary: current.Summary,
			Description: current.Description, Requirements: current.Requirements,
			ApplicationDeadline: current.ApplicationDeadline,
		}
		if err := validateOpportunity(input, true); err != nil {
			h.writeError(w, http.StatusConflict, "opportunity_not_publishable")
			return
		}
		var deadlineOpen bool
		if err := tx.QueryRowContext(ctx, `
			SELECT coalesce(application_deadline >= current_date, false)
			FROM opportunities.opportunities WHERE id=$1::uuid
		`, id).Scan(&deadlineOpen); err != nil {
			h.fail(w, ctx, "validate opportunity deadline", err)
			return
		}
		if !deadlineOpen {
			h.writeError(w, http.StatusConflict, "opportunity_not_publishable")
			return
		}
	}
	item, err := scanOpportunity(tx.QueryRowContext(ctx, `
		UPDATE opportunities.opportunities SET status=$2,updated_at=now()
		WHERE id=$1::uuid
		RETURNING id::text,type,title,location,work_mode,summary,description,requirements,
		          application_deadline::text,status,created_at,updated_at
	`, id, target))
	if err != nil {
		h.fail(w, ctx, "change opportunity status", err)
		return
	}
	if err := recordAudit(ctx, tx, principal.ID, "opportunity."+target, "opportunity", id, nil); err != nil {
		h.fail(w, ctx, "audit opportunity status", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, ctx, "commit opportunity status", err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"opportunity": item})
}

func (h *Handler) submitApplication(w http.ResponseWriter, r *http.Request) {
	opportunityID := r.PathValue("opportunityID")
	if !validUUID(opportunityID) {
		h.writeError(w, http.StatusBadRequest, "invalid_opportunity_id")
		return
	}
	if !h.submissions.allow(clientIP(r)) {
		h.writeError(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	if h.db == nil {
		h.writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	if h.scanner == nil {
		h.writeError(w, http.StatusServiceUnavailable, "document_scanning_unavailable")
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		h.writeError(w, http.StatusUnsupportedMediaType, "content_type_must_be_multipart_form_data")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBodySize)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			h.writeError(w, http.StatusRequestEntityTooLarge, "document_too_large")
			return
		}
		h.writeError(w, http.StatusBadRequest, "invalid_multipart_request")
		return
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}
	input, err := parseApplicationForm(r.MultipartForm)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_application")
		return
	}
	fileHeader := r.MultipartForm.File["document"][0]
	if fileHeader.Size < 1 || fileHeader.Size > maxDocumentSize {
		h.writeError(w, http.StatusRequestEntityTooLarge, "document_too_large")
		return
	}
	if !utf8.ValidString(fileHeader.Filename) || strings.ContainsAny(fileHeader.Filename, `/\`) ||
		strings.TrimSpace(fileHeader.Filename) == "" ||
		utf8.RuneCountInString(fileHeader.Filename) > 200 || strings.IndexFunc(fileHeader.Filename, unicode.IsControl) >= 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_document_name")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_document")
		return
	}
	document, readErr := io.ReadAll(io.LimitReader(file, maxDocumentSize+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || int64(len(document)) != fileHeader.Size || len(document) > maxDocumentSize {
		h.writeError(w, http.StatusBadRequest, "invalid_document")
		return
	}
	documentType, valid := validateDocument(fileHeader.Filename, document)
	if !valid {
		h.writeError(w, http.StatusBadRequest, "unsupported_document")
		return
	}
	if err := h.scanner.Scan(r.Context(), document); err != nil {
		var scanErr *MalwareDetectedError
		if errors.As(err, &scanErr) {
			h.writeError(w, http.StatusUnprocessableEntity, "unsafe_document")
			return
		}
		h.logger.ErrorContext(r.Context(), "scan SIWES application document", "request_id", requestID(r.Context()), "error", err)
		h.writeError(w, http.StatusServiceUnavailable, "document_scanning_unavailable")
		return
	}
	if err := h.checkPublicOrigin(r); err != nil {
		h.writeError(w, http.StatusForbidden, "origin_not_allowed")
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		h.fail(w, r.Context(), "begin SIWES application", err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var allowed bool
	if err := tx.QueryRowContext(r.Context(), `
		SELECT type='siwes' AND status='published'
		       AND (application_deadline IS NULL OR application_deadline >= current_date)
		FROM opportunities.opportunities WHERE id=$1::uuid FOR SHARE
	`, opportunityID).Scan(&allowed); errors.Is(err, sql.ErrNoRows) {
		h.writeError(w, http.StatusNotFound, "opportunity_not_found")
		return
	} else if err != nil {
		h.fail(w, r.Context(), "check SIWES application opportunity", err)
		return
	}
	if !allowed {
		h.writeError(w, http.StatusConflict, "applications_closed")
		return
	}
	item, err := scanApplication(tx.QueryRowContext(r.Context(), `
		INSERT INTO opportunities.siwes_applications
			(opportunity_id,full_name,email,phone,institution,course,matric_number,message,
			 document_name,document_type,document_size_bytes,document_data)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id::text,opportunity_id::text,full_name,email,phone,institution,course,
		          matric_number,message,status,reviewer_note,document_name,document_type,
		          document_size_bytes,submitted_at,reviewed_at
	`, opportunityID, input.FullName, input.Email, input.Phone, input.Institution,
		input.Course, input.MatricNumber, input.Message, fileHeader.Filename, documentType,
		int64(len(document)), document))
	if err != nil {
		h.fail(w, r.Context(), "save SIWES application", err)
		return
	}
	if err := recordPublicAudit(r.Context(), tx, opportunityID, item.ID); err != nil {
		h.fail(w, r.Context(), "audit SIWES application", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, r.Context(), "commit SIWES application", err)
		return
	}
	h.writeJSON(w, http.StatusCreated, map[string]any{"application": item})
}

func (h *Handler) listApplications(w http.ResponseWriter, r *http.Request) {
	_, ctx, ok := h.authorize(w, r, "it_applications.review", false)
	if !ok {
		return
	}
	if h.db == nil {
		h.writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	query, err := parseApplicationQuery(r.URL.Query())
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_query")
		return
	}
	where := "true"
	args := []any{}
	if query.OpportunityID != "" {
		args = append(args, query.OpportunityID)
		where += fmt.Sprintf(" AND opportunity_id=$%d::uuid", len(args))
	}
	if query.Status != "" {
		args = append(args, query.Status)
		where += fmt.Sprintf(" AND status=$%d", len(args))
	}
	var total int
	if err := h.db.QueryRowContext(ctx, "SELECT count(*) FROM opportunities.siwes_applications WHERE "+where, args...).Scan(&total); err != nil {
		h.fail(w, ctx, "count SIWES applications", err)
		return
	}
	pageArgs := append(append([]any(nil), args...), query.Limit, query.Offset)
	rows, err := h.db.QueryContext(ctx, `
		SELECT id::text,opportunity_id::text,full_name,email,phone,institution,course,
		       matric_number,message,status,reviewer_note,document_name,document_type,
		       document_size_bytes,submitted_at,reviewed_at
		FROM opportunities.siwes_applications WHERE `+where+
		fmt.Sprintf(" ORDER BY submitted_at DESC,id LIMIT $%d OFFSET $%d", len(pageArgs)-1, len(pageArgs)),
		pageArgs...)
	if err != nil {
		h.fail(w, ctx, "list SIWES applications", err)
		return
	}
	defer rows.Close()
	items := make([]siwesApplication, 0)
	for rows.Next() {
		item, scanErr := scanApplication(rows)
		if scanErr != nil {
			h.fail(w, ctx, "scan SIWES application", scanErr)
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		h.fail(w, ctx, "read SIWES applications", err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"applications": items, "page": page{Limit: query.Limit, Offset: query.Offset, Total: total},
	})
}

func (h *Handler) getApplication(w http.ResponseWriter, r *http.Request) {
	_, ctx, ok := h.authorize(w, r, "it_applications.review", false)
	if !ok {
		return
	}
	id := r.PathValue("applicationID")
	if !validUUID(id) {
		h.writeError(w, http.StatusBadRequest, "invalid_application_id")
		return
	}
	if h.db == nil {
		h.writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	item, err := h.readApplication(ctx, id)
	if errors.Is(err, errNotFound) {
		h.writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		h.fail(w, ctx, "get SIWES application", err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"application": item})
}

func (h *Handler) patchApplication(w http.ResponseWriter, r *http.Request) {
	principal, ctx, ok := h.authorize(w, r, "it_applications.review", true)
	if !ok {
		return
	}
	id := r.PathValue("applicationID")
	if !validUUID(id) {
		h.writeError(w, http.StatusBadRequest, "invalid_application_id")
		return
	}
	var patch reviewerPatch
	if !h.decodeJSON(w, r, &patch) || (patch.Status == nil && patch.ReviewerNote == nil) {
		h.writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if patch.Status != nil {
		value := strings.TrimSpace(*patch.Status)
		patch.Status = &value
		if !validApplicationStatus(value) {
			h.writeError(w, http.StatusBadRequest, "invalid_application_status")
			return
		}
	}
	if patch.ReviewerNote != nil {
		value := strings.TrimSpace(*patch.ReviewerNote)
		patch.ReviewerNote = &value
		if utf8.RuneCountInString(value) > 2000 {
			h.writeError(w, http.StatusBadRequest, "invalid_reviewer_note")
			return
		}
	}
	if h.db == nil {
		h.writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		h.fail(w, ctx, "begin SIWES review", err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var current string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM opportunities.siwes_applications WHERE id=$1::uuid FOR UPDATE`, id).Scan(&current); errors.Is(err, sql.ErrNoRows) {
		h.writeError(w, http.StatusNotFound, "not_found")
		return
	} else if err != nil {
		h.fail(w, ctx, "lock SIWES application", err)
		return
	}
	if patch.Status != nil && !validApplicationTransition(current, *patch.Status) {
		h.writeError(w, http.StatusConflict, "application_status_conflict")
		return
	}
	item, err := scanApplication(tx.QueryRowContext(ctx, `
		UPDATE opportunities.siwes_applications SET
			status=coalesce($2,status),
			reviewer_note=coalesce($3,reviewer_note),
			reviewed_at=CASE WHEN $2::text IS NULL THEN reviewed_at ELSE now() END,
			reviewed_by=CASE WHEN $2::text IS NULL THEN reviewed_by ELSE $4::uuid END
		WHERE id=$1::uuid
		RETURNING id::text,opportunity_id::text,full_name,email,phone,institution,course,
		          matric_number,message,status,reviewer_note,document_name,document_type,
		          document_size_bytes,submitted_at,reviewed_at
	`, id, patch.Status, patch.ReviewerNote, principal.ID))
	if err != nil {
		h.fail(w, ctx, "update SIWES application", err)
		return
	}
	if err := recordAudit(ctx, tx, principal.ID, "siwes_application.review", "siwes_application", id,
		map[string]any{"status": patch.Status, "reviewer_note_updated": patch.ReviewerNote != nil}); err != nil {
		h.fail(w, ctx, "audit SIWES review", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, ctx, "commit SIWES review", err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"application": item})
}

func (h *Handler) downloadDocument(w http.ResponseWriter, r *http.Request) {
	_, ctx, ok := h.authorize(w, r, "it_applications.review", false)
	if !ok {
		return
	}
	id := r.PathValue("applicationID")
	if !validUUID(id) {
		h.writeError(w, http.StatusBadRequest, "invalid_application_id")
		return
	}
	if h.db == nil {
		h.writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	var name, contentType string
	var data []byte
	err := h.db.QueryRowContext(ctx, `
		SELECT document_name,document_type,document_data
		FROM opportunities.siwes_applications WHERE id=$1::uuid
	`, id).Scan(&name, &contentType, &data)
	if errors.Is(err, sql.ErrNoRows) {
		h.writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		h.fail(w, ctx, "read SIWES application document", err)
		return
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

type listQuery struct {
	Type   string
	Status string
	Limit  int
	Offset int
}

func parseListQuery(values url.Values, managed bool) (listQuery, error) {
	query := listQuery{Limit: defaultPageSize}
	for key, entries := range values {
		if len(entries) != 1 {
			return listQuery{}, errors.New("duplicate query parameter")
		}
		value := strings.TrimSpace(entries[0])
		switch key {
		case "type":
			if value != "job" && value != "siwes" {
				return listQuery{}, errors.New("invalid opportunity type")
			}
			query.Type = value
		case "status":
			if !managed && value == "published" {
				continue
			}
			if !managed || !validOpportunityStatus(value) {
				return listQuery{}, errors.New("invalid opportunity status")
			}
			query.Status = value
		case "limit", "offset":
			number, err := parseInteger(value)
			if err != nil {
				return listQuery{}, err
			}
			if key == "limit" {
				if number < 1 || number > maxPageSize {
					return listQuery{}, errors.New("limit out of range")
				}
				query.Limit = number
			} else {
				if number > maxPageOffset {
					return listQuery{}, errors.New("offset out of range")
				}
				query.Offset = number
			}
		default:
			return listQuery{}, errors.New("unknown query parameter")
		}
	}
	return query, nil
}

type applicationQuery struct {
	OpportunityID string
	Status        string
	Limit         int
	Offset        int
}

func parseApplicationQuery(values url.Values) (applicationQuery, error) {
	query := applicationQuery{Limit: defaultPageSize}
	for key, entries := range values {
		if len(entries) != 1 {
			return applicationQuery{}, errors.New("duplicate query parameter")
		}
		value := strings.TrimSpace(entries[0])
		switch key {
		case "opportunity_id":
			if !validUUID(value) {
				return applicationQuery{}, errors.New("invalid opportunity ID")
			}
			query.OpportunityID = value
		case "status":
			if !validApplicationStatus(value) {
				return applicationQuery{}, errors.New("invalid application status")
			}
			query.Status = value
		case "limit", "offset":
			number, err := parseInteger(value)
			if err != nil {
				return applicationQuery{}, err
			}
			if key == "limit" {
				if number < 1 || number > maxPageSize {
					return applicationQuery{}, errors.New("limit out of range")
				}
				query.Limit = number
			} else {
				if number > maxPageOffset {
					return applicationQuery{}, errors.New("offset out of range")
				}
				query.Offset = number
			}
		default:
			return applicationQuery{}, errors.New("unknown query parameter")
		}
	}
	return query, nil
}

type applicationForm struct {
	FullName     string
	Email        string
	Phone        string
	Institution  string
	Course       string
	MatricNumber string
	Message      string
}

func parseApplicationForm(form *multipart.Form) (applicationForm, error) {
	if form == nil || len(form.File) != 1 || len(form.File["document"]) != 1 ||
		len(form.Value) < 6 || len(form.Value) > 7 {
		return applicationForm{}, errors.New("invalid multipart fields")
	}
	allowed := map[string]bool{
		"full_name": true, "email": true, "phone": true, "institution": true,
		"course": true, "matric_number": true, "message": true,
	}
	for key, values := range form.Value {
		if !allowed[key] || len(values) != 1 {
			return applicationForm{}, errors.New("invalid text field")
		}
	}
	get := func(key string) string {
		values := form.Value[key]
		if len(values) == 0 {
			return ""
		}
		return strings.TrimSpace(values[0])
	}
	input := applicationForm{
		FullName: get("full_name"), Email: strings.ToLower(get("email")),
		Phone: get("phone"), Institution: get("institution"),
		Course: get("course"), MatricNumber: get("matric_number"), Message: get("message"),
	}
	email, err := mail.ParseAddress(input.Email)
	if err != nil || email.Address != input.Email ||
		!within(input.FullName, 1, 160) || !within(input.Email, 3, 254) ||
		!within(input.Phone, 1, 40) || !within(input.Institution, 1, 180) ||
		!within(input.Course, 1, 160) || !within(input.MatricNumber, 1, 80) ||
		utf8.RuneCountInString(input.Message) > 2000 {
		return applicationForm{}, errors.New("invalid application fields")
	}
	return input, nil
}

func validateDocument(filename string, data []byte) (string, bool) {
	extension := strings.ToLower(path.Ext(filename))
	switch extension {
	case ".pdf":
		return "application/pdf", len(data) >= 5 && bytes.HasPrefix(data, []byte("%PDF-"))
	case ".doc":
		return "application/msword", len(data) >= 8 &&
			bytes.Equal(data[:8], []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1})
	case ".docx":
		if len(data) < 4 || !bytes.Equal(data[:4], []byte{'P', 'K', 3, 4}) {
			return "", false
		}
		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil || len(reader.File) > 1000 {
			return "", false
		}
		var contentTypes, document bool
		var total uint64
		seen := make(map[string]bool, len(reader.File))
		for _, file := range reader.File {
			if file.UncompressedSize64 > 50<<20 || total > 50<<20-file.UncompressedSize64 || seen[file.Name] {
				return "", false
			}
			total += file.UncompressedSize64
			seen[file.Name] = true
			if file.Name == "[Content_Types].xml" {
				contentTypes = true
			}
			if file.Name == "word/document.xml" {
				document = true
			}
		}
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document", contentTypes && document
	default:
		return "", false
	}
}

func (h *Handler) readApplication(ctx context.Context, id string) (siwesApplication, error) {
	item, err := scanApplication(h.db.QueryRowContext(ctx, `
		SELECT id::text,opportunity_id::text,full_name,email,phone,institution,course,
		       matric_number,message,status,reviewer_note,document_name,document_type,
		       document_size_bytes,submitted_at,reviewed_at
		FROM opportunities.siwes_applications WHERE id=$1::uuid
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return siwesApplication{}, errNotFound
	}
	if err != nil {
		return siwesApplication{}, fmt.Errorf("query SIWES application: %w", err)
	}
	return item, nil
}

type scanner interface {
	Scan(...any) error
}

func scanOpportunity(row scanner) (opportunity, error) {
	var item opportunity
	var deadline sql.NullString
	err := row.Scan(&item.ID, &item.Type, &item.Title, &item.Location, &item.WorkMode,
		&item.Summary, &item.Description, &item.Requirements, &deadline, &item.Status,
		&item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return opportunity{}, err
	}
	if deadline.Valid {
		item.ApplicationDeadline = &deadline.String
	}
	return item, nil
}

func scanApplication(row scanner) (siwesApplication, error) {
	var item siwesApplication
	var reviewed sql.NullTime
	var size int64
	err := row.Scan(&item.ID, &item.OpportunityID, &item.FullName, &item.Email, &item.Phone,
		&item.Institution, &item.Course, &item.MatricNumber, &item.Message, &item.Status,
		&item.ReviewerNote, &item.Document.FileName, &item.Document.ContentType,
		&size, &item.SubmittedAt, &reviewed)
	if err != nil {
		return siwesApplication{}, err
	}
	item.Type = "siwes"
	item.Document.SizeBytes = size
	item.Document.DownloadAvailable = true
	if reviewed.Valid {
		item.ReviewedAt = &reviewed.Time
	}
	return item, nil
}

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request, permission string, write bool) (Principal, context.Context, bool) {
	ctx := r.Context()
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return Principal{}, nil, false
	}
	if h.authenticator == nil {
		h.writeError(w, http.StatusServiceUnavailable, "identity_unavailable")
		return Principal{}, nil, false
	}
	principal, err := h.authenticator.Authenticate(ctx, cookie.Value)
	if err != nil {
		if errors.Is(err, ErrUnauthenticated) {
			h.writeError(w, http.StatusUnauthorized, "unauthorized")
		} else {
			h.logger.ErrorContext(ctx, "authenticate opportunity request", "request_id", requestID(ctx), "error", err)
			h.writeError(w, http.StatusServiceUnavailable, "identity_unavailable")
		}
		return Principal{}, nil, false
	}
	if !validUUID(principal.ID) {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return Principal{}, nil, false
	}
	if permission != "" && !hasPermission(principal, permission) {
		h.writeError(w, http.StatusForbidden, "permission_denied")
		return Principal{}, nil, false
	}
	if write {
		if err := h.checkPublicOrigin(r); err != nil {
			h.writeError(w, http.StatusForbidden, "origin_not_allowed")
			return Principal{}, nil, false
		}
	}
	return principal, ctx, true
}

func (h *Handler) checkPublicOrigin(r *http.Request) error {
	origin, err := url.Parse(r.Header.Get("Origin"))
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		scheme = "https"
	}
	if err != nil || origin == nil || !strings.EqualFold(origin.Host, r.Host) || origin.Scheme != scheme ||
		origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return errors.New("request origin does not match host")
	}
	return nil
}

func (h *Handler) decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return false
	}
	var extra any
	return errors.Is(decoder.Decode(&extra), io.EOF)
}

func (h *Handler) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw [16]byte
		if _, err := rand.Read(raw[:]); err != nil {
			h.logger.ErrorContext(r.Context(), "generate opportunity request ID", "error", err)
			http.Error(w, "request ID unavailable", http.StatusInternalServerError)
			return
		}
		ctx := context.WithValue(r.Context(), requestIDKey{}, hex.EncodeToString(raw[:]))
		w.Header().Set("X-Request-ID", requestID(ctx))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if err := h.checkPublicOrigin(r); err != nil {
			h.writeError(w, http.StatusForbidden, "origin_not_allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) fail(w http.ResponseWriter, ctx context.Context, operation string, err error) {
	h.logger.ErrorContext(ctx, operation, "request_id", requestID(ctx), "error", err)
	h.writeError(w, http.StatusInternalServerError, "internal_error")
}

func (h *Handler) writeError(w http.ResponseWriter, status int, code string) {
	h.writeJSON(w, status, map[string]any{"error": map[string]string{
		"code": code, "request_id": w.Header().Get("X-Request-ID"),
	}})
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, payload any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "response encoding failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(encoded, '\n'))
}

type requestIDKey struct{}

func requestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func recordAudit(ctx context.Context, tx *sql.Tx, actor, action, resourceType, resourceID string, details map[string]any) error {
	if details == nil {
		details = map[string]any{}
	}
	encoded, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode audit details: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit.audit_events (id,event_key,actor_type,actor_id,action,resource_type,resource_id,details)
		VALUES (gen_random_uuid(),gen_random_uuid()::text,'user',$1,$2,$3,$4,$5::jsonb)
	`, actor, action, resourceType, resourceID, string(encoded)); err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

func recordPublicAudit(ctx context.Context, tx *sql.Tx, opportunityID, applicationID string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit.audit_events (id,event_key,actor_type,actor_id,action,resource_type,resource_id,details)
		VALUES (gen_random_uuid(),gen_random_uuid()::text,'public','anonymous',
		        'siwes_application.submit','siwes_application',$1,
		        jsonb_build_object('opportunity_id',$2::text))
	`, applicationID, opportunityID); err != nil {
		return fmt.Errorf("insert public SIWES audit event: %w", err)
	}
	return nil
}

func normalizeOpportunity(input *opportunityRequest) {
	input.Type = strings.TrimSpace(input.Type)
	input.Title = strings.TrimSpace(input.Title)
	input.Location = strings.TrimSpace(input.Location)
	input.WorkMode = strings.TrimSpace(input.WorkMode)
	input.Summary = strings.TrimSpace(input.Summary)
	input.Description = strings.TrimSpace(input.Description)
	input.Requirements = strings.TrimSpace(input.Requirements)
	if input.ApplicationDeadline != nil {
		value := strings.TrimSpace(*input.ApplicationDeadline)
		input.ApplicationDeadline = &value
	}
}

func validateOpportunity(input opportunityRequest, publishing bool) error {
	if input.Type != "job" && input.Type != "siwes" {
		return errors.New("invalid type")
	}
	if !within(input.Title, 1, 160) || !within(input.Location, 1, 120) ||
		!within(input.WorkMode, 1, 80) || !within(input.Summary, 1, 400) ||
		!within(input.Description, 1, 8000) || utf8.RuneCountInString(input.Requirements) > 4000 {
		return errors.New("invalid opportunity fields")
	}
	if input.ApplicationDeadline != nil {
		if _, err := time.Parse("2006-01-02", *input.ApplicationDeadline); err != nil {
			return errors.New("invalid deadline")
		}
	}
	if publishing {
		if input.ApplicationDeadline == nil {
			return errors.New("deadline is required to publish")
		}
	}
	return nil
}

func validatePatch(patch opportunityPatch) error {
	// Only validate fields that are present; the database validates the merged row.
	if patch.Type != nil && *patch.Type != "job" && *patch.Type != "siwes" {
		return errors.New("invalid type")
	}
	for _, field := range []struct {
		value *string
		min   int
		max   int
	}{
		{patch.Title, 1, 160}, {patch.Location, 1, 120}, {patch.WorkMode, 1, 80},
		{patch.Summary, 1, 400}, {patch.Description, 1, 8000}, {patch.Requirements, 0, 4000},
	} {
		if field.value != nil && !within(*field.value, field.min, field.max) {
			return errors.New("invalid opportunity field")
		}
	}
	if patch.deadlineSet && patch.parsedApplicationDate != nil {
		if *patch.parsedApplicationDate != "" {
			if _, err := time.Parse("2006-01-02", *patch.parsedApplicationDate); err != nil {
				return errors.New("invalid deadline")
			}
		}
	}
	return nil
}

func hasOpportunityPatch(patch opportunityPatch) bool {
	return patch.Type != nil || patch.Title != nil || patch.Location != nil ||
		patch.WorkMode != nil || patch.Summary != nil || patch.Description != nil ||
		patch.Requirements != nil || patch.ApplicationDeadline != nil
}

func nullableDate(value *string) any {
	if value == nil || *value == "" {
		return nil
	}
	return *value
}

func within(value string, min, max int) bool {
	length := utf8.RuneCountInString(value)
	return utf8.ValidString(value) && length >= min && length <= max
}

func validOpportunityStatus(value string) bool {
	return value == "draft" || value == "published" || value == "closed"
}

func validApplicationStatus(value string) bool {
	switch value {
	case "submitted", "under_review", "shortlisted", "accepted", "rejected":
		return true
	default:
		return false
	}
}

func validApplicationTransition(from, to string) bool {
	switch from {
	case "submitted":
		return to == "under_review" || to == "rejected"
	case "under_review":
		return to == "shortlisted" || to == "accepted" || to == "rejected"
	case "shortlisted":
		return to == "accepted" || to == "rejected"
	default:
		return false
	}
}

func canManage(principal Principal) bool {
	return canViewOpportunity(principal)
}

func canViewOpportunity(principal Principal) bool {
	return hasPermission(principal, "opportunities.publish") ||
		hasPermission(principal, "applications.review") ||
		hasPermission(principal, "it_applications.review")
}

func canViewType(principal Principal, typ string) bool {
	return hasPermission(principal, "opportunities.publish") ||
		(typ == "job" && hasPermission(principal, "applications.review")) ||
		(typ == "siwes" && hasPermission(principal, "it_applications.review"))
}

func hasPermission(principal Principal, permission string) bool {
	for _, candidate := range principal.Permissions {
		if candidate == permission {
			return true
		}
	}
	return false
}

func validUUID(value string) bool { return uuidPattern.MatchString(value) }

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

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && net.ParseIP(host) != nil {
		return host
	}
	if net.ParseIP(strings.TrimSpace(r.RemoteAddr)) != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}

type submissionLimiter struct {
	mu       sync.Mutex
	entries  map[string]submissionBucket
	max      int
	window   time.Duration
	maxItems int
	ops      uint64
}

type submissionBucket struct {
	count   int
	resetAt time.Time
}

func newSubmissionLimiter() *submissionLimiter {
	return &submissionLimiter{
		entries: make(map[string]submissionBucket), max: 5,
		window: time.Hour, maxItems: 10_000,
	}
}

func (l *submissionLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.ops++
	if l.ops%128 == 0 {
		for key, bucket := range l.entries {
			if !now.Before(bucket.resetAt) {
				delete(l.entries, key)
			}
		}
	}
	bucket, exists := l.entries[ip]
	if exists && !now.Before(bucket.resetAt) {
		delete(l.entries, ip)
		exists = false
	}
	if exists && bucket.count >= l.max && now.Before(bucket.resetAt) {
		return false
	}
	if !exists {
		if len(l.entries) >= l.maxItems {
			return false
		}
		bucket = submissionBucket{resetAt: now.Add(l.window)}
	}
	bucket.count++
	l.entries[ip] = bucket
	return true
}
