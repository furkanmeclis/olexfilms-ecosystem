-- TEC-505 (F5-09a): recommended sale price versions, the current projection
-- and price discipline snapshots. Versions are append-only (the database
-- refuses any UPDATE other than setting superseded_at once, and direct
-- DELETEs); the projection is maintained through
-- recommended_prices_make_current(). country_id NULL is the currency-wide
-- price; list filters take country id 0 for it. Sort keys come from
-- pricing/repository (docs/list-contract.md); id tiebreak.

-- Versions ----------------------------------------------------------------------

-- name: SupersedeLiveRecommendedPriceVersionsOn :execrows
-- Before a republish for the same key and day (one live version per day).
UPDATE recommended_price_versions
SET superseded_at = GREATEST(NOW(), published_at)
WHERE product_id = sqlc.arg(product_id)
  AND country_id IS NOT DISTINCT FROM sqlc.narg(country_id)::bigint
  AND currency = sqlc.arg(currency)
  AND effective_from = sqlc.arg(effective_from)
  AND superseded_at IS NULL;

-- name: InsertRecommendedPriceVersion :one
INSERT INTO recommended_price_versions (
    organization_id, brand_id, product_id, country_id, currency, price, effective_from,
    source, published_by_user_id, batch_id, note
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(product_id), sqlc.narg(country_id),
    sqlc.arg(currency), sqlc.arg(price), sqlc.arg(effective_from),
    sqlc.arg(source), sqlc.narg(published_by_user_id), sqlc.narg(batch_id), sqlc.arg(note)
)
RETURNING *;

-- name: GetRecommendedPriceVersion :one
SELECT * FROM recommended_price_versions
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: SupersedeRecommendedPriceVersion :execrows
-- Cancels a scheduled (not yet current) version; the only UPDATE a version
-- accepts.
UPDATE recommended_price_versions v
SET superseded_at = GREATEST(NOW(), v.published_at)
WHERE v.id = sqlc.arg(id) AND v.brand_id = sqlc.arg(brand_id)
  AND v.superseded_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM recommended_prices_current c WHERE c.version_id = v.id);

-- name: MakeRecommendedPriceCurrent :exec
-- Supersedes the older live versions of the key and points the projection
-- (and, for country NULL, product_prices.recommended_sale_price) at it.
SELECT recommended_prices_make_current(sqlc.arg(version_id)::bigint);

-- name: ListDueRecommendedPriceVersions :many
-- Live versions whose day has come and that are newer than the one in
-- force, the latest per key (input of the effective-date job).
SELECT DISTINCT ON (v.product_id, v.country_id, v.currency) v.*
FROM recommended_price_versions v
LEFT JOIN recommended_prices_current c
       ON c.product_id = v.product_id
      AND c.country_id IS NOT DISTINCT FROM v.country_id
      AND c.currency = v.currency
WHERE v.superseded_at IS NULL
  AND v.effective_from <= sqlc.arg(as_of)::date
  AND (c.id IS NULL OR (c.version_id <> v.id AND v.effective_from >= c.effective_from))
ORDER BY v.product_id, v.country_id, v.currency, v.effective_from DESC, v.id DESC
LIMIT sqlc.arg(row_limit);

-- name: ListRecommendedPriceVersions :many
-- Version history. Sort keys: effective_from | published_at | price.
SELECT v.*,
       p.name AS product_name,
       p.sku AS product_sku,
       COALESCE(co.iso2, '')::text AS country_iso2,
       COALESCE(co.name_en, '')::text AS country_name_en,
       COALESCE(co.name_tr, '')::text AS country_name_tr,
       TRIM(COALESCE(u.name, '') || ' ' || COALESCE(u.surname, ''))::text AS published_by_name,
       (c.id IS NOT NULL)::bool AS is_current,
       COUNT(*) OVER()::bigint AS total_count
