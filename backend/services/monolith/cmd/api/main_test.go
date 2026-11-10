package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRouterMountsOrganizationEndpoints(t *testing.T) {
	identity := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	organization := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	access := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	audit := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
	})
	tasks := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	notifications := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	opportunities := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	applicants := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	handler := newRouter(nil, identity, organization, access, audit, tasks, notifications, opportunities, applicants)

	for _, path := range []string{
		"/api/v1/staff",
		"/api/v1/staff/123",
		"/api/v1/departments",
		"/api/v1/departments/123",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNoContent {
			t.Errorf("%s reached status %d; want organization handler status %d", path, response.Code, http.StatusNoContent)
		}
	}

	for _, path := range []string{
		"/api/v1/auth/applicant/register",
		"/api/v1/auth/applicant/verify-email",
		"/api/v1/auth/applicant/resend-verification",
		"/api/v1/auth/applicant/login",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != http.StatusAccepted {
			t.Errorf("%s reached status %d; want applicant identity handler status %d",
				path, response.Code, http.StatusAccepted)
		}
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil))
	if response.Code != http.StatusTeapot {
		t.Errorf("identity endpoint reached status %d; want identity handler status %d", response.Code, http.StatusTeapot)
	}

	for _, route := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/roles"},
		{method: http.MethodPatch, path: "/api/v1/roles/auditor"},
		{method: http.MethodPut, path: "/api/v1/roles/auditor/permissions"},
		{method: http.MethodGet, path: "/api/v1/permissions"},
		{method: http.MethodGet, path: "/api/v1/users/00000000-0000-4000-8000-000000000001/roles"},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(route.method, route.path, nil))
		if response.Code != http.StatusAccepted {
			t.Errorf("%s %s reached status %d; want access handler status %d",
				route.method, route.path, response.Code, http.StatusAccepted)
		}
	}

	for _, path := range []string{
		"/api/v1/audit-events",
		"/api/v1/audit-events/00000000-0000-4000-8000-000000000001",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusPartialContent {
			t.Errorf("%s reached status %d; want audit handler status %d",
				path, response.Code, http.StatusPartialContent)
		}
	}

	for _, path := range []string{"/api/v1/tasks", "/api/v1/tasks/123", "/api/v1/me/tasks"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusCreated {
			t.Errorf("%s reached status %d; want task handler status %d",
				path, response.Code, http.StatusCreated)
		}

		for _, path := range []string{"/api/v1/notifications", "/api/v1/notifications/unread-count", "/api/v1/notifications/preferences"} {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusAccepted {
				t.Errorf("%s reached status %d; want notifications handler status %d",
					path, response.Code, http.StatusAccepted)
			}
		}
	}

	for _, path := range []string{
		"/api/v1/opportunities",
		"/api/v1/opportunities/manage",
		"/api/v1/opportunities/00000000-0000-4000-8000-000000000001",
		"/api/v1/siwes-applications",
		"/api/v1/siwes-applications/00000000-0000-4000-8000-000000000001",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Errorf("%s reached status %d; want opportunities handler status %d",
				path, response.Code, http.StatusOK)
		}
	}
}

func TestValidateDatabaseSettings(t *testing.T) {
	t.Setenv("IDENTITY_ONBOARDING_TOKEN", "a-long-enough-test-onboarding-token")
	t.Setenv("IDENTITY_BOOTSTRAP_TOKEN", "")

	tests := []struct {
		name        string
		databaseURL string
		appEnv      string
		wantErr     bool
	}{
		{
			name:        "development accepts PostgreSQL URL without ssl mode",
			databaseURL: "postgresql://user:pass@db.example.test:5432/mothercat",
			appEnv:      "development",
		},
		{
			name:        "production accepts required TLS",
			databaseURL: "postgresql://user:pass@db.example.test:5432/mothercat?sslmode=require",
			appEnv:      "production",
		},
		{
			name:        "production rejects missing TLS",
			databaseURL: "postgresql://user:pass@db.example.test:5432/mothercat",
			appEnv:      "production",
			wantErr:     true,
		},
		{
			name:        "staging rejects disabled TLS",
			databaseURL: "postgresql://user:pass@db.example.test:5432/mothercat?sslmode=disable",
			appEnv:      "staging",
			wantErr:     true,
		},
		{
			name:        "rejects a non PostgreSQL URL",
			databaseURL: "mongodb://user:pass@db.example.test",
			appEnv:      "development",
			wantErr:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateDatabaseSettings(test.databaseURL, test.appEnv)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateDatabaseSettings() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestValidateDatabaseSettingsValidatesOptionalBootstrapToken(t *testing.T) {
	t.Setenv("IDENTITY_ONBOARDING_TOKEN", "a-long-enough-test-onboarding-token")
	t.Setenv("IDENTITY_BOOTSTRAP_TOKEN", "short")
	databaseURL := "postgresql://user:pass@db.example.test:5432/mothercat"
	if err := validateDatabaseSettings(databaseURL, "development"); err == nil {
		t.Fatal("validateDatabaseSettings accepted a short bootstrap token")
	}
	t.Setenv("IDENTITY_BOOTSTRAP_TOKEN", "a-long-enough-test-bootstrap-token")
	if err := validateDatabaseSettings(databaseURL, "development"); err != nil {
		t.Fatalf("validateDatabaseSettings rejected a strong bootstrap token: %v", err)
	}
	t.Setenv("IDENTITY_BOOTSTRAP_TOKEN", "a-long-enough-test-onboarding-token")
	if err := validateDatabaseSettings(databaseURL, "development"); err == nil {
		t.Fatal("validateDatabaseSettings accepted identical onboarding and bootstrap tokens")
	}
}
