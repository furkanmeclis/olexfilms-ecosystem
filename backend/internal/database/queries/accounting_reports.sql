-- TEC-175 (F1-07e): cari statement and balance report. Read only; the caller
-- resolves the book (one organization inside the request scope) and passes
-- its id. Signs follow 000047: cari balance = income + charge + payment -
-- expense - collection (receivable positive); cash/bank balance = income +
-- collection + opening - expense - payment (opening: TEC-198, 000056).
-- Reversal rows carry negated amounts, so a plain sum nets them out.

-- GetCariStatementOpening is the cari balance before created_before (the
-- opening balance of a statement period).
-- name: GetCariStatementOpening :one
SELECT COALESCE(SUM(CASE e.direction
                        WHEN 'income' THEN e.amount
                        WHEN 'charge' THEN e.amount
                        WHEN 'payment' THEN e.amount
                        WHEN 'expense' THEN -e.amount
                        WHEN 'collection' THEN -e.amount
                    END), 0)::NUMERIC(18,2) AS balance
FROM finance_entries e
WHERE e.cari_id = sqlc.arg(cari_id)
  AND e.organization_id = sqlc.arg(organization_id)
  AND e.created_at < sqlc.arg(created_before)::timestamptz;

-- ListCariStatementLines returns the period rows of a cari in ledger order
-- with their signed cari effect (signed_amount).
-- name: ListCariStatementLines :many
SELECT e.id, e.uuid, e.created_at, e.direction, e.category, e.description,
       e.source_type, e.source_uuid, e.orig_currency, e.orig_amount, e.currency,
       (CASE e.direction
            WHEN 'income' THEN e.amount
            WHEN 'charge' THEN e.amount
            WHEN 'payment' THEN e.amount
            WHEN 'expense' THEN -e.amount
            WHEN 'collection' THEN -e.amount
        END)::NUMERIC(18,2) AS signed_amount,
       ro.uuid AS reversal_of_uuid,
       (rv.id IS NOT NULL)::boolean AS reversed
FROM finance_entries e
LEFT JOIN finance_entries ro ON ro.id = e.reversal_of_id
LEFT JOIN finance_entries rv ON rv.reversal_of_id = e.id
WHERE e.cari_id = sqlc.arg(cari_id)
  AND e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR e.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR e.created_at < sqlc.narg(created_to)::timestamptz)
ORDER BY e.created_at, e.id;

-- ListCariBalancesAsOf is every cari of the book with its balance over the
-- rows written before created_to (NULL = all rows).
-- name: ListCariBalancesAsOf :many
SELECT c.uuid, c.counterparty_type, c.currency, c.active,
       o.uuid AS counterparty_org_uuid, o.name AS counterparty_org_name, o.type AS counterparty_org_type,
       u.uuid AS counterparty_user_uuid,
       COALESCE(NULLIF(btrim(concat_ws(' ', u.name, u.surname)), ''), '')::text AS counterparty_user_name,
       COALESCE(SUM(CASE e.direction
                        WHEN 'income' THEN e.amount
                        WHEN 'charge' THEN e.amount
                        WHEN 'payment' THEN e.amount
                        WHEN 'expense' THEN -e.amount
                        WHEN 'collection' THEN -e.amount
                    END), 0)::NUMERIC(18,2) AS balance,
       COUNT(e.id) AS entry_count
FROM cari_accounts c
LEFT JOIN finance_entries e
       ON e.cari_id = c.id
      AND (sqlc.narg(created_to)::timestamptz IS NULL OR e.created_at < sqlc.narg(created_to)::timestamptz)
LEFT JOIN organizations o ON o.id = c.counterparty_org_id
LEFT JOIN users u ON u.id = c.counterparty_user_id
WHERE c.organization_id = sqlc.arg(organization_id)
GROUP BY c.id, o.id, u.id
ORDER BY COALESCE(o.name, u.name, ''), c.id;

