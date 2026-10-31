package http

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/application"
	identitydomain "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/domain"
)

const sessionCookieName = "mothercat_session"

var accountIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

const (
	maxLoginBodyBytes        = 8 << 10
	maxLoginAttemptsPerIP    = 5
	loginWindow              = 10 * time.Minute
	maxRecoveryAttemptsPerIP = 10
	recoveryWindow           = 10 * time.Minute
	maxRateLimitBuckets      = 10000
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
	RequestPasswordReset(context.Context, string) error
	CompletePasswordReset(context.Context, string, string) error
	CompleteActivation(context.Context, string, string) error
	ChangePassword(context.Context, string, string, string) error
	CreateActivationToken(context.Context, string) error
	ProvisionInactiveAccount(context.Context, string, string, string) (string, error)
	BootstrapSuperAdmin(context.Context, string, string, string, string) (string, error)
}

type AuthHandlerOptions struct {
	OnboardingServiceToken string
	BootstrapToken         string
}

// NewAuthHandler registers the authenticated Identity API routes.
func NewAuthHandler(service AuthService, logger *slog.Logger, configs ...AuthHandlerOptions) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	limiter := newLoginRateLimiter(maxLoginAttemptsPerIP, loginWindow)
	recoveryLimiter := newLoginRateLimiter(maxRecoveryAttemptsPerIP, recoveryWindow)
	config := AuthHandlerOptions{}
	if len(configs) > 0 {
		config = configs[0]
	}
	mux := http.NewServeMux()
	bootstrapLimiter := newLoginRateLimiter(maxLoginAttemptsPerIP, loginWindow)
	mux.HandleFunc("POST /api/v1/auth/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		if err := handleBootstrap(w, r, service, logger, config.BootstrapToken, bootstrapLimiter); err != nil {
			logger.WarnContext(r.Context(), "administrator bootstrap request failed",
				"request_id", requestIDFromContext(r.Context()), "error", err)
		}
	})
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
	mux.HandleFunc("POST /api/v1/auth/password/forgot", func(w http.ResponseWriter, r *http.Request) {
		if !allowWorkflowRequest(w, r, recoveryLimiter) {
			return
		}
		if err := handlePasswordForgot(w, r, service, logger); err != nil {
			logger.ErrorContext(r.Context(), "password recovery request failed", "request_id", requestIDFromContext(r.Context()), "error", err)
		}
	})
	mux.HandleFunc("POST /api/v1/auth/password/reset", func(w http.ResponseWriter, r *http.Request) {
		if !allowWorkflowRequest(w, r, recoveryLimiter) {
			return
		}
		if err := handlePasswordReset(w, r, service, logger); err != nil {
			logger.WarnContext(r.Context(), "password reset failed", "request_id", requestIDFromContext(r.Context()), "error", err)
		}
	})
	mux.HandleFunc("POST /api/v1/auth/password/change", func(w http.ResponseWriter, r *http.Request) {
		if err := handlePasswordChange(w, r, service, logger); err != nil {
			logger.WarnContext(r.Context(), "password change failed", "request_id", requestIDFromContext(r.Context()), "error", err)
		}
	})
	mux.HandleFunc("POST /api/v1/auth/activation/complete", func(w http.ResponseWriter, r *http.Request) {
		if !allowWorkflowRequest(w, r, recoveryLimiter) {
			return
		}
		if err := handleActivationComplete(w, r, service, logger); err != nil {
			logger.WarnContext(r.Context(), "account activation failed", "request_id", requestIDFromContext(r.Context()), "error", err)
		}
	})
	mux.HandleFunc("POST /internal/v1/accounts/{accountID}/activation", func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if config.OnboardingServiceToken == "" ||
			subtle.ConstantTimeCompare([]byte(provided), []byte(config.OnboardingServiceToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized", "service authentication required", requestIDFromContext(r.Context()))
			return
		}
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "service_unavailable", "identity service unavailable", requestIDFromContext(r.Context()))
			return
		}
		accountID := strings.TrimSpace(r.PathValue("accountID"))
		if !accountIDPattern.MatchString(accountID) {
			writeError(w, http.StatusBadRequest, "invalid_account_id", "account ID is invalid", requestIDFromContext(r.Context()))
			return
		}
		if err := service.CreateActivationToken(r.Context(), accountID); err != nil {
			logger.ErrorContext(r.Context(), "create account activation", "request_id", requestIDFromContext(r.Context()), "error", err)
			if errors.Is(err, application.ErrEmailDeliveryUnavailable) {
				writeError(w, http.StatusServiceUnavailable, "email_delivery_disabled", "account activation email is not configured", requestIDFromContext(r.Context()))
				return
			}
			writeError(w, http.StatusServiceUnavailable, "activation_unavailable", "account activation could not be initiated", requestIDFromContext(r.Context()))
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("POST /internal/v1/accounts", func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if config.OnboardingServiceToken == "" ||
			subtle.ConstantTimeCompare([]byte(provided), []byte(config.OnboardingServiceToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized", "service authentication required", requestIDFromContext(r.Context()))
			return
		}
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "service_unavailable", "identity service unavailable", requestIDFromContext(r.Context()))
			return
		}
		var payload struct {
			StaffID     string `json:"staff_id"`
			Email       string `json:"email"`
			DisplayName string `json:"display_name"`
		}
		if err := decodeJSON(w, r, &payload); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "valid staff_id, email, and display_name are required", requestIDFromContext(r.Context()))
			return
		}
		accountID, err := service.ProvisionInactiveAccount(r.Context(), payload.StaffID, payload.Email, payload.DisplayName)
		if errors.Is(err, application.ErrInvalidProvisioningDetails) {
			writeError(w, http.StatusBadRequest, "invalid_account", "valid staff_id, email, and display_name are required", requestIDFromContext(r.Context()))
			return
		}
		if errors.Is(err, application.ErrAccountConflict) {
			writeError(w, http.StatusConflict, "account_conflict", "account already exists with different details or is active", requestIDFromContext(r.Context()))
			return
		}
		if err != nil {
			logger.ErrorContext(r.Context(), "provision inactive account", "request_id", requestIDFromContext(r.Context()), "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "account provisioning failed", requestIDFromContext(r.Context()))
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"account_id": accountID})
	})

	return requestIDMiddleware(logger, requestLoggingMiddleware(logger, recoveryMiddleware(logger, mux)))
}

