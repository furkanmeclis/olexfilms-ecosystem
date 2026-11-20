-- TEC-1259: KVKK/GDPR anonymization (K19) also covers the contact data that
-- lives outside users / customer_profiles. anonymize_customer_contacts()
-- runs in the same transaction as AnonymizeUser, before the phone and
-- e-mail are cleared, and matches by account, E.164 phone and e-mail:
--
--   * conversations: contact_e164 becomes the placeholder 'anon:<id>' (the
--     column is NOT NULL and UNIQUE per channel), contact_name is cleared,
--     the conversation is closed and the AI is switched off. Rows stay so
--     inbox counters and campaign statistics keep working.
--   * messages of those conversations: body, media, raw provider payload,
--     failure reason and stored media reference are cleared (the media
--     objects are deleted from storage by the caller after commit).
--   * conversation_ai_runs of those conversations: stages, tool calls and
--     error (they quote the messages) are cleared.
--   * whatsapp_lid_map: rows of the phone are deleted (pure lookup data).
--   * notifications (by account, phone or e-mail recipient): title, body,
--     payload, action URL, recipient and last error are cleared; delivery
--     status rows stay for statistics. notification_deliveries.error too.
--   * leads (customer leads of the account, candidate leads with the phone
--     or e-mail): candidate company/contact name, phone, e-mail and notes
--     are cleared, lost_reason is masked (the status check needs a value).
--     note / call / message events of those leads get the masked payload
--     {"anonymized": true}: the only change lead_events now allows.
--
-- lead_events_append_only keeps the org merge bypass that 000152 added
-- (TEC-1003: olex.org_merge moves rows to the target organization).
--
-- Kept on purpose: contact_opt_outs (the record of an objection must
-- outlive the person, KVKK md. 11 / ETK) and activity_events (no personal
-- data in the anonymization audit payload).

CREATE OR REPLACE FUNCTION lead_events_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND current_setting('olex.org_merge', true) = 'on' THEN RETURN NEW; END IF; -- TEC-1003 org merge
    IF TG_OP = 'UPDATE'
       AND current_setting('olex.lead_events_anonymize', true) = 'on'
       AND NEW.payload = '{"anonymized": true}'::jsonb
       AND (NEW.id, NEW.uuid, NEW.lead_id, NEW.organization_id, NEW.brand_id, NEW.event_type,
            NEW.actor_user_id, NEW.created_at)
           IS NOT DISTINCT FROM
           (OLD.id, OLD.uuid, OLD.lead_id, OLD.organization_id, OLD.brand_id, OLD.event_type,
            OLD.actor_user_id, OLD.created_at)
    THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'lead_events is append-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation',
              HINT = 'Only anonymize_customer_contacts() may mask a row (K19).';
END;
$$;

CREATE FUNCTION anonymize_customer_contacts(p_user_id BIGINT, p_phone TEXT, p_email TEXT) RETURNS JSONB
LANGUAGE plpgsql AS $$
DECLARE
    v_phone TEXT := NULLIF(btrim(COALESCE(p_phone, '')), '');
    v_email TEXT := NULLIF(lower(btrim(COALESCE(p_email, ''))), '');
    v_conv  BIGINT[];
    v_leads BIGINT[];
    n_conv  INTEGER := 0;
    n_msg   INTEGER := 0;
    n_runs  INTEGER := 0;
    n_lid   INTEGER := 0;
    n_notif INTEGER := 0;
    n_deliv INTEGER := 0;
    n_leads INTEGER := 0;
    n_evts  INTEGER := 0;
