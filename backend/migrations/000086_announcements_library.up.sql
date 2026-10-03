-- TEC-329 (F3-05a): announcements and the document library (schema,
-- permissions, sqlc). Endpoints and screens come with the following F3-05
-- items; this migration only fixes the data model and its guards.
--
-- Announcements
--   * announcements: written by a center or a distributor organization
--     (organization_id = author organization, brand_id = its brand). The
--     default language text lives on the row (default_locale, title, body);
--     announcement_locales holds the translations. status draft | published
--     | archived; a published row has a publish_at. pinned and notify are
--     plain flags (notify = send a notification when published).
--   * announcement_audiences: who sees it. target_type all_network |
--     distributors | dealers | role | subtree, with an optional role_slug
--     filter (required for role) and target_organization_id (required for
--     subtree, NULL otherwise). A distributor may only target its own
--     subtree (trigger, check_violation).
--   * announcement_reads: one row per (announcement, user); a second plain
--     insert answers 23505, the application upserts with ON CONFLICT.
--   Body sanitizing (Markdown / HTML) is done by the application before it
--   writes; body_format only records which renderer applies.
--
-- Document library (library_* so it does not clash with the documents
-- module / document_templates)
--   * library_folders: a tree per organization (parent_id in the same
--     organization, no cycles).
--   * library_items: name, description, tags, access_level all_network |
--     distributors | dealers | center_only, optional role_slug filter.
--   * library_item_versions: one file per (item, locale, version_no);
--     append-only like the ledger (UPDATE / DELETE / TRUNCATE rejected).
--
-- role_slug columns are not foreign keys: roles may be renamed or removed
-- and an unknown slug simply matches nobody (fail closed).

-- 1. announcements ---------------------------------------------------------
CREATE TABLE announcements (
    id               BIGSERIAL     PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    default_locale   VARCHAR(8)    NOT NULL DEFAULT 'tr',
    title            VARCHAR(255)  NOT NULL,
    body             TEXT          NOT NULL DEFAULT '',
    body_format      VARCHAR(16)   NOT NULL DEFAULT 'markdown',
    status           VARCHAR(16)   NOT NULL DEFAULT 'draft',
    pinned           BOOLEAN       NOT NULL DEFAULT false,
    notify           BOOLEAN       NOT NULL DEFAULT false,
    publish_at       TIMESTAMPTZ   NULL,
    expires_at       TIMESTAMPTZ   NULL,
    author_user_id   BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_announcements_uuid UNIQUE (uuid),
    CONSTRAINT chk_announcements_status CHECK (status IN ('draft', 'published', 'archived')),
    CONSTRAINT chk_announcements_body_format CHECK (body_format IN ('markdown', 'html')),
    CONSTRAINT chk_announcements_default_locale CHECK (
        default_locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar')
    ),
    CONSTRAINT chk_announcements_title CHECK (btrim(title) <> ''),
    CONSTRAINT chk_announcements_published CHECK (status <> 'published' OR publish_at IS NOT NULL),
    CONSTRAINT chk_announcements_window CHECK (
        expires_at IS NULL OR publish_at IS NULL OR expires_at > publish_at
    )
);

CREATE INDEX idx_announcements_org_created ON announcements (organization_id, created_at DESC);
CREATE INDEX idx_announcements_feed ON announcements (brand_id, pinned DESC, publish_at DESC)
    WHERE status = 'published';

CREATE TRIGGER trg_announcements_set_updated_at
    BEFORE UPDATE ON announcements
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- brand_id must be the brand of the organization (000048 helper).
CREATE TRIGGER trg_announcements_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON announcements
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- Only a center or a distributor writes announcements; the author
-- organization is fixed once written (the audiences were checked against
-- it).
CREATE FUNCTION announcements_check_author() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_type VARCHAR(16);
BEGIN
    IF TG_OP = 'UPDATE' AND NEW.organization_id IS DISTINCT FROM OLD.organization_id THEN
        RAISE EXCEPTION 'announcements: author organization is immutable (announcement %)', OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    SELECT o.type INTO org_type FROM organizations o WHERE o.id = NEW.organization_id;
    IF NOT FOUND THEN
        RETURN NEW; -- the foreign key reports it
    END IF;
    IF org_type NOT IN ('center', 'distributor') THEN
        RAISE EXCEPTION 'announcements: a % organization cannot write announcements', org_type
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_announcements_check_author
    BEFORE INSERT OR UPDATE OF organization_id ON announcements
    FOR EACH ROW
    EXECUTE FUNCTION announcements_check_author();

