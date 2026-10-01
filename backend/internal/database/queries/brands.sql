-- name: ListBrands :many
SELECT * FROM brands
ORDER BY id ASC;

-- name: ListBrandDomains :many
SELECT d.host, d.brand_id
FROM brand_domains d
ORDER BY d.host ASC;

-- name: GetBrandBySlug :one
SELECT * FROM brands
WHERE slug = $1;

-- name: GetBrandByID :one
SELECT * FROM brands
WHERE id = $1;

-- name: GetBrandCenter :one
SELECT * FROM organizations
WHERE brand_id = $1 AND type = 'center' AND deleted_at IS NULL;
