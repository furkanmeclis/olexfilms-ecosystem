-- TEC-623: WhatsApp LID -> phone map. whatsmeow addresses privacy-mode
-- contacts by a linked id (…@lid) and only sometimes attaches the phone JID
-- (SenderAlt/RecipientAlt). The backend learns each pair it sees and uses it
-- to resolve inbound messages that arrive with the LID alone.
CREATE TABLE whatsapp_lid_map (
    lid           VARCHAR(64)  PRIMARY KEY,
    phone_e164    VARCHAR(20)  NOT NULL,
    first_seen_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_whatsapp_lid_map_phone ON whatsapp_lid_map (phone_e164);
