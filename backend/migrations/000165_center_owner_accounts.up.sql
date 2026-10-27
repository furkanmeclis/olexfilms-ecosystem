-- Demo flow (TEC-1024/1028 follow-up): the center owner opens the center's
-- accounts itself. members.write (brand) adds staff to the center with
-- their warehouses (POST /v1/tenant/members, warehouse member routes);
-- organizations.write (brand) opens a dealer with its owner directly under
-- the center (POST /v1/tenant/dealers). Only center_owner gets them; the Go
-- catalog adds them on top of the center role union (CenterOwnerExtras).
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'brand'
FROM roles r
JOIN permissions p ON p.slug IN ('members.write', 'organizations.write')
WHERE r.slug = 'center_owner'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
