-- TEC-260 (F2-01i): legacy warranty numbers and historical vehicle transfers.
--
-- The old hub has no warranty number of its own: its public warranty page
-- and QR codes are /warranty/{service_no}, and the service number is "DS" +
-- 8 alphanumerics (10 characters). The migrator keeps that number as the
-- public_code of one warranty of the service, so the old links and printed
-- QR codes keep resolving through GET /v1/public/warranties/{code}.
--
--   1. chk_warranties_public_code accepts 4..32 URL-safe characters (the
--      public lookup's path check, TEC-248). Codes generated here are still
--      22 characters.
--   2. warranty_public_code_aliases: a legacy number whose warranty was
--      merged into another one (duplicate legacy warranties of the same
--      vehicle and unit; the earliest start is kept) still resolves, to the
--      warranty that was kept. A code is either a public_code or an alias,
--      never both (trigger). Aliases are written only by the migrator and
--      never change.
--   3. vehicle_transfers: a transfer inserted as already completed is a
--      historical record (the hub's completed service_customer_transfers).
--      The vehicle may have changed hands again since, so the "starts from
--      the current owner" check applies to new (pending) transfers only.
--      The application always inserts pending transfers.

-- 1. Public code length.
ALTER TABLE warranties DROP CONSTRAINT chk_warranties_public_code;
ALTER TABLE warranties
    ADD CONSTRAINT chk_warranties_public_code CHECK (public_code ~ '^[A-Za-z0-9_-]{4,32}$');

-- Target of the alias FK, so an alias always carries its warranty's scope.
ALTER TABLE warranties
    ADD CONSTRAINT uq_warranties_id_org_brand UNIQUE (id, organization_id, brand_id);

-- 2. Aliases.
CREATE TABLE warranty_public_code_aliases (
    id               BIGSERIAL     PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    code             VARCHAR(32)   NOT NULL,
    warranty_id      BIGINT        NOT NULL,
    organization_id  BIGINT        NOT NULL,
    brand_id         BIGINT        NOT NULL,
    -- Why the code is not the warranty's own public_code (e.g. 'merged').
    reason           VARCHAR(32)   NOT NULL,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_warranty_public_code_aliases_uuid UNIQUE (uuid),
    CONSTRAINT uq_warranty_public_code_aliases_code UNIQUE (code),
    CONSTRAINT fk_warranty_public_code_aliases_warranty FOREIGN KEY (warranty_id, organization_id, brand_id)
        REFERENCES warranties (id, organization_id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_warranty_public_code_aliases_code CHECK (code ~ '^[A-Za-z0-9_-]{4,32}$'),
    CONSTRAINT chk_warranty_public_code_aliases_reason CHECK (btrim(reason) <> '')
);

CREATE INDEX idx_warranty_public_code_aliases_warranty ON warranty_public_code_aliases (warranty_id);

CREATE FUNCTION warranty_public_code_aliases_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION 'warranty_public_code_aliases: aliases are immutable (alias %)', OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    IF EXISTS (SELECT 1 FROM warranties w WHERE w.public_code = NEW.code) THEN
        RAISE EXCEPTION 'warranty_public_code_aliases: % is already a public code', NEW.code
            USING ERRCODE = 'unique_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_warranty_public_code_aliases_check_row
    BEFORE INSERT OR UPDATE ON warranty_public_code_aliases
    FOR EACH ROW
    EXECUTE FUNCTION warranty_public_code_aliases_check_row();

-- 3. Historical (completed) transfers skip the current owner check.
CREATE OR REPLACE FUNCTION vehicle_transfers_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.status <> 'completed' AND EXISTS (SELECT 1 FROM vehicles v
                   WHERE v.id = NEW.vehicle_id AND v.user_id IS DISTINCT FROM NEW.from_user_id) THEN
            RAISE EXCEPTION 'vehicle_transfers: vehicle % does not belong to user %', NEW.vehicle_id, NEW.from_user_id
                USING ERRCODE = 'check_violation';
        END IF;
        RETURN NEW;
    END IF;
    IF NEW.vehicle_id IS DISTINCT FROM OLD.vehicle_id OR NEW.from_user_id IS DISTINCT FROM OLD.from_user_id THEN
        RAISE EXCEPTION 'vehicle_transfers: vehicle and current owner are immutable (transfer %)', OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.status IS DISTINCT FROM OLD.status AND OLD.status <> 'pending' THEN
        RAISE EXCEPTION 'vehicle_transfers: status % is final (transfer %)', OLD.status, OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;
