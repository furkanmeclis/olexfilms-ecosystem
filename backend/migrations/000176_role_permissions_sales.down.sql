-- TEC-1089: center_sales memberships fall back to center_staff (the default
-- center staff role) so no member is left without a role; the role and its
-- grants are removed (member rows cascade). The distributor grants and the
-- descriptions return to their pre-TEC-1089 state.
INSERT INTO organization_member_roles (member_id, role_id)
SELECT mr.member_id, staff.id
FROM organization_member_roles mr
JOIN roles sales ON sales.id = mr.role_id AND sales.slug = 'center_sales'
JOIN roles staff ON staff.slug = 'center_staff'
WHERE NOT EXISTS (
    SELECT 1 FROM organization_member_roles other
    WHERE other.member_id = mr.member_id AND other.role_id <> sales.id
)
ON CONFLICT DO NOTHING;

DELETE FROM roles WHERE slug = 'center_sales';

DELETE FROM role_permissions rp
USING roles r, permissions p
WHERE rp.role_id = r.id AND rp.permission_id = p.id
  AND ((r.slug = 'distributor_warehouse_staff' AND p.slug = 'organizations.read')
    OR (r.slug = 'distributor_accounting' AND p.slug IN ('einvoice.read', 'einvoice.manage')));

UPDATE permissions SET scopes = ARRAY['brand', 'all']
WHERE slug IN ('einvoice.read', 'einvoice.manage');

UPDATE roles SET description = 'Distributor warehouse operator'
WHERE slug = 'distributor_warehouse_staff';
UPDATE roles SET description = 'Dealer staff: reads the dealer''s services, writes own records; no prices or accounting'
WHERE slug = 'dealer_staff';
UPDATE roles SET description = 'Center service point staff: the center''s own services, customers, warranties and appointments; no warehouse, prices or accounting'
WHERE slug = 'center_service';
