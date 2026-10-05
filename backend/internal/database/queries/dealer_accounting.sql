-- TEC-341 (F3-07a): dealer accounting schema (migration 000093). Every read
-- and write is bounded by the organization the API layer resolved; ledger
-- rows are written through ledger.Post and only linked here.

-- ---------------------------------------------------------------------------
-- Customer cari.

-- name: GetServedCustomerByUUID :one
SELECT u.*
FROM users u
JOIN customer_organizations co ON co.user_id = u.id
WHERE u.uuid = sqlc.arg(uuid)
  AND co.organization_id = sqlc.arg(organization_id)
  AND u.deleted_at IS NULL;

-- CreateCariForUserIfMissing opens the cari of organization_id with a
-- customer. When it already exists no row is returned (pgx.ErrNoRows) and
-- the caller reads it with GetCariAccountByCounterpartyUser.
-- name: CreateCariForUserIfMissing :one
INSERT INTO cari_accounts (organization_id, brand_id, counterparty_type, counterparty_user_id, currency)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), 'user',
    sqlc.arg(counterparty_user_id)::bigint, sqlc.arg(currency)
)
ON CONFLICT (organization_id, counterparty_user_id) WHERE counterparty_user_id IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetCariAccountByCounterpartyUser :one
SELECT * FROM cari_accounts
WHERE organization_id = sqlc.arg(organization_id)
  AND counterparty_user_id = sqlc.arg(counterparty_user_id)::bigint;

-- name: ListCustomerCariAccounts :many
SELECT * FROM cari_accounts
WHERE organization_id = sqlc.arg(organization_id)
  AND counterparty_type = 'user'
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountCustomerCariAccounts :one
SELECT COUNT(*) FROM cari_accounts
WHERE organization_id = sqlc.arg(organization_id)
  AND counterparty_type = 'user';

-- ---------------------------------------------------------------------------
-- Dealer product prices.

-- name: UpsertDealerProductPrice :one
INSERT INTO dealer_product_prices (
    organization_id, brand_id, product_id, sale_price, currency, updated_by_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(product_id),
    sqlc.arg(sale_price), sqlc.arg(currency), sqlc.narg(updated_by_user_id)
)
ON CONFLICT (organization_id, product_id) DO UPDATE
SET sale_price = EXCLUDED.sale_price,
    currency = EXCLUDED.currency,
    updated_by_user_id = EXCLUDED.updated_by_user_id
RETURNING *;

-- name: GetDealerProductPrice :one
SELECT * FROM dealer_product_prices
WHERE organization_id = sqlc.arg(organization_id) AND product_id = sqlc.arg(product_id);

-- name: ListDealerProductPrices :many
SELECT * FROM dealer_product_prices
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.arg(product_ids)::bigint[] IS NULL OR product_id = ANY(sqlc.arg(product_ids)::bigint[]))
ORDER BY product_id;

-- name: GetRecommendedProductPrice :one
SELECT recommended_sale_price
FROM product_prices
WHERE brand_id = sqlc.arg(brand_id)
  AND product_id = sqlc.arg(product_id)
  AND currency = sqlc.arg(currency)::text
  AND recommended_sale_price IS NOT NULL;

-- name: DeleteDealerProductPrice :execrows
DELETE FROM dealer_product_prices
WHERE organization_id = sqlc.arg(organization_id) AND product_id = sqlc.arg(product_id);

-- ---------------------------------------------------------------------------
-- Product sales.

