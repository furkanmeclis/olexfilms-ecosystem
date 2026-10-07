-- TEC-348 (F3-07h): paged list reads of the dealer sales screens (sale
-- prices, quick sales, suppliers, purchases). Sort keys come from the
-- dealeraccounting handler specs (docs/list-contract.md); every read is
-- bounded by the dealer organization the API layer resolved.

-- ---------------------------------------------------------------------------
-- Sale price catalog: the brand's active piece products (plus any product
-- the dealer already priced) with the dealer's own price and the brand's
-- recommended price.

-- name: ListDealerPriceCatalog :many
SELECT p.id AS product_id, p.uuid AS product_uuid, p.sku, p.name, p.uses_fixed_barcode,
       dp.sale_price, dp.currency AS sale_currency, dp.updated_at AS price_updated_at,
       pp.recommended_sale_price
FROM products p
LEFT JOIN dealer_product_prices dp
       ON dp.product_id = p.id AND dp.organization_id = sqlc.arg(organization_id)
LEFT JOIN product_prices pp
       ON pp.product_id = p.id AND pp.brand_id = p.brand_id AND pp.currency = sqlc.arg(currency)::text
WHERE p.brand_id = sqlc.arg(brand_id)
  AND ((p.active AND p.unit_type = 'piece') OR dp.id IS NOT NULL)
  AND (sqlc.narg(q)::text IS NULL OR p.name ILIKE '%' || sqlc.narg(q)::text || '%' OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(priced)::boolean IS NULL OR (dp.id IS NOT NULL) = sqlc.narg(priced)::boolean)
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'name' THEN p.name WHEN 'sku' THEN p.sku END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'name' THEN p.name WHEN 'sku' THEN p.sku END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'sale_price' THEN dp.sale_price END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'sale_price' THEN dp.sale_price END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'recommended_sale_price' THEN pp.recommended_sale_price END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'recommended_sale_price' THEN pp.recommended_sale_price END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'updated_at' THEN dp.updated_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'updated_at' THEN dp.updated_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN p.id END DESC,
  p.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountDealerPriceCatalog :one
SELECT COUNT(*)
FROM products p
LEFT JOIN dealer_product_prices dp
       ON dp.product_id = p.id AND dp.organization_id = sqlc.arg(organization_id)
WHERE p.brand_id = sqlc.arg(brand_id)
  AND ((p.active AND p.unit_type = 'piece') OR dp.id IS NOT NULL)
  AND (sqlc.narg(q)::text IS NULL OR p.name ILIKE '%' || sqlc.narg(q)::text || '%' OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(priced)::boolean IS NULL OR (dp.id IS NOT NULL) = sqlc.narg(priced)::boolean);

-- ---------------------------------------------------------------------------
-- Quick sales. voided: the sale's income entry was reversed.

-- name: ListProductSalesPage :many
WITH sales AS (
    SELECT s.*,
           COALESCE(NULLIF(btrim(u.name || ' ' || u.surname), ''), '') AS customer_name,
           u.uuid AS customer_uuid,
           NOT EXISTS (
               SELECT 1 FROM finance_entries e
               WHERE e.source_type = 'product_sale' AND e.source_uuid = s.uuid
                 AND e.reversal_of_id IS NULL
                 AND NOT EXISTS (SELECT 1 FROM finance_entries r WHERE r.reversal_of_id = e.id)
           ) AS voided
    FROM product_sales s
    LEFT JOIN users u ON u.id = s.customer_user_id
    WHERE s.organization_id = sqlc.arg(organization_id)
)
SELECT s.id, s.uuid, s.payment_method, s.currency, s.total, s.sold_at, s.note,
       s.customer_name::text AS customer_name, s.customer_uuid, s.voided::bool AS voided,
       (SELECT COUNT(*) FROM product_sale_lines l WHERE l.sale_id = s.id)::bigint AS line_count,
       (SELECT COALESCE(SUM(l.line_total - COALESCE(l.purchase_unit_cost * l.quantity, 0)), 0)
          FROM product_sale_lines l WHERE l.sale_id = s.id)::numeric(18,2) AS profit,
       (SELECT COALESCE(string_agg(p.name, ', ' ORDER BY l.sort_order, l.id), '')
          FROM product_sale_lines l JOIN products p ON p.id = l.product_id
          WHERE l.sale_id = s.id)::text AS products
