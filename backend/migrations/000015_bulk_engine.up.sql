-- Bulk action jobs (sync/async batch mutations)
CREATE TABLE bulk_jobs (
    id             BIGSERIAL PRIMARY KEY,
    uuid           UUID         NOT NULL DEFAULT gen_random_uuid(),
    resource       VARCHAR(128) NOT NULL,
    action         VARCHAR(64)  NOT NULL,
    actor_id       BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    locale         VARCHAR(8)   NOT NULL DEFAULT 'tr',
    status         VARCHAR(32)  NOT NULL DEFAULT 'queued',
    target_json    JSONB        NOT NULL DEFAULT '{}'::jsonb,
    result_json    JSONB        NOT NULL DEFAULT '{}'::jsonb,
    error          TEXT         NULL,
    rollback_until TIMESTAMPTZ  NULL,
    applied_at     TIMESTAMPTZ  NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT bulk_jobs_uuid_uq UNIQUE (uuid),
    CONSTRAINT bulk_jobs_status_chk CHECK (status IN (
        'queued', 'processing', 'completed', 'failed',
        'rolled_back', 'rolled_back_partial'
    ))
);

CREATE INDEX bulk_jobs_actor_idx ON bulk_jobs (actor_id, created_at DESC);
CREATE INDEX bulk_jobs_status_idx ON bulk_jobs (status, created_at DESC);

CREATE TRIGGER trg_bulk_jobs_set_updated_at
    BEFORE UPDATE ON bulk_jobs
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Per-entity change log for rollback
CREATE TABLE bulk_changes (
    id            BIGSERIAL PRIMARY KEY,
    uuid          UUID         NOT NULL DEFAULT gen_random_uuid(),
    job_id        BIGINT       NOT NULL REFERENCES bulk_jobs (id) ON DELETE CASCADE,
    entity_type   VARCHAR(64)  NOT NULL,
    entity_uuid   UUID         NOT NULL,
    op            VARCHAR(16)  NOT NULL,
    previous_json JSONB        NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT bulk_changes_uuid_uq UNIQUE (uuid),
    CONSTRAINT bulk_changes_op_chk CHECK (op IN ('update', 'delete'))
);

CREATE INDEX bulk_changes_job_idx ON bulk_changes (job_id);

-- Notification templates
INSERT INTO notification_templates (code, channel, language, subject, body, active) VALUES
    ('bulk.completed', 'inapp', 'en', 'Bulk action completed',
     'Bulk {{action}} on {{resource}} finished: {{succeeded}} succeeded, {{failed}} failed.', TRUE),
    ('bulk.completed', 'realtime', 'en', 'Bulk action completed',
     'Bulk action completed.', TRUE),
    ('bulk.failed', 'inapp', 'en', 'Bulk action failed',
     'Bulk {{action}} on {{resource}} failed: {{error}}.', TRUE),
    ('bulk.completed', 'inapp', 'tr', 'Toplu işlem tamamlandı',
     '{{resource}} için {{action}} toplu işlemi bitti: {{succeeded}} başarılı, {{failed}} hatalı.', TRUE),
    ('bulk.completed', 'realtime', 'tr', 'Toplu işlem tamamlandı',
     'Toplu işlem tamamlandı.', TRUE),
    ('bulk.failed', 'inapp', 'tr', 'Toplu işlem başarısız',
     '{{resource}} için {{action}} toplu işlemi başarısız: {{error}}.', TRUE);
