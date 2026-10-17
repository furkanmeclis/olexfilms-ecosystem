-- TEC-769 (Depo F4.4): user <-> warehouse membership.
--
-- A user without any membership row in an organization keeps the old
-- behaviour and works in every warehouse of it. Once the user has at least
-- one row, warehouse writes (stock entry, transfer, count, move) are limited
-- to the warehouses listed here. is_default marks the warehouse the mobile
-- app and the panel preselect (at most one per user and organization).
--
-- The warehouse side is brand-independent (K20): no brand_id. The member
-- must be a member of the organization (composite FK to
-- organization_members); leaving the organization drops the rows.
CREATE TABLE warehouse_members (
    id                  BIGSERIAL PRIMARY KEY,
    uuid                UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT       NOT NULL,
    warehouse_id        BIGINT       NOT NULL,
    user_id             BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    is_default          BOOLEAN      NOT NULL DEFAULT false,
    created_by_user_id  BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_warehouse_members_uuid UNIQUE (uuid),
    CONSTRAINT uq_warehouse_members_warehouse_user UNIQUE (warehouse_id, user_id),
    CONSTRAINT fk_warehouse_members_warehouse FOREIGN KEY (warehouse_id, organization_id)
        REFERENCES warehouses (id, organization_id) ON DELETE CASCADE,
    CONSTRAINT fk_warehouse_members_org_member FOREIGN KEY (organization_id, user_id)
        REFERENCES organization_members (organization_id, user_id) ON DELETE CASCADE
);

CREATE INDEX idx_warehouse_members_org_user ON warehouse_members (organization_id, user_id);

CREATE UNIQUE INDEX uq_warehouse_members_default
    ON warehouse_members (organization_id, user_id)
    WHERE is_default;

CREATE TRIGGER trg_warehouse_members_set_updated_at
    BEFORE UPDATE ON warehouse_members
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
