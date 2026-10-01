-- TEC-153 (F1-02a): stock ledger schema. Decisions: TEC-94 orchestrator
-- comment items 1-5.
--
--   * units: one physical unit (serial piece, roll) or one fixed barcode
--     (a single code standing for many identical pieces, decision 1).
--   * stock_movements: append-only ledger; every stock change is a row.
--   * unit_current_state (serial units: exactly one active owner, PK unit_id),
--     fixed_barcode_holdings (fixed units: quantity per owner), and the
--     bin/organization product stock projections are derived from the ledger
--     (rebuild, TEC-94d). The single write path is ledger.Post (TEC-154).
--   * warehouse_locations: minimal skeleton so ownership is protected by a
--     foreign key (decision 3); TEC-95 extends it.
--   * stock_reclassifications and the import staging tables (TEC-94e/f).
--
-- Ownership: units.organization_id is the issuing center and never changes
-- (decision 4, K14); the organization holding a unit right now is the
-- projection column holder_org_id, and scope filters use it. The warehouse
-- side is brand-independent (K20), so locations carry no brand_id.
--
-- Owners are polymorphic (owner_type + owner_id). Stored generated columns
-- turn them into real foreign keys: warehouse_location -> warehouse_locations
-- (with the holder org pinned to the location's org), organization and trash
-- -> organizations (holder = owner). service gets its FK with TEC-97.

-- 1. Warehouse locations (minimal; decision 3).
CREATE TABLE warehouse_locations (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    parent_id        BIGINT        NULL,
    code             VARCHAR(64)   NOT NULL,
    name             VARCHAR(200)  NOT NULL,
    active           BOOLEAN       NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_warehouse_locations_uuid UNIQUE (uuid),
    CONSTRAINT uq_warehouse_locations_id_org UNIQUE (id, organization_id),
    CONSTRAINT uq_warehouse_locations_org_code UNIQUE (organization_id, code),
    -- A parent location belongs to the same organization.
    CONSTRAINT fk_warehouse_locations_parent FOREIGN KEY (parent_id, organization_id)
        REFERENCES warehouse_locations (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_warehouse_locations_parent CHECK (parent_id IS NULL OR parent_id <> id),
    CONSTRAINT chk_warehouse_locations_code CHECK (btrim(code) <> ''),
    CONSTRAINT chk_warehouse_locations_name CHECK (btrim(name) <> '')
);

CREATE INDEX idx_warehouse_locations_parent ON warehouse_locations (parent_id);

CREATE TRIGGER trg_warehouse_locations_set_updated_at
    BEFORE UPDATE ON warehouse_locations
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 2. Units.
--
-- status is the lifecycle of the unit. reserved means "label reserved, not
-- printed yet" only (decision 2; order reservations live in TEC-96's
-- stock_reservations). For serial units the ledger keeps it equal to
-- unit_current_state.status; for fixed units it is the only status.
-- Meters are set for roll_meter products only (trigger below).
CREATE TABLE units (
    id                BIGSERIAL PRIMARY KEY,
    uuid              UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id   BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id          BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    product_id        BIGINT         NOT NULL,
    barcode           VARCHAR(64)    NOT NULL,
    unit_kind         VARCHAR(16)    NOT NULL DEFAULT 'serial',
    source            VARCHAR(16)    NOT NULL DEFAULT 'generated',
    status            VARCHAR(16)    NOT NULL DEFAULT 'reserved',
    initial_meters    NUMERIC(10,2)  NULL,
    remaining_meters  NUMERIC(10,2)  NULL,
    connection_id     BIGINT         NULL,
    external_id       VARCHAR(128)   NULL,
    external_status   VARCHAR(32)    NULL,
    created_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_units_uuid UNIQUE (uuid),
    CONSTRAINT uq_units_brand_barcode UNIQUE (brand_id, barcode),
    CONSTRAINT uq_units_id_brand UNIQUE (id, brand_id),
    -- Targets of the kind-pinned FKs from unit_current_state and
    -- fixed_barcode_holdings.
    CONSTRAINT uq_units_id_brand_kind UNIQUE (id, brand_id, unit_kind),
    -- A unit and its product always share the brand (K1).
    CONSTRAINT fk_units_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_units_barcode CHECK (btrim(barcode) <> ''),
    CONSTRAINT chk_units_kind CHECK (unit_kind IN ('serial', 'fixed')),
    CONSTRAINT chk_units_source CHECK (source IN ('generated', 'imported', 'external')),
    CONSTRAINT chk_units_status CHECK (status IN (
        'reserved', 'printed', 'available', 'placed', 'in_transit', 'used', 'void'
    )),
    CONSTRAINT chk_units_meters_pair CHECK ((initial_meters IS NULL) = (remaining_meters IS NULL)),
    CONSTRAINT chk_units_initial_meters CHECK (initial_meters IS NULL OR initial_meters >= 0),
    CONSTRAINT chk_units_remaining_meters CHECK (
        remaining_meters IS NULL OR (remaining_meters >= 0 AND remaining_meters <= initial_meters)
    ),
    CONSTRAINT chk_units_fixed_no_meters CHECK (unit_kind = 'serial' OR initial_meters IS NULL)
);

CREATE INDEX idx_units_product ON units (product_id);
CREATE INDEX idx_units_org_created ON units (organization_id, created_at);
CREATE UNIQUE INDEX uq_units_connection_external
    ON units (connection_id, external_id)
    WHERE connection_id IS NOT NULL AND external_id IS NOT NULL;

CREATE TRIGGER trg_units_set_updated_at
    BEFORE UPDATE ON units
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Units are issued by the center of their brand (K14) and the issuer never
-- changes (decision 4). The product decides the shape: fixed barcode
-- products get fixed units, roll_meter products carry meters, pieces none.
-- product_id may change (reclassification), so the shape is checked again.
CREATE FUNCTION units_check_integrity() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_type    TEXT;
    org_brand   BIGINT;
    p_unit_type TEXT;
    p_fixed     BOOLEAN;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF NEW.organization_id IS DISTINCT FROM OLD.organization_id
           OR NEW.brand_id IS DISTINCT FROM OLD.brand_id
           OR NEW.barcode IS DISTINCT FROM OLD.barcode
           OR NEW.unit_kind IS DISTINCT FROM OLD.unit_kind THEN
            RAISE EXCEPTION 'unit % issuer, brand, barcode and kind are immutable', OLD.id
                USING ERRCODE = 'check_violation';
        END IF;
    ELSE
        SELECT type, brand_id INTO org_type, org_brand
        FROM organizations WHERE id = NEW.organization_id;
        IF org_type IS DISTINCT FROM 'center' OR org_brand IS DISTINCT FROM NEW.brand_id THEN
            RAISE EXCEPTION 'unit issuer % must be the center organization of brand %',
                NEW.organization_id, NEW.brand_id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;

    SELECT unit_type, uses_fixed_barcode INTO p_unit_type, p_fixed
    FROM products WHERE id = NEW.product_id;
    IF p_fixed IS DISTINCT FROM (NEW.unit_kind = 'fixed') THEN
        RAISE EXCEPTION 'unit kind % does not match product % (uses_fixed_barcode=%)',
            NEW.unit_kind, NEW.product_id, p_fixed
            USING ERRCODE = 'check_violation';
    END IF;
    IF (p_unit_type = 'roll_meter') IS DISTINCT FROM (NEW.initial_meters IS NOT NULL) THEN
        RAISE EXCEPTION 'unit meters must be set exactly for roll_meter products (product %)',
            NEW.product_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_units_integrity
    BEFORE INSERT OR UPDATE ON units
    FOR EACH ROW
    EXECUTE FUNCTION units_check_integrity();

-- 3. Stock movements: append-only ledger.
--
-- quantity_delta / meters_delta are signed changes of the moved stock;
-- from/to owner and status describe the transition. idempotency_key makes a
-- retried command write nothing new (ledger.Post: ON CONFLICT DO NOTHING
-- then read the existing row).
CREATE TABLE stock_movements (
    id                BIGSERIAL PRIMARY KEY,
    uuid              UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id   BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id          BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    unit_id           BIGINT         NOT NULL,
    product_id        BIGINT         NOT NULL,
    type              VARCHAR(32)    NOT NULL,
    quantity_delta    INT            NOT NULL DEFAULT 0,
    meters_delta      NUMERIC(10,2)  NOT NULL DEFAULT 0,
    from_owner_type   VARCHAR(32)    NULL,
    from_owner_id     BIGINT         NULL,
    to_owner_type     VARCHAR(32)    NULL,
    to_owner_id       BIGINT         NULL,
    from_status       VARCHAR(16)    NULL,
    to_status         VARCHAR(16)    NULL,
    reference_type    VARCHAR(64)    NULL,
    reference_id      BIGINT         NULL,
    actor_user_id     BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    reason            TEXT           NULL,
    metadata          JSONB          NOT NULL DEFAULT '{}'::jsonb,
    idempotency_key   VARCHAR(128)   NOT NULL,
    created_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_movements_uuid UNIQUE (uuid),
    CONSTRAINT uq_stock_movements_idempotency_key UNIQUE (idempotency_key),
    CONSTRAINT fk_stock_movements_unit FOREIGN KEY (unit_id, brand_id)
        REFERENCES units (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_stock_movements_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_stock_movements_type CHECK (type IN (
        'entry', 'placement', 'transfer_out', 'transfer_in', 'transfer_cancel_restore',
        'order_out', 'order_cancel_restore', 'received', 'consumption', 'partial_consumption',
        'return', 'reclassification', 'count_adjustment', 'void', 'external_outbound'
    )),
    CONSTRAINT chk_stock_movements_from_owner CHECK (
        (from_owner_type IS NULL) = (from_owner_id IS NULL)
        AND (from_owner_type IS NULL
             OR from_owner_type IN ('warehouse_location', 'organization', 'service', 'trash'))
    ),
    CONSTRAINT chk_stock_movements_to_owner CHECK (
        (to_owner_type IS NULL) = (to_owner_id IS NULL)
        AND (to_owner_type IS NULL
             OR to_owner_type IN ('warehouse_location', 'organization', 'service', 'trash'))
    ),
    CONSTRAINT chk_stock_movements_from_status CHECK (from_status IS NULL OR from_status IN (
        'reserved', 'printed', 'available', 'placed', 'in_transit', 'used', 'void'
    )),
    CONSTRAINT chk_stock_movements_to_status CHECK (to_status IS NULL OR to_status IN (
        'reserved', 'printed', 'available', 'placed', 'in_transit', 'used', 'void'
    )),
    CONSTRAINT chk_stock_movements_reference CHECK ((reference_type IS NULL) = (reference_id IS NULL)),
    CONSTRAINT chk_stock_movements_idempotency_key CHECK (btrim(idempotency_key) <> ''),
    CONSTRAINT chk_stock_movements_metadata CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX idx_stock_movements_org_created ON stock_movements (organization_id, created_at);
CREATE INDEX idx_stock_movements_brand_created ON stock_movements (brand_id, created_at);
CREATE INDEX idx_stock_movements_unit_created ON stock_movements (unit_id, created_at, id);
CREATE INDEX idx_stock_movements_reference ON stock_movements (reference_type, reference_id)
    WHERE reference_type IS NOT NULL;

CREATE FUNCTION stock_movements_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'stock_movements is append-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation',
              HINT = 'Correct a movement with a reverse movement.';
END;
$$;

CREATE TRIGGER trg_stock_movements_append_only
    BEFORE UPDATE OR DELETE ON stock_movements
    FOR EACH ROW
    EXECUTE FUNCTION stock_movements_append_only();

CREATE TRIGGER trg_stock_movements_no_truncate
    BEFORE TRUNCATE ON stock_movements
    FOR EACH STATEMENT
    EXECUTE FUNCTION stock_movements_append_only();

-- 4. Current state of serial units: the primary key guarantees one active
-- owner per unit (decision 1). Fixed units cannot have a row here
-- (unit_kind is pinned to 'serial' through the FK).
CREATE TABLE unit_current_state (
    unit_id                BIGINT       PRIMARY KEY,
    brand_id               BIGINT       NOT NULL,
    unit_kind              VARCHAR(16)  NOT NULL DEFAULT 'serial',
    owner_type             VARCHAR(32)  NOT NULL,
    owner_id               BIGINT       NOT NULL,
    holder_org_id          BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    status                 VARCHAR(16)  NOT NULL,
    last_movement_id       BIGINT       NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    version                BIGINT       NOT NULL DEFAULT 1,
    owner_location_id      BIGINT       GENERATED ALWAYS AS (
        CASE WHEN owner_type = 'warehouse_location' THEN owner_id END
    ) STORED,
    owner_organization_id  BIGINT       GENERATED ALWAYS AS (
        CASE WHEN owner_type IN ('organization', 'trash') THEN owner_id END
    ) STORED,
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_unit_current_state_unit FOREIGN KEY (unit_id, brand_id, unit_kind)
        REFERENCES units (id, brand_id, unit_kind) ON DELETE RESTRICT,
    CONSTRAINT fk_unit_current_state_location FOREIGN KEY (owner_location_id, holder_org_id)
        REFERENCES warehouse_locations (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_unit_current_state_owner_org FOREIGN KEY (owner_organization_id)
        REFERENCES organizations (id) ON DELETE RESTRICT,
    CONSTRAINT chk_unit_current_state_kind CHECK (unit_kind = 'serial'),
    CONSTRAINT chk_unit_current_state_owner_type CHECK (owner_type IN (
        'warehouse_location', 'organization', 'service', 'trash'
    )),
    CONSTRAINT chk_unit_current_state_holder CHECK (
        owner_type NOT IN ('organization', 'trash') OR holder_org_id = owner_id
    ),
    CONSTRAINT chk_unit_current_state_status CHECK (status IN (
        'reserved', 'printed', 'available', 'placed', 'in_transit', 'used', 'void'
    )),
    CONSTRAINT chk_unit_current_state_placed CHECK (status <> 'placed' OR owner_type = 'warehouse_location'),
    CONSTRAINT chk_unit_current_state_used CHECK (status <> 'used' OR owner_type IN ('service', 'trash')),
    CONSTRAINT chk_unit_current_state_version CHECK (version >= 1)
);

CREATE INDEX idx_unit_current_state_holder ON unit_current_state (holder_org_id, status);
CREATE INDEX idx_unit_current_state_owner ON unit_current_state (owner_type, owner_id);

CREATE TRIGGER trg_unit_current_state_set_updated_at
    BEFORE UPDATE ON unit_current_state
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 5. Fixed barcode holdings: quantity on hand per (unit, owner) (decision
-- 1). A transfer is two movements (out of the source, into the target);
-- every increase is a movement too. Negative stock is rejected by CHECK.
CREATE TABLE fixed_barcode_holdings (
    id                     BIGSERIAL    PRIMARY KEY,
    unit_id                BIGINT       NOT NULL,
    brand_id               BIGINT       NOT NULL,
    unit_kind              VARCHAR(16)  NOT NULL DEFAULT 'fixed',
    owner_type             VARCHAR(32)  NOT NULL,
    owner_id               BIGINT       NOT NULL,
    holder_org_id          BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    quantity_on_hand       INT          NOT NULL DEFAULT 0,
    last_movement_id       BIGINT       NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    version                BIGINT       NOT NULL DEFAULT 1,
    owner_location_id      BIGINT       GENERATED ALWAYS AS (
        CASE WHEN owner_type = 'warehouse_location' THEN owner_id END
    ) STORED,
    owner_organization_id  BIGINT       GENERATED ALWAYS AS (
        CASE WHEN owner_type IN ('organization', 'trash') THEN owner_id END
    ) STORED,
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_fixed_barcode_holdings_owner UNIQUE (unit_id, owner_type, owner_id),
    CONSTRAINT fk_fixed_barcode_holdings_unit FOREIGN KEY (unit_id, brand_id, unit_kind)
        REFERENCES units (id, brand_id, unit_kind) ON DELETE RESTRICT,
    CONSTRAINT fk_fixed_barcode_holdings_location FOREIGN KEY (owner_location_id, holder_org_id)
        REFERENCES warehouse_locations (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_fixed_barcode_holdings_owner_org FOREIGN KEY (owner_organization_id)
        REFERENCES organizations (id) ON DELETE RESTRICT,
    CONSTRAINT chk_fixed_barcode_holdings_kind CHECK (unit_kind = 'fixed'),
    CONSTRAINT chk_fixed_barcode_holdings_owner_type CHECK (owner_type IN (
        'warehouse_location', 'organization', 'service', 'trash'
    )),
    CONSTRAINT chk_fixed_barcode_holdings_holder CHECK (
        owner_type NOT IN ('organization', 'trash') OR holder_org_id = owner_id
    ),
    CONSTRAINT chk_fixed_barcode_holdings_quantity CHECK (quantity_on_hand >= 0),
    CONSTRAINT chk_fixed_barcode_holdings_version CHECK (version >= 1)
);

CREATE INDEX idx_fixed_barcode_holdings_holder ON fixed_barcode_holdings (holder_org_id);
CREATE INDEX idx_fixed_barcode_holdings_owner ON fixed_barcode_holdings (owner_type, owner_id);

CREATE TRIGGER trg_fixed_barcode_holdings_set_updated_at
    BEFORE UPDATE ON fixed_barcode_holdings
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 6. Product stock projections (derived; rebuild recreates them).
CREATE TABLE bin_product_stocks (
    location_id      BIGINT         NOT NULL,
    organization_id  BIGINT         NOT NULL,
    brand_id         BIGINT         NOT NULL,
    product_id       BIGINT         NOT NULL,
    quantity         INT            NOT NULL DEFAULT 0,
    meters           NUMERIC(14,2)  NOT NULL DEFAULT 0,
    updated_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    PRIMARY KEY (location_id, product_id),
    CONSTRAINT fk_bin_product_stocks_location FOREIGN KEY (location_id, organization_id)
        REFERENCES warehouse_locations (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_bin_product_stocks_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_bin_product_stocks_quantity CHECK (quantity >= 0),
    CONSTRAINT chk_bin_product_stocks_meters CHECK (meters >= 0)
);

CREATE INDEX idx_bin_product_stocks_org ON bin_product_stocks (organization_id, product_id);

CREATE TRIGGER trg_bin_product_stocks_set_updated_at
    BEFORE UPDATE ON bin_product_stocks
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE organization_product_stocks (
    organization_id  BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    product_id       BIGINT         NOT NULL,
    brand_id         BIGINT         NOT NULL,
    quantity         INT            NOT NULL DEFAULT 0,
    meters           NUMERIC(14,2)  NOT NULL DEFAULT 0,
    updated_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, product_id),
    CONSTRAINT fk_organization_product_stocks_product FOREIGN KEY (product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_organization_product_stocks_quantity CHECK (quantity >= 0),
    CONSTRAINT chk_organization_product_stocks_meters CHECK (meters >= 0)
);

CREATE INDEX idx_organization_product_stocks_brand ON organization_product_stocks (brand_id, product_id);

CREATE TRIGGER trg_organization_product_stocks_set_updated_at
    BEFORE UPDATE ON organization_product_stocks
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 7. Reclassification requests (wrong product on a unit; TEC-94e). The
-- approved request points at its reclassification movement.
CREATE TABLE stock_reclassifications (
    id                    BIGSERIAL    PRIMARY KEY,
    uuid                  UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id       BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id              BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    unit_id               BIGINT       NOT NULL,
    from_product_id       BIGINT       NOT NULL,
    to_product_id         BIGINT       NOT NULL,
    reason                TEXT         NOT NULL,
    status                VARCHAR(16)  NOT NULL DEFAULT 'pending',
    requested_by_user_id  BIGINT       NULL REFERENCES users (id) ON DELETE RESTRICT,
    decided_by_user_id    BIGINT       NULL REFERENCES users (id) ON DELETE RESTRICT,
    decided_at            TIMESTAMPTZ  NULL,
    decision_note         TEXT         NULL,
    movement_id           BIGINT       NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_reclassifications_uuid UNIQUE (uuid),
    CONSTRAINT fk_stock_reclassifications_unit FOREIGN KEY (unit_id, brand_id)
        REFERENCES units (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_stock_reclassifications_from_product FOREIGN KEY (from_product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT fk_stock_reclassifications_to_product FOREIGN KEY (to_product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_stock_reclassifications_products CHECK (from_product_id <> to_product_id),
    CONSTRAINT chk_stock_reclassifications_reason CHECK (btrim(reason) <> ''),
    CONSTRAINT chk_stock_reclassifications_status CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    CONSTRAINT chk_stock_reclassifications_decided CHECK ((status = 'pending') = (decided_at IS NULL)),
    CONSTRAINT chk_stock_reclassifications_movement CHECK ((status = 'approved') = (movement_id IS NOT NULL))
);

-- At most one open request per unit.
CREATE UNIQUE INDEX uq_stock_reclassifications_pending
    ON stock_reclassifications (unit_id)
    WHERE status = 'pending';
CREATE INDEX idx_stock_reclassifications_org_status
    ON stock_reclassifications (organization_id, status, created_at);

CREATE TRIGGER trg_stock_reclassifications_set_updated_at
    BEFORE UPDATE ON stock_reclassifications
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 8. Stock import staging (TEC-94f): a batch is parsed into rows, validated
-- (dry run), then applied as movements; undo writes reverse movements.
CREATE TABLE stock_import_batches (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    status              VARCHAR(16)   NOT NULL DEFAULT 'draft',
    source_filename     VARCHAR(255)  NULL,
    target_owner_type   VARCHAR(32)   NULL,
    target_owner_id     BIGINT        NULL,
    rows_total          INT           NOT NULL DEFAULT 0,
    rows_new            INT           NOT NULL DEFAULT 0,
    rows_duplicate      INT           NOT NULL DEFAULT 0,
    rows_invalid        INT           NOT NULL DEFAULT 0,
    rows_conflict       INT           NOT NULL DEFAULT 0,
    error               TEXT          NULL,
    applied_at          TIMESTAMPTZ   NULL,
    undone_at           TIMESTAMPTZ   NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_import_batches_uuid UNIQUE (uuid),
    CONSTRAINT chk_stock_import_batches_status CHECK (status IN (
        'draft', 'validated', 'applying', 'applied', 'failed', 'undone'
    )),
    CONSTRAINT chk_stock_import_batches_target CHECK (
        (target_owner_type IS NULL) = (target_owner_id IS NULL)
        AND (target_owner_type IS NULL
             OR target_owner_type IN ('warehouse_location', 'organization'))
    ),
    CONSTRAINT chk_stock_import_batches_counts CHECK (
        rows_total >= 0 AND rows_new >= 0 AND rows_duplicate >= 0
        AND rows_invalid >= 0 AND rows_conflict >= 0
    )
);

CREATE INDEX idx_stock_import_batches_org_created ON stock_import_batches (organization_id, created_at);

CREATE TRIGGER trg_stock_import_batches_set_updated_at
    BEFORE UPDATE ON stock_import_batches
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Rows inherit organization and brand from their batch.
CREATE TABLE stock_import_rows (
    id                 BIGSERIAL      PRIMARY KEY,
    batch_id           BIGINT         NOT NULL REFERENCES stock_import_batches (id) ON DELETE CASCADE,
    row_number         INT            NOT NULL,
    barcode            VARCHAR(64)    NULL,
    product_sku        VARCHAR(64)    NULL,
    product_id         BIGINT         NULL REFERENCES products (id) ON DELETE RESTRICT,
    quantity           INT            NULL,
    meters             NUMERIC(10,2)  NULL,
    target_owner_type  VARCHAR(32)    NULL,
    target_owner_id    BIGINT         NULL,
    raw                JSONB          NOT NULL DEFAULT '{}'::jsonb,
    row_status         VARCHAR(16)    NOT NULL DEFAULT 'new',
    errors             JSONB          NOT NULL DEFAULT '[]'::jsonb,
    unit_id            BIGINT         NULL REFERENCES units (id) ON DELETE RESTRICT,
    movement_id        BIGINT         NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    created_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_import_rows_batch_row UNIQUE (batch_id, row_number),
    CONSTRAINT chk_stock_import_rows_row_number CHECK (row_number >= 1),
    CONSTRAINT chk_stock_import_rows_status CHECK (row_status IN (
        'new', 'duplicate', 'invalid', 'conflict', 'applied', 'skipped'
    )),
    CONSTRAINT chk_stock_import_rows_quantity CHECK (quantity IS NULL OR quantity >= 0),
    CONSTRAINT chk_stock_import_rows_meters CHECK (meters IS NULL OR meters >= 0),
    CONSTRAINT chk_stock_import_rows_target CHECK (
        (target_owner_type IS NULL) = (target_owner_id IS NULL)
        AND (target_owner_type IS NULL
             OR target_owner_type IN ('warehouse_location', 'organization'))
    ),
    CONSTRAINT chk_stock_import_rows_raw CHECK (jsonb_typeof(raw) = 'object'),
    CONSTRAINT chk_stock_import_rows_errors CHECK (jsonb_typeof(errors) = 'array')
);

CREATE INDEX idx_stock_import_rows_batch_status ON stock_import_rows (batch_id, row_status);

CREATE TRIGGER trg_stock_import_rows_set_updated_at
    BEFORE UPDATE ON stock_import_rows
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 9. Permissions. Source of truth: internal/platform/rbac/catalog.go
-- (appended last, so sort_order continues after the current maximum).
-- center_warehouse holds stock.* at scope all: the warehouse is
-- brand-independent (K20). Distributor warehouse roles act on their own
-- organization (managed); the dealer owner reads its simple stock (K12).
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read stock', 'stock.read', 'stock', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Units, barcode history and stock summaries of the organization.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Write stock', 'stock.write', 'stock', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Stock entry, placement, transfers and receipts.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Adjust stock', 'stock.adjust', 'stock', ARRAY['managed', 'subtree', 'brand', 'all']::text[], true, false,
       'Count adjustments, voids and ledger repairs. Requires step-up.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Reclassify stock', 'stock.reclassify', 'stock', ARRAY['brand', 'all']::text[], true, false,
       'Approve moving a unit to another product (center only). Requires step-up.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Import stock', 'stock.import', 'stock', ARRAY['brand', 'all']::text[], false, false,
       'Bulk stock import with dry run and undo (center only, K14).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'stock.read', 'all'),
    ('super_admin', 'stock.write', 'all'),
    ('super_admin', 'stock.adjust', 'all'),
    ('super_admin', 'stock.reclassify', 'all'),
    ('super_admin', 'stock.import', 'all'),
    ('center_warehouse', 'stock.read', 'all'),
    ('center_warehouse', 'stock.write', 'all'),
    ('center_warehouse', 'stock.adjust', 'all'),
    ('center_warehouse', 'stock.reclassify', 'all'),
    ('center_warehouse', 'stock.import', 'all'),
    ('distributor_owner', 'stock.read', 'managed'),
    ('distributor_owner', 'stock.write', 'managed'),
    ('distributor_owner', 'stock.adjust', 'managed'),
    ('distributor_warehouse_staff', 'stock.read', 'managed'),
    ('distributor_warehouse_staff', 'stock.write', 'managed'),
    ('distributor_warehouse_staff', 'stock.adjust', 'managed'),
    ('dealer_owner', 'stock.read', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
