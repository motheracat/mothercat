CREATE SCHEMA IF NOT EXISTS access;

INSERT INTO access.roles (role_key, display_name, description, is_system_role)
VALUES ('staff', 'Staff', 'Default least-privilege staff role', TRUE)
ON CONFLICT (role_key) DO NOTHING;
