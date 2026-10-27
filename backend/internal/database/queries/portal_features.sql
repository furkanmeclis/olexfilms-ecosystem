-- TEC-1023: the organizations whose module state decides what a portal
-- session (GET /v1/portal/features) may use.

-- name: ListPortalCustomerOrgIDs :many
-- The organizations a portal customer is linked to in a brand: the
-- customer_organizations rows plus the organizations of the services the
-- user owns or holds a warranty of (the GET /v1/portal/services rule).
SELECT co.organization_id::bigint AS organization_id
FROM customer_organizations co
WHERE co.user_id = sqlc.arg(user_id)::bigint
  AND co.brand_id = sqlc.arg(brand_id)::bigint
UNION
SELECT s.organization_id::bigint
FROM services s
WHERE (s.customer_user_id = sqlc.arg(user_id)::bigint
       OR EXISTS (SELECT 1 FROM warranties hw
                  WHERE hw.service_id = s.id AND hw.holder_user_id = sqlc.arg(user_id)::bigint))
  AND s.brand_id = sqlc.arg(brand_id)::bigint
  AND s.status <> 'draft'
ORDER BY 1;

-- name: ListPortalBookableDealerIDs :many
-- The dealers a portal customer may book at (GetPortalAppointmentDealer
-- rule without the module check, which the features service resolves).
SELECT o.id
FROM organizations o
JOIN appointment_settings s ON s.organization_id = o.id
WHERE o.brand_id = sqlc.arg(brand_id)::bigint
  AND o.deleted_at IS NULL
  AND o.status = 'active'
  AND o.type = 'dealer'
  AND o.access_starts_at <= NOW()
  AND (o.access_ends_at IS NULL OR o.access_ends_at > NOW())
  AND s.portal_appointments_enabled = TRUE
ORDER BY o.id;

-- name: ListPortalFleetActiveDealerIDs :many
-- The dealers actively linked to the fleet of a signed-in fleet user in a
-- brand (the fleet portal opens while one of them has the fleet module).
SELECT DISTINCT l.dealer_org_id
FROM fleet_users fu
JOIN organizations f ON f.id = fu.fleet_org_id AND f.deleted_at IS NULL
JOIN fleet_dealer_links l ON l.fleet_org_id = fu.fleet_org_id AND l.status = 'active'
WHERE fu.user_id = sqlc.arg(user_id)::bigint
  AND fu.status = 'active'
  AND f.brand_id = sqlc.arg(brand_id)::bigint
ORDER BY l.dealer_org_id;
