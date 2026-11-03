package http

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/application"
	"github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/domain"
)

const (
	sessionCookieName   = "mothercat_session"
	maxRequestBody      = 16 << 10
	maxImportFileSize   = 5 << 20
	maxImportBody       = maxImportFileSize + 64<<10
	maxProfileImageSize = 5 << 20
	maxProfileImageBody = maxProfileImageSize + 64<<10
	maxImportRows       = 5000
	maxImportIssues     = 100
	defaultPageSize     = 50
	maxPageSize         = 100
)

type store interface {
	Ping(context.Context) error
	Create(context.Context, string, domain.CreateStaffRequest) (domain.Staff, error)
	FindByID(context.Context, string) (domain.Staff, error)
	UpdateProfileImage(context.Context, string, string) (domain.Staff, error)
	CompleteProvisioning(context.Context, string, string) (domain.Staff, error)
	ListStaff(context.Context, domain.StaffListFilter) ([]domain.Staff, int, error)
	UpdateStaff(context.Context, string, domain.UpdateStaffRequest) (domain.Staff, error)
	DeactivateStaff(context.Context, string) (domain.Staff, error)
	ListDepartments(context.Context) ([]domain.Department, error)
	CreateDepartment(context.Context, string, domain.DepartmentRequest) (domain.Department, error)
	UpdateDepartment(context.Context, string, domain.DepartmentRequest) (domain.Department, error)
	ImportStaffBatch(context.Context, string, string, []domain.ImportStaffRow) (domain.StaffImportBatch, error)
	RecordFailedStaffImport(context.Context, string, string, domain.StaffImportResult) (domain.StaffImportBatch, error)
	FindStaffImportBatch(context.Context, string) (domain.StaffImportBatch, error)
}

type requestIDContextKey struct{}

func NewHandler(database store, authenticator application.Authenticator, audit application.AuditRecorder, logger *slog.Logger, onboarding ...application.OnboardingServices) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if database == nil {
			writeError(w, http.StatusServiceUnavailable, "service_not_ready", requestID(r.Context()))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := database.Ping(ctx); err != nil {
			logger.WarnContext(ctx, "Organization readiness check failed", "request_id", requestID(ctx), "error", err)
			writeError(w, http.StatusServiceUnavailable, "service_not_ready", requestID(ctx))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /api/v1/staff", func(w http.ResponseWriter, r *http.Request) {
		listStaff(w, r, database, authenticator, logger)
	})
	mux.HandleFunc("POST /api/v1/staff", func(w http.ResponseWriter, r *http.Request) {
		createStaff(w, r, database, authenticator, audit, logger)
	})
	mux.HandleFunc("GET /api/v1/staff/{staffId}", func(w http.ResponseWriter, r *http.Request) {
		getStaff(w, r, database, authenticator, logger)
	})
	mux.HandleFunc("POST /api/v1/staff/{staffId}/profile-picture", func(w http.ResponseWriter, r *http.Request) {
		var uploader application.ProfileImageUploader
		if len(onboarding) > 0 {
			uploader = onboarding[0].ProfileImageUploader
		}
		uploadProfilePicture(w, r, database, authenticator, audit, uploader, logger)
	})
	mux.HandleFunc("PATCH /api/v1/staff/{staffId}", func(w http.ResponseWriter, r *http.Request) {
		updateStaff(w, r, database, authenticator, audit, logger)
	})
	mux.HandleFunc("POST /api/v1/staff/{staffId}/deactivate", func(w http.ResponseWriter, r *http.Request) {
		var services *application.OnboardingServices
		if len(onboarding) > 0 {
			services = &onboarding[0]
		}
		deactivateStaff(w, r, database, authenticator, audit, services, logger)
	})
	mux.HandleFunc("POST /api/v1/staff/imports", func(w http.ResponseWriter, r *http.Request) {
		importStaff(w, r, database, authenticator, audit, logger)
	})
	mux.HandleFunc("GET /api/v1/staff/imports/{importId}", func(w http.ResponseWriter, r *http.Request) {
		getStaffImport(w, r, database, authenticator, logger)
	})
	mux.HandleFunc("POST /api/v1/staff/{staffID}/provision", func(w http.ResponseWriter, r *http.Request) {
		var services *application.OnboardingServices
		if len(onboarding) > 0 {
			services = &onboarding[0]
		}
		provisionStaff(w, r, database, authenticator, audit, services, logger)
	})
	mux.HandleFunc("GET /api/v1/departments", func(w http.ResponseWriter, r *http.Request) {
		listDepartments(w, r, database, authenticator, logger)
	})
	mux.HandleFunc("POST /api/v1/departments", func(w http.ResponseWriter, r *http.Request) {
		createDepartment(w, r, database, authenticator, audit, logger)
	})
	mux.HandleFunc("PATCH /api/v1/departments/{departmentId}", func(w http.ResponseWriter, r *http.Request) {
		updateDepartment(w, r, database, authenticator, audit, logger)
	})
	return requestLoggingMiddleware(logger, mux)
}

