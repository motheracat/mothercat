package notifications

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	outboxBatchSize = 50
	outboxInterval  = 2 * time.Second
	outboxTimeout   = 10 * time.Second
)

type Dispatcher struct {
	db     *sql.DB
	logger *slog.Logger
}

type outboxEvent struct {
	ID        string
	EventType string
	TaskID    string
	ActorID   string
	RecipientID string
	Title     string
	Body      string
}

func NewDispatcher(db *sql.DB, logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{db: db, logger: logger}
}

func (d *Dispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(outboxInterval)
	defer ticker.Stop()
	for {
		if _, err := d.ProcessBatch(ctx); err != nil && ctx.Err() == nil {
			d.logger.ErrorContext(ctx, "process notification outbox batch", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (d *Dispatcher) ProcessBatch(ctx context.Context) (int, error) {
	if d == nil || d.db == nil {
		return 0, errors.New("notification outbox database is unavailable")
	}
	queryCtx, cancelQuery := context.WithTimeout(ctx, outboxTimeout)
	rows, err := d.db.QueryContext(queryCtx, `
		SELECT id::text FROM tasks.outbox_events
		WHERE processed_at IS NULL AND available_at <= now()
		ORDER BY created_at, id
		LIMIT $1
	`, outboxBatchSize)
	if err != nil {
		cancelQuery()
		return 0, fmt.Errorf("select pending notification events: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			cancelQuery()
			return 0, fmt.Errorf("scan pending notification event: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		cancelQuery()
		return 0, fmt.Errorf("read pending notification events: %w", err)
	}
	if err := rows.Close(); err != nil {
		cancelQuery()
		return 0, fmt.Errorf("close pending notification events: %w", err)
	}
	cancelQuery()

	processed := 0
	var failures []error
	for _, id := range ids {
		eventCtx, cancelEvent := context.WithTimeout(ctx, outboxTimeout)
		ok, err := d.processOne(eventCtx, id)
		cancelEvent()
		if err != nil {
			if retryErr := d.recordFailure(ctx, id, err); retryErr != nil {
				failures = append(failures, fmt.Errorf("record retry for event %s: %w", id, retryErr))
			}
			failures = append(failures, fmt.Errorf("deliver event %s: %w", id, err))
			continue
		}
		if ok {
			processed++
		}
	}
	return processed, errors.Join(failures...)
}

func (d *Dispatcher) processOne(ctx context.Context, id string) (bool, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin notification delivery: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var event outboxEvent
	var recipientID sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT id::text, event_type, task_id::text, actor_account_id::text,
		       recipient_account_id::text, title, body
		FROM tasks.outbox_events
		WHERE id=$1::uuid AND processed_at IS NULL AND available_at <= now()
		FOR UPDATE SKIP LOCKED
	`, id).Scan(&event.ID, &event.EventType, &event.TaskID, &event.ActorID,
		&recipientID, &event.Title, &event.Body)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("finish skipped notification event: %w", err)
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock notification event: %w", err)
	}
	if recipientID.Valid {
		event.RecipientID = recipientID.String
	}

	recipients, err := loadRecipients(ctx, tx, event)
	if err != nil {
		return false, err
	}
	for _, recipient := range recipients {
		if recipient == event.ActorID {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO notifications.notifications (
				recipient_account_id, event_type, title, body, resource_type, resource_id, source_event_id
			)
			SELECT $1::uuid, $2, $3, $4, 'task', $5::uuid, $6::uuid
			WHERE coalesce((
				SELECT enabled FROM notifications.notification_preferences
				WHERE account_id=$1::uuid AND event_type=$2
			), true)
			ON CONFLICT (source_event_id, recipient_account_id) DO NOTHING
		`, recipient, event.EventType, event.Title, event.Body, event.TaskID, event.ID); err != nil {
			return false, fmt.Errorf("persist notification for recipient: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE tasks.outbox_events SET processed_at=now(), last_error=NULL
		WHERE id=$1::uuid AND processed_at IS NULL
	`, event.ID); err != nil {
		return false, fmt.Errorf("mark notification event processed: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit notification delivery: %w", err)
	}
	return true, nil
}

func loadRecipients(ctx context.Context, tx *sql.Tx, event outboxEvent) ([]string, error) {
	if event.RecipientID != "" {
		if event.EventType != "task.assigned" {
			return nil, fmt.Errorf("unexpected explicit recipient for event %q", event.EventType)
		}
		return []string{event.RecipientID}, nil
	}
	var query string
	switch event.EventType {
	case "task.assigned", "task.approved", "task.changes_requested", "task.rejected":
		query = `
			SELECT assignee_account_id::text FROM tasks.task_assignments
			WHERE task_id=$1::uuid AND unassigned_at IS NULL
		`
	case "task.submitted":
		query = `SELECT created_by::text FROM tasks.tasks WHERE id=$1::uuid`
	case "task.comment_added", "task.due_soon", "task.overdue":
		query = `
			SELECT created_by::text FROM tasks.tasks WHERE id=$1::uuid
			UNION
			SELECT assignee_account_id::text FROM tasks.task_assignments
			WHERE task_id=$1::uuid AND unassigned_at IS NULL
		`
	default:
		return nil, fmt.Errorf("unsupported notification event type %q", event.EventType)
	}
	rows, err := tx.QueryContext(ctx, query, event.TaskID)
	if err != nil {
		return nil, fmt.Errorf("find notification recipients: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var recipients []string
	for rows.Next() {
		var recipient string
		if err := rows.Scan(&recipient); err != nil {
			return nil, fmt.Errorf("scan notification recipient: %w", err)
		}
		recipients = append(recipients, recipient)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read notification recipients: %w", err)
	}
	return recipients, nil
}

func (d *Dispatcher) recordFailure(ctx context.Context, id string, deliveryErr error) error {
	message := strings.TrimSpace(deliveryErr.Error())
	if runes := []rune(message); len(runes) > 1000 {
		message = string(runes[:1000])
	}
	retryCtx, cancel := context.WithTimeout(ctx, outboxTimeout)
	defer cancel()
	_, err := d.db.ExecContext(retryCtx, `
		UPDATE tasks.outbox_events
		SET attempts=attempts+1,
		    available_at=now() + make_interval(secs =>
		        LEAST(3600, 5 * power(2, LEAST(attempts, 10)))::integer
		    ),
		    last_error=$2
		WHERE id=$1::uuid AND processed_at IS NULL
	`, id, message)
	if err != nil {
		return fmt.Errorf("update failed notification event: %w", err)
	}
	return nil
}
