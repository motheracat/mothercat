ALTER TABLE identity.password_reset_tokens
    ADD COLUMN IF NOT EXISTS token_hash CHAR(64),
    ADD COLUMN IF NOT EXISTS purpose VARCHAR(24) NOT NULL DEFAULT 'password_reset';

UPDATE identity.password_reset_tokens
SET token_hash = encode(digest(convert_to(token, 'UTF8'), 'sha256'), 'hex')
WHERE token_hash IS NULL;

ALTER TABLE identity.password_reset_tokens
    ALTER COLUMN token_hash SET NOT NULL;

ALTER TABLE identity.password_reset_tokens
    ADD CONSTRAINT identity_recovery_token_purpose_check
    CHECK (purpose IN ('password_reset', 'activation'));

CREATE UNIQUE INDEX IF NOT EXISTS idx_identity_recovery_tokens_hash
    ON identity.password_reset_tokens (token_hash);

CREATE INDEX IF NOT EXISTS idx_identity_recovery_tokens_account_purpose
    ON identity.password_reset_tokens (account_id, purpose, expires_at);

ALTER TABLE identity.password_reset_tokens
    DROP COLUMN IF EXISTS token;