func authorizeRequest(
	w http.ResponseWriter,
	r *http.Request,
	authenticator application.Authenticator,
	logger *slog.Logger,
	permission string,
	requireOrigin bool,
) (context.Context, context.CancelFunc, application.Principal, bool) {
	if requireOrigin && !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "invalid_origin", requestID(r.Context()))
		return nil, nil, application.Principal{}, false
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", requestID(r.Context()))
		return nil, nil, application.Principal{}, false
	}
	if authenticator == nil {
		writeError(w, http.StatusServiceUnavailable, "identity_unavailable", requestID(r.Context()))
		return nil, nil, application.Principal{}, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	ctx = application.ContextWithRequestID(ctx, requestID(ctx))
	principal, err := authenticator.Authenticate(ctx, cookie.Value)
	if err != nil {
		cancel()
		if errors.Is(err, application.ErrUnauthenticated) {
			writeError(w, http.StatusUnauthorized, "unauthorized", requestID(r.Context()))
			return nil, nil, application.Principal{}, false
		}
		logger.ErrorContext(ctx, "validate Organization request session",
			"request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusServiceUnavailable, "identity_unavailable", requestID(r.Context()))
		return nil, nil, application.Principal{}, false
	}
	if !hasPermission(principal.Permissions, permission) {
		cancel()
		writeError(w, http.StatusForbidden, "permission_denied", requestID(r.Context()))
		return nil, nil, application.Principal{}, false
	}
	return ctx, cancel, principal, true
}

func listStaff(w http.ResponseWriter, r *http.Request, database store, authenticator application.Authenticator, logger *slog.Logger) {
	ctx, cancel, _, ok := authorizeRequest(w, r, authenticator, logger, "staff.read", false)
	if !ok {
		return
	}
	defer cancel()
	if database == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	filter, err := parseStaffListFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_query", requestID(ctx))
		return
	}
	staff, total, err := database.ListStaff(ctx, filter)
	if err != nil {
		logger.ErrorContext(ctx, "list staff records", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"staff": staff,
		"pagination": map[string]int{
			"limit": filter.Limit, "offset": filter.Offset, "total": total,
		},
	})
}

func parseStaffListFilter(r *http.Request) (domain.StaffListFilter, error) {
	query := r.URL.Query()
	for key, values := range query {
		switch key {
		case "limit", "offset", "search", "department_id", "employment_status":
		default:
			return domain.StaffListFilter{}, fmt.Errorf("unsupported query parameter %q", key)
		}
		if len(values) != 1 {
			return domain.StaffListFilter{}, fmt.Errorf("query parameter %q must be supplied once", key)
		}
	}
	filter := domain.StaffListFilter{Limit: defaultPageSize}
	if value := query.Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil || limit < 1 || limit > maxPageSize {
			return domain.StaffListFilter{}, errors.New("limit is out of range")
		}
		filter.Limit = limit
	}
	if value := query.Get("offset"); value != "" {
		offset, err := strconv.Atoi(value)
		if err != nil || offset < 0 || offset > 1_000_000 {
			return domain.StaffListFilter{}, errors.New("offset is out of range")
		}
		filter.Offset = offset
	}
	filter.Search = strings.TrimSpace(query.Get("search"))
	if len(filter.Search) > 100 {
		return domain.StaffListFilter{}, errors.New("search is too long")
	}
	filter.DepartmentID = strings.TrimSpace(query.Get("department_id"))
	if filter.DepartmentID != "" && !domain.IsUUID(filter.DepartmentID) {
		return domain.StaffListFilter{}, errors.New("department_id is invalid")
	}
	filter.EmploymentStatus = strings.TrimSpace(query.Get("employment_status"))
	switch filter.EmploymentStatus {
	case "", "pending", "active", "inactive", "deactivated":
	default:
		return domain.StaffListFilter{}, errors.New("employment_status is invalid")
	}
	return filter, nil
}

func getStaff(w http.ResponseWriter, r *http.Request, database store, authenticator application.Authenticator, logger *slog.Logger) {
	ctx, cancel, _, ok := authorizeRequest(w, r, authenticator, logger, "staff.read", false)
	if !ok {
		return
	}
	defer cancel()
	if database == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	staffID := strings.TrimSpace(r.PathValue("staffId"))
	if !domain.IsUUID(staffID) {
		writeError(w, http.StatusBadRequest, "invalid_staff_id", requestID(ctx))
		return
	}
	staff, err := database.FindByID(ctx, staffID)
	if errors.Is(err, domain.ErrStaffNotFound) {
		writeError(w, http.StatusNotFound, "staff_not_found", requestID(ctx))
		return
	}
	if err != nil {
		logger.ErrorContext(ctx, "get staff record", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"staff": staff})
}

