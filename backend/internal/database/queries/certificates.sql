-- TEC-479 (F5-03a): certificate types, user certificates and service
-- certificate warnings. API/usecase layers enforce the "at least one
-- product/category binding" rule by checking CountCertificateTypeBindings
-- in the same transaction that edits bindings.

-- ---------------------------------------------------------------------------
-- Certificate types.

-- name: CreateCertificateType :one
INSERT INTO certificate_types (
    organization_id, brand_id, name, description, validity_months, active, sort_order
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(name), sqlc.arg(description),
    sqlc.narg(validity_months), sqlc.arg(active), sqlc.arg(sort_order)
)
RETURNING *;

-- name: GetCertificateType :one
SELECT * FROM certificate_types
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: GetCertificateTypeByUUID :one
SELECT * FROM certificate_types
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: UpdateCertificateType :one
UPDATE certificate_types
SET name = sqlc.arg(name),
    description = sqlc.arg(description),
    validity_months = sqlc.narg(validity_months),
    active = sqlc.arg(active),
    sort_order = sqlc.arg(sort_order)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: DeleteCertificateType :execrows
DELETE FROM certificate_types
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: ListCertificateTypes :many
SELECT * FROM certificate_types
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(active)::bool IS NULL OR active = sqlc.narg(active)::bool)
  AND (
    sqlc.narg(q)::text IS NULL
    OR name::text ILIKE '%' || sqlc.narg(q)::text || '%'
    OR description::text ILIKE '%' || sqlc.narg(q)::text || '%'
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'sort_order' THEN sort_order END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'sort_order' THEN sort_order END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN created_at WHEN 'updated_at' THEN updated_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN created_at WHEN 'updated_at' THEN updated_at END
  END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN id END DESC,
  id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountCertificateTypes :one
SELECT COUNT(*)::bigint
FROM certificate_types
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(active)::bool IS NULL OR active = sqlc.narg(active)::bool)
  AND (
    sqlc.narg(q)::text IS NULL
    OR name::text ILIKE '%' || sqlc.narg(q)::text || '%'
    OR description::text ILIKE '%' || sqlc.narg(q)::text || '%'
  );

-- name: AddCertificateTypeCategory :one
INSERT INTO certificate_type_categories (type_id, organization_id, brand_id, category_id)
VALUES (sqlc.arg(type_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(category_id))
ON CONFLICT (type_id, category_id) DO NOTHING
RETURNING *;

-- name: AddCertificateTypeProduct :one
INSERT INTO certificate_type_products (type_id, organization_id, brand_id, product_id)
VALUES (sqlc.arg(type_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(product_id))
ON CONFLICT (type_id, product_id) DO NOTHING
RETURNING *;

-- name: DeleteCertificateTypeCategories :execrows
DELETE FROM certificate_type_categories
WHERE type_id = sqlc.arg(type_id) AND brand_id = sqlc.arg(brand_id);

-- name: DeleteCertificateTypeProducts :execrows
DELETE FROM certificate_type_products
WHERE type_id = sqlc.arg(type_id) AND brand_id = sqlc.arg(brand_id);

-- name: ListCertificateTypeCategories :many
SELECT * FROM certificate_type_categories
WHERE type_id = sqlc.arg(type_id) AND brand_id = sqlc.arg(brand_id)
ORDER BY category_id;

-- name: ListCertificateTypeProducts :many
SELECT * FROM certificate_type_products
WHERE type_id = sqlc.arg(type_id) AND brand_id = sqlc.arg(brand_id)
ORDER BY product_id;

-- name: CountCertificateTypeBindings :one
SELECT (
    (SELECT COUNT(*) FROM certificate_type_categories ctc WHERE ctc.type_id = sqlc.arg(type_id) AND ctc.brand_id = sqlc.arg(brand_id))
    +
    (SELECT COUNT(*) FROM certificate_type_products ctp WHERE ctp.type_id = sqlc.arg(type_id) AND ctp.brand_id = sqlc.arg(brand_id))
)::bigint;

-- ---------------------------------------------------------------------------
-- Certificates.

-- name: CreateCertificate :one
INSERT INTO certificates (
    user_id, organization_id, brand_id, type_id, storage_key, sha256, issued_at, expires_at, status
)
SELECT
    sqlc.arg(user_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(type_id),
    sqlc.arg(storage_key), sqlc.arg(sha256), sqlc.arg(issued_at),
    COALESCE(
        sqlc.narg(expires_at)::timestamptz,
        CASE WHEN ct.validity_months IS NULL THEN NULL
             ELSE sqlc.arg(issued_at)::timestamptz + (ct.validity_months || ' months')::interval END
    ),
    sqlc.arg(status)::text
FROM certificate_types ct
WHERE ct.id = sqlc.arg(type_id) AND ct.brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: GetCertificate :one
SELECT * FROM certificates
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: GetCertificateByUUID :one
SELECT * FROM certificates
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: VerifyCertificate :one
UPDATE certificates
SET status = 'valid',
    verified_by_user_id = sqlc.arg(verified_by_user_id),
    verified_by_org_id = sqlc.arg(verified_by_org_id),
    verified_at = sqlc.arg(verified_at),
    reject_reason = NULL
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id) AND status = 'pending'
RETURNING *;

-- name: RejectCertificate :one
UPDATE certificates
SET status = 'rejected',
    verified_by_user_id = sqlc.arg(verified_by_user_id),
    verified_by_org_id = sqlc.arg(verified_by_org_id),
    verified_at = sqlc.arg(verified_at),
    reject_reason = sqlc.arg(reject_reason)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id) AND status = 'pending'
RETURNING *;

-- name: RevokeCertificate :one
UPDATE certificates
SET status = 'revoked',
    verified_by_user_id = NULL,
    verified_by_org_id = NULL,
    verified_at = NULL,
    reject_reason = NULL
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id) AND status IN ('pending', 'valid')
RETURNING *;

