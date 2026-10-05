-- TEC-309 rollback: remove the service subscription contract links while
-- keeping the base F3-01 contract schema intact.

DROP INDEX IF EXISTS uq_contract_instances_service_subscription_subject;

ALTER TABLE service_subscriptions
    DROP CONSTRAINT IF EXISTS fk_service_subscriptions_contract;

DROP TRIGGER IF EXISTS trg_service_catalog_items_check_contract_template_kind ON service_catalog_items;
DROP FUNCTION IF EXISTS service_catalog_items_check_contract_template_kind();

ALTER TABLE service_catalog_items
    DROP CONSTRAINT IF EXISTS fk_service_catalog_items_contract_template;

ALTER TABLE contract_instances
    DROP CONSTRAINT IF EXISTS chk_contract_instances_subject_type,
    ADD CONSTRAINT chk_contract_instances_subject_type
        CHECK (subject_type IN ('service', 'service_subscription'));