func uploadProfilePicture(
	w http.ResponseWriter,
	r *http.Request,
	database store,
	authenticator application.Authenticator,
	audit application.AuditRecorder,
	uploader application.ProfileImageUploader,
	logger *slog.Logger,
) {
	ctx, cancel, principal, ok := authorizeRequest(w, r, authenticator, logger, "staff.read", true)
	if !ok {
		return
	}
	defer cancel()
	staffID := strings.TrimSpace(r.PathValue("staffId"))
	if !domain.IsUUID(staffID) {
		writeError(w, http.StatusBadRequest, "invalid_staff_id", requestID(ctx))
		return
	}
	if database == nil || audit == nil || uploader == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	staffRecord, err := database.FindByID(ctx, staffID)
	if errors.Is(err, domain.ErrStaffNotFound) {
		writeError(w, http.StatusNotFound, "staff_not_found", requestID(ctx))
		return
	}
	if err != nil {
		logger.ErrorContext(ctx, "load staff before profile picture upload", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		return
	}
	if (staffRecord.AccountID == nil || *staffRecord.AccountID != principal.ID) &&
		!hasPermission(principal.Permissions, "staff.update") {
		writeError(w, http.StatusForbidden, "permission_denied", requestID(ctx))
		return
	}
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data;") {
		writeError(w, http.StatusBadRequest, "invalid_image", requestID(ctx))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxProfileImageBody)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(w, http.StatusRequestEntityTooLarge, "image_too_large", requestID(ctx))
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_image", requestID(ctx))
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	files := r.MultipartForm.File["file"]
	if len(files) != 1 {
		writeError(w, http.StatusBadRequest, "invalid_image", requestID(ctx))
		return
	}
	file, err := files[0].Open()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_image", requestID(ctx))
		return
	}
	defer file.Close()
	imageBytes, err := io.ReadAll(io.LimitReader(file, maxProfileImageSize+1))
	if err != nil {
		logger.ErrorContext(ctx, "read profile picture", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusBadRequest, "invalid_image", requestID(ctx))
		return
	}
	if len(imageBytes) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_image", requestID(ctx))
		return
	}
	if len(imageBytes) > maxProfileImageSize {
		writeError(w, http.StatusRequestEntityTooLarge, "image_too_large", requestID(ctx))
		return
	}
	contentType := http.DetectContentType(imageBytes)
	expectedFormat := map[string]string{
		"image/jpeg": "jpeg",
		"image/png":  "png",
		"image/gif":  "gif",
	}[contentType]
	if expectedFormat == "" {
		writeError(w, http.StatusBadRequest, "unsupported_image_type", requestID(ctx))
		return
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(imageBytes))
	if err != nil || format != expectedFormat || config.Width < 1 || config.Height < 1 ||
		config.Width > 10000 || config.Height > 10000 || config.Width > 40000000/config.Height {
		writeError(w, http.StatusBadRequest, "invalid_image", requestID(ctx))
		return
	}
	eventID, err := newUUID()
	if err != nil {
		logger.ErrorContext(ctx, "generate profile picture audit identifier", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		return
	}
	if err := recordOrganizationAudit(ctx, audit, logger, application.AuditEvent{
		EventKey: "organization:staff.profile_picture.upload:" + eventID,
		ActorID:  principal.ID, Action: "staff.profile_picture.upload.requested", ResourceID: staffID,
		Details: map[string]any{"content_type": contentType, "size_bytes": len(imageBytes)},
	}); err != nil {
		writeError(w, http.StatusServiceUnavailable, "audit_unavailable", requestID(ctx))
		return
	}
	imageURL, err := uploader.Upload(ctx, contentType, imageBytes)
	if err != nil {
		logger.ErrorContext(ctx, "upload profile picture to Cloudinary", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusBadGateway, "image_upload_failed", requestID(ctx))
		return
	}
	staff, err := database.UpdateProfileImage(ctx, staffID, imageURL)
	switch {
	case errors.Is(err, domain.ErrStaffNotFound):
		writeError(w, http.StatusNotFound, "staff_not_found", requestID(ctx))
	case err != nil:
		logger.ErrorContext(ctx, "save profile picture URL", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
	default:
		writeJSON(w, http.StatusOK, map[string]any{"staff": staff})
	}
}

func updateStaff(w http.ResponseWriter, r *http.Request, database store, authenticator application.Authenticator, audit application.AuditRecorder, logger *slog.Logger) {
	ctx, cancel, principal, ok := authorizeRequest(w, r, authenticator, logger, "staff.update", true)
	if !ok {
		return
	}
	defer cancel()
	staffID := strings.TrimSpace(r.PathValue("staffId"))
	if !domain.IsUUID(staffID) {
		writeError(w, http.StatusBadRequest, "invalid_staff_id", requestID(ctx))
		return
	}
	if database == nil || audit == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	var payload domain.UpdateStaffRequest
	if err := decodeJSONRequest(w, r, &payload, maxRequestBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID(ctx))
		return
	}
	if err := payload.NormalizeAndValidate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_staff_record", requestID(ctx))
		return
	}
	if payload.WorkEmail.Set && payload.WorkEmail.Value != nil {
		address, err := mail.ParseAddress(*payload.WorkEmail.Value)
		if err != nil || address.Address != *payload.WorkEmail.Value {
			writeError(w, http.StatusBadRequest, "invalid_staff_record", requestID(ctx))
			return
		}
	}
	eventID, err := newUUID()
	if err != nil {
		logger.ErrorContext(ctx, "generate staff update audit identifier", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		return
	}
	if err := recordOrganizationAudit(ctx, audit, logger, application.AuditEvent{
		EventKey: "organization:staff.update:" + eventID,
		ActorID:  principal.ID, Action: "staff.update.requested", ResourceID: staffID,
		Details: map[string]any{"fields": changedStaffFields(payload)},
	}); err != nil {
		writeError(w, http.StatusServiceUnavailable, "audit_unavailable", requestID(ctx))
		return
	}
	staff, err := database.UpdateStaff(ctx, staffID, payload)
	switch {
	case errors.Is(err, domain.ErrStaffNotFound):
		writeError(w, http.StatusNotFound, "staff_not_found", requestID(ctx))
	case errors.Is(err, domain.ErrDepartmentNotFound):
		writeError(w, http.StatusBadRequest, "department_not_found", requestID(ctx))
	case err != nil:
		logger.ErrorContext(ctx, "update staff record", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
	default:
		writeJSON(w, http.StatusOK, map[string]any{"staff": staff})
	}
}

func changedStaffFields(payload domain.UpdateStaffRequest) []string {
	fields := make([]string, 0, 4)
	if payload.FullName.Set {
		fields = append(fields, "full_name")
	}
	if payload.WorkEmail.Set {
		fields = append(fields, "work_email")
	}
	if payload.JobTitle.Set {
		fields = append(fields, "job_title")
	}
	if payload.DepartmentID.Set {
		fields = append(fields, "department_id")
	}
	return fields
}

func deactivateStaff(w http.ResponseWriter, r *http.Request, database store, authenticator application.Authenticator, audit application.AuditRecorder, onboarding *application.OnboardingServices, logger *slog.Logger) {
	ctx, cancel, principal, ok := authorizeRequest(w, r, authenticator, logger, "staff.deactivate", true)
	if !ok {
		return
	}
	defer cancel()
	staffID := strings.TrimSpace(r.PathValue("staffId"))
	if !domain.IsUUID(staffID) {
		writeError(w, http.StatusBadRequest, "invalid_staff_id", requestID(ctx))
		return
	}
	if database == nil || audit == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	existing, err := database.FindByID(ctx, staffID)
	if errors.Is(err, domain.ErrStaffNotFound) {
		writeError(w, http.StatusNotFound, "staff_not_found", requestID(ctx))
		return
	}
	if err != nil {
		logger.ErrorContext(ctx, "load staff before deactivation", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		return
	}
	eventID, err := newUUID()
	if err != nil {
		logger.ErrorContext(ctx, "generate staff deactivation audit identifier", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		return
	}
	if err := recordOrganizationAudit(ctx, audit, logger, application.AuditEvent{
		EventKey: "organization:staff.deactivate:" + eventID,
		ActorID:  principal.ID, Action: "staff.deactivate.requested", ResourceID: staffID,
	}); err != nil {
		writeError(w, http.StatusServiceUnavailable, "audit_unavailable", requestID(ctx))
		return
	}
	if existing.AccountID != nil && existing.EmploymentStatus != "inactive" &&
		existing.EmploymentStatus != "deactivated" {
		if onboarding == nil || onboarding.IdentityState == nil {
			writeError(w, http.StatusServiceUnavailable, "identity_unavailable", requestID(ctx))
			return
		}
		if err := onboarding.IdentityState.SuspendAccount(ctx, existing.StaffID, principal.ID); err != nil {
			logger.ErrorContext(ctx, "suspend login account before staff deactivation",
				"request_id", requestID(ctx), "staff_id", existing.StaffID, "error", err)
			writeError(w, http.StatusServiceUnavailable, "account_suspension_unavailable", requestID(ctx))
			return
		}
	}
	staff, err := database.DeactivateStaff(ctx, staffID)
	if errors.Is(err, domain.ErrStaffNotFound) {
		writeError(w, http.StatusNotFound, "staff_not_found", requestID(ctx))
		return
	}
	if err != nil {
		logger.ErrorContext(ctx, "deactivate staff record", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"staff": staff})
}

func listDepartments(w http.ResponseWriter, r *http.Request, database store, authenticator application.Authenticator, logger *slog.Logger) {
	ctx, cancel, _, ok := authorizeRequest(w, r, authenticator, logger, "staff.read", false)
	if !ok {
		return
	}
	defer cancel()
	if database == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	departments, err := database.ListDepartments(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "list departments", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"departments": departments})
}

func createDepartment(w http.ResponseWriter, r *http.Request, database store, authenticator application.Authenticator, audit application.AuditRecorder, logger *slog.Logger) {
	ctx, cancel, principal, ok := authorizeRequest(w, r, authenticator, logger, "departments.manage", true)
	if !ok {
		return
	}
	defer cancel()
	if database == nil || audit == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	var payload domain.DepartmentRequest
	if err := decodeJSONRequest(w, r, &payload, maxRequestBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID(ctx))
		return
	}
	if err := payload.NormalizeAndValidate(true); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_department", requestID(ctx))
		return
	}
	id, err := newUUID()
	if err != nil {
		logger.ErrorContext(ctx, "generate department identifier", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		return
	}
	if err := recordOrganizationAudit(ctx, audit, logger, application.AuditEvent{
		EventKey: "organization:department.create:" + id,
		ActorID:  principal.ID, Action: "department.create.requested", ResourceID: id,
		Details: map[string]any{"name": *payload.Name.Value},
	}); err != nil {
		writeError(w, http.StatusServiceUnavailable, "audit_unavailable", requestID(ctx))
		return
	}
	department, err := database.CreateDepartment(ctx, id, payload)
	if errors.Is(err, domain.ErrDepartmentConflict) {
		writeError(w, http.StatusConflict, "department_name_conflict", requestID(ctx))
		return
	}
	if err != nil {
		logger.ErrorContext(ctx, "create department", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"department": department})
}

func updateDepartment(w http.ResponseWriter, r *http.Request, database store, authenticator application.Authenticator, audit application.AuditRecorder, logger *slog.Logger) {
	ctx, cancel, principal, ok := authorizeRequest(w, r, authenticator, logger, "departments.manage", true)
	if !ok {
		return
	}
	defer cancel()
	departmentID := strings.TrimSpace(r.PathValue("departmentId"))
	if !domain.IsUUID(departmentID) {
		writeError(w, http.StatusBadRequest, "invalid_department_id", requestID(ctx))
		return
	}
	if database == nil || audit == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	var payload domain.DepartmentRequest
	if err := decodeJSONRequest(w, r, &payload, maxRequestBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID(ctx))
		return
	}
	if err := payload.NormalizeAndValidate(false); err != nil || (!payload.Name.Set && !payload.Description.Set) {
		writeError(w, http.StatusBadRequest, "invalid_department", requestID(ctx))
		return
	}
	fields := make([]string, 0, 2)
	if payload.Name.Set {
		fields = append(fields, "name")
	}
	if payload.Description.Set {
		fields = append(fields, "description")
	}
	eventID, err := newUUID()
	if err != nil {
		logger.ErrorContext(ctx, "generate department update audit identifier", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		return
	}
	if err := recordOrganizationAudit(ctx, audit, logger, application.AuditEvent{
		EventKey: "organization:department.update:" + eventID,
		ActorID:  principal.ID, Action: "department.update.requested", ResourceID: departmentID,
		Details: map[string]any{"fields": fields},
	}); err != nil {
		writeError(w, http.StatusServiceUnavailable, "audit_unavailable", requestID(ctx))
		return
	}
	department, err := database.UpdateDepartment(ctx, departmentID, payload)
	switch {
	case errors.Is(err, domain.ErrDepartmentNotFound):
		writeError(w, http.StatusNotFound, "department_not_found", requestID(ctx))
	case errors.Is(err, domain.ErrDepartmentConflict):
		writeError(w, http.StatusConflict, "department_name_conflict", requestID(ctx))
	case err != nil:
		logger.ErrorContext(ctx, "update department", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
	default:
		writeJSON(w, http.StatusOK, map[string]any{"department": department})
	}
}

func decodeJSONRequest(w http.ResponseWriter, r *http.Request, destination any, maxBytes int64) error {
	if !isJSONRequest(r) {
		return errors.New("content-type must be application/json")
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if !errors.Is(decoder.Decode(&extra), io.EOF) {
		return errors.New("request must contain exactly one JSON object")
	}
	return nil
}

func recordOrganizationAudit(ctx context.Context, audit application.AuditRecorder, logger *slog.Logger, event application.AuditEvent) error {
	if err := audit.Record(ctx, event); err != nil {
		logger.ErrorContext(ctx, "record Organization audit event",
			"request_id", requestID(ctx), "action", event.Action, "error", err)
		return err
	}
	return nil
}

func importStaff(w http.ResponseWriter, r *http.Request, database store, authenticator application.Authenticator, audit application.AuditRecorder, logger *slog.Logger) {
	ctx, cancel, principal, ok := authorizeRequest(w, r, authenticator, logger, "staff.import", true)
	if !ok {
		return
	}
	defer cancel()
	if database == nil || audit == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		writeError(w, http.StatusUnsupportedMediaType, "content_type_must_be_multipart_form_data", requestID(ctx))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBody)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "import_file_too_large", requestID(ctx))
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_import_upload", requestID(ctx))
		return
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}
	if r.MultipartForm == nil || len(r.MultipartForm.File) != 1 ||
		len(r.MultipartForm.File["file"]) != 1 || len(r.MultipartForm.Value) != 0 {
		writeError(w, http.StatusBadRequest, "file_field_required", requestID(ctx))
		return
	}
	fileHeader := r.MultipartForm.File["file"][0]
	if fileHeader.Size > maxImportFileSize {
		writeError(w, http.StatusRequestEntityTooLarge, "import_file_too_large", requestID(ctx))
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_import_upload", requestID(ctx))
		return
	}
	rows, result, err := parseStaffCSV(file)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		logger.ErrorContext(ctx, "read staff import CSV", "request_id", requestID(ctx), "error", errors.Join(err, closeErr))
		writeError(w, http.StatusBadRequest, "invalid_import_upload", requestID(ctx))
		return
	}
	batchID, err := newUUID()
	if err != nil {
		logger.ErrorContext(ctx, "generate staff import identifier", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		return
	}
	if err := recordOrganizationAudit(ctx, audit, logger, application.AuditEvent{
		EventKey: "organization:staff.import:" + batchID,
		ActorID:  principal.ID, Action: "staff.import.requested", ResourceID: batchID,
		Details: map[string]any{"total_rows": result.TotalRows},
	}); err != nil {
		writeError(w, http.StatusServiceUnavailable, "audit_unavailable", requestID(ctx))
		return
	}
	var batch domain.StaffImportBatch
	if result.ErrorCount > 0 {
		batch, err = database.RecordFailedStaffImport(ctx, batchID, principal.ID, result)
	} else {
		batch, err = database.ImportStaffBatch(ctx, batchID, principal.ID, rows)
	}
	if err != nil {
		logger.ErrorContext(ctx, "process staff import", "request_id", requestID(ctx), "batch_id", batchID, "error", err)
		writeError(w, http.StatusInternalServerError, "staff_import_failed", requestID(ctx))
		return
	}
	if batch.Status == "failed" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error": map[string]string{
				"code": "staff_import_validation_failed", "request_id": requestID(ctx),
			},
			"import": batch,
		})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"import": batch})
}