-- name: UpdateCertificateStorageKey :one
UPDATE certificates
SET storage_key = sqlc.arg(storage_key)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: UpdateCertificateExpiry :one
UPDATE certificates
SET expires_at = sqlc.narg(expires_at)::timestamptz
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: ExpireDueCertificates :many
UPDATE certificates
SET status = 'expired',
    verified_by_user_id = NULL,
    verified_by_org_id = NULL,
    verified_at = NULL,
    reject_reason = NULL
WHERE status = 'valid'
  AND expires_at IS NOT NULL
  AND expires_at <= sqlc.arg(now)::timestamptz
RETURNING *;

-- name: ListCertificates :many
-- Sort keys follow docs/list-contract.md: expires_at, issued_at, status,
-- user_name and created_at. Default is expires_at; id is the tiebreak.
SELECT c.*, u.name AS user_name, u.surname AS user_surname, u.uuid AS user_uuid,
       ct.uuid AS type_uuid, ct.name AS type_name,
       o.uuid AS organization_uuid, o.name AS organization_name
FROM certificates c
JOIN users u ON u.id = c.user_id
JOIN certificate_types ct ON ct.id = c.type_id
JOIN organizations o ON o.id = c.organization_id
WHERE c.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR c.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR c.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(type_ids)::bigint[]), 0) = 0 OR c.type_id = ANY (sqlc.narg(type_ids)::bigint[]))
  AND (sqlc.narg(expires_from)::timestamptz IS NULL OR c.expires_at >= sqlc.narg(expires_from)::timestamptz)
  AND (sqlc.narg(expires_to)::timestamptz IS NULL OR c.expires_at < sqlc.narg(expires_to)::timestamptz)
  AND (
    COALESCE(cardinality(sqlc.narg(organization_uuids)::uuid[]), 0) = 0
    OR c.organization_id IN (SELECT fo.id FROM organizations fo WHERE fo.uuid = ANY (sqlc.narg(organization_uuids)::uuid[]))
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR (u.name || ' ' || u.surname) ILIKE '%' || sqlc.narg(q)::text || '%'
    OR u.email ILIKE '%' || sqlc.narg(q)::text || '%'
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'expires_at' THEN c.expires_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'expires_at' THEN c.expires_at END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'status' THEN c.status::text
      WHEN 'user_name' THEN (u.name || ' ' || u.surname)::text
    END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'status' THEN c.status::text
      WHEN 'user_name' THEN (u.name || ' ' || u.surname)::text
    END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'issued_at' THEN c.issued_at WHEN 'created_at' THEN c.created_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'issued_at' THEN c.issued_at WHEN 'created_at' THEN c.created_at END
  END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN c.id END DESC,
  c.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountCertificates :one
SELECT COUNT(*)::bigint
FROM certificates c
JOIN users u ON u.id = c.user_id
WHERE c.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR c.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR c.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(type_ids)::bigint[]), 0) = 0 OR c.type_id = ANY (sqlc.narg(type_ids)::bigint[]))
  AND (sqlc.narg(expires_from)::timestamptz IS NULL OR c.expires_at >= sqlc.narg(expires_from)::timestamptz)
  AND (sqlc.narg(expires_to)::timestamptz IS NULL OR c.expires_at < sqlc.narg(expires_to)::timestamptz)
  AND (
    COALESCE(cardinality(sqlc.narg(organization_uuids)::uuid[]), 0) = 0
    OR c.organization_id IN (SELECT fo.id FROM organizations fo WHERE fo.uuid = ANY (sqlc.narg(organization_uuids)::uuid[]))
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR (u.name || ' ' || u.surname) ILIKE '%' || sqlc.narg(q)::text || '%'
    OR u.email ILIKE '%' || sqlc.narg(q)::text || '%'
  );

