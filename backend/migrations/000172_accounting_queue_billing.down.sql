DROP INDEX IF EXISTS idx_stock_transfer_requests_return_queue;

ALTER TABLE stock_transfer_requests
    DROP COLUMN IF EXISTS accounting_note,
    DROP COLUMN IF EXISTS accounting_processed_by_user_id,
    DROP COLUMN IF EXISTS accounting_processed_at;

ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS chk_organizations_iban,
    DROP COLUMN IF EXISTS billing_updated_by_user_id,
    DROP COLUMN IF EXISTS billing_updated_at,
    DROP COLUMN IF EXISTS iban,
    DROP COLUMN IF EXISTS billing_email,
    DROP COLUMN IF EXISTS billing_address,
    DROP COLUMN IF EXISTS tax_no,
    DROP COLUMN IF EXISTS tax_office,
    DROP COLUMN IF EXISTS billing_title;