-- name: CreateProductSale :one
INSERT INTO product_sales (
    organization_id, brand_id, customer_user_id, payment_method, cari_id,
    currency, total, sold_at, note, created_by_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.narg(customer_user_id),
    sqlc.arg(payment_method), sqlc.narg(cari_id), sqlc.arg(currency), sqlc.arg(total),
    sqlc.arg(sold_at), sqlc.arg(note), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: CreateProductSaleLine :one
INSERT INTO product_sale_lines (
    sale_id, organization_id, brand_id, product_id, unit_id, quantity,
    unit_price, line_total, purchase_unit_cost, stock_movement_id, sort_order
) VALUES (
    sqlc.arg(sale_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(product_id),
    sqlc.narg(unit_id), sqlc.arg(quantity), sqlc.arg(unit_price), sqlc.arg(line_total),
    sqlc.narg(purchase_unit_cost), sqlc.narg(stock_movement_id), sqlc.arg(sort_order)
)
RETURNING *;

-- name: SetProductSaleFinanceEntry :one
UPDATE product_sales
SET finance_entry_id = sqlc.arg(finance_entry_id)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
  AND finance_entry_id IS NULL
RETURNING *;

-- name: GetProductSaleByUUID :one
SELECT * FROM product_sales
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: CountOpenFinanceEntriesBySource :one
SELECT COUNT(*)
FROM finance_entries e
WHERE e.source_type = sqlc.arg(source_type)::text
  AND e.source_uuid = sqlc.arg(source_uuid)
  AND e.reversal_of_id IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM finance_entries r WHERE r.reversal_of_id = e.id
  );

-- name: ListProductSaleLines :many
SELECT * FROM product_sale_lines
WHERE sale_id = sqlc.arg(sale_id) AND organization_id = sqlc.arg(organization_id)
ORDER BY sort_order, id;

-- name: ListProductSaleLinesBySales :many
SELECT * FROM product_sale_lines
WHERE organization_id = sqlc.arg(organization_id)
  AND sale_id = ANY(sqlc.arg(sale_ids)::bigint[])
ORDER BY sale_id, sort_order, id;

-- name: ListProductSales :many
SELECT * FROM product_sales
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(customer_user_id)::bigint IS NULL OR customer_user_id = sqlc.narg(customer_user_id)::bigint)
  AND (sqlc.narg(payment_method)::varchar IS NULL OR payment_method = sqlc.narg(payment_method)::varchar)
  AND (sqlc.narg(sold_from)::timestamptz IS NULL OR sold_at >= sqlc.narg(sold_from)::timestamptz)
  AND (sqlc.narg(sold_to)::timestamptz IS NULL OR sold_at < sqlc.narg(sold_to)::timestamptz)
ORDER BY sold_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountProductSales :one
SELECT COUNT(*) FROM product_sales
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(customer_user_id)::bigint IS NULL OR customer_user_id = sqlc.narg(customer_user_id)::bigint)
  AND (sqlc.narg(payment_method)::varchar IS NULL OR payment_method = sqlc.narg(payment_method)::varchar)
  AND (sqlc.narg(sold_from)::timestamptz IS NULL OR sold_at >= sqlc.narg(sold_from)::timestamptz)
  AND (sqlc.narg(sold_to)::timestamptz IS NULL OR sold_at < sqlc.narg(sold_to)::timestamptz);

-- Profit per product over a period: revenue and the purchase cost snapshot
-- of the sold lines (lines without a cost snapshot count as zero cost and
-- are reported separately).
-- name: ProductSaleProfitByProduct :many
SELECT l.product_id,
       SUM(l.quantity)::numeric(18,2)                                              AS quantity,
       SUM(l.line_total)::numeric(18,2)                                            AS revenue,
       COALESCE(SUM(l.purchase_unit_cost * l.quantity), 0)::numeric(18,2)          AS cost,
       COUNT(*) FILTER (WHERE l.purchase_unit_cost IS NULL)::bigint                AS lines_without_cost
FROM product_sale_lines l
JOIN product_sales s ON s.id = l.sale_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.sold_at >= sqlc.arg(sold_from)::timestamptz
  AND s.sold_at < sqlc.arg(sold_to)::timestamptz
GROUP BY l.product_id
ORDER BY l.product_id;