func handleBootstrap(
	w http.ResponseWriter,
	r *http.Request,
	service AuthService,
	logger *slog.Logger,
	bootstrapToken string,
	limiter *loginRateLimiter,
) error {
	requestID := requestIDFromContext(r.Context())
	if bootstrapToken == "" {
		writeError(w, http.StatusNotFound, "not_found", "endpoint not found", requestID)
		return nil
	}
	clientKey, _ := rateLimitKeys(r, "")
	if !limiter.allowKeys(clientKey) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many requests; please wait before retrying", requestID)
		return nil
	}
	authorization := strings.Fields(r.Header.Get("Authorization"))
	providedToken := ""
	if len(authorization) == 2 && strings.EqualFold(authorization[0], "Bearer") {
		providedToken = authorization[1]
	}
	if subtle.ConstantTimeCompare([]byte(providedToken), []byte(bootstrapToken)) != 1 {
		limiter.recordFailure(clientKey)
		writeError(w, http.StatusUnauthorized, "unauthorized", "valid bootstrap credentials are required", requestID)
		return errors.New("invalid administrator bootstrap credentials")
	}
	limiter.recordSuccess([]string{clientKey}, clientKey)
	if service == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "identity service unavailable", requestID)
		return errors.New("auth service is nil")
	}
	var payload struct {
		StaffID     string `json:"staff_id"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "valid staff_id, email, display_name, and password are required", requestID)
		return err
	}
	accountID, err := service.BootstrapSuperAdmin(
		r.Context(), payload.StaffID, payload.Email, payload.DisplayName, payload.Password,
	)
	if errors.Is(err, application.ErrBootstrapClosed) {
		writeError(w, http.StatusConflict, "bootstrap_closed", "administrator bootstrap has already been completed", requestID)
		return err
	}
	if errors.Is(err, application.ErrInvalidProvisioningDetails) {
		writeError(w, http.StatusBadRequest, "invalid_account", "valid staff_id, email, and display_name are required", requestID)
		return err
	}
	if err != nil && strings.Contains(err.Error(), "password must") {
		writeError(w, http.StatusBadRequest, "weak_password", err.Error(), requestID)
		return err
	}
	if err != nil {
		logger.ErrorContext(r.Context(), "create initial administrator",
			"request_id", requestID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "administrator bootstrap failed", requestID)
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]string{"account_id": accountID})
	return nil
}

func allowWorkflowRequest(w http.ResponseWriter, r *http.Request, limiter *loginRateLimiter) bool {
	clientKey, _ := rateLimitKeys(r, "")
	if !limiter.allowKeys(clientKey) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many requests; please wait before retrying", requestIDFromContext(r.Context()))
		return false
	}
	limiter.recordFailure(clientKey)
	return true
}

func handlePasswordForgot(w http.ResponseWriter, r *http.Request, service AuthService, logger *slog.Logger) error {
	var payload struct {
		StaffID string `json:"staff_id"`
	}
	if err := decodeJSON(w, r, &payload); err != nil || strings.TrimSpace(payload.StaffID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "staff_id is required", requestIDFromContext(r.Context()))
		return errors.New("invalid password recovery request")
	}
	if service == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "identity service unavailable", requestIDFromContext(r.Context()))
		return errors.New("auth service is nil")
	}
	if err := service.RequestPasswordReset(r.Context(), strings.TrimSpace(payload.StaffID)); err != nil {
		if errors.Is(err, application.ErrEmailDeliveryUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "email_delivery_disabled", "password recovery email is not configured", requestIDFromContext(r.Context()))
			return err
		}
		logger.ErrorContext(r.Context(), "deliver password recovery", "request_id", requestIDFromContext(r.Context()), "error", err)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "accepted",
		"message": "If the account is eligible, recovery instructions will be sent.",
	})
	return nil
}

func handlePasswordReset(w http.ResponseWriter, r *http.Request, service AuthService, logger *slog.Logger) error {
	var payload struct {
		Token           string `json:"token"`
		Password        string `json:"password"`
		ConfirmPassword string `json:"confirm_password"`
	}
	if err := decodeJSON(w, r, &payload); err != nil || payload.Token == "" ||
		payload.Password == "" || payload.Password != payload.ConfirmPassword {
		writeError(w, http.StatusBadRequest, "invalid_request", "token and matching passwords are required", requestIDFromContext(r.Context()))
		return errors.New("invalid password reset payload")
	}
	if service == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "identity service unavailable", requestIDFromContext(r.Context()))
		return errors.New("auth service is nil")
	}
	if err := service.CompletePasswordReset(r.Context(), payload.Token, payload.Password); err != nil {
		return writeCredentialWorkflowError(w, r, logger, err, "password reset")
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "password_reset"})
	return nil
}

func handleActivationComplete(w http.ResponseWriter, r *http.Request, service AuthService, logger *slog.Logger) error {
	var payload struct {
		Token           string `json:"token"`
		Password        string `json:"password"`
		ConfirmPassword string `json:"confirm_password"`
	}
	if err := decodeJSON(w, r, &payload); err != nil || payload.Token == "" ||
		payload.Password == "" || payload.Password != payload.ConfirmPassword {
		writeError(w, http.StatusBadRequest, "invalid_request", "token and matching passwords are required", requestIDFromContext(r.Context()))
		return errors.New("invalid activation payload")
	}
	if service == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "identity service unavailable", requestIDFromContext(r.Context()))
		return errors.New("auth service is nil")
	}
	if err := service.CompleteActivation(r.Context(), payload.Token, payload.Password); err != nil {
		return writeCredentialWorkflowError(w, r, logger, err, "account activation")
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "activated"})
	return nil
}

func handlePasswordChange(w http.ResponseWriter, r *http.Request, service AuthService, logger *slog.Logger) error {
	var payload struct {
		CurrentPassword string `json:"current_password"`
		Password        string `json:"password"`
		ConfirmPassword string `json:"confirm_password"`
	}
	if err := decodeJSON(w, r, &payload); err != nil || payload.CurrentPassword == "" ||
		payload.Password == "" || payload.Password != payload.ConfirmPassword {
		writeError(w, http.StatusBadRequest, "invalid_request", "current password and matching new passwords are required", requestIDFromContext(r.Context()))
		return errors.New("invalid password change payload")
	}
	if service == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "identity service unavailable", requestIDFromContext(r.Context()))
		return errors.New("auth service is nil")
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required", requestIDFromContext(r.Context()))
		return errors.New("password change requires a session")
	}
	if err := service.ChangePassword(r.Context(), cookie.Value, payload.CurrentPassword, payload.Password); err != nil {
		if errors.Is(err, application.ErrInvalidCurrentPassword) || errors.Is(err, identitydomain.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "current password or session is invalid", requestIDFromContext(r.Context()))
			return err
		}
		if strings.Contains(err.Error(), "password must") {
			writeError(w, http.StatusBadRequest, "weak_password", err.Error(), requestIDFromContext(r.Context()))
			return err
		}
		logger.ErrorContext(r.Context(), "change password", "request_id", requestIDFromContext(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred", requestIDFromContext(r.Context()))
		return err
	}
	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "password_changed", "message": "Sign in again with your new password."})
	return nil
}

func writeCredentialWorkflowError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error, operation string) error {
	if errors.Is(err, application.ErrInvalidRecoveryToken) {
		writeError(w, http.StatusBadRequest, "invalid_token", "token is invalid, expired, or already used", requestIDFromContext(r.Context()))
		return err
	}
	if strings.Contains(err.Error(), "password must") {
		writeError(w, http.StatusBadRequest, "weak_password", err.Error(), requestIDFromContext(r.Context()))
		return err
	}
	logger.ErrorContext(r.Context(), operation+" failed", "request_id", requestIDFromContext(r.Context()), "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred", requestIDFromContext(r.Context()))
	return err
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	mediaType, _, mediaTypeErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaTypeErr != nil || mediaType != "application/json" {
		return errors.New("content-type must be application/json")
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLoginBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request must contain exactly one JSON object")
	}
	return nil
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
