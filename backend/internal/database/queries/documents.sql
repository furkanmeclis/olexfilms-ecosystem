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
SELECT t.*, b.slug AS brand_slug
FROM document_templates t
LEFT JOIN brands b ON b.id = t.brand_id
WHERE (sqlc.narg(kind)::text IS NULL OR t.kind = sqlc.narg(kind)::text)
  AND (sqlc.narg(language)::text IS NULL OR t.language = sqlc.narg(language)::text)
  AND (NOT sqlc.arg(current_only)::bool OR t.is_active OR t.published_at IS NULL)
ORDER BY t.kind, t.brand_id NULLS FIRST, t.language, t.version DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountDocumentTemplates :one
SELECT COUNT(*)::bigint
FROM document_templates t
WHERE (sqlc.narg(kind)::text IS NULL OR t.kind = sqlc.narg(kind)::text)
  AND (sqlc.narg(language)::text IS NULL OR t.language = sqlc.narg(language)::text)
  AND (NOT sqlc.arg(current_only)::bool OR t.is_active OR t.published_at IS NULL);

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
