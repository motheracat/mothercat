package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/application"
	identitydomain "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/domain"
)

type authServiceStub struct {
	loginFn      func(context.Context, string, string) (identitydomain.Account, string, error)
	sessionFn    func(context.Context, string) (identitydomain.Account, error)
	logoutFn     func(context.Context, string) error
	forgotFn     func(context.Context, string) error
	activationFn func(context.Context, string) error
	bootstrapFn  func(context.Context, string, string, string, string) (string, error)
}

func (s *authServiceStub) Login(ctx context.Context, staffID, password string) (identitydomain.Account, string, error) {
	if s.loginFn == nil {
		return identitydomain.Account{}, "", nil
	}
	return s.loginFn(ctx, staffID, password)
}

func (s *authServiceStub) ResolveSession(ctx context.Context, sessionID string) (identitydomain.Account, error) {
	if s.sessionFn == nil {
		return identitydomain.Account{}, identitydomain.ErrInvalidCredentials
	}
	return s.sessionFn(ctx, sessionID)
}

func (s *authServiceStub) Logout(ctx context.Context, sessionID string) error {
	if s.logoutFn == nil {
		return nil
	}
	return s.logoutFn(ctx, sessionID)
}

func (s *authServiceStub) RequestPasswordReset(ctx context.Context, staffID string) error {
	if s.forgotFn != nil {
		return s.forgotFn(ctx, staffID)
	}
	return nil
}
func (s *authServiceStub) CompletePasswordReset(context.Context, string, string) error {
	return nil
}
func (s *authServiceStub) CompleteActivation(context.Context, string, string) error {
	return nil
}
func (s *authServiceStub) ChangePassword(context.Context, string, string, string) error {
	return nil
}
func (s *authServiceStub) CreateActivationToken(ctx context.Context, accountID string) error {
	if s.activationFn != nil {
		return s.activationFn(ctx, accountID)
	}
	return nil
}
func (*authServiceStub) ProvisionInactiveAccount(context.Context, string, string, string) (string, error) {
	return "1c9491b8-21ce-4c66-a025-006bb346b810", nil
}
func (s *authServiceStub) BootstrapSuperAdmin(ctx context.Context, staffID, email, displayName, password string) (string, error) {
	if s.bootstrapFn != nil {
		return s.bootstrapFn(ctx, staffID, email, displayName, password)
	}
	return "1c9491b8-21ce-4c66-a025-006bb346b810", nil
}

func TestAdministratorBootstrapRequiresSecretAndClosesAfterSuccess(t *testing.T) {
	var calls int
	service := &authServiceStub{
		bootstrapFn: func(_ context.Context, staffID, email, displayName, password string) (string, error) {
			calls++
			if staffID != "ADM-001" || email != "admin@example.test" ||
				displayName != "Initial Admin" || password != "MyVeryStrongPass123!" {
				t.Fatal("bootstrap request payload was not passed to the service as expected")
			}
			if calls > 1 {
				return "", application.ErrBootstrapClosed
			}
			return "1c9491b8-21ce-4c66-a025-006bb346b810", nil
		},
	}
	const bootstrapToken = "a-long-random-bootstrap-token-value"
	handler := NewAuthHandler(service, slog.New(slog.NewJSONHandler(io.Discard, nil)), AuthHandlerOptions{
		BootstrapToken: bootstrapToken,
	})
	body := `{"staff_id":"ADM-001","email":"admin@example.test","display_name":"Initial Admin","password":"MyVeryStrongPass123!"}`

	unauthorized := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap", strings.NewReader(body))
	unauthorized.Header.Set("Content-Type", "application/json")
	unauthorized.Header.Set("Authorization", "Bearer wrong-token")
	unauthorized.RemoteAddr = "192.0.2.10:12345"
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized || calls != 0 {
		t.Fatalf("unauthorized status=%d calls=%d", unauthorizedResponse.Code, calls)
	}

	for attempt, wantStatus := range []int{http.StatusCreated, http.StatusConflict} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+bootstrapToken)
		request.RemoteAddr = "192.0.2.10:12345"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != wantStatus {
			t.Fatalf("attempt %d status=%d body=%s, want %d", attempt+1, response.Code, response.Body.String(), wantStatus)
		}
		if strings.Contains(response.Body.String(), "MyVeryStrongPass123!") {
			t.Fatal("bootstrap response leaked the submitted password")
		}
	}
	if calls != 2 {
		t.Fatalf("bootstrap service calls=%d, want 2", calls)
	}
}

