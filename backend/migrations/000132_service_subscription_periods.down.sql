DROP TABLE IF EXISTS service_subscription_reminders;

-- A grant next to a manual value cannot survive the single org index; the
-- manual value wins.
DELETE FROM module_flags s
WHERE s.source = 'service'
  AND EXISTS (
      SELECT 1 FROM module_flags m
      WHERE m.scope = s.scope AND m.organization_id = s.organization_id
        AND m.module_key = s.module_key AND m.source <> 'service');
ALTER TABLE module_flags DROP CONSTRAINT IF EXISTS chk_module_flags_service;
DROP INDEX IF EXISTS uq_module_flags_service;
DROP INDEX IF EXISTS uq_module_flags_org;
CREATE UNIQUE INDEX uq_module_flags_org ON module_flags (scope, organization_id, module_key)
    WHERE organization_id IS NOT NULL;

DROP INDEX IF EXISTS idx_service_subscriptions_scheduled;
UPDATE service_subscriptions SET status = 'active' WHERE status = 'scheduled';
ALTER TABLE service_subscriptions DROP CONSTRAINT chk_service_subscriptions_status;
ALTER TABLE service_subscriptions ADD CONSTRAINT chk_service_subscriptions_status CHECK (
    status IN ('active', 'cancel_requested', 'cancelled', 'expired'));
