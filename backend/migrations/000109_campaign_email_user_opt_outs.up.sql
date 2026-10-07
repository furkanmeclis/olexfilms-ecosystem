-- TEC-463: campaign unsubscribe/opt-out by user identity for recipients
-- without a phone number. Phone opt-outs remain in contact_opt_outs.

CREATE TABLE campaign_user_opt_outs (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    user_id             BIGINT        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    scope               VARCHAR(16)   NOT NULL,
    action              VARCHAR(8)    NOT NULL DEFAULT 'out',
    source              VARCHAR(16)   NOT NULL,
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    note                TEXT          NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_campaign_user_opt_outs_uuid UNIQUE (uuid),
    CONSTRAINT chk_campaign_user_opt_outs_scope CHECK (scope IN ('marketing')),
    CONSTRAINT chk_campaign_user_opt_outs_action CHECK (action IN ('out', 'in')),
    CONSTRAINT chk_campaign_user_opt_outs_source CHECK (
        source IN ('campaign', 'panel', 'portal', 'import', 'system')),
    CONSTRAINT chk_campaign_user_opt_outs_note CHECK (note IS NULL OR char_length(note) <= 1000)
);

CREATE INDEX idx_campaign_user_opt_outs_user ON campaign_user_opt_outs (user_id, scope, id DESC);
CREATE INDEX idx_campaign_user_opt_outs_created_by ON campaign_user_opt_outs (created_by_user_id)
    WHERE created_by_user_id IS NOT NULL;

CREATE FUNCTION campaign_user_opt_outs_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'campaign_user_opt_outs is append-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$;

CREATE TRIGGER trg_campaign_user_opt_outs_append_only
    BEFORE UPDATE OR DELETE ON campaign_user_opt_outs
    FOR EACH ROW
    EXECUTE FUNCTION campaign_user_opt_outs_append_only();

CREATE TRIGGER trg_campaign_user_opt_outs_no_truncate
    BEFORE TRUNCATE ON campaign_user_opt_outs
    FOR EACH STATEMENT
    EXECUTE FUNCTION campaign_user_opt_outs_append_only();

CREATE TABLE campaign_user_opt_out_state (
    user_id       BIGINT        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    scope         VARCHAR(16)   NOT NULL,
    opted_out     BOOLEAN       NOT NULL,
    last_entry_id BIGINT        NOT NULL REFERENCES campaign_user_opt_outs (id) ON DELETE RESTRICT,
    source        VARCHAR(16)   NOT NULL,
    changed_at    TIMESTAMPTZ   NOT NULL,
    PRIMARY KEY (user_id, scope),
    CONSTRAINT chk_campaign_user_opt_out_state_scope CHECK (scope IN ('marketing'))
);

CREATE INDEX idx_campaign_user_opt_out_state_opted_out ON campaign_user_opt_out_state (scope, user_id)
    WHERE opted_out;
CREATE INDEX idx_campaign_user_opt_out_state_last_entry ON campaign_user_opt_out_state (last_entry_id);

CREATE FUNCTION campaign_user_opt_outs_project() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO campaign_user_opt_out_state (user_id, scope, opted_out, last_entry_id, source, changed_at)
    VALUES (NEW.user_id, NEW.scope, NEW.action = 'out', NEW.id, NEW.source, NEW.created_at)
    ON CONFLICT (user_id, scope) DO UPDATE
    SET opted_out = EXCLUDED.opted_out,
        last_entry_id = EXCLUDED.last_entry_id,
        source = EXCLUDED.source,
        changed_at = EXCLUDED.changed_at
    WHERE campaign_user_opt_out_state.last_entry_id < EXCLUDED.last_entry_id;
    RETURN NULL;
END;
$$;

CREATE TRIGGER trg_campaign_user_opt_outs_project
    AFTER INSERT ON campaign_user_opt_outs
    FOR EACH ROW
    EXECUTE FUNCTION campaign_user_opt_outs_project();
