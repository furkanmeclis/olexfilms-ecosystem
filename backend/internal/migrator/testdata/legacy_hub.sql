-- TEC-253: eski olexfilms hub (Laravel/MariaDB) şemasının PostgreSQL karşılığı.
--
-- Yalnız migrator test fixture'ıdır; golang-migrate'e GİRMEZ ve prod'da
-- çalışmaz. Kaynak: olexfilms database/schema/mysql-schema.sql (dump) +
-- dump sonrası migration'lar (country, locale, plate_country, tax_no,
-- latitude/longitude, google_business_url, review_request_sms_sent_at,
-- channel, external_reference, nexptg mobil alanları, hero_car_image_path,
-- logo_height, short_urls, service_customer_transfers; customers.fcm_token
-- düşürüldü).
--
-- Yalnız design.md §7'de taşınacak tablolar alınır. Kolon adları birebir;
-- tipler: bigint unsigned -> bigint, tinyint(1) -> boolean, int -> integer,
-- timestamp/datetime -> timestamp, longtext(json) -> jsonb, decimal -> numeric.
-- AUTO_INCREMENT yerine sabit id'ler yazılır (kaynak salt okunur).
--
-- VERİ TAMAMEN SENTETİKTİR: isimler, telefonlar, e-postalar (example.test),
-- barkodlar ve VIN'ler uydurmadır; gerçek kayıt içermez.
--
-- Tüm adlar şema nitelikli yazılır; yükleyici search_path'i pg_catalog'a
-- çeker, böylece nitelikisiz bir ad public'e düşemez.

DROP SCHEMA IF EXISTS legacy_hub CASCADE;
CREATE SCHEMA legacy_hub;

-- ---------------------------------------------------------------- yapı

CREATE TABLE legacy_hub.dealers (
    id bigint PRIMARY KEY,
    dealer_code varchar(8) DEFAULT NULL,
    name varchar(255) NOT NULL,
    email varchar(255) NOT NULL,
    phone varchar(255) NOT NULL,
    tax_no varchar(255) DEFAULT NULL,
    tax_office varchar(255) DEFAULT NULL,
    address text NOT NULL,
    facebook_url varchar(255) DEFAULT NULL,
    instagram_url varchar(255) DEFAULT NULL,
    twitter_url varchar(255) DEFAULT NULL,
    linkedin_url varchar(255) DEFAULT NULL,
    google_business_url varchar(255) DEFAULT NULL,
    website_url varchar(255) DEFAULT NULL,
    city varchar(255) DEFAULT NULL,
    district varchar(255) DEFAULT NULL,
    country varchar(255) DEFAULT 'TR',
    latitude numeric(10, 8) DEFAULT NULL,
    longitude numeric(11, 8) DEFAULT NULL,
    logo_path varchar(255) DEFAULT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    CONSTRAINT dealers_dealer_code_unique UNIQUE (dealer_code)
);
CREATE INDEX dealers_tax_no_index ON legacy_hub.dealers (tax_no);

CREATE TABLE legacy_hub.users (
    id bigint PRIMARY KEY,
    name varchar(255) NOT NULL,
    email varchar(255) NOT NULL,
    phone varchar(255) NOT NULL,
    avatar_url varchar(255) DEFAULT NULL,
    is_active boolean NOT NULL DEFAULT true,
    locale varchar(10) DEFAULT 'tr',
    email_verified_at timestamp NULL DEFAULT NULL,
    password varchar(255) NOT NULL,
    remember_token varchar(100) DEFAULT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    dealer_id bigint DEFAULT NULL REFERENCES legacy_hub.dealers (id) ON DELETE SET NULL,
    CONSTRAINT users_email_unique UNIQUE (email)
);

CREATE TABLE legacy_hub.roles (
    id bigint PRIMARY KEY,
    name varchar(255) NOT NULL,
    guard_name varchar(255) NOT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    CONSTRAINT roles_name_guard_name_unique UNIQUE (name, guard_name)
);

CREATE TABLE legacy_hub.model_has_roles (
    role_id bigint NOT NULL REFERENCES legacy_hub.roles (id) ON DELETE CASCADE,
    model_type varchar(255) NOT NULL,
    model_id bigint NOT NULL,
    PRIMARY KEY (role_id, model_id, model_type)
);
CREATE INDEX model_has_roles_model_id_model_type_index ON legacy_hub.model_has_roles (model_id, model_type);

CREATE TABLE legacy_hub.customers (
    id bigint PRIMARY KEY,
    dealer_id bigint DEFAULT NULL REFERENCES legacy_hub.dealers (id) ON DELETE CASCADE,
    created_by bigint NOT NULL REFERENCES legacy_hub.users (id) ON DELETE CASCADE,
    type varchar(255) NOT NULL,
    tc_no varchar(255) DEFAULT NULL,
    tax_no varchar(255) DEFAULT NULL,
    tax_office varchar(255) DEFAULT NULL,
    name varchar(255) NOT NULL,
    phone varchar(255) NOT NULL,
    email varchar(255) DEFAULT NULL,
    address text DEFAULT NULL,
    city varchar(255) DEFAULT NULL,
    district varchar(255) DEFAULT NULL,
    country varchar(255) DEFAULT 'TR',
    notification_settings jsonb DEFAULT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    deleted_at timestamp NULL DEFAULT NULL
);
CREATE INDEX customers_dealer_id_index ON legacy_hub.customers (dealer_id);
CREATE INDEX customers_created_by_index ON legacy_hub.customers (created_by);