-- ListFinanceAccountBalancesAsOf is every cash/bank account of the book with
-- its balance over the rows written before created_to (NULL = all rows).
-- name: ListFinanceAccountBalancesAsOf :many
SELECT a.uuid, a.type, a.name, a.currency, a.active,
       COALESCE(SUM(CASE e.direction
                        WHEN 'income' THEN e.amount
                        WHEN 'collection' THEN e.amount
                        WHEN 'opening' THEN e.amount
                        WHEN 'expense' THEN -e.amount
                        WHEN 'payment' THEN -e.amount
                    END), 0)::NUMERIC(18,2) AS balance,
       COUNT(e.id) AS entry_count
FROM finance_accounts a
LEFT JOIN finance_entries e
       ON e.account_id = a.id
      AND (sqlc.narg(created_to)::timestamptz IS NULL OR e.created_at < sqlc.narg(created_to)::timestamptz)
WHERE a.organization_id = sqlc.arg(organization_id)
GROUP BY a.id
ORDER BY a.type, a.name, a.id;

-- TEC-346 (F3-07f) reports below read one book (the caller's own
-- organization); periods are half-open UTC ranges.
-- ListPnlSums is the income/expense total of the book per UTC month,
-- category and direction. Reversal rows carry negated amounts, so a voided
-- row nets out.
-- name: ListPnlSums :many
SELECT to_char(e.created_at AT TIME ZONE 'UTC', 'YYYY-MM')::text AS month,
       e.category, e.direction,
       SUM(e.amount)::NUMERIC(18,2) AS total,
       COUNT(e.id) AS entry_count
FROM finance_entries e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.direction IN ('income', 'expense')
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR e.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR e.created_at < sqlc.narg(created_to)::timestamptz)
GROUP BY 1, e.category, e.direction
ORDER BY 1, e.direction, e.category;

-- ListMarginProductSales is the product sale margin per product: revenue
-- and the purchase cost snapshot of the lines (F3-07d). A voided sale (its
-- income row reversed) is left out; lines without a cost snapshot are
-- counted so the report can flag an incomplete cost.
-- name: ListMarginProductSales :many
SELECT p.uuid AS product_uuid, p.sku, p.name,
       SUM(l.quantity)::NUMERIC(18,2) AS quantity,
       SUM(l.line_total)::NUMERIC(18,2) AS revenue,
       COALESCE(SUM(l.quantity * l.purchase_unit_cost), 0)::NUMERIC(18,2) AS cost,
       COUNT(l.id) FILTER (WHERE l.purchase_unit_cost IS NULL) AS lines_without_cost,
       COUNT(DISTINCT s.id) AS sale_count
FROM product_sale_lines l
JOIN product_sales s ON s.id = l.sale_id AND s.organization_id = l.organization_id
JOIN products p ON p.id = l.product_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(sold_from)::timestamptz IS NULL OR s.sold_at >= sqlc.narg(sold_from)::timestamptz)
  AND (sqlc.narg(sold_to)::timestamptz IS NULL OR s.sold_at < sqlc.narg(sold_to)::timestamptz)
  AND NOT EXISTS (SELECT 1 FROM finance_entries rv
                  WHERE s.finance_entry_id IS NOT NULL AND rv.reversal_of_id = s.finance_entry_id)
GROUP BY p.id
ORDER BY SUM(l.line_total) DESC, p.name, p.id;