func parseStaffCSV(input io.Reader) ([]domain.ImportStaffRow, domain.StaffImportResult, error) {
	reader := csv.NewReader(input)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return nil, domain.StaffImportResult{
			ErrorCount: 1,
			Errors:     []domain.StaffImportIssue{{Row: 1, Code: "missing_header", Message: "CSV header row is required"}},
		}, nil
	}
	if err != nil {
		return nil, domain.StaffImportResult{
			ErrorCount: 1,
			Errors:     []domain.StaffImportIssue{{Row: 1, Code: "invalid_csv", Message: "CSV header row is invalid"}},
		}, nil
	}
	if len(header) > 5 {
		return nil, domain.StaffImportResult{
			ErrorCount: 1,
			Errors:     []domain.StaffImportIssue{{Row: 1, Code: "too_many_columns", Message: "CSV cannot contain more than five columns"}},
		}, nil
	}
	indexes := make(map[string]int, len(header))
	allowed := map[string]bool{
		"staff_id": true, "full_name": true, "work_email": true,
		"job_title": true, "department_id": true,
	}
	issues := make([]domain.StaffImportIssue, 0)
	for index, value := range header {
		name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "\ufeff")))
		if name == "" || !allowed[name] {
			issues = appendImportIssue(issues, domain.StaffImportIssue{
				Row: 1, Code: "invalid_header", Message: "CSV contains an empty or unsupported column",
			})
			continue
		}
		if _, exists := indexes[name]; exists {
			issues = appendImportIssue(issues, domain.StaffImportIssue{
				Row: 1, Code: "duplicate_header", Message: "CSV contains a duplicate column",
			})
			continue
		}
		indexes[name] = index
	}
	for _, required := range []string{"staff_id", "full_name"} {
		if _, exists := indexes[required]; !exists {
			issues = appendImportIssue(issues, domain.StaffImportIssue{
				Row: 1, Code: "missing_required_column", Message: "CSV must include staff_id and full_name columns",
			})
		}
	}
	if len(issues) > 0 {
		return nil, domain.StaffImportResult{ErrorCount: len(issues), Errors: issues}, nil
	}

	rows := make([]domain.ImportStaffRow, 0)
	seenStaffIDs := make(map[string]int)
	result := domain.StaffImportResult{}
	recordNumber := 1
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		recordNumber++
		if err != nil {
			rowNumber := recordNumber
			var parseErr *csv.ParseError
			if errors.As(err, &parseErr) {
				rowNumber = parseErr.Line
			}
			result.TotalRows++
			result.ErrorCount++
			issues = appendImportIssue(issues, domain.StaffImportIssue{
				Row: rowNumber, Code: "invalid_csv", Message: "CSV row is malformed",
			})
			break
		}
		blank := true
		for _, value := range record {
			if strings.TrimSpace(value) != "" {
				blank = false
				break
			}
		}
		if blank {
			continue
		}
		result.TotalRows++
		if result.TotalRows > maxImportRows {
			result.ErrorCount++
			issues = appendImportIssue(issues, domain.StaffImportIssue{
				Row: recordNumber, Code: "row_limit_exceeded", Message: "CSV cannot contain more than 5000 data rows",
			})
			break
		}
		if len(record) != len(header) {
			result.ErrorCount++
			issues = appendImportIssue(issues, domain.StaffImportIssue{
				Row: recordNumber, Code: "invalid_column_count", Message: "CSV row has a different number of values than the header",
			})
			continue
		}
		field := func(name string) string {
			index, exists := indexes[name]
			if !exists {
				return ""
			}
			return record[index]
		}
		request := domain.CreateStaffRequest{
			StaffID: field("staff_id"), FullName: field("full_name"),
			WorkEmail:    optionalCSVValue(field("work_email")),
			JobTitle:     optionalCSVValue(field("job_title")),
			DepartmentID: optionalCSVValue(field("department_id")),
		}
		row := domain.ImportStaffRow{RowNumber: recordNumber, Staff: request}
		if err := row.Staff.NormalizeAndValidate(); err != nil {
			result.ErrorCount++
			issues = appendImportIssue(issues, domain.StaffImportIssue{
				Row: recordNumber, StaffID: strings.TrimSpace(request.StaffID),
				Code: "invalid_staff_record", Message: "staff ID, name, or optional field is invalid",
			})
			continue
		}
		if request.WorkEmail != nil {
			address, err := mail.ParseAddress(*request.WorkEmail)
			if err != nil || address.Address != *request.WorkEmail {
				result.ErrorCount++
				issues = appendImportIssue(issues, domain.StaffImportIssue{
					Row: recordNumber, StaffID: request.StaffID,
					Code: "invalid_email", Message: "work_email is invalid",
				})
				continue
			}
		}
		if firstRow, exists := seenStaffIDs[request.StaffID]; exists {
			result.ErrorCount++
			issues = appendImportIssue(issues, domain.StaffImportIssue{
				Row: recordNumber, StaffID: request.StaffID,
				Code:    "duplicate_staff_id_in_file",
				Message: fmt.Sprintf("staff ID is duplicated in CSV row %d", firstRow),
			})
			continue
		}
		seenStaffIDs[request.StaffID] = recordNumber
		rows = append(rows, row)
	}
	if result.TotalRows == 0 && result.ErrorCount == 0 {
		result.ErrorCount = 1
		issues = appendImportIssue(issues, domain.StaffImportIssue{
			Row: 1, Code: "empty_import", Message: "CSV must contain at least one staff record",
		})
	}
	result.ErrorsTruncated = result.ErrorCount > len(issues)
	result.Errors = issues
	return rows, result, nil
}

