-- Reverts TEC-356 permission seed.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug = 'services.cancel_completed'
);

DELETE FROM permissions WHERE slug = 'services.cancel_completed';

UPDATE services SET completed_at = NULL WHERE status = 'cancelled';

ALTER TABLE services
    DROP CONSTRAINT IF EXISTS chk_services_completed,
    ADD CONSTRAINT chk_services_completed CHECK ((status = 'completed') = (completed_at IS NOT NULL));

CREATE OR REPLACE FUNCTION services_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_brand   BIGINT;
    model_brand BIGINT;
BEGIN
    IF TG_OP = 'UPDATE' AND OLD.status IN ('completed', 'cancelled')
       AND NEW.status IS DISTINCT FROM OLD.status THEN
        RAISE EXCEPTION 'services: status % is final (service %)', OLD.status, OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    IF TG_OP = 'INSERT' OR NEW.organization_id IS DISTINCT FROM OLD.organization_id
       OR NEW.brand_id IS DISTINCT FROM OLD.brand_id THEN
        SELECT brand_id INTO org_brand FROM organizations WHERE id = NEW.organization_id;
        IF FOUND AND org_brand IS DISTINCT FROM NEW.brand_id THEN
            RAISE EXCEPTION 'services: brand % does not match organization % (brand %)',
                NEW.brand_id, NEW.organization_id, org_brand
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    -- The vehicle belongs to the customer when the pair is written; a later
    -- ownership transfer (F1-06) does not rewrite past services.
    IF TG_OP = 'INSERT' OR NEW.vehicle_id IS DISTINCT FROM OLD.vehicle_id
       OR NEW.customer_user_id IS DISTINCT FROM OLD.customer_user_id THEN
        IF EXISTS (SELECT 1 FROM vehicles v
                   WHERE v.id = NEW.vehicle_id AND v.user_id IS DISTINCT FROM NEW.customer_user_id) THEN
            RAISE EXCEPTION 'services: vehicle % does not belong to user %',
                NEW.vehicle_id, NEW.customer_user_id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    IF TG_OP = 'INSERT' OR NEW.car_model_id IS DISTINCT FROM OLD.car_model_id
       OR NEW.car_brand_id IS DISTINCT FROM OLD.car_brand_id THEN
        SELECT car_brand_id INTO model_brand FROM car_models WHERE id = NEW.car_model_id;
        IF FOUND AND model_brand IS DISTINCT FROM NEW.car_brand_id THEN
            RAISE EXCEPTION 'services: car model % belongs to car brand %, not %',
                NEW.car_model_id, model_brand, NEW.car_brand_id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
