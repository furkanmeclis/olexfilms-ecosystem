-- TEC-405 (F4-04b): campaign audience resolution. One row per candidate
-- user of the validated audience filter; the use case applies the locale
-- filter (resolved locale), the marketing consent rule and the channel
-- reachability on these rows.
--
--   * reach_org_ids: organizations of the campaign reach (NULL = the whole
--     brand, center campaigns; distributor = its subtree; dealer = itself).
--     organization_ids narrows the reach further (filter "served by").
--   * customers: users linked to a reached organization in
--     customer_organizations; service based filters (last service date,
--     products, categories, warranties) look at reached organizations only.
--     Geography comes from customer_profiles.address ids (K29).
--   * dealer_users / distributor_users: members of reached dealer /
--     distributor organizations; geography comes from that organization.
--   * Geography and catalog ids are compared as text so a malformed address
--     value never fails the query.

-- name: ListCampaignAudience :many
WITH reach AS (
    SELECT o.id, o.type, o.locale, o.country_id, o.province_id, o.district_id
    FROM organizations o
    WHERE o.brand_id = sqlc.arg(brand_id)
      AND o.deleted_at IS NULL
      AND (sqlc.narg(reach_org_ids)::bigint[] IS NULL OR o.id = ANY (sqlc.narg(reach_org_ids)::bigint[]))
      AND (COALESCE(cardinality(sqlc.narg(organization_ids)::bigint[]), 0) = 0
           OR o.id = ANY (sqlc.narg(organization_ids)::bigint[]))
),
candidates AS (
    SELECT co.user_id,
           (SELECT r2.locale FROM customer_organizations co2 JOIN reach r2 ON r2.id = co2.organization_id
            WHERE co2.user_id = co.user_id
            ORDER BY co2.created_at DESC, co2.id DESC LIMIT 1)::text AS org_locale,
           (SELECT cp.address->>'country_id' FROM customer_profiles cp WHERE cp.user_id = co.user_id)::text AS country_key,
           (SELECT cp.address->>'province_id' FROM customer_profiles cp WHERE cp.user_id = co.user_id)::text AS province_key,
           (SELECT cp.address->>'district_id' FROM customer_profiles cp WHERE cp.user_id = co.user_id)::text AS district_key
    FROM customer_organizations co
    JOIN reach r ON r.id = co.organization_id
    WHERE sqlc.arg(audience_type)::text = 'customers'
      AND co.brand_id = sqlc.arg(brand_id)
    GROUP BY co.user_id
    UNION ALL
    SELECT m.user_id, m.locale::text, m.country_id::text, m.province_id::text, m.district_id::text
    FROM (
        SELECT DISTINCT ON (om.user_id) om.user_id, r.locale, r.country_id, r.province_id, r.district_id
        FROM organization_members om
        JOIN reach r ON r.id = om.organization_id
        WHERE (sqlc.arg(audience_type)::text = 'dealer_users' AND r.type = 'dealer')
           OR (sqlc.arg(audience_type)::text = 'distributor_users' AND r.type = 'distributor')
        ORDER BY om.user_id, r.id
    ) m
)
SELECT u.id AS user_id,
       u.uuid AS user_uuid,
       u.name,
       u.surname,
       COALESCE(u.email, '')::text AS email,
       COALESCE(u.phone_e164, '')::text AS phone_e164,
       COALESCE(NULLIF(u.locale, ''), NULLIF(c.org_locale, ''), (
           SELECT ce.locale FROM organizations ce
           WHERE ce.brand_id = sqlc.arg(brand_id) AND ce.type = 'center' AND ce.deleted_at IS NULL
           ORDER BY ce.id LIMIT 1), 'tr')::text AS locale,
       (SELECT COUNT(*) FROM device_push_tokens t WHERE t.user_id = u.id AND t.revoked_at IS NULL)::int AS push_tokens,
       COALESCE((SELECT NOT (cp.notification_prefs->'push' = 'false'::jsonb) FROM customer_profiles cp WHERE cp.user_id = u.id), true)::bool AS pref_push,
       COALESCE((SELECT NOT (cp.notification_prefs->'whatsapp' = 'false'::jsonb) FROM customer_profiles cp WHERE cp.user_id = u.id), true)::bool AS pref_whatsapp,
       COALESCE((SELECT NOT (cp.notification_prefs->'email' = 'false'::jsonb) FROM customer_profiles cp WHERE cp.user_id = u.id), true)::bool AS pref_email,
       COALESCE((SELECT mc.accepted FROM consents mc
                 WHERE mc.user_id = u.id AND mc.kind = 'marketing_consent'
                 ORDER BY mc.decided_at DESC, mc.id DESC LIMIT 1), false)::bool AS marketing_accepted,
       COALESCE((SELECT oo.opted_out FROM contact_opt_out_state oo
                 WHERE oo.contact_e164 = u.phone_e164 AND oo.scope = 'marketing'), false)::bool AS marketing_opted_out