func optionalCSVValue(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func appendImportIssue(issues []domain.StaffImportIssue, issue domain.StaffImportIssue) []domain.StaffImportIssue {
	if len(issues) >= maxImportIssues {
		return issues
	}
	return append(issues, issue)
}

func getStaffImport(w http.ResponseWriter, r *http.Request, database store, authenticator application.Authenticator, logger *slog.Logger) {
	ctx, cancel, _, ok := authorizeRequest(w, r, authenticator, logger, "staff.import", false)
	if !ok {
		return
	}
	defer cancel()
	if database == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(ctx))
		return
	}
	importID := strings.TrimSpace(r.PathValue("importId"))
	if !domain.IsUUID(importID) {
		writeError(w, http.StatusBadRequest, "invalid_import_id", requestID(ctx))
		return
	}
	batch, err := database.FindStaffImportBatch(ctx, importID)
	if errors.Is(err, domain.ErrImportNotFound) {
		writeError(w, http.StatusNotFound, "staff_import_not_found", requestID(ctx))
		return
	}
	if err != nil {
		logger.ErrorContext(ctx, "get staff import result", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"import": batch})
}

func provisionStaff(w http.ResponseWriter, r *http.Request, database store, authenticator application.Authenticator, audit application.AuditRecorder, onboarding *application.OnboardingServices, logger *slog.Logger) {
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "invalid_origin", requestID(r.Context()))
		return
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", requestID(r.Context()))
		return
	}
	if authenticator == nil || database == nil || audit == nil || onboarding == nil ||
		onboarding.Identity == nil || onboarding.Access == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(r.Context()))
		return
	}
	staffID := strings.TrimSpace(r.PathValue("staffID"))
	if !domain.IsUUID(staffID) {
		writeError(w, http.StatusBadRequest, "invalid_staff_id", requestID(r.Context()))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	ctx = application.ContextWithRequestID(ctx, requestID(ctx))
	principal, err := authenticator.Authenticate(ctx, cookie.Value)
	if err != nil {
		if errors.Is(err, application.ErrUnauthenticated) {
			writeError(w, http.StatusUnauthorized, "unauthorized", requestID(r.Context()))
			return
		}
		logger.ErrorContext(ctx, "validate Organization provisioning session", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusServiceUnavailable, "identity_unavailable", requestID(ctx))
		return
	}
	if !hasPermission(principal.Permissions, "user_roles.assign") {
		writeError(w, http.StatusForbidden, "permission_denied", requestID(r.Context()))
		return
	}
	staff, err := database.FindByID(ctx, staffID)
	if err != nil {
		if errors.Is(err, domain.ErrStaffNotFound) {
			writeError(w, http.StatusNotFound, "staff_not_found", requestID(ctx))
			return
		}
		logger.ErrorContext(ctx, "load staff for provisioning", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		return
	}
	if staff.EmploymentStatus == "active" && staff.AccountID != nil {
		if err := audit.Record(ctx, application.AuditEvent{
			EventKey:   "organization:staff.provision.completed:" + staff.ID,
			ActorID:    principal.ID,
			Action:     "staff.provision.completed",
			ResourceID: staff.ID,
			Details:    map[string]any{"staff_id": staff.StaffID},
		}); err != nil {
			logger.ErrorContext(ctx, "record completed staff provisioning", "request_id", requestID(ctx), "error", err)
			writeError(w, http.StatusServiceUnavailable, "audit_unavailable", requestID(ctx))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"staff": staff, "invitation_sent": true})
		return
	}
	if staff.EmploymentStatus != "pending" || staff.AccountID != nil {
		writeError(w, http.StatusConflict, "staff_not_provisionable", requestID(ctx))
		return
	}
	if staff.WorkEmail == nil || strings.TrimSpace(*staff.WorkEmail) == "" {
		writeError(w, http.StatusBadRequest, "work_email_required", requestID(ctx))
		return
	}
	if utf8.RuneCountInString(staff.FullName) > 160 {
		writeError(w, http.StatusBadRequest, "invalid_staff_name", requestID(ctx))
		return
	}
	if err := audit.Record(ctx, application.AuditEvent{
		EventKey:   "organization:staff.provision:" + staff.ID,
		ActorID:    principal.ID,
		Action:     "staff.provision.requested",
		ResourceID: staff.ID,
		Details:    map[string]any{"staff_id": staff.StaffID},
	}); err != nil {
		logger.ErrorContext(ctx, "record staff provisioning request", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusServiceUnavailable, "audit_unavailable", requestID(ctx))
		return
	}

	accountID, err := onboarding.Identity.ProvisionAccount(ctx, staff.StaffID, *staff.WorkEmail, staff.FullName)
	if err != nil {
		logger.ErrorContext(ctx, "prepare Identity account", "request_id", requestID(ctx), "error", err)
		if errors.Is(err, application.ErrAccountProvisioningConflict) {
			writeError(w, http.StatusConflict, "account_provisioning_conflict", requestID(ctx))
			return
		}
		writeError(w, http.StatusServiceUnavailable, "identity_unavailable", requestID(ctx))
		return
	}
	if err := onboarding.Access.AssignDefaultStaffRole(ctx, accountID); err != nil {
		logger.ErrorContext(ctx, "assign default Staff role", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusServiceUnavailable, "access_unavailable", requestID(ctx))
		return
	}
	if err := onboarding.Identity.SendActivation(ctx, accountID); err != nil {
		logger.ErrorContext(ctx, "send staff activation email", "request_id", requestID(ctx), "error", err)
		if errors.Is(err, application.ErrEmailDeliveryUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "email_delivery_disabled", requestID(ctx))
			return
		}
		writeError(w, http.StatusServiceUnavailable, "activation_unavailable", requestID(ctx))
		return
	}
	staff, err = database.CompleteProvisioning(ctx, staff.ID, accountID)
	if err != nil {
		logger.ErrorContext(ctx, "complete Organization staff provisioning", "request_id", requestID(ctx), "error", err)
		if errors.Is(err, domain.ErrStaffAlreadyProvisioned) {
			writeError(w, http.StatusConflict, "staff_not_provisionable", requestID(ctx))
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		return
	}
	if err := audit.Record(ctx, application.AuditEvent{
		EventKey:   "organization:staff.provision.completed:" + staff.ID,
		ActorID:    principal.ID,
		Action:     "staff.provision.completed",
		ResourceID: staff.ID,
		Details:    map[string]any{"staff_id": staff.StaffID},
	}); err != nil {
		logger.ErrorContext(ctx, "record completed staff provisioning", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusServiceUnavailable, "audit_unavailable", requestID(ctx))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"staff": staff, "invitation_sent": true})
}

