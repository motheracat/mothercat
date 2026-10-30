package application

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/observability"
)

type repositoryStub struct {
	account           AccountRecord
	findErr           error
	sessionCreated    bool
	recoveryHash      string
	recoveryPurpose   string
	recoveryExpiry    time.Time
	completedPurpose  string
	completedHash     string
	completedActivate bool
}

func (r *repositoryStub) FindByStaffID(context.Context, string) (AccountRecord, error) {
	return r.account, r.findErr
}
func (r *repositoryStub) FindBySessionToken(context.Context, string) (AccountRecord, error) {
	return r.account, nil
}
func (r *repositoryStub) CreateSession(context.Context, string, string, string, time.Time) error {
	r.sessionCreated = true
	return nil
}
func (*repositoryStub) RevokeSession(context.Context, string) error { return nil }
func (*repositoryStub) RecordAuthenticationEvent(context.Context, string, *string, bool) error {
	return nil
}
func (r *repositoryStub) FindByAccountID(context.Context, string) (AccountRecord, error) {
	return r.account, nil
}
func (r *repositoryStub) CreateRecoveryToken(_ context.Context, _, purpose, tokenHash string, expiresAt time.Time) error {
	r.recoveryHash, r.recoveryPurpose, r.recoveryExpiry = tokenHash, purpose, expiresAt
	return nil
}
func (r *repositoryStub) CompleteRecovery(_ context.Context, _, purpose, passwordHash string, activate bool) (AccountRecord, error) {
	r.completedPurpose, r.completedHash, r.completedActivate = purpose, passwordHash, activate
	return r.account, nil
}
func (*repositoryStub) ChangePassword(context.Context, string, string) error { return nil }

type authorizationStub struct {
	authorization Authorization
	err           error
}

func (a authorizationStub) GetAuthorization(context.Context, string) (Authorization, error) {
	return a.authorization, a.err
}

type mailerStub struct {
	recipient string
	purpose   string
	token     string
}

func (m *mailerStub) SendRecovery(_ context.Context, recipient, purpose, token string) error {
	m.recipient, m.purpose, m.token = recipient, purpose, token
	return nil
}

func TestLoginReturnsAccessAuthorization(t *testing.T) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	repo := &repositoryStub{account: AccountRecord{
		ID: "account-1", StaffID: "STAFF-1", Email: "staff@example.test",
		DisplayName: "Staff Member", PasswordHash: string(passwordHash), IsActive: true,
	}}
	service := NewService(repo, authorizationStub{authorization: Authorization{
		Role: "SuperAdmin", Roles: []string{"SuperAdmin"}, Permissions: []string{"roles.manage"},
	}}, nil)

	account, session, err := service.Login(context.Background(), "STAFF-1", "correct-password")

	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if session == "" || !repo.sessionCreated {
		t.Fatalf("session = %q, sessionCreated = %v", session, repo.sessionCreated)
	}
	if account.Role != "SuperAdmin" || len(account.Roles) != 1 ||
		len(account.Permissions) != 1 || account.Permissions[0] != "roles.manage" {
		t.Fatalf("authorization = role %q, roles %v, permissions %v", account.Role, account.Roles, account.Permissions)
	}
}

func TestLoginFailsClosedWhenAccessIsUnavailable(t *testing.T) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	repo := &repositoryStub{account: AccountRecord{
		ID: "account-1", StaffID: "STAFF-1", PasswordHash: string(passwordHash), IsActive: true,
	}}
	service := NewService(repo, authorizationStub{err: errors.New("Access unavailable")}, nil)

	if _, _, err := service.Login(context.Background(), "STAFF-1", "correct-password"); err == nil {
		t.Fatal("Login() succeeded while Access was unavailable")
	}
	if repo.sessionCreated {
		t.Fatal("a session was created without authorization data")
	}
}

func TestLoginLogsCorrelatedStageTimingsWithoutCredentialsOrAccountIdentifiers(t *testing.T) {
	password := "correct-password"
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	repo := &repositoryStub{account: AccountRecord{
		ID: "account-private-id", StaffID: "STAFF-PRIVATE-ID",
		PasswordHash: string(passwordHash), IsActive: true,
	}}
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	service := NewService(repo, authorizationStub{authorization: Authorization{
		Role: "SuperAdmin", Roles: []string{"SuperAdmin"},
	}}, nil, logger)
	ctx := observability.ContextWithRequestID(context.Background(), "0123456789abcdef0123456789abcdef")

	if _, _, err := service.Login(ctx, "STAFF-PRIVATE-ID", password); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	logs := output.String()
	for _, expected := range []string{
		"0123456789abcdef0123456789abcdef",
		"identity_account_lookup",
		"password_hash_verify",
		"access_authorization_lookup",
		"identity_session_persist",
		"duration_ms",
	} {
		if !strings.Contains(logs, expected) {
			t.Errorf("logs do not contain %q: %s", expected, logs)
		}
	}
	for _, privateValue := range []string{password, "STAFF-PRIVATE-ID", "account-private-id"} {
		if strings.Contains(logs, privateValue) {
			t.Errorf("logs contain private value %q", privateValue)
		}
	}
}

func TestPasswordRecoveryStoresOnlyHashedSingleUseToken(t *testing.T) {
	repo := &repositoryStub{account: AccountRecord{
		ID: "account-1", StaffID: "STAFF-1", Email: "staff@example.test", IsActive: true,
	}}
	mailer := &mailerStub{}
	service := NewService(repo, nil, mailer)

	if err := service.RequestPasswordReset(context.Background(), "STAFF-1"); err != nil {
		t.Fatalf("RequestPasswordReset() error = %v", err)
	}
	if mailer.token == "" || repo.recoveryHash != hashRecoveryToken(mailer.token) ||
		repo.recoveryHash == mailer.token {
		t.Fatal("recovery token was not stored as a hash and sent to the mailer")
	}
	if mailer.recipient != repo.account.Email || mailer.purpose != "password_reset" ||
		repo.recoveryPurpose != "password_reset" {
		t.Fatalf("unexpected recovery delivery: %+v", mailer)
	}
	if time.Until(repo.recoveryExpiry) <= 0 || time.Until(repo.recoveryExpiry) > recoveryTokenTTL {
		t.Fatalf("recovery token expiry is outside the expected window: %v", repo.recoveryExpiry)
	}
}

func TestPasswordResetHashesPasswordAndUsesResetPurpose(t *testing.T) {
	repo := &repositoryStub{}
	service := NewService(repo, nil, nil)
	password := "Correct-Horse-Battery7!"

	if err := service.CompletePasswordReset(context.Background(), "opaque-token", password); err != nil {
		t.Fatalf("CompletePasswordReset() error = %v", err)
	}
	if repo.completedPurpose != "password_reset" || repo.completedActivate {
		t.Fatalf("purpose = %q, activate = %v", repo.completedPurpose, repo.completedActivate)
	}
	if repo.completedHash == password || bcrypt.CompareHashAndPassword([]byte(repo.completedHash), []byte(password)) != nil {
		t.Fatal("replacement password was not stored as a valid bcrypt hash")
	}
}

func TestActivationTokenCannotBeIssuedForActiveAccount(t *testing.T) {
	repo := &repositoryStub{account: AccountRecord{ID: "account-1", IsActive: true}}
	service := NewService(repo, nil, &mailerStub{})

	if err := service.CreateActivationToken(context.Background(), "account-1"); err == nil {
		t.Fatal("activation token was issued for an active account")
	}
	if repo.recoveryHash != "" {
		t.Fatal("activation token was persisted for an active account")
	}
}
