-- TEC-230: consumption correction of a completed service (conservative
-- default until the "cancel a completed service" decision is made).
--
--   * One row per corrected service item (UNIQUE service_item_id): the unit
--     consumed by mistake went back into the service organization's stock
--     with a ledger return movement (return_movement_id) and, optionally,
--     the correct unit of the same product was consumed instead
--     (replacement_unit_id / replacement_movement_id). service_items stay
--     locked (completed is final); this table is the record of the change.
--   * The composite FK copies the item's service, organization, brand,
--     product, unit and kind (same reference as the warranties FK), so a
--     correction cannot drift from its item.
--   * Append-only, like the ledger. No accounting row is touched.
--   * The warranty listener and its repair scan skip corrected items, so a
--     returned unit never gets a warranty afterwards.
CREATE TABLE service_item_corrections (
    id                       BIGSERIAL PRIMARY KEY,
    uuid                     UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id          BIGINT        NOT NULL,
    brand_id                 BIGINT        NOT NULL,
    service_id               BIGINT        NOT NULL,
    service_item_id          BIGINT        NOT NULL,
    product_id               BIGINT        NOT NULL,
    unit_id                  BIGINT        NOT NULL,
    item_kind                VARCHAR(16)   NOT NULL,
    return_movement_id       BIGINT        NOT NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    replacement_unit_id      BIGINT        NULL,
    replacement_movement_id  BIGINT        NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    reason                   TEXT          NOT NULL,
    created_by_user_id       BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    actor_org_id             BIGINT        NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    created_at               TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_service_item_corrections_uuid UNIQUE (uuid),
    CONSTRAINT uq_service_item_corrections_item UNIQUE (service_item_id),
    CONSTRAINT uq_service_item_corrections_return UNIQUE (return_movement_id),
    CONSTRAINT fk_service_item_corrections_item
        FOREIGN KEY (service_item_id, service_id, organization_id, brand_id, product_id, unit_id, item_kind)
        REFERENCES service_items (id, service_id, organization_id, brand_id, product_id, unit_id, kind)
        ON DELETE RESTRICT,
    CONSTRAINT fk_service_item_corrections_replacement
        FOREIGN KEY (replacement_unit_id, brand_id) REFERENCES units (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_service_item_corrections_replacement CHECK (
        (replacement_unit_id IS NULL) = (replacement_movement_id IS NULL)
        AND (replacement_unit_id IS NULL OR replacement_unit_id <> unit_id)
    ),
    CONSTRAINT chk_service_item_corrections_reason CHECK (
        btrim(reason) <> '' AND char_length(reason) <= 1000
    )
);

CREATE INDEX idx_service_item_corrections_service ON service_item_corrections (service_id, id);
CREATE INDEX idx_service_item_corrections_org_created ON service_item_corrections (organization_id, created_at);

CREATE FUNCTION service_item_corrections_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'service_item_corrections is append-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$;

CREATE TRIGGER trg_service_item_corrections_append_only
    BEFORE UPDATE OR DELETE ON service_item_corrections
    FOR EACH ROW
    EXECUTE FUNCTION service_item_corrections_append_only();

CREATE TRIGGER trg_service_item_corrections_no_truncate
    BEFORE TRUNCATE ON service_item_corrections
    FOR EACH STATEMENT
    EXECUTE FUNCTION service_item_corrections_append_only();
