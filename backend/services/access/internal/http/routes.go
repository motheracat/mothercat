package http

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	accessdomain "github.com/mothercat-enterprise/mothercat/backend/services/access/internal/domain"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var requestIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{32}$`)
var authorizationRoutePattern = regexp.MustCompile(`^/internal/v1/users/[^/]+/authorization$`)

type requestIDContextKey struct{}

type authorizationStore interface {
	GetUserAuthorization(context.Context, string) (accessdomain.Authorization, error)
	AssignDefaultStaffRole(context.Context, string) error
	Ping(context.Context) error
}

func NewHandler(store authorizationStore, serviceToken string, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, "service_not_ready")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := store.Ping(ctx); err != nil {
			logger.WarnContext(ctx, "Access readiness check failed", "request_id", requestIDFromContext(r.Context()), "error", err)
			writeError(w, http.StatusServiceUnavailable, "service_not_ready")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /internal/v1/users/{userID}/authorization", func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if serviceToken == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(serviceToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		userID := strings.TrimSpace(r.PathValue("userID"))
		if !uuidPattern.MatchString(userID) {
			writeError(w, http.StatusBadRequest, "invalid_user_id")
			return
		}
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, "service_unavailable")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		started := time.Now()
		authorization, err := store.GetUserAuthorization(ctx, userID)
		if err != nil {
			logger.ErrorContext(ctx, "read user authorization",
				"request_id", requestIDFromContext(r.Context()),
				"stage", "access_authorization_database_lookup",
				"duration_ms", time.Since(started).Milliseconds(),
				"outcome", "error",
				"error", err,
			)
			writeError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		logger.InfoContext(ctx, "Access authorization database lookup",
			"request_id", requestIDFromContext(r.Context()),
			"stage", "access_authorization_database_lookup",
			"duration_ms", time.Since(started).Milliseconds(),
			"role_count", len(authorization.Roles),
			"permission_count", len(authorization.Permissions),
			"outcome", "success",
		)
		if authorization.Roles == nil {
			authorization.Roles = []string{}
		}
		if authorization.Permissions == nil {
			authorization.Permissions = []string{}
		}
		writeJSON(w, http.StatusOK, authorization)
	})
	mux.HandleFunc("PUT /internal/v1/users/{userID}/roles/staff", func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if serviceToken == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(serviceToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		userID := strings.TrimSpace(r.PathValue("userID"))
		if !uuidPattern.MatchString(userID) {
			writeError(w, http.StatusBadRequest, "invalid_user_id")
			return
		}
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, "service_unavailable")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := store.AssignDefaultStaffRole(ctx, userID); err != nil {
			logger.ErrorContext(ctx, "assign default Staff role",
				"request_id", requestIDFromContext(ctx),
				"error", err,
			)
			writeError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return requestLoggingMiddleware(logger, mux)
}

func requestLoggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if !requestIDPattern.MatchString(requestID) {
			var id [16]byte
			if _, err := rand.Read(id[:]); err != nil {
				logger.ErrorContext(r.Context(), "generate Access request ID", "error", err)
				requestID = "unavailable"
			} else {
				requestID = hex.EncodeToString(id[:])
			}
		}
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", requestID)
		writer := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		started := time.Now()
		next.ServeHTTP(writer, r)
		logger.InfoContext(ctx, "Access HTTP request",
			"request_id", requestID,
			"method", r.Method,
			"route", accessRouteName(r.URL.Path),
			"status", writer.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func accessRouteName(path string) string {
	switch path {
	case "/health/live":
		return "health_live"
	case "/health/ready":
		return "health_ready"
	}
	if authorizationRoutePattern.MatchString(path) {
		return "user_authorization"
	}
	if strings.HasPrefix(path, "/internal/v1/users/") && strings.HasSuffix(path, "/roles/staff") {
		return "assign_staff_role"
	}
	return "unmatched"
}

func requestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
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
	w.status = status
	w.wroteHeader = true
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

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
