-- TEC-495 (F5-05f): per user x organization report widget layout of the
-- /v1/reports contract (mobile home screen and panel widgets). The layout
-- is replaced atomically by PUT /v1/reports/layout; widgets is the ordered
-- list [{id, report, period, granularity}] validated by the reports
-- usecase against the report catalog. No row means the default layout
-- (overview only).
CREATE TABLE report_layouts (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    organization_id  BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    brand_id         BIGINT      NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    version          INT         NOT NULL DEFAULT 1,
    widgets          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_report_layouts_user_org UNIQUE (user_id, organization_id),
    CONSTRAINT chk_report_layouts_widgets CHECK (jsonb_typeof(widgets) = 'array'),
    CONSTRAINT chk_report_layouts_version CHECK (version > 0)
);

CREATE INDEX idx_report_layouts_org ON report_layouts (organization_id);
