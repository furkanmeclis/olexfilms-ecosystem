-- TEC-214 (F1-11a): center tasks. The center of a brand tracks follow-ups
-- about its distributors and dealers: a task is owned by the center
-- organization (organization_id), is about one distributor or dealer of the
-- same brand (subject_org_id) and is assigned to a member of the center
-- (assignee_user_id). Only center roles hold tasks.* (seeded below).
--
-- source keeps room for the F5 performance panel (TEC-130), which opens
-- tasks automatically (source = 'auto'); this migration's API opens manual
-- tasks only.

CREATE TABLE tasks (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    subject_org_id      BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    title               VARCHAR(200)  NOT NULL,
    description         TEXT          NOT NULL DEFAULT '',
    assignee_user_id    BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    priority            VARCHAR(16)   NOT NULL DEFAULT 'normal',
    due_at              TIMESTAMPTZ   NULL,
    status              VARCHAR(16)   NOT NULL DEFAULT 'open',
    source              VARCHAR(16)   NOT NULL DEFAULT 'manual',
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    closed_by_user_id   BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    closed_at           TIMESTAMPTZ   NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_tasks_uuid UNIQUE (uuid),
    CONSTRAINT chk_tasks_title CHECK (btrim(title) <> ''),
    CONSTRAINT chk_tasks_description CHECK (char_length(description) <= 10000),
    CONSTRAINT chk_tasks_priority CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
    CONSTRAINT chk_tasks_status CHECK (status IN ('open', 'in_progress', 'done', 'cancelled')),
    CONSTRAINT chk_tasks_source CHECK (source IN ('manual', 'auto')),
    CONSTRAINT chk_tasks_not_self CHECK (subject_org_id <> organization_id),
    CONSTRAINT chk_tasks_closed CHECK ((status IN ('done', 'cancelled')) = (closed_at IS NOT NULL))
);

CREATE INDEX idx_tasks_org_created ON tasks (organization_id, created_at);
CREATE INDEX idx_tasks_brand_status ON tasks (brand_id, status, created_at);
CREATE INDEX idx_tasks_subject ON tasks (subject_org_id, created_at);
CREATE INDEX idx_tasks_assignee_open ON tasks (assignee_user_id, due_at)
    WHERE status IN ('open', 'in_progress');

CREATE TRIGGER trg_tasks_set_updated_at
    BEFORE UPDATE ON tasks
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Row guard: the owner is the center of the task's brand; the subject is a
-- distributor or dealer of that brand; the assignee is a member of the
-- owning center. Identity columns never change.
CREATE FUNCTION tasks_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    o_type          TEXT;
    o_brand         BIGINT;
    check_subject   BOOLEAN := true;
    check_assignee  BOOLEAN := true;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF NEW.uuid <> OLD.uuid
           OR NEW.organization_id <> OLD.organization_id
           OR NEW.brand_id <> OLD.brand_id
           OR NEW.source <> OLD.source
           OR NEW.created_by_user_id IS DISTINCT FROM OLD.created_by_user_id
           OR NEW.created_at <> OLD.created_at THEN
            RAISE EXCEPTION 'tasks: identity of task % cannot change', OLD.id
                USING ERRCODE = 'check_violation';
        END IF;
        check_subject := NEW.subject_org_id <> OLD.subject_org_id;
        check_assignee := NEW.assignee_user_id IS DISTINCT FROM OLD.assignee_user_id;
    ELSE
        SELECT type, brand_id INTO o_type, o_brand FROM organizations WHERE id = NEW.organization_id;
        IF FOUND AND (o_type IS DISTINCT FROM 'center' OR o_brand IS DISTINCT FROM NEW.brand_id) THEN
            RAISE EXCEPTION 'tasks: owner % must be the center organization of brand %',
                NEW.organization_id, NEW.brand_id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;

    IF check_subject THEN
        SELECT type, brand_id INTO o_type, o_brand FROM organizations WHERE id = NEW.subject_org_id;
        IF FOUND AND (o_type NOT IN ('distributor', 'dealer') OR o_brand IS DISTINCT FROM NEW.brand_id) THEN
            RAISE EXCEPTION 'tasks: subject % must be a distributor or dealer of brand %',
                NEW.subject_org_id, NEW.brand_id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;

    IF check_assignee AND NEW.assignee_user_id IS NOT NULL THEN
        IF NOT EXISTS (
            SELECT 1 FROM organization_members m
            WHERE m.organization_id = NEW.organization_id AND m.user_id = NEW.assignee_user_id
        ) THEN
            RAISE EXCEPTION 'tasks: assignee % is not a member of center %',
                NEW.assignee_user_id, NEW.organization_id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_tasks_check_row
    BEFORE INSERT OR UPDATE ON tasks
    FOR EACH ROW
    EXECUTE FUNCTION tasks_check_row();

-- Comments are append-only notes on a task; organization_id and brand_id
-- follow the task.
CREATE TABLE task_comments (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    task_id          BIGINT       NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    organization_id  BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    author_user_id   BIGINT       NULL REFERENCES users (id) ON DELETE RESTRICT,
    body             TEXT         NOT NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_task_comments_uuid UNIQUE (uuid),
    CONSTRAINT chk_task_comments_body CHECK (btrim(body) <> '' AND char_length(body) <= 4000)
);

CREATE INDEX idx_task_comments_task_created ON task_comments (task_id, created_at);
CREATE INDEX idx_task_comments_org_created ON task_comments (organization_id, created_at);

CREATE FUNCTION task_comments_check_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    t_org    BIGINT;
    t_brand  BIGINT;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION 'task_comments: comment % is append-only', OLD.id
            USING ERRCODE = 'restrict_violation';
    END IF;
    SELECT organization_id, brand_id INTO t_org, t_brand FROM tasks WHERE id = NEW.task_id;
    IF FOUND AND (t_org <> NEW.organization_id OR t_brand <> NEW.brand_id) THEN
        RAISE EXCEPTION 'task_comments: organization/brand of comment must follow task %', NEW.task_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_task_comments_check_row
    BEFORE INSERT OR UPDATE ON task_comments
    FOR EACH ROW
    EXECUTE FUNCTION task_comments_check_row();

-- Permissions. Source of truth: internal/platform/rbac/catalog.go
-- (appended last). Center roles only, at brand scope; super_admin at all.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read tasks', 'tasks.read', 'tasks', ARRAY['brand', 'all']::text[], false, false,
       'Center tasks about distributors and dealers of the brand, with their comments.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Write tasks', 'tasks.write', 'tasks', ARRAY['brand', 'all']::text[], false, false,
       'Create, assign, update and close center tasks; comment on them (center only).',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'tasks.read', 'all'),
    ('super_admin', 'tasks.write', 'all'),
    ('center_staff', 'tasks.read', 'brand'),
    ('center_staff', 'tasks.write', 'brand'),
    ('center_warehouse', 'tasks.read', 'brand'),
    ('center_warehouse', 'tasks.write', 'brand'),
    ('center_accounting', 'tasks.read', 'brand'),
    ('center_accounting', 'tasks.write', 'brand'),
    ('center_social', 'tasks.read', 'brand'),
    ('center_social', 'tasks.write', 'brand')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
