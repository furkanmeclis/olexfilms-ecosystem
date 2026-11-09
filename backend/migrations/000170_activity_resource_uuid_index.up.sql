-- TEC-1060: the timeline of a record (an order's assignments and edits)
-- reads its activity events by resource and resource uuid.
CREATE INDEX activity_events_resource_uuid_idx
    ON activity_events (resource, resource_uuid, created_at, id)
    WHERE resource_uuid IS NOT NULL;
