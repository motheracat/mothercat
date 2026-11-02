ALTER TABLE identity.user_accounts
    ADD COLUMN IF NOT EXISTS suspended_at timestamptz,
    ADD COLUMN IF NOT EXISTS suspended_by uuid REFERENCES identity.user_accounts(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS user_accounts_active_unsuspended_idx
    ON identity.user_accounts (id)
    WHERE is_active AND suspended_at IS NULL;
