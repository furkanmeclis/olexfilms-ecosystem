-- TEC-252 (F2-01a): migrator bookkeeping (design §7, K26/K27).
--
-- migration_map pairs every legacy row (source system + table + id) with the
-- row it became in this database, so the migrator is idempotent and can be
-- rerun during the transition period (K27). checksum is a hash of the source
-- row; a changed checksum on a rerun means the target must be refreshed.
--
-- migration_runs is the run log: one row per run (step IS NULL) plus one row
-- per executed step (parent_id = the run row). watermark is the highest
-- source timestamp a step has seen; delta runs start from the last
-- successful watermark of the same profile + step.
--
-- Neither is a business table: they hold no tenant data of their own, are
-- written only by the migrator container and are dropped after the final
-- cutover (K27). So no organization_id / brand_id (AGENTS §3 applies to
-- business tables); the target row itself carries its scope.

CREATE TABLE migration_map (
    id             BIGSERIAL     PRIMARY KEY,
    source_system  VARCHAR(32)   NOT NULL,
    source_table   VARCHAR(64)   NOT NULL,
    source_id      VARCHAR(64)   NOT NULL,
    target_table   VARCHAR(64)   NOT NULL,
    target_uuid    UUID          NOT NULL,
    checksum       VARCHAR(64)   NOT NULL DEFAULT '',
    migrated_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_migration_map_source UNIQUE (source_system, source_table, source_id)
);

CREATE INDEX idx_migration_map_target ON migration_map (target_table, target_uuid);

CREATE TABLE migration_runs (
    id           BIGSERIAL     PRIMARY KEY,
    -- The run row of a step row; NULL on the run row itself.
    parent_id    BIGINT        NULL REFERENCES migration_runs (id) ON DELETE CASCADE,
    profile      VARCHAR(32)   NOT NULL,
    mode         VARCHAR(16)   NOT NULL,
    step         VARCHAR(64)   NULL,
    dry_run      BOOLEAN       NOT NULL DEFAULT FALSE,
    started_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    finished_at  TIMESTAMPTZ   NULL,
    watermark    TIMESTAMPTZ   NULL,
    counts       JSONB         NOT NULL DEFAULT '{}'::jsonb,
    status       VARCHAR(16)   NOT NULL DEFAULT 'running',
    error        TEXT          NULL,
    CONSTRAINT chk_migration_runs_mode CHECK (mode IN ('full', 'delta')),
    CONSTRAINT chk_migration_runs_status CHECK (status IN ('running', 'succeeded', 'failed')),
    CONSTRAINT chk_migration_runs_step_parent CHECK ((step IS NULL) = (parent_id IS NULL))
);

CREATE INDEX idx_migration_runs_step ON migration_runs (profile, step, started_at DESC);
CREATE INDEX idx_migration_runs_parent ON migration_runs (parent_id);
