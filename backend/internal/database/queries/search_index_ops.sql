-- TEC-210: organizations (name, dealer code = slug), orders (order number,
-- parties) and stock units (barcode, product, status, location) indexes.
-- Same contract as search_index.sql (TEC-209): the list endpoints filter
-- the index on the caller's scope and reload the hits from Postgres with
-- that scope, so the index is never the only access check.

-- name: ListOrganizationsForIndex :many
SELECT o.uuid, o.id, o.slug, o.name, o.type, o.status, o.brand_id, o.city, o.district, o.phone,
       p.name AS parent_name,
       -- TEC-467: a published dealer showcase (the approved snapshot).
       EXISTS (SELECT 1 FROM dealer_showcases s
               WHERE s.organization_id = o.id AND s.published_content IS NOT NULL)::boolean AS has_showcase
FROM organizations o
LEFT JOIN organizations p ON p.id = o.parent_id
WHERE o.deleted_at IS NULL AND o.type <> 'fleet'
ORDER BY o.id;

-- name: GetOrganizationForIndex :one
SELECT o.uuid, o.id, o.slug, o.name, o.type, o.status, o.brand_id, o.city, o.district, o.phone,
       p.name AS parent_name,
       -- TEC-467: a published dealer showcase (the approved snapshot).
       EXISTS (SELECT 1 FROM dealer_showcases s
               WHERE s.organization_id = o.id AND s.published_content IS NOT NULL)::boolean AS has_showcase
FROM organizations o
LEFT JOIN organizations p ON p.id = o.parent_id
-- TEC-472: fleets are not indexed (a stale document is removed).
WHERE o.uuid = sqlc.arg(uuid) AND o.deleted_at IS NULL AND o.type <> 'fleet';

-- name: ListOrdersForIndex :many
SELECT o.uuid, o.order_no, o.organization_id, o.buyer_org_id, o.brand_id, o.status,
       o.tracking_no, o.external_reference,
       so.name AS seller_name, so.slug AS seller_slug,
       bo.name AS buyer_name, bo.slug AS buyer_slug
FROM orders o
JOIN organizations so ON so.id = o.organization_id
JOIN organizations bo ON bo.id = o.buyer_org_id
ORDER BY o.id;

-- name: GetOrderForIndex :one
SELECT o.uuid, o.order_no, o.organization_id, o.buyer_org_id, o.brand_id, o.status,
       o.tracking_no, o.external_reference,
       so.name AS seller_name, so.slug AS seller_slug,
       bo.name AS buyer_name, bo.slug AS buyer_slug
FROM orders o
JOIN organizations so ON so.id = o.organization_id
JOIN organizations bo ON bo.id = o.buyer_org_id
WHERE o.uuid = sqlc.arg(uuid);

-- Orders an organization sells or buys: their documents carry the
-- organization's name and dealer code, refreshed when it changes.
-- name: ListOrderUuidsByOrganization :many
SELECT o.uuid FROM orders o
WHERE o.organization_id = sqlc.arg(organization_id)::bigint
   OR o.buyer_org_id = sqlc.arg(organization_id)::bigint
ORDER BY o.id;

-- Stock units: holder_org_ids are the organizations whose unit list
-- (GET /v1/stock/organizations/{uuid}/units) shows the unit (serial
-- current state, fixed barcode holdings with quantity on hand) and
-- location_codes the bins it sits in (full_code, else code).

-- name: ListStockUnitsForIndex :many
SELECT u.uuid, u.barcode, u.brand_id, u.status, u.product_id,
       p.sku, p.name AS product_name,
       ARRAY(
         SELECT s.holder_org_id FROM unit_current_state s
         WHERE s.unit_id = u.id AND s.owner_type IN ('organization', 'warehouse_location')
         UNION
         SELECT h.holder_org_id FROM fixed_barcode_holdings h
         WHERE h.unit_id = u.id AND h.owner_type IN ('organization', 'warehouse_location')
         GROUP BY h.holder_org_id HAVING SUM(h.quantity_on_hand) > 0
       )::bigint[] AS holder_org_ids,
       ARRAY(
         SELECT COALESCE(l.full_code, l.code)::text FROM warehouse_locations l
         WHERE l.id IN (
           SELECT s.owner_id FROM unit_current_state s
           WHERE s.unit_id = u.id AND s.owner_type = 'warehouse_location'
           UNION
           SELECT h.owner_id FROM fixed_barcode_holdings h
           WHERE h.unit_id = u.id AND h.owner_type = 'warehouse_location' AND h.quantity_on_hand > 0
         )
       )::text[] AS location_codes
FROM units u
JOIN products p ON p.id = u.product_id
ORDER BY u.id;

-- name: GetStockUnitForIndex :one
SELECT u.uuid, u.barcode, u.brand_id, u.status, u.product_id,
       p.sku, p.name AS product_name,
       ARRAY(
         SELECT s.holder_org_id FROM unit_current_state s
         WHERE s.unit_id = u.id AND s.owner_type IN ('organization', 'warehouse_location')
         UNION
         SELECT h.holder_org_id FROM fixed_barcode_holdings h
         WHERE h.unit_id = u.id AND h.owner_type IN ('organization', 'warehouse_location')
         GROUP BY h.holder_org_id HAVING SUM(h.quantity_on_hand) > 0
       )::bigint[] AS holder_org_ids,
       ARRAY(
         SELECT COALESCE(l.full_code, l.code)::text FROM warehouse_locations l
         WHERE l.id IN (
           SELECT s.owner_id FROM unit_current_state s
           WHERE s.unit_id = u.id AND s.owner_type = 'warehouse_location'
           UNION
           SELECT h.owner_id FROM fixed_barcode_holdings h
           WHERE h.unit_id = u.id AND h.owner_type = 'warehouse_location' AND h.quantity_on_hand > 0
         )
       )::text[] AS location_codes
FROM units u
JOIN products p ON p.id = u.product_id
WHERE u.uuid = sqlc.arg(uuid);