FROM sales s
WHERE (COALESCE(cardinality(sqlc.narg(payment_methods)::text[]), 0) = 0
       OR s.payment_method = ANY (sqlc.narg(payment_methods)::text[]))
  AND (sqlc.narg(voided)::boolean IS NULL OR s.voided = sqlc.narg(voided)::boolean)
  AND (sqlc.narg(sold_from)::timestamptz IS NULL OR s.sold_at >= sqlc.narg(sold_from)::timestamptz)
  AND (sqlc.narg(sold_before)::timestamptz IS NULL OR s.sold_at < sqlc.narg(sold_before)::timestamptz)
  AND (sqlc.narg(total_min)::numeric IS NULL OR s.total >= sqlc.narg(total_min)::numeric)
  AND (sqlc.narg(total_max)::numeric IS NULL OR s.total <= sqlc.narg(total_max)::numeric)
  AND (sqlc.narg(q)::text IS NULL
       OR s.customer_name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR s.note ILIKE '%' || sqlc.narg(q)::text || '%'
       OR EXISTS (
           SELECT 1 FROM product_sale_lines l
           JOIN products p ON p.id = l.product_id
           LEFT JOIN units un ON un.id = l.unit_id
           WHERE l.sale_id = s.id
             AND (p.name ILIKE '%' || sqlc.narg(q)::text || '%' OR un.barcode = sqlc.narg(q)::text)
       ))
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'sold_at' THEN s.sold_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'sold_at' THEN s.sold_at END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'total' THEN s.total END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'total' THEN s.total END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'payment_method' THEN s.payment_method END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'payment_method' THEN s.payment_method END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN s.id END DESC,
  s.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountProductSalesPage :one
WITH sales AS (
    SELECT s.*,
           COALESCE(NULLIF(btrim(u.name || ' ' || u.surname), ''), '') AS customer_name,
           NOT EXISTS (
               SELECT 1 FROM finance_entries e
               WHERE e.source_type = 'product_sale' AND e.source_uuid = s.uuid
                 AND e.reversal_of_id IS NULL
                 AND NOT EXISTS (SELECT 1 FROM finance_entries r WHERE r.reversal_of_id = e.id)
           ) AS voided
    FROM product_sales s
    LEFT JOIN users u ON u.id = s.customer_user_id
    WHERE s.organization_id = sqlc.arg(organization_id)
)
SELECT COUNT(*)
FROM sales s
WHERE (COALESCE(cardinality(sqlc.narg(payment_methods)::text[]), 0) = 0
       OR s.payment_method = ANY (sqlc.narg(payment_methods)::text[]))
  AND (sqlc.narg(voided)::boolean IS NULL OR s.voided = sqlc.narg(voided)::boolean)
  AND (sqlc.narg(sold_from)::timestamptz IS NULL OR s.sold_at >= sqlc.narg(sold_from)::timestamptz)
  AND (sqlc.narg(sold_before)::timestamptz IS NULL OR s.sold_at < sqlc.narg(sold_before)::timestamptz)
  AND (sqlc.narg(total_min)::numeric IS NULL OR s.total >= sqlc.narg(total_min)::numeric)
  AND (sqlc.narg(total_max)::numeric IS NULL OR s.total <= sqlc.narg(total_max)::numeric)
  AND (sqlc.narg(q)::text IS NULL
       OR s.customer_name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR s.note ILIKE '%' || sqlc.narg(q)::text || '%'
       OR EXISTS (
           SELECT 1 FROM product_sale_lines l
           JOIN products p ON p.id = l.product_id
           LEFT JOIN units un ON un.id = l.unit_id
           WHERE l.sale_id = s.id
             AND (p.name ILIKE '%' || sqlc.narg(q)::text || '%' OR un.barcode = sqlc.narg(q)::text)
       ));

-- ---------------------------------------------------------------------------
-- Suppliers.

-- name: ListSuppliersPage :many
SELECT * FROM suppliers
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::boolean IS NULL OR active = sqlc.narg(active)::boolean)
  AND (sqlc.narg(q)::text IS NULL
       OR name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR tax_no ILIKE '%' || sqlc.narg(q)::text || '%'
       OR phone_e164 ILIKE '%' || sqlc.narg(q)::text || '%'
       OR email ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR created_at < sqlc.narg(created_before)::timestamptz)
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN name END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN name END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN created_at WHEN 'updated_at' THEN updated_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN created_at WHEN 'updated_at' THEN updated_at END
  END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN id END DESC,
  id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountSuppliersPage :one
