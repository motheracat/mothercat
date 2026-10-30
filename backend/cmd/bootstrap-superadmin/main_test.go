package main

import (
	"strings"
	"testing"
	"time"
)

func setBootstrapEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "development")
	t.Setenv("BOOTSTRAP_DATABASE_URL", "postgres://admin:local-test-password@127.0.0.1:5434/mothercat?sslmode=disable")
	t.Setenv("BOOTSTRAP_SECRET", strings.Repeat("a", 64))
	t.Setenv("BOOTSTRAP_SECRET_EXPIRES_AT", time.Now().Add(30*time.Minute).UTC().Format(time.RFC3339))
	t.Setenv("DATABASE_ALLOW_UNVERIFIED_TLS", "")
}

func TestLoadOptionsAcceptsLocalDevelopmentConfiguration(t *testing.T) {
	setBootstrapEnvironment(t)
	cfg, err := loadOptions()
	if err != nil {
		t.Fatalf("loadOptions() error = %v", err)
	}
	if cfg.appEnv != "development" {
		t.Fatalf("appEnv = %q, want development", cfg.appEnv)
	}
}

func TestLoadOptionsRequiresSeparateBootstrapDatabaseURL(t *testing.T) {
	setBootstrapEnvironment(t)
	t.Setenv("BOOTSTRAP_DATABASE_URL", "")
	if _, err := loadOptions(); err == nil {
		t.Fatal("loadOptions() accepted a missing bootstrap database URL")
	}
}

func TestLoadOptionsRequiresShortLivedSecret(t *testing.T) {
	setBootstrapEnvironment(t)
	t.Setenv("BOOTSTRAP_SECRET", "short")
	if _, err := loadOptions(); err == nil {
		t.Fatal("loadOptions() accepted a short bootstrap secret")
	}
}

func TestLoadOptionsRejectsExpiredSecret(t *testing.T) {
	setBootstrapEnvironment(t)
	t.Setenv("BOOTSTRAP_SECRET_EXPIRES_AT", time.Now().Add(-time.Minute).UTC().Format(time.RFC3339))
	if _, err := loadOptions(); err == nil {
		t.Fatal("loadOptions() accepted an expired bootstrap secret")
	}
}

func TestLoadOptionsRejectsUnverifiedProductionTLSWithoutOptIn(t *testing.T) {
	setBootstrapEnvironment(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("BOOTSTRAP_DATABASE_URL", "postgres://admin:secret@db.example.test/mothercat?sslmode=require")
	t.Setenv("DATABASE_ALLOW_UNVERIFIED_TLS", "")
	if _, err := loadOptions(); err == nil {
		t.Fatal("loadOptions() accepted sslmode=require without the explicit production opt-in")
	}
}

func TestLoadOptionsAllowsRenderPrivateTLSOnlyWithOptIn(t *testing.T) {
	setBootstrapEnvironment(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("BOOTSTRAP_DATABASE_URL", "postgres://admin:secret@db.example.test/mothercat?sslmode=require")
	t.Setenv("DATABASE_ALLOW_UNVERIFIED_TLS", "true")
	if _, err := loadOptions(); err != nil {
		t.Fatalf("loadOptions() error = %v", err)
	}
}
