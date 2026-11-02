package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	app "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/application"
)

func TestCompleteRecoveryRejectsAccountUpdateThatAffectsNoRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const accountID = "00000000-0000-0000-0000-000000000001"
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE identity\.recovery_tokens`).
		WithArgs("token-hash", "activation").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(accountID))
	mock.ExpectExec(`UPDATE identity\.user_accounts`).
		WithArgs(accountID, "new-password-hash").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	repository := NewIdentityRepository(db)
	_, err = repository.CompleteRecovery(context.Background(), "token-hash", "activation", "new-password-hash", true)
	if !errors.Is(err, app.ErrInvalidRecoveryToken) {
		t.Fatalf("CompleteRecovery() error = %v, want %v", err, app.ErrInvalidRecoveryToken)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteRecoveryRejectsExpiredOrAlreadyUsedTokens(t *testing.T) {
	for _, test := range []struct {
		name string
	}{
		{name: "expired"},
		{name: "already used"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			mock.ExpectBegin()
			mock.ExpectQuery(`(?s)UPDATE identity\.recovery_tokens.*used_at IS NULL.*expires_at > now\(\)`).
				WithArgs("token-hash", "password_reset").
				WillReturnRows(sqlmock.NewRows([]string{"account_id"}))
			mock.ExpectRollback()

			_, err = NewIdentityRepository(db).CompleteRecovery(
				context.Background(), "token-hash", "password_reset", "new-password-hash", false,
			)
			if !errors.Is(err, app.ErrInvalidRecoveryToken) {
				t.Fatalf("CompleteRecovery() error = %v, want invalid recovery token", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFindBySessionTokenRejectsExpiredRevokedOrSuspendedAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`(?s)SELECT a\.id.*s\.revoked_at IS NULL.*s\.expires_at > now\(\).*a\.suspended_at IS NULL`).
		WithArgs(hashToken("session-token")).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "staff_id", "email", "display_name", "password_hash", "is_active", "must_change_password",
		}))
	_, err = NewIdentityRepository(db).FindBySessionToken(context.Background(), "session-token")
	if !errors.Is(err, app.ErrSessionNotFound) {
		t.Fatalf("FindBySessionToken() error = %v, want session not found", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSuspendAccountPreventsSuspendingLastActiveSuperAdmin(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const accountID = "00000000-0000-0000-0000-000000000001"
	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock\(761039123\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT a\.id.*FOR UPDATE`).
		WithArgs("ADMIN-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_active", "suspended", "super_admin"}).
			AddRow(accountID, true, false, true))
	mock.ExpectQuery(`(?s)SELECT count\(\*\).*super_admin`).
		WithArgs(accountID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()

	err = NewIdentityRepository(db).SuspendAccount(context.Background(), "ADMIN-1", accountID)
	if !errors.Is(err, app.ErrLastActiveSuperAdmin) {
		t.Fatalf("SuspendAccount() error = %v, want last active SuperAdmin protection", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSuspendAccountRevokesSessionsAndAuditsAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const accountID = "00000000-0000-0000-0000-000000000001"
	const actorID = "00000000-0000-0000-0000-000000000002"
	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock\(761039123\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT a\.id.*FOR UPDATE`).
		WithArgs("STAFF-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_active", "suspended", "super_admin"}).
			AddRow(accountID, true, false, false))
	mock.ExpectExec(`UPDATE identity\.user_accounts`).
		WithArgs(accountID, actorID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE identity\.sessions`).
		WithArgs(accountID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE identity\.recovery_tokens`).
		WithArgs(accountID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO audit\.audit_events`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), actorID, accountID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewIdentityRepository(db).SuspendAccount(context.Background(), "STAFF-1", actorID); err != nil {
		t.Fatalf("SuspendAccount() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSuspendInactiveAccountInvalidatesActivationTokens(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const accountID = "00000000-0000-0000-0000-000000000001"
	const actorID = "00000000-0000-0000-0000-000000000002"
	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock\(761039123\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT a\.id.*FOR UPDATE`).
		WithArgs("STAFF-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_active", "suspended", "super_admin"}).
			AddRow(accountID, false, false, false))
	mock.ExpectExec(`UPDATE identity\.user_accounts`).
		WithArgs(accountID, actorID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE identity\.sessions`).
		WithArgs(accountID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE identity\.recovery_tokens`).
		WithArgs(accountID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO audit\.audit_events`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), actorID, accountID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewIdentityRepository(db).SuspendAccount(context.Background(), "STAFF-1", actorID); err != nil {
		t.Fatalf("SuspendAccount() for inactive account = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReactivateAccountAuditsStateChangeAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const accountID = "00000000-0000-0000-0000-000000000001"
	const actorID = "00000000-0000-0000-0000-000000000002"
	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock\(761039123\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT id, is_active, suspended_at IS NOT NULL.*FOR UPDATE`).
		WithArgs("STAFF-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_active", "suspended"}).
			AddRow(accountID, false, true))
	mock.ExpectExec(`UPDATE identity\.user_accounts`).
		WithArgs(accountID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO audit\.audit_events`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), actorID, accountID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewIdentityRepository(db).ReactivateAccount(context.Background(), "STAFF-1", actorID); err != nil {
		t.Fatalf("ReactivateAccount() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