FROM recommended_price_versions v
JOIN products p ON p.id = v.product_id
LEFT JOIN countries co ON co.id = v.country_id
LEFT JOIN users u ON u.id = v.published_by_user_id
LEFT JOIN recommended_prices_current c ON c.version_id = v.id
WHERE v.brand_id = sqlc.arg(brand_id)
  AND (COALESCE(cardinality(sqlc.narg(product_ids)::bigint[]), 0) = 0
       OR v.product_id = ANY (sqlc.narg(product_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(country_ids)::bigint[]), 0) = 0
       OR COALESCE(v.country_id, 0) = ANY (sqlc.narg(country_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(currencies)::text[]), 0) = 0
       OR v.currency = ANY (sqlc.narg(currencies)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(sources)::text[]), 0) = 0
       OR v.source = ANY (sqlc.narg(sources)::text[]))
  AND (sqlc.narg(batch_id)::uuid IS NULL OR v.batch_id = sqlc.narg(batch_id)::uuid)
  AND (sqlc.narg(effective_from_from)::date IS NULL OR v.effective_from >= sqlc.narg(effective_from_from)::date)
  AND (sqlc.narg(effective_from_before)::date IS NULL OR v.effective_from < sqlc.narg(effective_from_before)::date)
  AND (sqlc.narg(q)::text IS NULL
       OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'effective_from' THEN v.effective_from END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'effective_from' THEN v.effective_from END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'published_at' THEN v.published_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'published_at' THEN v.published_at END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'price' THEN v.price END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'price' THEN v.price END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN v.id END DESC,
  v.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- Current projection -----------------------------------------------------------

-- name: GetApplicableRecommendedPrice :one
-- The price in force for a product in a country and currency: the country
-- row when there is one, else the currency-wide row.
SELECT * FROM recommended_prices_current
WHERE product_id = sqlc.arg(product_id)
  AND currency = sqlc.arg(currency)
  AND (country_id IS NULL OR country_id = sqlc.narg(country_id)::bigint)
ORDER BY country_id NULLS LAST
LIMIT 1;

-- name: WithdrawRecommendedPrice :execrows
-- Removes a recommendation from the projection and supersedes its version;
-- for country NULL product_prices.recommended_sale_price becomes NULL.
WITH removed AS (
    DELETE FROM recommended_prices_current rc
    WHERE rc.brand_id = sqlc.arg(brand_id)
      AND rc.product_id = sqlc.arg(product_id)
      AND rc.country_id IS NOT DISTINCT FROM sqlc.narg(country_id)::bigint
      AND rc.currency = sqlc.arg(currency)
    RETURNING rc.version_id
)
UPDATE recommended_price_versions v
SET superseded_at = GREATEST(NOW(), v.published_at)
FROM removed
WHERE v.id = removed.version_id AND v.superseded_at IS NULL;

-- name: ListRecommendedPricesCurrent :many
-- Current list. Sort keys: product_name | price | effective_from.
SELECT c.*,
       p.name AS product_name,
       p.sku AS product_sku,
       COALESCE(co.iso2, '')::text AS country_iso2,
       COALESCE(co.name_en, '')::text AS country_name_en,
       COALESCE(co.name_tr, '')::text AS country_name_tr,
       COUNT(*) OVER()::bigint AS total_count
FROM recommended_prices_current c
JOIN products p ON p.id = c.product_id
LEFT JOIN countries co ON co.id = c.country_id
WHERE c.brand_id = sqlc.arg(brand_id)
  AND (COALESCE(cardinality(sqlc.narg(product_ids)::bigint[]), 0) = 0
       OR c.product_id = ANY (sqlc.narg(product_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(country_ids)::bigint[]), 0) = 0
       OR COALESCE(c.country_id, 0) = ANY (sqlc.narg(country_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(currencies)::text[]), 0) = 0
       OR c.currency = ANY (sqlc.narg(currencies)::text[]))
  AND (sqlc.narg(q)::text IS NULL
       OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'product_name' THEN p.name END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'product_name' THEN p.name END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'price' THEN c.price END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'price' THEN c.price END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'effective_from' THEN c.effective_from END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'effective_from' THEN c.effective_from END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN c.id END DESC,
  c.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- Price discipline snapshots ----------------------------------------------------

-- name: UpsertPriceDisciplineSnapshot :one
INSERT INTO price_discipline_snapshots (
    snapshot_date, organization_id, brand_id, product_id, country_id, currency,
    recommended_version_id, recommended_price, list_price, deviation_pct,
    avg_sale_price, sales_quantity
)
VALUES (
    sqlc.arg(snapshot_date), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(product_id),
    sqlc.narg(country_id), sqlc.arg(currency), sqlc.narg(recommended_version_id),
    sqlc.arg(recommended_price), sqlc.arg(list_price), sqlc.narg(deviation_pct),
    sqlc.narg(avg_sale_price), sqlc.arg(sales_quantity)
)
ON CONFLICT (snapshot_date, organization_id, product_id, currency) DO UPDATE SET
    country_id             = EXCLUDED.country_id,
    recommended_version_id = EXCLUDED.recommended_version_id,
    recommended_price      = EXCLUDED.recommended_price,
    list_price             = EXCLUDED.list_price,
    deviation_pct          = EXCLUDED.deviation_pct,
    avg_sale_price         = EXCLUDED.avg_sale_price,
    sales_quantity         = EXCLUDED.sales_quantity,
    computed_at            = NOW()
RETURNING *;

-- name: GetLatestPriceDisciplineSnapshotDate :one
SELECT MAX(snapshot_date)::date AS snapshot_date
FROM price_discipline_snapshots
WHERE brand_id = sqlc.arg(brand_id);

-- name: DeletePriceDisciplineSnapshotsBefore :execrows
DELETE FROM price_discipline_snapshots
WHERE snapshot_date < sqlc.arg(before)::date;

-- name: ListPriceDisciplineSnapshots :many
-- Deviation list of one snapshot day. org_ids is the caller's scope (NULL =
-- whole brand); distributor_ids keeps the distributor and its dealers;
-- over_threshold compares |deviation_pct| with threshold_pct (NULL deviation
-- is never over). Sort keys: deviation_pct (NULLS LAST) | org_name |
-- product_name.
SELECT s.*,
       o.name AS org_name,
       o.type AS org_type,
       o.parent_id AS org_parent_id,
       p.name AS product_name,
       p.sku AS product_sku,
       COALESCE(co.iso2, '')::text AS country_iso2,
       COALESCE(co.name_en, '')::text AS country_name_en,
       COALESCE(co.name_tr, '')::text AS country_name_tr,
       COUNT(*) OVER()::bigint AS total_count
FROM price_discipline_snapshots s
JOIN organizations o ON o.id = s.organization_id
JOIN products p ON p.id = s.product_id
LEFT JOIN countries co ON co.id = s.country_id
WHERE s.brand_id = sqlc.arg(brand_id)
  AND s.snapshot_date = sqlc.arg(snapshot_date)::date
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR s.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(distributor_ids)::bigint[]), 0) = 0
       OR o.id = ANY (sqlc.narg(distributor_ids)::bigint[])
       OR o.parent_id = ANY (sqlc.narg(distributor_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(country_ids)::bigint[]), 0) = 0
       OR s.country_id = ANY (sqlc.narg(country_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(product_ids)::bigint[]), 0) = 0
       OR s.product_id = ANY (sqlc.narg(product_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(currencies)::text[]), 0) = 0
       OR s.currency = ANY (sqlc.narg(currencies)::text[]))
  AND (sqlc.narg(deviation_pct_min)::numeric IS NULL OR s.deviation_pct >= sqlc.narg(deviation_pct_min)::numeric)
  AND (sqlc.narg(deviation_pct_max)::numeric IS NULL OR s.deviation_pct <= sqlc.narg(deviation_pct_max)::numeric)
  AND (sqlc.narg(over_threshold)::bool IS NULL
       OR (COALESCE(ABS(s.deviation_pct) >= sqlc.arg(threshold_pct)::numeric, false) = sqlc.narg(over_threshold)::bool))
  AND (sqlc.narg(q)::text IS NULL
       OR o.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'deviation_pct' THEN s.deviation_pct END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'deviation_pct' THEN s.deviation_pct END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'org_name' THEN o.name
      WHEN 'product_name' THEN p.name
    END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'org_name' THEN o.name
      WHEN 'product_name' THEN p.name
    END
  END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN s.id END DESC,
  s.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
