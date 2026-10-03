-- Reverts TEC-223. Data loss: every return request and its units (their
-- stock movements and accounting rows stay: both ledgers are append-only).
DELETE FROM stock_transfer_request_items i
USING stock_transfer_requests r
WHERE i.request_id = r.id AND r.kind = 'return';
DELETE FROM stock_transfer_requests WHERE kind = 'return';

DROP INDEX IF EXISTS idx_stock_transfer_requests_kind;

ALTER TABLE stock_transfer_requests
    DROP CONSTRAINT chk_stock_transfer_requests_parties,
    DROP CONSTRAINT chk_stock_transfer_requests_kind;

ALTER TABLE stock_transfer_requests
    DROP COLUMN kind,
    ADD CONSTRAINT chk_stock_transfer_requests_parties CHECK (
        from_org_id <> to_org_id AND approver_org_id <> from_org_id AND approver_org_id <> to_org_id
    );

-- 000055 body.
CREATE OR REPLACE FUNCTION stock_transfer_requests_check_parties() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    f_type TEXT; f_brand BIGINT; f_parent BIGINT;
    t_type TEXT; t_brand BIGINT; t_parent BIGINT;
    p_brand BIGINT;
BEGIN
    IF TG_OP = 'UPDATE'
       AND NEW.from_org_id = OLD.from_org_id
       AND NEW.to_org_id = OLD.to_org_id
       AND NEW.approver_org_id = OLD.approver_org_id
       AND NEW.brand_id = OLD.brand_id THEN
        RETURN NEW;
    END IF;
    SELECT type, brand_id, parent_id INTO f_type, f_brand, f_parent FROM organizations WHERE id = NEW.from_org_id;
    SELECT type, brand_id, parent_id INTO t_type, t_brand, t_parent FROM organizations WHERE id = NEW.to_org_id;
    SELECT brand_id INTO p_brand FROM organizations WHERE id = NEW.approver_org_id;
    IF f_brand IS DISTINCT FROM NEW.brand_id OR t_brand IS DISTINCT FROM NEW.brand_id
       OR p_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'transfer parties must belong to brand %', NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    IF f_type IS DISTINCT FROM t_type OR f_type NOT IN ('dealer', 'distributor')
       OR f_parent IS DISTINCT FROM NEW.approver_org_id
       OR t_parent IS DISTINCT FROM NEW.approver_org_id THEN
        RAISE EXCEPTION 'transfer must be between siblings of parent % (K13)', NEW.approver_org_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;
