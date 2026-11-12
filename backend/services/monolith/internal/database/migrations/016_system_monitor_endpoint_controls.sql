CREATE TABLE system_monitor.endpoint_controls (
    method varchar(10) NOT NULL,
    route varchar(300) NOT NULL,
    paused_by uuid NOT NULL REFERENCES identity.user_accounts(id),
    paused_at timestamptz NOT NULL DEFAULT now(),
    reason varchar(1000) NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 1000),
    response_status integer NOT NULL DEFAULT 503 CHECK (response_status BETWEEN 400 AND 599),
    response_message varchar(500) NOT NULL CHECK (length(btrim(response_message)) BETWEEN 1 AND 500),
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (method, route),
    CHECK (expires_at > paused_at)
);

CREATE INDEX system_monitor_endpoint_controls_expiry_idx
    ON system_monitor.endpoint_controls (expires_at);

CREATE TABLE system_monitor.endpoint_control_history (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    method varchar(10) NOT NULL,
    route varchar(300) NOT NULL,
    action varchar(16) NOT NULL CHECK (action IN ('paused', 'resumed', 'expired')),
    actor_id uuid REFERENCES identity.user_accounts(id),
    actor_type varchar(16) NOT NULL CHECK (actor_type IN ('staff', 'system')),
    reason varchar(1000) NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 1000),
    response_status integer CHECK (response_status BETWEEN 400 AND 599),
    response_message varchar(500),
    expires_at timestamptz,
    changed_at timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (actor_type = 'staff' AND actor_id IS NOT NULL)
        OR (actor_type = 'system' AND actor_id IS NULL)
    )
);

CREATE INDEX system_monitor_endpoint_control_history_recent_idx
    ON system_monitor.endpoint_control_history (changed_at DESC, id DESC);
