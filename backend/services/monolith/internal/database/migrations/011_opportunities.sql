CREATE SCHEMA IF NOT EXISTS opportunities;

CREATE TABLE opportunities.opportunities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    type varchar(16) NOT NULL CHECK (type IN ('job', 'siwes')),
    title varchar(160) NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 160),
    location varchar(120) NOT NULL CHECK (length(btrim(location)) BETWEEN 1 AND 120),
    work_mode varchar(80) NOT NULL CHECK (length(btrim(work_mode)) BETWEEN 1 AND 80),
    summary varchar(400) NOT NULL CHECK (length(btrim(summary)) BETWEEN 1 AND 400),
    description varchar(8000) NOT NULL CHECK (length(btrim(description)) BETWEEN 1 AND 8000),
    requirements varchar(4000) NOT NULL DEFAULT '' CHECK (length(requirements) <= 4000),
    application_deadline date,
    status varchar(16) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'published', 'closed')),
    created_by uuid NOT NULL REFERENCES identity.user_accounts(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX opportunities_public_idx
    ON opportunities.opportunities (type, application_deadline, created_at DESC, id)
    WHERE status = 'published';
CREATE INDEX opportunities_admin_idx
    ON opportunities.opportunities (status, type, created_at DESC, id);

CREATE TABLE opportunities.siwes_applications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    opportunity_id uuid NOT NULL REFERENCES opportunities.opportunities(id),
    full_name varchar(160) NOT NULL CHECK (length(btrim(full_name)) BETWEEN 1 AND 160),
    email varchar(254) NOT NULL CHECK (length(btrim(email)) BETWEEN 3 AND 254),
    phone varchar(40) NOT NULL CHECK (length(btrim(phone)) BETWEEN 1 AND 40),
    institution varchar(180) NOT NULL CHECK (length(btrim(institution)) BETWEEN 1 AND 180),
    course varchar(160) NOT NULL CHECK (length(btrim(course)) BETWEEN 1 AND 160),
    matric_number varchar(80) NOT NULL CHECK (length(btrim(matric_number)) BETWEEN 1 AND 80),
    message varchar(2000) NOT NULL DEFAULT '' CHECK (length(message) <= 2000),
    status varchar(24) NOT NULL DEFAULT 'submitted'
        CHECK (status IN ('submitted', 'under_review', 'shortlisted', 'accepted', 'rejected')),
    reviewer_note varchar(2000) NOT NULL DEFAULT '' CHECK (length(reviewer_note) <= 2000),
    document_name varchar(200) NOT NULL,
    document_type varchar(100) NOT NULL
        CHECK (document_type IN ('application/pdf', 'application/msword',
            'application/vnd.openxmlformats-officedocument.wordprocessingml.document')),
    document_size_bytes bigint NOT NULL CHECK (document_size_bytes BETWEEN 1 AND 10485760),
    document_data bytea NOT NULL,
    submitted_at timestamptz NOT NULL DEFAULT now(),
    reviewed_at timestamptz,
    reviewed_by uuid REFERENCES identity.user_accounts(id)
);
CREATE INDEX siwes_applications_opportunity_idx
    ON opportunities.siwes_applications (opportunity_id, status, submitted_at DESC, id);
CREATE INDEX siwes_applications_status_idx
    ON opportunities.siwes_applications (status, submitted_at DESC, id);
CREATE INDEX siwes_applications_retention_idx
    ON opportunities.siwes_applications (submitted_at);

INSERT INTO access.permissions (permission_key, description)
VALUES
    ('opportunities.publish', 'Create and manage job and SIWES opportunities'),
    ('applications.review', 'Review job applications'),
    ('it_applications.review', 'Review IT and SIWES applications')
ON CONFLICT (permission_key) DO NOTHING;

INSERT INTO access.role_permissions (role_key, permission_key)
SELECT role_key, permission_key
FROM access.roles CROSS JOIN access.permissions
WHERE role_key IN ('admin', 'super_admin')
  AND permission_key IN ('opportunities.publish', 'applications.review', 'it_applications.review')
ON CONFLICT DO NOTHING;
