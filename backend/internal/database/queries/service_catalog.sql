-- TEC-305 (F3-08a): non-product service catalog, distributor price
-- overrides, subscriptions, accounted periods and cancellation requests.
-- Every read is bounded by the brand of the active organization.

-- name: CreateServiceCatalogItem :one
INSERT INTO service_catalog_items (
    organization_id, brand_id, name, description, category, default_price, currency,
    recurrence, cancellation_fee, contract_template_id, is_active
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(name), sqlc.arg(description),
    sqlc.arg(category), sqlc.arg(default_price), sqlc.arg(currency), sqlc.arg(recurrence),
    sqlc.arg(cancellation_fee), sqlc.narg(contract_template_id), sqlc.arg(is_active)
)
RETURNING *;

-- name: GetServiceCatalogItemByUUID :one
SELECT * FROM service_catalog_items
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: GetServiceCatalogItemByID :one
SELECT * FROM service_catalog_items
WHERE id = sqlc.arg(id);

-- name: GetServiceCatalogItem :one
SELECT * FROM service_catalog_items
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: ListServiceCatalogItems :many
SELECT * FROM service_catalog_items
-- TEC-369: category / recurrence are CSV multi-value filters; q matches
-- name and description. Full array (small brand list, client-side table).
WHERE brand_id = sqlc.arg(brand_id)
  AND (
    COALESCE(cardinality(sqlc.narg(categories)::text[]), 0) = 0
    OR category = ANY (sqlc.narg(categories)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(recurrences)::text[]), 0) = 0
    OR recurrence = ANY (sqlc.narg(recurrences)::text[])
  )
  AND (sqlc.narg(is_active)::boolean IS NULL OR is_active = sqlc.narg(is_active)::boolean)
  AND (
    sqlc.narg(q)::text IS NULL
    OR name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR description ILIKE '%' || sqlc.narg(q)::text || '%'
  )
ORDER BY name, id;

-- name: UpdateServiceCatalogItem :one
UPDATE service_catalog_items
SET name = sqlc.arg(name),
    description = sqlc.arg(description),
    category = sqlc.arg(category),
    default_price = sqlc.arg(default_price),
    currency = sqlc.arg(currency),
    recurrence = sqlc.arg(recurrence),
    cancellation_fee = sqlc.arg(cancellation_fee),
    contract_template_id = sqlc.narg(contract_template_id),
    is_active = sqlc.arg(is_active)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: AddServiceCatalogModule :exec
INSERT INTO service_catalog_modules (item_id, module_key)
VALUES (sqlc.arg(item_id), sqlc.arg(module_key))
ON CONFLICT (item_id, module_key) DO NOTHING;

-- name: DeleteServiceCatalogModules :execrows
DELETE FROM service_catalog_modules WHERE item_id = sqlc.arg(item_id);

-- name: ListServiceCatalogModules :many
SELECT module_key FROM service_catalog_modules
WHERE item_id = sqlc.arg(item_id)
ORDER BY module_key;

