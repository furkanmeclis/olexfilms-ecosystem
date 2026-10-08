-- TEC-490 (F5-05a): performance and targets. Authorization scope is
-- resolved by the usecase and arrives as organization id lists (NULL = the
-- whole brand). Metric keys: internal/modules/performance/model.

-- Monthly metrics ---------------------------------------------------------------

-- name: UpsertPerformanceMetric :one
-- Idempotent write of the metric worker: one row per org x month x metric.
INSERT INTO performance_metrics_monthly (
    organization_id, brand_id, period, metric, value, numerator, denominator, currency, computed_at
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(period), sqlc.arg(metric),
    sqlc.arg(value), sqlc.narg(numerator), sqlc.narg(denominator), sqlc.narg(currency), NOW()
)
ON CONFLICT (organization_id, period, metric) DO UPDATE
SET value = EXCLUDED.value,
    numerator = EXCLUDED.numerator,
    denominator = EXCLUDED.denominator,
    currency = EXCLUDED.currency,
    computed_at = EXCLUDED.computed_at
RETURNING *;

-- name: ListPerformanceMetrics :many
-- Metrics of the organizations over a closed period range (YYYY-MM).
SELECT m.*
FROM performance_metrics_monthly m
WHERE m.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR m.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND m.period >= sqlc.arg(period_from)::text
  AND m.period <= sqlc.arg(period_to)::text
  AND (COALESCE(cardinality(sqlc.narg(metrics)::text[]), 0) = 0 OR m.metric = ANY (sqlc.narg(metrics)::text[]))
ORDER BY m.organization_id, m.period, m.metric;

-- name: ListPerformanceRanking :many
-- Ranking list of distributors and dealers for one month, one column per
-- metric (NULL = not computed). Sort: docs/list-contract.md, keys from
-- performance/repository.RankingSort (metric keys | name); metric columns
-- sort NULLS LAST in both directions; id tiebreak.
WITH m AS (
    SELECT pm.organization_id,
           MAX(pm.value) FILTER (WHERE pm.metric = 'services_count') AS services_count,
           MAX(pm.value) FILTER (WHERE pm.metric = 'warranty_start_rate') AS warranty_start_rate,
           MAX(pm.value) FILTER (WHERE pm.metric = 'measurement_rate') AS measurement_rate,
           MAX(pm.value) FILTER (WHERE pm.metric = 'review_avg') AS review_avg,
           MAX(pm.value) FILTER (WHERE pm.metric = 'stock_turnover') AS stock_turnover,
           MAX(pm.value) FILTER (WHERE pm.metric = 'contract_days_left') AS contract_days_left,
           MAX(pm.value) FILTER (WHERE pm.metric = 'cari_overdue_amount') AS cari_overdue_amount,
           MAX(pm.value) FILTER (WHERE pm.metric = 'cari_overdue_days') AS cari_overdue_days,
           MAX(pm.value) FILTER (WHERE pm.metric = 'certificate_coverage') AS certificate_coverage,
           MAX(pm.value) FILTER (WHERE pm.metric = 'lead_conversion_rate') AS lead_conversion_rate,
           MAX(pm.value) FILTER (WHERE pm.metric = 'waste_ratio') AS waste_ratio,
           MAX(pm.value) FILTER (WHERE pm.metric = 'order_volume') AS order_volume,
           MAX(pm.computed_at)::timestamptz AS computed_at
    FROM performance_metrics_monthly pm
    WHERE pm.brand_id = sqlc.arg(brand_id)
      AND pm.period = sqlc.arg(period)::text
    GROUP BY pm.organization_id
)
SELECT o.id AS organization_id,
       o.uuid AS organization_uuid,
       o.name,
       o.type,
       o.parent_id,
       o.province_id,
       o.currency,
       m.services_count::numeric AS services_count,
       m.warranty_start_rate::numeric AS warranty_start_rate,
       m.measurement_rate::numeric AS measurement_rate,
       m.review_avg::numeric AS review_avg,
       m.stock_turnover::numeric AS stock_turnover,
       m.contract_days_left::numeric AS contract_days_left,
       m.cari_overdue_amount::numeric AS cari_overdue_amount,
       m.cari_overdue_days::numeric AS cari_overdue_days,
       m.certificate_coverage::numeric AS certificate_coverage,
       m.lead_conversion_rate::numeric AS lead_conversion_rate,
       m.waste_ratio::numeric AS waste_ratio,
       m.order_volume::numeric AS order_volume,
       m.computed_at AS computed_at,
       COUNT(*) OVER()::bigint AS total_count
