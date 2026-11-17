-- TEC-1221: unit usage credits (ledger unit_credit_movements, projection
-- unit_credit_balances). Writes go through modules/stock/credits only.

-- name: GetUnitCreditDefault :one
-- -1: the unit carries no credits (fixed barcode or roll).
SELECT COALESCE(unit_credit_default(sqlc.arg(unit_id)::bigint), -1)::int AS credits;

-- name: GetUnitCreditBalance :one
SELECT * FROM unit_credit_balances WHERE unit_id = sqlc.arg(unit_id);

-- name: LockUnitCreditBalance :one
SELECT * FROM unit_credit_balances WHERE unit_id = sqlc.arg(unit_id) FOR UPDATE;

-- name: InsertUnitCreditMovement :one
INSERT INTO unit_credit_movements (
    organization_id, brand_id, unit_id, type, total_delta, remaining_delta, uses_delta,
    service_id, service_item_id, actor_user_id, reason, metadata, idempotency_key
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(unit_id), sqlc.arg(type),
    sqlc.arg(total_delta), sqlc.arg(remaining_delta), sqlc.arg(uses_delta),
    sqlc.narg(service_id), sqlc.narg(service_item_id), sqlc.narg(actor_user_id), sqlc.narg(reason),
    sqlc.arg(metadata), sqlc.arg(idempotency_key)
)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING *;

-- name: InsertUnitCreditBalance :one
INSERT INTO unit_credit_balances (
    unit_id, organization_id, brand_id, credits_total, credits_remaining, used_count, last_movement_id
) VALUES (
    sqlc.arg(unit_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(credits_total),
    sqlc.arg(credits_remaining), sqlc.arg(used_count), sqlc.arg(last_movement_id)
)
RETURNING *;

-- name: ApplyUnitCreditMovement :one
UPDATE unit_credit_balances
SET credits_total = credits_total + sqlc.arg(total_delta)::int,
    credits_remaining = credits_remaining + sqlc.arg(remaining_delta)::int,
    used_count = used_count + sqlc.arg(uses_delta)::int,
    last_movement_id = sqlc.arg(movement_id)
WHERE unit_id = sqlc.arg(unit_id)
RETURNING *;

-- name: SumUnitCreditsByItem :one
-- Net credits a service item holds: remaining_delta is negative for what
-- it took, uses_delta the uses it counts.
SELECT COALESCE(SUM(remaining_delta), 0)::int AS remaining_delta,
       COALESCE(SUM(uses_delta), 0)::int AS uses_delta,
       COUNT(*)::int AS movements
FROM unit_credit_movements
WHERE service_item_id = sqlc.arg(service_item_id);

-- name: ListUnitCreditMovements :many
SELECT m.id, m.uuid, m.type, m.total_delta, m.remaining_delta, m.uses_delta, m.reason, m.created_at,
       s.uuid AS service_uuid, s.service_no AS service_no,
       usr.uuid AS actor_uuid, usr.name AS actor_name, usr.surname AS actor_surname
FROM unit_credit_movements m
LEFT JOIN services s ON s.id = m.service_id
LEFT JOIN users usr ON usr.id = m.actor_user_id
WHERE m.unit_id = sqlc.arg(unit_id)
ORDER BY m.created_at DESC, m.id DESC
LIMIT 200;

-- name: ListUnitCreditServices :many
-- Services the unit was selected in (credit uses), newest first.
SELECT s.id, s.uuid, s.service_no, s.status, s.organization_id, s.created_at, s.completed_at,
       i.uuid AS item_uuid, i.finished_product,
       COALESCE((SELECT -SUM(m.remaining_delta) FROM unit_credit_movements m
                 WHERE m.service_item_id = i.id), 0)::int AS credits_taken
FROM service_items i
JOIN services s ON s.id = i.service_id
WHERE i.unit_id = sqlc.arg(unit_id)
ORDER BY s.created_at DESC, s.id DESC
LIMIT 200;

-- name: UnitCreditsBackfill :one
SELECT unit_credits_backfill()::int AS opened;

-- name: ListUnitCreditDrift :many
-- TEC-1229: credit balances that differ from the sum of their movements
-- (rebuild check). organization_id 0 checks every unit; otherwise the units
-- with a movement or a balance of the organization. has_expected: the unit
-- has movements; has_actual: it has a balance row.
WITH scope AS (
    SELECT unit_id FROM unit_credit_movements
    WHERE sqlc.arg(organization_id)::bigint = 0 OR organization_id = sqlc.arg(organization_id)::bigint
    UNION
    SELECT unit_id FROM unit_credit_balances
    WHERE sqlc.arg(organization_id)::bigint = 0 OR organization_id = sqlc.arg(organization_id)::bigint
), exp AS (
    SELECT m.unit_id,
           SUM(m.total_delta)::int AS total, SUM(m.remaining_delta)::int AS remaining,
           SUM(m.uses_delta)::int AS used, MAX(m.id)::bigint AS last_movement_id,
           (ARRAY_AGG(m.organization_id ORDER BY m.id))[1]::bigint AS organization_id,
           MAX(m.brand_id)::bigint AS brand_id
    FROM unit_credit_movements m
    WHERE m.unit_id IN (SELECT unit_id FROM scope)
    GROUP BY m.unit_id
)
SELECT s.unit_id::bigint AS unit_id, u.barcode, u.brand_id AS unit_brand_id,
       (e.unit_id IS NOT NULL)::bool AS has_expected,
       (b.unit_id IS NOT NULL)::bool AS has_actual,
       COALESCE(e.total, 0)::int AS expected_total,
       COALESCE(e.remaining, 0)::int AS expected_remaining,
       COALESCE(e.used, 0)::int AS expected_used,
       COALESCE(e.last_movement_id, 0)::bigint AS expected_last_movement_id,
       COALESCE(e.organization_id, b.organization_id, u.organization_id)::bigint AS organization_id,
       COALESCE(b.credits_total, 0)::int AS actual_total,
       COALESCE(b.credits_remaining, 0)::int AS actual_remaining,
       COALESCE(b.used_count, 0)::int AS actual_used,
       COALESCE(b.last_movement_id, 0)::bigint AS actual_last_movement_id
FROM scope s
JOIN units u ON u.id = s.unit_id
LEFT JOIN exp e ON e.unit_id = s.unit_id
LEFT JOIN unit_credit_balances b ON b.unit_id = s.unit_id
WHERE e.unit_id IS NULL OR b.unit_id IS NULL
   OR e.total <> b.credits_total OR e.remaining <> b.credits_remaining
   OR e.used <> b.used_count OR e.last_movement_id IS DISTINCT FROM b.last_movement_id
ORDER BY s.unit_id;

-- name: UpsertUnitCreditBalanceForRepair :exec
-- TEC-1229: rebuild repair sets the balance to the sum of the movements.
INSERT INTO unit_credit_balances (
    unit_id, organization_id, brand_id, credits_total, credits_remaining, used_count, last_movement_id
) VALUES (
    sqlc.arg(unit_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(credits_total),
    sqlc.arg(credits_remaining), sqlc.arg(used_count), sqlc.arg(last_movement_id)
)
ON CONFLICT (unit_id) DO UPDATE
SET credits_total = EXCLUDED.credits_total,
    credits_remaining = EXCLUDED.credits_remaining,
    used_count = EXCLUDED.used_count,
    last_movement_id = EXCLUDED.last_movement_id;
