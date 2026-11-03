package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/mothercat-enterprise/mothercat/backend/services/access/internal/domain"
)

const (
	sessionCookieName = "mothercat_session"
	maxRequestBody    = 16 << 10
)

var (
	roleIDPattern        = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	permissionKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,149}$`)
	uuidPattern          = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

type Principal struct {
	ID          string
	Permissions []string
}

var ErrUnauthenticated = errors.New("unauthenticated")

type Authenticator interface {
	Authenticate(context.Context, string) (Principal, error)
}

type store interface {
	ListRoles(context.Context) ([]domain.Role, error)
	ListPermissions(context.Context) ([]domain.Permission, error)
	CreateRole(context.Context, string, domain.Role) (domain.Role, error)
	UpdateRole(context.Context, string, string, *string, *string) (domain.Role, error)
	ReplaceRolePermissions(context.Context, string, string, []string) ([]string, error)
	GetUserRoles(context.Context, string) ([]string, error)
	ReplaceUserRoles(context.Context, string, string, []string) ([]string, error)
}

type Handler struct {
	store         store
	authenticator Authenticator
	logger        *slog.Logger
}

func NewHandler(database store, authenticator Authenticator, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	handler := &Handler{store: database, authenticator: authenticator, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/roles", handler.listRoles)
	mux.HandleFunc("POST /api/v1/roles", handler.createRole)
	mux.HandleFunc("PATCH /api/v1/roles/{roleID}", handler.updateRole)
	mux.HandleFunc("GET /api/v1/permissions", handler.listPermissions)
	mux.HandleFunc("PUT /api/v1/roles/{roleID}/permissions", handler.replaceRolePermissions)
	mux.HandleFunc("GET /api/v1/users/{userID}/roles", handler.getUserRoles)
	mux.HandleFunc("PUT /api/v1/users/{userID}/roles", handler.replaceUserRoles)
	return handler.requestLoggingMiddleware(mux)
}

type roleCreateRequest struct {
	RoleID      string `json:"role_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type roleUpdateRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type permissionSetRequest struct {
	PermissionKeys []string `json:"permission_keys"`
}

type userRoleSetRequest struct {
	RoleIDs []string `json:"role_ids"`
}

func (h *Handler) listRoles(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, _, ok := h.authorize(w, r, "roles.read", false)
	if !ok {
		return
	}
	defer cancel()
	roles, err := h.store.ListRoles(ctx)
	if err != nil {
		h.fail(w, ctx, "list roles", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
}

func (h *Handler) listPermissions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, _, ok := h.authorize(w, r, "roles.read", false)
	if !ok {
		return
	}
	defer cancel()
	permissions, err := h.store.ListPermissions(ctx)
	if err != nil {
		h.fail(w, ctx, "list permissions", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"permissions": permissions})
}

func (h *Handler) createRole(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, principal, ok := h.authorize(w, r, "roles.manage", true)
	if !ok {
		return
	}
	defer cancel()
	var request roleCreateRequest
	if !decodeJSON(w, r, &request) {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID(ctx))
		return
	}
	request.RoleID = strings.TrimSpace(request.RoleID)
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	if !roleIDPattern.MatchString(request.RoleID) || request.Name == "" || len(request.Name) > 100 ||
		len(request.Description) > 2000 {
		writeError(w, http.StatusBadRequest, "invalid_role", requestID(ctx))
		return
	}
	role, err := h.store.CreateRole(ctx, principal.ID, domain.Role{
		ID: request.RoleID, Name: request.Name, Description: request.Description,
	})
	if err != nil {
		h.fail(w, ctx, "create role", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"role": role})
}

func (h *Handler) updateRole(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, principal, ok := h.authorize(w, r, "roles.manage", true)
	if !ok {
		return
	}
	defer cancel()
	roleID := r.PathValue("roleID")
	if !roleIDPattern.MatchString(roleID) {
		writeError(w, http.StatusBadRequest, "invalid_role_id", requestID(ctx))
		return
	}
	var request roleUpdateRequest
	if !decodeJSON(w, r, &request) || (request.Name == nil && request.Description == nil) {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID(ctx))
		return
	}
	if request.Name != nil {
		trimmed := strings.TrimSpace(*request.Name)
		if trimmed == "" || len(trimmed) > 100 {
			writeError(w, http.StatusBadRequest, "invalid_role", requestID(ctx))
			return
		}
		request.Name = &trimmed
	}
	if request.Description != nil {
		trimmed := strings.TrimSpace(*request.Description)
		if len(trimmed) > 2000 {
			writeError(w, http.StatusBadRequest, "invalid_role", requestID(ctx))
			return
		}
		request.Description = &trimmed
	}
	role, err := h.store.UpdateRole(ctx, principal.ID, roleID, request.Name, request.Description)
	if err != nil {
		h.fail(w, ctx, "update role", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"role": role})
}

func (h *Handler) replaceRolePermissions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, principal, ok := h.authorize(w, r, "roles.manage", true)
	if !ok {
		return
	}
	defer cancel()
	roleID := r.PathValue("roleID")
	if !roleIDPattern.MatchString(roleID) {
		writeError(w, http.StatusBadRequest, "invalid_role_id", requestID(ctx))
		return
	}
	var request permissionSetRequest
	if !decodeJSON(w, r, &request) || request.PermissionKeys == nil || len(request.PermissionKeys) > 200 {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID(ctx))
		return
	}
	for _, permission := range request.PermissionKeys {
		if !permissionKeyPattern.MatchString(permission) {
			writeError(w, http.StatusBadRequest, "invalid_permission_set", requestID(ctx))
			return
		}
	}
	if hasDuplicates(request.PermissionKeys) {
		writeError(w, http.StatusBadRequest, "invalid_permission_set", requestID(ctx))
		return
	}
	permissions, err := h.store.ReplaceRolePermissions(ctx, principal.ID, roleID, request.PermissionKeys)
	if err != nil {
		h.fail(w, ctx, "replace role permissions", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"role_id": roleID, "permission_keys": permissions,
	})
}

