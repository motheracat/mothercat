package notifications

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const (
	outboxEventID = "00000000-0000-4000-8000-000000000010"
	taskID        = "00000000-0000-4000-8000-000000000020"
	actorID       = "00000000-0000-4000-8000-000000000030"
	recipientID   = "00000000-0000-4000-8000-000000000040"
)

func newMockDispatcher(t *testing.T) (*Dispatcher, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewDispatcher(db, slog.New(slog.NewTextHandler(discardWriter{}, nil))), mock
}

func expectPendingEvent(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT id::text FROM tasks\.outbox_events`).
		WithArgs(outboxBatchSize).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(outboxEventID))
}

func expectLockedEvent(mock sqlmock.Sqlmock, eventType string) {
	mock.ExpectBegin()
	var recipient any
	if eventType == "task.assigned" {
		recipient = recipientID
	}
	mock.ExpectQuery(`SELECT id::text, event_type, task_id::text, actor_account_id::text`).
		WithArgs(outboxEventID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "event_type", "task_id", "actor_account_id", "recipient_account_id", "title", "body",
		}).AddRow(outboxEventID, eventType, taskID, actorID, recipient, "Task update", "Updated"))
}

func TestProcessBatchDeliversAndMarksOutboxEventAtomically(t *testing.T) {
	dispatcher, mock := newMockDispatcher(t)
	expectPendingEvent(mock)
	expectLockedEvent(mock, "task.assigned")
	mock.ExpectExec(`INSERT INTO notifications\.notifications`).
		WithArgs(recipientID, "task.assigned", "Task update", "Updated", taskID, outboxEventID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE tasks\.outbox_events SET processed_at=now\(\), last_error=NULL`).
		WithArgs(outboxEventID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	processed, err := dispatcher.ProcessBatch(context.Background())
	if err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	if processed != 1 {
		t.Fatalf("ProcessBatch() processed %d events, want 1", processed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProcessBatchSchedulesRetryAfterDeliveryFailure(t *testing.T) {
	dispatcher, mock := newMockDispatcher(t)
	expectPendingEvent(mock)
	expectLockedEvent(mock, "task.submitted")
	mock.ExpectQuery(`SELECT created_by::text FROM tasks\.tasks`).
		WithArgs(taskID).
		WillReturnRows(sqlmock.NewRows([]string{"created_by"}).AddRow(recipientID))
	mock.ExpectExec(`INSERT INTO notifications\.notifications`).
		WithArgs(recipientID, "task.submitted", "Task update", "Updated", taskID, outboxEventID).
		WillReturnError(errors.New("temporary database failure"))
	mock.ExpectRollback()
	mock.ExpectExec(`UPDATE tasks\.outbox_events`).
		WithArgs(outboxEventID, "persist notification for recipient: temporary database failure").
		WillReturnResult(sqlmock.NewResult(0, 1))

	processed, err := dispatcher.ProcessBatch(context.Background())
	if err == nil {
		t.Fatal("ProcessBatch() returned nil error after delivery failure")
	}
	if processed != 0 {
		t.Fatalf("ProcessBatch() processed %d events after delivery failure, want 0", processed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRecipientsSupportsEvents(t *testing.T) {
	tests := []struct {
		eventType string
		query     string
	}{
		{"task.assigned", `(?s)SELECT assignee_account_id::text.*task_id=\$1::uuid`},
		{"task.submitted", `(?s)SELECT created_by::text.*tasks\.tasks.*id=\$1::uuid`},
		{"task.comment_added", `(?s)SELECT created_by::text.*UNION.*assignee_account_id::text`},
		{"task.due_soon", `(?s)SELECT created_by::text.*UNION.*assignee_account_id::text`},
	}
	for _, test := range tests {
		t.Run(test.eventType, func(t *testing.T) {
			dispatcher, mock := newMockDispatcher(t)
			mock.ExpectBegin()
			tx, err := dispatcher.db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			mock.ExpectQuery(test.query).
				WithArgs(taskID).
				WillReturnRows(sqlmock.NewRows([]string{"recipient"}).AddRow(recipientID))
			recipients, err := loadRecipients(context.Background(), tx, outboxEvent{
				EventType: test.eventType, TaskID: taskID,
			})
			if err != nil {
				t.Fatalf("loadRecipients() error = %v", err)
			}
			if len(recipients) != 1 || recipients[0] != recipientID {
				t.Fatalf("recipients=%v, want [%s]", recipients, recipientID)
			}
			mock.ExpectRollback()
			_ = tx.Rollback()
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
