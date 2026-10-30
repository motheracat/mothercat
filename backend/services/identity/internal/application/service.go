package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	identitydomain "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/domain"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/observability"
)

var ErrInactiveAccount = errors.New("inactive_account")
var ErrAccountNotFound = errors.New("account_not_found")
var ErrAccountConflict = errors.New("account_conflict")
var ErrInvalidProvisioningDetails = errors.New("invalid_account_provisioning_details")
var ErrSessionNotFound = errors.New("session_not_found")

const SessionTTL = time.Hour
const recoveryTokenTTL = 30 * time.Minute
const activationTokenTTL = 24 * time.Hour
const minimumPasswordLength = 14
const maximumPasswordLength = 72

var ErrInvalidRecoveryToken = errors.New("invalid_recovery_token")
var ErrInvalidCurrentPassword = errors.New("invalid_current_password")
var ErrEmailDeliveryUnavailable = errors.New("email_delivery_unavailable")

// Repository defines the data access contract required for Identity auth flows.
type Repository interface {
	FindByStaffID(ctx context.Context, staffID string) (AccountRecord, error)
	FindBySessionToken(ctx context.Context, token string) (AccountRecord, error)
	CreateSession(ctx context.Context, accountID, staffID, token string, expiresAt time.Time) error
	RevokeSession(ctx context.Context, token string) error
	RecordAuthenticationEvent(ctx context.Context, staffID string, accountID *string, successful bool) error
	FindByAccountID(ctx context.Context, accountID string) (AccountRecord, error)
	CreateRecoveryToken(ctx context.Context, accountID, purpose, tokenHash string, expiresAt time.Time) error
	CompleteRecovery(ctx context.Context, tokenHash, purpose, passwordHash string, activate bool) (AccountRecord, error)
	ChangePassword(ctx context.Context, sessionToken, passwordHash string) error
	CreateInactiveAccount(ctx context.Context, staffID, email, passwordHash, displayName string) (string, error)
}

type Authorization struct {
	Role        string
	Roles       []string
	Permissions []string
}

type AuthorizationProvider interface {
	GetAuthorization(context.Context, string) (Authorization, error)
}

type Mailer interface {
	SendRecovery(context.Context, string, string, string) error
}

// AccountRecord is the persistence representation of a user account. It keeps the
// password hash server-side and converts to the public domain account shape.
type AccountRecord struct {
	ID                 string
	StaffID            string
	Email              string
	DisplayName        string
	PasswordHash       string
	IsActive           bool
	MustChangePassword bool
}

type Service struct {
	repository     Repository
	authorizations AuthorizationProvider
	mailer         Mailer
	logger         *slog.Logger
}

func NewService(repository Repository, authorizations AuthorizationProvider, mailer Mailer, loggers ...*slog.Logger) *Service {
	logger := slog.Default()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	return &Service{repository: repository, authorizations: authorizations, mailer: mailer, logger: logger}
}

func (s *Service) Login(ctx context.Context, staffID, password string) (identitydomain.Account, string, error) {
	if s == nil || s.repository == nil {
		return identitydomain.Account{}, "", identitydomain.ErrInvalidCredentials
	}
	if strings.TrimSpace(staffID) == "" || password == "" {
		return identitydomain.Account{}, "", identitydomain.ErrInvalidCredentials
	}

	started := time.Now()
	accountRecord, err := s.repository.FindByStaffID(ctx, staffID)
	if err != nil {
		outcome := "error"
		if errors.Is(err, ErrAccountNotFound) {
			outcome = "not_found"
		}
		s.logAuthStage(ctx, "identity_account_lookup", started, outcome)
		if !errors.Is(err, ErrAccountNotFound) {
			return identitydomain.Account{}, "", err
		}
		started = time.Now()
		if eventErr := s.repository.RecordAuthenticationEvent(ctx, staffID, nil, false); eventErr != nil {
			s.logAuthStage(ctx, "identity_auth_event_write", started, "error")
			return identitydomain.Account{}, "", eventErr
		}
		s.logAuthStage(ctx, "identity_auth_event_write", started, "success")
		return identitydomain.Account{}, "", identitydomain.ErrInvalidCredentials
	}
	s.logAuthStage(ctx, "identity_account_lookup", started, "found")
	if !accountRecord.IsActive {
		started = time.Now()
		if eventErr := s.repository.RecordAuthenticationEvent(ctx, staffID, &accountRecord.ID, false); eventErr != nil {
			s.logAuthStage(ctx, "identity_auth_event_write", started, "error")
			return identitydomain.Account{}, "", eventErr
		}
		s.logAuthStage(ctx, "identity_auth_event_write", started, "success")
		return identitydomain.Account{}, "", identitydomain.ErrInvalidCredentials
	}
	started = time.Now()
	if err := verifyPasswordHash(accountRecord.PasswordHash, password); err != nil {
		s.logAuthStage(ctx, "password_hash_verify", started, "invalid")
		started = time.Now()
		if eventErr := s.repository.RecordAuthenticationEvent(ctx, staffID, &accountRecord.ID, false); eventErr != nil {
			s.logAuthStage(ctx, "identity_auth_event_write", started, "error")
			return identitydomain.Account{}, "", eventErr
		}
		s.logAuthStage(ctx, "identity_auth_event_write", started, "success")
		return identitydomain.Account{}, "", identitydomain.ErrInvalidCredentials
	}
	s.logAuthStage(ctx, "password_hash_verify", started, "valid")
	authorization, err := s.authorization(ctx, accountRecord.ID)
	if err != nil {
		return identitydomain.Account{}, "", err
	}

	sessionToken, err := newSessionToken(rand.Reader)
	if err != nil {
		return identitydomain.Account{}, "", err
	}
	started = time.Now()
	if err := s.repository.CreateSession(ctx, accountRecord.ID, accountRecord.StaffID, sessionToken, time.Now().Add(SessionTTL)); err != nil {
		s.logAuthStage(ctx, "identity_session_persist", started, "error")
		return identitydomain.Account{}, "", err
	}
	s.logAuthStage(ctx, "identity_session_persist", started, "success")

	return publicAccount(accountRecord, authorization), sessionToken, nil
}

