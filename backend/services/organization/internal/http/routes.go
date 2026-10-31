package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/application"
	"github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/domain"
)

const (
	sessionCookieName = "mothercat_session"
	maxRequestBody    = 16 << 10
)

type store interface {
	Ping(context.Context) error
	Create(context.Context, string, domain.CreateStaffRequest) (domain.Staff, error)
	FindByID(context.Context, string) (domain.Staff, error)
	CompleteProvisioning(context.Context, string, string) (domain.Staff, error)
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
	mux.HandleFunc("POST /api/v1/staff", func(w http.ResponseWriter, r *http.Request) {
		createStaff(w, r, database, authenticator, audit, logger)
	})
	mux.HandleFunc("POST /api/v1/staff/{staffID}/provision", func(w http.ResponseWriter, r *http.Request) {
		var services *application.OnboardingServices
		if len(onboarding) > 0 {
			services = &onboarding[0]
		}
		provisionStaff(w, r, database, authenticator, audit, services, logger)
	})
	return requestLoggingMiddleware(logger, mux)
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
