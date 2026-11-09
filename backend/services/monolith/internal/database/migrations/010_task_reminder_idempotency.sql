ALTER TABLE tasks.outbox_events
    ALTER COLUMN actor_account_id DROP NOT NULL;

CREATE UNIQUE INDEX tasks_outbox_one_reminder_per_condition_idx
    ON tasks.outbox_events (task_id, event_type)
    WHERE event_type IN ('task.due_soon', 'task.overdue');