-- name: ListProductSaleStockCandidates :many
WITH stock AS (
    SELECT s.unit_id, s.owner_type, s.owner_id, s.holder_org_id, 1::int AS quantity_on_hand
    FROM unit_current_state s
    WHERE s.holder_org_id = sqlc.arg(organization_id)
      AND s.brand_id = sqlc.arg(brand_id)
      AND s.status IN ('available', 'placed')
      AND s.owner_type IN ('organization', 'warehouse_location')
    UNION ALL
    SELECT h.unit_id, h.owner_type, h.owner_id, h.holder_org_id, h.quantity_on_hand
    FROM fixed_barcode_holdings h
    WHERE h.holder_org_id = sqlc.arg(organization_id)
      AND h.brand_id = sqlc.arg(brand_id)
      AND h.owner_type IN ('organization', 'warehouse_location')
      AND h.quantity_on_hand > 0
)
SELECT u.id AS unit_id, u.uuid AS unit_uuid, u.product_id, u.barcode, u.unit_kind,
       u.initial_meters, u.remaining_meters,
       stock.owner_type, stock.owner_id, stock.holder_org_id, stock.quantity_on_hand
FROM stock
JOIN units u ON u.id = stock.unit_id
WHERE u.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(product_id)::bigint IS NULL OR u.product_id = sqlc.narg(product_id)::bigint)
  AND (sqlc.narg(barcode)::text IS NULL OR u.barcode = sqlc.narg(barcode)::text)
ORDER BY CASE WHEN stock.owner_type = 'organization' THEN 0 ELSE 1 END, u.id, stock.owner_id;

-- ---------------------------------------------------------------------------
-- Suppliers.

-- name: CreateSupplier :one
INSERT INTO suppliers (organization_id, brand_id, name, tax_no, phone_e164, email, note)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(name), sqlc.narg(tax_no),
    sqlc.narg(phone_e164), sqlc.narg(email), sqlc.arg(note)
)
RETURNING *;

-- name: UpdateSupplier :one
UPDATE suppliers
SET name = sqlc.arg(name),
    tax_no = sqlc.narg(tax_no),
    phone_e164 = sqlc.narg(phone_e164),
    email = sqlc.narg(email),
    note = sqlc.arg(note),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
RETURNING *;

-- name: GetSupplierByUUID :one
SELECT * FROM suppliers
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: ListSuppliers :many
SELECT * FROM suppliers
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::boolean IS NULL OR active = sqlc.narg(active)::boolean)
  AND (sqlc.narg(q)::text IS NULL OR name ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY name, id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountSuppliers :one
SELECT COUNT(*) FROM suppliers
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::boolean IS NULL OR active = sqlc.narg(active)::boolean)
  AND (sqlc.narg(q)::text IS NULL OR name ILIKE '%' || sqlc.narg(q)::text || '%');

-- ---------------------------------------------------------------------------
-- Purchases (no stock movement, K12).

-- name: CreatePurchase :one
INSERT INTO purchases (
    organization_id, brand_id, supplier_id, purchased_on, currency, amount,
    payment_method, note, created_by_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(supplier_id), sqlc.arg(purchased_on),
    sqlc.arg(currency), sqlc.arg(amount), sqlc.arg(payment_method), sqlc.arg(note),
    sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: CreatePurchaseLine :one
INSERT INTO purchase_lines (
    purchase_id, organization_id, brand_id, description, quantity, unit_price, line_total, sort_order
) VALUES (
    sqlc.arg(purchase_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(description),
    sqlc.arg(quantity), sqlc.arg(unit_price), sqlc.arg(line_total), sqlc.arg(sort_order)
)
RETURNING *;

-- name: SetPurchaseFinanceEntry :one
UPDATE purchases
SET finance_entry_id = sqlc.arg(finance_entry_id)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
  AND finance_entry_id IS NULL
RETURNING *;

-- name: GetPurchaseByUUID :one
SELECT * FROM purchases
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: ListPurchaseLines :many
SELECT * FROM purchase_lines
WHERE purchase_id = sqlc.arg(purchase_id) AND organization_id = sqlc.arg(organization_id)
ORDER BY sort_order, id;

-- name: ListPurchases :many
SELECT * FROM purchases
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(supplier_id)::bigint IS NULL OR supplier_id = sqlc.narg(supplier_id)::bigint)
  AND (sqlc.narg(date_from)::date IS NULL OR purchased_on >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR purchased_on <= sqlc.narg(date_to)::date)
ORDER BY purchased_on DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountPurchases :one
SELECT COUNT(*) FROM purchases
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(supplier_id)::bigint IS NULL OR supplier_id = sqlc.narg(supplier_id)::bigint)
  AND (sqlc.narg(date_from)::date IS NULL OR purchased_on >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR purchased_on <= sqlc.narg(date_to)::date);

