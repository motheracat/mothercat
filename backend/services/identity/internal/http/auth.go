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
	"strings"
	"time"

	identitydomain "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/domain"
)

const sessionCookieName = "mothercat_session"

// AuthService defines the contract for Identity login and session workflows.
type AuthService interface {
	Login(context.Context, string, string) (identitydomain.Account, error)
	ResolveSession(context.Context, string) (identitydomain.Account, error)
	Logout(context.Context, string) error
}

// NewAuthHandler registers the authenticated Identity API routes.
func NewAuthHandler(service AuthService, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if err := handleLogin(w, r, service, logger); err != nil {
			logger.ErrorContext(r.Context(), "login failed", "request_id", requestIDFromContext(r.Context()), "error", err)
		}
	})
	mux.HandleFunc("GET /api/v1/auth/session", func(w http.ResponseWriter, r *http.Request) {
		if err := handleSession(w, r, service, logger); err != nil {
			logger.WarnContext(r.Context(), "session lookup failed", "request_id", requestIDFromContext(r.Context()), "error", err)
		}
	})
	mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if err := handleLogout(w, r, service, logger); err != nil {
			logger.WarnContext(r.Context(), "logout failed", "request_id", requestIDFromContext(r.Context()), "error", err)
		}
	})

	return requestIDMiddleware(logger, requestLoggingMiddleware(logger, recoveryMiddleware(logger, mux)))
}

func handleLogin(w http.ResponseWriter, r *http.Request, service AuthService, logger *slog.Logger) error {
	if service == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "identity service unavailable", requestIDFromContext(r.Context()))
		return errors.New("auth service is nil")
	}

	var payload struct {
		StaffID string `json:"staff_id"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body must contain staff_id and password", requestIDFromContext(r.Context()))
		return err
	}
	payload.StaffID = strings.TrimSpace(payload.StaffID)
	payload.Password = strings.TrimSpace(payload.Password)
	if payload.StaffID == "" || payload.Password == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "staff_id and password are required", requestIDFromContext(r.Context()))
		return fmt.Errorf("missing staff_id or password")
	}

	account, err := service.Login(r.Context(), payload.StaffID, payload.Password)
	if err != nil {
		if errors.Is(err, identitydomain.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid staff credentials", requestIDFromContext(r.Context()))
			return err
		}
		logger.ErrorContext(r.Context(), "login service failure", "request_id", requestIDFromContext(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred", requestIDFromContext(r.Context()))
		return err
	}

	sessionID := "sess_" + randomToken(24)
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		MaxAge:   3600,
		Expires:  time.Now().Add(time.Hour),
	}
	http.SetCookie(w, cookie)
	writeJSON(w, http.StatusOK, map[string]any{"user": account})
	return nil
}

func handleSession(w http.ResponseWriter, r *http.Request, service AuthService, logger *slog.Logger) error {
	if service == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "identity service unavailable", requestIDFromContext(r.Context()))
		return errors.New("auth service is nil")
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required", requestIDFromContext(r.Context()))
		return err
	}

	account, err := service.ResolveSession(r.Context(), cookie.Value)
	if err != nil {
		if errors.Is(err, identitydomain.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid_session", "session expired or invalid", requestIDFromContext(r.Context()))
			return err
		}
		logger.ErrorContext(r.Context(), "session resolution failed", "request_id", requestIDFromContext(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred", requestIDFromContext(r.Context()))
		return err
	}

	writeJSON(w, http.StatusOK, map[string]any{"user": account})
	return nil
}

func handleLogout(w http.ResponseWriter, r *http.Request, service AuthService, logger *slog.Logger) error {
	if service == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "identity service unavailable", requestIDFromContext(r.Context()))
		return errors.New("auth service is nil")
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		clearSessionCookie(w)
		writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
		return nil
	}

	if err := service.Logout(r.Context(), cookie.Value); err != nil {
		logger.WarnContext(r.Context(), "logout service failed", "request_id", requestIDFromContext(r.Context()), "error", err)
	}
	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
	return nil
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func isSecureRequest(r *http.Request) bool {
	return r != nil && (r.TLS != nil || strings.HasPrefix(r.URL.Scheme, "https"))
}

func randomToken(length int) string {
	count := length / 2
	if count < 1 {
		count = 1
	}
	buf := make([]byte, count)
	if _, err := rand.Read(buf); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(buf)
}
