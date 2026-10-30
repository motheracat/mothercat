package http

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	accessdomain "github.com/mothercat-enterprise/mothercat/backend/services/access/internal/domain"
)

type storeStub struct {
	authorization accessdomain.Authorization
	err           error
}

func (s *storeStub) GetUserAuthorization(context.Context, string) (accessdomain.Authorization, error) {
	return s.authorization, s.err
}

func (s *storeStub) Ping(context.Context) error { return nil }

func TestAuthorizationRequiresServiceBearerToken(t *testing.T) {
	handler := NewHandler(&storeStub{}, strings.Repeat("x", 32), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodGet, "/internal/v1/users/1c9491b8-21ce-4c66-a025-006bb346b810/authorization", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestAuthorizationReturnsRoleAndPermissions(t *testing.T) {
	handler := NewHandler(&storeStub{authorization: accessdomain.Authorization{
		Role: "SuperAdmin", Roles: []string{"SuperAdmin"},
		Permissions: []string{"roles.manage"},
	}}, strings.Repeat("x", 32), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodGet, "/internal/v1/users/1c9491b8-21ce-4c66-a025-006bb346b810/authorization", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"role":"SuperAdmin"`) ||
		!strings.Contains(response.Body.String(), `"permissions":["roles.manage"]`) {
		t.Fatalf("response does not contain expected authorization: %s", response.Body.String())
	}
}

func TestAuthorizationRejectsMalformedUserID(t *testing.T) {
	handler := NewHandler(&storeStub{}, strings.Repeat("x", 32), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodGet, "/internal/v1/users/not-a-uuid/authorization", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestAuthorizationHidesStoreErrors(t *testing.T) {
	handler := NewHandler(&storeStub{err: errors.New("sensitive database detail")}, strings.Repeat("x", 32), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodGet, "/internal/v1/users/1c9491b8-21ce-4c66-a025-006bb346b810/authorization", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if strings.Contains(response.Body.String(), "sensitive database detail") {
		t.Fatalf("response leaked a store error: %s", response.Body.String())
	}
}