FROM candidates c
JOIN users u ON u.id = c.user_id
WHERE u.deleted_at IS NULL
  AND u.status = 'active'
  AND u.merged_into_user_id IS NULL
  AND (COALESCE(cardinality(sqlc.narg(country_keys)::text[]), 0) = 0 OR c.country_key = ANY (sqlc.narg(country_keys)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(province_keys)::text[]), 0) = 0 OR c.province_key = ANY (sqlc.narg(province_keys)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(district_keys)::text[]), 0) = 0 OR c.district_key = ANY (sqlc.narg(district_keys)::text[]))
  AND (
      (sqlc.narg(last_service_from)::timestamptz IS NULL AND sqlc.narg(last_service_before)::timestamptz IS NULL)
      OR EXISTS (
          SELECT 1 FROM (
              SELECT MAX(s.completed_at) AS last_at
              FROM services s JOIN reach r ON r.id = s.organization_id
              WHERE s.customer_user_id = u.id AND s.status = 'completed'
          ) ls
          WHERE ls.last_at IS NOT NULL
            AND (sqlc.narg(last_service_from)::timestamptz IS NULL OR ls.last_at >= sqlc.narg(last_service_from)::timestamptz)
            AND (sqlc.narg(last_service_before)::timestamptz IS NULL OR ls.last_at < sqlc.narg(last_service_before)::timestamptz)
      )
  )
  AND (COALESCE(cardinality(sqlc.narg(car_brand_uuids)::uuid[]), 0) = 0 OR EXISTS (
      SELECT 1 FROM vehicles v JOIN car_brands cb ON cb.id = v.car_brand_id
      WHERE v.user_id = u.id AND v.deleted_at IS NULL AND v.brand_id = sqlc.arg(brand_id)
        AND cb.uuid = ANY (sqlc.narg(car_brand_uuids)::uuid[])))
  AND (COALESCE(cardinality(sqlc.narg(product_uuids)::uuid[]), 0) = 0 OR EXISTS (
      SELECT 1 FROM service_items si
      JOIN services s ON s.id = si.service_id
      JOIN reach r ON r.id = s.organization_id
      JOIN products p ON p.id = si.product_id
      WHERE s.customer_user_id = u.id AND s.status = 'completed'
        AND p.uuid = ANY (sqlc.narg(product_uuids)::uuid[])))
  AND (COALESCE(cardinality(sqlc.narg(category_uuids)::uuid[]), 0) = 0 OR EXISTS (
      SELECT 1 FROM service_items si
      JOIN services s ON s.id = si.service_id
      JOIN reach r ON r.id = s.organization_id
      JOIN products p ON p.id = si.product_id
      JOIN product_categories pc ON pc.id = p.category_id
      WHERE s.customer_user_id = u.id AND s.status = 'completed'
        AND pc.uuid = ANY (sqlc.narg(category_uuids)::uuid[])))
  AND (COALESCE(cardinality(sqlc.narg(warranty_statuses)::text[]), 0) = 0 OR EXISTS (
      SELECT 1 FROM warranties w JOIN reach r ON r.id = w.organization_id
      WHERE w.holder_user_id = u.id AND w.status = 'active' AND w.end_at > sqlc.arg(now)::timestamptz
        AND ('active' = ANY (sqlc.narg(warranty_statuses)::text[])
             OR ('expiring' = ANY (sqlc.narg(warranty_statuses)::text[])
                 AND w.end_at <= sqlc.arg(now)::timestamptz + INTERVAL '30 days')))
      OR ('expired' = ANY (sqlc.narg(warranty_statuses)::text[])
          AND EXISTS (
              SELECT 1 FROM warranties w JOIN reach r ON r.id = w.organization_id
              WHERE w.holder_user_id = u.id
                AND (w.status = 'expired' OR (w.status = 'active' AND w.end_at <= sqlc.arg(now)::timestamptz)))
          AND NOT EXISTS (
              SELECT 1 FROM warranties w JOIN reach r ON r.id = w.organization_id
              WHERE w.holder_user_id = u.id AND w.status = 'active' AND w.end_at > sqlc.arg(now)::timestamptz)))
ORDER BY u.id;

-- name: ResolveCampaignOrganizations :many
-- Organizations of the brand with the given uuids (filter "served by").
SELECT id, uuid, type FROM organizations
WHERE brand_id = sqlc.arg(brand_id) AND deleted_at IS NULL AND uuid = ANY (sqlc.arg(uuids)::uuid[]);

-- name: EnsureCampaignContent :one
-- The content of a locale, created empty when missing (media upload).
INSERT INTO campaign_contents (campaign_id, locale)
VALUES (sqlc.arg(campaign_id), sqlc.arg(locale))
ON CONFLICT (campaign_id, locale) DO UPDATE SET locale = campaign_contents.locale
RETURNING *;

-- name: CountCampaignContentMedia :one
SELECT COUNT(*) FROM campaign_media WHERE content_id = sqlc.arg(content_id);
