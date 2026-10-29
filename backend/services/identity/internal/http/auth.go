package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/application"
	identitydomain "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/domain"
)

const sessionCookieName = "mothercat_session"

const (
	maxLoginBodyBytes     = 8 << 10
	maxLoginAttemptsPerIP = 5
	loginWindow           = 10 * time.Minute
	maxRateLimitBuckets   = 10000
)

type loginRateLimiter struct {
	mu         sync.Mutex
	buckets    map[string]loginBucket
	limit      int
	window     time.Duration
	operations uint64
}

type loginBucket struct {
	count    int
	inFlight int
	resetAt  time.Time
}

func newLoginRateLimiter(limit int, window time.Duration) *loginRateLimiter {
	if limit <= 0 {
		limit = maxLoginAttemptsPerIP
	}
	if window <= 0 {
		window = loginWindow
	}
	return &loginRateLimiter{buckets: make(map[string]loginBucket), limit: limit, window: window}
}

func (l *loginRateLimiter) allow(key string) bool {
	return l.allowKeys(key)
}

func (l *loginRateLimiter) allowKeys(keys ...string) bool {
	if l == nil || len(keys) == 0 {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.operations++
	if l.operations%128 == 0 {
		l.pruneExpired(now)
	}
	missing := 0
	for _, key := range keys {
		if key == "" {
			continue
		}
		entry, exists := l.buckets[key]
		if exists && now.Before(entry.resetAt) && entry.count+entry.inFlight >= l.limit {
			return false
		}
		if !exists || !now.Before(entry.resetAt) {
			missing++
		}
	}
	if len(l.buckets)+missing > maxRateLimitBuckets {
		return false
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		entry, exists := l.buckets[key]
		if !exists || !now.Before(entry.resetAt) {
			entry = loginBucket{resetAt: now.Add(l.window)}
		}
		entry.inFlight++
		l.buckets[key] = entry
	}
	return true
}

func (l *loginRateLimiter) recordFailure(keys ...string) {
	if l == nil || len(keys) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	for _, key := range keys {
		if key == "" {
			continue
		}
		entry, exists := l.buckets[key]
		if !exists || !now.Before(entry.resetAt) {
			if len(l.buckets) >= maxRateLimitBuckets {
				continue
			}
			l.buckets[key] = loginBucket{count: 1, resetAt: now.Add(l.window)}
			continue
		}
		if entry.inFlight > 0 {
			entry.inFlight--
		}
		entry.count++
		l.buckets[key] = entry
	}
}

func (l *loginRateLimiter) release(keys ...string) {
	if l == nil || len(keys) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, key := range keys {
		entry, exists := l.buckets[key]
		if exists && entry.inFlight > 0 {
			entry.inFlight--
			l.buckets[key] = entry
		}
	}
}

func (l *loginRateLimiter) recordSuccess(keys []string, accountKey string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, key := range keys {
		entry, exists := l.buckets[key]
		if !exists {
			continue
		}
		if entry.inFlight > 0 {
			entry.inFlight--
		}
		if key == accountKey {
			entry.count = 0
		}
		l.buckets[key] = entry
	}
}

func (l *loginRateLimiter) pruneExpired(now time.Time) {
	for key, entry := range l.buckets {
		if !now.Before(entry.resetAt) {
			delete(l.buckets, key)
		}
	}
}

// AuthService defines the contract for Identity login and session workflows.
type AuthService interface {
	Login(context.Context, string, string) (identitydomain.Account, string, error)
	ResolveSession(context.Context, string) (identitydomain.Account, error)
	Logout(context.Context, string) error
}

// NewAuthHandler registers the authenticated Identity API routes.
func NewAuthHandler(service AuthService, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	limiter := newLoginRateLimiter(maxLoginAttemptsPerIP, loginWindow)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if err := handleLogin(w, r, service, logger, limiter); err != nil {
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

func handleLogin(w http.ResponseWriter, r *http.Request, service AuthService, logger *slog.Logger, limiter *loginRateLimiter) error {
	if service == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "identity service unavailable", requestIDFromContext(r.Context()))
		return errors.New("auth service is nil")
	}

	mediaType, _, mediaTypeErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaTypeErr != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "invalid_request", "content-type must be application/json", requestIDFromContext(r.Context()))
		return fmt.Errorf("unexpected content type")
	}

	var payload struct {
		StaffID  string `json:"staff_id"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLoginBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body must contain staff_id and password", requestIDFromContext(r.Context()))
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body must contain exactly one JSON object", requestIDFromContext(r.Context()))
		return fmt.Errorf("unexpected trailing login request data")
	}
	payload.StaffID = strings.TrimSpace(payload.StaffID)
	if payload.StaffID == "" || payload.Password == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "staff_id and password are required", requestIDFromContext(r.Context()))
		return fmt.Errorf("missing staff_id or password")
	}

	clientKey, accountKey := rateLimitKeys(r, payload.StaffID)
	if !limiter.allowKeys(clientKey, accountKey) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many failed login attempts; please wait a little while before retrying", requestIDFromContext(r.Context()))
		return errors.New("login rate limited")
	}

	account, sessionID, err := service.Login(r.Context(), payload.StaffID, payload.Password)
	if err != nil {
		if errors.Is(err, identitydomain.ErrInvalidCredentials) {
			limiter.recordFailure(clientKey, accountKey)
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid staff credentials", requestIDFromContext(r.Context()))
			return err
		}
		limiter.release(clientKey, accountKey)
		logger.ErrorContext(r.Context(), "login service failure", "request_id", requestIDFromContext(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred", requestIDFromContext(r.Context()))
		return err
	}
	if sessionID == "" {
		limiter.release(clientKey, accountKey)
		writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred", requestIDFromContext(r.Context()))
		return errors.New("identity service returned an empty session token")
	}
	limiter.recordSuccess([]string{clientKey, accountKey}, accountKey)
	cookie := buildSessionCookie(sessionID, application.SessionTTL)
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
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "logout could not be completed; please retry", requestIDFromContext(r.Context()))
		return err
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
		Secure:   true,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func buildSessionCookie(value string, ttl time.Duration) *http.Cookie {
	if ttl <= 0 {
		ttl = application.SessionTTL
	}
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
		MaxAge:   int(ttl / time.Second),
		Expires:  time.Now().Add(ttl),
	}
}

func rateLimitKeys(r *http.Request, staffID string) (string, string) {
	clientIP := "unknown"
	if r != nil {
		remoteHost, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
		if err == nil {
			if ip := net.ParseIP(remoteHost); ip != nil {
				clientIP = ip.String()
			}
		} else if ip := net.ParseIP(strings.TrimSpace(r.RemoteAddr)); ip != nil {
			clientIP = ip.String()
		}
	}
	return "ip:" + clientIP, "account:" + strings.ToUpper(strings.TrimSpace(staffID))
}
