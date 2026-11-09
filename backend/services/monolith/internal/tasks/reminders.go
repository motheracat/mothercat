package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const (
	reminderInterval = 15 * time.Minute
	reminderTimeout  = 10 * time.Second
)

type ReminderJob struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewReminderJob(db *sql.DB, logger *slog.Logger) *ReminderJob {
	if logger == nil {
		logger = slog.Default()
	}
	return &ReminderJob{db: db, logger: logger}
}

func (j *ReminderJob) Run(ctx context.Context) {
	ticker := time.NewTicker(reminderInterval)
	defer ticker.Stop()
	for {
		if _, err := j.EnqueueDueReminders(ctx); err != nil && ctx.Err() == nil {
			j.logger.ErrorContext(ctx, "enqueue task due reminders", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (j *ReminderJob) EnqueueDueReminders(ctx context.Context) (int64, error) {
	if j == nil || j.db == nil {
		return 0, errors.New("task reminder database is unavailable")
	}
	queryCtx, cancel := context.WithTimeout(ctx, reminderTimeout)
	defer cancel()

	result, err := j.db.ExecContext(queryCtx, `
		INSERT INTO tasks.outbox_events (event_type, task_id, actor_account_id, title, body)
		SELECT reminder.event_type, reminder.task_id, NULL, reminder.title, reminder.body
		FROM (
			SELECT t.id AS task_id, 'task.due_soon'::varchar AS event_type,
			       'Task due soon'::varchar AS title,
			       'A task is due within the next 24 hours.'::varchar AS body
			FROM tasks.tasks t
			WHERE t.due_at > now() AND t.due_at <= now() + interval '24 hours'
			  AND t.status IN ('assigned', 'in_progress', 'changes_requested')
			UNION ALL
			SELECT t.id AS task_id, 'task.overdue'::varchar AS event_type,
			       'Task overdue'::varchar AS title,
			       'A task is past its due date.'::varchar AS body
			FROM tasks.tasks t
			WHERE t.due_at <= now()
			  AND t.status IN ('assigned', 'in_progress', 'changes_requested')
		) reminder
		ON CONFLICT (task_id, event_type)
			WHERE event_type IN ('task.due_soon', 'task.overdue')
		DO NOTHING
	`)
	if err != nil {
		return 0, fmt.Errorf("insert task reminder outbox events: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count task reminder outbox events: %w", err)
	}
	return count, nil
}
