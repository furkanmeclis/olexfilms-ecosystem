-- TEC-249 (F2-04d): short URL service, /s/{token}.
--
-- A short URL points at an internal path of the brand's frontend (K3):
-- /portal, /garanti or /bayi. There is no external target, so the public
-- resolver can never be used as an open redirect. The allowlist lives in
-- the usecase (shorturls.Create); the CHECK below only keeps a stored
-- target a same-origin absolute path ("/x", never "//host" or "/\host").
--
-- token: 4..16 case-sensitive base62 characters. New tokens are 10
-- characters; the old hub's short_urls tokens (Laravel Str::random(8) in a
-- VARCHAR(16), served at /_/{token}) fit the same column and are kept
-- unchanged by the migrator (TEC-263), so old links keep resolving.
--
-- uuid is the public id migration_map.target_uuid points at (TEC-263
-- idempotency). legacy_target_url keeps the hub's original absolute URL of
-- a migrated row for audit only; it is never used as a redirect target.
--
-- organization_id is optional (brand-wide links such as the warranty
-- page); brand_id is required: a token resolves only on its brand's domains.
CREATE TABLE short_urls (
    id                BIGSERIAL     PRIMARY KEY,
    uuid              UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id   BIGINT        NULL REFERENCES organizations (id) ON DELETE SET NULL,
    brand_id          BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    token             VARCHAR(16)   NOT NULL,
    target_path       VARCHAR(2048) NOT NULL,
    legacy_target_url VARCHAR(2048) NULL,
    expires_at        TIMESTAMPTZ   NULL,
    created_by        BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    hit_count         BIGINT        NOT NULL DEFAULT 0,
    last_hit_at       TIMESTAMPTZ   NULL,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_short_urls_uuid UNIQUE (uuid),
    CONSTRAINT uq_short_urls_token UNIQUE (token),
    CONSTRAINT chk_short_urls_token CHECK (token ~ '^[A-Za-z0-9]{4,16}$'),
    CONSTRAINT chk_short_urls_target_path CHECK (target_path ~ '^/[^/\\]' AND target_path !~ '[[:space:]]'),
    CONSTRAINT chk_short_urls_hit_count CHECK (hit_count >= 0)
);

CREATE INDEX idx_short_urls_brand ON short_urls (brand_id);
CREATE INDEX idx_short_urls_organization ON short_urls (organization_id) WHERE organization_id IS NOT NULL;
CREATE INDEX idx_short_urls_expires_at ON short_urls (expires_at) WHERE expires_at IS NOT NULL;
