-- TEC-462 (F3-06i): a completed warranty claim may be manually reopened
-- only by a platform super admin. The application enforces the role and
-- required reason; the database allows exactly closed -> approved.

ALTER TABLE warranty_claim_events
    DROP CONSTRAINT IF EXISTS chk_warranty_claim_events_type;

ALTER TABLE warranty_claim_events
    ADD CONSTRAINT chk_warranty_claim_events_type CHECK (event_type IN (
        'created', 'status_changed', 'note', 'part_added', 'photo_added',
        'ai_triaged', 'reapply_linked', 'reopened'));

ALTER TABLE warranty_claim_events
    DROP CONSTRAINT IF EXISTS chk_warranty_claim_events_status_change;

ALTER TABLE warranty_claim_events
    ADD CONSTRAINT chk_warranty_claim_events_status_change CHECK (
        (event_type = 'created' AND from_status IS NULL AND to_status IS NOT NULL)
        OR (event_type IN ('status_changed', 'reopened') AND from_status IS NOT NULL AND to_status IS NOT NULL)
        OR (event_type NOT IN ('created', 'status_changed', 'reopened') AND from_status IS NULL AND to_status IS NULL));

CREATE OR REPLACE FUNCTION warranty_claims_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    w_org      BIGINT;
    w_brand    BIGINT;
    w_service  BIGINT;
    w_vehicle  BIGINT;
    w_holder   BIGINT;
    c_type     VARCHAR(16);
    s_brand    BIGINT;
    s_vehicle  BIGINT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        SELECT organization_id, brand_id, service_id, vehicle_id, holder_user_id
          INTO w_org, w_brand, w_service, w_vehicle, w_holder
          FROM warranties WHERE id = NEW.warranty_id;
        IF FOUND THEN
            IF w_brand IS DISTINCT FROM NEW.brand_id THEN
                RAISE EXCEPTION 'warranty_claims: warranty % is outside brand %', NEW.warranty_id, NEW.brand_id
                    USING ERRCODE = 'check_violation';
            END IF;
            IF NEW.organization_id IS DISTINCT FROM w_org THEN
                SELECT type INTO c_type FROM organizations WHERE id = NEW.organization_id;
                IF c_type IS DISTINCT FROM 'distributor' OR NOT EXISTS (
                    WITH RECURSIVE ancestors(id, parent_id, depth) AS (
                        SELECT o.id, o.parent_id, 0 FROM organizations o WHERE o.id = w_org
                        UNION ALL
                        SELECT o.id, o.parent_id, a.depth + 1
                        FROM organizations o JOIN ancestors a ON o.id = a.parent_id
                        WHERE a.depth < 32
                    )
                    SELECT 1 FROM ancestors WHERE id = NEW.organization_id
                ) THEN
                    RAISE EXCEPTION 'warranty_claims: organization % may not claim warranty % of organization %',
                        NEW.organization_id, NEW.warranty_id, w_org
                        USING ERRCODE = 'check_violation';
                END IF;
            END IF;
            IF NEW.service_id IS DISTINCT FROM w_service
               OR NEW.vehicle_id IS DISTINCT FROM w_vehicle
               OR NEW.customer_user_id IS DISTINCT FROM w_holder THEN
                RAISE EXCEPTION 'warranty_claims: service, vehicle and customer must match warranty %', NEW.warranty_id
                    USING ERRCODE = 'check_violation';
            END IF;
        END IF;
    ELSE
        IF NEW.uuid <> OLD.uuid
           OR NEW.organization_id <> OLD.organization_id
           OR NEW.brand_id <> OLD.brand_id
           OR NEW.claim_no <> OLD.claim_no
           OR NEW.warranty_id <> OLD.warranty_id
           OR NEW.service_id <> OLD.service_id
           OR NEW.vehicle_id <> OLD.vehicle_id
           OR NEW.customer_user_id <> OLD.customer_user_id THEN
            RAISE EXCEPTION 'warranty_claims: owner and claimed warranty of claim % cannot change', OLD.id
                USING ERRCODE = 'check_violation';
        END IF;
        IF OLD.status = 'closed'
           AND NOT (
               NEW.status = 'approved'
               AND NEW.closed_at IS NULL
               AND NEW.reapply_service_id IS NULL
           ) THEN
            RAISE EXCEPTION 'warranty_claims: claim % is closed', OLD.id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;

    IF NEW.reapply_service_id IS NOT NULL
       AND (TG_OP = 'INSERT' OR NEW.reapply_service_id IS DISTINCT FROM OLD.reapply_service_id) THEN
        SELECT brand_id, vehicle_id INTO s_brand, s_vehicle FROM services WHERE id = NEW.reapply_service_id;
        IF FOUND AND (s_brand IS DISTINCT FROM NEW.brand_id OR s_vehicle IS DISTINCT FROM NEW.vehicle_id) THEN
            RAISE EXCEPTION 'warranty_claims: re-application service % is outside the brand or vehicle of claim %',
                NEW.reapply_service_id, NEW.id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;

    RETURN NEW;
END;
$$;
