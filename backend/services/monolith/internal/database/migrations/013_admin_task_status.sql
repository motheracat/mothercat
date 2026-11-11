ALTER TABLE tasks.tasks
    ADD COLUMN admin_status varchar(16) NOT NULL DEFAULT 'open'
        CHECK (admin_status IN ('open', 'scheduled', 'in_progress', 'closed')),
    ADD COLUMN admin_status_updated_by uuid REFERENCES identity.user_accounts(id),
    ADD COLUMN admin_status_updated_at timestamptz;

INSERT INTO access.permissions (permission_key, description)
VALUES ('tasks.update_status', 'Update the administrative task status')
ON CONFLICT (permission_key) DO NOTHING;

INSERT INTO access.role_permissions (role_key, permission_key)
SELECT role_key, 'tasks.update_status'
FROM access.roles
WHERE role_key IN ('admin', 'super_admin')
ON CONFLICT DO NOTHING;
