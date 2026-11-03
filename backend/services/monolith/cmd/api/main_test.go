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
	handler := newRouter(nil, identity, organization)

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

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil))
	if response.Code != http.StatusTeapot {
		t.Errorf("identity endpoint reached status %d; want identity handler status %d", response.Code, http.StatusTeapot)
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
