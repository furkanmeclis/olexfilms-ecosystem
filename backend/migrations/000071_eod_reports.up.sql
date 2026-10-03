-- TEC-207 (F1-03g): end-of-day warehouse reports.
--
-- One row per (organization, warehouse, local day). warehouse_id NULL is
-- the system report: every warehouse of the organization plus the
-- organization-level movements (serial entries before placement, orders,
-- service consumption). The summary is derived from stock_movements of the
-- day in the organization's time zone (the ledger stays the single
-- source). It is written by the hourly cron (kind = 'auto': the previous
-- local day of every center / distributor, insert-only, so reruns write
-- nothing) or on demand (kind = 'manual': regenerates the row). The PDF is
-- rendered from the stored summary by an export job on worker-docs.
--
-- The warehouse side is brand-independent (K20); brand_id is the
-- organization's brand, kept for the business-table convention.

CREATE TABLE eod_reports (
    id                    BIGSERIAL     PRIMARY KEY,
    uuid                  UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id       BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id              BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    warehouse_id          BIGINT        NULL,
    report_date           DATE          NOT NULL,
    timezone              VARCHAR(64)   NOT NULL,
    period_start          TIMESTAMPTZ   NOT NULL,
    period_end            TIMESTAMPTZ   NOT NULL,
    kind                  VARCHAR(16)   NOT NULL,
    summary               JSONB         NOT NULL DEFAULT '{}'::jsonb,
    generated_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    generated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    created_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_eod_reports_uuid UNIQUE (uuid),
    CONSTRAINT uq_eod_reports_scope UNIQUE NULLS NOT DISTINCT (organization_id, warehouse_id, report_date),
    -- A report is derived data: deleting an (empty) warehouse drops its reports.
    CONSTRAINT fk_eod_reports_warehouse FOREIGN KEY (warehouse_id, organization_id)
        REFERENCES warehouses (id, organization_id) ON DELETE CASCADE,
    CONSTRAINT chk_eod_reports_kind CHECK (kind IN ('auto', 'manual')),
    CONSTRAINT chk_eod_reports_period CHECK (period_end > period_start)
);

CREATE INDEX idx_eod_reports_org_date ON eod_reports (organization_id, report_date DESC, id DESC);

CREATE TRIGGER trg_eod_reports_set_updated_at
    BEFORE UPDATE ON eod_reports
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- The day's movements are read by time first (a movement may concern the
-- organization through its target location, not only through
-- organization_id), so the summary scans one day of the ledger.
CREATE INDEX idx_stock_movements_created ON stock_movements (created_at);