-- 2. announcement_locales --------------------------------------------------
CREATE TABLE announcement_locales (
    id               BIGSERIAL     PRIMARY KEY,
    announcement_id  BIGINT        NOT NULL REFERENCES announcements (id) ON DELETE CASCADE,
    locale           VARCHAR(8)    NOT NULL,
    title            VARCHAR(255)  NOT NULL,
    body             TEXT          NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_announcement_locales_locale UNIQUE (announcement_id, locale),
    CONSTRAINT chk_announcement_locales_locale CHECK (
        locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar')
    ),
    CONSTRAINT chk_announcement_locales_title CHECK (btrim(title) <> '')
);

CREATE TRIGGER trg_announcement_locales_set_updated_at
    BEFORE UPDATE ON announcement_locales
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 3. announcement_audiences ------------------------------------------------
CREATE TABLE announcement_audiences (
    id                      BIGSERIAL    PRIMARY KEY,
    announcement_id         BIGINT       NOT NULL REFERENCES announcements (id) ON DELETE CASCADE,
    target_type             VARCHAR(16)  NOT NULL,
    role_slug               VARCHAR(100) NULL,
    target_organization_id  BIGINT       NULL REFERENCES organizations (id) ON DELETE CASCADE,
    created_at              TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_announcement_audiences_type CHECK (
        target_type IN ('all_network', 'distributors', 'dealers', 'role', 'subtree')
    ),
    CONSTRAINT chk_announcement_audiences_role CHECK (
        (target_type <> 'role' OR role_slug IS NOT NULL)
        AND (role_slug IS NULL OR btrim(role_slug) <> '')
    ),
    CONSTRAINT chk_announcement_audiences_org CHECK (
        (target_type = 'subtree') = (target_organization_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX uq_announcement_audiences_target ON announcement_audiences (
    announcement_id, target_type, COALESCE(role_slug, ''), COALESCE(target_organization_id, 0)
);
CREATE INDEX idx_announcement_audiences_org ON announcement_audiences (target_organization_id)
    WHERE target_organization_id IS NOT NULL;

-- A distributor targets only its own subtree (itself or an organization
-- below it); a subtree target of a center stays in its brand.
CREATE FUNCTION announcement_audiences_check_target() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    author_org    BIGINT;
    author_brand  BIGINT;
    author_type   VARCHAR(16);
    target_brand  BIGINT;
    in_subtree    BOOLEAN;
BEGIN
    SELECT a.organization_id, a.brand_id, o.type
      INTO author_org, author_brand, author_type
      FROM announcements a
      JOIN organizations o ON o.id = a.organization_id
     WHERE a.id = NEW.announcement_id;
    IF NOT FOUND THEN
        RETURN NEW; -- the foreign key reports it
    END IF;

    IF author_type = 'distributor' AND NEW.target_type <> 'subtree' THEN
        RAISE EXCEPTION 'announcement_audiences: a distributor announcement may only target its subtree, not %',
            NEW.target_type
            USING ERRCODE = 'check_violation';
    END IF;

    IF NEW.target_organization_id IS NOT NULL THEN
        SELECT brand_id INTO target_brand FROM organizations WHERE id = NEW.target_organization_id;
        IF FOUND AND target_brand IS DISTINCT FROM author_brand THEN
            RAISE EXCEPTION 'announcement_audiences: target organization % is outside brand %',
                NEW.target_organization_id, author_brand
                USING ERRCODE = 'check_violation';
        END IF;
        IF author_type = 'distributor' THEN
            WITH RECURSIVE up AS (
                SELECT o.id, o.parent_id, 1 AS depth
                FROM organizations o
                WHERE o.id = NEW.target_organization_id
                UNION ALL
                SELECT p.id, p.parent_id, up.depth + 1
                FROM organizations p
                JOIN up ON p.id = up.parent_id
                WHERE up.depth < 32
            )
            SELECT EXISTS (SELECT 1 FROM up WHERE up.id = author_org) INTO in_subtree;
            IF NOT in_subtree THEN
                RAISE EXCEPTION 'announcement_audiences: organization % is outside the subtree of distributor %',
                    NEW.target_organization_id, author_org
                    USING ERRCODE = 'check_violation';
            END IF;
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_announcement_audiences_check_target
    BEFORE INSERT OR UPDATE ON announcement_audiences
    FOR EACH ROW
    EXECUTE FUNCTION announcement_audiences_check_target();

-- 4. announcement_reads ----------------------------------------------------
CREATE TABLE announcement_reads (
    announcement_id  BIGINT       NOT NULL REFERENCES announcements (id) ON DELETE CASCADE,
    user_id          BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    read_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_announcement_reads_user UNIQUE (announcement_id, user_id)
);

CREATE INDEX idx_announcement_reads_user ON announcement_reads (user_id, announcement_id);

-- 5. library_folders -------------------------------------------------------
CREATE TABLE library_folders (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    parent_id           BIGINT        NULL,
    name                VARCHAR(255)  NOT NULL,
    sort_order          INTEGER       NOT NULL DEFAULT 0,
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ   NULL,
    CONSTRAINT uq_library_folders_uuid UNIQUE (uuid),
    CONSTRAINT uq_library_folders_id_org UNIQUE (id, organization_id),
    CONSTRAINT fk_library_folders_parent FOREIGN KEY (parent_id, organization_id)
        REFERENCES library_folders (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_library_folders_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_library_folders_not_self CHECK (parent_id IS NULL OR parent_id <> id)
);

CREATE UNIQUE INDEX uq_library_folders_sibling_name
    ON library_folders (organization_id, COALESCE(parent_id, 0), lower(name))
    WHERE deleted_at IS NULL;
CREATE INDEX idx_library_folders_parent ON library_folders (parent_id) WHERE parent_id IS NOT NULL;

CREATE TRIGGER trg_library_folders_set_updated_at
    BEFORE UPDATE ON library_folders
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_library_folders_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON library_folders
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- Moving a folder below one of its descendants would close a cycle.
CREATE FUNCTION library_folders_check_cycle() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    cycle BOOLEAN;
BEGIN
    IF NEW.parent_id IS NULL THEN
        RETURN NEW;
    END IF;
    WITH RECURSIVE up AS (
        SELECT f.id, f.parent_id, 1 AS depth
        FROM library_folders f
        WHERE f.id = NEW.parent_id
        UNION ALL
        SELECT p.id, p.parent_id, up.depth + 1
        FROM library_folders p
        JOIN up ON p.id = up.parent_id
        WHERE up.depth < 64
    )
    SELECT EXISTS (SELECT 1 FROM up WHERE up.id = NEW.id) INTO cycle;
    IF cycle THEN
        RAISE EXCEPTION 'library_folders: moving folder % under % closes a cycle', NEW.id, NEW.parent_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_library_folders_check_cycle
    BEFORE UPDATE OF parent_id ON library_folders
    FOR EACH ROW
    EXECUTE FUNCTION library_folders_check_cycle();

-- 6. library_items ---------------------------------------------------------
CREATE TABLE library_items (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    folder_id           BIGINT        NULL,
    name                VARCHAR(255)  NOT NULL,
    description         TEXT          NULL,
    tags                TEXT[]        NOT NULL DEFAULT '{}',
    access_level        VARCHAR(16)   NOT NULL DEFAULT 'center_only',
    role_slug           VARCHAR(100)  NULL,
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ   NULL,
    CONSTRAINT uq_library_items_uuid UNIQUE (uuid),
    CONSTRAINT fk_library_items_folder FOREIGN KEY (folder_id, organization_id)
        REFERENCES library_folders (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_library_items_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_library_items_access_level CHECK (
        access_level IN ('all_network', 'distributors', 'dealers', 'center_only')
    ),
    CONSTRAINT chk_library_items_role CHECK (role_slug IS NULL OR btrim(role_slug) <> ''),
    CONSTRAINT chk_library_items_tags CHECK (array_position(tags, NULL) IS NULL)
);

CREATE INDEX idx_library_items_brand_folder ON library_items (brand_id, folder_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_library_items_org ON library_items (organization_id, created_at DESC);
CREATE INDEX idx_library_items_tags ON library_items USING GIN (tags);

CREATE TRIGGER trg_library_items_set_updated_at
    BEFORE UPDATE ON library_items
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_library_items_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON library_items
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 7. library_item_versions (append-only) -----------------------------------
CREATE TABLE library_item_versions (
    id                   BIGSERIAL     PRIMARY KEY,
    uuid                 UUID          NOT NULL DEFAULT gen_random_uuid(),
    item_id              BIGINT        NOT NULL REFERENCES library_items (id) ON DELETE RESTRICT,
    locale               VARCHAR(8)    NOT NULL,
    version_no           INTEGER       NOT NULL,
    storage_key          TEXT          NOT NULL,
    mime                 VARCHAR(255)  NOT NULL,
    size_bytes           BIGINT        NOT NULL,
    sha256               CHAR(64)      NOT NULL,
    uploaded_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at           TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_library_item_versions_uuid UNIQUE (uuid),
    CONSTRAINT uq_library_item_versions_no UNIQUE (item_id, locale, version_no),
    CONSTRAINT chk_library_item_versions_locale CHECK (
        locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar')
    ),
    CONSTRAINT chk_library_item_versions_no CHECK (version_no > 0),
    CONSTRAINT chk_library_item_versions_storage_key CHECK (btrim(storage_key) <> ''),
    CONSTRAINT chk_library_item_versions_mime CHECK (btrim(mime) <> ''),
    CONSTRAINT chk_library_item_versions_size CHECK (size_bytes >= 0),
    CONSTRAINT chk_library_item_versions_sha256 CHECK (sha256 ~ '^[0-9a-f]{64}$')
);

CREATE FUNCTION library_item_versions_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'library_item_versions is append-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$;

CREATE TRIGGER trg_library_item_versions_append_only
    BEFORE UPDATE OR DELETE ON library_item_versions
    FOR EACH ROW
    EXECUTE FUNCTION library_item_versions_append_only();

CREATE TRIGGER trg_library_item_versions_no_truncate
    BEFORE TRUNCATE ON library_item_versions
    FOR EACH STATEMENT
    EXECUTE FUNCTION library_item_versions_append_only();

-- 8. Permissions. Source of truth: internal/platform/rbac/catalog.go
-- (appended last, sort_order MAX+10). Every network role reads
-- announcements and the library in its own organization (managed); the
-- center writes both for its brand, the distributor owner writes
-- announcements for its subtree.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read announcements', 'announcements.read', 'announcements', ARRAY['managed']::text[], false, false,
       'Read the announcements published to the organization and mark them as read.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Write announcements', 'announcements.write', 'announcements',
       ARRAY['subtree', 'brand', 'all']::text[], false, false,
       'Write, publish and archive announcements (center: the brand network; distributor: its subtree).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read the document library', 'library.read', 'library', ARRAY['managed']::text[], false, false,
       'Browse and download the guides and materials of the document library allowed to the organization.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage the document library', 'library.manage', 'library', ARRAY['brand', 'all']::text[], false, false,
       'Create folders, upload documents and new language versions, set access levels (center only).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'announcements.read', 'managed'),
    ('super_admin', 'announcements.write', 'all'),
    ('super_admin', 'library.read', 'managed'),
    ('super_admin', 'library.manage', 'all'),
    ('center_staff', 'announcements.read', 'managed'),
    ('center_staff', 'announcements.write', 'brand'),
    ('center_staff', 'library.read', 'managed'),
    ('center_staff', 'library.manage', 'brand'),
    ('center_warehouse', 'announcements.read', 'managed'),
    ('center_warehouse', 'library.read', 'managed'),
    ('center_accounting', 'announcements.read', 'managed'),
    ('center_accounting', 'library.read', 'managed'),
    ('center_social', 'announcements.read', 'managed'),
    ('center_social', 'announcements.write', 'brand'),
    ('center_social', 'library.read', 'managed'),
    ('center_social', 'library.manage', 'brand'),
    ('distributor_owner', 'announcements.read', 'managed'),
    ('distributor_owner', 'announcements.write', 'subtree'),
    ('distributor_owner', 'library.read', 'managed'),
    ('distributor_staff', 'announcements.read', 'managed'),
    ('distributor_staff', 'library.read', 'managed'),
    ('distributor_warehouse_staff', 'announcements.read', 'managed'),
    ('distributor_warehouse_staff', 'library.read', 'managed'),
    ('distributor_accounting', 'announcements.read', 'managed'),
    ('distributor_accounting', 'library.read', 'managed'),
    ('dealer_owner', 'announcements.read', 'managed'),
    ('dealer_owner', 'library.read', 'managed'),
    ('dealer_staff', 'announcements.read', 'managed'),
    ('dealer_staff', 'library.read', 'managed'),
    ('dealer_accounting', 'announcements.read', 'managed'),
    ('dealer_accounting', 'library.read', 'managed')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
