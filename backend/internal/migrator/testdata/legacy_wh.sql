-- TEC-253: eski olexfilms-warehouse (Laravel/MariaDB) şemasının PostgreSQL
-- karşılığı.
--
-- Yalnız migrator test fixture'ıdır; golang-migrate'e GİRMEZ ve prod'da
-- çalışmaz. Kaynak: olexfilms-warehouse database/migrations/* (create +
-- sonraki alter'lar: brand_id, barcode_short_code, external_id/source,
-- catalog sync alanları, stock item sync alanları, connection_id,
-- merged_into_customer_id, tax_no/tax_office, uses_fixed_barcode,
-- quantity_on_hand).
--
-- Yalnız design.md §7'de taşınacak tablolar ve onların zorunlu ebeveynleri
-- alınır. Taşınmayan tablolara giden FK'lar (users, stock_entries,
-- product_barcode_templates, delivery_methods, integration_connections)
-- kolon olarak korunur, kısıt olarak yazılmaz. Laravel uuid -> uuid,
-- json -> jsonb, boolean -> boolean, unsignedInteger -> integer.
--
-- VERİ TAMAMEN SENTETİKTİR; gerçek kayıt içermez.

DROP SCHEMA IF EXISTS legacy_wh CASCADE;
CREATE SCHEMA legacy_wh;

-- ---------------------------------------------------------------- yapı

CREATE TABLE legacy_wh.warehouses (
    id uuid PRIMARY KEY,
    code varchar(255) NOT NULL,
    name varchar(255) NOT NULL,
    country varchar(255) DEFAULT NULL,
    city varchar(255) DEFAULT NULL,
    district varchar(255) DEFAULT NULL,
    is_active boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0,
    settings jsonb DEFAULT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    deleted_at timestamp NULL DEFAULT NULL,
    CONSTRAINT warehouses_code_unique UNIQUE (code)
);

CREATE TABLE legacy_wh.warehouse_sites (
    id uuid PRIMARY KEY,
    warehouse_id uuid NOT NULL REFERENCES legacy_wh.warehouses (id) ON DELETE CASCADE,
    code varchar(255) NOT NULL,
    name varchar(255) NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    deleted_at timestamp NULL DEFAULT NULL,
    CONSTRAINT warehouse_sites_warehouse_id_code_unique UNIQUE (warehouse_id, code)
);

CREATE TABLE legacy_wh.warehouse_locations (
    id uuid PRIMARY KEY,
    warehouse_id uuid NOT NULL REFERENCES legacy_wh.warehouses (id) ON DELETE CASCADE,
    warehouse_site_id uuid NOT NULL REFERENCES legacy_wh.warehouse_sites (id) ON DELETE CASCADE,
    parent_id uuid DEFAULT NULL REFERENCES legacy_wh.warehouse_locations (id) ON DELETE SET NULL,
    type varchar(255) NOT NULL,
    code varchar(255) NOT NULL,
    name varchar(255) DEFAULT NULL,
    full_code varchar(255) NOT NULL,
    barcode_payload varchar(255) NOT NULL,
    sort_order integer NOT NULL DEFAULT 0,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    deleted_at timestamp NULL DEFAULT NULL,
    CONSTRAINT warehouse_locations_site_parent_code_unique UNIQUE (warehouse_site_id, parent_id, code)
);
CREATE INDEX warehouse_locations_full_code_index ON legacy_wh.warehouse_locations (full_code);

CREATE TABLE legacy_wh.brands (
    id uuid PRIMARY KEY,
    name varchar(255) NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    deleted_at timestamp NULL DEFAULT NULL,
    CONSTRAINT brands_name_unique UNIQUE (name)
);

CREATE TABLE legacy_wh.product_categories (
    id uuid PRIMARY KEY,
    connection_id uuid DEFAULT NULL,
    name varchar(255) NOT NULL,
    source varchar(255) DEFAULT NULL,
    external_id varchar(255) DEFAULT NULL,
    available_parts jsonb DEFAULT NULL,
    is_active boolean NOT NULL DEFAULT true,
    last_synced_at timestamp NULL DEFAULT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    deleted_at timestamp NULL DEFAULT NULL,
    CONSTRAINT product_categories_connection_external_unique UNIQUE (connection_id, external_id)
);

CREATE TABLE legacy_wh.products (
    id uuid PRIMARY KEY,
    connection_id uuid DEFAULT NULL,
    category_id uuid NOT NULL REFERENCES legacy_wh.product_categories (id) ON DELETE RESTRICT,
    brand_id uuid DEFAULT NULL REFERENCES legacy_wh.brands (id) ON DELETE RESTRICT,
    name varchar(255) NOT NULL,
    sku varchar(255) NOT NULL,
    source varchar(255) DEFAULT NULL,
    external_id varchar(255) DEFAULT NULL,
    barcode_short_code varchar(16) DEFAULT NULL,
    description text DEFAULT NULL,
    warranty_duration integer DEFAULT NULL,
    micron_thickness integer DEFAULT NULL,
    price numeric(10, 2) NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    uses_fixed_barcode boolean NOT NULL DEFAULT false,
    last_synced_at timestamp NULL DEFAULT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    deleted_at timestamp NULL DEFAULT NULL,
    CONSTRAINT products_barcode_short_code_unique UNIQUE (barcode_short_code),
    CONSTRAINT products_connection_external_unique UNIQUE (connection_id, external_id),
    CONSTRAINT products_connection_sku_unique UNIQUE (connection_id, sku)
);

CREATE TABLE legacy_wh.customers (
    id uuid PRIMARY KEY,
    merged_into_customer_id uuid DEFAULT NULL REFERENCES legacy_wh.customers (id) ON DELETE SET NULL,
    source varchar(255) NOT NULL DEFAULT 'olexfilms',
    external_id varchar(255) DEFAULT NULL,
    dealer_code varchar(255) DEFAULT NULL,
    name varchar(255) NOT NULL,
    email varchar(255) DEFAULT NULL,
    phone varchar(255) DEFAULT NULL,
    tax_no varchar(255) DEFAULT NULL,
    tax_office varchar(255) DEFAULT NULL,
    address text DEFAULT NULL,
    city varchar(255) DEFAULT NULL,
    district varchar(255) DEFAULT NULL,
    country varchar(255) DEFAULT NULL,
    latitude numeric(10, 7) DEFAULT NULL,
    longitude numeric(10, 7) DEFAULT NULL,
    is_active boolean NOT NULL DEFAULT true,
    website_url varchar(255) DEFAULT NULL,
    logo_url varchar(255) DEFAULT NULL,
    notes text DEFAULT NULL,
    last_synced_at timestamp NULL DEFAULT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    deleted_at timestamp NULL DEFAULT NULL,
    CONSTRAINT customers_source_external_id_unique UNIQUE (source, external_id),
    CONSTRAINT customers_dealer_code_unique UNIQUE (dealer_code)
);

CREATE TABLE legacy_wh.product_barcodes (
    id uuid PRIMARY KEY,
    connection_id uuid DEFAULT NULL,
    product_id uuid NOT NULL REFERENCES legacy_wh.products (id) ON DELETE RESTRICT,
    code varchar(255) NOT NULL,
    external_id varchar(255) DEFAULT NULL,
    external_status varchar(255) DEFAULT NULL,
    external_location varchar(255) DEFAULT NULL,
    external_dealer_id varchar(255) DEFAULT NULL,
    customer_id uuid DEFAULT NULL REFERENCES legacy_wh.customers (id) ON DELETE SET NULL,
    source varchar(255) NOT NULL DEFAULT 'generated',
    status varchar(255) NOT NULL DEFAULT 'reserved',
    quantity_on_hand integer NOT NULL DEFAULT 1,
    warehouse_id uuid DEFAULT NULL REFERENCES legacy_wh.warehouses (id) ON DELETE SET NULL,
    warehouse_location_id uuid DEFAULT NULL REFERENCES legacy_wh.warehouse_locations (id) ON DELETE SET NULL,
    stock_entry_id uuid DEFAULT NULL,
    template_id uuid DEFAULT NULL,
    printed_at timestamp NULL DEFAULT NULL,
    placed_at timestamp NULL DEFAULT NULL,
    last_synced_at timestamp NULL DEFAULT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    deleted_at timestamp NULL DEFAULT NULL,
    CONSTRAINT product_barcodes_code_unique UNIQUE (code),
    CONSTRAINT product_barcodes_connection_external_unique UNIQUE (connection_id, external_id)
);

CREATE TABLE legacy_wh.bin_product_stocks (
    id uuid PRIMARY KEY,
    warehouse_location_id uuid NOT NULL REFERENCES legacy_wh.warehouse_locations (id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES legacy_wh.products (id) ON DELETE RESTRICT,
    quantity_on_hand integer NOT NULL DEFAULT 0,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    CONSTRAINT bin_product_stocks_location_product_unique UNIQUE (warehouse_location_id, product_id)
);

CREATE TABLE legacy_wh.stock_movements (
    id uuid PRIMARY KEY,
    product_id uuid NOT NULL REFERENCES legacy_wh.products (id) ON DELETE RESTRICT,
    product_barcode_id uuid DEFAULT NULL REFERENCES legacy_wh.product_barcodes (id) ON DELETE SET NULL,
    warehouse_id uuid NOT NULL REFERENCES legacy_wh.warehouses (id) ON DELETE RESTRICT,
    warehouse_location_id uuid DEFAULT NULL REFERENCES legacy_wh.warehouse_locations (id) ON DELETE SET NULL,
    quantity_delta integer NOT NULL,
    movement_type varchar(255) NOT NULL,
    reference_type varchar(255) DEFAULT NULL,
    reference_id uuid DEFAULT NULL,
    user_id uuid DEFAULT NULL,
    notes text DEFAULT NULL,
    created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE legacy_wh.orders (
    id uuid PRIMARY KEY,
    warehouse_id uuid NOT NULL REFERENCES legacy_wh.warehouses (id) ON DELETE CASCADE,
    customer_id uuid NOT NULL REFERENCES legacy_wh.customers (id) ON DELETE RESTRICT,
    delivery_method_id uuid DEFAULT NULL,
    status varchar(255) NOT NULL DEFAULT 'draft',
    syncs_to_inventory boolean NOT NULL DEFAULT false,
    external_order_id varchar(255) DEFAULT NULL,
    external_reference varchar(255) NOT NULL,
    tracking_number varchar(255) DEFAULT NULL,
    notes text DEFAULT NULL,
    created_by uuid DEFAULT NULL,
    shipped_by uuid DEFAULT NULL,
    delivered_by uuid DEFAULT NULL,
    received_by uuid DEFAULT NULL,
    cancelled_by uuid DEFAULT NULL,
    confirmed_at timestamp NULL DEFAULT NULL,
    shipped_at timestamp NULL DEFAULT NULL,
    delivered_at timestamp NULL DEFAULT NULL,
    received_at timestamp NULL DEFAULT NULL,
    cancelled_at timestamp NULL DEFAULT NULL,
    last_sync_error text DEFAULT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    deleted_at timestamp NULL DEFAULT NULL,
    CONSTRAINT orders_external_reference_unique UNIQUE (external_reference)
);

CREATE TABLE legacy_wh.order_items (
    id uuid PRIMARY KEY,
    order_id uuid NOT NULL REFERENCES legacy_wh.orders (id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES legacy_wh.products (id) ON DELETE RESTRICT,
    quantity integer NOT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    CONSTRAINT order_items_order_id_product_id_unique UNIQUE (order_id, product_id)
);

CREATE TABLE legacy_wh.order_item_barcodes (
    id bigint PRIMARY KEY,
    order_item_id uuid NOT NULL REFERENCES legacy_wh.order_items (id) ON DELETE CASCADE,
    product_barcode_id uuid NOT NULL REFERENCES legacy_wh.product_barcodes (id) ON DELETE RESTRICT,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    CONSTRAINT order_item_barcodes_order_item_id_product_barcode_id_unique UNIQUE (order_item_id, product_barcode_id)
);

-- ---------------------------------------------------------------- sentetik veri
-- UUID düzeni: 0000000X-0000-4000-8000-<tablo kodu><sıra>, okunabilir ve sabit.

-- 2 depo.
INSERT INTO legacy_wh.warehouses (id, code, name, country, city, district, is_active, sort_order, settings, created_at, updated_at) VALUES
    ('00000001-0000-4000-8000-000000000001', 'SYN-WH-1', 'Sentetik Merkez Depo', 'TR', 'İstanbul', 'Tuzla', true, 0, '{"location_view": "tree", "hide_site_picker_when_single": true, "default_site_id": "00000002-0000-4000-8000-000000000001"}', '2026-07-15 09:00:00', '2026-07-15 09:00:00'),
    ('00000001-0000-4000-8000-000000000002', 'SYN-WH-2', 'Sentetik Glorian Depo', 'TR', 'İstanbul', 'Pendik', true, 1, '{"location_view": "tree", "hide_site_picker_when_single": true, "default_site_id": "00000002-0000-4000-8000-000000000002"}', '2026-07-15 09:00:00', '2026-07-15 09:00:00');

INSERT INTO legacy_wh.warehouse_sites (id, warehouse_id, code, name, is_active, sort_order, created_at, updated_at) VALUES
    ('00000002-0000-4000-8000-000000000001', '00000001-0000-4000-8000-000000000001', 'DEF', 'Varsayılan', true, 0, '2026-07-15 09:00:00', '2026-07-15 09:00:00'),
    ('00000002-0000-4000-8000-000000000002', '00000001-0000-4000-8000-000000000002', 'DEF', 'Varsayılan', true, 0, '2026-07-15 09:00:00', '2026-07-15 09:00:00');

-- 5 konum: depo 1'de koridor > raf > göz, depo 2'de koridor > göz.
INSERT INTO legacy_wh.warehouse_locations (id, warehouse_id, warehouse_site_id, parent_id, type, code, name, full_code, barcode_payload, sort_order, is_active, created_at, updated_at) VALUES
    ('00000003-0000-4000-8000-000000000001', '00000001-0000-4000-8000-000000000001', '00000002-0000-4000-8000-000000000001', NULL, 'aisle', 'A', 'Koridor A', 'A', 'LOC:SYN-WH-1:A', 0, true, '2026-07-15 09:00:00', '2026-07-15 09:00:00'),
    ('00000003-0000-4000-8000-000000000002', '00000001-0000-4000-8000-000000000001', '00000002-0000-4000-8000-000000000001', '00000003-0000-4000-8000-000000000001', 'shelf', '01', NULL, 'A-01', 'LOC:SYN-WH-1:A-01', 0, true, '2026-07-15 09:00:00', '2026-07-15 09:00:00'),
    ('00000003-0000-4000-8000-000000000003', '00000001-0000-4000-8000-000000000001', '00000002-0000-4000-8000-000000000001', '00000003-0000-4000-8000-000000000002', 'bin', '01', NULL, 'A-01-01', 'LOC:SYN-WH-1:A-01-01', 0, true, '2026-07-15 09:00:00', '2026-07-15 09:00:00'),
    ('00000003-0000-4000-8000-000000000004', '00000001-0000-4000-8000-000000000002', '00000002-0000-4000-8000-000000000002', NULL, 'aisle', 'B', 'Koridor B', 'B', 'LOC:SYN-WH-2:B', 0, true, '2026-07-15 09:00:00', '2026-07-15 09:00:00'),
    ('00000003-0000-4000-8000-000000000005', '00000001-0000-4000-8000-000000000002', '00000002-0000-4000-8000-000000000002', '00000003-0000-4000-8000-000000000004', 'bin', '01', NULL, 'B-01', 'LOC:SYN-WH-2:B-01', 0, true, '2026-07-15 09:00:00', '2026-07-15 09:00:00');

INSERT INTO legacy_wh.brands (id, name, is_active, created_at, updated_at) VALUES
    ('00000004-0000-4000-8000-000000000001', 'Olex Films', true, '2026-07-17 09:00:00', '2026-07-17 09:00:00'),
    ('00000004-0000-4000-8000-000000000002', 'Glorian', true, '2026-07-17 09:00:00', '2026-07-17 09:00:00');

INSERT INTO legacy_wh.product_categories (id, connection_id, name, source, external_id, available_parts, is_active, last_synced_at, created_at, updated_at) VALUES
    ('00000005-0000-4000-8000-000000000001', NULL, 'Sentetik PPF', 'olexfilms', '1', '["hood", "roof", "front_bumper"]', true, '2026-07-23 21:00:00', '2026-07-15 09:00:00', '2026-07-23 21:00:00');

-- 2 ürün; SYN-PPF-GLOSS hub products(id=1) ile eşleşir (sku + external_id).
INSERT INTO legacy_wh.products (id, connection_id, category_id, brand_id, name, sku, source, external_id, barcode_short_code, description, warranty_duration, micron_thickness, price, is_active, uses_fixed_barcode, last_synced_at, created_at, updated_at) VALUES
    ('00000006-0000-4000-8000-000000000001', NULL, '00000005-0000-4000-8000-000000000001', '00000004-0000-4000-8000-000000000001', 'Sentetik PPF Parlak', 'SYN-PPF-GLOSS', 'olexfilms', '1', 'SPG', NULL, 60, 190, 1000.00, true, false, '2026-07-23 21:00:00', '2026-07-15 09:00:00', '2026-07-23 21:00:00'),
    ('00000006-0000-4000-8000-000000000002', NULL, '00000005-0000-4000-8000-000000000001', '00000004-0000-4000-8000-000000000002', 'Sentetik Glorian PPF', 'SYN-GLR-PPF', NULL, NULL, 'SGP', NULL, 120, 200, 1500.00, true, false, NULL, '2026-07-17 09:00:00', '2026-07-17 09:00:00');

-- 2 bayi müşterisi (hub bayilerinin depo aynası).
INSERT INTO legacy_wh.customers (id, merged_into_customer_id, source, external_id, dealer_code, name, email, phone, city, district, country, is_active, last_synced_at, created_at, updated_at) VALUES
    ('00000007-0000-4000-8000-000000000001', NULL, 'olexfilms', '1', 'SYN00001', 'Sentetik Bayi Bir', 'bayi1@example.test', '0555 000 01 01', 'İstanbul', 'Kadıköy', 'TR', true, '2026-07-23 21:00:00', '2026-07-23 21:00:00', '2026-07-23 21:00:00'),
    ('00000007-0000-4000-8000-000000000002', NULL, 'olexfilms', '2', 'SYN00002', 'Sentetik Bayi İki', 'bayi2@example.test', '0555 000 02 02', 'Ankara', 'Çankaya', 'TR', true, '2026-07-23 21:00:00', '2026-07-23 21:00:00', '2026-07-23 21:00:00');

-- 4 barkod; SYN-DUP-0001 hub stock_items.barcode ile çakışır.
INSERT INTO legacy_wh.product_barcodes (id, connection_id, product_id, code, external_id, external_status, external_location, external_dealer_id, customer_id, source, status, quantity_on_hand, warehouse_id, warehouse_location_id, printed_at, placed_at, last_synced_at, created_at, updated_at) VALUES
    ('00000008-0000-4000-8000-000000000001', NULL, '00000006-0000-4000-8000-000000000001', 'SYN-DUP-0001', NULL, NULL, NULL, NULL, NULL, 'generated', 'placed', 1, '00000001-0000-4000-8000-000000000001', '00000003-0000-4000-8000-000000000003', '2026-07-16 09:00:00', '2026-07-16 10:00:00', NULL, '2026-07-16 09:00:00', '2026-07-16 10:00:00'),
    ('00000008-0000-4000-8000-000000000002', NULL, '00000006-0000-4000-8000-000000000001', 'SYN-WH-0002', NULL, NULL, NULL, NULL, NULL, 'generated', 'placed', 1, '00000001-0000-4000-8000-000000000001', '00000003-0000-4000-8000-000000000003', '2026-07-16 09:00:00', '2026-07-16 10:00:00', NULL, '2026-07-16 09:00:00', '2026-07-16 10:00:00'),
    ('00000008-0000-4000-8000-000000000003', NULL, '00000006-0000-4000-8000-000000000002', 'SYN-WH-0003', NULL, NULL, NULL, NULL, NULL, 'generated', 'placed', 1, '00000001-0000-4000-8000-000000000002', '00000003-0000-4000-8000-000000000005', '2026-07-17 09:00:00', '2026-07-17 10:00:00', NULL, '2026-07-17 09:00:00', '2026-07-17 10:00:00'),
    ('00000008-0000-4000-8000-000000000004', NULL, '00000006-0000-4000-8000-000000000002', 'SYN-WH-0004', NULL, NULL, NULL, NULL, '00000007-0000-4000-8000-000000000002', 'generated', 'used', 1, '00000001-0000-4000-8000-000000000002', NULL, '2026-07-17 09:00:00', '2026-07-17 10:00:00', NULL, '2026-07-17 09:00:00', '2026-07-24 09:00:00');

INSERT INTO legacy_wh.bin_product_stocks (id, warehouse_location_id, product_id, quantity_on_hand, created_at, updated_at) VALUES
    ('00000009-0000-4000-8000-000000000001', '00000003-0000-4000-8000-000000000003', '00000006-0000-4000-8000-000000000001', 2, '2026-07-16 10:00:00', '2026-07-16 10:00:00'),
    ('00000009-0000-4000-8000-000000000002', '00000003-0000-4000-8000-000000000005', '00000006-0000-4000-8000-000000000002', 1, '2026-07-17 10:00:00', '2026-07-24 09:00:00'),
    ('00000009-0000-4000-8000-000000000003', '00000003-0000-4000-8000-000000000002', '00000006-0000-4000-8000-000000000002', 0, '2026-07-17 10:00:00', '2026-07-17 10:00:00');

-- 5 hareket: 4 yerleştirme + 1 sipariş çıkışı.
INSERT INTO legacy_wh.stock_movements (id, product_id, product_barcode_id, warehouse_id, warehouse_location_id, quantity_delta, movement_type, reference_type, reference_id, user_id, notes, created_at) VALUES
    ('0000000a-0000-4000-8000-000000000001', '00000006-0000-4000-8000-000000000001', '00000008-0000-4000-8000-000000000001', '00000001-0000-4000-8000-000000000001', '00000003-0000-4000-8000-000000000003', 1, 'placement', NULL, NULL, NULL, NULL, '2026-07-16 10:00:00'),
    ('0000000a-0000-4000-8000-000000000002', '00000006-0000-4000-8000-000000000001', '00000008-0000-4000-8000-000000000002', '00000001-0000-4000-8000-000000000001', '00000003-0000-4000-8000-000000000003', 1, 'placement', NULL, NULL, NULL, NULL, '2026-07-16 10:00:00'),
    ('0000000a-0000-4000-8000-000000000003', '00000006-0000-4000-8000-000000000002', '00000008-0000-4000-8000-000000000003', '00000001-0000-4000-8000-000000000002', '00000003-0000-4000-8000-000000000005', 1, 'placement', NULL, NULL, NULL, NULL, '2026-07-17 10:00:00'),
    ('0000000a-0000-4000-8000-000000000004', '00000006-0000-4000-8000-000000000002', '00000008-0000-4000-8000-000000000004', '00000001-0000-4000-8000-000000000002', '00000003-0000-4000-8000-000000000005', 1, 'placement', NULL, NULL, NULL, NULL, '2026-07-17 10:00:00'),
    ('0000000a-0000-4000-8000-000000000005', '00000006-0000-4000-8000-000000000002', '00000008-0000-4000-8000-000000000004', '00000001-0000-4000-8000-000000000002', '00000003-0000-4000-8000-000000000005', -1, 'order_out', 'App\Models\Order', '0000000b-0000-4000-8000-000000000001', NULL, 'Sentetik sipariş çıkışı', '2026-07-24 09:00:00');

-- 2 sipariş; SYN-WH-REF-0001 hub orders(id=2).external_reference ile eşleşir.
INSERT INTO legacy_wh.orders (id, warehouse_id, customer_id, delivery_method_id, status, syncs_to_inventory, external_order_id, external_reference, tracking_number, notes, confirmed_at, shipped_at, created_at, updated_at) VALUES
    ('0000000b-0000-4000-8000-000000000001', '00000001-0000-4000-8000-000000000002', '00000007-0000-4000-8000-000000000002', NULL, 'shipped', true, '2', 'SYN-WH-REF-0001', 'SYNTRK0002', NULL, '2026-07-24 08:00:00', '2026-07-24 09:00:00', '2026-07-24 07:00:00', '2026-07-24 09:00:00'),
    ('0000000b-0000-4000-8000-000000000002', '00000001-0000-4000-8000-000000000001', '00000007-0000-4000-8000-000000000001', NULL, 'draft', false, NULL, 'SYN-WH-REF-0002', NULL, 'Taslak', NULL, NULL, '2026-07-25 07:00:00', '2026-07-25 07:00:00');

INSERT INTO legacy_wh.order_items (id, order_id, product_id, quantity, created_at, updated_at) VALUES
    ('0000000c-0000-4000-8000-000000000001', '0000000b-0000-4000-8000-000000000001', '00000006-0000-4000-8000-000000000002', 1, '2026-07-24 07:00:00', '2026-07-24 07:00:00'),
    ('0000000c-0000-4000-8000-000000000002', '0000000b-0000-4000-8000-000000000002', '00000006-0000-4000-8000-000000000001', 1, '2026-07-25 07:00:00', '2026-07-25 07:00:00');

INSERT INTO legacy_wh.order_item_barcodes (id, order_item_id, product_barcode_id, created_at, updated_at) VALUES
    (1, '0000000c-0000-4000-8000-000000000001', '00000008-0000-4000-8000-000000000004', '2026-07-24 08:30:00', '2026-07-24 08:30:00');