FROM organizations o
LEFT JOIN m ON m.organization_id = o.id
WHERE o.brand_id = sqlc.arg(brand_id)
  AND o.deleted_at IS NULL
  AND o.type IN ('distributor', 'dealer')
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR o.id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(q)::text IS NULL OR o.name ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (COALESCE(cardinality(sqlc.narg(org_types)::text[]), 0) = 0 OR o.type = ANY (sqlc.narg(org_types)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(distributor_ids)::bigint[]), 0) = 0
       OR o.id = ANY (sqlc.narg(distributor_ids)::bigint[])
       OR o.parent_id = ANY (sqlc.narg(distributor_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(province_ids)::bigint[]), 0) = 0
       OR o.province_id = ANY (sqlc.narg(province_ids)::bigint[]))
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN o.name END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN o.name END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'services_count' THEN m.services_count
      WHEN 'warranty_start_rate' THEN m.warranty_start_rate
      WHEN 'measurement_rate' THEN m.measurement_rate
      WHEN 'review_avg' THEN m.review_avg
      WHEN 'stock_turnover' THEN m.stock_turnover
      WHEN 'contract_days_left' THEN m.contract_days_left
      WHEN 'cari_overdue_amount' THEN m.cari_overdue_amount
      WHEN 'cari_overdue_days' THEN m.cari_overdue_days
      WHEN 'certificate_coverage' THEN m.certificate_coverage
      WHEN 'lead_conversion_rate' THEN m.lead_conversion_rate
      WHEN 'waste_ratio' THEN m.waste_ratio
      WHEN 'order_volume' THEN m.order_volume
    END
  END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'services_count' THEN m.services_count
      WHEN 'warranty_start_rate' THEN m.warranty_start_rate
      WHEN 'measurement_rate' THEN m.measurement_rate
      WHEN 'review_avg' THEN m.review_avg
      WHEN 'stock_turnover' THEN m.stock_turnover
      WHEN 'contract_days_left' THEN m.contract_days_left
      WHEN 'cari_overdue_amount' THEN m.cari_overdue_amount
      WHEN 'cari_overdue_days' THEN m.cari_overdue_days
      WHEN 'certificate_coverage' THEN m.certificate_coverage
      WHEN 'lead_conversion_rate' THEN m.lead_conversion_rate
      WHEN 'waste_ratio' THEN m.waste_ratio
      WHEN 'order_volume' THEN m.order_volume
    END
  END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN o.id END DESC,
  o.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- Targets ---------------------------------------------------------------------------