BEGIN
    IF p_user_id IS NULL AND v_phone IS NULL AND v_email IS NULL THEN
        RETURN '{}'::jsonb;
    END IF;

    SELECT COALESCE(array_agg(id), '{}') INTO v_conv
    FROM conversations
    WHERE (p_user_id IS NOT NULL AND (user_id = p_user_id OR identity_user_id = p_user_id))
       OR (v_phone IS NOT NULL AND contact_e164 = v_phone);

    UPDATE messages
    SET body = NULL, media = NULL, raw = NULL, failure_reason = NULL,
        media_storage_key = NULL, media_mime = NULL, media_size = NULL
    WHERE conversation_id = ANY (v_conv)
      AND (body IS NOT NULL OR media IS NOT NULL OR raw IS NOT NULL
           OR failure_reason IS NOT NULL OR media_storage_key IS NOT NULL);
    GET DIAGNOSTICS n_msg = ROW_COUNT;

    UPDATE conversation_ai_runs
    SET stages = '[]'::jsonb, tool_calls = '[]'::jsonb, error = NULL
    WHERE conversation_id = ANY (v_conv)
      AND (stages <> '[]'::jsonb OR tool_calls <> '[]'::jsonb OR error IS NOT NULL);
    GET DIAGNOSTICS n_runs = ROW_COUNT;

    UPDATE conversations
    SET contact_e164 = 'anon:' || id::text,
        contact_name = NULL,
        status = 'closed',
        ai_mode = 'off',
        ai_paused_until = NULL
    WHERE id = ANY (v_conv)
      AND contact_e164 <> 'anon:' || id::text;
    GET DIAGNOSTICS n_conv = ROW_COUNT;

    IF v_phone IS NOT NULL THEN
        DELETE FROM whatsapp_lid_map WHERE phone_e164 = v_phone;
        GET DIAGNOSTICS n_lid = ROW_COUNT;
    END IF;

    UPDATE notifications
    SET title = '', body = '', payload = '{}'::jsonb, action_url = NULL,
        recipient = NULL, last_error = NULL
    WHERE ((p_user_id IS NOT NULL AND user_id = p_user_id)
        OR (v_phone IS NOT NULL AND recipient = v_phone)
        OR (v_email IS NOT NULL AND lower(recipient) = v_email))
      AND (title <> '' OR body <> '' OR payload <> '{}'::jsonb OR action_url IS NOT NULL
           OR recipient IS NOT NULL OR last_error IS NOT NULL);
    GET DIAGNOSTICS n_notif = ROW_COUNT;

    IF p_user_id IS NOT NULL THEN
        UPDATE notification_deliveries SET error = NULL
        WHERE user_id = p_user_id AND error IS NOT NULL;
        GET DIAGNOSTICS n_deliv = ROW_COUNT;
    END IF;

    SELECT COALESCE(array_agg(id), '{}') INTO v_leads
    FROM leads
    WHERE (p_user_id IS NOT NULL AND customer_user_id = p_user_id)
       OR (v_phone IS NOT NULL AND candidate_phone_e164 = v_phone)
       OR (v_email IS NOT NULL AND lower(candidate_email) = v_email);

    UPDATE leads
    SET candidate_company_name = NULL,
        candidate_contact_name = NULL,
        candidate_phone_e164   = NULL,
        candidate_email        = NULL,
        notes                  = '',
        lost_reason            = CASE WHEN lost_reason IS NULL THEN NULL ELSE '[anonymized]' END
    WHERE id = ANY (v_leads)
      AND (candidate_company_name IS NOT NULL OR candidate_contact_name IS NOT NULL
           OR candidate_phone_e164 IS NOT NULL OR candidate_email IS NOT NULL
           OR notes <> '' OR (lost_reason IS NOT NULL AND lost_reason <> '[anonymized]'));
    GET DIAGNOSTICS n_leads = ROW_COUNT;

    PERFORM set_config('olex.lead_events_anonymize', 'on', true);
    UPDATE lead_events
    SET payload = '{"anonymized": true}'::jsonb
    WHERE lead_id = ANY (v_leads)
      AND event_type IN ('note', 'call', 'message')
      AND payload <> '{"anonymized": true}'::jsonb;
    GET DIAGNOSTICS n_evts = ROW_COUNT;
    PERFORM set_config('olex.lead_events_anonymize', 'off', true);

    RETURN jsonb_build_object(
        'conversations', n_conv, 'messages', n_msg, 'conversation_ai_runs', n_runs,
        'whatsapp_lid_map', n_lid, 'notifications', n_notif, 'notification_deliveries', n_deliv,
        'leads', n_leads, 'lead_events', n_evts);
END;
$$;
