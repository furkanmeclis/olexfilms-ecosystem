-- TEC-760: a staged, record-scoped barcode import for a draft stock entry.
CREATE TABLE entry_barcode_imports (
    job_id BIGINT PRIMARY KEY REFERENCES import_jobs(id) ON DELETE CASCADE,
    entry_id BIGINT NOT NULL REFERENCES stock_entries(id) ON DELETE RESTRICT,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    brand_id BIGINT NOT NULL REFERENCES brands(id) ON DELETE RESTRICT,
    existing_policy VARCHAR(16) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'draft',
    rows JSONB NOT NULL DEFAULT '[]'::jsonb,
    CONSTRAINT entry_barcode_import_policy CHECK (existing_policy IN ('none', 'all_existing', 'conflicts_only')),
    CONSTRAINT entry_barcode_import_status CHECK (status IN ('draft', 'validated', 'applied', 'undone')),
    CONSTRAINT entry_barcode_import_rows_array CHECK (jsonb_typeof(rows) = 'array')
);
CREATE INDEX entry_barcode_imports_entry ON entry_barcode_imports(entry_id);