-- name: UpsertServicePriceOverride :one
INSERT INTO service_price_overrides (item_id, organization_id, brand_id, price, currency)
VALUES (sqlc.arg(item_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(price), sqlc.arg(currency))
ON CONFLICT (item_id, organization_id) DO UPDATE
SET price = EXCLUDED.price, currency = EXCLUDED.currency
RETURNING *;

-- name: DeleteServicePriceOverride :execrows
DELETE FROM service_price_overrides
WHERE item_id = sqlc.arg(item_id) AND organization_id = sqlc.arg(organization_id)
  AND brand_id = sqlc.arg(brand_id);

-- name: GetServicePriceOverride :one
SELECT * FROM service_price_overrides
WHERE item_id = sqlc.arg(item_id) AND organization_id = sqlc.arg(organization_id);

-- name: ListServicePriceOverrides :many
SELECT * FROM service_price_overrides
WHERE item_id = sqlc.arg(item_id) AND brand_id = sqlc.arg(brand_id)
ORDER BY organization_id;

-- name: ListServicePriceOverridesForItems :many
SELECT * FROM service_price_overrides
WHERE item_id = ANY(sqlc.arg(item_ids)::bigint[])
  AND organization_id = sqlc.arg(organization_id)
  AND brand_id = sqlc.arg(brand_id)
ORDER BY item_id;

-- name: CountServiceSubscriptionsByItem :one
SELECT count(*) FROM service_subscriptions
WHERE item_id = sqlc.arg(item_id) AND brand_id = sqlc.arg(brand_id);

-- name: CreateServiceSubscription :one
INSERT INTO service_subscriptions (
    organization_id, brand_id, seller_org_id, item_id, assigned_by_org_id, assigned_by_user_id,
    starts_on, ends_on, recurrence, price, currency, rate_snapshot, cancellation_fee, contract_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(seller_org_id), sqlc.arg(item_id),
    sqlc.arg(assigned_by_org_id), sqlc.narg(assigned_by_user_id), sqlc.arg(starts_on), sqlc.arg(ends_on),
    sqlc.arg(recurrence), sqlc.arg(price), sqlc.arg(currency), sqlc.arg(rate_snapshot),
    sqlc.arg(cancellation_fee), sqlc.narg(contract_id)
)
RETURNING *;

-- name: SetServiceSubscriptionContract :one
UPDATE service_subscriptions
SET contract_id = sqlc.narg(contract_id)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: GetServiceSubscriptionByUUID :one
SELECT * FROM service_subscriptions
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: LockServiceSubscription :one
SELECT * FROM service_subscriptions
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
FOR UPDATE;

-- name: CountActiveServiceModuleSubscriptions :one
SELECT count(*)
FROM service_subscriptions s
JOIN service_catalog_modules m ON m.item_id = s.item_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.brand_id = sqlc.arg(brand_id)
  AND m.module_key = sqlc.arg(module_key)
  AND s.status IN ('active', 'cancel_requested');

-- name: SetServiceSubscriptionStatus :one
UPDATE service_subscriptions
SET status = sqlc.arg(status)::text,
    cancelled_at = CASE WHEN sqlc.arg(status)::text = 'cancelled' THEN NOW() ELSE cancelled_at END,
    expired_at = CASE WHEN sqlc.arg(status)::text = 'expired' THEN NOW() ELSE expired_at END
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: ExpireServiceSubscriptions :many
UPDATE service_subscriptions
SET status = 'expired', expired_at = NOW()
WHERE status IN ('active', 'cancel_requested') AND ends_on < sqlc.arg(today)::date
RETURNING *;

-- name: SetServiceSubscriptionCancelRequested :one
UPDATE service_subscriptions
SET status = 'cancel_requested'
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id) AND status = 'active'
RETURNING *;

-- name: InsertServiceSubscriptionPeriod :one
-- Idempotent: a period already written returns no row.
INSERT INTO service_subscription_periods (subscription_id, organization_id, brand_id, period_start, period_end)
VALUES (sqlc.arg(subscription_id), sqlc.arg(organization_id), sqlc.arg(brand_id),
        sqlc.arg(period_start), sqlc.arg(period_end))
ON CONFLICT (subscription_id, period_start) DO NOTHING
RETURNING *;

-- name: MarkServiceSubscriptionPeriodPosted :one
UPDATE service_subscription_periods
SET posted_at = NOW()
WHERE id = sqlc.arg(id) AND posted_at IS NULL
RETURNING *;

-- name: ListServiceSubscriptionPeriods :many
SELECT * FROM service_subscription_periods
WHERE subscription_id = sqlc.arg(subscription_id)
ORDER BY period_start;

-- name: GetPrimaryOrganizationOwnerForServiceContract :one
SELECT u.*
FROM organization_members om
JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
WHERE om.organization_id = sqlc.arg(organization_id)
  AND om.role = 'owner'
ORDER BY om.user_id
LIMIT 1;

-- name: CreateServiceSubscriptionCancelRequest :one
INSERT INTO service_subscription_cancel_requests (
    subscription_id, organization_id, brand_id, requested_by_user_id, requested_by_org_id,
    reason, cancellation_fee, currency
)
VALUES (
    sqlc.arg(subscription_id), sqlc.arg(organization_id), sqlc.arg(brand_id),
    sqlc.narg(requested_by_user_id), sqlc.arg(requested_by_org_id), sqlc.arg(reason),
    sqlc.arg(cancellation_fee), sqlc.arg(currency)
)
RETURNING *;

-- name: GetServiceSubscriptionCancelRequestByUUID :one
SELECT * FROM service_subscription_cancel_requests
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: DecideServiceSubscriptionCancelRequest :one
UPDATE service_subscription_cancel_requests
SET status = sqlc.arg(status)::text,
    decided_by_user_id = sqlc.narg(decided_by_user_id),
    decided_at = NOW(),
    decision_note = sqlc.narg(decision_note)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id) AND status = 'pending'
RETURNING *;

-- TEC-311: paged subscription list (docs/list-contract.md). Sort keys from
-- servicecatalog usecase SubscriptionsSortSpec; organization_ids NULL = no
-- organization restriction (all/brand scope).
-- name: ListServiceSubscriptionsPage :many
SELECT sqlc.embed(s), o.uuid AS org_uuid, o.name AS organization_name,
       i.uuid AS item_uuid, i.name AS item_name, i.category AS item_category
