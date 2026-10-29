CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE identity.user_sessions
    ADD COLUMN IF NOT EXISTS session_token_hash CHAR(64);

UPDATE identity.user_sessions
SET session_token_hash = encode(digest(convert_to(session_token, 'UTF8'), 'sha256'), 'hex')
WHERE session_token_hash IS NULL;

ALTER TABLE identity.user_sessions
    ALTER COLUMN session_token_hash SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_identity_sessions_token_hash
    ON identity.user_sessions (session_token_hash);

ALTER TABLE identity.user_sessions
    DROP COLUMN IF EXISTS session_token;

CREATE INDEX IF NOT EXISTS idx_identity_auth_events_staff_time
    ON identity.authentication_events (staff_id, occurred_at DESC);