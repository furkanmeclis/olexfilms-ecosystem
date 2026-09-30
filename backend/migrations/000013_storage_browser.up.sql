CREATE TABLE storage_stars (
    id         BIGSERIAL PRIMARY KEY,
    uuid       UUID         NOT NULL DEFAULT gen_random_uuid(),
    user_id    BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    object_key TEXT         NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT storage_stars_uuid_uq UNIQUE (uuid),
    CONSTRAINT storage_stars_user_key_uq UNIQUE (user_id, object_key)
);

CREATE INDEX storage_stars_key_idx ON storage_stars (object_key);

CREATE TABLE storage_trash (
    id           BIGSERIAL PRIMARY KEY,
    uuid         UUID         NOT NULL DEFAULT gen_random_uuid(),
    object_key   TEXT         NOT NULL,
    original_key TEXT         NOT NULL,
    name         TEXT         NOT NULL,
    size_bytes   BIGINT       NOT NULL DEFAULT 0,
    mime_type    TEXT         NOT NULL DEFAULT '',
    deleted_by   BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    deleted_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ  NOT NULL,
    CONSTRAINT storage_trash_uuid_uq UNIQUE (uuid)
);

CREATE INDEX storage_trash_expires_idx ON storage_trash (expires_at);
CREATE INDEX storage_trash_original_idx ON storage_trash (original_key);

CREATE TABLE storage_links (
    id           BIGSERIAL PRIMARY KEY,
    uuid         UUID         NOT NULL DEFAULT gen_random_uuid(),
    object_key   TEXT         NOT NULL,
    kind         TEXT         NOT NULL,
    slug         TEXT         NULL,
    token_hash   TEXT         NULL,
    expires_at   TIMESTAMPTZ  NULL,
    can_view     BOOLEAN      NOT NULL DEFAULT TRUE,
    can_download BOOLEAN      NOT NULL DEFAULT TRUE,
    can_upload   BOOLEAN      NOT NULL DEFAULT FALSE,
    created_by   BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    revoked_at   TIMESTAMPTZ  NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT storage_links_uuid_uq UNIQUE (uuid),
    CONSTRAINT storage_links_kind_chk CHECK (kind IN ('public', 'signed')),
    CONSTRAINT storage_links_slug_uq UNIQUE (slug)
);

CREATE INDEX storage_links_key_idx ON storage_links (object_key);
CREATE INDEX storage_links_token_idx ON storage_links (token_hash)
    WHERE token_hash IS NOT NULL;

CREATE TABLE storage_shares (
    id         BIGSERIAL PRIMARY KEY,
    uuid       UUID         NOT NULL DEFAULT gen_random_uuid(),
    object_key TEXT         NOT NULL,
    user_id    BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role       TEXT         NOT NULL,
    created_by BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT storage_shares_uuid_uq UNIQUE (uuid),
    CONSTRAINT storage_shares_key_user_uq UNIQUE (object_key, user_id),
    CONSTRAINT storage_shares_role_chk CHECK (role IN ('viewer', 'editor', 'owner'))
);

CREATE INDEX storage_shares_user_idx ON storage_shares (user_id);
CREATE INDEX storage_shares_key_idx ON storage_shares (object_key);

CREATE TABLE storage_activity (
    id            BIGSERIAL PRIMARY KEY,
    uuid          UUID         NOT NULL DEFAULT gen_random_uuid(),
    object_key    TEXT         NOT NULL,
    actor_user_id BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    action        TEXT         NOT NULL,
    payload       JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT storage_activity_uuid_uq UNIQUE (uuid)
);

CREATE INDEX storage_activity_key_idx ON storage_activity (object_key, created_at DESC);
