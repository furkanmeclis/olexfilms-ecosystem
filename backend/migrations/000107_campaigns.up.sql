-- TEC-404 (F4-04a): campaign schema. Campaign, per-locale contents and their
-- media, the append-only approval/lifecycle events, the recipient snapshot
-- with its statistics projection, the marketing consent legal text and the
-- campaigns.approve permission. Endpoints land in TEC-405+ (F4-04b..d).
--
--   * campaigns: channels push | whatsapp | email (one or more, no SMS);
--     audience_filter is the validated audience definition (F4-04b);
--     approver_org_id is the organization whose members decide the
--     campaign (distributor for a dealer, center otherwise; F4 user answer
--     S4). recipients_* are the statistics projection maintained by the
--     campaign_recipients triggers below.
--   * campaign_contents: one row per (campaign, locale); channel length
--     limits are checked in the application.
--   * campaign_media: image | document attached to a content.
--   * campaign_events: append-only; rows go away only together with their
--     campaign (deleting a draft).
--   * campaign_recipients: the snapshot taken when sending starts, one row
--     per (campaign, user, channel). target_address is the E.164 number
--     (whatsapp) or the e-mail address; push rows carry the token count.
--   * legal_texts / consents kind marketing_consent: explicit opt-in to
--     commercial messages (unchecked by default). Customers are reached only
--     with an accepted marketing_consent and no marketing opt-out in
--     contact_opt_out_state (000103).

-- 1. Campaigns ---------------------------------------------------------------
-- Channel list: non-empty, known values, no duplicates (array_position finds
-- the first occurrence only, so a repeated value fails the check).
CREATE FUNCTION campaign_channels_valid(ch TEXT[]) RETURNS BOOLEAN
LANGUAGE sql IMMUTABLE AS $$
    SELECT ch IS NOT NULL
       AND cardinality(ch) BETWEEN 1 AND 3
       AND ch <@ ARRAY['push', 'whatsapp', 'email']::text[]
       AND (cardinality(ch) < 2 OR ch[1] <> ch[2])
       AND (cardinality(ch) < 3 OR (ch[3] <> ch[1] AND ch[3] <> ch[2]))
$$;

CREATE TABLE campaigns (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE CASCADE,
    name                VARCHAR(200)  NOT NULL,
    channels            TEXT[]        NOT NULL,
    audience_filter     JSONB         NOT NULL DEFAULT '{}'::jsonb,
    status              VARCHAR(24)   NOT NULL DEFAULT 'draft',
    scheduled_at        TIMESTAMPTZ   NULL,
    approver_org_id     BIGINT        NULL REFERENCES organizations (id) ON DELETE SET NULL,
    started_at          TIMESTAMPTZ   NULL,
    finished_at         TIMESTAMPTZ   NULL,
    recipients_total    INTEGER       NOT NULL DEFAULT 0,
    recipients_sent     INTEGER       NOT NULL DEFAULT 0,
    recipients_failed   INTEGER       NOT NULL DEFAULT 0,
    recipients_skipped  INTEGER       NOT NULL DEFAULT 0,
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_campaigns_uuid UNIQUE (uuid),
    -- Target of the composite foreign keys of events and recipients.
    CONSTRAINT uq_campaigns_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT chk_campaigns_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_campaigns_channels CHECK (campaign_channels_valid(channels)),
    CONSTRAINT chk_campaigns_audience_filter CHECK (jsonb_typeof(audience_filter) = 'object'),
    CONSTRAINT chk_campaigns_status CHECK (status IN (
        'draft', 'pending_approval', 'approved', 'scheduled', 'sending',
        'sent', 'partially_failed', 'cancelled', 'rejected')),
    CONSTRAINT chk_campaigns_scheduled CHECK (status <> 'scheduled' OR scheduled_at IS NOT NULL),
    CONSTRAINT chk_campaigns_approver CHECK (status <> 'pending_approval' OR approver_org_id IS NOT NULL),
    CONSTRAINT chk_campaigns_finished CHECK (
        finished_at IS NULL OR status IN ('sent', 'partially_failed', 'cancelled')),
    CONSTRAINT chk_campaigns_counters CHECK (
        recipients_total >= 0 AND recipients_sent >= 0 AND recipients_failed >= 0
        AND recipients_skipped >= 0
        AND recipients_sent + recipients_failed + recipients_skipped <= recipients_total)
);

CREATE INDEX idx_campaigns_org_created ON campaigns (organization_id, created_at DESC, id DESC);
CREATE INDEX idx_campaigns_brand_created ON campaigns (brand_id, created_at DESC, id DESC);
CREATE INDEX idx_campaigns_brand_status ON campaigns (brand_id, status);
CREATE INDEX idx_campaigns_approval_queue ON campaigns (approver_org_id, created_at)
    WHERE status = 'pending_approval';
