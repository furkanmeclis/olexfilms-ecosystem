-- TEC-473 (F5-02b): the users of a fleet. A fleet user holds the global
-- `fleet` role (user_roles, portal realm); this table says which fleet the
-- user belongs to. organization_members is not used: a membership opens the
-- panel (TEC-90 realm rule) and fleets live outside the organization tree.
--
--   * One fleet per user (uq_fleet_users_user): the portal resolves the
--     signed-in fleet user's fleet from this row.
--   * is_primary: the first invited user owns the fleet vehicles
--     (fleet_profiles.primary_user_id = vehicles.user_id); one per fleet.
--   * status active | disabled: a disabled fleet user keeps the row as
--     history and cannot sign in (users.status = 'disabled').
CREATE TABLE fleet_users (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    fleet_org_id        BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    user_id             BIGINT        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    is_primary          BOOLEAN       NOT NULL DEFAULT false,
    status              VARCHAR(16)   NOT NULL DEFAULT 'active',
    invited_by_org_id   BIGINT        NULL REFERENCES organizations (id) ON DELETE SET NULL,
    invited_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    disabled_at         TIMESTAMPTZ   NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_fleet_users_uuid UNIQUE (uuid),
    CONSTRAINT uq_fleet_users_user UNIQUE (user_id),
    CONSTRAINT chk_fleet_users_status CHECK (status IN ('active', 'disabled')),
    CONSTRAINT chk_fleet_users_disabled CHECK ((status = 'disabled') = (disabled_at IS NOT NULL))
);

CREATE UNIQUE INDEX uq_fleet_users_primary ON fleet_users (fleet_org_id) WHERE is_primary;
CREATE INDEX idx_fleet_users_fleet ON fleet_users (fleet_org_id, created_at, id);

CREATE TRIGGER trg_fleet_users_set_updated_at
    BEFORE UPDATE ON fleet_users
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_fleet_users_check_org
    BEFORE INSERT OR UPDATE OF fleet_org_id, brand_id ON fleet_users
    FOR EACH ROW
    EXECUTE FUNCTION fleet_check_org('fleet_org_id', 'fleet');
