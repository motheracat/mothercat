INSERT INTO access.roles (role_key, display_name, description, is_system_role)
VALUES ('admin', 'Admin', 'Administrator with staff, department, and audit access', true)
ON CONFLICT (role_key) DO UPDATE
SET display_name = EXCLUDED.display_name,
    description = EXCLUDED.description,
    is_system_role = EXCLUDED.is_system_role;

INSERT INTO access.role_permissions (role_key, permission_key)
VALUES
    ('admin', 'staff.read'),
    ('admin', 'staff.create'),
    ('admin', 'staff.update'),
    ('admin', 'staff.deactivate'),
    ('admin', 'staff.import'),
    ('admin', 'departments.manage'),
    ('admin', 'audit.read')
ON CONFLICT DO NOTHING;
