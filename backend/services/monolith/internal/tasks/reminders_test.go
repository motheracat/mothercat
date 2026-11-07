package tasks

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

type reminderDiscardWriter struct{}

func (reminderDiscardWriter) Write(data []byte) (int, error) { return len(data), nil }

func TestEnqueueDueRemindersInsertsIdempotently(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectExec(`INSERT INTO tasks\.outbox_events`).
		WillReturnResult(sqlmock.NewResult(0, 3))
	job := NewReminderJob(db, slog.New(slog.NewTextHandler(reminderDiscardWriter{}, nil)))
	count, err := job.EnqueueDueReminders(context.Background())
	if err != nil {
		t.Fatalf("EnqueueDueReminders() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("EnqueueDueReminders() count = %d, want 3", count)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEnqueueDueRemindersReportsDatabaseError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectExec(`INSERT INTO tasks\.outbox_events`).WillReturnError(errors.New("database unavailable"))
	job := NewReminderJob(db, slog.New(slog.NewTextHandler(reminderDiscardWriter{}, nil)))
	if _, err := job.EnqueueDueReminders(context.Background()); err == nil {
		t.Fatal("EnqueueDueReminders() returned nil error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEnqueueDueRemindersRejectsNilJob(t *testing.T) {
	var job *ReminderJob
	if _, err := job.EnqueueDueReminders(context.Background()); err == nil {
		t.Fatal("EnqueueDueReminders() returned nil error for nil job")
	}
}
