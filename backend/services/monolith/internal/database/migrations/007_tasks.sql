CREATE SCHEMA IF NOT EXISTS tasks;

CREATE TABLE tasks.tasks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title varchar(200) NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 20000),
    priority varchar(16) NOT NULL DEFAULT 'normal'
        CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
    status varchar(24) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'assigned', 'in_progress', 'submitted',
                          'changes_requested', 'approved', 'rejected', 'cancelled')),
    created_by uuid NOT NULL REFERENCES identity.user_accounts(id),
    department_id uuid NOT NULL REFERENCES organization.departments(id),
    due_at timestamptz,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tasks_department_status_due_idx
    ON tasks.tasks (department_id, status, due_at);
CREATE INDEX tasks_creator_created_idx
    ON tasks.tasks (created_by, created_at DESC, id);

CREATE TABLE tasks.task_assignments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id uuid NOT NULL REFERENCES tasks.tasks(id) ON DELETE CASCADE,
    assignee_account_id uuid NOT NULL REFERENCES identity.user_accounts(id),
    assigned_by uuid NOT NULL REFERENCES identity.user_accounts(id),
    assigned_at timestamptz NOT NULL DEFAULT now(),
    unassigned_at timestamptz
);
CREATE UNIQUE INDEX tasks_one_active_assignee_idx
    ON tasks.task_assignments (task_id) WHERE unassigned_at IS NULL;
CREATE INDEX tasks_assignments_account_idx
    ON tasks.task_assignments (assignee_account_id, task_id) WHERE unassigned_at IS NULL;

CREATE TABLE tasks.task_submissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id uuid NOT NULL REFERENCES tasks.tasks(id) ON DELETE CASCADE,
    submission_number integer NOT NULL CHECK (submission_number > 0),
    summary text NOT NULL,
    links jsonb NOT NULL DEFAULT '[]'::jsonb,
    submitter_account_id uuid NOT NULL REFERENCES identity.user_accounts(id),
    submitted_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (task_id, submission_number)
);

CREATE TABLE tasks.task_reviews (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id uuid NOT NULL REFERENCES tasks.tasks(id) ON DELETE CASCADE,
    submission_id uuid NOT NULL REFERENCES tasks.task_submissions(id) ON DELETE CASCADE,
    reviewer_account_id uuid NOT NULL REFERENCES identity.user_accounts(id),
    decision varchar(24) NOT NULL
        CHECK (decision IN ('approved', 'changes_requested', 'rejected')),
    feedback text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tasks.task_comments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id uuid NOT NULL REFERENCES tasks.tasks(id) ON DELETE CASCADE,
    author_account_id uuid NOT NULL REFERENCES identity.user_accounts(id),
    body varchar(5000) NOT NULL CHECK (length(btrim(body)) BETWEEN 1 AND 5000),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tasks_comments_task_time_idx
    ON tasks.task_comments (task_id, created_at, id);

CREATE TABLE tasks.task_status_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id uuid NOT NULL REFERENCES tasks.tasks(id) ON DELETE CASCADE,
    actor_account_id uuid NOT NULL REFERENCES identity.user_accounts(id),
    from_status varchar(24),
    to_status varchar(24) NOT NULL,
    reason varchar(2000) NOT NULL DEFAULT '',
    occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tasks_history_task_time_idx
    ON tasks.task_status_history (task_id, occurred_at, id);

INSERT INTO access.permissions (permission_key, description)
VALUES
    ('tasks.create', 'Create tasks'),
    ('tasks.assign', 'Assign tasks to active staff in the task department'),
    ('tasks.review', 'Review submitted tasks'),
    ('tasks.cancel', 'Cancel tasks'),
    ('tasks.read_department', 'Read tasks in departments the user belongs to'),
    ('tasks.read_all', 'Read all tasks')
ON CONFLICT (permission_key) DO NOTHING;

INSERT INTO access.role_permissions (role_key, permission_key)
SELECT role_key, permission_key
FROM access.roles CROSS JOIN access.permissions
WHERE role_key IN ('admin', 'super_admin')
  AND permission_key LIKE 'tasks.%'
ON CONFLICT DO NOTHING;
