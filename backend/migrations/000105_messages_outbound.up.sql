-- TEC-395 (F4-02d): WhatsApp outgoing message queue. An outgoing AI, staff or
-- system message is stored first as status = queued and sent by the
-- whatsapp:send task (idempotency key: messages.uuid, also the provider's
-- client message id in external_id).
--
--   * status gains 'queued' (before the provider accepted the message).
--   * send_attempts counts provider calls (first try + retries).
--   * failure_reason keeps the last provider error; a failed message keeps
--     it, a sent one clears it.
ALTER TABLE messages DROP CONSTRAINT chk_messages_status;
ALTER TABLE messages
    ADD CONSTRAINT chk_messages_status CHECK (status IN ('queued', 'received', 'sent', 'delivered', 'read', 'failed')),
    ADD COLUMN send_attempts  SMALLINT NOT NULL DEFAULT 0,
    ADD COLUMN failure_reason TEXT     NULL,
    ADD CONSTRAINT chk_messages_send_attempts CHECK (send_attempts >= 0),
    ADD CONSTRAINT chk_messages_failure_reason CHECK (failure_reason IS NULL OR char_length(failure_reason) <= 1000);

-- Stale queue sweep: outgoing messages still waiting for the provider.
CREATE INDEX idx_messages_queued ON messages (created_at) WHERE status = 'queued';