CREATE TABLE legacy_hub.car_brands (
    id bigint PRIMARY KEY,
    name varchar(255) NOT NULL,
    external_id varchar(255) NOT NULL,
    logo varchar(255) DEFAULT NULL,
    hero_car_image_path varchar(255) DEFAULT NULL,
    last_update timestamp NULL DEFAULT NULL,
    is_active boolean NOT NULL DEFAULT true,
    show_name boolean NOT NULL DEFAULT true,
    logo_height integer NOT NULL DEFAULT 25,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    deleted_at timestamp NULL DEFAULT NULL,
    CONSTRAINT car_brands_external_id_unique UNIQUE (external_id)
);
CREATE INDEX car_brands_name_index ON legacy_hub.car_brands (name);

CREATE TABLE legacy_hub.car_models (
    id bigint PRIMARY KEY,
    brand_id bigint NOT NULL REFERENCES legacy_hub.car_brands (id) ON DELETE CASCADE,
    name varchar(255) NOT NULL,
    external_id varchar(255) NOT NULL,
    hero_car_image_path varchar(255) DEFAULT NULL,
    last_update timestamp NULL DEFAULT NULL,
    powertrain varchar(255) DEFAULT NULL,
    yearstart integer DEFAULT NULL,
    yearstop integer DEFAULT NULL,
    coupe varchar(255) DEFAULT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    deleted_at timestamp NULL DEFAULT NULL,
    CONSTRAINT car_models_external_id_unique UNIQUE (external_id)
);
CREATE INDEX car_models_brand_id_name_index ON legacy_hub.car_models (brand_id, name);

CREATE TABLE legacy_hub.product_categories (
    id bigint PRIMARY KEY,
    name varchar(255) NOT NULL,
    available_parts jsonb NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    deleted_at timestamp NULL DEFAULT NULL
);

CREATE TABLE legacy_hub.products (
    id bigint PRIMARY KEY,
    category_id bigint NOT NULL REFERENCES legacy_hub.product_categories (id) ON DELETE CASCADE,
    name varchar(255) NOT NULL,
    sku varchar(255) NOT NULL,
    description text DEFAULT NULL,
    warranty_duration integer DEFAULT NULL,
    micron_thickness integer DEFAULT NULL,
    price numeric(10, 2) NOT NULL,
    image_path varchar(255) DEFAULT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    deleted_at timestamp NULL DEFAULT NULL,
    CONSTRAINT products_sku_unique UNIQUE (sku)
);

CREATE TABLE legacy_hub.stock_items (
    id bigint PRIMARY KEY,
    product_id bigint NOT NULL REFERENCES legacy_hub.products (id) ON DELETE CASCADE,
    dealer_id bigint DEFAULT NULL REFERENCES legacy_hub.dealers (id) ON DELETE SET NULL,
    sku varchar(255) NOT NULL,
    barcode varchar(255) NOT NULL,
    location varchar(255) NOT NULL,
    status varchar(255) NOT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    CONSTRAINT stock_items_barcode_unique UNIQUE (barcode)
);
CREATE INDEX stock_items_updated_at_id_index ON legacy_hub.stock_items (updated_at, id);

CREATE TABLE legacy_hub.stock_movements (
    id bigint PRIMARY KEY,
    stock_item_id bigint NOT NULL REFERENCES legacy_hub.stock_items (id) ON DELETE CASCADE,
    user_id bigint NOT NULL REFERENCES legacy_hub.users (id) ON DELETE CASCADE,
    action varchar(255) NOT NULL,
    description text DEFAULT NULL,
    created_at timestamp NOT NULL
);

CREATE TABLE legacy_hub.orders (
    id bigint PRIMARY KEY,
    dealer_id bigint NOT NULL REFERENCES legacy_hub.dealers (id) ON DELETE CASCADE,
    created_by bigint NOT NULL REFERENCES legacy_hub.users (id) ON DELETE CASCADE,
    status varchar(255) NOT NULL DEFAULT 'pending',
    cargo_company varchar(255) DEFAULT NULL,
    tracking_number varchar(255) DEFAULT NULL,
    notes text DEFAULT NULL,
    external_reference varchar(255) DEFAULT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    CONSTRAINT orders_external_reference_unique UNIQUE (external_reference)
);
CREATE INDEX orders_dealer_id_status_index ON legacy_hub.orders (dealer_id, status);

CREATE TABLE legacy_hub.order_items (
    id bigint PRIMARY KEY,
    order_id bigint NOT NULL REFERENCES legacy_hub.orders (id) ON DELETE CASCADE,
    product_id bigint NOT NULL REFERENCES legacy_hub.products (id) ON DELETE CASCADE,
    quantity integer NOT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL
);

CREATE TABLE legacy_hub.order_item_stock (
    id bigint PRIMARY KEY,
    order_item_id bigint NOT NULL REFERENCES legacy_hub.order_items (id) ON DELETE CASCADE,
    stock_item_id bigint NOT NULL REFERENCES legacy_hub.stock_items (id) ON DELETE CASCADE,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    CONSTRAINT order_item_stock_order_item_id_stock_item_id_unique UNIQUE (order_item_id, stock_item_id)
);

CREATE TABLE legacy_hub.services (
    id bigint PRIMARY KEY,
    service_no varchar(255) NOT NULL,
    dealer_id bigint NOT NULL REFERENCES legacy_hub.dealers (id) ON DELETE CASCADE,
    customer_id bigint NOT NULL REFERENCES legacy_hub.customers (id) ON DELETE CASCADE,
    user_id bigint NOT NULL REFERENCES legacy_hub.users (id) ON DELETE CASCADE,
    car_brand_id bigint NOT NULL REFERENCES legacy_hub.car_brands (id) ON DELETE CASCADE,
    car_model_id bigint NOT NULL REFERENCES legacy_hub.car_models (id) ON DELETE CASCADE,
    year integer NOT NULL,
    vin varchar(255) DEFAULT NULL,
    plate varchar(255) NOT NULL,
    plate_country varchar(10) DEFAULT 'TR',
    km integer DEFAULT NULL,
    package varchar(255) DEFAULT NULL,
    applied_parts jsonb DEFAULT NULL,
    notes text DEFAULT NULL,
    status varchar(255) NOT NULL,
    completed_at timestamp NULL DEFAULT NULL,
    review_request_sms_sent_at timestamp NULL DEFAULT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    CONSTRAINT services_service_no_unique UNIQUE (service_no)
);

