CREATE TABLE organizations (
    id                BIGSERIAL PRIMARY KEY,
    uuid              UUID         NOT NULL DEFAULT gen_random_uuid(),
    slug              VARCHAR(64)  NOT NULL,
    name              VARCHAR(200) NOT NULL,
    city              VARCHAR(100) NOT NULL DEFAULT '',
    district          VARCHAR(100) NOT NULL DEFAULT '',
    phone             VARCHAR(32)  NOT NULL DEFAULT '',
    address           TEXT         NOT NULL DEFAULT '',
    logo_object_key   TEXT         NULL,
    status            VARCHAR(32)  NOT NULL DEFAULT 'active',
    plan_code         VARCHAR(32)  NULL DEFAULT 'trial',
    access_starts_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    access_ends_at    TIMESTAMPTZ  NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ  NULL,
    CONSTRAINT uq_organizations_uuid UNIQUE (uuid),
    CONSTRAINT chk_organizations_status CHECK (status IN ('pending', 'active', 'suspended', 'expired'))
);

CREATE UNIQUE INDEX uq_organizations_slug_active
    ON organizations (slug)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_organizations_status
    ON organizations (status)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_organizations_set_updated_at
    BEFORE UPDATE ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE organization_members (
    id              BIGSERIAL PRIMARY KEY,
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role            VARCHAR(32)  NOT NULL DEFAULT 'staff',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_organization_members UNIQUE (organization_id, user_id),
    CONSTRAINT chk_organization_members_role CHECK (role IN ('owner', 'staff'))
);

CREATE INDEX idx_organization_members_user
    ON organization_members (user_id);

INSERT INTO permissions (name, slug) VALUES
    ('Read platform organizations', 'platform.organizations.read'),
    ('Write platform organizations', 'platform.organizations.write');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('platform.organizations.read', 'platform.organizations.write')
WHERE r.slug = 'super_admin';
