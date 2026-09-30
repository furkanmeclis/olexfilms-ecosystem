CREATE TABLE app_logs (
    id         BIGSERIAL PRIMARY KEY,
    uuid       UUID         NOT NULL DEFAULT gen_random_uuid(),
    level      TEXT         NOT NULL,
    message    TEXT         NOT NULL,
    source     TEXT         NOT NULL DEFAULT '',
    attrs      JSONB        NOT NULL DEFAULT '{}'::jsonb,
    request_id TEXT         NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT app_logs_uuid_uq UNIQUE (uuid),
    CONSTRAINT app_logs_level_chk CHECK (level IN ('debug', 'warn', 'error'))
);

CREATE INDEX app_logs_created_idx ON app_logs (created_at DESC);
CREATE INDEX app_logs_level_created_idx ON app_logs (level, created_at DESC);
CREATE INDEX app_logs_source_created_idx ON app_logs (source, created_at DESC);

CREATE TABLE log_purge_rules (
    id                 BIGSERIAL PRIMARY KEY,
    uuid               UUID         NOT NULL DEFAULT gen_random_uuid(),
    name               TEXT         NOT NULL,
    enabled            BOOLEAN      NOT NULL DEFAULT TRUE,
    is_system          BOOLEAN      NOT NULL DEFAULT FALSE,
    levels             TEXT[]       NOT NULL DEFAULT '{}'::text[],
    source             TEXT         NULL,
    message_contains   TEXT         NULL,
    older_than_hours   INTEGER      NOT NULL,
    interval_minutes   INTEGER      NOT NULL,
    last_run_at        TIMESTAMPTZ  NULL,
    last_deleted_count BIGINT       NOT NULL DEFAULT 0,
    last_error         TEXT         NULL,
    created_by         BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT log_purge_rules_uuid_uq UNIQUE (uuid),
    CONSTRAINT log_purge_rules_name_chk CHECK (char_length(name) BETWEEN 1 AND 120),
    CONSTRAINT log_purge_rules_older_chk CHECK (older_than_hours >= 1 AND older_than_hours <= 43800),
    CONSTRAINT log_purge_rules_interval_chk CHECK (
        interval_minutes IN (5, 15, 30, 60, 180, 360, 720, 1440, 10080)
    )
);

CREATE INDEX log_purge_rules_enabled_idx ON log_purge_rules (enabled)
    WHERE enabled = TRUE;

CREATE TRIGGER trg_log_purge_rules_set_updated_at
    BEFORE UPDATE ON log_purge_rules
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

INSERT INTO log_purge_rules (name, enabled, is_system, levels, older_than_hours, interval_minutes)
VALUES
    ('Drop debug after 7 days', TRUE, TRUE, ARRAY['debug']::text[], 168, 1440),
    ('Drop warnings after 30 days', TRUE, TRUE, ARRAY['warn']::text[], 720, 1440),
    ('Drop errors after 90 days', TRUE, TRUE, ARRAY['error']::text[], 2160, 1440);