CREATE TABLE legacy_hub.service_items (
    id bigint PRIMARY KEY,
    service_id bigint NOT NULL REFERENCES legacy_hub.services (id) ON DELETE CASCADE,
    stock_item_id bigint NOT NULL REFERENCES legacy_hub.stock_items (id) ON DELETE CASCADE,
    usage_type varchar(255) NOT NULL,
    notes varchar(255) DEFAULT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL
);

CREATE TABLE legacy_hub.service_images (
    id bigint PRIMARY KEY,
    service_id bigint NOT NULL REFERENCES legacy_hub.services (id) ON DELETE CASCADE,
    image_path varchar(255) NOT NULL,
    title varchar(255) DEFAULT NULL,
    "order" integer NOT NULL DEFAULT 0,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL
);

CREATE TABLE legacy_hub.service_status_logs (
    id bigint PRIMARY KEY,
    service_id bigint NOT NULL REFERENCES legacy_hub.services (id) ON DELETE CASCADE,
    from_dealer_id bigint DEFAULT NULL REFERENCES legacy_hub.dealers (id) ON DELETE SET NULL,
    to_dealer_id bigint DEFAULT NULL REFERENCES legacy_hub.dealers (id) ON DELETE SET NULL,
    user_id bigint NOT NULL REFERENCES legacy_hub.users (id) ON DELETE CASCADE,
    notes text DEFAULT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL
);

CREATE TABLE legacy_hub.warranties (
    id bigint PRIMARY KEY,
    service_id bigint NOT NULL REFERENCES legacy_hub.services (id) ON DELETE CASCADE,
    stock_item_id bigint NOT NULL REFERENCES legacy_hub.stock_items (id) ON DELETE CASCADE,
    start_date date NOT NULL,
    end_date date NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL
);

CREATE TABLE legacy_hub.nexptg_api_users (
    id bigint PRIMARY KEY,
    user_id bigint DEFAULT NULL REFERENCES legacy_hub.users (id) ON DELETE CASCADE,
    username varchar(255) NOT NULL,
    password varchar(255) NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    last_used_at timestamp NULL DEFAULT NULL,
    created_by bigint DEFAULT NULL REFERENCES legacy_hub.users (id) ON DELETE SET NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    CONSTRAINT nexptg_api_users_username_unique UNIQUE (username),
    CONSTRAINT nexptg_api_users_user_id_unique UNIQUE (user_id)
);

CREATE TABLE legacy_hub.nexptg_reports (
    id bigint PRIMARY KEY,
    api_user_id bigint DEFAULT NULL REFERENCES legacy_hub.nexptg_api_users (id) ON DELETE CASCADE,
    user_id bigint DEFAULT NULL REFERENCES legacy_hub.users (id) ON DELETE SET NULL,
    external_id integer DEFAULT NULL,
    name varchar(255) NOT NULL,
    date timestamp NOT NULL,
    calibration_date timestamp NULL DEFAULT NULL,
    device_serial_number varchar(255) DEFAULT NULL,
    model varchar(255) DEFAULT NULL,
    car_model_id bigint DEFAULT NULL REFERENCES legacy_hub.car_models (id) ON DELETE SET NULL,
    brand varchar(255) DEFAULT NULL,
    car_brand_id bigint DEFAULT NULL REFERENCES legacy_hub.car_brands (id) ON DELETE SET NULL,
    type_of_body varchar(255) DEFAULT NULL,
    body_type varchar(255) DEFAULT NULL,
    capacity varchar(255) DEFAULT NULL,
    power varchar(255) DEFAULT NULL,
    vin varchar(255) DEFAULT NULL,
    fuel_type varchar(255) DEFAULT NULL,
    year varchar(255) DEFAULT NULL,
    unit_of_measure varchar(255) DEFAULT NULL,
    extra_fields jsonb DEFAULT NULL,
    comment text DEFAULT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    CONSTRAINT nexptg_reports_api_user_external_unique UNIQUE (api_user_id, external_id)
);

CREATE TABLE legacy_hub.nexptg_report_measurements (
    id bigint PRIMARY KEY,
    report_id bigint NOT NULL REFERENCES legacy_hub.nexptg_reports (id) ON DELETE CASCADE,
    is_inside boolean NOT NULL DEFAULT false,
    place_id varchar(255) NOT NULL,
    part_type varchar(255) NOT NULL,
    value numeric(10, 2) DEFAULT NULL,
    interpretation integer DEFAULT NULL,
    substrate_type varchar(255) DEFAULT NULL,
    "timestamp" timestamp NULL DEFAULT NULL,
    position integer DEFAULT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL
);
CREATE INDEX nexptg_report_measurements_report_id_index ON legacy_hub.nexptg_report_measurements (report_id);

CREATE TABLE legacy_hub.service_nexptg_report (
    id bigint PRIMARY KEY,
    service_id bigint NOT NULL REFERENCES legacy_hub.services (id) ON DELETE CASCADE,
    nexptg_report_id bigint NOT NULL REFERENCES legacy_hub.nexptg_reports (id) ON DELETE CASCADE,
    match_type varchar(255) NOT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    CONSTRAINT service_nexptg_report_nexptg_report_id_unique UNIQUE (nexptg_report_id)
);

