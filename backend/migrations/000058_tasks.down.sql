-- Reverts TEC-214. Data loss: every center task and its comments.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tasks.read', 'tasks.write'));
DELETE FROM permissions WHERE slug IN ('tasks.read', 'tasks.write');

DROP TABLE IF EXISTS task_comments;
DROP FUNCTION IF EXISTS task_comments_check_row();
DROP TABLE IF EXISTS tasks;
DROP FUNCTION IF EXISTS tasks_check_row();
