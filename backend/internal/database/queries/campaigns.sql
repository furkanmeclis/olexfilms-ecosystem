-- TEC-404 (F4-04a): campaigns, contents, media, events, recipient snapshot
-- and the marketing reachability read. Scope: organization_ids NULL = whole
-- brand (center), otherwise the organizations the caller reaches.

-- name: CreateCampaign :one
INSERT INTO campaigns (organization_id, brand_id, name, channels, audience_filter,
                       created_by_user_id, updated_by_user_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(name), sqlc.arg(channels)::text[],
        sqlc.arg(audience_filter)::jsonb, sqlc.narg(created_by_user_id), sqlc.narg(created_by_user_id))
RETURNING *;

-- name: GetCampaignByUUID :one
SELECT * FROM campaigns
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: GetCampaignByIDForUpdate :one
SELECT * FROM campaigns
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
FOR UPDATE;

-- name: UpdateCampaignDraft :one
-- Name, channels and audience of a draft (no row once it left draft).
UPDATE campaigns
SET name               = sqlc.arg(name),
    channels           = sqlc.arg(channels)::text[],
    audience_filter    = sqlc.arg(audience_filter)::jsonb,
    updated_by_user_id = sqlc.narg(actor_user_id)
WHERE id = sqlc.arg(id) AND status = 'draft'
RETURNING *;

-- name: SetCampaignStatus :one
-- Moves the campaign from from_status to status (no row when it moved
-- meanwhile). approver_org_id and scheduled_at are replaced by the given
-- values; sending stamps started_at, the end states stamp finished_at.
UPDATE campaigns
SET status             = sqlc.arg(status)::varchar,
    approver_org_id    = sqlc.narg(approver_org_id)::bigint,
    scheduled_at       = sqlc.narg(scheduled_at)::timestamptz,
    started_at         = CASE WHEN sqlc.arg(status)::varchar = 'sending' THEN COALESCE(started_at, NOW())
                              ELSE started_at END,
    finished_at        = CASE WHEN sqlc.arg(status)::varchar IN ('sent', 'partially_failed', 'cancelled')
                              THEN NOW() ELSE NULL END,
    updated_by_user_id = sqlc.narg(actor_user_id)::bigint
WHERE id = sqlc.arg(id) AND status = sqlc.arg(from_status)::varchar
RETURNING *;

-- name: DeleteDraftCampaign :execrows
DELETE FROM campaigns
WHERE id = sqlc.arg(id) AND status = 'draft';

-- name: ListDueScheduledCampaigns :many
SELECT * FROM campaigns
WHERE status = 'scheduled' AND scheduled_at <= sqlc.arg(now)::timestamptz
ORDER BY scheduled_at, id
LIMIT sqlc.arg(page_limit);