func createStaff(w http.ResponseWriter, r *http.Request, database store, authenticator application.Authenticator, audit application.AuditRecorder, logger *slog.Logger) {
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "invalid_origin", requestID(r.Context()))
		return
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", requestID(r.Context()))
		return
	}
	if authenticator == nil {
		writeError(w, http.StatusServiceUnavailable, "identity_unavailable", requestID(r.Context()))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	ctx = application.ContextWithRequestID(ctx, requestID(ctx))
	principal, err := authenticator.Authenticate(ctx, cookie.Value)
	if err != nil {
		if errors.Is(err, application.ErrUnauthenticated) {
			writeError(w, http.StatusUnauthorized, "unauthorized", requestID(r.Context()))
			return
		}
		logger.ErrorContext(ctx, "validate Organization request session", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusServiceUnavailable, "identity_unavailable", requestID(ctx))
		return
	}
	if !hasPermission(principal.Permissions, "staff.create") {
		writeError(w, http.StatusForbidden, "permission_denied", requestID(r.Context()))
		return
	}
	if database == nil || audit == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(r.Context()))
		return
	}
	if !isJSONRequest(r) {
		writeError(w, http.StatusUnsupportedMediaType, "content_type_must_be_application_json", requestID(r.Context()))
		return
	}
	var payload domain.CreateStaffRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID(r.Context()))
		return
	}
	var extra any
	if !errors.Is(decoder.Decode(&extra), io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID(r.Context()))
		return
	}
	if err := payload.NormalizeAndValidate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_staff_record", requestID(r.Context()))
		return
	}
	if payload.WorkEmail != nil {
		address, err := mail.ParseAddress(*payload.WorkEmail)
		if err != nil || address.Address != *payload.WorkEmail {
			writeError(w, http.StatusBadRequest, "invalid_staff_record", requestID(r.Context()))
			return
		}
	}
	staffID, err := newUUID()
	if err != nil {
		logger.ErrorContext(ctx, "generate staff identifier", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		return
	}

	event := application.AuditEvent{
		EventKey:   "organization:staff.create:" + staffID,
		ActorID:    principal.ID,
		Action:     "staff.create.requested",
		ResourceID: staffID,
		Details: map[string]any{
			"staff_id": payload.StaffID,
		},
	}
	if err := audit.Record(ctx, event); err != nil {
		logger.ErrorContext(ctx, "record staff creation audit event", "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusServiceUnavailable, "audit_unavailable", requestID(ctx))
		return
	}
	staff, err := database.Create(ctx, staffID, payload)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrDuplicateStaffID):
			writeError(w, http.StatusConflict, "duplicate_staff_id", requestID(ctx))
		case errors.Is(err, domain.ErrDepartmentNotFound):
			writeError(w, http.StatusBadRequest, "department_not_found", requestID(ctx))
		default:
			logger.ErrorContext(ctx, "create staff record", "request_id", requestID(ctx), "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
		}
		return
	}
	logger.InfoContext(ctx, "staff record created",
		"request_id", requestID(ctx),
		"actor_id", principal.ID,
		"resource_id", staff.ID,
		"employment_status", staff.EmploymentStatus,
	)
	writeJSON(w, http.StatusCreated, map[string]any{"staff": staff})
}

