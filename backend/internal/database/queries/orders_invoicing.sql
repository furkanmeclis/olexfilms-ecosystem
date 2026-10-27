-- TEC-1030: manual "invoiced" marks of received orders (seller accounting).

-- name: CreateOrderInvoice :one
INSERT INTO order_invoices (order_id, organization_id, brand_id, invoice_no, invoice_date, note, created_by)
VALUES (sqlc.arg(order_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(invoice_no),
        sqlc.arg(invoice_date), sqlc.narg(note), sqlc.narg(created_by))
RETURNING *;

-- name: CreateOrderInvoiceItem :exec
INSERT INTO order_invoice_items (order_invoice_id, order_item_id)
VALUES (sqlc.arg(order_invoice_id), sqlc.arg(order_item_id));

-- name: GetOrderInvoiceByUUID :one
SELECT * FROM order_invoices
WHERE uuid = sqlc.arg(uuid) AND order_id = sqlc.arg(order_id);

-- name: DeleteOrderInvoice :execrows
DELETE FROM order_invoices WHERE id = sqlc.arg(id);

-- name: ListOrderInvoices :many
SELECT i.*, u.name AS created_by_name, u.surname AS created_by_surname
FROM order_invoices i
LEFT JOIN users u ON u.id = i.created_by
WHERE i.order_id = sqlc.arg(order_id)
ORDER BY i.id;

-- name: ListOrderInvoiceItemsByOrder :many
SELECT ii.order_invoice_id, ii.order_item_id, i.uuid AS invoice_uuid, i.invoice_no
FROM order_invoice_items ii
JOIN order_invoices i ON i.id = ii.order_invoice_id
WHERE i.order_id = sqlc.arg(order_id)
ORDER BY ii.order_item_id;

-- name: CountInvoicedOrderItems :one
SELECT COUNT(*) FROM order_invoice_items ii
JOIN order_items oi ON oi.id = ii.order_item_id
WHERE oi.order_id = sqlc.arg(order_id);

-- name: CountOrderItems :one
SELECT COUNT(*) FROM order_items WHERE order_id = sqlc.arg(order_id);

-- name: SetOrderInvoiceStatus :one
UPDATE orders SET invoice_status = sqlc.arg(invoice_status)
WHERE id = sqlc.arg(id)
RETURNING *;
