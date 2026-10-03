-- Reverts TEC-252. Data loss: the legacy id map and the run log; the next
-- migrator run would treat every legacy row as new.
DROP TABLE IF EXISTS migration_runs;
DROP TABLE IF EXISTS migration_map;
