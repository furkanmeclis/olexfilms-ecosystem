-- Reverts 000082. Data loss: the public code aliases are dropped (the old
-- hub numbers they held stop resolving). Migrated warranties keep their
-- short (legacy) public codes; the restored 12..32 check is NOT VALID so it
-- does not reject them, but it applies to new rows again.

CREATE OR REPLACE FUNCTION vehicle_transfers_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF EXISTS (SELECT 1 FROM vehicles v
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

DROP TABLE IF EXISTS warranty_public_code_aliases;
DROP FUNCTION IF EXISTS warranty_public_code_aliases_check_row();

ALTER TABLE warranties DROP CONSTRAINT IF EXISTS uq_warranties_id_org_brand;

ALTER TABLE warranties DROP CONSTRAINT chk_warranties_public_code;
ALTER TABLE warranties
    ADD CONSTRAINT chk_warranties_public_code CHECK (public_code ~ '^[A-Za-z0-9_-]{12,32}$') NOT VALID;