CREATE TABLE legacy_hub.service_customer_transfers (
    id bigint PRIMARY KEY,
    service_id bigint NOT NULL REFERENCES legacy_hub.services (id) ON DELETE CASCADE,
    current_customer_id bigint NOT NULL REFERENCES legacy_hub.customers (id) ON DELETE CASCADE,
    new_customer_id bigint NOT NULL REFERENCES legacy_hub.customers (id) ON DELETE CASCADE,
    current_customer_code varchar(6) NOT NULL,
    new_customer_code varchar(6) NOT NULL,
    current_customer_verified boolean NOT NULL DEFAULT false,
    new_customer_verified boolean NOT NULL DEFAULT false,
    transferred_at timestamp NULL DEFAULT NULL,
    transferred_by bigint DEFAULT NULL REFERENCES legacy_hub.users (id) ON DELETE SET NULL,
    status varchar(255) NOT NULL DEFAULT 'pending',
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL
);

CREATE TABLE legacy_hub.short_urls (
    id bigint PRIMARY KEY,
    token varchar(16) NOT NULL,
    target_url varchar(2048) NOT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL,
    CONSTRAINT short_urls_token_unique UNIQUE (token)
);

-- Arşiv tabloları (yeni bildirim merkezine girmez, salt okunur taşınır).
-- sms_logs.bulk_sms_id: bulk_sms taşınmadığı için FK'sız tutulur.
CREATE TABLE legacy_hub.sms_logs (
    id bigint PRIMARY KEY,
    phone varchar(255) NOT NULL,
    message text NOT NULL,
    sender varchar(255) NOT NULL,
    message_type varchar(255) NOT NULL DEFAULT 'normal',
    message_content_type varchar(255) NOT NULL DEFAULT 'bilgi',
    channel varchar(255) NOT NULL DEFAULT 'sms',
    status varchar(255) NOT NULL,
    response_id bigint DEFAULT NULL,
    quantity integer DEFAULT NULL,
    amount numeric(10, 2) DEFAULT NULL,
    number_count integer DEFAULT NULL,
    description text DEFAULT NULL,
    response_data jsonb DEFAULT NULL,
    invalid_phones jsonb DEFAULT NULL,
    notifiable_type varchar(255) DEFAULT NULL,
    notifiable_id bigint DEFAULT NULL,
    bulk_sms_id bigint DEFAULT NULL,
    sent_by bigint DEFAULT NULL REFERENCES legacy_hub.users (id) ON DELETE SET NULL,
    sent_at timestamp NULL DEFAULT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL
);

CREATE TABLE legacy_hub.notifications (
    id char(36) PRIMARY KEY,
    type varchar(255) NOT NULL,
    notifiable_type varchar(255) NOT NULL,
    notifiable_id bigint NOT NULL,
    data text NOT NULL,
    read_at timestamp NULL DEFAULT NULL,
    created_at timestamp NULL DEFAULT NULL,
    updated_at timestamp NULL DEFAULT NULL
);

-- ---------------------------------------------------------------- sentetik veri

-- 3 bayi (biri pasif).
INSERT INTO legacy_hub.dealers (id, dealer_code, name, email, phone, tax_no, tax_office, address, city, district, country, latitude, longitude, is_active, created_at, updated_at) VALUES
    (1, 'SYN00001', 'Sentetik Bayi Bir', 'bayi1@example.test', '0555 000 01 01', '1111111111', 'Sentetik VD', 'Sentetik Cad. No:1', 'İstanbul', 'Kadıköy', 'TR', 40.99000000, 29.03000000, true, '2025-01-10 09:00:00', '2025-01-10 09:00:00'),
    (2, 'SYN00002', 'Sentetik Bayi İki', 'bayi2@example.test', '0555 000 02 02', NULL, NULL, 'Sentetik Cad. No:2', 'Ankara', 'Çankaya', 'TR', NULL, NULL, true, '2025-01-11 09:00:00', '2025-01-11 09:00:00'),
    (3, 'SYN00003', 'Sentetik Bayi Üç', 'bayi3@example.test', '0555 000 03 03', NULL, NULL, 'Sentetik Cad. No:3', 'İzmir', 'Konak', 'TR', NULL, NULL, false, '2025-01-12 09:00:00', '2025-01-12 09:00:00');

-- 5 user: süper admin, merkez personeli, iki bayi sahibi, bir bayi personeli.
INSERT INTO legacy_hub.users (id, name, email, phone, is_active, locale, password, created_at, updated_at, dealer_id) VALUES
    (1, 'Sentetik Süper Admin', 'admin@example.test', '05550000001', true, 'tr', '$2y$12$syntheticsyntheticsyntheticsyntheticsyntheticsynthe', '2025-01-01 08:00:00', '2025-01-01 08:00:00', NULL),
    (2, 'Sentetik Merkez Personeli', 'merkez@example.test', '05550000002', true, 'tr', '$2y$12$syntheticsyntheticsyntheticsyntheticsyntheticsynthe', '2025-01-01 08:00:00', '2025-01-01 08:00:00', NULL),
    (3, 'Sentetik Bayi Bir Sahibi', 'sahip1@example.test', '05550000003', true, 'tr', '$2y$12$syntheticsyntheticsyntheticsyntheticsyntheticsynthe', '2025-01-10 09:00:00', '2025-01-10 09:00:00', 1),
    (4, 'Sentetik Bayi Bir Personeli', 'personel1@example.test', '05550000004', true, 'en', '$2y$12$syntheticsyntheticsyntheticsyntheticsyntheticsynthe', '2025-01-10 09:00:00', '2025-01-10 09:00:00', 1),
    (5, 'Sentetik Bayi İki Sahibi', 'sahip2@example.test', '05550000005', false, 'tr', '$2y$12$syntheticsyntheticsyntheticsyntheticsyntheticsynthe', '2025-01-11 09:00:00', '2025-01-11 09:00:00', 2);

