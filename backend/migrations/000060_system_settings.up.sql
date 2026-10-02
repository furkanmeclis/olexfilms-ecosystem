-- TEC-215 (F1-11c): global system settings store.
--
-- One row per setting key; the value is JSONB so a key may hold a number,
-- a boolean, a string or an object. Rows are global (no organization_id /
-- brand_id): this is platform configuration, not business data. Defaults
-- live in Go (platform/sysconfig catalog); a missing row means "default".
-- schema_version records which catalog schema validated the value, so a
-- later catalog change can migrate or ignore stale rows.
CREATE TABLE system_settings (
    key            VARCHAR(100) PRIMARY KEY,
    value          JSONB        NOT NULL,
    schema_version INTEGER      NOT NULL DEFAULT 1,
    updated_by     BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_system_settings_key CHECK (key ~ '^[a-z0-9_]+(\.[a-z0-9_]+)*$'),
    CONSTRAINT chk_system_settings_schema_version CHECK (schema_version >= 1)
);

CREATE TRIGGER trg_system_settings_set_updated_at
    BEFORE UPDATE ON system_settings
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
