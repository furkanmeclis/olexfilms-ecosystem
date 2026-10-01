-- TEC-90 (F0-14): customer/fleet portal. Legal texts (admin Markdown, per
-- locale, versioned) and the consent record of each user (K19, K22).
--
-- legal_texts and consents are identity-level rows: a customer is global
-- (K11), so they carry no organization_id / brand_id.

-- 1. Legal texts. Every edit appends a new version for (kind, locale); the
-- latest version is the one shown. A new version asks again (K22).
CREATE TABLE legal_texts (
    id         BIGSERIAL   PRIMARY KEY,
    uuid       UUID        NOT NULL DEFAULT gen_random_uuid(),
    kind       VARCHAR(32) NOT NULL,
    locale     VARCHAR(8)  NOT NULL,
    version    INTEGER     NOT NULL,
    body       TEXT        NOT NULL,
    created_by BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_legal_texts_uuid UNIQUE (uuid),
    CONSTRAINT uq_legal_texts_kind_locale_version UNIQUE (kind, locale, version),
    CONSTRAINT chk_legal_texts_kind CHECK (kind IN ('ai_guidelines')),
    CONSTRAINT chk_legal_texts_version CHECK (version >= 1),
    CONSTRAINT chk_legal_texts_body CHECK (length(btrim(body)) > 0)
);

INSERT INTO legal_texts (kind, locale, version, body) VALUES
    ('ai_guidelines', 'tr', 1, E'## Yapay zekâ asistanı yönergesi\n\nPortaldaki yapay zekâ asistanı sorularınızı yanıtlamak için hizmet ve garanti kayıtlarınızı kullanır. Yanıtlar bilgilendirme amaçlıdır; kesin bilgi için bayinize danışın.\n\n- Asistana kişisel sağlık, kimlik veya ödeme bilgisi yazmayın.\n- Konuşmalar hizmet kalitesi için saklanır.\n- Onayınızı istediğiniz zaman profilinizden geri alabilirsiniz.'),
    ('ai_guidelines', 'en', 1, E'## AI assistant guidelines\n\nThe AI assistant in the portal uses your service and warranty records to answer your questions. Answers are for information only; ask your dealer for binding details.\n\n- Do not share health, identity or payment details with the assistant.\n- Conversations are kept to improve the service.\n- You can withdraw your consent at any time from your profile.');

-- 2. Consents: one row per user and legal text version. A decline is a
-- record too ("asked once"); it never locks the portal.
CREATE TABLE consents (
    id            BIGSERIAL   PRIMARY KEY,
    uuid          UUID        NOT NULL DEFAULT gen_random_uuid(),
    user_id       BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    legal_text_id BIGINT      NOT NULL REFERENCES legal_texts (id) ON DELETE RESTRICT,
    kind          VARCHAR(32) NOT NULL,
    locale        VARCHAR(8)  NOT NULL,
    text_version  INTEGER     NOT NULL,
    accepted      BOOLEAN     NOT NULL,
    decided_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ip            VARCHAR(64) NULL,
    user_agent    TEXT        NULL,
    CONSTRAINT uq_consents_uuid UNIQUE (uuid),
    CONSTRAINT uq_consents_user_text UNIQUE (user_id, legal_text_id),
    CONSTRAINT chk_consents_kind CHECK (kind IN ('ai_guidelines'))
);

CREATE INDEX idx_consents_user_kind_decided ON consents (user_id, kind, decided_at DESC);

-- 3. Permission: platform.legal_texts.write (catalog entry in
-- internal/platform/rbac), appended after the existing catalog.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Write legal texts', 'platform.legal_texts.write', 'platform', ARRAY['all']::text[], false, false,
       'Edit the portal legal texts (AI guidelines) in Markdown, per language.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'all'
FROM roles r, permissions p
WHERE r.slug = 'super_admin' AND p.slug = 'platform.legal_texts.write'
ON CONFLICT DO NOTHING;
