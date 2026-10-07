-- TEC-468: helper lookups for public showcase lead / WhatsApp referral.

-- name: GetActiveDealerBySlug :one
SELECT o.id, o.uuid, o.slug, o.name, o.city, o.district, o.phone, o.address,
       o.status, o.type, o.parent_id, o.brand_id, o.currency, o.locale,
       o.timezone, o.country_id, o.province_id, o.district_id
FROM organizations o
WHERE o.slug = sqlc.arg(slug)
  AND o.brand_id = sqlc.arg(brand_id)
  AND o.deleted_at IS NULL
  AND o.status = 'active'
  AND o.type IN ('dealer', 'distributor')
  AND o.access_starts_at <= NOW()
  AND (o.access_ends_at IS NULL OR o.access_ends_at > NOW())
  AND (o.contract_valid_until IS NULL OR o.contract_valid_until >= CURRENT_DATE);
