-- lead_events is append-only (UPDATE/DELETE are rejected by trigger), so
-- task_created rows cannot be rewritten; the down step refuses while any exist.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM lead_events WHERE event_type = 'task_created') THEN
        RAISE EXCEPTION 'lead_events has task_created rows; cannot narrow chk_lead_events_type';
    END IF;
END $$;
ALTER TABLE lead_events DROP CONSTRAINT chk_lead_events_type;
ALTER TABLE lead_events ADD CONSTRAINT chk_lead_events_type CHECK (
    event_type IN ('created', 'status_changed', 'note', 'call', 'message',
                   'quote_sent', 'assigned', 'follow_up_set', 'converted'));
