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

-- name: GetServiceCatalogItem :one
SELECT * FROM service_catalog_items
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: ListServiceCatalogItems :many
SELECT * FROM service_catalog_items
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(category)::text IS NULL OR category = sqlc.narg(category)::text)
  AND (sqlc.narg(is_active)::boolean IS NULL OR is_active = sqlc.narg(is_active)::boolean)
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

-- name: GetServiceSubscriptionByUUID :one
SELECT * FROM service_subscriptions
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: LockServiceSubscription :one
SELECT * FROM service_subscriptions
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
FOR UPDATE;

-- name: ListServiceSubscriptionsByOrgs :many
SELECT * FROM service_subscriptions
WHERE brand_id = sqlc.arg(brand_id)
  AND organization_id = ANY(sqlc.arg(organization_ids)::bigint[])
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY created_at DESC, id DESC;

-- name: SetServiceSubscriptionStatus :one
UPDATE service_subscriptions
SET status = sqlc.arg(status),
    cancelled_at = CASE WHEN sqlc.arg(status)::text = 'cancelled' THEN NOW() ELSE cancelled_at END,
    expired_at = CASE WHEN sqlc.arg(status)::text = 'expired' THEN NOW() ELSE expired_at END
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: ExpireServiceSubscriptions :many
UPDATE service_subscriptions
SET status = 'expired', expired_at = NOW()
WHERE status IN ('active', 'cancel_requested') AND ends_on < sqlc.arg(today)::date
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

-- name: ListServiceSubscriptionCancelRequests :many
SELECT * FROM service_subscription_cancel_requests
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY created_at DESC, id DESC;

-- name: DecideServiceSubscriptionCancelRequest :one
UPDATE service_subscription_cancel_requests
SET status = sqlc.arg(status),
    decided_by_user_id = sqlc.narg(decided_by_user_id),
    decided_at = NOW(),
    decision_note = sqlc.narg(decision_note)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id) AND status = 'pending'
RETURNING *;
