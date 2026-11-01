CREATE SCHEMA IF NOT EXISTS identity;
CREATE SCHEMA IF NOT EXISTS organization;
CREATE SCHEMA IF NOT EXISTS access;
CREATE SCHEMA IF NOT EXISTS audit;

CREATE TABLE IF NOT EXISTS identity.user_accounts (
    id uuid PRIMARY KEY,
    staff_id varchar(80) NOT NULL UNIQUE,
    email varchar(254) NOT NULL UNIQUE,
    display_name varchar(160) NOT NULL,
    password_hash text NOT NULL,
    is_active boolean NOT NULL DEFAULT false,
    must_change_password boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS identity.sessions (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES identity.user_accounts(id) ON DELETE CASCADE,
    token_hash char(64) NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sessions_account_expiry_idx
    ON identity.sessions (account_id, expires_at);

CREATE TABLE IF NOT EXISTS identity.recovery_tokens (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES identity.user_accounts(id) ON DELETE CASCADE,
    token_hash char(64) NOT NULL UNIQUE,
    purpose varchar(32) NOT NULL CHECK (purpose IN ('activation', 'password_reset')),
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS recovery_tokens_account_purpose_expiry_idx
    ON identity.recovery_tokens (account_id, purpose, expires_at);

CREATE TABLE IF NOT EXISTS identity.authentication_events (
    id uuid PRIMARY KEY,
    account_id uuid,
    event_type varchar(64) NOT NULL,
    staff_id varchar(80) NOT NULL,
    successful boolean NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS authentication_events_account_time_idx
    ON identity.authentication_events (account_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS authentication_events_staff_time_idx
    ON identity.authentication_events (staff_id, occurred_at DESC);

CREATE TABLE IF NOT EXISTS identity.bootstrap_state (
    key text PRIMARY KEY,
    state text NOT NULL,
    account_id uuid,
    completed_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS organization.departments (
    id uuid PRIMARY KEY,
    name varchar(160) NOT NULL UNIQUE,
    description text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS organization.staff_members (
    id uuid PRIMARY KEY,
    staff_id varchar(80) NOT NULL UNIQUE,
    account_id uuid UNIQUE REFERENCES identity.user_accounts(id) ON DELETE SET NULL,
    department_id uuid REFERENCES organization.departments(id) ON DELETE SET NULL,
    full_name varchar(200) NOT NULL,
    work_email varchar(254),
    job_title varchar(150),
    employment_status varchar(24) NOT NULL DEFAULT 'pending'
        CHECK (employment_status IN ('pending', 'active', 'inactive', 'deactivated')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS staff_members_department_idx
    ON organization.staff_members (department_id);

CREATE TABLE IF NOT EXISTS organization.department_memberships (
    department_id uuid NOT NULL REFERENCES organization.departments(id) ON DELETE CASCADE,
    account_id uuid NOT NULL REFERENCES identity.user_accounts(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (department_id, account_id)
);

CREATE TABLE IF NOT EXISTS organization.staff_import_batches (
    id uuid PRIMARY KEY,
    status varchar(24) NOT NULL,
    created_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    result jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS access.roles (
    role_key varchar(100) PRIMARY KEY,
    display_name varchar(100) NOT NULL UNIQUE,
    description text NOT NULL DEFAULT '',
    is_system_role boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS access.permissions (
    permission_key varchar(150) PRIMARY KEY,
    description text NOT NULL
);

CREATE TABLE IF NOT EXISTS access.role_permissions (
    role_key varchar(100) NOT NULL REFERENCES access.roles(role_key) ON DELETE CASCADE,
    permission_key varchar(150) NOT NULL REFERENCES access.permissions(permission_key) ON DELETE CASCADE,
    PRIMARY KEY (role_key, permission_key)
);

CREATE TABLE IF NOT EXISTS access.user_roles (
    user_id uuid NOT NULL REFERENCES identity.user_accounts(id) ON DELETE CASCADE,
    role_key varchar(100) NOT NULL REFERENCES access.roles(role_key) ON DELETE CASCADE,
    assigned_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_key)
);

CREATE TABLE IF NOT EXISTS audit.audit_events (
    id uuid PRIMARY KEY,
    event_key text NOT NULL UNIQUE,
    actor_type varchar(32) NOT NULL,
    actor_id text NOT NULL,
    action varchar(160) NOT NULL,
    resource_type varchar(100) NOT NULL,
    resource_id text NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    details jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS audit_events_resource_time_idx
    ON audit.audit_events (resource_type, resource_id, occurred_at DESC);

INSERT INTO access.roles (role_key, display_name, description, is_system_role)
VALUES
    ('staff', 'Staff', 'Default least-privilege staff role', true),
    ('super_admin', 'SuperAdmin', 'Initial system administrator role', true)
ON CONFLICT (role_key) DO NOTHING;

INSERT INTO access.permissions (permission_key, description)
VALUES
    ('staff.read', 'Search and view authorized staff profiles'),
    ('staff.create', 'Create approved staff profiles'),
    ('staff.update', 'Update permitted staff profile fields'),
    ('staff.deactivate', 'Deactivate staff employment records'),
    ('staff.import', 'Import and validate the official staff directory'),
    ('departments.manage', 'Create and update departments'),
    ('roles.read', 'View roles and permissions'),
    ('roles.manage', 'Create roles and manage role permissions'),
    ('user_roles.assign', 'Assign roles within the actor''s delegation rights'),
    ('audit.read', 'Search and view authorized audit events'),
    ('settings.manage', 'Manage system settings')
ON CONFLICT (permission_key) DO NOTHING;

INSERT INTO access.role_permissions (role_key, permission_key)
VALUES ('staff', 'staff.read')
ON CONFLICT DO NOTHING;

INSERT INTO access.role_permissions (role_key, permission_key)
SELECT 'super_admin', permission_key FROM access.permissions
ON CONFLICT DO NOTHING;
