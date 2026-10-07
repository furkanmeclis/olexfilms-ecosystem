-- Reverts TEC-395. Messages still queued become failed (the old status set
-- has no queued); attempt counters and failure reasons are dropped.
DROP INDEX IF EXISTS idx_messages_queued;
UPDATE messages SET status = 'failed' WHERE status = 'queued';
ALTER TABLE messages
    DROP CONSTRAINT IF EXISTS chk_messages_failure_reason,
    DROP CONSTRAINT IF EXISTS chk_messages_send_attempts,
    DROP COLUMN IF EXISTS failure_reason,
    DROP COLUMN IF EXISTS send_attempts,
    DROP CONSTRAINT chk_messages_status;
ALTER TABLE messages
    ADD CONSTRAINT chk_messages_status CHECK (status IN ('received', 'sent', 'delivered', 'read', 'failed'));
