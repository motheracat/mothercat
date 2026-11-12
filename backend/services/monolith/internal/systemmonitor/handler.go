package systemmonitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const sessionCookieName = "mothercat_session"

var ErrUnauthenticated = errors.New("unauthenticated")
var ErrPasswordVerificationFailed = errors.New("password verification failed")

type Authenticator interface {
	Authenticate(context.Context, string) (string, error)
}

type Handler struct {
	db            *sql.DB
	engine        *Engine
	authenticator Authenticator
	logger        *slog.Logger
	security      SecurityManager
}

type PasswordVerifier interface {
	VerifyPassword(context.Context, string, string) error
}

type SecurityManager interface {
	Status(context.Context, string) (SecurityStatus, error)
	BeginMFAEnrollment(context.Context, string) (MFAEnrollment, error)
	ConfirmMFAEnrollment(context.Context, string, string) error
	DisableMFA(context.Context, string, string) error
	VerifyMFA(context.Context, string, string) error
	ListSessions(context.Context, string, string) ([]MonitorSession, error)
	RevokeSession(context.Context, string, string, string, string) error
	Events(context.Context, int) ([]SecurityEvent, error)
	Alerts(context.Context, int) ([]SecurityAlert, error)
	AuditEvents(context.Context, int) ([]SecurityAuditEvent, error)
	ActiveGrants(context.Context, int) ([]MonitorAccessGrant, error)
}

func NewHandler(db *sql.DB, engine *Engine, authenticator Authenticator, logger *slog.Logger,
	securityServices ...SecurityManager) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	handler := &Handler{db: db, engine: engine, authenticator: authenticator, logger: logger}
	if len(securityServices) > 0 {
		handler.security = securityServices[0]
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/system-monitor/overview", handler.overview)
	mux.HandleFunc("GET /api/v1/system-monitor/endpoints", handler.endpoints)
	mux.HandleFunc("GET /api/v1/system-monitor/events", handler.events)
	mux.HandleFunc("POST /api/v1/system-monitor/endpoints/pause", handler.pauseEndpoint)
	mux.HandleFunc("POST /api/v1/system-monitor/endpoints/resume", handler.resumeEndpoint)
	mux.HandleFunc("GET /api/v1/system-monitor/endpoints/history", handler.endpointHistory)
	mux.HandleFunc("GET /api/v1/system-monitor/analytics", handler.endpointAnalytics)
	mux.HandleFunc("GET /api/v1/system-monitor/security/status", handler.securityStatus)
	mux.HandleFunc("GET /api/v1/system-monitor/security/console", handler.securityConsole)
	mux.HandleFunc("POST /api/v1/system-monitor/security/mfa/enroll", handler.beginMFAEnrollment)
	mux.HandleFunc("POST /api/v1/system-monitor/security/mfa/confirm", handler.confirmMFAEnrollment)
	mux.HandleFunc("POST /api/v1/system-monitor/security/mfa/disable", handler.disableMFA)
	mux.HandleFunc("GET /api/v1/system-monitor/security/sessions", handler.securitySessions)
	mux.HandleFunc("POST /api/v1/system-monitor/security/sessions/revoke", handler.revokeSecuritySession)
	mux.HandleFunc("GET /api/v1/system-monitor/security/events", handler.securityEvents)
	mux.HandleFunc("GET /api/v1/system-monitor/security/alerts", handler.securityAlerts)
	mux.HandleFunc("GET /api/v1/system-monitor/security/audit", handler.securityAudit)
	mux.HandleFunc("GET /api/v1/system-monitor/security/grants", handler.securityGrants)
	return mux
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorize(w, r)
	if !ok {
		return
	}
	defer cancel()
	writeJSON(w, http.StatusOK, map[string]any{"overview": h.engine.Overview(ctx, h.db)})
}

func (h *Handler) endpoints(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorize(w, r)
	if !ok {
		return
	}
	defer cancel()
	if err := h.engine.RefreshControls(ctx); err != nil {
		h.logger.ErrorContext(ctx, "load endpoint control state", "error", err)
		writeError(w, http.StatusServiceUnavailable, "endpoint_controls_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"endpoints": h.engine.Endpoints()})
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	_, cancel, ok := h.authorize(w, r)
	if !ok {
		return
	}
	defer cancel()
	query := r.URL.Query()
	if len(query) > 1 || len(query["limit"]) > 1 {
		writeError(w, http.StatusBadRequest, "invalid_query")
		return
	}
	limit := 50
	if value := query.Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > eventCapacity {
			writeError(w, http.StatusBadRequest, "invalid_query")
			return
		}
		limit = parsed
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events": h.engine.Events(limit),
		"retention": "process-local; recent events are lost on restart",
	})
}

