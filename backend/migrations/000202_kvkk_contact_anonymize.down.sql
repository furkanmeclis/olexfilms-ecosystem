DROP FUNCTION IF EXISTS anonymize_customer_contacts(BIGINT, TEXT, TEXT);

CREATE OR REPLACE FUNCTION lead_events_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'lead_events is append-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$;
