package http

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	accessdomain "github.com/mothercat-enterprise/mothercat/backend/services/access/internal/domain"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type authorizationStore interface {
	GetUserAuthorization(context.Context, string) (accessdomain.Authorization, error)
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
			logger.WarnContext(ctx, "Access readiness check failed", "error", err)
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
		authorization, err := store.GetUserAuthorization(ctx, userID)
		if err != nil {
			logger.ErrorContext(ctx, "read user authorization", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		if authorization.Roles == nil {
			authorization.Roles = []string{}
		}
		if authorization.Permissions == nil {
			authorization.Permissions = []string{}
		}
		writeJSON(w, http.StatusOK, authorization)
	})
	return mux
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