type pauseEndpointPayload struct {
	Method          string    `json:"method"`
	Route           string    `json:"route"`
	Reason          string    `json:"reason"`
	ResponseStatus  int       `json:"response_status"`
	ResponseMessage string    `json:"response_message"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type resumeEndpointPayload struct {
	Method string `json:"method"`
	Route  string `json:"route"`
	Reason string `json:"reason"`
}

func (h *Handler) pauseEndpoint(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorizeMFA(w, r)
	if !ok {
		return
	}
	defer cancel()
	if !validActionRequest(w, r) {
		return
	}
	var payload pauseEndpointPayload
	if !decodeActionPayload(w, r, &payload) {
		return
	}
	err := h.engine.PauseEndpoint(ctx, h.db, PauseRequest{
		Method: payload.Method, Route: payload.Route, ActorID: accountIDFromContext(ctx),
		Reason: payload.Reason, ResponseStatus: payload.ResponseStatus,
		ResponseMessage: payload.ResponseMessage, ExpiresAt: payload.ExpiresAt,
	})
	if err != nil {
		h.writeControlError(ctx, w, err, "pause endpoint")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (h *Handler) resumeEndpoint(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorizeMFA(w, r)
	if !ok {
		return
	}
	defer cancel()
	if !validActionRequest(w, r) {
		return
	}
	var payload resumeEndpointPayload
	if !decodeActionPayload(w, r, &payload) {
		return
	}
	err := h.engine.ResumeEndpoint(ctx, h.db, ResumeRequest{
		Method: payload.Method, Route: payload.Route, ActorID: accountIDFromContext(ctx),
		Reason: payload.Reason,
	})
	if err != nil {
		h.writeControlError(ctx, w, err, "resume endpoint")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

func (h *Handler) endpointHistory(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorize(w, r)
	if !ok {
		return
	}
	defer cancel()
	query := r.URL.Query()
	if len(query) > 1 || len(query["limit"]) > 1 {
		writeError(w, http.StatusBadRequest, "invalid_query")
		return
	}
	limit := 50
	if value := query.Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > maxHistoryLimit {
			writeError(w, http.StatusBadRequest, "invalid_query")
			return
		}
		limit = parsed
	}
	history, err := EndpointControlHistory(ctx, h.db, limit)
	if err != nil {
		h.logger.ErrorContext(ctx, "read endpoint control history", "error", err)
		writeError(w, http.StatusServiceUnavailable, "endpoint_history_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": history})
}

func (h *Handler) endpointAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorize(w, r)
	if !ok {
		return
	}
	defer cancel()
	queryValues := r.URL.Query()
	for key := range queryValues {
		switch key {
		case "method", "route", "from", "to", "interval":
		default:
			writeError(w, http.StatusBadRequest, "invalid_query")
			return
		}
		if len(queryValues[key]) != 1 {
			writeError(w, http.StatusBadRequest, "invalid_query")
			return
		}
	}
	method := queryValues.Get("method")
	route := queryValues.Get("route")
	if method == "" || route == "" {
		writeError(w, http.StatusBadRequest, "invalid_query")
		return
	}
	now := time.Now().UTC()
	to := now
	from := now.Add(-24 * time.Hour)
	interval := queryValues.Get("interval")
	if interval == "" {
		interval = "hour"
	}
	if value := queryValues.Get("from"); value != "" {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_query")
			return
		}
		from = parsed
	}
	if value := queryValues.Get("to"); value != "" {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_query")
			return
		}
		to = parsed
	}
	report, err := EndpointAnalytics(ctx, h.db, h.engine.catalog, AnalyticsQuery{
		Method: method, Route: route, From: from, To: to, Interval: interval,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrEndpointNotFound):
			writeError(w, http.StatusNotFound, "endpoint_not_found")
		case errors.Is(err, ErrInvalidAnalyticsQuery):
			writeError(w, http.StatusBadRequest, "invalid_query")
		default:
			h.logger.ErrorContext(ctx, "read endpoint analytics", "error", err)
			writeError(w, http.StatusServiceUnavailable, "analytics_unavailable")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"analytics": report})
}

type mfaEnrollPayload struct {
	Password string `json:"password"`
}

type mfaCodePayload struct {
	Code string `json:"code"`
}

type mfaDisablePayload struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

type revokeSessionPayload struct {
	SessionID string `json:"session_id"`
	Reason    string `json:"reason"`
}

func (h *Handler) securityStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorize(w, r)
	if !ok {
		return
	}
	defer cancel()
	if !onlyAllowedQuery(r, nil) {
		writeError(w, http.StatusBadRequest, "invalid_query")
		return
	}
	if h.security == nil {
		writeError(w, http.StatusServiceUnavailable, "security_unavailable")
		return
	}
	status, err := h.security.Status(ctx, accountIDFromContext(ctx))
	if err != nil {
		h.writeSecurityError(ctx, w, err, "read MFA status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"security": status})
}

func (h *Handler) securityConsole(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorizeMFA(w, r)
	if !ok {
		return
	}
	defer cancel()
	if !onlyAllowedQuery(r, nil) {
		writeError(w, http.StatusBadRequest, "invalid_query")
		return
	}
	cookie, _ := r.Cookie(sessionCookieName)
	accountID := accountIDFromContext(ctx)
	status, err := h.security.Status(ctx, accountID)
	if err != nil {
		h.writeSecurityError(ctx, w, err, "read MFA status")
		return
	}
	sessions, err := h.security.ListSessions(ctx, accountID, tokenHash(cookie.Value))
	if err != nil {
		h.writeSecurityError(ctx, w, err, "list sessions")
		return
	}
	events, err := h.security.Events(ctx, 50)
	if err != nil {
		h.writeSecurityError(ctx, w, err, "read security events")
		return
	}
	alerts, err := h.security.Alerts(ctx, 50)
	if err != nil {
		h.writeSecurityError(ctx, w, err, "read security alerts")
		return
	}
	audit, err := h.security.AuditEvents(ctx, 50)
	if err != nil {
		h.writeSecurityError(ctx, w, err, "read security audit trail")
		return
	}
	grants, err := h.security.ActiveGrants(ctx, 50)
	if err != nil {
		h.writeSecurityError(ctx, w, err, "read Monitor access grants")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"security": status, "sessions": sessions, "events": events,
		"alerts": alerts, "audit": audit, "grants": grants,
		"device_metadata_available": false,
	})
}

func (h *Handler) beginMFAEnrollment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorize(w, r)
	if !ok {
		return
	}
	defer cancel()
	if h.security == nil {
		writeError(w, http.StatusServiceUnavailable, "security_unavailable")
		return
	}
	if !validActionRequest(w, r) {
		return
	}
	var payload mfaEnrollPayload
	if !decodeActionPayload(w, r, &payload) {
		return
	}
	verifier, ok := h.authenticator.(PasswordVerifier)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "password_verification_unavailable")
		return
	}
	if err := verifier.VerifyPassword(ctx, accountIDFromContext(ctx), payload.Password); err != nil {
		if errors.Is(err, ErrPasswordVerificationFailed) {
			writeError(w, http.StatusUnauthorized, "reauthentication_failed")
		} else {
			h.logger.ErrorContext(ctx, "verify password for System Monitor MFA enrollment", "error", err)
			writeError(w, http.StatusServiceUnavailable, "reauthentication_unavailable")
		}
		return
	}
	enrollment, err := h.security.BeginMFAEnrollment(ctx, accountIDFromContext(ctx))
	if err != nil {
		h.writeSecurityError(ctx, w, err, "begin MFA enrollment")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"enrollment": enrollment})
}

func (h *Handler) confirmMFAEnrollment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorize(w, r)
	if !ok {
		return
	}
	defer cancel()
	if h.security == nil {
		writeError(w, http.StatusServiceUnavailable, "security_unavailable")
		return
	}
	if !validActionRequest(w, r) {
		return
	}
	var payload mfaCodePayload
	if !decodeActionPayload(w, r, &payload) {
		return
	}
	if err := h.security.ConfirmMFAEnrollment(ctx, accountIDFromContext(ctx), payload.Code); err != nil {
		h.writeSecurityError(ctx, w, err, "confirm MFA enrollment")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "enabled"})
}

func (h *Handler) disableMFA(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorize(w, r)
	if !ok {
		return
	}
	defer cancel()
	if h.security == nil {
		writeError(w, http.StatusServiceUnavailable, "security_unavailable")
		return
	}
	if !validActionRequest(w, r) {
		return
	}
	var payload mfaDisablePayload
	if !decodeActionPayload(w, r, &payload) {
		return
	}
	if !validReason(payload.Reason) || !validMFACode(payload.Code) {
		writeError(w, http.StatusBadRequest, "invalid_security_action")
		return
	}
	if err := h.security.VerifyMFA(ctx, accountIDFromContext(ctx), payload.Code); err != nil {
		h.writeSecurityError(ctx, w, err, "verify MFA before disabling")
		return
	}
	if err := h.security.DisableMFA(ctx, accountIDFromContext(ctx), payload.Reason); err != nil {
		h.writeSecurityError(ctx, w, err, "disable MFA")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

func (h *Handler) securitySessions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorizeMFA(w, r)
	if !ok {
		return
	}
	defer cancel()
	if !onlyAllowedQuery(r, nil) {
		writeError(w, http.StatusBadRequest, "invalid_query")
		return
	}
	cookie, _ := r.Cookie(sessionCookieName)
	sessions, err := h.security.ListSessions(ctx, accountIDFromContext(ctx), tokenHash(cookie.Value))
	if err != nil {
		h.writeSecurityError(ctx, w, err, "list sessions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions, "device_metadata_available": false})
}

func (h *Handler) revokeSecuritySession(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorizeMFA(w, r)
	if !ok {
		return
	}
	defer cancel()
	if !validActionRequest(w, r) {
		return
	}
	var payload revokeSessionPayload
	if !decodeActionPayload(w, r, &payload) {
		return
	}
	if !validUUID(payload.SessionID) || !validReason(payload.Reason) {
		writeError(w, http.StatusBadRequest, "invalid_security_action")
		return
	}
	cookie, _ := r.Cookie(sessionCookieName)
	err := h.security.RevokeSession(ctx, accountIDFromContext(ctx), payload.SessionID,
		tokenHash(cookie.Value), payload.Reason)
	if err != nil {
		switch {
		case errors.Is(err, ErrCurrentSession):
			writeError(w, http.StatusConflict, "cannot_revoke_current_session")
		case errors.Is(err, ErrSessionUnavailable):
			writeError(w, http.StatusNotFound, "session_not_found")
		default:
			h.writeSecurityError(ctx, w, err, "revoke session")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (h *Handler) securityEvents(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorizeMFA(w, r)
	if !ok {
		return
	}
	defer cancel()
	limit, valid := parseLimit(r, securityEventLimit)
	if !valid {
		writeError(w, http.StatusBadRequest, "invalid_query")
		return
	}
	events, err := h.security.Events(ctx, limit)
	if err != nil {
		h.writeSecurityError(ctx, w, err, "read security events")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (h *Handler) securityAlerts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorizeMFA(w, r)
	if !ok {
		return
	}
	defer cancel()
	limit, valid := parseLimit(r, securityEventLimit)
	if !valid {
		writeError(w, http.StatusBadRequest, "invalid_query")
		return
	}
	alerts, err := h.security.Alerts(ctx, limit)
	if err != nil {
		h.writeSecurityError(ctx, w, err, "read security alerts")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": alerts})
}

func (h *Handler) securityAudit(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorizeMFA(w, r)
	if !ok {
		return
	}
	defer cancel()
	limit, valid := parseLimit(r, securityEventLimit)
	if !valid {
		writeError(w, http.StatusBadRequest, "invalid_query")
		return
	}
	events, err := h.security.AuditEvents(ctx, limit)
	if err != nil {
		h.writeSecurityError(ctx, w, err, "read security audit trail")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (h *Handler) securityGrants(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorizeMFA(w, r)
	if !ok {
		return
	}
	defer cancel()
	limit, valid := parseLimit(r, securityEventLimit)
	if !valid {
		writeError(w, http.StatusBadRequest, "invalid_query")
		return
	}
	grants, err := h.security.ActiveGrants(ctx, limit)
	if err != nil {
		h.writeSecurityError(ctx, w, err, "read Monitor access grants")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"grants": grants})
}

func (h *Handler) authorizeMFA(w http.ResponseWriter, r *http.Request) (context.Context, context.CancelFunc, bool) {
	ctx, cancel, ok := h.authorize(w, r)
	if !ok {
		return nil, nil, false
	}
	if h.security == nil {
		cancel()
		writeError(w, http.StatusServiceUnavailable, "security_unavailable")
		return nil, nil, false
	}
	code := r.Header.Get("X-System-Monitor-MFA")
	if code == "" {
		cancel()
		writeError(w, http.StatusPreconditionRequired, "mfa_verification_required")
		return nil, nil, false
	}
	if err := h.security.VerifyMFA(ctx, accountIDFromContext(ctx), code); err != nil {
		cancel()
		h.writeSecurityError(r.Context(), w, err, "verify MFA")
		return nil, nil, false
	}
	return ctx, cancel, true
}

func (h *Handler) writeSecurityError(ctx context.Context, w http.ResponseWriter, err error, operation string) {
	switch {
	case errors.Is(err, ErrMFAAlreadyEnabled):
		writeError(w, http.StatusConflict, "mfa_already_enabled")
	case errors.Is(err, ErrMFANotEnabled):
		writeError(w, http.StatusPreconditionRequired, "mfa_enrollment_required")
	case errors.Is(err, ErrInvalidMFACode):
		writeError(w, http.StatusUnauthorized, "mfa_verification_failed")
	case errors.Is(err, ErrMFARateLimited):
		writeError(w, http.StatusTooManyRequests, "mfa_rate_limited")
	case errors.Is(err, ErrInvalidSecurityInput):
		writeError(w, http.StatusBadRequest, "invalid_security_action")
	default:
		h.logger.ErrorContext(ctx, operation, "error", err)
		writeError(w, http.StatusServiceUnavailable, "security_operation_unavailable")
	}
}

func parseLimit(r *http.Request, maximum int) (int, bool) {
	query := r.URL.Query()
	for key, values := range query {
		if key != "limit" || len(values) != 1 {
			return 0, false
		}
	}
	limit := 50
	if value := query.Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > maximum {
			return 0, false
		}
		limit = parsed
	}
	return limit, true
}

func onlyAllowedQuery(r *http.Request, allowed map[string]bool) bool {
	for key, values := range r.URL.Query() {
		if !allowed[key] || len(values) != 1 {
			return false
		}
	}
	return true
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
			continue
		}
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') &&
			(char < 'A' || char > 'F') {
			return false
		}
	}
	return true
}

func (h *Handler) writeControlError(ctx context.Context, w http.ResponseWriter, err error, operation string) {
	switch {
	case errors.Is(err, ErrInvalidEndpointControl):
		writeError(w, http.StatusBadRequest, "invalid_endpoint_control")
	case errors.Is(err, ErrEndpointNotControllable):
		writeError(w, http.StatusUnprocessableEntity, "endpoint_not_controllable")
	case errors.Is(err, ErrEndpointNotPaused):
		writeError(w, http.StatusConflict, "endpoint_not_paused")
	default:
		h.logger.ErrorContext(ctx, operation, "error", err)
		writeError(w, http.StatusServiceUnavailable, "endpoint_control_unavailable")
	}
}

func validActionRequest(w http.ResponseWriter, r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" ||
		r.Header.Get("X-System-Monitor-Action") != "confirm" {
		writeError(w, http.StatusUnsupportedMediaType, "json_and_action_header_required")
		return false
	}
	return true
}

func decodeActionPayload(w http.ResponseWriter, r *http.Request, payload any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request_body")
		return false
	}
	return true
}

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) (context.Context, context.CancelFunc, bool) {
	w.Header().Set("Cache-Control", "no-store")
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return nil, nil, false
	}
	if h.authenticator == nil {
		writeError(w, http.StatusServiceUnavailable, "identity_unavailable")
		return nil, nil, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	accountID, err := h.authenticator.Authenticate(ctx, cookie.Value)
	if err != nil {
		cancel()
		if errors.Is(err, ErrUnauthenticated) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
		} else {
			writeError(w, http.StatusServiceUnavailable, "identity_unavailable")
		}
		return nil, nil, false
	}
	allowed, err := HasAccess(ctx, h.db, accountID)
	if err != nil {
		h.logger.ErrorContext(ctx, "check System Monitor authorization", "error", err)
		cancel()
		writeError(w, http.StatusServiceUnavailable, "authorization_unavailable")
		return nil, nil, false
	}
	if !allowed {
		cancel()
		writeError(w, http.StatusForbidden, "permission_denied")
		return nil, nil, false
	}
	ctx = context.WithValue(ctx, accountIDContextKey{}, accountID)
	return ctx, cancel, true
}

func accountIDFromContext(ctx context.Context) string {
	accountID, _ := ctx.Value(accountIDContextKey{}).(string)
	return accountID
}

type accountIDContextKey struct{}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code}})
}
