-- TEC-144: product price list and distributor-specific prices (K8). Prices
-- go in as text so no precision is lost between NUMERIC and Go. Nullable
-- price columns come back as NUMERIC (a NULL cannot scan into a text cast's
-- string); NOT NULL prices come back as text. Field masking by pricing.*
-- permission happens in the use case layer (TEC-146).

-- name: UpsertProductPrice :one
INSERT INTO product_prices (
    product_id, brand_id, currency, purchase_price, sale_to_distributor_price, recommended_sale_price
)
VALUES (
    sqlc.arg(product_id), sqlc.arg(brand_id), sqlc.arg(currency),
    sqlc.narg(purchase_price)::text::numeric,
    sqlc.narg(sale_to_distributor_price)::text::numeric,
    sqlc.narg(recommended_sale_price)::text::numeric
)
ON CONFLICT (product_id, currency) DO UPDATE SET
    purchase_price = EXCLUDED.purchase_price,
    sale_to_distributor_price = EXCLUDED.sale_to_distributor_price,
    recommended_sale_price = EXCLUDED.recommended_sale_price
RETURNING id, product_id, brand_id, currency,
    purchase_price,
    sale_to_distributor_price,
    recommended_sale_price,
    created_at, updated_at;

-- name: GetProductPrice :one
SELECT id, product_id, brand_id, currency,
    purchase_price,
    sale_to_distributor_price,
    recommended_sale_price,
    created_at, updated_at
FROM product_prices
WHERE product_id = sqlc.arg(product_id) AND brand_id = sqlc.arg(brand_id)
  AND currency = sqlc.arg(currency);

-- name: ListProductPrices :many
SELECT id, product_id, brand_id, currency,
    purchase_price,
    sale_to_distributor_price,
    recommended_sale_price,
    created_at, updated_at
FROM product_prices
WHERE product_id = sqlc.arg(product_id) AND brand_id = sqlc.arg(brand_id)
ORDER BY currency ASC;

-- name: DeleteProductPrice :execrows
DELETE FROM product_prices
WHERE product_id = sqlc.arg(product_id) AND brand_id = sqlc.arg(brand_id)
  AND currency = sqlc.arg(currency);

-- name: UpsertDistributorPriceOverride :one
-- The database refuses a target that is not a distributor of the brand.
INSERT INTO distributor_price_overrides (product_id, brand_id, distributor_org_id, currency, price)
VALUES (
    sqlc.arg(product_id), sqlc.arg(brand_id), sqlc.arg(distributor_org_id),
    sqlc.arg(currency), sqlc.arg(price)::text::numeric
)
ON CONFLICT (product_id, distributor_org_id, currency) DO UPDATE SET
    price = EXCLUDED.price
RETURNING id, product_id, brand_id, distributor_org_id, currency, price::text AS price,
    created_at, updated_at;

-- name: GetDistributorPriceOverride :one
SELECT id, product_id, brand_id, distributor_org_id, currency, price::text AS price,
    created_at, updated_at
FROM distributor_price_overrides
WHERE product_id = sqlc.arg(product_id) AND brand_id = sqlc.arg(brand_id)
  AND distributor_org_id = sqlc.arg(distributor_org_id) AND currency = sqlc.arg(currency);

-- name: ListDistributorPriceOverrides :many
SELECT id, product_id, brand_id, distributor_org_id, currency, price::text AS price,
    created_at, updated_at
FROM distributor_price_overrides
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(product_id)::bigint IS NULL OR product_id = sqlc.narg(product_id)::bigint)
  AND (sqlc.narg(distributor_org_id)::bigint IS NULL OR distributor_org_id = sqlc.narg(distributor_org_id)::bigint)
ORDER BY product_id ASC, distributor_org_id ASC, currency ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountDistributorPriceOverrides :one
SELECT COUNT(*)::bigint FROM distributor_price_overrides
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(product_id)::bigint IS NULL OR product_id = sqlc.narg(product_id)::bigint)
  AND (sqlc.narg(distributor_org_id)::bigint IS NULL OR distributor_org_id = sqlc.narg(distributor_org_id)::bigint);

