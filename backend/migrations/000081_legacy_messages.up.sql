-- TEC-263 (F2-01l): read-only archive of the old hub's messages.
--
-- The hub's sms_logs (channel sms / whatsapp; WhatsApp messages are the
-- sms_logs rows with channel 'whatsapp') and notifications (Laravel database
-- notifications) are copied here by the migrator. They do not enter the new
-- notification center and have no UI; only a read-only repository query
-- reads them.
--
-- The table is append-only: UPDATE, DELETE and TRUNCATE are rejected by a
-- trigger. The single exception is the KVKK/GDPR anonymization (K19):
-- anonymize_legacy_messages() masks recipient, body and payload of a
-- person's rows. It flags the transaction with a local setting the trigger
-- checks, and even then the trigger accepts only the exact masked values
-- with every other column unchanged, so the exception cannot rewrite a
-- message.
--
-- recipient is E.164 when the legacy phone resolved; an unresolved phone is
-- kept in payload.recipient_raw (also masked by the anonymization). user_id
-- is the migrated account the message was about (the hub notifiable),
-- when known. Source columns make reruns idempotent.
CREATE TABLE legacy_messages (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL,
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id        BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    channel         VARCHAR(16)  NOT NULL,
    recipient       VARCHAR(20)  NULL,
    user_id         BIGINT       NULL REFERENCES users (id) ON DELETE RESTRICT,
    body            TEXT         NOT NULL,
    payload         JSONB        NOT NULL DEFAULT '{}'::jsonb,
    sent_at         TIMESTAMPTZ  NULL,
    source_system   VARCHAR(32)  NOT NULL,
    source_table    VARCHAR(64)  NOT NULL,
    source_id       VARCHAR(64)  NOT NULL,
    anonymized_at   TIMESTAMPTZ  NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_legacy_messages_uuid UNIQUE (uuid),
    CONSTRAINT uq_legacy_messages_source UNIQUE (source_system, source_table, source_id),
    CONSTRAINT chk_legacy_messages_channel CHECK (channel IN ('sms', 'notification', 'whatsapp')),
    CONSTRAINT chk_legacy_messages_recipient CHECK (recipient IS NULL OR recipient ~ '^\+[1-9][0-9]{6,14}$'),
    CONSTRAINT chk_legacy_messages_payload CHECK (jsonb_typeof(payload) = 'object')
);

CREATE INDEX idx_legacy_messages_brand_sent ON legacy_messages (brand_id, sent_at DESC, id DESC);
CREATE INDEX idx_legacy_messages_organization ON legacy_messages (organization_id);
CREATE INDEX idx_legacy_messages_user ON legacy_messages (user_id, sent_at DESC, id DESC) WHERE user_id IS NOT NULL;
CREATE INDEX idx_legacy_messages_recipient ON legacy_messages (recipient) WHERE recipient IS NOT NULL;

-- Masked values written by the anonymization; the guard accepts only these.
CREATE FUNCTION legacy_messages_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE'
       AND current_setting('olex.legacy_messages_anonymize', true) = 'on'
       AND OLD.anonymized_at IS NULL
       AND NEW.anonymized_at IS NOT NULL
       AND NEW.recipient IS NULL
       AND NEW.body = '[anonymized]'
       AND NEW.payload = '{"anonymized": true}'::jsonb
       AND (NEW.id, NEW.uuid, NEW.organization_id, NEW.brand_id, NEW.channel, NEW.user_id, NEW.sent_at,
            NEW.source_system, NEW.source_table, NEW.source_id, NEW.created_at)
           IS NOT DISTINCT FROM
           (OLD.id, OLD.uuid, OLD.organization_id, OLD.brand_id, OLD.channel, OLD.user_id, OLD.sent_at,
            OLD.source_system, OLD.source_table, OLD.source_id, OLD.created_at)
    THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'legacy_messages is read-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation',
              HINT = 'Only anonymize_legacy_messages() may mask a row (K19).';
END;
$$;

CREATE TRIGGER trg_legacy_messages_guard
    BEFORE UPDATE OR DELETE ON legacy_messages
    FOR EACH ROW
    EXECUTE FUNCTION legacy_messages_guard();

CREATE TRIGGER trg_legacy_messages_no_truncate
    BEFORE TRUNCATE ON legacy_messages
    FOR EACH STATEMENT
    EXECUTE FUNCTION legacy_messages_guard();

-- K19: masks the archived messages of a person (by migrated account and/or
-- E.164 phone) and returns the number of rows masked. Already masked rows
-- are skipped, so a second call masks nothing.
CREATE FUNCTION anonymize_legacy_messages(p_user_id BIGINT, p_phone TEXT) RETURNS INTEGER
LANGUAGE plpgsql AS $$
DECLARE
    n INTEGER;
BEGIN
    IF p_user_id IS NULL AND (p_phone IS NULL OR p_phone = '') THEN
        RETURN 0;
    END IF;
    PERFORM set_config('olex.legacy_messages_anonymize', 'on', true);
    UPDATE legacy_messages
    SET recipient     = NULL,
        body          = '[anonymized]',
        payload       = '{"anonymized": true}'::jsonb,
        anonymized_at = NOW()
    WHERE anonymized_at IS NULL
      AND ((p_user_id IS NOT NULL AND user_id = p_user_id)
        OR (p_phone IS NOT NULL AND p_phone <> '' AND recipient = p_phone));
    GET DIAGNOSTICS n = ROW_COUNT;
    PERFORM set_config('olex.legacy_messages_anonymize', 'off', true);
    RETURN n;
END;
$$;