-- name: CreatePerformanceTarget :one
INSERT INTO performance_targets (
    organization_id, brand_id, target_org_id, metric, period_kind, period_start,
    value, currency, contract_ref, note, created_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(target_org_id), sqlc.arg(metric),
    sqlc.arg(period_kind), sqlc.arg(period_start), sqlc.arg(value), sqlc.narg(currency),
    sqlc.narg(contract_ref), sqlc.narg(note), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetPerformanceTarget :one
SELECT * FROM performance_targets
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: UpdatePerformanceTarget :one
UPDATE performance_targets
SET value = sqlc.arg(value),
    currency = sqlc.narg(currency),
    contract_ref = sqlc.narg(contract_ref),
    note = sqlc.narg(note)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: DeletePerformanceTarget :execrows
DELETE FROM performance_targets
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: ListPerformanceTargets :many
-- Targets with their achievement: the target metric summed over the months
-- of the target period (same currency for order volume). Sort keys from
-- performance/repository.TargetSort; id tiebreak.
SELECT t.*,
       o.name AS target_name,
       o.type AS target_type,
       a.actual::numeric AS actual,
       CASE WHEN a.actual IS NULL THEN NULL
            ELSE ROUND(a.actual / t.value * 100, 2)
       END::numeric AS achievement_pct,
       COUNT(*) OVER()::bigint AS total_count
FROM performance_targets t
JOIN organizations o ON o.id = t.target_org_id
LEFT JOIN LATERAL (
    SELECT SUM(pm.value) AS actual
    FROM performance_metrics_monthly pm
    WHERE pm.organization_id = t.target_org_id
      AND pm.metric = t.metric
      AND pm.currency IS NOT DISTINCT FROM t.currency
      AND pm.period >= to_char(t.period_start, 'YYYY-MM')
      AND pm.period < to_char(t.period_end, 'YYYY-MM')
) a ON true
WHERE t.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(owner_org_ids)::bigint[] IS NULL OR t.organization_id = ANY (sqlc.narg(owner_org_ids)::bigint[]))
  AND (sqlc.narg(target_org_ids)::bigint[] IS NULL OR t.target_org_id = ANY (sqlc.narg(target_org_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(metrics)::text[]), 0) = 0 OR t.metric = ANY (sqlc.narg(metrics)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(period_kinds)::text[]), 0) = 0 OR t.period_kind = ANY (sqlc.narg(period_kinds)::text[]))
  AND (sqlc.narg(period_from)::date IS NULL OR t.period_end > sqlc.narg(period_from)::date)
  AND (sqlc.narg(period_before)::date IS NULL OR t.period_start < sqlc.narg(period_before)::date)
  AND (sqlc.narg(q)::text IS NULL OR o.name ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'target_name' THEN o.name
      WHEN 'metric' THEN t.metric
      WHEN 'period_kind' THEN t.period_kind
    END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'target_name' THEN o.name
      WHEN 'metric' THEN t.metric
      WHEN 'period_kind' THEN t.period_kind
    END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'period_start' THEN t.period_start END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'period_start' THEN t.period_start END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN t.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN t.created_at END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'value' THEN t.value
      WHEN 'achievement_pct' THEN a.actual / t.value
    END
  END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'value' THEN t.value
      WHEN 'achievement_pct' THEN a.actual / t.value
    END
  END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN t.id END DESC,
  t.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- Staff targets ---------------------------------------------------------------------

-- name: UpsertStaffTarget :one
INSERT INTO staff_targets (
    organization_id, brand_id, user_id, period, metric, value, currency, created_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(user_id), sqlc.arg(period),
    sqlc.arg(metric), sqlc.arg(value), sqlc.narg(currency), sqlc.narg(created_by_user_id)
)
ON CONFLICT (organization_id, user_id, period, metric) DO UPDATE
SET value = EXCLUDED.value,
    currency = EXCLUDED.currency
RETURNING *;

-- name: GetStaffTarget :one
SELECT * FROM staff_targets
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: DeleteStaffTarget :execrows
DELETE FROM staff_targets
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- name: ListStaffTargets :many
SELECT st.*,
       u.name AS user_name,
       u.surname AS user_surname
FROM staff_targets st
JOIN users u ON u.id = st.user_id
WHERE st.organization_id = sqlc.arg(organization_id)
  AND st.period >= sqlc.arg(period_from)::text
  AND st.period <= sqlc.arg(period_to)::text
  AND (sqlc.narg(user_id)::bigint IS NULL OR st.user_id = sqlc.narg(user_id)::bigint)
ORDER BY st.period DESC, u.name, u.surname, st.metric, st.id;

-- Bonus rules, settings and accruals ------------------------------------------------

-- name: CreateBonusRule :one
INSERT INTO bonus_rules (
    organization_id, brand_id, name, metric, threshold_pct, kind, amount, percent, currency,
    active, created_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(name), sqlc.arg(metric),
    sqlc.arg(threshold_pct), sqlc.arg(kind), sqlc.narg(amount), sqlc.narg(percent),
    sqlc.narg(currency), sqlc.arg(active), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetBonusRule :one
SELECT * FROM bonus_rules
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: UpdateBonusRule :one
UPDATE bonus_rules
SET name = sqlc.arg(name),
    metric = sqlc.arg(metric),
    threshold_pct = sqlc.arg(threshold_pct),
    kind = sqlc.arg(kind),
    amount = sqlc.narg(amount),
    percent = sqlc.narg(percent),
    currency = sqlc.narg(currency),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
RETURNING *;

-- name: DeleteBonusRule :execrows
-- A rule with accruals is kept (FK RESTRICT); the usecase deactivates it.
DELETE FROM bonus_rules br
WHERE br.id = sqlc.arg(id) AND br.organization_id = sqlc.arg(organization_id)
  AND NOT EXISTS (SELECT 1 FROM bonus_accruals a WHERE a.rule_id = br.id);

-- name: ListBonusRules :many
SELECT * FROM bonus_rules
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::bool IS NULL OR active = sqlc.narg(active)::bool)
ORDER BY active DESC, threshold_pct, id;

-- name: GetBonusSettings :one
SELECT * FROM bonus_settings
WHERE organization_id = sqlc.arg(organization_id);

-- name: UpsertBonusSettings :one
INSERT INTO bonus_settings (organization_id, brand_id, payout_day)
VALUES (sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(payout_day))
ON CONFLICT (organization_id) DO UPDATE
SET payout_day = EXCLUDED.payout_day
RETURNING *;

-- name: UpsertBonusAccrual :one
-- Month-end calculation. A recalculation refreshes a still 'calculated'
-- accrual; approved/posted ones are left untouched (no row returned).
INSERT INTO bonus_accruals (
    organization_id, brand_id, user_id, period, rule_id, achievement_pct, amount, currency
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(user_id), sqlc.arg(period),
    sqlc.arg(rule_id), sqlc.arg(achievement_pct), sqlc.arg(amount), sqlc.arg(currency)
)
ON CONFLICT (user_id, rule_id, period) WHERE status <> 'cancelled' DO UPDATE
SET achievement_pct = EXCLUDED.achievement_pct,
    amount = EXCLUDED.amount,
    currency = EXCLUDED.currency
WHERE bonus_accruals.status = 'calculated'
RETURNING *;

-- name: LockBonusAccrual :one
SELECT * FROM bonus_accruals
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id)
FOR UPDATE;

