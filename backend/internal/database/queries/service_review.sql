-- TEC-192 (F1-06h): review request 24 hours after a service is completed.

-- name: ClaimServiceReviewRequest :one
-- Stamps review_request_sent_at when every send condition holds: the
-- service is still completed, the request was not sent yet, the dealer has
-- a google_business_url (TEC-98 decision 7) and the customer is a live,
-- non-anonymized (TEC-161), non-merged user with a phone. The conditional
-- UPDATE is the idempotency barrier: a second run (or a concurrent one)
-- matches no row, so only one outbox event is ever written.
UPDATE services s
SET review_request_sent_at = sqlc.arg(now)::timestamptz
FROM organizations o, users u
WHERE s.id = sqlc.arg(service_id)::bigint
  AND s.status = 'completed'
  AND s.review_request_sent_at IS NULL
  AND o.id = s.organization_id
  AND o.google_business_url IS NOT NULL
  AND btrim(o.google_business_url) <> ''
  AND u.id = s.customer_user_id
  AND u.deleted_at IS NULL
  AND u.status <> 'anonymized'
  AND u.merged_into_user_id IS NULL
  AND u.phone_e164 IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM customer_profiles cp
      WHERE cp.user_id = u.id AND cp.anonymized_at IS NOT NULL
  )
RETURNING s.id, s.uuid, s.service_no, s.organization_id, s.brand_id,
          s.customer_user_id, s.plate, s.review_request_sent_at,
          o.name AS organization_name,
          o.google_business_url::text AS review_url;