CREATE INDEX idx_campaigns_due ON campaigns (scheduled_at)
    WHERE status = 'scheduled';
CREATE INDEX idx_campaigns_created_by ON campaigns (created_by_user_id) WHERE created_by_user_id IS NOT NULL;

CREATE TRIGGER trg_campaigns_set_updated_at
    BEFORE UPDATE ON campaigns
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 2. Contents per locale -----------------------------------------------------
-- title: push title / e-mail subject (unused by WhatsApp).
CREATE TABLE campaign_contents (
    id           BIGSERIAL     PRIMARY KEY,
    uuid         UUID          NOT NULL DEFAULT gen_random_uuid(),
    campaign_id  BIGINT        NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    locale       VARCHAR(8)    NOT NULL,
    title        VARCHAR(200)  NOT NULL DEFAULT '',
    body         TEXT          NOT NULL DEFAULT '',
    deeplink     TEXT          NULL,
    created_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_campaign_contents_uuid UNIQUE (uuid),
    CONSTRAINT uq_campaign_contents_locale UNIQUE (campaign_id, locale),
    CONSTRAINT chk_campaign_contents_locale CHECK (
        locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar')),
    CONSTRAINT chk_campaign_contents_body CHECK (char_length(body) <= 20000),
    CONSTRAINT chk_campaign_contents_deeplink CHECK (
        deeplink IS NULL OR (btrim(deeplink) <> '' AND char_length(deeplink) <= 2048))
);

CREATE TRIGGER trg_campaign_contents_set_updated_at
    BEFORE UPDATE ON campaign_contents
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 3. Media of a content ------------------------------------------------------
CREATE TABLE campaign_media (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    content_id          BIGINT        NOT NULL REFERENCES campaign_contents (id) ON DELETE CASCADE,
    kind                VARCHAR(16)   NOT NULL,
    storage_key         TEXT          NOT NULL,
    mime_type           VARCHAR(128)  NOT NULL,
    size_bytes          BIGINT        NOT NULL,
    file_name           VARCHAR(255)  NULL,
    sort_order          INTEGER       NOT NULL DEFAULT 0,
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_campaign_media_uuid UNIQUE (uuid),
    CONSTRAINT chk_campaign_media_kind CHECK (kind IN ('image', 'document')),
    CONSTRAINT chk_campaign_media_storage_key CHECK (btrim(storage_key) <> ''),
    CONSTRAINT chk_campaign_media_mime CHECK (
        (kind = 'image' AND mime_type IN ('image/jpeg', 'image/png', 'image/webp'))
        OR (kind = 'document' AND mime_type = 'application/pdf')),
    CONSTRAINT chk_campaign_media_size CHECK (size_bytes > 0),
    CONSTRAINT chk_campaign_media_file_name CHECK (file_name IS NULL OR btrim(file_name) <> '')
);

CREATE INDEX idx_campaign_media_content ON campaign_media (content_id, sort_order, id);

-- 4. Events (append-only) ----------------------------------------------------
CREATE TABLE campaign_events (
    id               BIGSERIAL     PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    campaign_id      BIGINT        NOT NULL,
    organization_id  BIGINT        NOT NULL,
    brand_id         BIGINT        NOT NULL,
    event_type       VARCHAR(24)   NOT NULL,
    from_status      VARCHAR(24)   NULL,
    to_status        VARCHAR(24)   NULL,
    actor_user_id    BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    actor_org_id     BIGINT        NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    reason           TEXT          NULL,
    payload          JSONB         NOT NULL DEFAULT '{}'::jsonb,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_campaign_events_uuid UNIQUE (uuid),
    CONSTRAINT fk_campaign_events_campaign FOREIGN KEY (campaign_id, organization_id, brand_id)
        REFERENCES campaigns (id, organization_id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_campaign_events_type CHECK (event_type IN (
        'submitted', 'approved', 'rejected', 'changes_requested', 'scheduled',
        'cancelled', 'started', 'finished')),
    -- rejected and changes_requested always carry the reason (F4-04c).
    CONSTRAINT chk_campaign_events_reason CHECK (
        (event_type NOT IN ('rejected', 'changes_requested') OR (reason IS NOT NULL AND btrim(reason) <> ''))
        AND (reason IS NULL OR char_length(reason) <= 2000)),
    CONSTRAINT chk_campaign_events_payload CHECK (jsonb_typeof(payload) = 'object')
);

CREATE INDEX idx_campaign_events_campaign ON campaign_events (campaign_id, created_at, id);
CREATE INDEX idx_campaign_events_org ON campaign_events (organization_id, created_at DESC);
CREATE INDEX idx_campaign_events_actor ON campaign_events (actor_user_id) WHERE actor_user_id IS NOT NULL;
CREATE INDEX idx_campaign_events_actor_org ON campaign_events (actor_org_id) WHERE actor_org_id IS NOT NULL;

-- Updates and truncation are refused; a delete passes only when the campaign
-- itself is gone (ON DELETE CASCADE of a deleted draft).
CREATE FUNCTION campaign_events_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' AND NOT EXISTS (SELECT 1 FROM campaigns WHERE id = OLD.campaign_id) THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'campaign_events is append-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$;

CREATE TRIGGER trg_campaign_events_append_only
    BEFORE UPDATE OR DELETE ON campaign_events
    FOR EACH ROW
    EXECUTE FUNCTION campaign_events_append_only();

CREATE TRIGGER trg_campaign_events_no_truncate
    BEFORE TRUNCATE ON campaign_events
    FOR EACH STATEMENT
    EXECUTE FUNCTION campaign_events_append_only();

-- 5. Recipient snapshot ------------------------------------------------------
CREATE TABLE campaign_recipients (
    id                BIGSERIAL     PRIMARY KEY,
    campaign_id       BIGINT        NOT NULL,
    organization_id   BIGINT        NOT NULL,
    brand_id          BIGINT        NOT NULL,
    user_id           BIGINT        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    channel           VARCHAR(16)   NOT NULL,
    locale            VARCHAR(8)    NOT NULL,
    target_address    VARCHAR(320)  NULL,
    push_token_count  INTEGER       NULL,
    status            VARCHAR(16)   NOT NULL DEFAULT 'pending',
    reason            TEXT          NULL,
    attempts          INTEGER       NOT NULL DEFAULT 0,
    sent_at           TIMESTAMPTZ   NULL,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_campaign_recipients_user_channel UNIQUE (campaign_id, user_id, channel),
    CONSTRAINT fk_campaign_recipients_campaign FOREIGN KEY (campaign_id, organization_id, brand_id)
        REFERENCES campaigns (id, organization_id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_campaign_recipients_channel CHECK (channel IN ('push', 'whatsapp', 'email')),
    CONSTRAINT chk_campaign_recipients_locale CHECK (
        locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar')),
    CONSTRAINT chk_campaign_recipients_target CHECK (
        (channel = 'push' AND target_address IS NULL AND push_token_count IS NOT NULL AND push_token_count >= 0)
        OR (channel = 'whatsapp' AND push_token_count IS NULL AND target_address ~ '^\+[1-9][0-9]{6,14}$')
        OR (channel = 'email' AND push_token_count IS NULL AND target_address ~ '^[^@\s]+@[^@\s]+$')),
    CONSTRAINT chk_campaign_recipients_status CHECK (status IN ('pending', 'sent', 'failed', 'skipped')),
    CONSTRAINT chk_campaign_recipients_sent_at CHECK ((status = 'sent') = (sent_at IS NOT NULL)),
    CONSTRAINT chk_campaign_recipients_reason CHECK (reason IS NULL OR char_length(reason) <= 1000),
    CONSTRAINT chk_campaign_recipients_attempts CHECK (attempts >= 0)
);

CREATE INDEX idx_campaign_recipients_campaign_status ON campaign_recipients (campaign_id, status, id);
CREATE INDEX idx_campaign_recipients_user ON campaign_recipients (user_id);

CREATE TRIGGER trg_campaign_recipients_set_updated_at
    BEFORE UPDATE ON campaign_recipients
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Statistics projection: campaigns.recipients_* follow the snapshot in the
-- same statement. Statement-level triggers with transition tables keep a
-- snapshot insert of thousands of rows to one update per campaign.
CREATE FUNCTION campaign_recipients_apply_counts(
    p_campaign_id BIGINT, d_total BIGINT, d_sent BIGINT, d_failed BIGINT, d_skipped BIGINT
) RETURNS void
LANGUAGE sql AS $$
    UPDATE campaigns
    SET recipients_total   = recipients_total + d_total,
        recipients_sent    = recipients_sent + d_sent,
        recipients_failed  = recipients_failed + d_failed,
        recipients_skipped = recipients_skipped + d_skipped
    WHERE id = p_campaign_id
$$;

CREATE FUNCTION campaign_recipients_project_insert() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM campaign_recipients_apply_counts(n.campaign_id, n.total, n.sent, n.failed, n.skipped)
    FROM (
        SELECT campaign_id,
               COUNT(*) AS total,
               COUNT(*) FILTER (WHERE status = 'sent') AS sent,
               COUNT(*) FILTER (WHERE status = 'failed') AS failed,
               COUNT(*) FILTER (WHERE status = 'skipped') AS skipped
        FROM new_rows
        GROUP BY campaign_id
    ) n;
    RETURN NULL;
END;
$$;

CREATE FUNCTION campaign_recipients_project_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM campaign_recipients_apply_counts(d.campaign_id, 0, d.sent, d.failed, d.skipped)
    FROM (
        SELECT campaign_id, SUM(sent) AS sent, SUM(failed) AS failed, SUM(skipped) AS skipped
        FROM (
            SELECT campaign_id,
                   (status = 'sent')::int AS sent, (status = 'failed')::int AS failed,
                   (status = 'skipped')::int AS skipped
            FROM new_rows
            UNION ALL
            SELECT campaign_id,
                   -(status = 'sent')::int, -(status = 'failed')::int, -(status = 'skipped')::int
            FROM old_rows
        ) x
        GROUP BY campaign_id
    ) d
    WHERE d.sent <> 0 OR d.failed <> 0 OR d.skipped <> 0;
    RETURN NULL;
END;
$$;

CREATE FUNCTION campaign_recipients_project_delete() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM campaign_recipients_apply_counts(o.campaign_id, -o.total, -o.sent, -o.failed, -o.skipped)
    FROM (
        SELECT campaign_id,
               COUNT(*) AS total,
               COUNT(*) FILTER (WHERE status = 'sent') AS sent,
               COUNT(*) FILTER (WHERE status = 'failed') AS failed,
               COUNT(*) FILTER (WHERE status = 'skipped') AS skipped
        FROM old_rows
        GROUP BY campaign_id
    ) o;
    RETURN NULL;
END;
$$;

CREATE TRIGGER trg_campaign_recipients_project_insert
    AFTER INSERT ON campaign_recipients
    REFERENCING NEW TABLE AS new_rows
    FOR EACH STATEMENT
    EXECUTE FUNCTION campaign_recipients_project_insert();

CREATE TRIGGER trg_campaign_recipients_project_update
    AFTER UPDATE ON campaign_recipients
    REFERENCING OLD TABLE AS old_rows NEW TABLE AS new_rows
    FOR EACH STATEMENT
    EXECUTE FUNCTION campaign_recipients_project_update();

CREATE TRIGGER trg_campaign_recipients_project_delete
    AFTER DELETE ON campaign_recipients
    REFERENCING OLD TABLE AS old_rows
    FOR EACH STATEMENT
    EXECUTE FUNCTION campaign_recipients_project_delete();

-- 6. Marketing consent legal text -------------------------------------------
ALTER TABLE legal_texts DROP CONSTRAINT chk_legal_texts_kind;
ALTER TABLE legal_texts ADD CONSTRAINT chk_legal_texts_kind CHECK (
    kind IN ('ai_guidelines', 'marketing_consent'));
ALTER TABLE consents DROP CONSTRAINT chk_consents_kind;
ALTER TABLE consents ADD CONSTRAINT chk_consents_kind CHECK (
    kind IN ('ai_guidelines', 'marketing_consent'));

INSERT INTO legal_texts (kind, locale, version, body) VALUES
    ('marketing_consent', 'tr', 1, E'## Ticari elektronik ileti izni\n\nKampanya, indirim, yeni ürün ve hizmet duyurularının bana push bildirimi, WhatsApp ve e-posta yoluyla gönderilmesine izin veriyorum.\n\n- Bu izin isteğe bağlıdır; vermemeniz hizmet ve garanti işlemlerinizi etkilemez.\n- Hizmet, randevu ve garanti bildirimleri bu izinden bağımsız olarak gönderilir.\n- İzninizi istediğiniz zaman profilinizden geri alabilirsiniz.'),
    ('marketing_consent', 'en', 1, E'## Consent to commercial messages\n\nI agree to receive campaigns, discounts and announcements of new products and services by push notification, WhatsApp and e-mail.\n\n- This consent is optional; declining does not affect your services or warranties.\n- Service, appointment and warranty notifications are sent regardless of this consent.\n- You can withdraw your consent at any time from your profile.');

CREATE INDEX idx_consents_kind_user ON consents (kind, user_id, decided_at DESC, id DESC);

-- 7. Permission campaigns.approve (catalog entry in internal/platform/rbac)
-- and the default campaign grants.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Approve campaigns', 'campaigns.approve', 'campaigns',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Approve, reject or send back campaigns submitted to the organization for approval.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'campaigns.approve', 'all'),
    ('center_social', 'campaigns.approve', 'brand'),
    ('distributor_owner', 'campaigns.read', 'subtree'),
    ('distributor_owner', 'campaigns.write', 'managed'),
    ('distributor_owner', 'campaigns.approve', 'subtree'),
    ('dealer_owner', 'campaigns.read', 'managed'),
    ('dealer_owner', 'campaigns.write', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