-- name: ApproveBonusAccrual :one
UPDATE bonus_accruals
SET status = 'approved',
    approved_by_user_id = sqlc.narg(approved_by_user_id),
    approved_at = NOW()
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND status = 'calculated'
RETURNING *;

-- name: MarkBonusAccrualPosted :one
UPDATE bonus_accruals
SET status = 'posted',
    staff_payment_id = sqlc.arg(staff_payment_id)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND status = 'approved'
RETURNING *;

-- name: CancelBonusAccrual :one
UPDATE bonus_accruals
SET status = 'cancelled',
    cancelled_at = NOW()
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
  AND status IN ('calculated', 'approved')
RETURNING *;

-- name: ListBonusAccruals :many
SELECT a.*,
       u.name AS user_name,
       u.surname AS user_surname,
       r.name AS rule_name,
       COUNT(*) OVER()::bigint AS total_count
FROM bonus_accruals a
JOIN users u ON u.id = a.user_id
JOIN bonus_rules r ON r.id = a.rule_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(period)::text IS NULL OR a.period = sqlc.narg(period)::text)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR a.status = ANY (sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(user_id)::bigint IS NULL OR a.user_id = sqlc.narg(user_id)::bigint)
ORDER BY a.period DESC, u.name, u.surname, a.id
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- Weak dealer rules -------------------------------------------------------------

-- name: CreateWeakDealerRule :one
INSERT INTO weak_dealer_rules (
    organization_id, brand_id, name, metric, operator, threshold, create_task, notify,
    assignee_user_id, active, created_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(name), sqlc.arg(metric),
    sqlc.arg(operator), sqlc.arg(threshold), sqlc.arg(create_task), sqlc.arg(notify),
    sqlc.narg(assignee_user_id), sqlc.arg(active), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetWeakDealerRule :one
SELECT * FROM weak_dealer_rules
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: UpdateWeakDealerRule :one
UPDATE weak_dealer_rules
SET name = sqlc.arg(name),
    metric = sqlc.arg(metric),
    operator = sqlc.arg(operator),
    threshold = sqlc.arg(threshold),
    create_task = sqlc.arg(create_task),
    notify = sqlc.arg(notify),
    assignee_user_id = sqlc.narg(assignee_user_id),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: DeleteWeakDealerRule :execrows
-- A rule that opened tasks is kept (FK RESTRICT); the usecase deactivates it.
DELETE FROM weak_dealer_rules wr
WHERE wr.id = sqlc.arg(id) AND wr.brand_id = sqlc.arg(brand_id)
  AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.auto_rule_id = wr.id);

-- name: ListWeakDealerRules :many
SELECT r.*,
       o.name AS owner_name,
       o.type AS owner_type
FROM weak_dealer_rules r
JOIN organizations o ON o.id = r.organization_id
WHERE r.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(owner_org_ids)::bigint[] IS NULL OR r.organization_id = ANY (sqlc.narg(owner_org_ids)::bigint[]))
  AND (sqlc.narg(active)::bool IS NULL OR r.active = sqlc.narg(active)::bool)
ORDER BY o.type, r.organization_id, r.name, r.id;

-- Automatic tasks ---------------------------------------------------------------

-- name: InsertAutoPerformanceTask :one
-- One task per subject x rule x month (uq_tasks_auto); a second insert for
-- the same month fails with unique_violation.
INSERT INTO tasks (
    organization_id, brand_id, subject_org_id, title, description,
    assignee_user_id, priority, due_at, source, auto_rule_id, auto_period
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(subject_org_id), sqlc.arg(title),
    sqlc.arg(description), sqlc.narg(assignee_user_id), sqlc.arg(priority), sqlc.narg(due_at),
    'auto', sqlc.arg(auto_rule_id), sqlc.arg(auto_period)
)
RETURNING *;
