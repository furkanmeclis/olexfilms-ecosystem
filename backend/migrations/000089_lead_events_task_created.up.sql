-- TEC-313: a center task opened from a lead is its own timeline event
-- (task_created), not a lead conversion.
ALTER TABLE lead_events DROP CONSTRAINT chk_lead_events_type;
ALTER TABLE lead_events ADD CONSTRAINT chk_lead_events_type CHECK (
    event_type IN ('created', 'status_changed', 'note', 'call', 'message',
                   'quote_sent', 'assigned', 'follow_up_set', 'converted',
                   'task_created'));
