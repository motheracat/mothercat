CREATE TABLE tasks.outbox_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type varchar(80) NOT NULL,
    task_id uuid NOT NULL REFERENCES tasks.tasks(id) ON DELETE CASCADE,
    actor_account_id uuid NOT NULL REFERENCES identity.user_accounts(id),
    recipient_account_id uuid REFERENCES identity.user_accounts(id) ON DELETE CASCADE,
    title varchar(200) NOT NULL,
    body varchar(2000) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    available_at timestamptz NOT NULL DEFAULT now(),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error varchar(1000),
    processed_at timestamptz,
    CHECK (event_type IN (
        'task.assigned', 'task.submitted', 'task.approved', 'task.changes_requested',
        'task.rejected', 'task.comment_added', 'task.due_soon', 'task.overdue'
    ))
);
CREATE INDEX tasks_outbox_pending_idx
    ON tasks.outbox_events (available_at, created_at, id)
    WHERE processed_at IS NULL;
