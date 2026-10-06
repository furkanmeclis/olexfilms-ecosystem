-- TEC-285 (F3-01a): contract templates, instances, signers, signatures and
-- media (migration 000083).

-- ---------------------------------------------------------------------------
-- Templates.

-- name: ListContractTemplates :many
SELECT * FROM contract_templates
WHERE brand_id = sqlc.arg(brand_id)
  AND (
    COALESCE(cardinality(sqlc.narg(kinds)::text[]), 0) = 0
    OR kind = ANY (sqlc.narg(kinds)::text[])
  )
  -- TEC-369: active / is_default are true|false|omitted filters.
  AND (sqlc.narg(is_active)::bool IS NULL OR is_active = sqlc.narg(is_active)::bool)
  AND (sqlc.narg(is_default)::bool IS NULL OR is_default = sqlc.narg(is_default)::bool)
ORDER BY kind, is_default DESC, name, id;

-- name: GetContractTemplateByUUID :one
SELECT * FROM contract_templates
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: GetContractTemplateByID :one
SELECT * FROM contract_templates WHERE id = sqlc.arg(id);

-- name: GetDefaultContractTemplate :one
SELECT * FROM contract_templates
WHERE brand_id = sqlc.arg(brand_id) AND kind = sqlc.arg(kind) AND is_default;