INSERT INTO legacy_hub.roles (id, name, guard_name, created_at, updated_at) VALUES
    (1, 'super_admin', 'web', '2025-01-01 08:00:00', '2025-01-01 08:00:00'),
    (2, 'center_staff', 'web', '2025-01-01 08:00:00', '2025-01-01 08:00:00'),
    (3, 'dealer_owner', 'web', '2025-01-01 08:00:00', '2025-01-01 08:00:00'),
    (4, 'dealer_staff', 'web', '2025-01-01 08:00:00', '2025-01-01 08:00:00');

INSERT INTO legacy_hub.model_has_roles (role_id, model_type, model_id) VALUES
    (1, 'App\Models\User', 1),
    (2, 'App\Models\User', 2),
    (3, 'App\Models\User', 3),
    (4, 'App\Models\User', 4),
    (3, 'App\Models\User', 5);

-- 8 müşteri. Telefon senaryoları (K26, K29):
--   çift 1: 1 ve 2 aynı numara (05551112233 / +90 555 111 22 33), farklı bayi;
--   çift 2: 3 ve 4 aynı numara ((0532) 444 55 66 / 905324445566);
--   5 telefonsuz (kolon NOT NULL olduğu için boş dize);
--   6 çözülemeyen ('12345');
--   7 kurumsal, ülke kodsuz 10 hane; 8 yurt dışı (DE).
INSERT INTO legacy_hub.customers (id, dealer_id, created_by, type, tc_no, tax_no, tax_office, name, phone, email, city, district, country, notification_settings, created_at, updated_at, deleted_at) VALUES
    (1, 1, 3, 'individual', NULL, NULL, NULL, 'Sentetik Müşteri Bir', '05551112233', 'musteri1@example.test', 'İstanbul', 'Kadıköy', 'TR', '{"sms": true, "push": false}', '2025-02-01 10:00:00', '2025-02-01 10:00:00', NULL),
    (2, 2, 5, 'individual', NULL, NULL, NULL, 'Sentetik Müşteri Bir (Ankara)', '+90 555 111 22 33', NULL, 'Ankara', 'Çankaya', 'TR', NULL, '2025-02-02 10:00:00', '2025-02-02 10:00:00', NULL),
    (3, 1, 4, 'individual', NULL, NULL, NULL, 'Sentetik Müşteri Üç', '(0532) 444 55 66', NULL, 'İstanbul', 'Üsküdar', 'TR', NULL, '2025-02-03 10:00:00', '2025-02-03 10:00:00', NULL),
    (4, 3, 1, 'individual', NULL, NULL, NULL, 'Sentetik Müşteri Üç (İzmir)', '905324445566', NULL, 'İzmir', 'Konak', 'TR', NULL, '2025-02-04 10:00:00', '2025-02-04 10:00:00', NULL),
    (5, 1, 3, 'individual', NULL, NULL, NULL, 'Sentetik Müşteri Telefonsuz', '', NULL, 'İstanbul', NULL, 'TR', NULL, '2025-02-05 10:00:00', '2025-02-05 10:00:00', NULL),
    (6, 2, 5, 'individual', NULL, NULL, NULL, 'Sentetik Müşteri Bozuk Telefon', '12345', NULL, 'Ankara', NULL, 'TR', NULL, '2025-02-06 10:00:00', '2025-02-06 10:00:00', NULL),
    (7, 1, 3, 'corporate', NULL, '2222222222', 'Sentetik VD', 'Sentetik Kurumsal A.Ş.', '5419876543', 'kurumsal@example.test', 'İstanbul', 'Ataşehir', 'TR', NULL, '2025-02-07 10:00:00', '2025-02-07 10:00:00', NULL),
    (8, 2, 5, 'individual', NULL, NULL, NULL, 'Sentetik Müşteri Yurt Dışı', '+49 151 23456789', NULL, 'Berlin', NULL, 'DE', NULL, '2025-02-08 10:00:00', '2025-02-08 10:00:00', '2025-06-01 10:00:00');

INSERT INTO legacy_hub.car_brands (id, name, external_id, logo, is_active, show_name, logo_height, created_at, updated_at) VALUES
    (1, 'Sentetik Marka A', 'syn-brand-a', 'car-brands/syn-a.png', true, true, 25, '2025-01-01 08:00:00', '2025-01-01 08:00:00'),
    (2, 'Sentetik Marka B', 'syn-brand-b', NULL, true, false, 30, '2025-01-01 08:00:00', '2025-01-01 08:00:00');

INSERT INTO legacy_hub.car_models (id, brand_id, name, external_id, powertrain, yearstart, yearstop, coupe, is_active, created_at, updated_at) VALUES
    (1, 1, 'Model A1', 'syn-model-a1', 'petrol', 2018, NULL, 'sedan', true, '2025-01-01 08:00:00', '2025-01-01 08:00:00'),
    (2, 1, 'Model A2', 'syn-model-a2', 'electric', 2021, NULL, 'suv', true, '2025-01-01 08:00:00', '2025-01-01 08:00:00'),
    (3, 2, 'Model B1', 'syn-model-b1', 'diesel', 2015, 2020, 'hatchback', true, '2025-01-01 08:00:00', '2025-01-01 08:00:00');

INSERT INTO legacy_hub.product_categories (id, name, available_parts, is_active, created_at, updated_at) VALUES
    (1, 'Sentetik PPF', '["hood", "roof", "front_bumper"]', true, '2025-01-01 08:00:00', '2025-01-01 08:00:00'),
    (2, 'Sentetik Cam Filmi', '["windshield"]', true, '2025-01-01 08:00:00', '2025-01-01 08:00:00');

