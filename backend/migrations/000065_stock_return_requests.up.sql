-- TEC-223 (F1-12c): return requests from a dealer (or distributor) to its
-- direct parent, on the stock transfer request tables of TEC-197.
--
-- stock_transfer_requests gets a kind:
--   * sibling (default, every existing row): K13 transfer between siblings,
--     approver_org_id is the common parent (unchanged).
--   * return: the child (from_org_id = organization_id) hands units back to
--     its direct parent; to_org_id = approver_org_id = that parent, which
--     approves or rejects and receives. The status flow and the ledger
--     movements are the sibling ones (transfer_out on shipping, transfer_in
--     into the parent on receipt, transfer_cancel_restore on a cancel after
--     shipping; a rejection writes nothing), so no new stock movement type
--     is needed and the open-request guard of a unit covers both kinds.
--     The accounting reversal (parent: sale_return, child: purchase_return)
--     is booked by the use case on receipt.

ALTER TABLE stock_transfer_requests
    ADD COLUMN kind VARCHAR(16) NOT NULL DEFAULT 'sibling',
    DROP CONSTRAINT chk_stock_transfer_requests_parties;

ALTER TABLE stock_transfer_requests
    ADD CONSTRAINT chk_stock_transfer_requests_kind CHECK (kind IN ('sibling', 'return')),
    ADD CONSTRAINT chk_stock_transfer_requests_parties CHECK (
        from_org_id <> to_org_id AND approver_org_id <> from_org_id
        AND (
            (kind = 'sibling' AND approver_org_id <> to_org_id)
            OR (kind = 'return' AND approver_org_id = to_org_id)
        )
    );

CREATE INDEX idx_stock_transfer_requests_kind ON stock_transfer_requests (brand_id, kind, created_at);

-- Sibling: both sides of the request's brand, same type (dealer or
-- distributor), direct children of approver_org_id (000055, K13).
-- Return: the giver is a dealer or distributor of the brand and the
-- receiver (= approver) is its direct parent in the same brand.
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
       AND NEW.brand_id = OLD.brand_id
       AND NEW.kind = OLD.kind THEN
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
    IF NEW.kind = 'return' THEN
        IF f_type IS NULL OR f_type NOT IN ('dealer', 'distributor')
           OR f_parent IS DISTINCT FROM NEW.to_org_id
           OR NEW.approver_org_id IS DISTINCT FROM NEW.to_org_id THEN
            RAISE EXCEPTION 'a return goes to the direct parent only (from %)', NEW.from_org_id
                USING ERRCODE = 'check_violation';
        END IF;
        RETURN NEW;
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
