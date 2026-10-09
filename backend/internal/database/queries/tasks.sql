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
    due_at = sqlc.narg(due_at)::timestamptz,
    -- TEC-221: a new deadline is reminded again.
    due_soon_notified_at = CASE WHEN due_at IS DISTINCT FROM sqlc.narg(due_at)::timestamptz
                                THEN NULL ELSE due_soon_notified_at END,
    overdue_notified_at = CASE WHEN due_at IS DISTINCT FROM sqlc.narg(due_at)::timestamptz
                               THEN NULL ELSE overdue_notified_at END,
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

-- TEC-379 (DT-BE-8): list contract (docs/list-contract.md). status and
-- priority sort by their rank (open → cancelled, low → urgent), subject by
-- the subject organization name; due_at keeps tasks without a deadline
-- last in both directions; id is the tiebreak. q matches the title or the
-- description (the caller escapes LIKE wildcards).
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
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR t.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(priorities)::text[]), 0) = 0 OR t.priority = ANY (sqlc.narg(priorities)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(sources)::text[]), 0) = 0 OR t.source = ANY (sqlc.narg(sources)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(subject_org_uuids)::uuid[]), 0) = 0
       OR t.subject_org_id IN (SELECT so.id FROM organizations so
                               WHERE so.uuid = ANY (sqlc.narg(subject_org_uuids)::uuid[])))
  AND (sqlc.narg(assignee_user_id)::bigint IS NULL OR t.assignee_user_id = sqlc.narg(assignee_user_id)::bigint)
  AND (sqlc.narg(due_after)::timestamptz IS NULL OR t.due_at >= sqlc.narg(due_after)::timestamptz)
  AND (sqlc.narg(due_before)::timestamptz IS NULL OR t.due_at < sqlc.narg(due_before)::timestamptz)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR t.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR t.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR t.title ILIKE '%' || sqlc.narg(q)::text || '%'
       OR t.description ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text
    WHEN 'title' THEN lower(t.title) WHEN 'subject' THEN lower(s.name) END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text
    WHEN 'title' THEN lower(t.title) WHEN 'subject' THEN lower(s.name) END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text
    WHEN 'status' THEN CASE t.status WHEN 'open' THEN 1 WHEN 'in_progress' THEN 2 WHEN 'done' THEN 3 ELSE 4 END
    WHEN 'priority' THEN CASE t.priority WHEN 'low' THEN 1 WHEN 'normal' THEN 2 WHEN 'high' THEN 3 ELSE 4 END
  END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text
    WHEN 'status' THEN CASE t.status WHEN 'open' THEN 1 WHEN 'in_progress' THEN 2 WHEN 'done' THEN 3 ELSE 4 END
    WHEN 'priority' THEN CASE t.priority WHEN 'low' THEN 1 WHEN 'normal' THEN 2 WHEN 'high' THEN 3 ELSE 4 END
  END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text
    WHEN 'created_at' THEN t.created_at WHEN 'updated_at' THEN t.updated_at END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text
    WHEN 'created_at' THEN t.created_at WHEN 'updated_at' THEN t.updated_at END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'due_at' THEN t.due_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'due_at' THEN t.due_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN t.id END DESC,
  t.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountTasks :one
SELECT COUNT(*)::bigint FROM tasks t
WHERE t.brand_id = sqlc.arg(brand_id)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR t.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(priorities)::text[]), 0) = 0 OR t.priority = ANY (sqlc.narg(priorities)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(sources)::text[]), 0) = 0 OR t.source = ANY (sqlc.narg(sources)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(subject_org_uuids)::uuid[]), 0) = 0
       OR t.subject_org_id IN (SELECT so.id FROM organizations so
                               WHERE so.uuid = ANY (sqlc.narg(subject_org_uuids)::uuid[])))
  AND (sqlc.narg(assignee_user_id)::bigint IS NULL OR t.assignee_user_id = sqlc.narg(assignee_user_id)::bigint)
  AND (sqlc.narg(due_after)::timestamptz IS NULL OR t.due_at >= sqlc.narg(due_after)::timestamptz)
  AND (sqlc.narg(due_before)::timestamptz IS NULL OR t.due_at < sqlc.narg(due_before)::timestamptz)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR t.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR t.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR t.title ILIKE '%' || sqlc.narg(q)::text || '%'
       OR t.description ILIKE '%' || sqlc.narg(q)::text || '%');

