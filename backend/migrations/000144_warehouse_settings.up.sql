-- TEC-769 (Depo F4.4): organization-level warehouse settings.
--
-- eod_cutoff_minutes is the end-of-day close time in minutes after local
-- midnight (organizations.timezone, K10); 1080 = 18:00. NULL keeps the
-- calendar day: the report of day D covers [D 00:00, D+1 00:00) and is
-- written after midnight. With a cutoff the business day D covers
-- [D-1 cutoff, D cutoff) and its report is written once the cutoff passed.
-- The warehouse side is brand-independent (K20): no brand_id.
CREATE TABLE warehouse_settings (
    organization_id     BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    eod_cutoff_minutes  SMALLINT     NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_warehouse_settings_eod_cutoff CHECK (eod_cutoff_minutes IS NULL OR eod_cutoff_minutes BETWEEN 0 AND 1439)
);

CREATE TRIGGER trg_warehouse_settings_set_updated_at
    BEFORE UPDATE ON warehouse_settings
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