func hasPermission(permissions []string, required string) bool {
	for _, permission := range permissions {
		if permission == required {
			return true
		}
	}
	return false
}

func isJSONRequest(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "application/json"
}

func sameOrigin(r *http.Request) bool {
	rawOrigin := strings.TrimSpace(r.Header.Get("Origin"))
	if rawOrigin == "" {
		return false
	}
	origin, err := url.Parse(rawOrigin)
	if err != nil || origin.Host == "" || origin.User != nil || origin.Path != "" ||
		origin.RawQuery != "" || origin.Fragment != "" {
		return false
	}
	if origin.Scheme != "https" && !(origin.Scheme == "http" && r.TLS == nil) {
		return false
	}
	return strings.EqualFold(origin.Host, r.Host)
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate UUID: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return string(encoded), nil
}

func requestLoggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			logger.ErrorContext(r.Context(), "generate Organization request ID", "error", err)
		}
		requestID := hex.EncodeToString(id[:])
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", requestID)
		writer := &statusWriter{ResponseWriter: w}
		started := time.Now()
		next.ServeHTTP(writer, r)
		logger.InfoContext(ctx, "Organization HTTP request",
			"request_id", requestID,
			"method", r.Method,
			"route", organizationRoute(r.URL.Path),
			"status", writer.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func organizationRoute(path string) string {
	switch path {
	case "/health/live":
		return "health_live"
	case "/health/ready":
		return "health_ready"
	case "/api/v1/staff":
		return "create_staff"
	default:
		return "unmatched"
	}
}

func requestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status, w.wroteHeader = status, true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func writeError(w http.ResponseWriter, status int, code, requestID string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{
		"code": code, "request_id": requestID,
	}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