-- TEC-379: "select all matching" of the task bulk actions (same filters).
-- name: ListTaskUUIDsFiltered :many
SELECT t.uuid FROM tasks t
WHERE t.brand_id = sqlc.arg(brand_id)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR t.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(priorities)::text[]), 0) = 0 OR t.priority = ANY (sqlc.narg(priorities)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(sources)::text[]), 0) = 0 OR t.source = ANY (sqlc.narg(sources)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(subject_org_uuids)::uuid[]), 0) = 0
       OR t.subject_org_id IN (SELECT so.id FROM organizations so
                               WHERE so.uuid = ANY (sqlc.narg(subject_org_uuids)::uuid[])))
  AND (sqlc.narg(assignee_user_id)::bigint IS NULL OR t.assignee_user_id = sqlc.narg(assignee_user_id)::bigint)
  AND (sqlc.narg(due_after)::timestamptz IS NULL OR t.due_at >= sqlc.narg(due_after)::timestamptz)
  AND (sqlc.narg(due_before)::timestamptz IS NULL OR t.due_at < sqlc.narg(due_before)::timestamptz)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR t.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR t.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR t.title ILIKE '%' || sqlc.narg(q)::text || '%'
       OR t.description ILIKE '%' || sqlc.narg(q)::text || '%')
  AND t.organization_id = sqlc.arg(organization_id)
ORDER BY t.created_at DESC, t.id DESC
LIMIT sqlc.arg(row_limit);

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

-- TEC-379 (DT-BE-8): bulk set_status / set_priority. Closing stamps
-- closed_at (chk_tasks_closed); reopening clears closed_at and closed_by.
-- name: SetTaskStatus :one
UPDATE tasks
SET status = sqlc.arg(status)::text,
    closed_at = CASE WHEN sqlc.arg(status)::text IN ('done', 'cancelled')
                     THEN COALESCE(sqlc.narg(closed_at)::timestamptz, closed_at, NOW()) END,
    closed_by_user_id = CASE WHEN sqlc.arg(status)::text IN ('done', 'cancelled')
                             THEN sqlc.narg(closed_by_user_id)::bigint END
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetTaskPriority :one
UPDATE tasks
SET priority = sqlc.arg(priority)
WHERE id = sqlc.arg(id)
RETURNING *;

-- TEC-221: assignee picker of the task form (members of the center).
-- name: ListCenterMembers :many
SELECT u.uuid, u.name, u.surname FROM users u
JOIN organization_members m ON m.user_id = u.id
WHERE m.organization_id = sqlc.arg(organization_id) AND u.deleted_at IS NULL
ORDER BY u.name, u.surname, u.id;

-- TEC-221: tasks:due_scan. Each claim stamps one threshold on a page of
-- open tasks and returns what the reminder needs; a stamped task leaves the
-- candidate list, so a second run finds nothing (SKIP LOCKED keeps two
-- runs apart). Tasks with nobody to notify (no assignee, no creator) are
-- never claimed. The overdue pass also stamps due_soon so a missed run does
-- not send a stale "due soon" after the deadline.
-- name: ClaimTasksOverdue :many
WITH picked AS (
    SELECT id FROM tasks
    WHERE status IN ('open', 'in_progress') AND due_at IS NOT NULL
      AND due_at <= sqlc.arg(now)::timestamptz
      AND overdue_notified_at IS NULL
      AND (assignee_user_id IS NOT NULL OR created_by_user_id IS NOT NULL)
    ORDER BY due_at, id
    LIMIT sqlc.arg(row_limit)
    FOR UPDATE SKIP LOCKED
)
UPDATE tasks t
SET overdue_notified_at = sqlc.arg(now)::timestamptz,
    due_soon_notified_at = COALESCE(t.due_soon_notified_at, sqlc.arg(now)::timestamptz)
FROM picked, organizations o, organizations s
WHERE t.id = picked.id AND o.id = t.organization_id AND s.id = t.subject_org_id
RETURNING t.id, t.uuid, t.organization_id, t.brand_id, t.title, t.priority, t.due_at,
          t.assignee_user_id, t.created_by_user_id,
          o.timezone AS timezone, s.name AS subject_name;

-- name: ClaimTasksDueSoon :many
WITH picked AS (
    SELECT id FROM tasks
    WHERE status IN ('open', 'in_progress') AND due_at IS NOT NULL
      AND due_at > sqlc.arg(now)::timestamptz
      AND due_at <= sqlc.arg(horizon)::timestamptz
      AND due_soon_notified_at IS NULL AND overdue_notified_at IS NULL
      AND (assignee_user_id IS NOT NULL OR created_by_user_id IS NOT NULL)
    ORDER BY due_at, id
    LIMIT sqlc.arg(row_limit)
    FOR UPDATE SKIP LOCKED
)
UPDATE tasks t
SET due_soon_notified_at = sqlc.arg(now)::timestamptz
FROM picked, organizations o, organizations s
WHERE t.id = picked.id AND o.id = t.organization_id AND s.id = t.subject_org_id
RETURNING t.id, t.uuid, t.organization_id, t.brand_id, t.title, t.priority, t.due_at,
          t.assignee_user_id, t.created_by_user_id,
          o.timezone AS timezone, s.name AS subject_name;
