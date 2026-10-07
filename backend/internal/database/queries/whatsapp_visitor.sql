-- TEC-397 (F4-02e): WhatsApp visitor flow — abuse limits, the KVKK notice
-- before a lead, the 30 day lead window and the address of a visitor lead.

-- name: CountConversationModelRunsSince :one
-- AI runs of a conversation since a time that called the model (a fixed
-- reply without a model call is not a turn).
SELECT COUNT(*)::bigint
FROM conversation_ai_runs
WHERE conversation_id = sqlc.arg(conversation_id)
  AND model <> ''
  AND started_at >= sqlc.arg(since)::timestamptz;

-- name: SumVisitorWhatsAppTokensSince :one
-- Quota tokens of unidentified WhatsApp contacts (no user) on the system
-- pool of an organization since a time.
SELECT COALESCE(SUM(quota_tokens), 0)::bigint
FROM ai_usage
WHERE organization_id = sqlc.arg(organization_id)
  AND pool = 'system'
  AND channel = 'whatsapp'
  AND user_id IS NULL
  AND created_at >= sqlc.arg(since)::timestamptz;

-- name: CountConversationRunsWithStageBefore :one
-- Earlier AI runs (id below before_run_id) of a conversation since a time
-- that recorded a stage (e.g. the KVKK notice of a visitor lead).
SELECT COUNT(*)::bigint
FROM conversation_ai_runs
WHERE conversation_id = sqlc.arg(conversation_id)
  AND id < sqlc.arg(before_run_id)::bigint
  AND started_at >= sqlc.arg(since)::timestamptz
  AND stages @> jsonb_build_array(jsonb_build_object('stage', sqlc.arg(stage)::text));

-- name: GetRecentWhatsAppVisitorLead :one
-- The newest WhatsApp customer lead of a phone number in a brand since a
-- time (one lead per number per 30 days).
SELECT * FROM leads
WHERE brand_id = sqlc.arg(brand_id)
  AND candidate_phone_e164 = sqlc.arg(phone_e164)
  AND source = 'whatsapp'
  AND target_type = 'customer'
  AND deleted_at IS NULL
  AND created_at >= sqlc.arg(since)::timestamptz
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: FindProvinceByName :one
-- A province of an active country by name, case- and Turkish-accent-
-- insensitively; Turkey first when several countries have the name.
SELECT p.*
FROM provinces p
JOIN countries c ON c.id = p.country_id
WHERE c.is_active
  AND lower(translate(p.name, 'İIıŞşĞğÜüÖöÇç', 'iiissgguuoocc')) = lower(translate(sqlc.arg(name)::text, 'İIıŞşĞğÜüÖöÇç', 'iiissgguuoocc'))
ORDER BY (c.iso2 = 'TR') DESC, p.id
LIMIT 1;

-- name: FindDistrictByName :one
-- A district of a province by name, case- and Turkish-accent-insensitively.
SELECT * FROM districts
WHERE province_id = sqlc.arg(province_id)
  AND lower(translate(name, 'İIıŞşĞğÜüÖöÇç', 'iiissgguuoocc')) = lower(translate(sqlc.arg(name)::text, 'İIıŞşĞğÜüÖöÇç', 'iiissgguuoocc'))
ORDER BY id
LIMIT 1;
