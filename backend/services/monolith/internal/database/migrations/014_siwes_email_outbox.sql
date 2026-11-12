CREATE TABLE opportunities.application_email_outbox (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id uuid NOT NULL REFERENCES opportunities.siwes_applications(id) ON DELETE CASCADE,
    status varchar(24) NOT NULL
        CHECK (status IN ('submitted', 'under_review', 'shortlisted', 'accepted', 'rejected')),
    created_at timestamptz NOT NULL DEFAULT now(),
    available_at timestamptz NOT NULL DEFAULT now(),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error varchar(1000),
    processed_at timestamptz,
    UNIQUE (application_id, status)
);

CREATE INDEX application_email_outbox_pending_idx
    ON opportunities.application_email_outbox (available_at, created_at, id)
    WHERE processed_at IS NULL;
