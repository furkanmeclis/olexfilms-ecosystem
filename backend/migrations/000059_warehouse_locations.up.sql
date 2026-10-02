-- TEC-201 (F1-03a): warehouse and location tree.
--
--   warehouses -> rooms -> warehouse_locations (aisle -> shelf -> bin)
--
-- An organization (center or distributor, K4/K12) may run several
-- warehouses. The warehouse side is brand-independent (K20): none of these
-- tables carries brand_id; every row is scoped by organization_id.
--
-- warehouse_locations (000046 skeleton) gains warehouse_id, room_id, type,
-- full_code and sort_order. Rows created before this migration (no room)
-- stay valid "legacy" locations: every ledger and order foreign key to
-- warehouse_locations (id, organization_id) is untouched. Typed rows derive
-- warehouse_id and full_code in a trigger:
--
--   full_code = <warehouse code>-<room code>-<aisle>-<shelf>-<bin>
--
-- and full_code is unique per organization. Renaming a warehouse, room or
-- location rewrites the full_code of everything below it.

-- 1. Warehouses.
CREATE TABLE warehouses (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    code             VARCHAR(32)   NOT NULL,
    name             VARCHAR(200)  NOT NULL,
    address          TEXT          NULL,
    active           BOOLEAN       NOT NULL DEFAULT true,
    sort_order       INTEGER       NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_warehouses_uuid UNIQUE (uuid),
    CONSTRAINT uq_warehouses_id_org UNIQUE (id, organization_id),
    CONSTRAINT uq_warehouses_org_code UNIQUE (organization_id, code),
    CONSTRAINT chk_warehouses_code CHECK (code ~ '^[A-Z0-9_]{1,32}$'),
    CONSTRAINT chk_warehouses_name CHECK (btrim(name) <> '')
);

CREATE INDEX idx_warehouses_org_sort ON warehouses (organization_id, sort_order, id);

