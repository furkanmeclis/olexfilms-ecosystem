CREATE TABLE outbox_events (
    id            BIGSERIAL PRIMARY KEY,
    uuid          UUID         NOT NULL DEFAULT gen_random_uuid(),
    event_name    TEXT         NOT NULL,
    payload       JSONB        NOT NULL,
    status        TEXT         NOT NULL DEFAULT 'pending',
    attempts      INT          NOT NULL DEFAULT 0,
    last_error    TEXT         NULL,
    available_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    published_at  TIMESTAMPTZ  NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_outbox_events_uuid UNIQUE (uuid),
    CONSTRAINT chk_outbox_events_status CHECK (status IN ('pending', 'published', 'failed')),
    CONSTRAINT chk_outbox_events_attempts CHECK (attempts >= 0)
);

CREATE INDEX idx_outbox_events_pending
    ON outbox_events (status, available_at)
    WHERE status = 'pending';
