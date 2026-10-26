DELETE FROM org_merge_changes WHERE kind IN ('repoint', 'number');
ALTER TABLE org_merge_changes
    DROP CONSTRAINT chk_org_merge_changes_kind,
    DROP CONSTRAINT chk_org_merge_changes_column,
    DROP CONSTRAINT chk_org_merge_changes_row,
    ADD CONSTRAINT chk_org_merge_changes_kind CHECK (kind IN ('update', 'insert', 'delete')),
    ADD CONSTRAINT chk_org_merge_changes_column CHECK ((kind = 'update') = (column_name IS NOT NULL)),
    ADD CONSTRAINT chk_org_merge_changes_row CHECK ((kind = 'delete') = (row_data IS NOT NULL));

DROP INDEX uq_warranty_claims_org_no;
ALTER TABLE warranty_claims ADD CONSTRAINT uq_warranty_claims_org_no UNIQUE (organization_id, claim_no);
ALTER TABLE warranty_claims DROP COLUMN numbered_by_org;

DROP INDEX uq_quotes_org_no;
ALTER TABLE quotes ADD CONSTRAINT uq_quotes_org_no UNIQUE (organization_id, quote_no);
ALTER TABLE quotes DROP COLUMN numbered_by_org;

DROP INDEX uq_contract_instances_org_no;
ALTER TABLE contract_instances ADD CONSTRAINT uq_contract_instances_org_no UNIQUE (organization_id, contract_no);
ALTER TABLE contract_instances DROP COLUMN numbered_by_org;
