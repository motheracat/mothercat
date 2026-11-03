INSERT INTO access.role_permissions (role_key, permission_key)
VALUES
    ('admin', 'staff.read'),
    ('admin', 'staff.create'),
    ('admin', 'staff.update'),
    ('admin', 'staff.deactivate'),
    ('admin', 'staff.import'),
    ('admin', 'departments.manage'),
    ('admin', 'roles.read'),
    ('admin', 'roles.manage'),
    ('admin', 'user_roles.assign'),
    ('admin', 'audit.read'),
    ('admin', 'settings.manage')
ON CONFLICT DO NOTHING;
