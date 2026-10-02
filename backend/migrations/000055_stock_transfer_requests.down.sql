-- Reverts TEC-197. Data loss: every stock transfer request and its units
-- (their stock movements stay: stock_movements is append-only); the
-- single-line form of 000049 cannot hold multi-unit requests.
DELETE FROM role_permissions rp
USING roles r, permissions p
WHERE rp.role_id = r.id
  AND rp.permission_id = p.id
  AND r.slug = 'distributor_owner'
  AND p.slug = 'transfers.request';

DROP TABLE IF EXISTS stock_transfer_request_items;
DELETE FROM stock_transfer_requests;

ALTER TABLE stock_transfer_requests
    DROP CONSTRAINT chk_stock_transfer_requests_received,
    DROP CONSTRAINT chk_stock_transfer_requests_shipped,
    DROP CONSTRAINT chk_stock_transfer_requests_decided,
    DROP CONSTRAINT chk_stock_transfer_requests_total,
    DROP CONSTRAINT chk_stock_transfer_requests_status,
    DROP CONSTRAINT chk_stock_transfer_requests_transfer_no,
    DROP CONSTRAINT uq_stock_transfer_requests_id_org_brand,
    DROP CONSTRAINT uq_stock_transfer_requests_brand_no,
    DROP COLUMN received_at,
    DROP COLUMN shipped_at,
    DROP COLUMN cancelled_by_user_id,
    DROP COLUMN received_by_user_id,
    DROP COLUMN shipped_by_user_id,
    DROP COLUMN cancel_reason,
    DROP COLUMN total,
    DROP COLUMN transfer_no,
    ALTER COLUMN product_id SET NOT NULL;

DROP SEQUENCE IF EXISTS stock_transfer_no_seq;

ALTER TABLE stock_transfer_requests
    ADD CONSTRAINT chk_stock_transfer_requests_amount CHECK (
        (quantity IS NULL) <> (meters IS NULL)
        AND (quantity IS NULL OR quantity > 0)
        AND (meters IS NULL OR meters > 0)
    ),
    ADD CONSTRAINT chk_stock_transfer_requests_status CHECK (status IN (
        'requested', 'approved', 'rejected', 'completed', 'cancelled'
    )),
    ADD CONSTRAINT chk_stock_transfer_requests_decided CHECK (
        (status IN ('approved', 'rejected', 'completed')) = (decided_at IS NOT NULL)
        OR (status = 'cancelled')
    ),
    ADD CONSTRAINT chk_stock_transfer_requests_priced CHECK (
        status NOT IN ('approved', 'completed') OR (unit_price IS NOT NULL AND line_total IS NOT NULL)
    ),
    ADD CONSTRAINT chk_stock_transfer_requests_completed CHECK ((status = 'completed') = (completed_at IS NOT NULL));

-- 000049 body: sibling dealers under the approving distributor.
CREATE OR REPLACE FUNCTION stock_transfer_requests_check_parties() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    f_type TEXT; f_brand BIGINT; f_parent BIGINT;
    t_type TEXT; t_brand BIGINT; t_parent BIGINT;
    a_type TEXT; a_brand BIGINT;
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
    SELECT type, brand_id INTO a_type, a_brand FROM organizations WHERE id = NEW.approver_org_id;
    IF f_brand IS DISTINCT FROM NEW.brand_id OR t_brand IS DISTINCT FROM NEW.brand_id
       OR a_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'transfer parties must belong to brand %', NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    IF f_type IS DISTINCT FROM 'dealer' OR t_type IS DISTINCT FROM 'dealer'
       OR a_type IS DISTINCT FROM 'distributor'
       OR f_parent IS DISTINCT FROM NEW.approver_org_id
       OR t_parent IS DISTINCT FROM NEW.approver_org_id THEN
        RAISE EXCEPTION 'transfer must be between sibling dealers of approver % (K13)', NEW.approver_org_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;
