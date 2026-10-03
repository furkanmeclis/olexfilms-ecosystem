-- TEC-206 (F1-03f): stock counts.
--
--   draft -> (initial_placement: start approval) -> in_progress (scans)
--         -> pending_review (complete: lines = differences) -> approved
--   any open state -> cancelled
--
-- A count belongs to one warehouse of the active organization (center or
-- distributor, K12) and covers a scope: the whole warehouse, a room, a
-- location subtree or one product inside the warehouse. Scans outside the
-- scope are refused. Methods:
--
--   location_first     scan a location, then its units (location required);
--   unit_first         scan units, the location is optional per scan;
--   product_qty        scan SKUs / fixed barcodes with a quantity per location;
--   initial_placement  place the organization's unlocated units into the
--                      scope's locations (needs a start approval).
--
-- visibility: blind hides the expected values until the count is completed;
-- guided shows them while counting.
--
-- Completing builds stock_count_lines (result: matched, missing,
-- wrong_location, unlocated, unexpected, qty_variance, meter_variance) and
-- touches no stock. Approving applies each line's resolution through
-- ledger.Post in one transaction with the idempotency keys
-- stock_count:stock_count_line:<line id>:<type>:<barcode>:
--   void_missing        -> void (missing serial unit);
--   relocate            -> placement (+ count_adjustment of roll meters);
--   increase_unlocated  -> count_adjustment (fixed quantity / roll meters);
--   ignore              -> nothing.
-- No new movement type: the existing placement, void and count_adjustment
-- cover every resolution, so replay/rebuild are unchanged.
--
-- The warehouse side is brand-independent (K20): scans and lines may hold
-- units of any brand in the warehouse; the count carries the active
-- organization's brand (the product scope is a product of that brand, K1).

