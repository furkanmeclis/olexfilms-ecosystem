-- TEC-193 (F1-08c2): admin customer merge (TEC-100 decision 2). The source
-- user is never deleted: it keeps its id, gets merged_into_user_id, status
-- disabled and an unusable password; its customer records move to the
-- target user. The warranty holder move lives here (not in warranties.sql)
-- because it belongs to the customer merge.

-- Locks both users in id order (no deadlock between two opposite merges).
-- name: LockUsersForMerge :many
SELECT * FROM users
WHERE id = ANY (sqlc.arg(ids)::bigint[]) AND deleted_at IS NULL
ORDER BY id
FOR UPDATE;

-- Open (pending) vehicle transfers that involve the user; the transfer's
-- current owner is immutable, so a merge waits until they are closed.
-- name: CountPendingVehicleTransfersForUser :one
SELECT COUNT(*)::bigint FROM vehicle_transfers
WHERE status = 'pending' AND (from_user_id = sqlc.arg(user_id) OR to_user_id = sqlc.arg(user_id));

-- Organization links the target already has: the target row keeps the
-- earliest dates of both rows.
-- name: MergeConflictingCustomerOrganizations :execrows
UPDATE customer_organizations t
SET first_service_at = CASE
        WHEN s.first_service_at IS NULL THEN t.first_service_at
        WHEN t.first_service_at IS NULL THEN s.first_service_at
        ELSE LEAST(t.first_service_at, s.first_service_at)
    END,
    created_at = LEAST(t.created_at, s.created_at)
FROM customer_organizations s
WHERE s.user_id = sqlc.arg(source_user_id)
  AND t.user_id = sqlc.arg(target_user_id)
  AND t.organization_id = s.organization_id;

-- The duplicate link of the source is folded into the target's (above).
-- name: DeleteMergedCustomerOrganizations :execrows
DELETE FROM customer_organizations s
WHERE s.user_id = sqlc.arg(source_user_id)
  AND EXISTS (SELECT 1 FROM customer_organizations t
              WHERE t.user_id = sqlc.arg(target_user_id) AND t.organization_id = s.organization_id);

-- name: MoveCustomerOrganizations :execrows
UPDATE customer_organizations
SET user_id = sqlc.arg(target_user_id)
WHERE user_id = sqlc.arg(source_user_id);

-- Every vehicle (also soft-deleted ones, so their services can follow).
-- name: MoveVehiclesToUser :execrows
UPDATE vehicles
SET user_id = sqlc.arg(target_user_id)
WHERE user_id = sqlc.arg(source_user_id);

-- Services follow their vehicle. A service whose vehicle was transferred to
-- a third person stays with the source (services_check_row requires the
-- vehicle to belong to the customer).
-- name: MoveServicesToUser :execrows
UPDATE services s
SET customer_user_id = sqlc.arg(target_user_id)
FROM vehicles v
WHERE s.customer_user_id = sqlc.arg(source_user_id)
  AND v.id = s.vehicle_id
  AND v.user_id = sqlc.arg(target_user_id);

-- name: CountServicesOfUser :one
SELECT COUNT(*)::bigint FROM services WHERE customer_user_id = sqlc.arg(customer_user_id);

-- Warranty holder (warranties_check_row keeps the holder writable).
-- name: MoveWarrantiesToHolder :execrows
UPDATE warranties
SET holder_user_id = sqlc.arg(target_user_id)
WHERE holder_user_id = sqlc.arg(source_user_id);

-- Consents the target has not decided yet; a decision the target already
-- made for the same legal text wins and the source's stays as a record.
-- name: MoveConsentsToUser :execrows
UPDATE consents s
SET user_id = sqlc.arg(target_user_id)
WHERE s.user_id = sqlc.arg(source_user_id)
  AND NOT EXISTS (SELECT 1 FROM consents t
                  WHERE t.user_id = sqlc.arg(target_user_id) AND t.legal_text_id = s.legal_text_id);

-- name: CountConsentsOfUser :one
SELECT COUNT(*)::bigint FROM consents WHERE user_id = sqlc.arg(user_id);

-- Customer cari accounts are ledgers (append-only spirit): they are not
-- moved, only reported.
-- name: CountUserCariAccounts :one
SELECT COUNT(*)::bigint FROM cari_accounts WHERE counterparty_user_id = sqlc.arg(user_id);

-- name: CountActiveRefreshTokensForUser :one
SELECT COUNT(*)::bigint FROM refresh_tokens WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- The source's profile becomes the target's when the target has none.
-- name: MoveCustomerProfile :execrows
UPDATE customer_profiles s
SET user_id = sqlc.arg(target_user_id)
WHERE s.user_id = sqlc.arg(source_user_id)
  AND NOT EXISTS (SELECT 1 FROM customer_profiles t WHERE t.user_id = sqlc.arg(target_user_id));

-- Both have a profile: the target keeps its values and only fills its empty
-- fields from the source (an identity number moves with its mask).
-- name: FillCustomerProfileFromSource :execrows
UPDATE customer_profiles t
SET company_name      = COALESCE(t.company_name, s.company_name),
    tax_office        = COALESCE(t.tax_office, s.tax_office),
    national_id_enc   = CASE WHEN t.national_id_enc IS NULL THEN s.national_id_enc ELSE t.national_id_enc END,
    national_id_last4 = CASE WHEN t.national_id_enc IS NULL THEN s.national_id_last4 ELSE t.national_id_last4 END,
    tax_no_enc        = CASE WHEN t.tax_no_enc IS NULL THEN s.tax_no_enc ELSE t.tax_no_enc END,
    tax_no_last4      = CASE WHEN t.tax_no_enc IS NULL THEN s.tax_no_last4 ELSE t.tax_no_last4 END,
    address           = CASE WHEN t.address = '{}'::jsonb THEN s.address ELSE t.address END
FROM customer_profiles s
WHERE s.user_id = sqlc.arg(source_user_id)
  AND t.user_id = sqlc.arg(target_user_id);

-- Closes the source account. phone_e164/email are the values it keeps (the
-- caller clears the ones handed to the target; the e-mail is a placeholder
-- when nothing is left, chk_users_email_or_phone). Runs before the target
-- takes the identifiers over (unique indexes are checked per statement).
-- name: CloseMergedUser :one
UPDATE users
SET merged_into_user_id = sqlc.arg(target_user_id),
    status              = 'disabled',
    password_hash       = sqlc.arg(password_hash),
    phone_e164          = sqlc.narg(phone_e164),
    phone_verified_at   = CASE WHEN sqlc.narg(phone_e164)::text IS NULL THEN NULL ELSE phone_verified_at END,
    email               = sqlc.narg(email),
    email_verified_at   = CASE WHEN sqlc.narg(email)::text IS DISTINCT FROM email THEN NULL ELSE email_verified_at END
WHERE id = sqlc.arg(id) AND merged_into_user_id IS NULL
RETURNING *;

-- The target takes over the phone / e-mail it does not have yet.
-- name: TakeOverMergedIdentity :exec
UPDATE users
SET phone_e164        = COALESCE(phone_e164, sqlc.narg(phone_e164)),
    phone_verified_at = CASE WHEN phone_e164 IS NULL AND sqlc.narg(phone_e164)::text IS NOT NULL
                             THEN sqlc.narg(phone_verified_at) ELSE phone_verified_at END,
    email             = COALESCE(email, sqlc.narg(email)),
    email_verified_at = CASE WHEN email IS NULL AND sqlc.narg(email)::text IS NOT NULL
                             THEN sqlc.narg(email_verified_at) ELSE email_verified_at END
WHERE id = sqlc.arg(id);