func (s *Service) ResolveSession(ctx context.Context, sessionToken string) (identitydomain.Account, error) {
	if s == nil || s.repository == nil {
		return identitydomain.Account{}, identitydomain.ErrInvalidCredentials
	}
	started := time.Now()
	accountRecord, err := s.repository.FindBySessionToken(ctx, sessionToken)
	if err != nil {
		outcome := "error"
		if errors.Is(err, ErrSessionNotFound) {
			outcome = "not_found"
		}
		s.logAuthStage(ctx, "identity_session_lookup", started, outcome)
		if errors.Is(err, ErrSessionNotFound) {
			return identitydomain.Account{}, identitydomain.ErrInvalidCredentials
		}
		return identitydomain.Account{}, err
	}
	s.logAuthStage(ctx, "identity_session_lookup", started, "found")
	authorization, err := s.authorization(ctx, accountRecord.ID)
	if err != nil {
		return identitydomain.Account{}, err
	}
	return publicAccount(accountRecord, authorization), nil
}

func (s *Service) Logout(ctx context.Context, sessionToken string) error {
	if s == nil || s.repository == nil {
		return identitydomain.ErrInvalidCredentials
	}
	return s.repository.RevokeSession(ctx, sessionToken)
}

func (s *Service) RequestPasswordReset(ctx context.Context, staffID string) error {
	if s == nil || s.repository == nil {
		return errors.New("identity repository is unavailable")
	}
	if s.mailer == nil {
		return ErrEmailDeliveryUnavailable
	}
	account, err := s.repository.FindByStaffID(ctx, strings.TrimSpace(staffID))
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			return nil
		}
		return fmt.Errorf("find account for password recovery: %w", err)
	}
	if !account.IsActive {
		return nil
	}
	token, err := newSessionToken(rand.Reader)
	if err != nil {
		return err
	}
	if err := s.repository.CreateRecoveryToken(ctx, account.ID, "password_reset", hashRecoveryToken(token), time.Now().Add(recoveryTokenTTL)); err != nil {
		return fmt.Errorf("create password recovery token: %w", err)
	}
	if err := s.mailer.SendRecovery(ctx, account.Email, "password_reset", token); err != nil {
		return fmt.Errorf("deliver password recovery message: %w", err)
	}
	return nil
}

// CreateActivationToken is intended for a trusted onboarding orchestrator, not
// a public unauthenticated route. Only inactive accounts can receive one.
func (s *Service) CreateActivationToken(ctx context.Context, accountID string) error {
	if s == nil || s.repository == nil {
		return errors.New("identity repository is unavailable")
	}
	if s.mailer == nil {
		return ErrEmailDeliveryUnavailable
	}
	account, err := s.repository.FindByAccountID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("find account for activation: %w", err)
	}
	if account.IsActive {
		return errors.New("active account cannot be activated again")
	}
	token, err := newSessionToken(rand.Reader)
	if err != nil {
		return err
	}
	if err := s.repository.CreateRecoveryToken(ctx, account.ID, "activation", hashRecoveryToken(token), time.Now().Add(activationTokenTTL)); err != nil {
		return fmt.Errorf("create account activation token: %w", err)
	}
	if err := s.mailer.SendRecovery(ctx, account.Email, "activation", token); err != nil {
		return fmt.Errorf("deliver account activation message: %w", err)
	}
	return nil
}