-- ---------------------------------------------------------------------------
-- Staff.

-- name: CreateStaffProfile :one
INSERT INTO staff_profiles (
    organization_id, brand_id, user_id, name, title, hired_on, monthly_salary, currency, active
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.narg(user_id), sqlc.arg(name),
    sqlc.narg(title), sqlc.narg(hired_on), sqlc.narg(monthly_salary), sqlc.arg(currency),
    sqlc.arg(active)
)
RETURNING *;

-- name: UpdateStaffProfile :one
UPDATE staff_profiles
SET user_id = sqlc.narg(user_id),
    name = sqlc.arg(name),
    title = sqlc.narg(title),
    hired_on = sqlc.narg(hired_on),
    monthly_salary = sqlc.narg(monthly_salary),
    currency = sqlc.arg(currency),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
RETURNING *;

-- name: GetStaffProfileByUUID :one
SELECT * FROM staff_profiles
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: ListStaffProfiles :many
SELECT * FROM staff_profiles
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::boolean IS NULL OR active = sqlc.narg(active)::boolean)
ORDER BY name, id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountStaffProfiles :one
SELECT COUNT(*) FROM staff_profiles
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::boolean IS NULL OR active = sqlc.narg(active)::boolean);

-- ---------------------------------------------------------------------------
-- Staff payments.

-- name: CreateStaffPayment :one
INSERT INTO staff_payments (
    organization_id, brand_id, staff_id, type, period, amount, currency,
    paid_on, description, created_by_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(staff_id), sqlc.arg(type),
    sqlc.arg(period), sqlc.arg(amount), sqlc.arg(currency), sqlc.arg(paid_on),
    sqlc.narg(description), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: SetStaffPaymentFinanceEntry :one
UPDATE staff_payments
SET finance_entry_id = sqlc.arg(finance_entry_id)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
  AND finance_entry_id IS NULL
RETURNING *;

-- VoidStaffPayment marks a payment void after its ledger row was reversed;
-- a voided salary frees its period.
-- name: VoidStaffPayment :one
UPDATE staff_payments
SET voided_at = NOW()
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
  AND voided_at IS NULL
RETURNING *;

-- name: GetStaffPaymentByUUID :one
SELECT * FROM staff_payments
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: ListStaffPayments :many
SELECT * FROM staff_payments
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(staff_id)::bigint IS NULL OR staff_id = sqlc.narg(staff_id)::bigint)
  AND (sqlc.narg(period)::text IS NULL OR period = sqlc.narg(period)::text)
  AND (sqlc.narg(type)::varchar IS NULL OR type = sqlc.narg(type)::varchar)
  AND (sqlc.arg(include_voided)::boolean OR voided_at IS NULL)
ORDER BY paid_on DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountStaffPayments :one
SELECT COUNT(*) FROM staff_payments
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(staff_id)::bigint IS NULL OR staff_id = sqlc.narg(staff_id)::bigint)
  AND (sqlc.narg(period)::text IS NULL OR period = sqlc.narg(period)::text)
  AND (sqlc.narg(type)::varchar IS NULL OR type = sqlc.narg(type)::varchar)
  AND (sqlc.arg(include_voided)::boolean OR voided_at IS NULL);

-- Period summary: per staff and type, the paid total of a period (voided
-- payments excluded).
-- name: SumStaffPaymentsByPeriod :many
SELECT staff_id, type, currency, SUM(amount)::numeric(18,2) AS total, COUNT(*)::bigint AS payments
FROM staff_payments
WHERE organization_id = sqlc.arg(organization_id)
  AND period = sqlc.arg(period)
  AND voided_at IS NULL
GROUP BY staff_id, type, currency
ORDER BY staff_id, type;

-- ---------------------------------------------------------------------------
-- Per-service income.

-- name: SetServiceIncomeEntry :one
UPDATE services
SET income_entry_id = sqlc.narg(income_entry_id),
    income_amount = sqlc.narg(income_amount)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
RETURNING *;
