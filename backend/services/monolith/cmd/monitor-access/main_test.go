package main

import (
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