func (s *Service) ProvisionInactiveAccount(ctx context.Context, staffID, email, displayName string) (string, error) {
	if s == nil || s.repository == nil {
		return "", errors.New("identity repository is unavailable")
	}
	staffID = strings.TrimSpace(staffID)
	email = strings.TrimSpace(email)
	displayName = strings.TrimSpace(displayName)
	address, err := mail.ParseAddress(email)
	if staffID == "" || len(staffID) > 80 || err != nil || address == nil || address.Address != email ||
		len(email) > 254 || displayName == "" || utf8.RuneCountInString(displayName) > 160 {
		return "", ErrInvalidProvisioningDetails
	}
	temporaryPassword, err := newSessionToken(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate initial credential: %w", err)
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(temporaryPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash initial credential: %w", err)
	}
	accountID, err := s.repository.CreateInactiveAccount(ctx, staffID, email, string(passwordHash), displayName)
	if err != nil {
		if errors.Is(err, ErrAccountConflict) {
			return "", ErrAccountConflict
		}
		return "", fmt.Errorf("create inactive account: %w", err)
	}
	return accountID, nil
}

func (s *Service) CompletePasswordReset(ctx context.Context, token, password string) error {
	return s.completeRecovery(ctx, token, password, "password_reset", false)
}

func (s *Service) CompleteActivation(ctx context.Context, token, password string) error {
	return s.completeRecovery(ctx, token, password, "activation", true)
}

func (s *Service) completeRecovery(ctx context.Context, token, password, purpose string, activate bool) error {
	if s == nil || s.repository == nil {
		return errors.New("identity repository is unavailable")
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash replacement password: %w", err)
	}
	_, err = s.repository.CompleteRecovery(ctx, hashRecoveryToken(token), purpose, string(passwordHash), activate)
	if err != nil {
		if errors.Is(err, ErrInvalidRecoveryToken) {
			return ErrInvalidRecoveryToken
		}
		return fmt.Errorf("complete credential recovery: %w", err)
	}
	return nil
}

func (s *Service) ChangePassword(ctx context.Context, sessionToken, currentPassword, newPassword string) error {
	if s == nil || s.repository == nil {
		return errors.New("identity repository is unavailable")
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	account, err := s.repository.FindBySessionToken(ctx, sessionToken)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return identitydomain.ErrInvalidCredentials
		}
		return fmt.Errorf("find authenticated account: %w", err)
	}
	if err := verifyPasswordHash(account.PasswordHash, currentPassword); err != nil {
		return ErrInvalidCurrentPassword
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	return s.repository.ChangePassword(ctx, sessionToken, string(passwordHash))
}

func (s *Service) authorization(ctx context.Context, accountID string) (Authorization, error) {
	if s.authorizations == nil {
		s.logAuthStage(ctx, "access_authorization_lookup", time.Now(), "not_configured")
		return Authorization{Role: "Staff", Roles: []string{}, Permissions: []string{}}, nil
	}
	started := time.Now()
	authorization, err := s.authorizations.GetAuthorization(ctx, accountID)
	if err != nil {
		s.logAuthStage(ctx, "access_authorization_lookup", started, "error")
		return Authorization{}, fmt.Errorf("retrieve account authorization: %w", err)
	}
	s.logAuthStage(ctx, "access_authorization_lookup", started, "success")
	if authorization.Roles == nil {
		authorization.Roles = []string{}
	}
	if authorization.Permissions == nil {
		authorization.Permissions = []string{}
	}
	if authorization.Role == "" && len(authorization.Roles) > 0 {
		authorization.Role = authorization.Roles[0]
	}
	if authorization.Role == "" {
		authorization.Role = "Staff"
	}
	return authorization, nil
}

func (s *Service) logAuthStage(ctx context.Context, stage string, started time.Time, outcome string) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.InfoContext(ctx, "authentication stage",
		"request_id", observability.RequestIDFromContext(ctx),
		"stage", stage,
		"duration_ms", time.Since(started).Milliseconds(),
		"outcome", outcome,
	)
}

func publicAccount(record AccountRecord, authorization Authorization) identitydomain.Account {
	return identitydomain.Account{
		ID: record.ID, StaffID: record.StaffID, DisplayName: record.DisplayName,
		Email: record.Email, Role: authorization.Role, Roles: authorization.Roles,
		Permissions: authorization.Permissions,
	}
}

func validatePassword(password string) error {
	if len(password) < minimumPasswordLength || len(password) > maximumPasswordLength {
		return fmt.Errorf("password must be between %d and %d bytes", minimumPasswordLength, maximumPasswordLength)
	}
	upper, lower, digit, symbol := false, false, false, false
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			upper = true
		case unicode.IsLower(char):
			lower = true
		case unicode.IsDigit(char):
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
		return errors.New("password must contain at least three character classes")
	}
	return nil
}

func hashRecoveryToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func verifyPasswordHash(passwordHash, password string) error {
	if passwordHash == "" || password == "" {
		return identitydomain.ErrInvalidCredentials
	}
	if _, err := bcrypt.Cost([]byte(passwordHash)); err != nil {
		return identitydomain.ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return identitydomain.ErrInvalidCredentials
	}
	return nil
}

func newSessionToken(source io.Reader) (string, error) {
	if source == nil {
		return "", errors.New("secure random source unavailable")
	}
	buf := make([]byte, 32)
	if _, err := io.ReadFull(source, buf); err != nil {
		return "", errors.New("generate session token: secure random source failed")
	}
	return "sess_" + hex.EncodeToString(buf), nil
}