-- 4 ürün; SYN-PPF-GLOSS depo products.sku ile eşleşir.
INSERT INTO legacy_hub.products (id, category_id, name, sku, description, warranty_duration, micron_thickness, price, is_active, created_at, updated_at) VALUES
    (1, 1, 'Sentetik PPF Parlak', 'SYN-PPF-GLOSS', 'Sentetik açıklama', 60, 190, 1000.00, true, '2025-01-01 08:00:00', '2025-01-01 08:00:00'),
    (2, 1, 'Sentetik PPF Mat', 'SYN-PPF-MATTE', NULL, 60, 190, 1200.00, true, '2025-01-01 08:00:00', '2025-01-01 08:00:00'),
    (3, 2, 'Sentetik Cam Filmi', 'SYN-WIN-01', NULL, 24, NULL, 300.00, true, '2025-01-01 08:00:00', '2025-01-01 08:00:00'),
    (4, 2, 'Sentetik Eski Ürün', 'SYN-OLD-01', NULL, NULL, NULL, 50.00, false, '2025-01-01 08:00:00', '2025-01-01 08:00:00');

-- 8 stok birimi; SYN-DUP-0001 depo product_barcodes.code ile çakışır.
INSERT INTO legacy_hub.stock_items (id, product_id, dealer_id, sku, barcode, location, status, created_at, updated_at) VALUES
    (1, 1, NULL, 'SYN-PPF-GLOSS', 'SYN-HUB-0001', 'center', 'available', '2025-03-01 09:00:00', '2025-03-01 09:00:00'),
    (2, 1, 1, 'SYN-PPF-GLOSS', 'SYN-HUB-0002', 'dealer', 'available', '2025-03-01 09:00:00', '2025-03-05 09:00:00'),
    (3, 1, 1, 'SYN-PPF-GLOSS', 'SYN-HUB-0003', 'service', 'used', '2025-03-01 09:00:00', '2025-03-10 09:00:00'),
    (4, 2, 1, 'SYN-PPF-MATTE', 'SYN-HUB-0004', 'service', 'used', '2025-03-01 09:00:00', '2025-03-12 09:00:00'),
    (5, 2, 2, 'SYN-PPF-MATTE', 'SYN-HUB-0005', 'service', 'used', '2025-03-01 09:00:00', '2025-03-15 09:00:00'),
    (6, 3, 2, 'SYN-WIN-01', 'SYN-HUB-0006', 'dealer', 'reserved', '2025-03-01 09:00:00', '2025-03-06 09:00:00'),
    (7, 4, NULL, 'SYN-OLD-01', 'SYN-HUB-0007', 'trash', 'used', '2025-03-01 09:00:00', '2025-03-20 09:00:00'),
    (8, 1, NULL, 'SYN-PPF-GLOSS', 'SYN-DUP-0001', 'center', 'available', '2025-03-01 09:00:00', '2025-03-01 09:00:00');

-- 16 hareket: 8 imported + 5 transferred_to_dealer + 3 used_in_service.
INSERT INTO legacy_hub.stock_movements (id, stock_item_id, user_id, action, description, created_at) VALUES
    (1, 1, 1, 'imported', 'Sentetik giriş', '2025-03-01 09:00:00'),
    (2, 2, 1, 'imported', NULL, '2025-03-01 09:00:00'),
    (3, 3, 1, 'imported', NULL, '2025-03-01 09:00:00'),
    (4, 4, 1, 'imported', NULL, '2025-03-01 09:00:00'),
    (5, 5, 1, 'imported', NULL, '2025-03-01 09:00:00'),
    (6, 6, 1, 'imported', NULL, '2025-03-01 09:00:00'),
    (7, 7, 1, 'imported', NULL, '2025-03-01 09:00:00'),
    (8, 8, 1, 'imported', NULL, '2025-03-01 09:00:00'),
    (9, 2, 2, 'transferred_to_dealer', 'Sipariş 1', '2025-03-05 09:00:00'),
    (10, 3, 2, 'transferred_to_dealer', 'Sipariş 1', '2025-03-05 09:00:00'),
    (11, 4, 2, 'transferred_to_dealer', 'Sipariş 1', '2025-03-05 09:00:00'),
    (12, 5, 2, 'transferred_to_dealer', NULL, '2025-03-06 09:00:00'),
    (13, 6, 2, 'transferred_to_dealer', 'Sipariş 2', '2025-03-06 09:00:00'),
    (14, 3, 3, 'used_in_service', 'SYN-S-0001', '2025-03-10 09:00:00'),
    (15, 4, 4, 'used_in_service', 'SYN-S-0003', '2025-03-12 09:00:00'),
    (16, 5, 5, 'used_in_service', 'SYN-S-0002', '2025-03-15 09:00:00');

-- 3 sipariş; 2 numaralı depo orders.external_reference ile eşleşir.
INSERT INTO legacy_hub.orders (id, dealer_id, created_by, status, cargo_company, tracking_number, notes, external_reference, created_at, updated_at) VALUES
    (1, 1, 3, 'delivered', 'Sentetik Kargo', 'SYNTRK0001', NULL, NULL, '2025-03-04 09:00:00', '2025-03-05 09:00:00'),
    (2, 2, 5, 'processing', NULL, NULL, 'Depoya iletildi', 'SYN-WH-REF-0001', '2025-03-06 09:00:00', '2025-03-06 09:00:00'),
    (3, 1, 4, 'pending', NULL, NULL, NULL, NULL, '2025-03-20 09:00:00', '2025-03-20 09:00:00');

INSERT INTO legacy_hub.order_items (id, order_id, product_id, quantity, created_at, updated_at) VALUES
    (1, 1, 1, 2, '2025-03-04 09:00:00', '2025-03-04 09:00:00'),
    (2, 1, 2, 1, '2025-03-04 09:00:00', '2025-03-04 09:00:00'),
    (3, 2, 3, 1, '2025-03-06 09:00:00', '2025-03-06 09:00:00'),
    (4, 3, 4, 1, '2025-03-20 09:00:00', '2025-03-20 09:00:00');

