-- TEC-468 (F5-01c): public showcase leads and WhatsApp dealer referral.
-- 000111 was already used in this branch; use the next migration number.

ALTER TABLE conversations
    ADD COLUMN referred_dealer_org_id BIGINT NULL REFERENCES organizations (id) ON DELETE SET NULL;

CREATE INDEX idx_conversations_referred_dealer_org
    ON conversations (referred_dealer_org_id)
    WHERE referred_dealer_org_id IS NOT NULL;
