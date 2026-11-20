-- TEC-1265, kullanıcı kararı 5 (TEC-1253): fiyatın sahibi ekosistemdir. Glorian
-- ile senkron ürünlerin merkez liste fiyatı (USD) hub'a tek yönlü gönderilir;
-- her gönderim bir push_prices çalıştırması olarak kaydedilir.
ALTER TABLE integration_sync_runs DROP CONSTRAINT chk_integration_sync_runs_kind;
ALTER TABLE integration_sync_runs ADD CONSTRAINT chk_integration_sync_runs_kind CHECK (kind IN (
    'pull_categories', 'pull_products', 'pull_dealers', 'pull_stock',
    'push_barcodes', 'push_prices', 'outbound', 'reconcile'));
