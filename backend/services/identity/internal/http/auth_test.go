package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identitydomain "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/domain"
)

type authServiceStub struct {
	loginFn      func(context.Context, string, string) (identitydomain.Account, error)
	sessionFn    func(context.Context, string) (identitydomain.Account, error)
	logoutFn     func(context.Context, string) error
}

func (s *authServiceStub) Login(ctx context.Context, staffID, password string) (identitydomain.Account, error) {
	if s.loginFn == nil {
		return identitydomain.Account{}, nil
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

func TestLoginSetsHttpOnlySessionCookieAndReturnsUser(t *testing.T) {
	service := &authServiceStub{loginFn: func(_ context.Context, staffID, password string) (identitydomain.Account, error) {
		if staffID != "STAFF-100" || password != "correct-password" {
			return identitydomain.Account{}, identitydomain.ErrInvalidCredentials
		}
		return identitydomain.Account{ID: "acct_123", StaffID: "STAFF-100", DisplayName: "Ada Okafor", Email: "ada.okafor@company.com", Roles: []string{"superadmin"}}, nil
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
	if cookies[0].Secure {
		t.Fatal("expected Secure flag to be false in test configuration")
	}
	if !strings.HasPrefix(cookies[0].Value, "sess_") {
		t.Fatalf("cookie value = %q, want prefix sess_", cookies[0].Value)
	}

	var body struct {
		User struct {
			ID          string   `json:"id"`
			StaffID     string   `json:"staff_id"`
			DisplayName string   `json:"display_name"`
			Roles       []string `json:"roles"`
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
}

func TestLoginRejectsInvalidCredentialsWithoutUserEnumeration(t *testing.T) {
	service := &authServiceStub{loginFn: func(_ context.Context, _, _ string) (identitydomain.Account, error) {
		return identitydomain.Account{}, identitydomain.ErrInvalidCredentials
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