-- List contract (docs/list-contract.md): sort created_at | scheduled_at |
-- name | status (flow rank), default -created_at; statuses and channels are
-- multi-valued (channels match on overlap); scheduled_from / scheduled_to;
-- q matches the name.
-- name: ListCampaigns :many
SELECT * FROM campaigns
WHERE campaigns.brand_id = sqlc.arg(brand_id)
  AND (sqlc.arg(organization_ids)::bigint[] IS NULL OR campaigns.organization_id = ANY(sqlc.arg(organization_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR campaigns.status = ANY(sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(channels)::text[]), 0) = 0 OR campaigns.channels && sqlc.narg(channels)::text[])
  AND (sqlc.narg(scheduled_from)::timestamptz IS NULL OR campaigns.scheduled_at >= sqlc.narg(scheduled_from)::timestamptz)
  AND (sqlc.narg(scheduled_to)::timestamptz IS NULL OR campaigns.scheduled_at < sqlc.narg(scheduled_to)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL OR campaigns.name ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN name END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN name END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'status' THEN
    CASE status WHEN 'draft' THEN 0 WHEN 'pending_approval' THEN 1 WHEN 'approved' THEN 2 WHEN 'scheduled' THEN 3
      WHEN 'sending' THEN 4 WHEN 'sent' THEN 5 WHEN 'partially_failed' THEN 6 WHEN 'cancelled' THEN 7
      WHEN 'rejected' THEN 8 ELSE 9 END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'status' THEN
    CASE status WHEN 'draft' THEN 0 WHEN 'pending_approval' THEN 1 WHEN 'approved' THEN 2 WHEN 'scheduled' THEN 3
      WHEN 'sending' THEN 4 WHEN 'sent' THEN 5 WHEN 'partially_failed' THEN 6 WHEN 'cancelled' THEN 7
      WHEN 'rejected' THEN 8 ELSE 9 END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN created_at END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'scheduled_at' THEN scheduled_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'scheduled_at' THEN scheduled_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN id END DESC,
  id ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountCampaigns :one
SELECT COUNT(*) FROM campaigns
WHERE campaigns.brand_id = sqlc.arg(brand_id)
  AND (sqlc.arg(organization_ids)::bigint[] IS NULL OR campaigns.organization_id = ANY(sqlc.arg(organization_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR campaigns.status = ANY(sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(channels)::text[]), 0) = 0 OR campaigns.channels && sqlc.narg(channels)::text[])
  AND (sqlc.narg(scheduled_from)::timestamptz IS NULL OR campaigns.scheduled_at >= sqlc.narg(scheduled_from)::timestamptz)
  AND (sqlc.narg(scheduled_to)::timestamptz IS NULL OR campaigns.scheduled_at < sqlc.narg(scheduled_to)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL OR campaigns.name ILIKE '%' || sqlc.narg(q)::text || '%');

-- Approval queue: pending campaigns whose approver is one of
-- approver_org_ids (the caller's organizations holding campaigns.approve).
-- Sort created_at | scheduled_at | name, default created_at (oldest first is
-- chosen by the handler); channels multi-valued; q matches the name.
-- name: ListCampaignApprovals :many
SELECT * FROM campaigns
WHERE campaigns.brand_id = sqlc.arg(brand_id)
  AND campaigns.status = 'pending_approval'
  AND campaigns.approver_org_id = ANY(sqlc.arg(approver_org_ids)::bigint[])
  AND (COALESCE(cardinality(sqlc.narg(channels)::text[]), 0) = 0 OR campaigns.channels && sqlc.narg(channels)::text[])
  AND (COALESCE(cardinality(sqlc.narg(organization_uuids)::uuid[]), 0) = 0
       OR campaigns.organization_id IN (SELECT fo.id FROM organizations fo WHERE fo.uuid = ANY (sqlc.narg(organization_uuids)::uuid[])))
  AND (sqlc.narg(q)::text IS NULL OR campaigns.name ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN name END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN name END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN created_at END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'scheduled_at' THEN scheduled_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'scheduled_at' THEN scheduled_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN id END DESC,
  id ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountCampaignApprovals :one
SELECT COUNT(*) FROM campaigns
WHERE campaigns.brand_id = sqlc.arg(brand_id)
  AND campaigns.status = 'pending_approval'
  AND campaigns.approver_org_id = ANY(sqlc.arg(approver_org_ids)::bigint[])
  AND (COALESCE(cardinality(sqlc.narg(channels)::text[]), 0) = 0 OR campaigns.channels && sqlc.narg(channels)::text[])
  AND (COALESCE(cardinality(sqlc.narg(organization_uuids)::uuid[]), 0) = 0
       OR campaigns.organization_id IN (SELECT fo.id FROM organizations fo WHERE fo.uuid = ANY (sqlc.narg(organization_uuids)::uuid[])))
  AND (sqlc.narg(q)::text IS NULL OR campaigns.name ILIKE '%' || sqlc.narg(q)::text || '%');

-- Contents ------------------------------------------------------------------

-- name: UpsertCampaignContent :one
INSERT INTO campaign_contents (campaign_id, locale, title, body, deeplink)
VALUES (sqlc.arg(campaign_id), sqlc.arg(locale), sqlc.arg(title), sqlc.arg(body), sqlc.narg(deeplink))
ON CONFLICT (campaign_id, locale) DO UPDATE
SET title    = EXCLUDED.title,
    body     = EXCLUDED.body,
    deeplink = EXCLUDED.deeplink
RETURNING *;

-- name: GetCampaignContent :one
SELECT * FROM campaign_contents
WHERE campaign_id = sqlc.arg(campaign_id) AND locale = sqlc.arg(locale);

-- name: ListCampaignContents :many
SELECT * FROM campaign_contents
WHERE campaign_id = sqlc.arg(campaign_id)
ORDER BY locale;

-- name: DeleteCampaignContent :execrows
DELETE FROM campaign_contents
WHERE campaign_id = sqlc.arg(campaign_id) AND locale = sqlc.arg(locale);

-- Media ---------------------------------------------------------------------

-- name: InsertCampaignMedia :one
INSERT INTO campaign_media (content_id, kind, storage_key, mime_type, size_bytes, file_name, sort_order,
                            created_by_user_id)
VALUES (sqlc.arg(content_id), sqlc.arg(kind), sqlc.arg(storage_key), sqlc.arg(mime_type), sqlc.arg(size_bytes),
        sqlc.narg(file_name), sqlc.arg(sort_order), sqlc.narg(created_by_user_id))
RETURNING *;

-- name: ListCampaignMedia :many
-- Every medium of the campaign with the locale of its content.
SELECT m.*, c.locale
FROM campaign_media m
JOIN campaign_contents c ON c.id = m.content_id
WHERE c.campaign_id = sqlc.arg(campaign_id)
ORDER BY c.locale, m.sort_order, m.id;

-- name: GetCampaignMediaByUUID :one
SELECT m.*
FROM campaign_media m
JOIN campaign_contents c ON c.id = m.content_id
WHERE m.uuid = sqlc.arg(uuid) AND c.campaign_id = sqlc.arg(campaign_id);

-- name: DeleteCampaignMedia :execrows
DELETE FROM campaign_media
WHERE id = sqlc.arg(id);

-- Events --------------------------------------------------------------------

-- name: InsertCampaignEvent :one
INSERT INTO campaign_events (campaign_id, organization_id, brand_id, event_type, from_status, to_status,
                             actor_user_id, actor_org_id, reason, payload)
VALUES (sqlc.arg(campaign_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(event_type),
        sqlc.narg(from_status), sqlc.narg(to_status), sqlc.narg(actor_user_id), sqlc.narg(actor_org_id),
        sqlc.narg(reason), sqlc.arg(payload)::jsonb)
RETURNING *;

-- name: ListCampaignEvents :many
SELECT * FROM campaign_events
WHERE campaign_id = sqlc.arg(campaign_id)
ORDER BY created_at, id;

-- Recipients ----------------------------------------------------------------

-- name: InsertCampaignRecipient :one
INSERT INTO campaign_recipients (campaign_id, organization_id, brand_id, user_id, channel, locale,
                                 target_address, push_token_count, status, reason)
VALUES (sqlc.arg(campaign_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(user_id),
        sqlc.arg(channel), sqlc.arg(locale), sqlc.narg(target_address), sqlc.narg(push_token_count),
        sqlc.arg(status), sqlc.narg(reason))
RETURNING *;

-- name: ClaimPendingCampaignRecipients :many
-- Next pending recipients of a sending campaign for the worker (F4-04d).
SELECT * FROM campaign_recipients
WHERE campaign_id = sqlc.arg(campaign_id) AND status = 'pending'
  AND (sqlc.narg(channel)::text IS NULL OR channel = sqlc.narg(channel)::text)
ORDER BY id
LIMIT sqlc.arg(page_limit)
FOR UPDATE SKIP LOCKED;

-- name: SetCampaignRecipientStatus :one
-- Ends a pending recipient (sent stamps sent_at); no row when it is no
-- longer pending.
UPDATE campaign_recipients
SET status   = sqlc.arg(status)::varchar,
    reason   = sqlc.narg(reason)::text,
    attempts = attempts + 1,
    sent_at  = CASE WHEN sqlc.arg(status)::varchar = 'sent' THEN NOW() END
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: SkipPendingCampaignRecipients :execrows
-- Cancellation while sending: the remaining pending recipients are skipped.
UPDATE campaign_recipients
SET status = 'skipped',
    reason = sqlc.arg(reason)::text
WHERE campaign_id = sqlc.arg(campaign_id) AND status = 'pending';

-- Recipient list of a campaign: statuses, channels and locales are
-- multi-valued; q matches the target address or the user's name. Sort
-- created_at | sent_at | status | channel | locale, default created_at.
-- name: ListCampaignRecipients :many
SELECT r.*, u.uuid AS user_uuid, u.name AS user_name, u.surname AS user_surname
FROM campaign_recipients r
JOIN users u ON u.id = r.user_id
WHERE r.campaign_id = sqlc.arg(campaign_id)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR r.status = ANY(sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(channels)::text[]), 0) = 0 OR r.channel = ANY(sqlc.narg(channels)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(locales)::text[]), 0) = 0 OR r.locale = ANY(sqlc.narg(locales)::text[]))
  AND (sqlc.narg(q)::text IS NULL
       OR r.target_address ILIKE '%' || sqlc.narg(q)::text || '%'
       OR (u.name || ' ' || u.surname) ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'status' THEN r.status WHEN 'channel' THEN r.channel WHEN 'locale' THEN r.locale END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'status' THEN r.status WHEN 'channel' THEN r.channel WHEN 'locale' THEN r.locale END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN r.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN r.created_at END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'sent_at' THEN r.sent_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'sent_at' THEN r.sent_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN r.id END DESC,
  r.id ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountCampaignRecipients :one
SELECT COUNT(*)
FROM campaign_recipients r
JOIN users u ON u.id = r.user_id
WHERE r.campaign_id = sqlc.arg(campaign_id)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR r.status = ANY(sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(channels)::text[]), 0) = 0 OR r.channel = ANY(sqlc.narg(channels)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(locales)::text[]), 0) = 0 OR r.locale = ANY(sqlc.narg(locales)::text[]))
  AND (sqlc.narg(q)::text IS NULL
       OR r.target_address ILIKE '%' || sqlc.narg(q)::text || '%'
       OR (u.name || ' ' || u.surname) ILIKE '%' || sqlc.narg(q)::text || '%');

-- Marketing reachability ----------------------------------------------------

-- name: ListMarketingReachability :many
-- Per user: the latest marketing_consent decision (any text version; a
-- decline or no record means no consent) and whether the user's phone is
-- opted out of marketing in contact_opt_out_state (000103) or the user
-- identity is opted out through a campaign e-mail unsubscribe (000109).
SELECT u.id AS user_id,
       COALESCE(mc.accepted, false)::bool AS marketing_accepted,
       (COALESCE(oo.opted_out, false) OR COALESCE(uoo.opted_out, false))::bool AS marketing_opted_out
FROM users u
LEFT JOIN LATERAL (
    SELECT c.accepted
    FROM consents c
    WHERE c.user_id = u.id AND c.kind = 'marketing_consent'
    ORDER BY c.decided_at DESC, c.id DESC
    LIMIT 1
) mc ON true
LEFT JOIN contact_opt_out_state oo
    ON oo.contact_e164 = u.phone_e164 AND oo.scope = 'marketing'
LEFT JOIN campaign_user_opt_out_state uoo
    ON uoo.user_id = u.id AND uoo.scope = 'marketing'
WHERE u.id = ANY(sqlc.arg(user_ids)::bigint[]);

-- Sending (TEC-407, F4-04d) -------------------------------------------------

-- name: InsertCampaignRecipientSnapshot :execrows
-- The recipient snapshot of a campaign in one statement (one statistics
-- projection update). rows is a JSON array of {user_id, channel, locale,
-- target_address, push_token_count, status, reason}. An existing
-- (campaign, user, channel) row is kept.
INSERT INTO campaign_recipients (campaign_id, organization_id, brand_id, user_id, channel, locale,
                                 target_address, push_token_count, status, reason)
SELECT sqlc.arg(campaign_id), sqlc.arg(organization_id), sqlc.arg(brand_id), r.user_id, r.channel, r.locale,
       r.target_address, r.push_token_count, r.status, r.reason
FROM jsonb_to_recordset(sqlc.arg(rows)::jsonb) AS r (
    user_id BIGINT, channel TEXT, locale TEXT, target_address TEXT, push_token_count INT, status TEXT, reason TEXT)
ON CONFLICT (campaign_id, user_id, channel) DO NOTHING;

-- name: LockCampaignRecipient :one
SELECT * FROM campaign_recipients
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: GetCampaignRecipientByID :one
SELECT * FROM campaign_recipients
WHERE id = sqlc.arg(id);

-- name: RecordCampaignRecipientAttempt :one
-- A failed attempt that will be retried: the recipient stays pending.
UPDATE campaign_recipients
SET attempts = attempts + 1,
    reason   = sqlc.narg(reason)::text
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: CountPendingCampaignRecipients :one
SELECT COUNT(*) FROM campaign_recipients
WHERE campaign_id = sqlc.arg(campaign_id) AND status = 'pending';

-- name: ListSendingCampaigns :many
SELECT * FROM campaigns
WHERE status = 'sending'
ORDER BY started_at, id
LIMIT sqlc.arg(page_limit);

-- name: ListPendingCampaignRecipientIDs :many
-- Pending recipients of a sending campaign, in id pages; before (when set)
-- keeps those not touched since then (the scheduler re-enqueues their task;
-- a still queued task is deduplicated by its task id).
SELECT id FROM campaign_recipients
WHERE campaign_id = sqlc.arg(campaign_id) AND status = 'pending'
  AND (sqlc.narg(before)::timestamptz IS NULL OR updated_at < sqlc.narg(before)::timestamptz)
  AND id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(page_limit);

-- name: GetCampaignRecipientContact :one
-- Current contact data of a recipient user (time zone for quiet hours).
SELECT u.id, u.uuid, u.name, u.surname, COALESCE(u.email, '')::text AS email,
       COALESCE(u.phone_e164, '')::text AS phone_e164, COALESCE(u.timezone, '')::text AS timezone
FROM users u
WHERE u.id = sqlc.arg(id);

-- name: InsertCampaignUserOptOut :one
-- Append-only; the AFTER INSERT trigger updates campaign_user_opt_out_state.
INSERT INTO campaign_user_opt_outs (user_id, scope, action, source, created_by_user_id, note)
VALUES (sqlc.arg(user_id), sqlc.arg(scope), sqlc.arg(action), sqlc.arg(source),
        sqlc.narg(created_by_user_id), sqlc.narg(note))
RETURNING *;

-- name: GetCampaignUserOptOutState :one
SELECT * FROM campaign_user_opt_out_state
WHERE user_id = sqlc.arg(user_id) AND scope = sqlc.arg(scope);

-- name: CountWebPushSubscriptionsByUsers :many
SELECT user_id, COUNT(*)::int AS subscriptions
FROM push_subscriptions
WHERE user_id = ANY(sqlc.arg(user_ids)::bigint[])
GROUP BY user_id;

-- name: GetCampaignByID :one
SELECT * FROM campaigns
WHERE id = sqlc.arg(id);
