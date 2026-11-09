-- TEC-1062: the center opens an order for a recipient that is not a dealer
-- yet (a dealer candidate or a retail delivery point). The recipient is a
-- dealer organization in status 'pending' directly under the center, with
-- no members and no sign-in, so the order, stock, shipping and cari flows
-- run unchanged. This table is the waiting pool (the legacy warehouse's
-- local customers without a dealer link): one row per recipient.
--
--   kind:  dealer_candidate | retail_point (what the center intends)
--   state: pending   in the pool
--          retail    kept as a retail / delivery point (leaves the pool)
--          converted turned into an active dealer (organizations.status
--                    'active', the owner attached when given)
--
-- organization_id is the center that keeps the pool, recipient_org_id the
-- recipient; orgmerge moves both, so their foreign keys are deferrable.
CREATE TABLE pending_recipients (
    id                  BIGSERIAL    PRIMARY KEY,
    organization_id     BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE
                                     DEFERRABLE INITIALLY DEFERRED,
    brand_id            BIGINT       NOT NULL REFERENCES brands (id) ON DELETE CASCADE,
    recipient_org_id    BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE
                                     DEFERRABLE INITIALLY DEFERRED,
    kind                VARCHAR(16)  NOT NULL,
    state               VARCHAR(16)  NOT NULL DEFAULT 'pending',
    note                TEXT         NULL,
    created_by_user_id  BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    resolved_by_user_id BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    resolved_at         TIMESTAMPTZ  NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_pending_recipients_recipient UNIQUE (recipient_org_id),
    CONSTRAINT chk_pending_recipients_kind CHECK (kind IN ('dealer_candidate', 'retail_point')),
    CONSTRAINT chk_pending_recipients_state CHECK (state IN ('pending', 'retail', 'converted')),
    CONSTRAINT chk_pending_recipients_self CHECK (organization_id <> recipient_org_id)
);

CREATE INDEX idx_pending_recipients_pool
    ON pending_recipients (organization_id, state, created_at DESC);