-- GetMarginServiceSummary is the service margin of the book: services whose
-- income (F3-07c, services.income_amount) was recorded in the period, and
-- the purchase cost of the units they consumed (same pricing as
-- GetServiceConsumedPurchaseCost: the last received order line of the unit).
-- name: GetMarginServiceSummary :one
WITH svc AS (
    SELECT s.id, s.organization_id, s.income_amount
    FROM services s
    JOIN finance_entries fe ON fe.id = s.income_entry_id
    WHERE s.organization_id = sqlc.arg(organization_id)
      AND s.income_entry_id IS NOT NULL
      AND (sqlc.narg(created_from)::timestamptz IS NULL OR fe.created_at >= sqlc.narg(created_from)::timestamptz)
      AND (sqlc.narg(created_to)::timestamptz IS NULL OR fe.created_at < sqlc.narg(created_to)::timestamptz)
), consumed AS (
    SELECT si.service_id, SUM(
        CASE
          WHEN si.meters IS NOT NULL THEN si.meters * bought.unit_price
          WHEN si.quantity IS NOT NULL THEN si.quantity * bought.unit_price
          WHEN bought.meters IS NOT NULL THEN bought.meters * bought.unit_price
          WHEN bought.quantity IS NOT NULL THEN bought.quantity * bought.unit_price
          ELSE 0
        END) AS cost
    FROM svc
    JOIN service_items si ON si.service_id = svc.id
    LEFT JOIN LATERAL (
        SELECT oi.unit_price, oiu.quantity, oiu.meters
        FROM order_item_units oiu
        JOIN order_items oi ON oi.id = oiu.order_item_id
        JOIN orders o ON o.id = oi.order_id
        WHERE oiu.unit_id = si.unit_id
          AND o.buyer_org_id = svc.organization_id
          AND o.status = 'received'
        ORDER BY o.id DESC, oi.id DESC, oiu.id DESC
        LIMIT 1
    ) bought ON TRUE
    GROUP BY si.service_id
)
SELECT COUNT(svc.id) AS service_count,
       COALESCE(SUM(svc.income_amount), 0)::NUMERIC(18,2) AS revenue,
       COALESCE(SUM(consumed.cost), 0)::NUMERIC(18,2) AS cost
FROM svc
LEFT JOIN consumed ON consumed.service_id = svc.id;

-- ListCariAgingLines is every cari row of the book written before
-- created_to (NULL = all), newest first per cari, with its signed cari
-- effect (receivable positive, as in ListCariStatementLines).
-- name: ListCariAgingLines :many
SELECT c.uuid AS cari_uuid, e.created_at,
       (CASE e.direction
            WHEN 'income' THEN e.amount
            WHEN 'charge' THEN e.amount
            WHEN 'payment' THEN e.amount
            WHEN 'expense' THEN -e.amount
            WHEN 'collection' THEN -e.amount
        END)::NUMERIC(18,2) AS signed_amount
FROM finance_entries e
JOIN cari_accounts c ON c.id = e.cari_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR e.created_at < sqlc.narg(created_to)::timestamptz)
ORDER BY c.id, e.created_at DESC, e.id DESC;

-- ListStaffCostTotals is the salary/advance/bonus total per staff card of
-- the book over the non-void booked payments paid in the period (paid_on,
-- both days inclusive). Planned payments are not costs yet (TEC-381).
-- name: ListStaffCostTotals :many
SELECT sp.uuid, sp.name, sp.title, sp.active,
       COALESCE(SUM(p.amount) FILTER (WHERE p.type = 'salary'), 0)::NUMERIC(18,2) AS salary,
       COALESCE(SUM(p.amount) FILTER (WHERE p.type = 'advance'), 0)::NUMERIC(18,2) AS advance,
       COALESCE(SUM(p.amount) FILTER (WHERE p.type = 'bonus'), 0)::NUMERIC(18,2) AS bonus,
       COALESCE(SUM(p.amount), 0)::NUMERIC(18,2) AS total,
       COUNT(p.id) AS payment_count
FROM staff_profiles sp
JOIN staff_payments p ON p.staff_id = sp.id
                     AND p.organization_id = sp.organization_id
                     AND p.voided_at IS NULL
                     AND p.status = 'posted'
                     AND (sqlc.narg(paid_from)::date IS NULL OR p.paid_on >= sqlc.narg(paid_from)::date)
                     AND (sqlc.narg(paid_to)::date IS NULL OR p.paid_on <= sqlc.narg(paid_to)::date)
WHERE sp.organization_id = sqlc.arg(organization_id)
GROUP BY sp.id
ORDER BY sp.name, sp.id;