-- name: CreateContractTemplate :one
INSERT INTO contract_templates (
    organization_id, brand_id, name, kind, is_default, otp_required,
    signature_required, is_active, created_by_user_id, updated_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(name), sqlc.arg(kind),
    sqlc.arg(is_default), sqlc.arg(otp_required), sqlc.arg(signature_required),
    sqlc.arg(is_active), sqlc.narg(created_by_user_id), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: UpdateContractTemplate :one
UPDATE contract_templates
SET name = sqlc.arg(name),
    is_default = sqlc.arg(is_default),
    otp_required = sqlc.arg(otp_required),
    signature_required = sqlc.arg(signature_required),
    is_active = sqlc.arg(is_active),
    updated_by_user_id = sqlc.narg(updated_by_user_id)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: ClearDefaultContractTemplate :exec
-- Run before setting a new default in the same transaction.
UPDATE contract_templates SET is_default = false
WHERE brand_id = sqlc.arg(brand_id) AND kind = sqlc.arg(kind) AND is_default
  AND id <> sqlc.arg(keep_id);

-- name: DeleteContractTemplate :execrows
-- A template used by an instance cannot be deleted (FK RESTRICT); the use
-- case deactivates it instead.
DELETE FROM contract_templates
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- ---------------------------------------------------------------------------
-- Template locales.

-- name: ListContractTemplateLocales :many
SELECT * FROM contract_template_locales
WHERE template_id = sqlc.arg(template_id)
ORDER BY locale;

-- name: GetContractTemplateLocale :one
SELECT * FROM contract_template_locales
WHERE template_id = sqlc.arg(template_id) AND locale = sqlc.arg(locale);

-- name: UpsertContractTemplateLocale :one
-- Inserts version 1 or replaces the content and bumps the version.
INSERT INTO contract_template_locales (
    template_id, organization_id, brand_id, locale, lexical_json, html, updated_by_user_id
)
VALUES (
    sqlc.arg(template_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(locale),
    sqlc.narg(lexical_json), sqlc.arg(html), sqlc.narg(updated_by_user_id)
)
ON CONFLICT (template_id, locale) DO UPDATE
SET lexical_json = EXCLUDED.lexical_json,
    html = EXCLUDED.html,
    updated_by_user_id = EXCLUDED.updated_by_user_id,
    version = contract_template_locales.version + 1
RETURNING *;

-- name: DeleteContractTemplateLocale :execrows
DELETE FROM contract_template_locales
WHERE template_id = sqlc.arg(template_id) AND locale = sqlc.arg(locale);

-- ---------------------------------------------------------------------------
-- Numbering.

-- name: NextContractNo :one
-- Takes the next contract number of the organization (1, 2, ...). The row
-- lock is held until the calling transaction ends, so numbers are gap-free
-- unless that transaction rolls back.
INSERT INTO contract_counters (organization_id, next_seq)
VALUES (sqlc.arg(organization_id), 2)
ON CONFLICT (organization_id) DO UPDATE SET next_seq = contract_counters.next_seq + 1
RETURNING (next_seq - 1)::bigint AS contract_no;

-- ---------------------------------------------------------------------------
-- Instances.

-- name: CreateContractInstance :one
INSERT INTO contract_instances (
    organization_id, brand_id, contract_no, subject_type, subject_id,
    template_id, kind, locale, template_version, otp_required, signature_required,
    status, rendered_html, content_sha256, created_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(contract_no),
    sqlc.arg(subject_type), sqlc.arg(subject_id),
    sqlc.arg(template_id), sqlc.arg(kind), sqlc.arg(locale), sqlc.arg(template_version),
    sqlc.arg(otp_required), sqlc.arg(signature_required),
    sqlc.arg(status), sqlc.narg(rendered_html), sqlc.narg(content_sha256),
    sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetContractInstanceByUUID :one
SELECT * FROM contract_instances WHERE uuid = sqlc.arg(uuid);

-- name: GetContractInstanceByUUIDScoped :one
SELECT * FROM contract_instances
WHERE uuid = sqlc.arg(uuid)
  AND (
    sqlc.narg(brand_id)::bigint IS NULL
    OR brand_id = sqlc.narg(brand_id)::bigint
  )
  AND (
    sqlc.arg(org_ids)::bigint[] IS NULL
    OR organization_id = ANY(sqlc.arg(org_ids)::bigint[])
  );

-- name: GetContractInstanceByID :one
SELECT * FROM contract_instances WHERE id = sqlc.arg(id);

-- name: GetContractInstanceForUpdate :one
SELECT * FROM contract_instances WHERE id = sqlc.arg(id) FOR UPDATE;

-- name: ListContractInstances :many
-- org_ids is the caller's scope (empty = no organization).
SELECT * FROM contract_instances
WHERE organization_id = ANY(sqlc.arg(org_ids)::bigint[])
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::int OFFSET sqlc.arg(row_offset)::int;

-- name: CountContractInstances :one
SELECT COUNT(*)::bigint FROM contract_instances
WHERE organization_id = ANY(sqlc.arg(org_ids)::bigint[])
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind)::text);

-- name: ListContractInstancesBySubject :many
SELECT * FROM contract_instances
WHERE subject_type = sqlc.arg(subject_type) AND subject_id = sqlc.arg(subject_id)
ORDER BY created_at DESC, id DESC;

-- name: UpdateContractInstanceContent :one
-- Re-renders an open contract (draft or pending).
UPDATE contract_instances
SET locale = sqlc.arg(locale),
    template_version = sqlc.arg(template_version),
    rendered_html = sqlc.narg(rendered_html),
    content_sha256 = sqlc.narg(content_sha256)
WHERE id = sqlc.arg(id) AND status IN ('draft', 'pending')
RETURNING *;

-- name: SetContractInstanceStatus :one
-- draft <-> pending only; execute and void have their own queries.
UPDATE contract_instances
SET status = sqlc.arg(status)
WHERE id = sqlc.arg(id) AND status IN ('draft', 'pending')
RETURNING *;

-- name: ExecuteContractInstance :one
UPDATE contract_instances
SET status = 'executed',
    rendered_html = sqlc.arg(rendered_html)::text,
    content_sha256 = sqlc.arg(content_sha256)::text,
    executed_at = NOW()
WHERE id = sqlc.arg(id) AND status IN ('draft', 'pending')
RETURNING *;

-- name: SetContractInstancePDFKey :one
UPDATE contract_instances
SET pdf_key = sqlc.arg(pdf_key)::text
WHERE id = sqlc.arg(id) AND status = 'executed' AND pdf_key IS NULL
RETURNING *;

-- name: GetPortalContractPDF :one
SELECT ci.*
FROM contract_instances ci
JOIN services s ON s.id = ci.subject_service_id
JOIN brands b ON b.id = ci.brand_id
WHERE ci.uuid = sqlc.arg(uuid)
  AND ci.status = 'executed'
  AND ci.pdf_key IS NOT NULL
  AND ci.brand_id = sqlc.arg(brand_id)::bigint
  AND b.slug <> 'glorian'
  AND (
    s.customer_user_id = sqlc.arg(user_id)::bigint
    OR EXISTS (SELECT 1 FROM warranties hw
               WHERE hw.service_id = s.id AND hw.holder_user_id = sqlc.arg(user_id)::bigint)
  );

-- name: VoidContractInstance :one
UPDATE contract_instances
SET status = 'voided',
    voided_at = NOW(),
    void_reason = sqlc.arg(void_reason)::text,
    voided_by_user_id = sqlc.narg(voided_by_user_id)
WHERE id = sqlc.arg(id) AND status <> 'voided'
RETURNING *;

-- name: DeleteContractInstance :execrows
-- Only an open contract without signatures can be deleted (triggers and
-- FKs); signers and media go first.
DELETE FROM contract_instances
WHERE id = sqlc.arg(id) AND status IN ('draft', 'pending');

-- name: SetServiceContract :execrows
-- Links (or unlinks with NULL) a contract of the service's organization.
UPDATE services
SET contract_id = sqlc.narg(contract_id)
WHERE id = sqlc.arg(service_id);

-- name: GetServiceForContractByUUID :one
SELECT
    s.*,
    cu.name AS customer_name,
    cu.surname AS customer_surname,
    cu.email AS customer_email,
    cu.phone_e164 AS customer_phone,
    creator.name AS staff_name,
    creator.surname AS staff_surname,
    o.name AS organization_name,
    o.phone AS organization_phone,
    o.email AS organization_email,
    o.address AS organization_address,
    cb.name AS car_brand_name,
    cm.name AS car_model_name
FROM services s
JOIN users cu ON cu.id = s.customer_user_id AND cu.deleted_at IS NULL
LEFT JOIN users creator ON creator.id = s.created_by_user_id AND creator.deleted_at IS NULL
JOIN organizations o ON o.id = s.organization_id AND o.deleted_at IS NULL
LEFT JOIN car_brands cb ON cb.id = s.car_brand_id
LEFT JOIN car_models cm ON cm.id = s.car_model_id
WHERE s.uuid = sqlc.arg(uuid)
  AND (
    sqlc.narg(brand_id)::bigint IS NULL
    OR s.brand_id = sqlc.narg(brand_id)::bigint
  )
  AND (
    sqlc.arg(org_ids)::bigint[] IS NULL
    OR s.organization_id = ANY(sqlc.arg(org_ids)::bigint[])
  );

-- name: GetServiceForContractByID :one
SELECT
    s.*,
    cu.name AS customer_name,
    cu.surname AS customer_surname,
    cu.email AS customer_email,
    cu.phone_e164 AS customer_phone,
    creator.name AS staff_name,
    creator.surname AS staff_surname,
    o.name AS organization_name,
    o.phone AS organization_phone,
    o.email AS organization_email,
    o.address AS organization_address,
    cb.name AS car_brand_name,
    cm.name AS car_model_name
FROM services s
JOIN users cu ON cu.id = s.customer_user_id AND cu.deleted_at IS NULL
LEFT JOIN users creator ON creator.id = s.created_by_user_id AND creator.deleted_at IS NULL
JOIN organizations o ON o.id = s.organization_id AND o.deleted_at IS NULL
LEFT JOIN car_brands cb ON cb.id = s.car_brand_id
LEFT JOIN car_models cm ON cm.id = s.car_model_id
WHERE s.id = sqlc.arg(id);

-- name: GetServiceSubscriptionForContractByID :one
SELECT * FROM service_subscriptions
WHERE id = sqlc.arg(id);

-- ---------------------------------------------------------------------------
-- Signers.

-- name: UpsertContractSigner :one
-- Fills the customer or staff slot of an open contract; a changed person
-- clears the OTP proof and the signed time.
INSERT INTO contract_signers (
    instance_id, organization_id, brand_id, role, user_id, name, phone_e164
)
VALUES (
    sqlc.arg(instance_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(role),
    sqlc.narg(user_id), sqlc.arg(name), sqlc.narg(phone_e164)
)
ON CONFLICT (instance_id, role) DO UPDATE
SET user_id = EXCLUDED.user_id,
    name = EXCLUDED.name,
    phone_e164 = EXCLUDED.phone_e164,
    otp_code_id = CASE
        WHEN contract_signers.user_id IS NOT DISTINCT FROM EXCLUDED.user_id
         AND contract_signers.phone_e164 IS NOT DISTINCT FROM EXCLUDED.phone_e164
        THEN contract_signers.otp_code_id END,
    otp_verified_at = CASE
        WHEN contract_signers.user_id IS NOT DISTINCT FROM EXCLUDED.user_id
         AND contract_signers.phone_e164 IS NOT DISTINCT FROM EXCLUDED.phone_e164
        THEN contract_signers.otp_verified_at END,
    signed_at = CASE
        WHEN contract_signers.user_id IS NOT DISTINCT FROM EXCLUDED.user_id
         AND contract_signers.phone_e164 IS NOT DISTINCT FROM EXCLUDED.phone_e164
        THEN contract_signers.signed_at END
RETURNING *;

-- name: ListContractSigners :many
SELECT * FROM contract_signers
WHERE instance_id = sqlc.arg(instance_id)
ORDER BY role, id;

-- name: ListContractPDFSigners :many
SELECT cs.id, cs.uuid, cs.instance_id, cs.organization_id, cs.brand_id, cs.role,
       cs.user_id, cs.name, cs.phone_e164, cs.otp_code_id, cs.otp_verified_at,
       cs.signed_at, cs.created_at, cs.updated_at,
       o.kvkk_locale, o.kvkk_version,
       COALESCE(sig.storage_key, '')::text AS signature_storage_key
FROM contract_signers cs
LEFT JOIN otp_codes o ON o.id = cs.otp_code_id
LEFT JOIN LATERAL (
  SELECT storage_key
  FROM contract_signatures
  WHERE signer_id = cs.id
  ORDER BY created_at DESC, id DESC
  LIMIT 1
) sig ON true
WHERE cs.instance_id = sqlc.arg(instance_id)
ORDER BY cs.role, cs.id;

-- name: GetContractSigner :one
SELECT * FROM contract_signers
WHERE instance_id = sqlc.arg(instance_id) AND role = sqlc.arg(role);

-- name: SetContractSignerOTP :one
-- Stores the consumed contract_sign OTP row as the signer's proof.
UPDATE contract_signers
SET otp_code_id = sqlc.arg(otp_code_id),
    otp_verified_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetContractSignerOTPByUUID :one
UPDATE contract_signers
SET otp_code_id = o.id,
    otp_verified_at = NOW()
FROM otp_codes o
WHERE contract_signers.id = sqlc.arg(signer_id)
  AND o.uuid = sqlc.arg(otp_uuid)
RETURNING contract_signers.*;

-- name: MarkContractSignerSigned :one
UPDATE contract_signers
SET signed_at = NOW()
WHERE id = sqlc.arg(id) AND signed_at IS NULL
RETURNING *;

-- name: DeleteContractSigners :execrows
DELETE FROM contract_signers WHERE instance_id = sqlc.arg(instance_id);

-- ---------------------------------------------------------------------------
-- Signatures (append-only).

-- name: InsertContractSignature :one
INSERT INTO contract_signatures (
    signer_id, instance_id, organization_id, brand_id, storage_key, sha256, ip_address, user_agent
)
VALUES (
    sqlc.arg(signer_id), sqlc.arg(instance_id), sqlc.arg(organization_id), sqlc.arg(brand_id),
    sqlc.arg(storage_key), sqlc.arg(sha256), sqlc.narg(ip_address), sqlc.narg(user_agent)
)
RETURNING *;

-- name: ListContractSignatures :many
SELECT * FROM contract_signatures
WHERE instance_id = sqlc.arg(instance_id)
ORDER BY created_at, id;

-- name: GetLatestContractSignature :one
SELECT * FROM contract_signatures
WHERE signer_id = sqlc.arg(signer_id)
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- ---------------------------------------------------------------------------
-- Media.

-- name: InsertContractMedia :one
INSERT INTO contract_media (
    instance_id, organization_id, brand_id, storage_key, mime_type, size_bytes,
    sha256, title, sort_order, uploaded_by_user_id
)
VALUES (
    sqlc.arg(instance_id), sqlc.arg(organization_id), sqlc.arg(brand_id),
    sqlc.arg(storage_key), sqlc.arg(mime_type), sqlc.arg(size_bytes), sqlc.arg(sha256),
    sqlc.narg(title), sqlc.arg(sort_order), sqlc.narg(uploaded_by_user_id)
)
RETURNING *;

-- name: ListContractMedia :many
SELECT * FROM contract_media
WHERE instance_id = sqlc.arg(instance_id)
ORDER BY sort_order, id;

-- name: GetContractMediaByUUID :one
SELECT * FROM contract_media
WHERE uuid = sqlc.arg(uuid) AND instance_id = sqlc.arg(instance_id);

-- name: DeleteContractMedia :execrows
DELETE FROM contract_media
WHERE uuid = sqlc.arg(uuid) AND instance_id = sqlc.arg(instance_id);
