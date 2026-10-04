-- TEC-356: completed service cancellation.
-- Source of truth: internal/platform/rbac/catalog.go.

ALTER TABLE services
    DROP CONSTRAINT IF EXISTS chk_services_completed,
    ADD CONSTRAINT chk_services_completed CHECK (
        (status = 'completed' AND completed_at IS NOT NULL)
        OR status = 'cancelled'
        OR (status NOT IN ('completed', 'cancelled') AND completed_at IS NULL)
    );

CREATE OR REPLACE FUNCTION services_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_brand   BIGINT;
    model_brand BIGINT;
BEGIN
    IF TG_OP = 'UPDATE' AND OLD.status IN ('completed', 'cancelled')
       AND NEW.status IS DISTINCT FROM OLD.status
       AND NOT (OLD.status = 'completed' AND NEW.status = 'cancelled') THEN
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

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Cancel completed services', 'services.cancel_completed', 'services',
       ARRAY['own', 'assigned', 'managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Cancel a completed service with warranty void, stock return and accounting reversal.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'services.cancel_completed', 'all'),
    ('center_staff', 'services.cancel_completed', 'brand'),
    ('distributor_owner', 'services.cancel_completed', 'subtree'),
    ('dealer_owner', 'services.cancel_completed', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
