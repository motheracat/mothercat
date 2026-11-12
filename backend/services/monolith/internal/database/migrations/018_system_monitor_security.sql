CREATE TABLE system_monitor.mfa_credentials (
    account_id uuid PRIMARY KEY REFERENCES identity.user_accounts(id) ON DELETE CASCADE,
    secret_ciphertext bytea NOT NULL,
    enabled_at timestamptz NOT NULL DEFAULT now(),
    last_used_step bigint NOT NULL DEFAULT 0 CHECK (last_used_step >= 0)
);

CREATE TABLE system_monitor.mfa_enrollment_challenges (
    account_id uuid PRIMARY KEY REFERENCES identity.user_accounts(id) ON DELETE CASCADE,
    secret_ciphertext bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at)
);

CREATE TABLE system_monitor.security_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id uuid REFERENCES identity.user_accounts(id) ON DELETE SET NULL,
    event_type varchar(64) NOT NULL,
    severity varchar(16) NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    details jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(details) = 'object'),
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX system_monitor_security_events_recent_idx
    ON system_monitor.security_events (occurred_at DESC, id DESC);

CREATE INDEX system_monitor_security_events_account_recent_idx
    ON system_monitor.security_events (account_id, occurred_at DESC)
    WHERE account_id IS NOT NULL;
