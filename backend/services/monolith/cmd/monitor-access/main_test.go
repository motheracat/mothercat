package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mothercat-enterprise/mothercat/backend/services/identity/monolith/internal/systemmonitor"
)

func TestParseGrantRequest(t *testing.T) {
	request, err := parseRequest([]string{
		"grant",
		"--account-id", "00000000-0000-4000-8000-000000000001",
		"--operator", "operator@example.com",
		"--reason", "approved change CHG-123",
		"--expires-at", "2026-11-20T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	grant, ok := request.(systemmonitor.GrantRequest)
	if !ok {
		t.Fatalf("parseRequest() returned %T, want GrantRequest", request)
	}
	if grant.AccountID != "00000000-0000-4000-8000-000000000001" ||
		grant.Operator != "operator@example.com" || grant.Reason != "approved change CHG-123" ||
		!grant.ExpiresAt.Equal(time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("parseRequest() returned unexpected grant: %#v", grant)
	}
}

func TestParseRevokeRejectsExpiry(t *testing.T) {
	_, err := parseRequest([]string{
		"revoke",
		"--account-id", "00000000-0000-4000-8000-000000000001",
		"--operator", "operator@example.com",
		"--reason", "access no longer required",
		"--expires-at", "2026-11-20T12:00:00Z",
	})
	if err == nil {
		t.Fatal("parseRequest() error = nil, want expiry rejection")
	}
}

func TestValidateDatabaseURLRequiresTLS(t *testing.T) {
	if err := validateDatabaseURL("postgresql://operator@db.example.test:5432/mothercat?sslmode=verify-full"); err != nil {
		t.Fatalf("validateDatabaseURL() error = %v", err)
	}
	if err := validateDatabaseURL("postgresql://operator@db.example.test:5432/mothercat?sslmode=disable"); err == nil {
		t.Fatal("validateDatabaseURL() error = nil, want TLS validation error")
	}
	if err := validateDatabaseURL(""); err == nil ||
		err.Error() != "SYSTEM_MONITOR_AUTHORIZATION_DATABASE_URL is required" {
		t.Fatal("validateDatabaseURL() should require the dedicated authorization database URL")
	}
}

func TestResolveAuthorizationDatabaseURLPrefersProcessEnvironment(t *testing.T) {
	root := createBackendEnv(t, "SYSTEM_MONITOR_AUTHORIZATION_DATABASE_URL=file-dedicated\nDATABASE_URL=file-fallback\n")
	lookup := mapLookup(map[string]string{"DATABASE_URL": " shell-database-url "})

	value, err := resolveAuthorizationDatabaseURL(lookup, root)
	if err != nil {
		t.Fatalf("resolveAuthorizationDatabaseURL() error = %v", err)
	}
	if value != "shell-database-url" {
		t.Fatalf("resolved database URL = %q, want process environment value", value)
	}
}

func TestResolveAuthorizationDatabaseURLLoadsDedicatedSettingFromBackendEnv(t *testing.T) {
	root := createBackendEnv(t, "DATABASE_URL=file-fallback\nSYSTEM_MONITOR_AUTHORIZATION_DATABASE_URL='file-dedicated'\n")
	lookup := mapLookup(nil)

	value, err := resolveAuthorizationDatabaseURL(lookup, filepath.Join(root, "backend", "services", "monolith"))
	if err != nil {
		t.Fatalf("resolveAuthorizationDatabaseURL() error = %v", err)
	}
	if value != "file-dedicated" {
		t.Fatalf("resolved database URL = %q, want dedicated backend/.env value", value)
	}
}

func TestResolveAuthorizationDatabaseURLFallsBackToDatabaseURLInBackendEnv(t *testing.T) {
	root := createBackendEnv(t, "export DATABASE_URL=\"file-database-url\" # local database\n")
	lookup := mapLookup(nil)

	value, err := resolveAuthorizationDatabaseURL(lookup, root)
	if err != nil {
		t.Fatalf("resolveAuthorizationDatabaseURL() error = %v", err)
	}
	if value != "file-database-url" {
		t.Fatalf("resolved database URL = %q, want DATABASE_URL fallback", value)
	}
}

func TestParseEnvValueAcceptsQuotedComments(t *testing.T) {
	for _, input := range []string{`"value with spaces" # comment`, `'value with spaces' # comment`} {
		value, err := parseEnvValue(input)
		if err != nil {
			t.Fatalf("parseEnvValue(%q) error = %v", input, err)
		}
		if value != "value with spaces" {
			t.Fatalf("parseEnvValue(%q) = %q, want quoted value", input, value)
		}
	}
}

func TestResolveAuthorizationDatabaseURLExplainsMissingConfiguration(t *testing.T) {
	_, err := resolveAuthorizationDatabaseURL(mapLookup(nil), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "backend/.env") {
		t.Fatalf("resolveAuthorizationDatabaseURL() error = %v, want actionable missing configuration error", err)
	}
}

func TestReadEnvFileDoesNotIncludeSecretValueInSyntaxErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	secret := "do-not-print-this"
	if err := os.WriteFile(path, []byte("DATABASE_URL="+secret+"\ninvalid line with "+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := readEnvFile(path)
	if err == nil {
		t.Fatal("readEnvFile() error = nil, want malformed assignment error")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("readEnvFile() returned unexpected missing-file error: %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("readEnvFile() included a secret value in the error")
	}
}

func createBackendEnv(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "backend", ".env")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
