-- TEC-1214: brand currency management (platform). brands.currency is the
-- price and order currency of the brand; organizations.currency stays the
-- ledger currency. Existing orders, caris and finance entries keep their
-- own currency and frozen rates.

-- name: ListBrandsWithCurrency :many
SELECT b.uuid, b.slug, b.name, b.status, b.currency::text AS currency
FROM brands b
ORDER BY b.id;

-- name: LockBrandByUUID :one
SELECT * FROM brands WHERE uuid = sqlc.arg(uuid) FOR UPDATE;

-- name: GetBrandByUUID :one
SELECT * FROM brands WHERE uuid = sqlc.arg(uuid);

-- name: SetBrandCurrency :one
UPDATE brands SET currency = sqlc.arg(currency)::text
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ListBrandProductsMissingPrice :many
-- Active products of the brand without a center sale price
-- (sale_to_distributor_price) in the currency; total_count is the full
-- count (rows stop at row_limit).
SELECT p.uuid, p.sku, p.name, COUNT(*) OVER ()::bigint AS total_count
FROM products p
WHERE p.brand_id = sqlc.arg(brand_id)
  AND p.active
  AND NOT EXISTS (SELECT 1 FROM product_prices pp
                  WHERE pp.product_id = p.id AND pp.currency = sqlc.arg(currency)::text
                    AND pp.sale_to_distributor_price IS NOT NULL)
ORDER BY p.sku, p.id
LIMIT sqlc.arg(row_limit);

-- name: CountBrandActiveProducts :one
SELECT COUNT(*)::bigint FROM products WHERE brand_id = sqlc.arg(brand_id) AND active;

-- name: ListBrandOpenOrders :many
-- Draft and submitted (waiting for approval) orders of the brand: their
-- prices are not frozen yet. total_count is the full count.
SELECT o.id, o.uuid, o.order_no, o.status, o.currency::text AS currency, o.total::text AS total,
       s.name AS seller_name, b.name AS buyer_name, o.created_at,
       COUNT(*) OVER ()::bigint AS total_count
FROM orders o
JOIN organizations s ON s.id = o.seller_org_id
JOIN organizations b ON b.id = o.buyer_org_id
WHERE o.brand_id = sqlc.arg(brand_id)
  AND o.status IN ('draft', 'submitted')
ORDER BY o.created_at, o.id
LIMIT sqlc.arg(row_limit);

-- name: LockBrandOpenOrders :many
SELECT o.id FROM orders o
WHERE o.brand_id = sqlc.arg(brand_id) AND o.status IN ('draft', 'submitted')
ORDER BY o.id
FOR UPDATE;

-- name: RepriceOrderItems :execrows
-- Converts the unfrozen line prices of an open order with a rate (brand
-- currency change, "reprice" option).
UPDATE order_items
SET unit_price = ROUND(unit_price * sqlc.arg(rate)::numeric, 4),
    list_unit_price = CASE WHEN list_unit_price IS NULL THEN NULL ELSE ROUND(list_unit_price * sqlc.arg(rate)::numeric, 4) END,
    recommended_price_snapshot = CASE WHEN recommended_price_snapshot IS NULL THEN NULL
                                      ELSE ROUND(recommended_price_snapshot * sqlc.arg(rate)::numeric, 4) END,
    line_total = ROUND(line_total * sqlc.arg(rate)::numeric, 2)
WHERE order_id = sqlc.arg(order_id);

-- name: RepriceOrderHeader :exec
UPDATE orders o
SET currency = sqlc.arg(currency)::text,
    tax_total = ROUND(o.tax_total * sqlc.arg(rate)::numeric, 2),
    subtotal = s.subtotal,
    total = s.subtotal + ROUND(o.tax_total * sqlc.arg(rate)::numeric, 2)
FROM (SELECT COALESCE(SUM(line_total), 0)::numeric(18,2) AS subtotal
      FROM order_items WHERE order_id = sqlc.arg(id)) s
WHERE o.id = sqlc.arg(id);

-- name: ListBrandPricesInCurrency :many
-- Price list rows of the brand in the source currency next to the row of
-- the target currency (price conversion tool).
SELECT p.id AS product_id, p.uuid AS product_uuid, p.sku, p.name,
       src.purchase_price AS purchase_price,
       src.sale_to_distributor_price AS sale_to_distributor_price,
       src.recommended_sale_price AS recommended_sale_price,
       src.meter_price AS meter_price,
       (dst.id IS NOT NULL)::bool AS target_exists,
       dst.purchase_price AS target_purchase_price,
       dst.sale_to_distributor_price AS target_sale_to_distributor_price,
       dst.recommended_sale_price AS target_recommended_sale_price,
       dst.meter_price AS target_meter_price
FROM product_prices src
JOIN products p ON p.id = src.product_id AND p.brand_id = src.brand_id
LEFT JOIN product_prices dst ON dst.product_id = src.product_id AND dst.currency = sqlc.arg(target)::text
WHERE src.brand_id = sqlc.arg(brand_id)
  AND src.currency = sqlc.arg(source)::text
  AND p.active
ORDER BY p.sku, p.id;

-- name: UpsertConvertedPrice :exec
INSERT INTO product_prices (product_id, brand_id, currency, purchase_price, sale_to_distributor_price, recommended_sale_price,
                            meter_price)
VALUES (sqlc.arg(product_id), sqlc.arg(brand_id), sqlc.arg(currency)::text,
        sqlc.narg(purchase_price)::numeric, sqlc.narg(sale_to_distributor_price)::numeric,
        sqlc.narg(recommended_sale_price)::numeric, sqlc.narg(meter_price)::numeric)
ON CONFLICT (product_id, currency) DO UPDATE SET
    purchase_price = EXCLUDED.purchase_price,
    sale_to_distributor_price = EXCLUDED.sale_to_distributor_price,
    recommended_sale_price = EXCLUDED.recommended_sale_price,
    -- TEC-1238: the meter list price is converted with the others.
    meter_price = EXCLUDED.meter_price;

-- name: GetSessionCurrencies :one
-- /auth/me (TEC-1214): the brand currency (price and order currency) of the
-- active organization's brand, else of the request brand, and the active
-- organization's ledger currency ('' when unknown).
SELECT COALESCE(
           (SELECT b.currency::text FROM organizations o JOIN brands b ON b.id = o.brand_id
            WHERE o.uuid = sqlc.narg(org_uuid)::uuid AND o.deleted_at IS NULL),
           (SELECT b.currency::text FROM brands b WHERE b.id = sqlc.narg(brand_id)::bigint),
           '')::text AS brand_currency,
       COALESCE(
           (SELECT o.currency::text FROM organizations o
            WHERE o.uuid = sqlc.narg(org_uuid)::uuid AND o.deleted_at IS NULL),
           '')::text AS organization_currency;
