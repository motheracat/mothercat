package config

import (
	"net/url"
	"strings"
	"testing"
)

func setLocalDatabaseEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "development")
	t.Setenv("HTTP_ADDR", ":8080")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("POSTGRES_HOST", "127.0.0.1")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_DB", "identity_test")
	t.Setenv("POSTGRES_USER", "identity_test")
	t.Setenv("POSTGRES_PASSWORD", "local-only-test-password")
	t.Setenv("POSTGRES_SSLMODE", "disable")
	t.Setenv("DATABASE_MAX_CONNS", "10")
	t.Setenv("DB_CONNECT_TIMEOUT", "10s")
	t.Setenv("HEALTH_CHECK_TIMEOUT", "2s")
	t.Setenv("SHUTDOWN_TIMEOUT", "10s")
}

func TestLoadBuildsLocalDatabaseURL(t *testing.T) {
	setLocalDatabaseEnvironment(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	parsed, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("parse generated database URL: %v", err)
	}
	if got := parsed.Query().Get("sslmode"); got != "disable" {
		t.Fatalf("sslmode = %q, want disable", got)
	}
	if got := parsed.Path; got != "/identity_test" {
		t.Fatalf("database path = %q, want /identity_test", got)
	}
	if cfg.DatabaseMaxConns != 10 {
		t.Fatalf("DatabaseMaxConns = %d, want 10", cfg.DatabaseMaxConns)
	}
}

func TestLoadRequiresVerifiedTLSInProduction(t *testing.T) {
	setLocalDatabaseEnvironment(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://identity:secret@db.example.test/identity?sslmode=require")

	if _, err := Load(); err == nil {
		t.Fatal("Load() succeeded with production sslmode=require; want an error")
	}
}

func TestLoadRejectsDuplicateTLSMode(t *testing.T) {
	setLocalDatabaseEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://identity:secret@db.example.test/identity?sslmode=verify-full&sslmode=disable")
	t.Setenv("APP_ENV", "production")

	if _, err := Load(); err == nil {
		t.Fatal("Load() succeeded with duplicate sslmode parameters; want an error")
	}
}

func TestLoadRejectsInvalidDatabasePoolSize(t *testing.T) {
	setLocalDatabaseEnvironment(t)
	t.Setenv("DATABASE_MAX_CONNS", "0")

	if _, err := Load(); err == nil {
		t.Fatal("Load() succeeded with DATABASE_MAX_CONNS=0; want an error")
	}
}

func TestLoadRejectsInvalidHTTPAddress(t *testing.T) {
	setLocalDatabaseEnvironment(t)
	t.Setenv("HTTP_ADDR", "0.0.0.0:not-a-port")

	if _, err := Load(); err == nil {
		t.Fatal("Load() succeeded with an invalid HTTP_ADDR; want an error")
	}
}

func TestLoadDoesNotIncludeDatabasePasswordInErrors(t *testing.T) {
	setLocalDatabaseEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://identity:secret@db.example.test/identity")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() succeeded without an explicit sslmode; want an error")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("configuration error leaked a password: %q", err)
	}
}