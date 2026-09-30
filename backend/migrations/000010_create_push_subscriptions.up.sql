CREATE TABLE push_subscriptions (
    id         BIGSERIAL PRIMARY KEY,
    uuid       UUID        NOT NULL DEFAULT gen_random_uuid(),
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    endpoint   TEXT        NOT NULL,
    key_p256dh TEXT        NOT NULL,
    key_auth   TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT push_subscriptions_uuid_uq UNIQUE (uuid),
    CONSTRAINT push_subscriptions_user_endpoint_uq UNIQUE (user_id, endpoint)
);

CREATE INDEX idx_push_subscriptions_user ON push_subscriptions (user_id);