INSERT INTO legacy_hub.order_item_stock (id, order_item_id, stock_item_id, created_at, updated_at) VALUES
    (1, 1, 2, '2025-03-05 09:00:00', '2025-03-05 09:00:00'),
    (2, 1, 3, '2025-03-05 09:00:00', '2025-03-05 09:00:00'),
    (3, 2, 4, '2025-03-05 09:00:00', '2025-03-05 09:00:00'),
    (4, 3, 6, '2025-03-06 09:00:00', '2025-03-06 09:00:00');

-- 3 hizmet; müşteri 1 hem bayi 1 hem bayi 2'de hizmet almış. 3 numara VIN'siz.
INSERT INTO legacy_hub.services (id, service_no, dealer_id, customer_id, user_id, car_brand_id, car_model_id, year, vin, plate, plate_country, km, package, applied_parts, notes, status, completed_at, review_request_sms_sent_at, created_at, updated_at) VALUES
    (1, 'SYN-S-0001', 1, 1, 3, 1, 1, 2020, 'SYNVIN00000000001', '34 SYN 001', 'TR', 15000, 'full', '["hood", "roof"]', NULL, 'completed', '2025-03-10 17:00:00', '2025-03-11 10:00:00', '2025-03-10 09:00:00', '2025-03-10 17:00:00'),
    (2, 'SYN-S-0002', 2, 1, 5, 1, 2, 2022, 'SYNVIN00000000002', '06 SYN 002', 'TR', 5000, 'partial', '["front_bumper"]', 'Sentetik not', 'completed', '2025-03-15 17:00:00', NULL, '2025-03-15 09:00:00', '2025-03-15 17:00:00'),
    (3, 'SYN-S-0003', 1, 3, 4, 2, 3, 2016, NULL, '34 SYN 003', 'TR', NULL, NULL, NULL, NULL, 'processing', NULL, NULL, '2025-03-12 09:00:00', '2025-03-12 09:00:00');

INSERT INTO legacy_hub.service_items (id, service_id, stock_item_id, usage_type, notes, created_at, updated_at) VALUES
    (1, 1, 3, 'full', NULL, '2025-03-10 09:30:00', '2025-03-10 09:30:00'),
    (2, 2, 5, 'full', NULL, '2025-03-15 09:30:00', '2025-03-15 09:30:00'),
    (3, 3, 4, 'partial', 'Yarım rulo', '2025-03-12 09:30:00', '2025-03-12 09:30:00');

INSERT INTO legacy_hub.service_images (id, service_id, image_path, title, "order", created_at, updated_at) VALUES
    (1, 1, 'services/syn-s-0001/1.jpg', 'Önce', 0, '2025-03-10 10:00:00', '2025-03-10 10:00:00'),
    (2, 1, 'services/syn-s-0001/2.jpg', 'Sonra', 1, '2025-03-10 17:00:00', '2025-03-10 17:00:00'),
    (3, 2, 'services/syn-s-0002/1.jpg', NULL, 0, '2025-03-15 17:00:00', '2025-03-15 17:00:00');

INSERT INTO legacy_hub.service_status_logs (id, service_id, from_dealer_id, to_dealer_id, user_id, notes, created_at, updated_at) VALUES
    (1, 1, NULL, 1, 3, 'Oluşturuldu', '2025-03-10 09:00:00', '2025-03-10 09:00:00'),
    (2, 2, 1, 2, 2, 'Bayi değişti', '2025-03-15 09:00:00', '2025-03-15 09:00:00'),
    (3, 3, NULL, 1, 4, NULL, '2025-03-12 09:00:00', '2025-03-12 09:00:00');

-- 4 garanti; 1 ve 2 aynı hizmet + stok birimi için çift kayıt.
INSERT INTO legacy_hub.warranties (id, service_id, stock_item_id, start_date, end_date, is_active, created_at, updated_at) VALUES
    (1, 1, 3, '2025-03-10', '2030-03-10', true, '2025-03-10 17:00:00', '2025-03-10 17:00:00'),
    (2, 1, 3, '2025-03-10', '2030-03-10', true, '2025-03-10 17:00:05', '2025-03-10 17:00:05'),
    (3, 2, 5, '2025-03-15', '2030-03-15', true, '2025-03-15 17:00:00', '2025-03-15 17:00:00'),
    (4, 3, 4, '2025-03-12', '2030-03-12', false, '2025-03-12 17:00:00', '2025-03-12 17:00:00');

INSERT INTO legacy_hub.nexptg_api_users (id, user_id, username, password, is_active, last_used_at, created_by, created_at, updated_at) VALUES
    (1, 3, 'syn-device-1', '$2y$12$syntheticsyntheticsyntheticsyntheticsyntheticsynthe', true, '2025-03-10 08:00:00', 1, '2025-02-01 08:00:00', '2025-03-10 08:00:00'),
    (2, 5, 'syn-device-2', '$2y$12$syntheticsyntheticsyntheticsyntheticsyntheticsynthe', false, NULL, 1, '2025-02-01 08:00:00', '2025-02-01 08:00:00');

-- 2 ölçüm raporu; 2 numara mobil kaynaklı ve VIN'siz ("tamamlanacak").
INSERT INTO legacy_hub.nexptg_reports (id, api_user_id, user_id, external_id, name, date, calibration_date, device_serial_number, model, car_model_id, brand, car_brand_id, type_of_body, body_type, capacity, power, vin, fuel_type, year, unit_of_measure, extra_fields, comment, created_at, updated_at) VALUES
    (1, 1, NULL, 1001, 'SYN Rapor 1', '2025-03-10 08:30:00', '2025-01-01 00:00:00', 'SYN-SN-0001', 'Model A1', 1, 'Sentetik Marka A', 1, 'sedan', NULL, '1.6', '120', 'SYNVIN00000000001', 'petrol', '2020', 'um', '{"source": "device"}', NULL, '2025-03-10 08:35:00', '2025-03-10 08:35:00'),
    (2, NULL, 3, NULL, 'SYN Rapor 2', '2025-03-12 08:30:00', NULL, NULL, NULL, 3, NULL, 2, NULL, 'hatchback', NULL, NULL, NULL, NULL, '2016', 'um', NULL, 'Mobil ölçüm', '2025-03-12 08:35:00', '2025-03-12 08:35:00');

