CREATE SCHEMA IF NOT EXISTS notifications;

CREATE TABLE notifications.notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    recipient_account_id uuid NOT NULL REFERENCES identity.user_accounts(id) ON DELETE CASCADE,
    event_type varchar(80) NOT NULL,
    title varchar(200) NOT NULL,
    body varchar(2000) NOT NULL DEFAULT '',
    resource_type varchar(40),
    resource_id uuid,
    source_event_id uuid NOT NULL,
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_event_id, recipient_account_id)
);
CREATE INDEX notifications_recipient_time_idx
    ON notifications.notifications (recipient_account_id, created_at DESC, id DESC);
CREATE INDEX notifications_unread_recipient_idx
    ON notifications.notifications (recipient_account_id, created_at DESC)
    WHERE read_at IS NULL;

CREATE TABLE notifications.notification_preferences (
    account_id uuid NOT NULL REFERENCES identity.user_accounts(id) ON DELETE CASCADE,
    event_type varchar(80) NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, event_type)
);
