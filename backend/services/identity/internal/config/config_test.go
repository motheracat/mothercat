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
	t.Setenv("PORT", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DATABASE_SSLMODE", "")
	t.Setenv("DATABASE_ALLOW_UNVERIFIED_TLS", "")
	t.Setenv("POSTGRES_HOST", "127.0.0.1")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_DB", "identity_test")
	t.Setenv("POSTGRES_USER", "identity_test")
	t.Setenv("POSTGRES_PASSWORD", "local-only-test-password")
	t.Setenv("POSTGRES_SSLMODE", "disable")
	t.Setenv("ACCESS_SERVICE_URL", "https://access.example.test")
	t.Setenv("ACCESS_SERVICE_TOKEN", strings.Repeat("a", 32))
	t.Setenv("ACCESS_SERVICE_TIMEOUT", "3s")
	t.Setenv("IDENTITY_ONBOARDING_TOKEN", strings.Repeat("b", 32))
	t.Setenv("SMTP_HOST", "smtp.example.test")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "identity@example.test")
	t.Setenv("SMTP_PASSWORD", "local-only-smtp-password")
	t.Setenv("SMTP_FROM", "identity@example.test")
	t.Setenv("PUBLIC_BASE_URL", "https://app.example.test")
	t.Setenv("SMTP_TIMEOUT", "10s")
	t.Setenv("DATABASE_MAX_CONNS", "10")
	t.Setenv("DB_CONNECT_TIMEOUT", "10s")
	t.Setenv("HEALTH_CHECK_TIMEOUT", "2s")
	t.Setenv("SHUTDOWN_TIMEOUT", "10s")
}

func setProductionDatabaseEnvironment(t *testing.T) {
	t.Helper()
	setLocalDatabaseEnvironment(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgresql://identity:local-test-password@db.example.test/identity?sslmode=verify-full")
}

func TestLoadUsesRenderPortWhenProvided(t *testing.T) {
	setLocalDatabaseEnvironment(t)
	t.Setenv("HTTP_ADDR", ":8080")
	t.Setenv("PORT", "10000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != ":10000" {
		t.Fatalf("HTTPAddr = %q, want :10000", cfg.HTTPAddr)
	}
}

func TestLoadRejectsInvalidRenderPort(t *testing.T) {
	setLocalDatabaseEnvironment(t)
	t.Setenv("PORT", "not-a-port")

	if _, err := Load(); err == nil {
		t.Fatal("Load() succeeded with an invalid Render PORT; want an error")
	}
}

func TestLoadAllowsRenderStagingTLSWithoutURLQuery(t *testing.T) {
	setLocalDatabaseEnvironment(t)
	t.Setenv("APP_ENV", "staging")
	t.Setenv("DATABASE_URL", "postgres://identity:secret@db.example.test/identity")
	t.Setenv("DATABASE_SSLMODE", "require")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	parsed, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("parse configured database URL: %v", err)
	}
	if got := parsed.Query().Get("sslmode"); got != "require" {
		t.Fatalf("sslmode = %q, want require", got)
	}
}

func TestLoadRejectsPlaintextStagingDatabaseURL(t *testing.T) {
	setLocalDatabaseEnvironment(t)
	t.Setenv("APP_ENV", "staging")
	t.Setenv("DATABASE_URL", "postgres://identity:secret@db.example.test/identity?sslmode=disable")

	if _, err := Load(); err == nil {
		t.Fatal("Load() succeeded with plaintext staging database URL; want an error")
	}
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
	t.Setenv("DATABASE_URL", "postgres://identity:unit-test-password@db.example.test/identity?sslmode=require")

	if _, err := Load(); err == nil {
		t.Fatal("Load() succeeded with production sslmode=require; want an error")
	}
}

func TestLoadAllowsRenderPrivateNetworkTLSInProductionWhenExplicitlyEnabled(t *testing.T) {
	setLocalDatabaseEnvironment(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://identity:secret@db.example.test/identity?sslmode=require")
	t.Setenv("DATABASE_ALLOW_UNVERIFIED_TLS", "true")

	if _, err := Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRejectsUnverifiedTLSOptInWithVerifiedMode(t *testing.T) {
	setLocalDatabaseEnvironment(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://identity:unit-test-password@db.example.test/identity?sslmode=verify-full")
	t.Setenv("DATABASE_ALLOW_UNVERIFIED_TLS", "true")

	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted DATABASE_ALLOW_UNVERIFIED_TLS with sslmode=verify-full")
	}
}

func TestLoadRejectsInvalidUnverifiedTLSSetting(t *testing.T) {
	setLocalDatabaseEnvironment(t)
	t.Setenv("DATABASE_ALLOW_UNVERIFIED_TLS", "sometimes")

	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted an invalid DATABASE_ALLOW_UNVERIFIED_TLS value")
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

func TestLoadRequiresAccessInProduction(t *testing.T) {
	setProductionDatabaseEnvironment(t)
	t.Setenv("ACCESS_SERVICE_URL", "")
	t.Setenv("ACCESS_SERVICE_TOKEN", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() succeeded without Access service configuration")
	}
}

func TestLoadRequiresSMTPInProduction(t *testing.T) {
	setProductionDatabaseEnvironment(t)
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_PORT", "")
	t.Setenv("SMTP_USERNAME", "")
	t.Setenv("SMTP_PASSWORD", "")
	t.Setenv("SMTP_FROM", "")
	t.Setenv("PUBLIC_BASE_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() succeeded without SMTP configuration")
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
	t.Setenv("DATABASE_SSLMODE", "")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() succeeded without an explicit sslmode; want an error")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("configuration error leaked a password: %q", err)
	}
}
