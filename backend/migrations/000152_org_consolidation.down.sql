-- Reverts TEC-1003 schema. Merged organizations stay merged (their rows are
-- not moved back); undo a merge with `org-merge -undo` before rolling back.
DELETE FROM roles WHERE slug = 'center_service';

-- K6 as in 000049 (no center -> dealer orders).
CREATE OR REPLACE FUNCTION orders_check_parties() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    s_type   TEXT;
    s_brand  BIGINT;
    b_type   TEXT;
    b_brand  BIGINT;
    b_parent BIGINT;
    b_cur    TEXT;
BEGIN
    IF TG_OP = 'UPDATE'
       AND NEW.seller_org_id = OLD.seller_org_id
       AND NEW.buyer_org_id = OLD.buyer_org_id
       AND NEW.brand_id = OLD.brand_id
       AND NEW.currency = OLD.currency THEN
        RETURN NEW;
    END IF;
    -- chk_orders_seller_not_buyer reports this case.
    IF NEW.seller_org_id = NEW.buyer_org_id THEN
        RETURN NEW;
    END IF;

    SELECT type, brand_id INTO s_type, s_brand FROM organizations WHERE id = NEW.seller_org_id;
    SELECT type, brand_id, parent_id INTO b_type, b_brand, b_parent
    FROM organizations WHERE id = NEW.buyer_org_id;
    IF s_brand IS DISTINCT FROM NEW.brand_id OR b_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'order parties must belong to brand %', NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    IF b_parent IS DISTINCT FROM NEW.seller_org_id THEN
        RAISE EXCEPTION 'order buyer % is not a direct child of seller %',
            NEW.buyer_org_id, NEW.seller_org_id
            USING ERRCODE = 'check_violation';
    END IF;
    IF NOT ((s_type = 'center' AND b_type = 'distributor')
            OR (s_type = 'distributor' AND b_type = 'dealer')) THEN
        RAISE EXCEPTION 'order flow %->% is not allowed (K6)', s_type, b_type
            USING ERRCODE = 'check_violation';
    END IF;

    IF TG_OP = 'INSERT' OR NEW.currency IS DISTINCT FROM OLD.currency
       OR NEW.brand_id IS DISTINCT FROM OLD.brand_id THEN
        SELECT currency INTO b_cur FROM brands WHERE id = NEW.brand_id;
        IF b_cur IS DISTINCT FROM NEW.currency THEN
            RAISE EXCEPTION 'order currency % must be the brand currency %', NEW.currency, b_cur
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;


DO $$
DECLARE r record;
BEGIN
    FOR r IN
        SELECT c.conname, c.conrelid::regclass AS tbl
        FROM pg_constraint c
        JOIN pg_namespace n ON n.oid = c.connamespace
        WHERE c.contype = 'f' AND n.nspname = 'public' AND c.condeferrable
          AND (c.confrelid = 'organizations'::regclass OR array_length(c.conkey, 1) > 1)
    LOOP
        EXECUTE format('ALTER TABLE %s ALTER CONSTRAINT %I NOT DEFERRABLE', r.tbl, r.conname);
    END LOOP;
END $$;

DO $$
DECLARE r record; def text;
BEGIN
    FOR r IN
        SELECT p.oid FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
        WHERE n.nspname = 'public' AND p.prosrc LIKE '%olex.org_merge%'
    LOOP
        def := regexp_replace(pg_get_functiondef(r.oid),
            E'\\n    IF TG_OP = ''UPDATE'' AND current_setting\\(''olex.org_merge'', true\\) = ''on'' THEN RETURN NEW; END IF; -- TEC-1003 org merge', '');
        EXECUTE def;
    END LOOP;
END $$;

DROP TABLE IF EXISTS org_merge_changes;
DROP TABLE IF EXISTS org_merge_runs;

ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS chk_organizations_merged_into,
    DROP CONSTRAINT IF EXISTS chk_organizations_service_point_name,
    DROP COLUMN IF EXISTS merged_into_id,
    DROP COLUMN IF EXISTS service_point_logo_object_key,
    DROP COLUMN IF EXISTS service_point_name,
    DROP COLUMN IF EXISTS service_point_enabled;
