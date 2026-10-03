-- TEC-214 (F1-11a): center tasks (tasks, task_comments). Every read is
-- bounded by the brand of the active center organization.

-- name: InsertTask :one
INSERT INTO tasks (
    organization_id, brand_id, subject_org_id, title, description,
    assignee_user_id, priority, due_at, source, created_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(subject_org_id), sqlc.arg(title),
    sqlc.arg(description), sqlc.narg(assignee_user_id), sqlc.arg(priority), sqlc.narg(due_at),
    sqlc.arg(source), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: LockTask :one
SELECT * FROM tasks
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id)
FOR UPDATE;

-- name: GetTaskByUUID :one
SELECT * FROM tasks
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: UpdateTask :one
UPDATE tasks
SET subject_org_id = sqlc.arg(subject_org_id),
    title = sqlc.arg(title),
    description = sqlc.arg(description),
    assignee_user_id = sqlc.narg(assignee_user_id),
    priority = sqlc.arg(priority),
    due_at = sqlc.narg(due_at),
    status = sqlc.arg(status),
    closed_at = sqlc.narg(closed_at),
    closed_by_user_id = sqlc.narg(closed_by_user_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: GetTaskView :one
SELECT t.*,
       s.uuid AS subject_uuid, s.name AS subject_name, s.type AS subject_type,
       a.uuid AS assignee_uuid, a.name AS assignee_name, a.surname AS assignee_surname,
       c.uuid AS creator_uuid, c.name AS creator_name, c.surname AS creator_surname,
       (SELECT COUNT(*) FROM task_comments tc WHERE tc.task_id = t.id)::bigint AS comment_count
FROM tasks t
JOIN organizations s ON s.id = t.subject_org_id
LEFT JOIN users a ON a.id = t.assignee_user_id
LEFT JOIN users c ON c.id = t.created_by_user_id
WHERE t.uuid = sqlc.arg(uuid) AND t.brand_id = sqlc.arg(brand_id);

-- name: ListTasks :many
SELECT t.*,
       s.uuid AS subject_uuid, s.name AS subject_name, s.type AS subject_type,
       a.uuid AS assignee_uuid, a.name AS assignee_name, a.surname AS assignee_surname,
       c.uuid AS creator_uuid, c.name AS creator_name, c.surname AS creator_surname,
       (SELECT COUNT(*) FROM task_comments tc WHERE tc.task_id = t.id)::bigint AS comment_count
FROM tasks t
JOIN organizations s ON s.id = t.subject_org_id
LEFT JOIN users a ON a.id = t.assignee_user_id
LEFT JOIN users c ON c.id = t.created_by_user_id
WHERE t.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(status)::text IS NULL OR t.status = sqlc.narg(status)::text)
  AND (NOT sqlc.arg(only_open)::boolean OR t.status IN ('open', 'in_progress'))
  AND (sqlc.narg(priority)::text IS NULL OR t.priority = sqlc.narg(priority)::text)
  AND (sqlc.narg(subject_org_id)::bigint IS NULL OR t.subject_org_id = sqlc.narg(subject_org_id)::bigint)
  AND (sqlc.narg(assignee_user_id)::bigint IS NULL OR t.assignee_user_id = sqlc.narg(assignee_user_id)::bigint)
ORDER BY t.created_at DESC, t.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountTasks :one
SELECT COUNT(*)::bigint FROM tasks t
WHERE t.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(status)::text IS NULL OR t.status = sqlc.narg(status)::text)
  AND (NOT sqlc.arg(only_open)::boolean OR t.status IN ('open', 'in_progress'))
  AND (sqlc.narg(priority)::text IS NULL OR t.priority = sqlc.narg(priority)::text)
  AND (sqlc.narg(subject_org_id)::bigint IS NULL OR t.subject_org_id = sqlc.narg(subject_org_id)::bigint)
  AND (sqlc.narg(assignee_user_id)::bigint IS NULL OR t.assignee_user_id = sqlc.narg(assignee_user_id)::bigint);

-- name: InsertTaskComment :one
INSERT INTO task_comments (task_id, organization_id, brand_id, author_user_id, body)
VALUES (sqlc.arg(task_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.narg(author_user_id), sqlc.arg(body))
RETURNING *;

-- name: ListTaskComments :many
SELECT tc.*, u.uuid AS author_uuid, u.name AS author_name, u.surname AS author_surname
FROM task_comments tc
LEFT JOIN users u ON u.id = tc.author_user_id
WHERE tc.task_id = sqlc.arg(task_id)
ORDER BY tc.created_at, tc.id
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountTaskComments :one
SELECT COUNT(*)::bigint FROM task_comments WHERE task_id = sqlc.arg(task_id);

-- An organization of the brand by uuid (the use case checks the type).
-- name: GetTaskSubjectOrg :one
SELECT id, uuid, name, type, brand_id FROM organizations
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id) AND deleted_at IS NULL;

-- A user that is a member of the given (center) organization.
-- name: GetCenterMemberByUUID :one
SELECT u.id, u.uuid, u.name, u.surname FROM users u
JOIN organization_members m ON m.user_id = u.id
WHERE u.uuid = sqlc.arg(uuid) AND m.organization_id = sqlc.arg(organization_id) AND u.deleted_at IS NULL;

-- TEC-212: bulk engine adapter (assign one task, logged + undoable).
-- name: SetTaskAssignee :one
UPDATE tasks
SET assignee_user_id = sqlc.narg(assignee_user_id)
WHERE id = sqlc.arg(id)
RETURNING *;
