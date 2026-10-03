-- TEC-212 (F1-10e): bulk operation log with undo.
--
-- One row per executed bulk action (sync or async), written in the same
-- transaction as the item updates. changes holds the per-record snapshot
-- [{entity_type, entity_uuid, op, previous, applied}]: previous is restored
-- by undo, applied is compared with the current row so a record changed
-- after the bulk action is skipped (conservative: never overwrite).
--
-- organization_id / brand_id scope tenant operations (catalog products,
-- tasks, ...); platform operations (platform.users, platform.roles) have no
-- organization and keep both NULL. Undo is allowed only from the same
-- organization, by a user holding the action's permission, once, and
-- within undo_until (system setting bulk_undo_window_hours, default 24).
CREATE TABLE bulk_operations (
    id                BIGSERIAL    PRIMARY KEY,
    uuid              UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id   BIGINT       NULL REFERENCES organizations (id) ON DELETE CASCADE,
    brand_id          BIGINT       NULL REFERENCES brands (id) ON DELETE CASCADE,
    job_id            BIGINT       NULL REFERENCES bulk_jobs (id) ON DELETE SET NULL,
    resource          VARCHAR(128) NOT NULL,
    action            VARCHAR(64)  NOT NULL,
    permission        VARCHAR(128) NOT NULL,
    actor_user_id     BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    target_json       JSONB        NOT NULL DEFAULT '{}'::jsonb,
    changes           JSONB        NOT NULL DEFAULT '[]'::jsonb,
    total             INTEGER      NOT NULL DEFAULT 0,
    succeeded         INTEGER      NOT NULL DEFAULT 0,
    failed            INTEGER      NOT NULL DEFAULT 0,
    undo_status       VARCHAR(16)  NOT NULL DEFAULT 'none',
    undo_until        TIMESTAMPTZ  NULL,
    undone_at         TIMESTAMPTZ  NULL,
    undone_by_user_id BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    undo_result       JSONB        NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_bulk_operations_uuid UNIQUE (uuid),
    CONSTRAINT chk_bulk_operations_scope CHECK ((organization_id IS NULL) = (brand_id IS NULL)),
    CONSTRAINT chk_bulk_operations_undo_status CHECK (undo_status IN ('none', 'available', 'undone', 'partial')),
    CONSTRAINT chk_bulk_operations_undone CHECK ((undo_status IN ('undone', 'partial')) = (undone_at IS NOT NULL))
);

CREATE INDEX idx_bulk_operations_org_created ON bulk_operations (organization_id, created_at DESC);
CREATE INDEX idx_bulk_operations_actor_created ON bulk_operations (actor_user_id, created_at DESC);
CREATE INDEX idx_bulk_operations_job ON bulk_operations (job_id) WHERE job_id IS NOT NULL;

CREATE TRIGGER trg_bulk_operations_set_updated_at
    BEFORE UPDATE ON bulk_operations
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
