-- TEC-508 (F5-10a): persistent module requests. POST /v1/features/{key}/request
-- writes a row (the notification stays). The decider is resolved on read, so
-- a reparented dealer's open request follows its new distributor:
--   * a dealer under a distributor -> that distributor
--     (GET /v1/tenant/modules/requests, approve opens it with SetForDealers);
--   * a distributor, or a dealer directly under the center -> the platform
--     admins (GET /v1/platform/modules/requests, approve is SetByAdmin).
-- At most one pending request per organization x module (partial UNIQUE).
-- A module that opens some other way (admin, distributor, dealer standard,
-- module bundle subscription) approves the pending request automatically
-- (decided_by_user_id NULL, decision_note 'auto').
CREATE TABLE module_requests (
    id                    BIGSERIAL    PRIMARY KEY,
    uuid                  UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id       BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    brand_id              BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    module_key            VARCHAR(64)  NOT NULL REFERENCES modules (key) ON DELETE RESTRICT,
    note                  TEXT         NOT NULL DEFAULT '',
    status                VARCHAR(16)  NOT NULL DEFAULT 'pending',
    requested_by_user_id  BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    decided_by_user_id    BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    decision_note         TEXT         NOT NULL DEFAULT '',
    decided_at            TIMESTAMPTZ  NULL,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_module_requests_uuid UNIQUE (uuid),
    CONSTRAINT chk_module_requests_status CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    CONSTRAINT chk_module_requests_note CHECK (char_length(note) <= 1000),
    CONSTRAINT chk_module_requests_decision_note CHECK (char_length(decision_note) <= 1000),
    CONSTRAINT chk_module_requests_decided CHECK ((status = 'pending') = (decided_at IS NULL))
);

CREATE UNIQUE INDEX uq_module_requests_open ON module_requests (organization_id, module_key)
    WHERE status = 'pending';
CREATE INDEX idx_module_requests_org ON module_requests (organization_id, created_at DESC);
CREATE INDEX idx_module_requests_pending_key ON module_requests (module_key) WHERE status = 'pending';

CREATE TRIGGER trg_module_requests_updated_at
    BEFORE UPDATE ON module_requests
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