SELECT COUNT(*) FROM suppliers
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::boolean IS NULL OR active = sqlc.narg(active)::boolean)
  AND (sqlc.narg(q)::text IS NULL
       OR name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR tax_no ILIKE '%' || sqlc.narg(q)::text || '%'
       OR phone_e164 ILIKE '%' || sqlc.narg(q)::text || '%'
       OR email ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR created_at < sqlc.narg(created_before)::timestamptz);

-- ---------------------------------------------------------------------------
-- Purchases with the supplier name and the (single) line description.

-- name: ListPurchasesPage :many
SELECT pu.id, pu.uuid, pu.purchased_on, pu.currency, pu.amount, pu.payment_method, pu.note, pu.created_at,
       su.uuid AS supplier_uuid, su.name AS supplier_name,
       COALESCE((SELECT l.description FROM purchase_lines l WHERE l.purchase_id = pu.id ORDER BY l.sort_order, l.id LIMIT 1), '')::text AS description
FROM purchases pu
JOIN suppliers su ON su.id = pu.supplier_id
WHERE pu.organization_id = sqlc.arg(organization_id)
  AND (COALESCE(cardinality(sqlc.narg(supplier_uuids)::uuid[]), 0) = 0
       OR su.uuid = ANY (sqlc.narg(supplier_uuids)::uuid[]))
  AND (COALESCE(cardinality(sqlc.narg(payment_methods)::text[]), 0) = 0
       OR pu.payment_method = ANY (sqlc.narg(payment_methods)::text[]))
  AND (sqlc.narg(date_from)::date IS NULL OR pu.purchased_on >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_before)::date IS NULL OR pu.purchased_on < sqlc.narg(date_before)::date)
  AND (sqlc.narg(amount_min)::numeric IS NULL OR pu.amount >= sqlc.narg(amount_min)::numeric)
  AND (sqlc.narg(amount_max)::numeric IS NULL OR pu.amount <= sqlc.narg(amount_max)::numeric)
  AND (sqlc.narg(q)::text IS NULL
       OR su.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR pu.note ILIKE '%' || sqlc.narg(q)::text || '%'
       OR EXISTS (SELECT 1 FROM purchase_lines l WHERE l.purchase_id = pu.id
                  AND l.description ILIKE '%' || sqlc.narg(q)::text || '%'))
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'purchased_on' THEN pu.purchased_on END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'purchased_on' THEN pu.purchased_on END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'amount' THEN pu.amount END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'amount' THEN pu.amount END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'supplier_name' THEN su.name WHEN 'payment_method' THEN pu.payment_method::text END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'supplier_name' THEN su.name WHEN 'payment_method' THEN pu.payment_method::text END
  END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN pu.id END DESC,
  pu.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountPurchasesPage :one
SELECT COUNT(*)
FROM purchases pu
JOIN suppliers su ON su.id = pu.supplier_id
WHERE pu.organization_id = sqlc.arg(organization_id)
  AND (COALESCE(cardinality(sqlc.narg(supplier_uuids)::uuid[]), 0) = 0
       OR su.uuid = ANY (sqlc.narg(supplier_uuids)::uuid[]))
  AND (COALESCE(cardinality(sqlc.narg(payment_methods)::text[]), 0) = 0
       OR pu.payment_method = ANY (sqlc.narg(payment_methods)::text[]))
  AND (sqlc.narg(date_from)::date IS NULL OR pu.purchased_on >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_before)::date IS NULL OR pu.purchased_on < sqlc.narg(date_before)::date)
  AND (sqlc.narg(amount_min)::numeric IS NULL OR pu.amount >= sqlc.narg(amount_min)::numeric)
  AND (sqlc.narg(amount_max)::numeric IS NULL OR pu.amount <= sqlc.narg(amount_max)::numeric)
  AND (sqlc.narg(q)::text IS NULL
       OR su.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR pu.note ILIKE '%' || sqlc.narg(q)::text || '%'
       OR EXISTS (SELECT 1 FROM purchase_lines l WHERE l.purchase_id = pu.id
                  AND l.description ILIKE '%' || sqlc.narg(q)::text || '%'));
