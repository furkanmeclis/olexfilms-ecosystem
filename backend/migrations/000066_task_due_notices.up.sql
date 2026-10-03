-- TEC-221 (F1-11b): due date reminders of center tasks. The hourly
-- tasks:due_scan stamps these in the transaction that writes the
-- tasks.due_soon / tasks.overdue outbox event, so a second run (or a
-- concurrent one) finds nothing for the same task and threshold. Changing
-- due_at clears both stamps (UpdateTask), so a new deadline is reminded
-- again. Templates are seeded by the Go catalog (SyncCatalog), not here.
ALTER TABLE tasks
    ADD COLUMN due_soon_notified_at TIMESTAMPTZ NULL,
    ADD COLUMN overdue_notified_at  TIMESTAMPTZ NULL;

CREATE INDEX idx_tasks_due_scan ON tasks (due_at)
    WHERE status IN ('open', 'in_progress') AND due_at IS NOT NULL AND overdue_notified_at IS NULL;
