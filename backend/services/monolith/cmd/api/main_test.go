package main

import "testing"

func TestValidateMongoSettings(t *testing.T) {
	t.Setenv("IDENTITY_ONBOARDING_TOKEN", "a-long-enough-test-onboarding-token")
	t.Setenv("IDENTITY_BOOTSTRAP_TOKEN", "")

	tests := []struct {
		name     string
		uri      string
		database string
		appEnv   string
		wantErr  bool
	}{
		{
			name:     "development permits local non-TLS MongoDB",
			uri:      "mongodb://localhost:27017",
			database: "mothercat",
			appEnv:   "development",
		},
		{
			name:     "production requires TLS",
			uri:      "mongodb://localhost:27017/?tls=true",
			database: "mothercat",
			appEnv:   "production",
		},
		{
			name:     "production rejects missing TLS",
			uri:      "mongodb://localhost:27017",
			database: "mothercat",
			appEnv:   "production",
			wantErr:  true,
		},
		{
			name:     "production rejects disabled TLS",
			uri:      "mongodb://localhost:27017/?tls=false",
			database: "mothercat",
			appEnv:   "production",
			wantErr:  true,
		},
		{
			name:     "production rejects insecure certificate verification",
			uri:      "mongodb://localhost:27017/?tls=true&tlsInsecure=true",
			database: "mothercat",
			appEnv:   "production",
			wantErr:  true,
		},
		{
			name:     "staging requires TLS",
			uri:      "mongodb://localhost:27017",
			database: "mothercat",
			appEnv:   "staging",
			wantErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateMongoSettings(test.uri, test.database, test.appEnv)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateMongoSettings() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestValidateMongoSettingsValidatesOptionalBootstrapToken(t *testing.T) {
	t.Setenv("IDENTITY_ONBOARDING_TOKEN", "a-long-enough-test-onboarding-token")
	t.Setenv("IDENTITY_BOOTSTRAP_TOKEN", "short")
	if err := validateMongoSettings("mongodb://localhost:27017", "mothercat", "development"); err == nil {
		t.Fatal("validateMongoSettings accepted a short bootstrap token")
	}
	t.Setenv("IDENTITY_BOOTSTRAP_TOKEN", "a-long-enough-test-bootstrap-token")
	if err := validateMongoSettings("mongodb://localhost:27017", "mothercat", "development"); err != nil {
		t.Fatalf("validateMongoSettings rejected a strong bootstrap token: %v", err)
	}
	t.Setenv("IDENTITY_BOOTSTRAP_TOKEN", "a-long-enough-test-onboarding-token")
	if err := validateMongoSettings("mongodb://localhost:27017", "mothercat", "development"); err == nil {
		t.Fatal("validateMongoSettings accepted identical onboarding and bootstrap tokens")
	}
}
