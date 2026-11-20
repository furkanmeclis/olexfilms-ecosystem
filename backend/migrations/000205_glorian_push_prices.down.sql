DELETE FROM integration_sync_runs WHERE kind = 'push_prices';
ALTER TABLE integration_sync_runs DROP CONSTRAINT chk_integration_sync_runs_kind;
ALTER TABLE integration_sync_runs ADD CONSTRAINT chk_integration_sync_runs_kind CHECK (kind IN (
    'pull_categories', 'pull_products', 'pull_dealers', 'pull_stock',
    'push_barcodes', 'outbound', 'reconcile'));
