CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS access;

CREATE TABLE IF NOT EXISTS access.roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role_key VARCHAR(80) NOT NULL UNIQUE,
    display_name VARCHAR(120) NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    is_system_role BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS access.permissions (
    permission_key VARCHAR(120) PRIMARY KEY,
    description TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS access.role_permissions (
    role_id UUID NOT NULL REFERENCES access.roles(id) ON DELETE CASCADE,
    permission_key VARCHAR(120) NOT NULL REFERENCES access.permissions(permission_key) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_key)
);

CREATE TABLE IF NOT EXISTS access.user_roles (
    user_id UUID NOT NULL,
    role_id UUID NOT NULL REFERENCES access.roles(id) ON DELETE CASCADE,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_id)
);
