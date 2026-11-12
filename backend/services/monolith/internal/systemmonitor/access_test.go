package systemmonitor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestHasAccessUsesSeparateActiveGrantAndAccountState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	accountID := "00000000-0000-4000-8000-000000000001"
	mock.ExpectQuery(`(?s)SELECT EXISTS \(.*system_monitor\.access_grants.*a\.is_active AND a\.suspended_at IS NULL`).
		WithArgs(accountID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	allowed, err := HasAccess(context.Background(), db, accountID)
	if err != nil {
		t.Fatalf("HasAccess() error = %v", err)
	}
	if !allowed {
		t.Fatal("HasAccess() = false, want active independent grant")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGrantAccessRejectsInvalidExpiryWithoutDatabaseAccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	err = GrantAccess(context.Background(), db, GrantRequest{
		AccountID: "00000000-0000-4000-8000-000000000001",
		Operator:  "operator@example.com",
		Reason:    "approved change CHG-123",
		ExpiresAt: time.Now().Add(31 * 24 * time.Hour),
	})
	if !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("GrantAccess() error = %v, want invalid grant", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGrantAccessAuditsWithinGrantTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	expiresAt := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock\(\$1\)`).
		WithArgs(accessLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)INSERT INTO system_monitor\.access_grants.*RETURNING id::text`).
		WithArgs("00000000-0000-4000-8000-000000000001", "operator@example.com", "approved change CHG-123", expiresAt).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("grant-id"))
	mock.ExpectExec(`(?s)INSERT INTO audit\.audit_events`).
		WithArgs(sqlmock.AnyArg(), "operator@example.com", "system_monitor.access.granted",
			"00000000-0000-4000-8000-000000000001", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = GrantAccess(context.Background(), db, GrantRequest{
		AccountID: "00000000-0000-4000-8000-000000000001",
		Operator:  "operator@example.com",
		Reason:    "approved change CHG-123",
		ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("GrantAccess() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGrantAccessRollsBackWhenAuditWriteFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	expiresAt := time.Now().Add(24 * time.Hour)
	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock\(\$1\)`).
		WithArgs(accessLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)INSERT INTO system_monitor\.access_grants.*RETURNING id::text`).
		WithArgs("00000000-0000-4000-8000-000000000001", "operator@example.com", "approved change CHG-123", expiresAt).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("grant-id"))
	mock.ExpectExec(`(?s)INSERT INTO audit\.audit_events`).
		WillReturnError(errors.New("audit unavailable"))
	mock.ExpectRollback()

	err = GrantAccess(context.Background(), db, GrantRequest{
		AccountID: "00000000-0000-4000-8000-000000000001",
		Operator:  "operator@example.com",
		Reason:    "approved change CHG-123",
		ExpiresAt: expiresAt,
	})
	if err == nil {
		t.Fatal("GrantAccess() error = nil, want audit failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRevokeAccessAuditsRevocation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock\(\$1\)`).
		WithArgs(accessLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)UPDATE system_monitor\.access_grants.*RETURNING id::text`).
		WithArgs("00000000-0000-4000-8000-000000000001", "operator@example.com", "access no longer required").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("grant-id"))
	mock.ExpectExec(`(?s)INSERT INTO audit\.audit_events`).
		WithArgs(sqlmock.AnyArg(), "operator@example.com", "system_monitor.access.revoked",
			"00000000-0000-4000-8000-000000000001", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = RevokeAccess(context.Background(), db, RevokeRequest{
		AccountID: "00000000-0000-4000-8000-000000000001",
		Operator:  "operator@example.com",
		Reason:    "access no longer required",
	})
	if err != nil {
		t.Fatalf("RevokeAccess() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
