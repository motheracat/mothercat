package applicantidentity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	applicantSessionCookie = "mothercat_applicant_session"
	applicantSessionTTL    = time.Hour
	verificationTokenTTL   = 24 * time.Hour
	maxRequestBody         = 8 << 10
)

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type Mailer interface {
	SendRecovery(context.Context, string, string, string) error
}

type Handler struct {
	db      *sql.DB
	mailer  Mailer
	logger  *slog.Logger
	limiter *rateLimiter
}

func NewHandler(db *sql.DB, mailer Mailer, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{db: db, mailer: mailer, logger: logger, limiter: newRateLimiter()}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/applicant/register", h.register)
	mux.HandleFunc("POST /api/v1/auth/applicant/verify-email", h.verifyEmail)
	mux.HandleFunc("POST /api/v1/auth/applicant/resend-verification", h.resendVerification)
	mux.HandleFunc("POST /api/v1/auth/applicant/login", h.login)
	return requestIDMiddleware(logger, mux)
}

type registrationRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, 5) {
		return
	}
	var input registrationRequest
	if !decodeJSON(w, r, &input) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	email, ok := normalizeEmail(input.Email)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if !ok || input.DisplayName == "" || utf8.RuneCountInString(input.DisplayName) > 160 ||
		validatePassword(input.Password) != nil {
		writeError(w, http.StatusBadRequest, "invalid_registration")
		return
	}
	if !h.allowAccount(w, email, 5) {
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	if h.mailer == nil {
		writeError(w, http.StatusServiceUnavailable, "email_delivery_disabled")
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		h.fail(w, r, "hash applicant password", err)
		return
	}
	token, err := newToken()
	if err != nil {
		h.fail(w, r, "generate applicant verification token", err)
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		h.fail(w, r, "begin applicant registration", err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var accountID string
	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO identity.applicant_accounts (email, display_name, password_hash)
		VALUES ($1, $2, $3)
		ON CONFLICT (email) DO NOTHING
		RETURNING id::text
	`, email, input.DisplayName, string(passwordHash)).Scan(&accountID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusAccepted, genericVerificationResponse())
		return
	}
	if err != nil {
		h.fail(w, r, "create applicant account", err)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO identity.applicant_email_verification_tokens
			(applicant_account_id, token_hash, expires_at)
		VALUES ($1::uuid, $2, now() + interval '24 hours')
	`, accountID, hashToken(token)); err != nil {
		h.fail(w, r, "save applicant verification token", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, r, "commit applicant registration", err)
		return
	}
	if err := h.mailer.SendRecovery(r.Context(), email, "applicant_verification", token); err != nil {
		h.fail(w, r, "send applicant verification email", err)
		writeError(w, http.StatusServiceUnavailable, "email_delivery_unavailable")
		return
	}
	writeJSON(w, http.StatusAccepted, genericVerificationResponse())
}

func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, 10) {
		return
	}
	var input struct {
		Token string `json:"token"`
	}
	if !decodeJSON(w, r, &input) || len(input.Token) < 32 || len(input.Token) > 256 {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		h.fail(w, r, "begin applicant email verification", err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var accountID string
	err = tx.QueryRowContext(r.Context(), `
		UPDATE identity.applicant_email_verification_tokens
		SET consumed_at=now()
		WHERE token_hash=$1 AND consumed_at IS NULL AND expires_at > now()
		RETURNING applicant_account_id::text
	`, hashToken(input.Token)).Scan(&accountID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "invalid_verification_token")
		return
	}
	if err != nil {
		h.fail(w, r, "consume applicant verification token", err)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE identity.applicant_accounts
		SET email_verified_at=coalesce(email_verified_at, now()), updated_at=now()
		WHERE id=$1::uuid
	`, accountID); err != nil {
		h.fail(w, r, "verify applicant email", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, r, "commit applicant email verification", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
}

func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, 5) {
		return
	}
	var input struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &input) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	email, ok := normalizeEmail(input.Email)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_email")
		return
	}
	if !h.allowAccount(w, email, 5) {
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	if h.mailer == nil {
		writeError(w, http.StatusServiceUnavailable, "email_delivery_disabled")
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		h.fail(w, r, "begin applicant verification resend", err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var accountID string
	err = tx.QueryRowContext(r.Context(), `
		SELECT id::text FROM identity.applicant_accounts
		WHERE email=$1 AND email_verified_at IS NULL
		FOR UPDATE
	`, email).Scan(&accountID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusAccepted, genericVerificationResponse())
		return
	}
	if err != nil {
		h.fail(w, r, "find unverified applicant", err)
		return
	}
	token, err := newToken()
	if err != nil {
		h.fail(w, r, "generate applicant verification token", err)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE identity.applicant_email_verification_tokens
		SET consumed_at=now()
		WHERE applicant_account_id=$1::uuid AND consumed_at IS NULL
	`, accountID); err != nil {
		h.fail(w, r, "replace applicant verification token", err)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO identity.applicant_email_verification_tokens
			(applicant_account_id, token_hash, expires_at)
		VALUES ($1::uuid, $2, now() + interval '24 hours')
	`, accountID, hashToken(token)); err != nil {
		h.fail(w, r, "save applicant verification token", err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.fail(w, r, "commit applicant verification resend", err)
		return
	}
	if err := h.mailer.SendRecovery(r.Context(), email, "applicant_verification", token); err != nil {
		h.fail(w, r, "send applicant verification email", err)
		writeError(w, http.StatusServiceUnavailable, "email_delivery_unavailable")
		return
	}
	writeJSON(w, http.StatusAccepted, genericVerificationResponse())
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, 5) {
		return
	}
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	email, ok := normalizeEmail(input.Email)
	if !ok || input.Password == "" || len(input.Password) > 72 {
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	if !h.allowAccount(w, email, 5) {
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	var account struct {
		ID          string
		Email       string
		DisplayName string
		Password    string
	}
	err := h.db.QueryRowContext(r.Context(), `
		SELECT id::text, email, display_name, password_hash
		FROM identity.applicant_accounts
		WHERE email=$1 AND email_verified_at IS NOT NULL
	`, email).Scan(&account.ID, &account.Email, &account.DisplayName, &account.Password)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(input.Password)) != nil) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	if err != nil {
		h.fail(w, r, "find applicant for login", err)
		return
	}
	token, err := newToken()
	if err != nil {
		h.fail(w, r, "generate applicant session", err)
		return
	}
	if _, err := h.db.ExecContext(r.Context(), `
		INSERT INTO identity.applicant_sessions (applicant_account_id, token_hash, expires_at)
		VALUES ($1::uuid, $2, now() + interval '1 hour')
	`, account.ID, hashToken(token)); err != nil {
		h.fail(w, r, "save applicant session", err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: applicantSessionCookie, Value: token, Path: "/", HttpOnly: true,
		Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: int(applicantSessionTTL.Seconds()),
		Expires: time.Now().Add(applicantSessionTTL),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"applicant": map[string]string{
			"id": account.ID, "email": account.Email, "display_name": account.DisplayName,
		},
	})
}

func (h *Handler) allow(w http.ResponseWriter, r *http.Request, limit int) bool {
	return h.allowKey(w, "ip:"+clientIP(r), limit)
}

func (h *Handler) allowAccount(w http.ResponseWriter, account string, limit int) bool {
	return h.allowKey(w, "applicant:"+account, limit)
}

func (h *Handler) allowKey(w http.ResponseWriter, key string, limit int) bool {
	if h.limiter.allow(key, limit) {
		return true
	}
	writeError(w, http.StatusTooManyRequests, "rate_limited")
	return false
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, operation string, err error) {
	h.logger.ErrorContext(r.Context(), operation,
		"request_id", w.Header().Get("X-Request-ID"), "error", err)
	writeError(w, http.StatusServiceUnavailable, "service_unavailable")
}

func decodeJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return false
	}
	var extra any
	return errors.Is(decoder.Decode(&extra), io.EOF)
}

func normalizeEmail(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) > 254 || !emailPattern.MatchString(value) {
		return "", false
	}
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value {
		return "", false
	}
	return value, true
}

func validatePassword(password string) error {
	if len(password) < 14 || len(password) > 72 || !utf8.ValidString(password) {
		return errors.New("password length is invalid")
	}
	upper, lower, digit, symbol := false, false, false, false
	for _, character := range password {
		switch {
		case unicode.IsUpper(character):
			upper = true
		case unicode.IsLower(character):
			lower = true
		case unicode.IsDigit(character):
			digit = true
		default:
			symbol = true
		}
	}
	classes := 0
	for _, present := range []bool{upper, lower, digit, symbol} {
		if present {
			classes++
		}
	}
	if classes < 3 {
		return errors.New("password complexity is insufficient")
	}
	return nil
}

func newToken() (string, error) {
	value := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", errors.New("secure random source unavailable")
	}
	return hex.EncodeToString(value), nil
}

func hashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func genericVerificationResponse() map[string]string {
	return map[string]string{
		"status":  "accepted",
		"message": "If the account is eligible, verification instructions will be sent.",
	}
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{
		"code": code, "request_id": w.Header().Get("X-Request-ID"),
	}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	if ip := net.ParseIP(strings.TrimSpace(r.RemoteAddr)); ip != nil {
		return ip.String()
	}
	return "unknown"
}

func requestIDMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			logger.ErrorContext(r.Context(), "generate applicant request ID", "error", err)
			w.Header().Set("X-Request-ID", "unavailable")
		} else {
			w.Header().Set("X-Request-ID", hex.EncodeToString(id[:]))
		}
		next.ServeHTTP(w, r)
	})
}

type rateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateEntry
}

type rateEntry struct {
	count   int
	resetAt time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{entries: make(map[string]rateEntry)}
}

func (l *rateLimiter) allow(key string, limit int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	entry := l.entries[key]
	if _, exists := l.entries[key]; !exists && len(l.entries) >= 10000 {
		for candidate, item := range l.entries {
			if !now.Before(item.resetAt) {
				delete(l.entries, candidate)
			}
		}
		if len(l.entries) >= 10000 {
			return false
		}
	}
	if !now.Before(entry.resetAt) {
		entry = rateEntry{resetAt: now.Add(10 * time.Minute)}
	}
	if entry.count >= limit {
		return false
	}
	entry.count++
	l.entries[key] = entry
	return true
}
