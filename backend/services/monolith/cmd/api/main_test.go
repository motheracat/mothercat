package main

import "testing"

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
