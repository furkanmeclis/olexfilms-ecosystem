-- TEC-1003 fix: merging an organization that numbers its documents
-- (contracts, quotes, warranty claims: 1, 2, ... per organization) into one
-- that has the same numbers. The merged documents keep their number; the
-- organization that issued it is kept in numbered_by_org and the number is
-- unique per issuer. A plain BIGINT (no FK, no *_org_id name) so the org
-- merge does not move it.
--
-- org_merge_changes gains 'repoint': a child row moved from a source row to
-- the target's duplicate (a measurement device with the same serial);
-- row_data holds {"from": <old id>, "to": <new id>}. 'number' records a
-- numbered_by_org the merge set (undo clears it).

ALTER TABLE contract_instances ADD COLUMN numbered_by_org BIGINT NULL;
ALTER TABLE contract_instances DROP CONSTRAINT uq_contract_instances_org_no;
CREATE UNIQUE INDEX uq_contract_instances_org_no
    ON contract_instances ((COALESCE(numbered_by_org, organization_id)), contract_no);

ALTER TABLE quotes ADD COLUMN numbered_by_org BIGINT NULL;
ALTER TABLE quotes DROP CONSTRAINT uq_quotes_org_no;
CREATE UNIQUE INDEX uq_quotes_org_no
    ON quotes ((COALESCE(numbered_by_org, organization_id)), quote_no);

ALTER TABLE warranty_claims ADD COLUMN numbered_by_org BIGINT NULL;
ALTER TABLE warranty_claims DROP CONSTRAINT uq_warranty_claims_org_no;
CREATE UNIQUE INDEX uq_warranty_claims_org_no
    ON warranty_claims ((COALESCE(numbered_by_org, organization_id)), claim_no);

ALTER TABLE org_merge_changes
    DROP CONSTRAINT chk_org_merge_changes_kind,
    DROP CONSTRAINT chk_org_merge_changes_column,
    DROP CONSTRAINT chk_org_merge_changes_row,
    ADD CONSTRAINT chk_org_merge_changes_kind CHECK (kind IN ('update', 'insert', 'delete', 'repoint', 'number')),
    ADD CONSTRAINT chk_org_merge_changes_column CHECK ((kind IN ('update', 'repoint', 'number')) = (column_name IS NOT NULL)),
    ADD CONSTRAINT chk_org_merge_changes_row CHECK ((kind IN ('delete', 'repoint')) = (row_data IS NOT NULL));
