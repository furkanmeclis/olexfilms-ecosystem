-- TEC-252: migrator bookkeeping (000074). Written only by cmd/migrator.

-- name: GetMigrationMap :one
SELECT * FROM migration_map
WHERE source_system = $1 AND source_table = $2 AND source_id = $3;

-- name: InsertMigrationMap :one
-- ON CONFLICT DO NOTHING: a concurrent insert of the same key returns no row
-- and the caller reads the existing one.
INSERT INTO migration_map (source_system, source_table, source_id, target_table, target_uuid, checksum)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (source_system, source_table, source_id) DO NOTHING
RETURNING *;

-- name: UpdateMigrationMapChecksum :exec
UPDATE migration_map
SET checksum = $2, migrated_at = NOW()
WHERE id = $1;

-- name: CreateMigrationRun :one
INSERT INTO migration_runs (parent_id, profile, mode, step, dry_run)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: FinishMigrationRun :one
UPDATE migration_runs
SET finished_at = NOW(), status = $2, counts = $3, watermark = $4, error = $5
WHERE id = $1
RETURNING *;

-- name: LastMigrationWatermark :one
-- Watermark of the last successful, non-dry-run execution of a step.
SELECT watermark FROM migration_runs
WHERE profile = $1 AND step = $2 AND status = 'succeeded' AND dry_run = FALSE
  AND watermark IS NOT NULL
ORDER BY started_at DESC, id DESC
LIMIT 1;

-- name: ListMigrationRuns :many
SELECT * FROM migration_runs
ORDER BY started_at DESC, id DESC
LIMIT $1;

-- name: CountMigrationMap :many
SELECT source_system, source_table, target_table, COUNT(*)::bigint AS rows, MAX(migrated_at)::timestamptz AS last_migrated_at
FROM migration_map
GROUP BY source_system, source_table, target_table
ORDER BY source_system, source_table, target_table;
