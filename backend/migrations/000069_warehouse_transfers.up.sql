-- TEC-205 (F1-03e): warehouse -> warehouse transfer documents.
--
-- A warehouse transfer moves serial units between two warehouses of the
-- same organization (the center's or a distributor's own warehouses, K4):
--
--   draft -> in_transit -> completed
--     |          |
--     +----------+-> cancelled
--
--   * draft: lines are added by scan; nothing moves yet. A unit is on at
--     most one open (draft or in_transit) warehouse transfer
--     (uq_warehouse_transfer_lines_open_unit) and never on one while an
--     order reserves it or a stock transfer request holds it.
--   * ship (draft -> in_transit): one ledger transfer_out per line, the
--     unit goes in transit owned by the organization itself; an in-transit
--     unit cannot be picked by any other flow (ledger status + open line).
--   * complete (in_transit -> completed): the paired transfer_in into the
--     organization and a placement onto a location of the target warehouse.
--   * cancel: a draft closes without stock; an in-transit transfer posts
--     transfer_cancel_restore per line (the unit goes back to its source
--     location and status).
--
-- Idempotency keys: warehouse_transfer:warehouse_transfer_line:<line id>:
-- <type>:<barcode>. Bin -> bin moves need no document (single placement).
-- Transfers between organizations live in the transfers module (TEC-197);
-- this table never crosses organizations.

CREATE SEQUENCE warehouse_transfer_no_seq;

CREATE TABLE warehouse_transfers (
    id                    BIGSERIAL     PRIMARY KEY,
    uuid                  UUID          NOT NULL DEFAULT gen_random_uuid(),
    transfer_no           VARCHAR(32)   NOT NULL
        DEFAULT ('WT-' || lpad(nextval('warehouse_transfer_no_seq')::text, 8, '0')),
    organization_id       BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id              BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    from_warehouse_id     BIGINT        NOT NULL,
    to_warehouse_id       BIGINT        NOT NULL,
    -- Default target location (optional; each line may carry its own).
    to_location_id        BIGINT        NULL,
    status                VARCHAR(16)   NOT NULL DEFAULT 'draft',
    note                  VARCHAR(500)  NULL,
    created_by_user_id    BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    shipped_by_user_id    BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    completed_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    cancelled_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    shipped_at            TIMESTAMPTZ   NULL,
    completed_at          TIMESTAMPTZ   NULL,
    cancelled_at          TIMESTAMPTZ   NULL,
    created_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_warehouse_transfers_uuid UNIQUE (uuid),
    CONSTRAINT uq_warehouse_transfers_no UNIQUE (transfer_no),
    CONSTRAINT fk_warehouse_transfers_from FOREIGN KEY (from_warehouse_id, organization_id)
        REFERENCES warehouses (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_warehouse_transfers_to FOREIGN KEY (to_warehouse_id, organization_id)
        REFERENCES warehouses (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT fk_warehouse_transfers_to_location FOREIGN KEY (to_location_id, organization_id)
        REFERENCES warehouse_locations (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_warehouse_transfers_distinct CHECK (from_warehouse_id <> to_warehouse_id),
    CONSTRAINT chk_warehouse_transfers_status CHECK (status IN ('draft', 'in_transit', 'completed', 'cancelled')),
    CONSTRAINT chk_warehouse_transfers_shipped CHECK (
        status IN ('draft', 'cancelled') OR shipped_at IS NOT NULL
    ),
    CONSTRAINT chk_warehouse_transfers_completed CHECK ((status = 'completed') = (completed_at IS NOT NULL)),
    CONSTRAINT chk_warehouse_transfers_cancelled CHECK ((status = 'cancelled') = (cancelled_at IS NOT NULL)),
    CONSTRAINT chk_warehouse_transfers_note CHECK (note IS NULL OR btrim(note) <> '')
);

CREATE INDEX idx_warehouse_transfers_org_created ON warehouse_transfers (organization_id, created_at DESC, id DESC);

CREATE TRIGGER trg_warehouse_transfers_set_updated_at
    BEFORE UPDATE ON warehouse_transfers
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE warehouse_transfer_lines (
    id                     BIGSERIAL     PRIMARY KEY,
    uuid                   UUID          NOT NULL DEFAULT gen_random_uuid(),
    transfer_id            BIGINT        NOT NULL REFERENCES warehouse_transfers (id) ON DELETE CASCADE,
    unit_id                BIGINT        NOT NULL REFERENCES units (id) ON DELETE RESTRICT,
    -- Location the unit left (the transfer_out source, set at ship).
    source_location_id     BIGINT        NULL REFERENCES warehouse_locations (id) ON DELETE RESTRICT,
    -- Location in the target warehouse (set before or at completion).
    target_location_id     BIGINT        NULL REFERENCES warehouse_locations (id) ON DELETE RESTRICT,
    -- TRUE while the transfer is draft or in_transit: the unit lock.
    is_open                BOOLEAN       NOT NULL DEFAULT TRUE,
    out_movement_id        BIGINT        NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    in_movement_id         BIGINT        NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    placement_movement_id  BIGINT        NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    restore_movement_id    BIGINT        NULL REFERENCES stock_movements (id) ON DELETE RESTRICT,
    created_at             TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_warehouse_transfer_lines_uuid UNIQUE (uuid),
    CONSTRAINT uq_warehouse_transfer_lines_transfer_unit UNIQUE (transfer_id, unit_id)
);

CREATE INDEX idx_warehouse_transfer_lines_transfer ON warehouse_transfer_lines (transfer_id, id);
-- A unit is on at most one open warehouse transfer.
CREATE UNIQUE INDEX uq_warehouse_transfer_lines_open_unit ON warehouse_transfer_lines (unit_id) WHERE is_open;
