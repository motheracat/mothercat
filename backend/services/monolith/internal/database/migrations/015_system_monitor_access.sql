CREATE SCHEMA IF NOT EXISTS system_monitor;

CREATE TABLE system_monitor.access_grants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL REFERENCES identity.user_accounts(id),
    granted_by varchar(200) NOT NULL CHECK (length(btrim(granted_by)) BETWEEN 1 AND 200),
    grant_reason varchar(1000) NOT NULL CHECK (length(btrim(grant_reason)) BETWEEN 1 AND 1000),
    granted_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    revoked_by varchar(200),
    revoke_reason varchar(1000),
    CHECK (
        (revoked_at IS NULL AND revoked_by IS NULL AND revoke_reason IS NULL)
        OR
        (revoked_at IS NOT NULL AND revoked_by IS NOT NULL AND revoke_reason IS NOT NULL)
    ),
    CHECK (expires_at > granted_at)
);

CREATE INDEX system_monitor_active_access_grants_idx
    ON system_monitor.access_grants (account_id, expires_at DESC)
    WHERE revoked_at IS NULL;
