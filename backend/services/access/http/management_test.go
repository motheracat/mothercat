package http

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mothercat-enterprise/mothercat/backend/services/access/internal/domain"
)

type authenticatorStub struct {
	principal Principal
	err       error
}

func (a authenticatorStub) Authenticate(context.Context, string) (Principal, error) {
	return a.principal, a.err
}

type storeStub struct {
	roles             []domain.Role
	permissions       []domain.Permission
	userRoles         []string
	createdRole       domain.Role
	updatedRole       domain.Role
	rolePermissions   []string
	replacedUserRoles []string
	createCalls       int
	updateCalls       int
	permissionCalls   int
	userRoleCalls     int
	roleListCalls     int
}

func (s *storeStub) ListRoles(context.Context) ([]domain.Role, error) {
	s.roleListCalls++
	return s.roles, nil
}

func (s *storeStub) ListPermissions(context.Context) ([]domain.Permission, error) {
	return s.permissions, nil
}

func (s *storeStub) CreateRole(_ context.Context, _ string, role domain.Role) (domain.Role, error) {
	s.createCalls++
	s.createdRole = role
	return role, nil
}

func (s *storeStub) UpdateRole(_ context.Context, _, roleID string, name, description *string) (domain.Role, error) {
	s.updateCalls++
	role := domain.Role{ID: roleID}
	if name != nil {
		role.Name = *name
	}
	if description != nil {
		role.Description = *description
	}
	s.updatedRole = role
	return role, nil
}

func (s *storeStub) ReplaceRolePermissions(_ context.Context, _, _ string, permissions []string) ([]string, error) {
	s.permissionCalls++
	s.rolePermissions = permissions
	return permissions, nil
}

func (s *storeStub) GetUserRoles(context.Context, string) ([]string, error) {
	return s.userRoles, nil
}

func (s *storeStub) ReplaceUserRoles(_ context.Context, _, _ string, roles []string) ([]string, error) {
	s.userRoleCalls++
	s.replacedUserRoles = roles
	return roles, nil
}

func authorizedAccessHandler(s *storeStub, permissions ...string) http.Handler {
	return NewHandler(
		s,
		authenticatorStub{principal: Principal{ID: "actor-id", Permissions: permissions}},
		slog.New(slog.NewTextHandler(testWriter{}, nil)),
	)
}

type testWriter struct{}

func (testWriter) Write(data []byte) (int, error) { return len(data), nil }

func authenticatedRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session"})
	request.Header.Set("Origin", "http://example.com")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}

func TestRoleReadRequiresBackendPermission(t *testing.T) {
	store := &storeStub{roles: []domain.Role{{ID: "staff", Name: "Staff"}}}
	handler := authorizedAccessHandler(store)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/roles", ""))
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if store.roleListCalls != 0 {
		t.Fatal("role store was read without roles.read permission")
	}
}

func TestCreateRoleRequiresSameOriginAndValidatesInput(t *testing.T) {
	t.Run("rejects cross-origin mutation", func(t *testing.T) {
		store := &storeStub{}
		handler := authorizedAccessHandler(store, "roles.manage")
		request := authenticatedRequest(http.MethodPost, "/api/v1/roles",
			`{"role_id":"auditor","name":"Auditor","description":"Read-only audit access"}`)
		request.Header.Set("Origin", "https://attacker.example")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
		}
		if store.createCalls != 0 {
			t.Fatal("role creation reached store for cross-origin request")
		}
	})

	t.Run("rejects invalid role key", func(t *testing.T) {
		store := &storeStub{}
		handler := authorizedAccessHandler(store, "roles.manage")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/v1/roles",
			`{"role_id":"Admin","name":"Admin","description":""}`))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
		}
		if store.createCalls != 0 {
			t.Fatal("invalid role reached store")
		}
	})

	t.Run("accepts valid role", func(t *testing.T) {
		store := &storeStub{}
		handler := authorizedAccessHandler(store, "roles.manage")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/v1/roles",
			`{"role_id":"auditor","name":"Auditor","description":"Read-only audit access"}`))
		if response.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body)
		}
		if store.createCalls != 1 || store.createdRole.ID != "auditor" {
			t.Fatalf("created role = %#v; call count %d", store.createdRole, store.createCalls)
		}
	})
}

func TestPermissionAndUserRoleMutationRequirePermissionAndOrigin(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		permission string
	}{
		{
			name: "role permissions", method: http.MethodPut,
			target: "/api/v1/roles/auditor/permissions",
			body:   `{"permission_keys":["audit.read"]}`, permission: "roles.manage",
		},
		{
			name: "user roles", method: http.MethodPut,
			target: "/api/v1/users/00000000-0000-4000-8000-000000000001/roles",
			body:   `{"role_ids":["staff"]}`, permission: "user_roles.assign",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &storeStub{}
			handler := authorizedAccessHandler(store, test.permission)
			request := authenticatedRequest(test.method, test.target, test.body)
			request.Header.Del("Origin")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
			}
			if store.permissionCalls != 0 || store.userRoleCalls != 0 {
				t.Fatal("mutation reached store without an Origin header")
			}
		})
	}
}

func TestRoleAndUserRoleRequestsRejectMalformedJSONAndIdentifiers(t *testing.T) {
	tests := []struct {
		name   string
		target string
		body   string
	}{
		{name: "unknown permission field", target: "/api/v1/roles/auditor/permissions", body: `{"permission_keys":[],"extra":true}`},
		{name: "invalid permission name", target: "/api/v1/roles/auditor/permissions", body: `{"permission_keys":["*"]}`},
		{name: "duplicate permission key", target: "/api/v1/roles/auditor/permissions", body: `{"permission_keys":["audit.read","audit.read"]}`},
		{name: "invalid user UUID", target: "/api/v1/users/not-a-uuid/roles", body: `{"role_ids":["staff"]}`},
		{name: "invalid role key", target: "/api/v1/users/00000000-0000-4000-8000-000000000001/roles", body: `{"role_ids":["SuperAdmin"]}`},
		{name: "duplicate role key", target: "/api/v1/users/00000000-0000-4000-8000-000000000001/roles", body: `{"role_ids":["staff","staff"]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &storeStub{}
			handler := authorizedAccessHandler(store, "roles.manage", "user_roles.assign")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authenticatedRequest(http.MethodPut, test.target, test.body))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body)
			}
			if store.permissionCalls != 0 || store.userRoleCalls != 0 {
				t.Fatal("invalid request reached the store")
			}
		})
	}
}

func TestUnauthenticatedRequestDoesNotReachStore(t *testing.T) {
	store := &storeStub{}
	handler := NewHandler(store, authenticatorStub{err: ErrUnauthenticated}, nil)
	request := authenticatedRequest(http.MethodGet, "/api/v1/roles", "")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}