CREATE TABLE stock_counts (
    id                         BIGSERIAL     PRIMARY KEY,
    uuid                       UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id            BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id                   BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    warehouse_id               BIGINT        NOT NULL,
    method                     VARCHAR(24)   NOT NULL,
    visibility                 VARCHAR(8)    NOT NULL,
    scope_type                 VARCHAR(16)   NOT NULL,
    scope_room_id              BIGINT        NULL,
    scope_location_id          BIGINT        NULL,
    scope_product_id           BIGINT        NULL,
    status                     VARCHAR(16)   NOT NULL DEFAULT 'draft',
    note                       VARCHAR(500)  NULL,
    created_by_user_id         BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    start_approved_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    start_approved_at          TIMESTAMPTZ   NULL,
    started_by_user_id         BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    started_at                 TIMESTAMPTZ   NULL,
    completed_by_user_id       BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    completed_at               TIMESTAMPTZ   NULL,
    approved_by_user_id        BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    approved_at                TIMESTAMPTZ   NULL,
    cancelled_at               TIMESTAMPTZ   NULL,
    created_at                 TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_counts_uuid UNIQUE (uuid),
    CONSTRAINT fk_stock_counts_warehouse FOREIGN KEY (warehouse_id, organization_id)
        REFERENCES warehouses (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_stock_counts_room FOREIGN KEY (scope_room_id, warehouse_id)
        REFERENCES rooms (id, warehouse_id) ON DELETE RESTRICT,
    CONSTRAINT fk_stock_counts_location FOREIGN KEY (scope_location_id, organization_id)
        REFERENCES warehouse_locations (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_stock_counts_product FOREIGN KEY (scope_product_id, brand_id)
        REFERENCES products (id, brand_id) ON DELETE RESTRICT,
    CONSTRAINT chk_stock_counts_method CHECK (method IN ('location_first', 'unit_first', 'product_qty', 'initial_placement')),
    CONSTRAINT chk_stock_counts_visibility CHECK (visibility IN ('blind', 'guided')),
    CONSTRAINT chk_stock_counts_status CHECK (status IN ('draft', 'in_progress', 'pending_review', 'approved', 'cancelled')),
    CONSTRAINT chk_stock_counts_scope CHECK (
        (scope_type = 'warehouse' AND scope_room_id IS NULL AND scope_location_id IS NULL AND scope_product_id IS NULL)
        OR (scope_type = 'room' AND scope_room_id IS NOT NULL AND scope_location_id IS NULL AND scope_product_id IS NULL)
        OR (scope_type = 'location' AND scope_room_id IS NULL AND scope_location_id IS NOT NULL AND scope_product_id IS NULL)
        OR (scope_type = 'product' AND scope_room_id IS NULL AND scope_location_id IS NULL AND scope_product_id IS NOT NULL)
    ),
    -- Initial placement puts unlocated units somewhere: no product scope.
    CONSTRAINT chk_stock_counts_initial_scope CHECK (method <> 'initial_placement' OR scope_type <> 'product'),
    CONSTRAINT chk_stock_counts_completed CHECK (status NOT IN ('pending_review', 'approved') OR completed_at IS NOT NULL),
    CONSTRAINT chk_stock_counts_approved CHECK ((status = 'approved') = (approved_at IS NOT NULL)),
    CONSTRAINT chk_stock_counts_note CHECK (note IS NULL OR btrim(note) <> '')
);

CREATE INDEX idx_stock_counts_org_created ON stock_counts (organization_id, created_at DESC, id DESC);
CREATE INDEX idx_stock_counts_warehouse_status ON stock_counts (warehouse_id, status);

CREATE TRIGGER trg_stock_counts_set_updated_at
    BEFORE UPDATE ON stock_counts
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Raw scans. A location scan sets the scanning user's location context
-- (location_first); a serial unit is scanned at most once per count; fixed
-- barcodes and products add up their quantities.
CREATE TABLE stock_count_scans (
    id                  BIGSERIAL      PRIMARY KEY,
    uuid                UUID           NOT NULL DEFAULT gen_random_uuid(),
    count_id            BIGINT         NOT NULL REFERENCES stock_counts (id) ON DELETE CASCADE,
    kind                VARCHAR(16)    NOT NULL,
    raw_code            VARCHAR(200)   NOT NULL,
    unit_id             BIGINT         NULL REFERENCES units (id) ON DELETE RESTRICT,
    product_id          BIGINT         NULL REFERENCES products (id) ON DELETE RESTRICT,
    location_id         BIGINT         NULL REFERENCES warehouse_locations (id) ON DELETE RESTRICT,
    quantity            INTEGER        NOT NULL DEFAULT 1,
    meters              NUMERIC(10,2)  NULL,
    scanned_by_user_id  BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_count_scans_uuid UNIQUE (uuid),
    CONSTRAINT chk_stock_count_scans_kind CHECK (
        (kind = 'location' AND location_id IS NOT NULL AND unit_id IS NULL AND product_id IS NULL)
        OR (kind IN ('serial', 'fixed') AND unit_id IS NOT NULL AND product_id IS NOT NULL)
        OR (kind = 'product' AND unit_id IS NULL AND product_id IS NOT NULL)
    ),
    CONSTRAINT chk_stock_count_scans_quantity CHECK (quantity >= 1 AND (kind <> 'serial' OR quantity = 1)),
    CONSTRAINT chk_stock_count_scans_meters CHECK (meters IS NULL OR (kind = 'serial' AND meters >= 0))
);

CREATE INDEX idx_stock_count_scans_count ON stock_count_scans (count_id, id);
CREATE UNIQUE INDEX uq_stock_count_scans_serial ON stock_count_scans (count_id, unit_id) WHERE kind = 'serial';

-- Differences built at completion; the resolution is chosen at approval.
CREATE TABLE stock_count_lines (
    id                    BIGSERIAL      PRIMARY KEY,
    uuid                  UUID           NOT NULL DEFAULT gen_random_uuid(),
    count_id              BIGINT         NOT NULL REFERENCES stock_counts (id) ON DELETE CASCADE,
    line_kind             VARCHAR(16)    NOT NULL,
    unit_id               BIGINT         NULL REFERENCES units (id) ON DELETE RESTRICT,
    product_id            BIGINT         NOT NULL REFERENCES products (id) ON DELETE RESTRICT,
    -- NULL: held by the organization itself (unlocated).
    expected_location_id  BIGINT         NULL REFERENCES warehouse_locations (id) ON DELETE RESTRICT,
    counted_location_id   BIGINT         NULL REFERENCES warehouse_locations (id) ON DELETE RESTRICT,
    expected_quantity     INTEGER        NOT NULL DEFAULT 0,
    counted_quantity      INTEGER        NOT NULL DEFAULT 0,
    expected_meters       NUMERIC(10,2)  NULL,
    counted_meters        NUMERIC(10,2)  NULL,
    result                VARCHAR(16)    NOT NULL,
    resolution            VARCHAR(24)    NULL,
    note                  VARCHAR(500)   NULL,
    resolved_at           TIMESTAMPTZ    NULL,
    created_at            TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_stock_count_lines_uuid UNIQUE (uuid),
    CONSTRAINT chk_stock_count_lines_kind CHECK (
        (line_kind IN ('serial', 'fixed') AND unit_id IS NOT NULL)
        OR (line_kind = 'product' AND unit_id IS NULL)
    ),
    CONSTRAINT chk_stock_count_lines_result CHECK (result IN (
        'matched', 'missing', 'wrong_location', 'unlocated', 'unexpected', 'qty_variance', 'meter_variance'
    )),
    CONSTRAINT chk_stock_count_lines_resolution CHECK (resolution IS NULL OR resolution IN (
        'ignore', 'relocate', 'void_missing', 'increase_unlocated'
    )),
    CONSTRAINT chk_stock_count_lines_quantities CHECK (expected_quantity >= 0 AND counted_quantity >= 0),
    CONSTRAINT chk_stock_count_lines_note CHECK (note IS NULL OR btrim(note) <> '')
);

CREATE INDEX idx_stock_count_lines_count ON stock_count_lines (count_id, id);