FROM service_subscriptions s
JOIN organizations o ON o.id = s.organization_id
JOIN service_catalog_items i ON i.id = s.item_id AND i.brand_id = s.brand_id
WHERE s.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(organization_ids)::bigint[] IS NULL OR s.organization_id = ANY (sqlc.narg(organization_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR s.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(org_uuids)::uuid[]), 0) = 0 OR o.uuid = ANY (sqlc.narg(org_uuids)::uuid[]))
  AND (COALESCE(cardinality(sqlc.narg(item_uuids)::uuid[]), 0) = 0 OR i.uuid = ANY (sqlc.narg(item_uuids)::uuid[]))
  AND (sqlc.narg(q)::text IS NULL OR o.name ILIKE '%' || sqlc.narg(q)::text || '%' OR i.name ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(ends_from)::date IS NULL OR s.ends_on >= sqlc.narg(ends_from)::date)
  AND (sqlc.narg(ends_before)::date IS NULL OR s.ends_on < sqlc.narg(ends_before)::date)
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'status' THEN s.status WHEN 'organization_name' THEN o.name WHEN 'item_name' THEN i.name END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'status' THEN s.status WHEN 'organization_name' THEN o.name WHEN 'item_name' THEN i.name END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'starts_on' THEN s.starts_on WHEN 'ends_on' THEN s.ends_on END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'starts_on' THEN s.starts_on WHEN 'ends_on' THEN s.ends_on END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'price' THEN s.price END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'price' THEN s.price END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN s.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN s.created_at END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN s.id END DESC,
  s.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountServiceSubscriptionsPage :one
SELECT count(*)
FROM service_subscriptions s
JOIN organizations o ON o.id = s.organization_id
JOIN service_catalog_items i ON i.id = s.item_id AND i.brand_id = s.brand_id
WHERE s.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(organization_ids)::bigint[] IS NULL OR s.organization_id = ANY (sqlc.narg(organization_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR s.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(org_uuids)::uuid[]), 0) = 0 OR o.uuid = ANY (sqlc.narg(org_uuids)::uuid[]))
  AND (COALESCE(cardinality(sqlc.narg(item_uuids)::uuid[]), 0) = 0 OR i.uuid = ANY (sqlc.narg(item_uuids)::uuid[]))
  AND (sqlc.narg(q)::text IS NULL OR o.name ILIKE '%' || sqlc.narg(q)::text || '%' OR i.name ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(ends_from)::date IS NULL OR s.ends_on >= sqlc.narg(ends_from)::date)
  AND (sqlc.narg(ends_before)::date IS NULL OR s.ends_on < sqlc.narg(ends_before)::date);

-- TEC-311: center cancellation queue (docs/list-contract.md). Sort keys
-- from servicecatalog usecase CancelRequestsSortSpec.
-- name: ListServiceSubscriptionCancelRequestsPage :many
SELECT sqlc.embed(r), s.uuid AS subscription_uuid, s.status AS subscription_status,
       s.starts_on, s.ends_on, o.uuid AS org_uuid, o.name AS organization_name, i.name AS item_name
FROM service_subscription_cancel_requests r
JOIN service_subscriptions s ON s.id = r.subscription_id
JOIN organizations o ON o.id = r.organization_id
JOIN service_catalog_items i ON i.id = s.item_id AND i.brand_id = s.brand_id
WHERE r.brand_id = sqlc.arg(brand_id)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR r.status = ANY (sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(q)::text IS NULL OR o.name ILIKE '%' || sqlc.narg(q)::text || '%' OR i.name ILIKE '%' || sqlc.narg(q)::text || '%' OR r.reason ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR r.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR r.created_at < sqlc.narg(created_before)::timestamptz)
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'status' THEN r.status WHEN 'organization_name' THEN o.name WHEN 'item_name' THEN i.name END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'status' THEN r.status WHEN 'organization_name' THEN o.name WHEN 'item_name' THEN i.name END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'cancellation_fee' THEN r.cancellation_fee END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'cancellation_fee' THEN r.cancellation_fee END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN r.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN r.created_at END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN r.id END DESC,
  r.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountServiceSubscriptionCancelRequestsPage :one
SELECT count(*)
FROM service_subscription_cancel_requests r
JOIN service_subscriptions s ON s.id = r.subscription_id
JOIN organizations o ON o.id = r.organization_id
JOIN service_catalog_items i ON i.id = s.item_id AND i.brand_id = s.brand_id
WHERE r.brand_id = sqlc.arg(brand_id)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR r.status = ANY (sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(q)::text IS NULL OR o.name ILIKE '%' || sqlc.narg(q)::text || '%' OR i.name ILIKE '%' || sqlc.narg(q)::text || '%' OR r.reason ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR r.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR r.created_at < sqlc.narg(created_before)::timestamptz);
