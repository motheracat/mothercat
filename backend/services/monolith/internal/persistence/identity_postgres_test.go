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
