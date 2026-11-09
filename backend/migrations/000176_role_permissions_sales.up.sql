-- TEC-1089 (revize 09.11): role permission decisions.
--  1. center_sales: the center's sales representative. Orders, catalog and
--     dealers of center_staff, orders on behalf of dealers (write, approve,
--     cancel), sale prices and
--     discounts (pricing.sale.write, TEC-1064); no purchase prices (no
--     margin), no warehouse, shipping or accounting writes. center_staff is
--     not changed. The role is a subset of center_staff ∪ center_accounting,
--     so the center_owner union (TEC-1026) is unchanged.
--  2. distributor_warehouse_staff: organizations.read (subtree) for the
--     read-only dealers page (TEC-1066).
--  3. distributor_accounting: einvoice.read and einvoice.manage (managed)
--     for the manual "invoiced" mark of its own sales (TEC-1030, TEC-1070);
--     both permissions now allow the managed scope. The e-invoice module stays center-only.
--  4. dealer_staff and center_service see no prices: only the descriptions
--     say so explicitly.
-- Effective permissions are computed from organization_member_roles ->
-- role_permissions at request time, so existing memberships need no
-- projection update. The Go catalog (internal/platform/rbac) holds the same
-- packages (rbacsync TestMigrationMatchesCatalog).

INSERT INTO roles (name, slug, description, is_system, org_type) VALUES
    ('Center sales representative', 'center_sales',
     'Center sales representative: dealer orders, sale prices and discounts, dealers, services and customers (read) and reports; no purchase prices, warehouse or accounting',
     TRUE, 'center')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('auth.session', 'own'),
    ('notifications.read', 'own'),
    ('notifications.manage', 'own'),
    ('vehicle_catalog.read', 'brand'),
    ('catalog.read', 'brand'),
    ('organizations.read', 'brand'),
    ('orders.read', 'brand'),
    ('orders.write', 'brand'),
    ('orders.approve', 'brand'),
    ('orders.cancel', 'brand'),
    ('pricing.sale.read', 'brand'),
    ('pricing.sale.write', 'brand'),
    ('pricing.recommended.read', 'brand'),
    ('services.read', 'brand'),
    ('customers.read', 'brand'),
    ('vehicles.read', 'brand'),
    ('warranties.read', 'brand'),
    ('performance.read', 'brand'),
    ('pricing.discipline.read', 'brand'),
    ('tasks.read', 'brand'),
    ('tasks.write', 'brand'),
    ('announcements.read', 'managed'),
    ('library.read', 'managed'),
    ('ai.use', 'own'),
    ('ai.actions.confirm', 'own'),
    ('mcp.connect', 'own')
) AS g(perm, scope)
JOIN permissions p ON p.slug = g.perm
JOIN roles r ON r.slug = 'center_sales'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'subtree'
FROM roles r JOIN permissions p ON p.slug = 'organizations.read'
WHERE r.slug = 'distributor_warehouse_staff'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

UPDATE permissions SET scopes = ARRAY['managed', 'brand', 'all']
WHERE slug IN ('einvoice.read', 'einvoice.manage');

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'managed'
FROM roles r JOIN permissions p ON p.slug IN ('einvoice.read', 'einvoice.manage')
WHERE r.slug = 'distributor_accounting'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

UPDATE roles SET description = 'Distributor warehouse operator; reads the dealers, sees no prices'
WHERE slug = 'distributor_warehouse_staff';
UPDATE roles SET description = 'Dealer staff: reads the dealer''s services, writes own records; sees no prices (purchase or sale) and no accounting'
WHERE slug = 'dealer_staff';
UPDATE roles SET description = 'Center service point staff: the center''s own services, customers, warranties and appointments; sees no prices (purchase or sale), no warehouse or accounting'
WHERE slug = 'center_service';