func (h *Handler) getUserRoles(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, _, ok := h.authorize(w, r, "roles.read", false)
	if !ok {
		return
	}
	defer cancel()
	userID := r.PathValue("userID")
	if !uuidPattern.MatchString(userID) {
		writeError(w, http.StatusBadRequest, "invalid_user_id", requestID(ctx))
		return
	}
	roles, err := h.store.GetUserRoles(ctx, userID)
	if err != nil {
		h.fail(w, ctx, "get user roles", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": userID, "role_ids": roles})
}

func (h *Handler) replaceUserRoles(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, principal, ok := h.authorize(w, r, "user_roles.assign", true)
	if !ok {
		return
	}
	defer cancel()
	userID := r.PathValue("userID")
	if !uuidPattern.MatchString(userID) {
		writeError(w, http.StatusBadRequest, "invalid_user_id", requestID(ctx))
		return
	}
	var request userRoleSetRequest
	if !decodeJSON(w, r, &request) || request.RoleIDs == nil || len(request.RoleIDs) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_request", requestID(ctx))
		return
	}
	for _, roleID := range request.RoleIDs {
		if !roleIDPattern.MatchString(roleID) {
			writeError(w, http.StatusBadRequest, "invalid_role_set", requestID(ctx))
			return
		}
	}
	if hasDuplicates(request.RoleIDs) {
		writeError(w, http.StatusBadRequest, "invalid_role_set", requestID(ctx))
		return
	}
	roles, err := h.store.ReplaceUserRoles(ctx, principal.ID, userID, request.RoleIDs)
	if err != nil {
		h.fail(w, ctx, "replace user roles", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": userID, "role_ids": roles})
}

func (h *Handler) authorize(
	w http.ResponseWriter,
	r *http.Request,
	required string,
	requireOrigin bool,
) (context.Context, context.CancelFunc, Principal, bool) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", requestID(r.Context()))
		return nil, nil, Principal{}, false
	}
	if requireOrigin && !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "invalid_origin", requestID(r.Context()))
		return nil, nil, Principal{}, false
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", requestID(r.Context()))
		return nil, nil, Principal{}, false
	}
	if h.authenticator == nil {
		writeError(w, http.StatusServiceUnavailable, "identity_unavailable", requestID(r.Context()))
		return nil, nil, Principal{}, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	ctx = context.WithValue(ctx, requestIDContextKey{}, requestID(r.Context()))
	principal, err := h.authenticator.Authenticate(ctx, cookie.Value)
	if err != nil {
		cancel()
		if errors.Is(err, ErrUnauthenticated) {
			writeError(w, http.StatusUnauthorized, "unauthorized", requestID(r.Context()))
		} else {
			h.logger.ErrorContext(ctx, "authenticate Access request",
				"request_id", requestID(ctx), "error", err)
			writeError(w, http.StatusServiceUnavailable, "identity_unavailable", requestID(r.Context()))
		}
		return nil, nil, Principal{}, false
	}
	if !hasPermission(principal.Permissions, required) {
		cancel()
		writeError(w, http.StatusForbidden, "permission_denied", requestID(r.Context()))
		return nil, nil, Principal{}, false
	}
	return ctx, cancel, principal, true
}

func (h *Handler) fail(w http.ResponseWriter, ctx context.Context, operation string, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", requestID(ctx))
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", requestID(ctx))
	case errors.Is(err, domain.ErrSystemRole):
		writeError(w, http.StatusConflict, "system_role_immutable", requestID(ctx))
	case errors.Is(err, domain.ErrPermissionDenied):
		writeError(w, http.StatusForbidden, "permission_denied", requestID(ctx))
	case errors.Is(err, domain.ErrLastActiveSuperAdmin):
		writeError(w, http.StatusConflict, "last_active_superadmin", requestID(ctx))
	case errors.Is(err, domain.ErrInvalidPermissionSet):
		writeError(w, http.StatusBadRequest, "invalid_permission_set", requestID(ctx))
	default:
		h.logger.ErrorContext(ctx, operation, "request_id", requestID(ctx), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", requestID(ctx))
	}
}

func (h *Handler) requestLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			h.logger.ErrorContext(r.Context(), "generate Access request ID", "error", err)
			http.Error(w, "request ID unavailable", http.StatusInternalServerError)
			return
		}
		requestID := hex.EncodeToString(id[:])
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", requestID)
		writer := &statusWriter{ResponseWriter: w}
		started := time.Now()
		next.ServeHTTP(writer, r)
		h.logger.InfoContext(ctx, "Access HTTP request",
			"request_id", requestID, "method", r.Method, "route", r.Pattern,
			"status", writer.status, "duration_ms", time.Since(started).Milliseconds())
	})
}

type requestIDContextKey struct{}

func requestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return false
	}
	var extra any
	return errors.Is(decoder.Decode(&extra), io.EOF)
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

func hasPermission(permissions []string, required string) bool {
	for _, permission := range permissions {
		if permission == required {
			return true
		}
	}
	return false
}

func hasDuplicates(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
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

func writeError(w http.ResponseWriter, status int, code, requestID string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{
		"code": code, "request_id": requestID,
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