-- name: DeleteDistributorPriceOverride :execrows
DELETE FROM distributor_price_overrides
WHERE product_id = sqlc.arg(product_id) AND brand_id = sqlc.arg(brand_id)
  AND distributor_org_id = sqlc.arg(distributor_org_id) AND currency = sqlc.arg(currency);

-- TEC-146: batch reads for the effective price views and the distributor's
-- dealer prices (000041).

-- name: ListPricedProducts :many
-- Products of the brand for the price list view.
SELECT id, uuid, sku, name, active
FROM products
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(active)::bool IS NULL OR active = sqlc.narg(active)::bool)
  AND (sqlc.narg(q)::text IS NULL OR name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR sku ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY sku ASC, id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountPricedProducts :one
SELECT COUNT(*)::bigint FROM products
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(active)::bool IS NULL OR active = sqlc.narg(active)::bool)
  AND (sqlc.narg(q)::text IS NULL OR name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR sku ILIKE '%' || sqlc.narg(q)::text || '%');

-- name: ListProductPricesForProducts :many
SELECT product_id, currency,
    purchase_price,
    sale_to_distributor_price,
    recommended_sale_price
FROM product_prices
WHERE brand_id = sqlc.arg(brand_id) AND product_id = ANY(sqlc.arg(product_ids)::bigint[])
ORDER BY product_id ASC, currency ASC;

-- name: ListDistributorOverridesForProducts :many
SELECT product_id, currency, price::text AS price
FROM distributor_price_overrides
WHERE brand_id = sqlc.arg(brand_id) AND distributor_org_id = sqlc.arg(distributor_org_id)
  AND product_id = ANY(sqlc.arg(product_ids)::bigint[])
ORDER BY product_id ASC, currency ASC;

-- name: ListDealerPricesForProducts :many
SELECT product_id, currency, price::text AS price
FROM distributor_dealer_prices
WHERE brand_id = sqlc.arg(brand_id) AND distributor_org_id = sqlc.arg(distributor_org_id)
  AND product_id = ANY(sqlc.arg(product_ids)::bigint[])
ORDER BY product_id ASC, currency ASC;

-- name: ListDistributorOverrideDetails :many
-- Center view of the distributor-specific prices with product and
-- distributor identities.
SELECT o.currency, o.price::text AS price, o.updated_at,
    p.uuid AS product_uuid, p.sku AS product_sku, p.name AS product_name,
    d.uuid AS distributor_uuid, d.name AS distributor_name
FROM distributor_price_overrides o
JOIN products p ON p.id = o.product_id
JOIN organizations d ON d.id = o.distributor_org_id
WHERE o.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(product_id)::bigint IS NULL OR o.product_id = sqlc.narg(product_id)::bigint)
  AND (sqlc.narg(distributor_org_id)::bigint IS NULL OR o.distributor_org_id = sqlc.narg(distributor_org_id)::bigint)
ORDER BY p.sku ASC, d.name ASC, o.currency ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: UpsertDistributorDealerPrice :one
-- The database refuses an owner that is not a distributor of the brand.
INSERT INTO distributor_dealer_prices (product_id, brand_id, distributor_org_id, currency, price)
VALUES (
    sqlc.arg(product_id), sqlc.arg(brand_id), sqlc.arg(distributor_org_id),
    sqlc.arg(currency), sqlc.arg(price)::text::numeric
)
ON CONFLICT (product_id, distributor_org_id, currency) DO UPDATE SET
    price = EXCLUDED.price
RETURNING id, product_id, brand_id, distributor_org_id, currency, price::text AS price,
    created_at, updated_at;

-- name: DeleteDistributorDealerPrice :execrows
DELETE FROM distributor_dealer_prices
WHERE product_id = sqlc.arg(product_id) AND brand_id = sqlc.arg(brand_id)
  AND distributor_org_id = sqlc.arg(distributor_org_id) AND currency = sqlc.arg(currency);

-- name: ListProductsByUUIDs :many
-- TEC-211: product refs of the brand for the catalog export price columns.
SELECT id, uuid, sku, name
FROM products
WHERE brand_id = sqlc.arg(brand_id) AND uuid = ANY(sqlc.arg(uuids)::uuid[])
ORDER BY id ASC;
