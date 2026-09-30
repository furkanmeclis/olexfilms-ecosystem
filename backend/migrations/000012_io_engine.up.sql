-- Singleton application settings (letterhead / branding)
CREATE TABLE app_settings (
    id               SMALLINT     PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    logo_object_key  TEXT         NULL,
    primary_color    VARCHAR(16)  NOT NULL DEFAULT '#0F172A',
    company_name     VARCHAR(255) NOT NULL DEFAULT 'App',
    tagline          VARCHAR(512) NOT NULL DEFAULT 'Platform & CMS console',
    address          TEXT         NOT NULL DEFAULT '',
    phone            VARCHAR(64)  NOT NULL DEFAULT '',
    email            VARCHAR(255) NOT NULL DEFAULT '',
    website          VARCHAR(512) NOT NULL DEFAULT '',
    footer_text      TEXT         NOT NULL DEFAULT '',
    paper_size       VARCHAR(8)   NOT NULL DEFAULT 'A4',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

INSERT INTO app_settings (id) VALUES (1);

CREATE TRIGGER trg_app_settings_set_updated_at
    BEFORE UPDATE ON app_settings
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Export jobs (async file generation)
CREATE TABLE export_jobs (
    id          BIGSERIAL PRIMARY KEY,
    uuid        UUID         NOT NULL DEFAULT gen_random_uuid(),
    resource    VARCHAR(128) NOT NULL,
    actor_id    BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    format      VARCHAR(16)  NOT NULL,
    query_json  JSONB        NOT NULL DEFAULT '{}'::jsonb,
    locale      VARCHAR(8)   NOT NULL DEFAULT 'tr',
    status      VARCHAR(32)  NOT NULL DEFAULT 'queued',
    file_key    TEXT         NULL,
    row_count   INTEGER      NOT NULL DEFAULT 0,
    error       TEXT         NULL,
    expires_at  TIMESTAMPTZ  NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT export_jobs_uuid_uq UNIQUE (uuid),
    CONSTRAINT export_jobs_format_chk CHECK (format IN ('pdf', 'xlsx', 'csv', 'json')),
    CONSTRAINT export_jobs_status_chk CHECK (status IN (
        'queued', 'processing', 'completed', 'failed', 'expired'
    ))
);

CREATE INDEX export_jobs_actor_idx ON export_jobs (actor_id, created_at DESC);
CREATE INDEX export_jobs_status_idx ON export_jobs (status, created_at DESC);

CREATE TRIGGER trg_export_jobs_set_updated_at
    BEFORE UPDATE ON export_jobs
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Import jobs (wizard pipeline)
CREATE TABLE import_jobs (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    resource         VARCHAR(128) NOT NULL,
    actor_id         BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    format           VARCHAR(16)  NOT NULL,
    locale           VARCHAR(8)   NOT NULL DEFAULT 'tr',
    status           VARCHAR(32)  NOT NULL DEFAULT 'uploaded',
    file_key         TEXT         NULL,
    mapping_json     JSONB        NOT NULL DEFAULT '{}'::jsonb,
    defaults_json    JSONB        NOT NULL DEFAULT '{}'::jsonb,
    preview_json     JSONB        NOT NULL DEFAULT '{}'::jsonb,
    error            TEXT         NULL,
    rollback_until   TIMESTAMPTZ  NULL,
    applied_at       TIMESTAMPTZ  NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT import_jobs_uuid_uq UNIQUE (uuid),
    CONSTRAINT import_jobs_format_chk CHECK (format IN ('json', 'xlsx', 'csv', 'tsv')),
    CONSTRAINT import_jobs_status_chk CHECK (status IN (
        'uploaded', 'mapped', 'previewed', 'queued', 'applying',
        'applied', 'failed', 'rolled_back'
    ))
);

CREATE INDEX import_jobs_actor_idx ON import_jobs (actor_id, created_at DESC);
CREATE INDEX import_jobs_status_idx ON import_jobs (status, created_at DESC);

CREATE TRIGGER trg_import_jobs_set_updated_at
    BEFORE UPDATE ON import_jobs
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Import change log for rollback
CREATE TABLE import_changes (
    id            BIGSERIAL PRIMARY KEY,
    uuid          UUID         NOT NULL DEFAULT gen_random_uuid(),
    job_id        BIGINT       NOT NULL REFERENCES import_jobs (id) ON DELETE CASCADE,
    entity_type   VARCHAR(64)  NOT NULL,
    entity_uuid   UUID         NOT NULL,
    op            VARCHAR(16)  NOT NULL,
    previous_json JSONB        NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT import_changes_uuid_uq UNIQUE (uuid),
    CONSTRAINT import_changes_op_chk CHECK (op IN ('create', 'update'))
);

CREATE INDEX import_changes_job_idx ON import_changes (job_id);

-- Activity log (system-wide audit trail)
CREATE TABLE activity_events (
    id            BIGSERIAL PRIMARY KEY,
    uuid          UUID         NOT NULL DEFAULT gen_random_uuid(),
    actor_user_id BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    action        VARCHAR(128) NOT NULL,
    resource      VARCHAR(128) NOT NULL DEFAULT '',
    resource_uuid UUID         NULL,
    payload       JSONB        NOT NULL DEFAULT '{}'::jsonb,
    ip_address    INET         NULL,
    user_agent    TEXT         NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT activity_events_uuid_uq UNIQUE (uuid)
);

CREATE INDEX activity_events_created_idx ON activity_events (created_at DESC);
CREATE INDEX activity_events_actor_idx ON activity_events (actor_user_id, created_at DESC);
CREATE INDEX activity_events_resource_idx ON activity_events (resource, created_at DESC);

INSERT INTO notification_templates (code, channel, language, subject, body, active) VALUES
    ('exports.ready', 'inapp', 'en', 'Export ready',
     'Your {{resource}} export ({{format}}) is ready to download.', TRUE),
    ('exports.ready', 'realtime', 'en', 'Export ready',
     'Your export is ready.', TRUE),
    ('imports.applied', 'inapp', 'en', 'Import completed',
     'Your {{resource}} import finished: {{created}} created, {{updated}} updated, {{failed}} failed.', TRUE),
    ('imports.failed', 'inapp', 'en', 'Import failed',
     'Your {{resource}} import failed: {{error}}.', TRUE),
    ('exports.ready', 'inapp', 'tr', 'Dışa aktarma hazır',
     '{{resource}} dışa aktarmanız ({{format}}) indirmeye hazır.', TRUE),
    ('exports.ready', 'realtime', 'tr', 'Dışa aktarma hazır',
     'Dışa aktarmanız hazır.', TRUE),
    ('imports.applied', 'inapp', 'tr', 'İçe aktarma tamamlandı',
     '{{resource}} içe aktarma bitti: {{created}} oluşturuldu, {{updated}} güncellendi, {{failed}} hatalı.', TRUE),
    ('imports.failed', 'inapp', 'tr', 'İçe aktarma başarısız',
     '{{resource}} içe aktarma başarısız: {{error}}.', TRUE);
