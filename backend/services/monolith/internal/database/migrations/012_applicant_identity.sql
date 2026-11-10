CREATE TABLE identity.applicant_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email varchar(254) NOT NULL UNIQUE,
    display_name varchar(160) NOT NULL,
    password_hash text NOT NULL,
    email_verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (length(btrim(email)) BETWEEN 3 AND 254),
    CHECK (length(btrim(display_name)) BETWEEN 1 AND 160)
);

CREATE TABLE identity.applicant_email_verification_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    applicant_account_id uuid NOT NULL REFERENCES identity.applicant_accounts(id) ON DELETE CASCADE,
    token_hash char(64) NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX applicant_email_verification_account_idx
    ON identity.applicant_email_verification_tokens (applicant_account_id, expires_at DESC)
    WHERE consumed_at IS NULL;

CREATE TABLE identity.applicant_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    applicant_account_id uuid NOT NULL REFERENCES identity.applicant_accounts(id) ON DELETE CASCADE,
    token_hash char(64) NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX applicant_sessions_account_expiry_idx
    ON identity.applicant_sessions (applicant_account_id, expires_at);
