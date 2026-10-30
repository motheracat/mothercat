CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS organization;

CREATE TABLE IF NOT EXISTS organization.departments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(120) NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS organization.staff_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    staff_id VARCHAR(80) NOT NULL UNIQUE,
    account_id UUID UNIQUE,
    department_id UUID REFERENCES organization.departments(id),
    full_name VARCHAR(200) NOT NULL,
    work_email VARCHAR(254),
    job_title VARCHAR(150),
    employment_status VARCHAR(24) NOT NULL DEFAULT 'pending'
        CHECK (employment_status IN ('pending', 'active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_organization_staff_department
    ON organization.staff_members (department_id, full_name);

CREATE INDEX IF NOT EXISTS idx_organization_staff_status
    ON organization.staff_members (employment_status, created_at DESC);
