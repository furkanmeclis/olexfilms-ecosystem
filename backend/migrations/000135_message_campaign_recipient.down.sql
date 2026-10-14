-- campaign_events is append-only: the rows of the removed type go with the
-- trigger off.
ALTER TABLE campaign_events DISABLE TRIGGER trg_campaign_events_append_only;
DELETE FROM campaign_events WHERE event_type = 'delivery_failed';
ALTER TABLE campaign_events ENABLE TRIGGER trg_campaign_events_append_only;
ALTER TABLE campaign_events DROP CONSTRAINT chk_campaign_events_type;
ALTER TABLE campaign_events ADD CONSTRAINT chk_campaign_events_type CHECK (event_type IN (
    'submitted', 'approved', 'rejected', 'changes_requested', 'scheduled',
    'cancelled', 'started', 'finished'));

DROP INDEX IF EXISTS idx_messages_campaign_recipient;
ALTER TABLE messages DROP COLUMN IF EXISTS campaign_recipient_id;
