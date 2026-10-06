-- name: GetDocumentTemplateByUUID :one
SELECT * FROM document_templates WHERE uuid = $1;

-- name: GetDocumentTemplateByID :one
SELECT * FROM document_templates WHERE id = $1;

-- name: GetActiveDocumentTemplate :one
SELECT * FROM document_templates
WHERE kind = sqlc.arg(kind)
  AND language = sqlc.arg(language)
  AND brand_id IS NOT DISTINCT FROM sqlc.narg(brand_id)::bigint
  AND is_active
LIMIT 1;

-- name: GetDraftDocumentTemplate :one
SELECT * FROM document_templates
WHERE kind = sqlc.arg(kind)
  AND language = sqlc.arg(language)
  AND brand_id IS NOT DISTINCT FROM sqlc.narg(brand_id)::bigint
  AND published_at IS NULL
LIMIT 1;

-- name: NextDocumentTemplateVersion :one
SELECT (COALESCE(MAX(version), 0) + 1)::int AS next_version
FROM document_templates
WHERE kind = sqlc.arg(kind)
  AND language = sqlc.arg(language)
  AND brand_id IS NOT DISTINCT FROM sqlc.narg(brand_id)::bigint;

-- name: CreateDocumentTemplate :one
INSERT INTO document_templates (
    kind, brand_id, language, name, version, lexical_json, html, variables, content_hash, created_by
) VALUES (
    sqlc.arg(kind), sqlc.narg(brand_id), sqlc.arg(language), sqlc.arg(name), sqlc.arg(version),
    sqlc.narg(lexical_json), sqlc.arg(html), sqlc.arg(variables), sqlc.arg(content_hash), sqlc.narg(created_by)
)
RETURNING *;

-- name: UpdateDocumentTemplateDraft :one
UPDATE document_templates
SET name = sqlc.arg(name),
    lexical_json = sqlc.narg(lexical_json),
    html = sqlc.arg(html),
    variables = sqlc.arg(variables),
    content_hash = sqlc.arg(content_hash)
WHERE id = sqlc.arg(id) AND published_at IS NULL
RETURNING *;

-- name: DeactivateDocumentTemplates :exec
UPDATE document_templates
SET is_active = FALSE
WHERE kind = sqlc.arg(kind)
  AND language = sqlc.arg(language)
  AND brand_id IS NOT DISTINCT FROM sqlc.narg(brand_id)::bigint
  AND is_active;

-- name: PublishDocumentTemplate :one
UPDATE document_templates
SET is_active = TRUE, published_at = NOW()
WHERE id = $1 AND published_at IS NULL
RETURNING *;

