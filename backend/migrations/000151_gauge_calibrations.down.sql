-- Drops every recorded gauge calibration (phones keep their local copy).
DROP TABLE IF EXISTS gauge_calibrations;
DROP FUNCTION IF EXISTS gauge_calibrations_append_only();