-- Required certificate types for the products/categories used by a service.
-- name: ListRequiredCertificateTypesForService :many
SELECT DISTINCT ct.*
FROM certificate_types ct
WHERE ct.brand_id = sqlc.arg(brand_id)
  AND ct.active = true
  AND (
    EXISTS (
      SELECT 1
      FROM certificate_type_products ctp
      JOIN service_items si ON si.product_id = ctp.product_id AND si.brand_id = ctp.brand_id
      WHERE ctp.type_id = ct.id AND si.service_id = sqlc.arg(service_id)
    )
    OR EXISTS (
      SELECT 1
      FROM certificate_type_categories ctc
      JOIN products p ON p.category_id = ctc.category_id AND p.brand_id = ctc.brand_id
      JOIN service_items si ON si.product_id = p.id AND si.brand_id = p.brand_id
      WHERE ctc.type_id = ct.id AND si.service_id = sqlc.arg(service_id)
    )
  )
ORDER BY ct.sort_order, ct.id;

-- name: ListValidCertificatesForServiceUser :many
SELECT DISTINCT c.*
FROM certificates c
WHERE c.brand_id = sqlc.arg(brand_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND c.user_id = sqlc.arg(user_id)
  AND c.status = 'valid'
  AND c.issued_at <= sqlc.arg(now)::timestamptz
  AND (c.expires_at IS NULL OR c.expires_at > sqlc.arg(now)::timestamptz)
  AND c.type_id IN (
    SELECT ct.id
    FROM certificate_types ct
    WHERE ct.brand_id = sqlc.arg(brand_id)
      AND ct.active = true
      AND (
        EXISTS (
          SELECT 1
          FROM certificate_type_products ctp
          JOIN service_items si ON si.product_id = ctp.product_id AND si.brand_id = ctp.brand_id
          WHERE ctp.type_id = ct.id AND si.service_id = sqlc.arg(service_id)
        )
        OR EXISTS (
          SELECT 1
          FROM certificate_type_categories ctc
          JOIN products p ON p.category_id = ctc.category_id AND p.brand_id = ctc.brand_id
          JOIN service_items si ON si.product_id = p.id AND si.brand_id = p.brand_id
          WHERE ctc.type_id = ct.id AND si.service_id = sqlc.arg(service_id)
        )
      )
  )
ORDER BY c.expires_at ASC NULLS LAST, c.id ASC;

-- name: ListCertificatesDueForExpiryNotice :many
SELECT * FROM certificates
WHERE status = 'valid'
  AND expiry_notice_sent_at IS NULL
  AND expires_at IS NOT NULL
  AND expires_at > sqlc.arg(now)::timestamptz
  AND expires_at <= sqlc.arg(now)::timestamptz + (sqlc.arg(days)::int || ' days')::interval
ORDER BY expires_at, id
LIMIT sqlc.arg(row_limit);

-- name: MarkCertificateExpiryNoticeSent :execrows
UPDATE certificates
SET expiry_notice_sent_at = sqlc.arg(now)::timestamptz
WHERE id = sqlc.arg(id) AND status = 'valid' AND expiry_notice_sent_at IS NULL;

-- ---------------------------------------------------------------------------
-- Service warnings.

-- name: UpsertServiceCertificateWarning :one
INSERT INTO service_certificate_warnings (
    service_id, organization_id, brand_id, user_id, type_id, reason, decision, note
)
VALUES (
    sqlc.arg(service_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(user_id),
    sqlc.arg(type_id), sqlc.arg(reason), sqlc.arg(decision), sqlc.arg(note)
)
ON CONFLICT (service_id, user_id, type_id) DO UPDATE
SET reason = EXCLUDED.reason,
    decision = CASE
        WHEN service_certificate_warnings.decision IN ('approved', 'rejected')
            THEN service_certificate_warnings.decision
        ELSE EXCLUDED.decision
    END,
    decided_by = CASE
        WHEN service_certificate_warnings.decision IN ('approved', 'rejected')
            THEN service_certificate_warnings.decided_by
        ELSE NULL
    END,
    decided_at = CASE
        WHEN service_certificate_warnings.decision IN ('approved', 'rejected')
            THEN service_certificate_warnings.decided_at
        ELSE NULL
    END,
    note = EXCLUDED.note
RETURNING *;

-- name: MarkServiceCertificateWarningsPending :execrows
UPDATE service_certificate_warnings
SET decision = 'pending_approval'
WHERE service_id = sqlc.arg(service_id)
  AND brand_id = sqlc.arg(brand_id)
  AND decision = 'none';

-- name: CountBlockingServiceCertificateWarnings :one
SELECT COUNT(*)::bigint
FROM service_certificate_warnings
WHERE service_id = sqlc.arg(service_id)
  AND brand_id = sqlc.arg(brand_id)
  AND decision IN ('none', 'pending_approval', 'rejected');

-- name: ListServiceCertificateWarningsByService :many
SELECT w.*, ct.uuid AS type_uuid, ct.name AS type_name,
       u.uuid AS user_uuid, u.name AS user_name, u.surname AS user_surname
FROM service_certificate_warnings w
JOIN certificate_types ct ON ct.id = w.type_id
JOIN users u ON u.id = w.user_id
WHERE w.service_id = sqlc.arg(service_id)
  AND w.brand_id = sqlc.arg(brand_id)
ORDER BY w.created_at, w.id;

-- name: GetServiceCertificateWarningByUUID :one
SELECT *
FROM service_certificate_warnings
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: LatestCertificateForUserType :one
SELECT *
FROM certificates
WHERE user_id = sqlc.arg(user_id)
  AND organization_id = sqlc.arg(organization_id)
  AND brand_id = sqlc.arg(brand_id)
  AND type_id = sqlc.arg(type_id)
ORDER BY
  CASE status WHEN 'valid' THEN 1 WHEN 'pending' THEN 2 WHEN 'expired' THEN 3 ELSE 4 END,
  expires_at DESC NULLS LAST,
  created_at DESC,
  id DESC
LIMIT 1;

-- name: DecideServiceCertificateWarning :one
UPDATE service_certificate_warnings
SET decision = sqlc.arg(decision)::text,
    decided_by = sqlc.arg(decided_by),
    decided_at = sqlc.arg(decided_at),
    note = sqlc.arg(note)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
  AND decision IN ('none', 'pending_approval')
  AND sqlc.arg(decision)::text IN ('approved', 'rejected')
RETURNING *;

-- name: ListServiceCertificateWarnings :many
SELECT w.*, s.uuid AS service_uuid, s.service_no,
       u.uuid AS user_uuid, u.name AS user_name, u.surname AS user_surname,
       ct.uuid AS type_uuid, ct.name AS type_name,
       o.uuid AS organization_uuid, o.name AS organization_name
FROM service_certificate_warnings w
JOIN services s ON s.id = w.service_id
JOIN users u ON u.id = w.user_id
JOIN certificate_types ct ON ct.id = w.type_id
JOIN organizations o ON o.id = w.organization_id
WHERE w.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR w.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(decisions)::text[]), 0) = 0 OR w.decision = ANY (sqlc.narg(decisions)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(reasons)::text[]), 0) = 0 OR w.reason = ANY (sqlc.narg(reasons)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.service_no ILIKE '%' || sqlc.narg(q)::text || '%'
    OR (u.name || ' ' || u.surname) ILIKE '%' || sqlc.narg(q)::text || '%'
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'decision' THEN w.decision::text
      WHEN 'reason' THEN w.reason::text
      WHEN 'service_no' THEN s.service_no::text
      WHEN 'user_name' THEN (u.name || ' ' || u.surname)::text
    END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'decision' THEN w.decision::text
      WHEN 'reason' THEN w.reason::text
      WHEN 'service_no' THEN s.service_no::text
      WHEN 'user_name' THEN (u.name || ' ' || u.surname)::text
    END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN w.created_at WHEN 'decided_at' THEN w.decided_at END
  END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN w.created_at WHEN 'decided_at' THEN w.decided_at END
  END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN w.id END DESC,
  w.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountServiceCertificateWarnings :one
SELECT COUNT(*)::bigint
FROM service_certificate_warnings w
JOIN services s ON s.id = w.service_id
JOIN users u ON u.id = w.user_id
WHERE w.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR w.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(decisions)::text[]), 0) = 0 OR w.decision = ANY (sqlc.narg(decisions)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(reasons)::text[]), 0) = 0 OR w.reason = ANY (sqlc.narg(reasons)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.service_no ILIKE '%' || sqlc.narg(q)::text || '%'
    OR (u.name || ' ' || u.surname) ILIKE '%' || sqlc.narg(q)::text || '%'
  );