CREATE TRIGGER trg_warehouses_set_updated_at
    BEFORE UPDATE ON warehouses
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 2. Rooms of a warehouse.
CREATE TABLE rooms (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT        NOT NULL,
    warehouse_id     BIGINT        NOT NULL,
    code             VARCHAR(32)   NOT NULL,
    name             VARCHAR(200)  NOT NULL,
    active           BOOLEAN       NOT NULL DEFAULT true,
    sort_order       INTEGER       NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_rooms_uuid UNIQUE (uuid),
    CONSTRAINT uq_rooms_id_warehouse UNIQUE (id, warehouse_id),
    CONSTRAINT uq_rooms_warehouse_code UNIQUE (warehouse_id, code),
    -- A room belongs to the organization of its warehouse.
    CONSTRAINT fk_rooms_warehouse FOREIGN KEY (warehouse_id, organization_id)
        REFERENCES warehouses (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_rooms_code CHECK (code ~ '^[A-Z0-9_]{1,32}$'),
    CONSTRAINT chk_rooms_name CHECK (btrim(name) <> '')
);

CREATE INDEX idx_rooms_warehouse_sort ON rooms (warehouse_id, sort_order, id);

CREATE TRIGGER trg_rooms_set_updated_at
    BEFORE UPDATE ON rooms
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 3. Locations: type, room and the derived full_code.
ALTER TABLE warehouse_locations
    ADD COLUMN warehouse_id BIGINT       NULL,
    ADD COLUMN room_id      BIGINT       NULL,
    ADD COLUMN type         VARCHAR(16)  NULL,
    ADD COLUMN full_code    VARCHAR(255) NULL,
    ADD COLUMN sort_order   INTEGER      NOT NULL DEFAULT 0;

ALTER TABLE warehouse_locations
    ADD CONSTRAINT fk_warehouse_locations_warehouse FOREIGN KEY (warehouse_id, organization_id)
        REFERENCES warehouses (id, organization_id) ON DELETE RESTRICT,
    ADD CONSTRAINT fk_warehouse_locations_room FOREIGN KEY (room_id, warehouse_id)
        REFERENCES rooms (id, warehouse_id) ON DELETE RESTRICT,
    ADD CONSTRAINT chk_warehouse_locations_type CHECK (type IS NULL OR type IN ('aisle', 'shelf', 'bin')),
    -- Typed rows have a room, a warehouse and a full_code; legacy rows none.
    ADD CONSTRAINT chk_warehouse_locations_tree CHECK (
        (room_id IS NULL AND warehouse_id IS NULL AND type IS NULL AND full_code IS NULL)
        OR (room_id IS NOT NULL AND warehouse_id IS NOT NULL AND type IS NOT NULL AND full_code IS NOT NULL)
    ),
    ADD CONSTRAINT chk_warehouse_locations_typed_code CHECK (
        room_id IS NULL OR code ~ '^[A-Z0-9_]{1,32}$'
    );

-- Sibling codes repeat across the tree (every shelf has a bin 01), so the
-- organization-wide code uniqueness now holds for legacy rows only; typed
-- rows are unique by full_code (which includes every ancestor code).
ALTER TABLE warehouse_locations DROP CONSTRAINT uq_warehouse_locations_org_code;
CREATE UNIQUE INDEX uq_warehouse_locations_org_code_legacy
    ON warehouse_locations (organization_id, code) WHERE room_id IS NULL;
CREATE UNIQUE INDEX uq_warehouse_locations_org_full_code
    ON warehouse_locations (organization_id, full_code) WHERE full_code IS NOT NULL;
CREATE INDEX idx_warehouse_locations_room_sort
    ON warehouse_locations (room_id, parent_id, sort_order, id) WHERE room_id IS NOT NULL;

-- Derives warehouse_id and full_code and checks the tree shape:
-- aisle at the room root, shelf at the room root or under an aisle, bin
-- under a shelf; the parent sits in the same room.
CREATE FUNCTION warehouse_locations_derive() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    r_warehouse  BIGINT;
    r_code       TEXT;
    w_code       TEXT;
    p_room       BIGINT;
    p_type       TEXT;
    p_full_code  TEXT;
BEGIN
    IF NEW.parent_id IS NOT NULL THEN
        SELECT room_id, type, full_code INTO p_room, p_type, p_full_code
        FROM warehouse_locations WHERE id = NEW.parent_id;
    END IF;

    IF NEW.room_id IS NULL THEN
        IF p_room IS NOT NULL THEN
            RAISE EXCEPTION 'a legacy location cannot sit under a typed location'
                USING ERRCODE = '23514', CONSTRAINT = 'chk_warehouse_locations_tree';
        END IF;
        NEW.warehouse_id := NULL;
        NEW.full_code := NULL;
        RETURN NEW;
    END IF;

    SELECT r.warehouse_id, r.code, w.code INTO r_warehouse, r_code, w_code
    FROM rooms r JOIN warehouses w ON w.id = r.warehouse_id
    WHERE r.id = NEW.room_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'room % not found', NEW.room_id
            USING ERRCODE = '23503', CONSTRAINT = 'fk_warehouse_locations_room';
    END IF;
    NEW.warehouse_id := r_warehouse;

    IF NEW.parent_id IS NULL THEN
        IF NEW.type = 'bin' THEN
            RAISE EXCEPTION 'a bin needs a shelf parent'
                USING ERRCODE = '23514', CONSTRAINT = 'chk_warehouse_locations_tree';
        END IF;
        NEW.full_code := w_code || '-' || r_code || '-' || NEW.code;
        RETURN NEW;
    END IF;

    IF p_room IS DISTINCT FROM NEW.room_id
        OR NOT ((NEW.type = 'shelf' AND p_type = 'aisle') OR (NEW.type = 'bin' AND p_type = 'shelf')) THEN
        RAISE EXCEPTION 'invalid parent for a % location', NEW.type
            USING ERRCODE = '23514', CONSTRAINT = 'chk_warehouse_locations_tree';
    END IF;
    NEW.full_code := p_full_code || '-' || NEW.code;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_warehouse_locations_derive
    BEFORE INSERT OR UPDATE OF code, parent_id, room_id, type ON warehouse_locations
    FOR EACH ROW
    EXECUTE FUNCTION warehouse_locations_derive();

-- A changed full_code is pushed to the children (each child re-derives
-- its own full_code and cascades further).
CREATE FUNCTION warehouse_locations_cascade() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    UPDATE warehouse_locations SET code = code WHERE parent_id = NEW.id;
    RETURN NULL;
END;
$$;

CREATE TRIGGER trg_warehouse_locations_cascade
    AFTER UPDATE ON warehouse_locations
    FOR EACH ROW
    WHEN (OLD.full_code IS DISTINCT FROM NEW.full_code)
    EXECUTE FUNCTION warehouse_locations_cascade();

-- Renaming a room or a warehouse re-derives the root locations below it.
CREATE FUNCTION rooms_cascade_code() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    UPDATE warehouse_locations SET code = code WHERE room_id = NEW.id AND parent_id IS NULL;
    RETURN NULL;
END;
$$;

CREATE TRIGGER trg_rooms_cascade_code
    AFTER UPDATE OF code ON rooms
    FOR EACH ROW
    WHEN (OLD.code IS DISTINCT FROM NEW.code)
    EXECUTE FUNCTION rooms_cascade_code();

CREATE FUNCTION warehouses_cascade_code() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    UPDATE warehouse_locations SET code = code WHERE warehouse_id = NEW.id AND parent_id IS NULL;
    RETURN NULL;
END;
$$;

CREATE TRIGGER trg_warehouses_cascade_code
    AFTER UPDATE OF code ON warehouses
    FOR EACH ROW
    WHEN (OLD.code IS DISTINCT FROM NEW.code)
    EXECUTE FUNCTION warehouses_cascade_code();

-- 4. K12: the dealer has no warehouse (no locations, no counts); the full
-- warehouse module belongs to the center and the distributor. dealer_owner
-- loses warehouse.read. Source of truth: internal/platform/rbac/catalog.go.
DELETE FROM role_permissions rp
USING roles r, permissions p
WHERE rp.role_id = r.id
  AND rp.permission_id = p.id
  AND r.slug = 'dealer_owner'
  AND p.slug = 'warehouse.read';
