-- TEC-91: QR web sign-in challenges.

-- name: CreateQRLoginChallenge :one
INSERT INTO qr_login_challenges (code, secret_hash, brand_id, web_ip, web_user_agent, expires_at)
VALUES (sqlc.arg(code), sqlc.arg(secret_hash), sqlc.narg(brand_id), sqlc.narg(web_ip), sqlc.narg(web_user_agent), sqlc.arg(expires_at))
RETURNING *;

-- name: GetQRLoginChallengeByCode :one
SELECT *
FROM qr_login_challenges
WHERE code = $1;

-- name: MarkQRLoginChallengeScanned :one
UPDATE qr_login_challenges
SET status = 'scanned', scanned_at = NOW()
WHERE code = $1
  AND status = 'pending'
  AND expires_at > NOW()
RETURNING *;

-- name: DecideQRLoginChallenge :one
UPDATE qr_login_challenges
SET status = sqlc.arg(status),
    decided_at = NOW(),
    user_id = sqlc.narg(user_id),
    organization_id = sqlc.narg(organization_id),
    approver_session_id = sqlc.narg(approver_session_id),
    approver_device = sqlc.narg(approver_device)
WHERE code = sqlc.arg(code)
  AND status IN ('pending', 'scanned')
  AND expires_at > NOW()
RETURNING *;

-- name: ConsumeQRLoginChallenge :one
UPDATE qr_login_challenges
SET status = 'consumed', consumed_at = NOW()
WHERE code = $1
  AND status = 'approved'
  AND expires_at > NOW()
RETURNING *;

-- name: DeleteStaleQRLoginChallenges :execrows
DELETE FROM qr_login_challenges
WHERE expires_at < NOW() - INTERVAL '1 day';
