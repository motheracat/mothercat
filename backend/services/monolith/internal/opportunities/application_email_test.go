package opportunities

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const (
	emailEventID     = "00000000-0000-4000-8000-000000000050"
	emailApplication = "00000000-0000-4000-8000-000000000060"
)

type testApplicationMailer struct {
	err       error
	recipient string
	name      string
	title     string
	status    string
}

func (m *testApplicationMailer) SendApplicationStatus(
	_ context.Context, recipient, name, title, status string,
) error {
	m.recipient, m.name, m.title, m.status = recipient, name, title, status
	return m.err
}

func newTestApplicationEmailDispatcher(t *testing.T, mailer ApplicationStatusMailer) (*ApplicationEmailDispatcher, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewApplicationEmailDispatcher(db, mailer, slog.New(slog.NewTextHandler(io.Discard, nil))), mock
}

func expectPendingApplicationEmail(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT id::text FROM opportunities\.application_email_outbox`).
		WithArgs(applicationEmailBatchSize).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(emailEventID))
}

func expectLockedApplicationEmail(mock sqlmock.Sqlmock, status string) {
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT e\.id::text, e\.status, a\.full_name, a\.email, o\.title.*FOR UPDATE OF e SKIP LOCKED`).
		WithArgs(emailEventID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "status", "full_name", "email", "title",
		}).AddRow(emailEventID, status, "Ada Example", "ada@example.edu", "Training placement"))
}

func TestProcessBatchSendsQueuedApplicationStatusAndMarksItProcessed(t *testing.T) {
	mailer := &testApplicationMailer{}
	dispatcher, mock := newTestApplicationEmailDispatcher(t, mailer)
	expectPendingApplicationEmail(mock)
	expectLockedApplicationEmail(mock, "shortlisted")
	mock.ExpectExec(`UPDATE opportunities\.application_email_outbox`).
		WithArgs(emailEventID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	processed, err := dispatcher.ProcessBatch(context.Background())
	if err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	if processed != 1 {
		t.Fatalf("ProcessBatch() processed %d events, want 1", processed)
	}
	if mailer.recipient != "ada@example.edu" || mailer.name != "Ada Example" ||
		mailer.title != "Training placement" || mailer.status != "shortlisted" {
		t.Fatalf("unexpected email delivery: %+v", mailer)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProcessBatchSchedulesRetryWhenEmailDeliveryFails(t *testing.T) {
	mailer := &testApplicationMailer{err: errors.New("temporary provider failure")}
	dispatcher, mock := newTestApplicationEmailDispatcher(t, mailer)
	expectPendingApplicationEmail(mock)
	expectLockedApplicationEmail(mock, "accepted")
	mock.ExpectRollback()
	mock.ExpectExec(`UPDATE opportunities\.application_email_outbox`).
		WithArgs(emailEventID, "send SIWES status email: temporary provider failure").
		WillReturnResult(sqlmock.NewResult(0, 1))

	processed, err := dispatcher.ProcessBatch(context.Background())
	if err == nil {
		t.Fatal("ProcessBatch() returned nil error after failed email delivery")
	}
	if processed != 0 {
		t.Fatalf("ProcessBatch() processed %d events, want 0", processed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
