-- TEC-309 (F3-08e): service catalog subscriptions use the F3-01 contract
-- engine. The nullable columns were introduced by 000084 without FKs because
-- the contract schema landed separately.

ALTER TABLE contract_instances
    DROP CONSTRAINT IF EXISTS chk_contract_instances_subject_type,
    ADD CONSTRAINT chk_contract_instances_subject_type
        CHECK (subject_type IN ('service', 'service_subscription'));

ALTER TABLE service_catalog_items
    ADD CONSTRAINT fk_service_catalog_items_contract_template
        FOREIGN KEY (contract_template_id, brand_id)
        REFERENCES contract_templates (id, brand_id) ON DELETE RESTRICT;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM service_catalog_items i
        JOIN contract_templates ct ON ct.id = i.contract_template_id AND ct.brand_id = i.brand_id
        WHERE i.contract_template_id IS NOT NULL
          AND ct.kind <> 'service_sale'
    ) THEN
        RAISE EXCEPTION 'service_catalog_items: all contract templates must be kind service_sale'
            USING ERRCODE = 'check_violation';
    END IF;
END;
$$;

CREATE FUNCTION service_catalog_items_check_contract_template_kind() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    t_kind TEXT;
BEGIN
    IF NEW.contract_template_id IS NULL THEN
        RETURN NEW;
    END IF;
    SELECT kind INTO t_kind
    FROM contract_templates
    WHERE id = NEW.contract_template_id AND brand_id = NEW.brand_id;
    IF t_kind IS DISTINCT FROM 'service_sale' THEN
        RAISE EXCEPTION 'service_catalog_items: contract template % must be kind service_sale',
            NEW.contract_template_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_service_catalog_items_check_contract_template_kind
    BEFORE INSERT OR UPDATE OF contract_template_id, brand_id ON service_catalog_items
    FOR EACH ROW
    EXECUTE FUNCTION service_catalog_items_check_contract_template_kind();

ALTER TABLE service_subscriptions
    ADD CONSTRAINT fk_service_subscriptions_contract
        FOREIGN KEY (contract_id, organization_id, brand_id)
        REFERENCES contract_instances (id, organization_id, brand_id) ON DELETE RESTRICT;

CREATE UNIQUE INDEX uq_contract_instances_service_subscription_subject
    ON contract_instances (subject_id)
    WHERE subject_type = 'service_subscription';