func TestAdministratorBootstrapEndpointIsHiddenWithoutConfiguredToken(t *testing.T) {
	service := &authServiceStub{bootstrapFn: func(context.Context, string, string, string, string) (string, error) {
		t.Fatal("bootstrap service should not be called when endpoint is disabled")
		return "", nil
	}}
	handler := NewAuthHandler(service, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", response.Code)
	}
}

func TestLoginSetsHttpOnlySessionCookieAndReturnsUser(t *testing.T) {
	service := &authServiceStub{loginFn: func(_ context.Context, staffID, password string) (identitydomain.Account, string, error) {
		if staffID != "STAFF-100" || password != "correct-password" {
			return identitydomain.Account{}, "", identitydomain.ErrInvalidCredentials
		}
		return identitydomain.Account{ID: "acct_123", StaffID: "STAFF-100", DisplayName: "Ada Okafor", Email: "ada.okafor@company.com", Role: "SuperAdmin", Roles: []string{"SuperAdmin"}, Permissions: []string{"roles.manage"}}, "sess_derived_token", nil
	}}

	handler := NewAuthHandler(service, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"staff_id":"STAFF-100","password":"correct-password"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookie count = %d, want 1", len(cookies))
	}
	if cookies[0].Name != "mothercat_session" {
		t.Fatalf("cookie name = %q, want mothercat_session", cookies[0].Name)
	}
	if !cookies[0].HttpOnly {
		t.Fatal("expected HttpOnly cookie")
	}
	if !cookies[0].Secure {
		t.Fatal("expected session cookie to always use the Secure flag")
	}
	if !strings.HasPrefix(cookies[0].Value, "sess_") {
		t.Fatalf("cookie value = %q, want prefix sess_", cookies[0].Value)
	}

	var body struct {
		User struct {
			ID          string   `json:"id"`
			StaffID     string   `json:"staff_id"`
			DisplayName string   `json:"display_name"`
			Role        string   `json:"role"`
			Roles       []string `json:"roles"`
			Permissions []string `json:"permissions"`
		} `json:"user"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if body.User.ID != "acct_123" {
		t.Fatalf("user id = %q, want acct_123", body.User.ID)
	}
	if body.User.StaffID != "STAFF-100" {
		t.Fatalf("staff id = %q, want STAFF-100", body.User.StaffID)
	}
	if body.User.Role != "SuperAdmin" || len(body.User.Roles) != 1 ||
		len(body.User.Permissions) != 1 || body.User.Permissions[0] != "roles.manage" {
		t.Fatalf("authorization = role %q, roles %v, permissions %v", body.User.Role, body.User.Roles, body.User.Permissions)
	}
}

func TestLoginRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "unknown field", body: `{"staff_id":"STAFF-1","password":"secret","is_admin":true}`},
		{name: "trailing object", body: `{"staff_id":"STAFF-1","password":"secret"}{}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &authServiceStub{loginFn: func(context.Context, string, string) (identitydomain.Account, string, error) {
				t.Fatal("login service should not be called for malformed payload")
				return identitydomain.Account{}, "", nil
			}}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()

			NewAuthHandler(service, slog.New(slog.NewJSONHandler(io.Discard, nil))).ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestLoginRateLimiterCountsOnlyFailuresAndIsSharedAcrossAccountKeys(t *testing.T) {
	limiter := newLoginRateLimiter(2, time.Minute)
	clientKey, firstAccountKey := "ip:192.0.2.1", "account:STAFF-1"

	for range 3 {
		if !limiter.allowKeys(clientKey, firstAccountKey) {
			t.Fatal("successful/non-failure attempt unexpectedly consumed a rate-limit slot")
		}
		limiter.recordSuccess([]string{clientKey, firstAccountKey}, firstAccountKey)
	}

	limiter.recordFailure(clientKey, firstAccountKey)
	if !limiter.allowKeys(clientKey, "account:STAFF-2") {
		t.Fatal("one failed attempt should not exhaust the limit")
	}
	limiter.recordFailure(clientKey, "account:STAFF-2")
	if limiter.allowKeys(clientKey, "account:STAFF-3") {
		t.Fatal("IP-level failure limit was not enforced across account IDs")
	}
}

func TestLoginRateLimiterReservesConcurrentAttempts(t *testing.T) {
	limiter := newLoginRateLimiter(2, time.Minute)
	if !limiter.allowKeys("ip:192.0.2.2", "account:STAFF-2") {
		t.Fatal("first in-flight attempt should be allowed")
	}
	if !limiter.allowKeys("ip:192.0.2.2", "account:STAFF-2") {
		t.Fatal("second in-flight attempt should be allowed")
	}
	if limiter.allowKeys("ip:192.0.2.2", "account:STAFF-2") {
		t.Fatal("third concurrent attempt bypassed reserved limit")
	}
	limiter.recordFailure("ip:192.0.2.2", "account:STAFF-2")
	limiter.recordFailure("ip:192.0.2.2", "account:STAFF-2")
	if limiter.allowKeys("ip:192.0.2.2", "account:STAFF-2") {
		t.Fatal("attempt was allowed after failure limit was reached")
	}
}

func TestRateLimitKeysIgnoreForwardedHeadersAndConnectionPorts(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	request.RemoteAddr = "192.0.2.8:12345"
	request.Header.Set("X-Forwarded-For", "198.51.100.10")
	clientKey, _ := rateLimitKeys(request, "staff-8")
	if clientKey != "ip:192.0.2.8" {
		t.Fatalf("client key = %q, want stable remote IP", clientKey)
	}

	request.RemoteAddr = "192.0.2.8:54321"
	clientKeyAgain, _ := rateLimitKeys(request, "staff-8")
	if clientKeyAgain != clientKey {
		t.Fatalf("client key changed with connection port: %q != %q", clientKeyAgain, clientKey)
	}
}

func TestLoginRejectsInvalidCredentialsWithoutUserEnumeration(t *testing.T) {
	service := &authServiceStub{loginFn: func(_ context.Context, _, _ string) (identitydomain.Account, string, error) {
		return identitydomain.Account{}, "", identitydomain.ErrInvalidCredentials
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"staff_id":"MISSING-USER","password":"wrong"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	NewAuthHandler(service, slog.New(slog.NewJSONHandler(io.Discard, nil))).ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(response.Body.String(), "invalid_credentials") {
		t.Fatalf("response body %q missing invalid_credentials code", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "MISSING-USER") {
		t.Fatalf("response body leaked the staff ID: %q", response.Body.String())
	}
}

func TestSessionRouteReturnsAuthenticatedUserFromCookie(t *testing.T) {
	service := &authServiceStub{sessionFn: func(_ context.Context, sessionID string) (identitydomain.Account, error) {
		if sessionID != "sess_321" {
			return identitydomain.Account{}, identitydomain.ErrInvalidCredentials
		}
		return identitydomain.Account{ID: "acct_321", StaffID: "STAFF-321", DisplayName: "Jane Doe", Email: "jane.doe@company.com", Roles: []string{"manager"}}, nil
	}}
	handler := NewAuthHandler(service, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	request.AddCookie(&http.Cookie{Name: "mothercat_session", Value: "sess_321", Path: "/", HttpOnly: true, Expires: time.Now().Add(time.Hour)})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), "STAFF-321") {
		t.Fatalf("response body %q does not contain the staff identifier", response.Body.String())
	}
}

func TestLogoutReportsFailureWhenSessionRevocationFails(t *testing.T) {
	service := &authServiceStub{logoutFn: func(context.Context, string) error {
		return errors.New("database unavailable")
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "sess_test"})
	response := httptest.NewRecorder()

	NewAuthHandler(service, slog.New(slog.NewJSONHandler(io.Discard, nil))).ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(response.Body.String(), "service_unavailable") {
		t.Fatalf("response body %q does not report logout failure", response.Body.String())
	}
	if cookies := response.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("logout failure unexpectedly changed the session cookie: %#v", cookies)
	}
}

func TestActivationIssuanceRequiresServiceToken(t *testing.T) {
	service := &authServiceStub{activationFn: func(context.Context, string) error {
		t.Fatal("activation must not be sent without service authentication")
		return nil
	}}
	handler := NewAuthHandler(service, slog.New(slog.NewJSONHandler(io.Discard, nil)), AuthHandlerOptions{
		OnboardingServiceToken: strings.Repeat("x", 32),
	})
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/accounts/1c9491b8-21ce-4c66-a025-006bb346b810/activation", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestActivationIssuanceUsesServiceToken(t *testing.T) {
	accountID := "1c9491b8-21ce-4c66-a025-006bb346b810"
	called := false
	service := &authServiceStub{activationFn: func(_ context.Context, gotID string) error {
		called = true
		if gotID != accountID {
			t.Fatalf("account ID = %q, want %q", gotID, accountID)
		}
		return nil
	}}
	token := strings.Repeat("x", 32)
	handler := NewAuthHandler(service, slog.New(slog.NewJSONHandler(io.Discard, nil)), AuthHandlerOptions{
		OnboardingServiceToken: token,
	})
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/accounts/"+accountID+"/activation", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted || !called {
		t.Fatalf("status = %d, called=%v; want 202 and service call", response.Code, called)
	}
}

func TestPasswordForgotReportsEmailDisabled(t *testing.T) {
	service := &authServiceStub{forgotFn: func(context.Context, string) error {
		return application.ErrEmailDeliveryUnavailable
	}}
	handler := NewAuthHandler(service, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/forgot", strings.NewReader(`{"staff_id":"STAFF-100"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "email_delivery_disabled") {
		t.Fatalf("status=%d body=%s; want email delivery disabled response", response.Code, response.Body.String())
	}
}

func TestActivationIssuanceReportsEmailDisabled(t *testing.T) {
	accountID := "1c9491b8-21ce-4c66-a025-006bb346b810"
	service := &authServiceStub{activationFn: func(context.Context, string) error {
		return application.ErrEmailDeliveryUnavailable
	}}
	token := strings.Repeat("x", 32)
	handler := NewAuthHandler(service, slog.New(slog.NewJSONHandler(io.Discard, nil)), AuthHandlerOptions{
		OnboardingServiceToken: token,
	})
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/accounts/"+accountID+"/activation", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "email_delivery_disabled") {
		t.Fatalf("status=%d body=%s; want email delivery disabled response", response.Code, response.Body.String())
	}
}

func TestPasswordRecoveryRateLimitIsPerRemoteIP(t *testing.T) {
	handler := NewAuthHandler(&authServiceStub{}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	var lastStatus int
	for range maxRecoveryAttemptsPerIP + 1 {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/forgot", strings.NewReader(`{"staff_id":"STAFF-100"}`))
		request.RemoteAddr = "192.0.2.42:1234"
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		lastStatus = response.Code
	}
	if lastStatus != http.StatusTooManyRequests {
		t.Fatalf("status after rate limit = %d, want %d", lastStatus, http.StatusTooManyRequests)
	}
}
