CREATE TABLE IF NOT EXISTS identity.initial_admin_bootstrap (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    secret_hash CHAR(64),
    expires_at TIMESTAMPTZ NOT NULL,
    account_id UUID REFERENCES identity.user_accounts(id),
    completed_at TIMESTAMPTZ,
    CHECK (
        (completed_at IS NULL AND secret_hash IS NOT NULL)
        OR (completed_at IS NOT NULL AND secret_hash IS NULL)
    )
);
