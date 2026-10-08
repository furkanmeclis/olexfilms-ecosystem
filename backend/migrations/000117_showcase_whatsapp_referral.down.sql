DROP INDEX IF EXISTS idx_conversations_referred_dealer_org;

ALTER TABLE conversations
    DROP COLUMN IF EXISTS referred_dealer_org_id;
