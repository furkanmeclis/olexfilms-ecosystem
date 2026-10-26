DROP VIEW IF EXISTS unit_traces;
DROP INDEX IF EXISTS uq_warehouses_primary;
ALTER TABLE warehouses DROP COLUMN IF EXISTS is_primary;