-- name: ListDocumentTemplates :many
-- Sort: docs/list-contract.md, keys from documents handler templatesSortSpec.
-- Secondary order keeps the catalog grouping (kind, brand, language, version DESC).
SELECT t.*, b.slug AS brand_slug
FROM document_templates t
LEFT JOIN brands b ON b.id = t.brand_id
WHERE (
    COALESCE(cardinality(sqlc.narg(kinds)::text[]), 0) = 0
    OR t.kind = ANY (sqlc.narg(kinds)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(languages)::text[]), 0) = 0
    OR t.language = ANY (sqlc.narg(languages)::text[])
  )
  AND (NOT sqlc.arg(current_only)::bool OR t.is_active OR t.published_at IS NULL)
  -- Derived status: draft (unpublished), active, superseded (published, not active).
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR (CASE WHEN t.published_at IS NULL THEN 'draft' WHEN t.is_active THEN 'active' ELSE 'superseded' END)
      = ANY (sqlc.narg(statuses)::text[])
  )
  -- Brand: platform_default_only = brand NULL; brand_id = that brand's override.
  AND (NOT sqlc.arg(platform_default_only)::bool OR t.brand_id IS NULL)
  AND (sqlc.narg(brand_id)::bigint IS NULL OR t.brand_id = sqlc.narg(brand_id))
  AND (sqlc.narg(q)::text IS NULL OR t.name ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'kind' THEN t.kind::text WHEN 'language' THEN t.language::text WHEN 'name' THEN t.name::text
    END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'kind' THEN t.kind::text WHEN 'language' THEN t.language::text WHEN 'name' THEN t.name::text
    END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'version' THEN t.version END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'version' THEN t.version END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'updated_at' THEN t.updated_at WHEN 'created_at' THEN t.created_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'updated_at' THEN t.updated_at WHEN 'created_at' THEN t.created_at END
  END DESC,
  t.kind, t.brand_id NULLS FIRST, t.language, t.version DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN t.id END DESC,
  t.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountDocumentTemplates :one
SELECT COUNT(*)::bigint
FROM document_templates t
WHERE (
    COALESCE(cardinality(sqlc.narg(kinds)::text[]), 0) = 0
    OR t.kind = ANY (sqlc.narg(kinds)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(languages)::text[]), 0) = 0
    OR t.language = ANY (sqlc.narg(languages)::text[])
  )
  AND (NOT sqlc.arg(current_only)::bool OR t.is_active OR t.published_at IS NULL)
  -- Derived status: draft (unpublished), active, superseded (published, not active).
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR (CASE WHEN t.published_at IS NULL THEN 'draft' WHEN t.is_active THEN 'active' ELSE 'superseded' END)
      = ANY (sqlc.narg(statuses)::text[])
  )
  -- Brand: platform_default_only = brand NULL; brand_id = that brand's override.
  AND (NOT sqlc.arg(platform_default_only)::bool OR t.brand_id IS NULL)
  AND (sqlc.narg(brand_id)::bigint IS NULL OR t.brand_id = sqlc.narg(brand_id))
  AND (sqlc.narg(q)::text IS NULL OR t.name ILIKE '%' || sqlc.narg(q) || '%');

-- name: ListDocumentTemplateVersions :many
SELECT * FROM document_templates
WHERE kind = sqlc.arg(kind)
  AND language = sqlc.arg(language)
  AND brand_id IS NOT DISTINCT FROM sqlc.narg(brand_id)::bigint
ORDER BY version DESC;

-- name: UpsertDocumentRender :one
-- One row per cache key: a repeated request returns the existing row.
INSERT INTO document_renders (
    organization_id, brand_id, kind, template_id, template_version,
    source_type, source_id, source_version, locale, cache_key, requested_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(kind), sqlc.arg(template_id), sqlc.arg(template_version),
    sqlc.arg(source_type), sqlc.arg(source_id), sqlc.arg(source_version), sqlc.arg(locale), sqlc.arg(cache_key),
    sqlc.narg(requested_by)
)
ON CONFLICT (cache_key) DO UPDATE SET cache_key = EXCLUDED.cache_key
RETURNING *;

-- name: GetDocumentRenderByID :one
SELECT * FROM document_renders WHERE id = $1;

-- name: GetDocumentRenderByUUID :one
SELECT * FROM document_renders WHERE uuid = $1;

-- name: RetryDocumentRender :one
-- A failed render is re-queued with a new attempt number (new task id).
UPDATE document_renders
SET status = 'pending', error = NULL, attempts = attempts + 1
WHERE id = $1 AND status = 'failed'
RETURNING *;

-- name: MarkDocumentRenderProcessing :one
UPDATE document_renders
SET status = 'processing'
WHERE id = $1 AND status IN ('pending', 'processing', 'failed')
RETURNING *;

-- name: MarkDocumentRenderReady :one
UPDATE document_renders
SET status = 'ready',
    storage_key = sqlc.arg(storage_key),
    sha256 = sqlc.arg(sha256),
    size_bytes = sqlc.arg(size_bytes),
    error = NULL,
    rendered_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: MarkDocumentRenderFailed :exec
UPDATE document_renders
SET status = 'failed', error = sqlc.arg(error)
WHERE id = sqlc.arg(id);
