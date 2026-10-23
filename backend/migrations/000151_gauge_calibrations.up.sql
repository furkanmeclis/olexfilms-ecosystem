-- TEC-939: SmartPTG / NexPTG zero (foil) calibrations, see
-- docs/smartptg/REVERSE_ENGINEERING.md §6.1 and §9.2.
--
-- The gauge only reports a probe frequency; the app converts it with the
-- serial's factory curve plus a per-serial frequency offset found by the
-- foil calibration (offset = curve frequency of the 90/150 µm point -
-- measured frequency). The official app keeps the offsets per phone; here
-- every calibration is a ledger row of the organization, so another phone
-- of the same organization picks the offsets up. The current offset of a
-- serial = the latest row of (organization_id, serial).
--
-- The factory curve itself stays in the object store (TEC-934,
-- smartptg/curves/{serial}.dat): it is vendor data, no table.
CREATE TABLE gauge_calibrations (
    id               BIGSERIAL     PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    serial           INTEGER       NOT NULL,
    -- Hz added to every frequency of the channel; NULL = channel not
    -- calibrated (Economic gauges have no Al channel).
    fe_offset_hz     INTEGER       NULL,
    al_offset_hz     INTEGER       NULL,
    -- Foil thickness used: 90 µm (curve comment has cal1/cal2/cal3) or 150 µm.
    target_um        SMALLINT      NOT NULL,
    -- Measured frequencies of the steady samples, {"fe":[...],"al":[...]} (audit).
    samples          JSONB         NOT NULL DEFAULT '{}'::jsonb,
    -- Idempotency key of the app (a retried upload stores one row).
    client_id        UUID          NULL,
    -- When the phone finished the calibration (it may upload later, offline).
    calibrated_at    TIMESTAMPTZ   NOT NULL,
    created_by       BIGINT        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_gauge_calibrations_uuid UNIQUE (uuid),
    CONSTRAINT chk_gauge_calibrations_serial CHECK (serial > 0),
    CONSTRAINT chk_gauge_calibrations_offsets CHECK (
        num_nonnulls(fe_offset_hz, al_offset_hz) > 0
        AND (fe_offset_hz IS NULL OR fe_offset_hz BETWEEN -20000 AND 20000)
        AND (al_offset_hz IS NULL OR al_offset_hz BETWEEN -20000 AND 20000)),
    CONSTRAINT chk_gauge_calibrations_target CHECK (target_um IN (90, 150)),
    CONSTRAINT chk_gauge_calibrations_samples CHECK (jsonb_typeof(samples) = 'object')
);

CREATE UNIQUE INDEX uq_gauge_calibrations_org_client
    ON gauge_calibrations (organization_id, client_id) WHERE client_id IS NOT NULL;
CREATE INDEX idx_gauge_calibrations_org_serial
    ON gauge_calibrations (organization_id, serial, calibrated_at DESC, id DESC);

CREATE TRIGGER trg_gauge_calibrations_check_org
    BEFORE INSERT ON gauge_calibrations
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

CREATE FUNCTION gauge_calibrations_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'gauge_calibrations is append-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation',
              HINT = 'Record a new calibration instead.';
END;
$$;

CREATE TRIGGER trg_gauge_calibrations_append_only
    BEFORE UPDATE OR DELETE ON gauge_calibrations
    FOR EACH ROW
    EXECUTE FUNCTION gauge_calibrations_append_only();

CREATE TRIGGER trg_gauge_calibrations_no_truncate
    BEFORE TRUNCATE ON gauge_calibrations
    FOR EACH STATEMENT
    EXECUTE FUNCTION gauge_calibrations_append_only();