INSERT INTO legacy_hub.nexptg_report_measurements (id, report_id, is_inside, place_id, part_type, value, interpretation, substrate_type, "timestamp", position, created_at, updated_at) VALUES
    (1, 1, false, 'left', 'front_fender', 112.50, 1, 'Fe', '2025-03-10 08:31:00', 1, '2025-03-10 08:35:00', '2025-03-10 08:35:00'),
    (2, 1, false, 'top', 'hood', 118.00, 1, 'Fe', '2025-03-10 08:32:00', 2, '2025-03-10 08:35:00', '2025-03-10 08:35:00'),
    (3, 1, true, 'right', 'door', 240.75, 3, 'Al', '2025-03-10 08:33:00', 3, '2025-03-10 08:35:00', '2025-03-10 08:35:00'),
    (4, 2, false, 'left', 'rear_door', 105.00, 1, NULL, NULL, 1, '2025-03-12 08:35:00', '2025-03-12 08:35:00'),
    (5, 2, false, 'back', 'trunk', 99.25, 1, NULL, NULL, 2, '2025-03-12 08:35:00', '2025-03-12 08:35:00'),
    (6, 2, false, 'top', 'roof', NULL, NULL, NULL, NULL, 3, '2025-03-12 08:35:00', '2025-03-12 08:35:00');

INSERT INTO legacy_hub.service_nexptg_report (id, service_id, nexptg_report_id, match_type, created_at, updated_at) VALUES
    (1, 1, 1, 'before', '2025-03-10 09:00:00', '2025-03-10 09:00:00');

-- Araç devri: hizmet 1 müşteri 1'den kurumsal müşteri 7'ye.
INSERT INTO legacy_hub.service_customer_transfers (id, service_id, current_customer_id, new_customer_id, current_customer_code, new_customer_code, current_customer_verified, new_customer_verified, transferred_at, transferred_by, status, created_at, updated_at) VALUES
    (1, 1, 1, 7, '111111', '222222', true, true, '2025-04-01 12:00:00', 3, 'completed', '2025-04-01 11:00:00', '2025-04-01 12:00:00');

INSERT INTO legacy_hub.short_urls (id, token, target_url, created_at, updated_at) VALUES
    (1, 'synA1b2C3', 'https://hub.example.test/garanti/SYN-S-0001', '2025-03-10 17:00:00', '2025-03-10 17:00:00'),
    (2, 'synD4e5F6', 'https://hub.example.test/bayi/SYN00001', '2025-03-11 10:00:00', '2025-03-11 10:00:00'),
    -- TEC-263: the hub's real shapes. customer.notify (Crypt ile şifreli
    -- müşteri id'si) -> /portal; Google Business gibi dış hedef eşlenemez.
    (3, 'synG7h8J9', 'https://hub.example.test/customer/eyJpdiI6InN5bnRoZXRpYyJ9', '2025-03-12 10:00:00', '2025-03-12 10:00:00'),
    (4, 'synK0m1N2', 'https://g.page/r/SYNTHETIC-review', '2025-03-12 11:00:00', '2025-03-12 11:00:00');

INSERT INTO legacy_hub.sms_logs (id, phone, message, sender, message_type, message_content_type, channel, status, response_id, quantity, amount, number_count, description, response_data, invalid_phones, notifiable_type, notifiable_id, bulk_sms_id, sent_by, sent_at, created_at, updated_at) VALUES
    (1, '05551112233', 'Sentetik hizmet mesajı', 'SYNSENDER', 'normal', 'bilgi', 'sms', 'sent', 900001, 1, 0.10, 1, NULL, '{"ok": true}', NULL, 'App\Models\Customer', 1, NULL, 3, '2025-03-10 17:01:00', '2025-03-10 17:01:00', '2025-03-10 17:01:00'),
    (2, '05551112233', 'Sentetik değerlendirme isteği', 'SYNSENDER', 'normal', 'bilgi', 'whatsapp', 'sent', NULL, 1, NULL, 1, NULL, NULL, NULL, 'App\Models\Customer', 1, NULL, NULL, '2025-03-11 10:00:00', '2025-03-11 10:00:00', '2025-03-11 10:00:00'),
    (3, '12345', 'Sentetik başarısız mesaj', 'SYNSENDER', 'normal', 'bilgi', 'sms', 'failed', NULL, 0, NULL, 0, 'Geçersiz numara', NULL, '["12345"]', 'App\Models\Customer', 6, NULL, 5, NULL, '2025-03-12 10:00:00', '2025-03-12 10:00:00');

INSERT INTO legacy_hub.notifications (id, type, notifiable_type, notifiable_id, data, read_at, created_at, updated_at) VALUES
    ('00000000-0000-4000-8000-000000000001', 'Filament\Notifications\DatabaseNotification', 'App\Models\User', 3, '{"title": "Sentetik bildirim"}', NULL, '2025-03-05 09:00:00', '2025-03-05 09:00:00'),
    ('00000000-0000-4000-8000-000000000002', 'Filament\Notifications\DatabaseNotification', 'App\Models\User', 1, '{"title": "Sentetik okunmuş"}', '2025-03-06 09:00:00', '2025-03-05 09:00:00', '2025-03-06 09:00:00');
