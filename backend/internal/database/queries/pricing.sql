-- TEC-144: product price list and distributor-specific prices (K8). Prices
-- travel as text so no precision is lost between NUMERIC and Go. Field
-- masking by pricing.* permission happens in the use case layer.

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
    purchase_price::text AS purchase_price,
    sale_to_distributor_price::text AS sale_to_distributor_price,
    recommended_sale_price::text AS recommended_sale_price,
    created_at, updated_at;

-- name: GetProductPrice :one
SELECT id, product_id, brand_id, currency,
    purchase_price::text AS purchase_price,
    sale_to_distributor_price::text AS sale_to_distributor_price,
    recommended_sale_price::text AS recommended_sale_price,
    created_at, updated_at
FROM product_prices
WHERE product_id = sqlc.arg(product_id) AND brand_id = sqlc.arg(brand_id)
  AND currency = sqlc.arg(currency);

-- name: ListProductPrices :many
SELECT id, product_id, brand_id, currency,
    purchase_price::text AS purchase_price,
    sale_to_distributor_price::text AS sale_to_distributor_price,
    recommended_sale_price::text AS recommended_sale_price,
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
