-- Reverts TEC-202. Units created by a batch keep existing (their barcodes
-- stay unique); only the batch link is dropped.
DROP INDEX IF EXISTS idx_units_batch;
ALTER TABLE units DROP COLUMN IF EXISTS batch_id;

DROP TABLE IF EXISTS barcode_batches;
DROP TABLE IF EXISTS barcode_counters;
DROP TABLE IF EXISTS label_templates;
