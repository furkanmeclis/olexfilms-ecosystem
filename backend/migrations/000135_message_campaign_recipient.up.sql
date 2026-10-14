-- TEC-635: a campaign WhatsApp message knows its campaign recipient, so a
-- send that fails after the hand-off moves the recipient to failed.
ALTER TABLE messages
    ADD COLUMN campaign_recipient_id BIGINT NULL REFERENCES campaign_recipients (id) ON DELETE SET NULL;

CREATE INDEX idx_messages_campaign_recipient ON messages (campaign_recipient_id)
    WHERE campaign_recipient_id IS NOT NULL;

-- A sent campaign whose WhatsApp message failed later moves to
-- partially_failed with a delivery_failed event.
ALTER TABLE campaign_events DROP CONSTRAINT chk_campaign_events_type;
ALTER TABLE campaign_events ADD CONSTRAINT chk_campaign_events_type CHECK (event_type IN (
    'submitted', 'approved', 'rejected', 'changes_requested', 'scheduled',
    'cancelled', 'started', 'finished', 'delivery_failed'));
