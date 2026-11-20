-- TEC-1265, kullanıcı kararı 6 (TEC-1253): Glorian bayisi ile ekosistem
-- organizasyonu bayi koduyla eşleşir; telefon yalnız yedek eşleşmedir.
--
-- organizations.glorian_dealer_code: organizasyonun Glorian'daki bayi kodu.
-- Marka içinde büyük/küçük harf duyarsız tekildir (silinmiş kayıtlar hariç).
-- Yabancı anahtar yok; org birleştirme (000152) etkilenmez.
ALTER TABLE organizations
    ADD COLUMN glorian_dealer_code VARCHAR(64) NULL,
    ADD CONSTRAINT chk_organizations_glorian_dealer_code
        CHECK (glorian_dealer_code IS NULL OR btrim(glorian_dealer_code) <> '');

CREATE UNIQUE INDEX uq_organizations_brand_glorian_dealer_code
    ON organizations (brand_id, lower(glorian_dealer_code))
    WHERE glorian_dealer_code IS NOT NULL AND deleted_at IS NULL;

-- integration_external_parties: Glorian'dan çekilen bayi kodu ve elle
-- eşleme. linked_organization_id doluysa kod ve telefon eşleşmesinden önce
-- gelir; bir organizasyon bir bağlantıda en çok bir bayiye elle eşlenir.
-- Yabancı anahtar org birleştirme (orgmerge) için DEFERRABLE'dır.
ALTER TABLE integration_external_parties
    ADD COLUMN dealer_code            VARCHAR(64) NULL,
    ADD COLUMN linked_organization_id BIGINT      NULL
        REFERENCES organizations (id) ON DELETE SET NULL DEFERRABLE INITIALLY IMMEDIATE,
    ADD COLUMN linked_at              TIMESTAMPTZ NULL;

CREATE INDEX idx_integration_external_parties_dealer_code
    ON integration_external_parties (connection_id, lower(dealer_code))
    WHERE dealer_code IS NOT NULL;

CREATE UNIQUE INDEX uq_integration_external_parties_linked_org
    ON integration_external_parties (connection_id, linked_organization_id)
    WHERE linked_organization_id IS NOT NULL;

-- glorian_dealer_matches: her Glorian bayisinin eşleştiği organizasyon ve
-- eşleşme türü. Sipariş gönderimi (order_outbound link) ve yönetim ekranı
-- aynı görünümü okur; kural tek yerdedir. Sıra, organizasyon başına:
--   1. manual: bayi elle bir organizasyona eşlenmiş.
--   2. code:   organizasyonun glorian_dealer_code'u bayinin koduyla aynı
--              (büyük/küçük harf duyarsız) ve organizasyon elle eşlenmemiş.
--   3. phone:  organizasyonun kodu yok, elle eşlenmemiş ve telefonu bayinin
--              telefonuyla aynı (yedek eşleşme).
-- Bir organizasyona birden çok etkin bayi düşerse ya da bir bayinin
-- telefonu birden çok organizasyona uyarsa eşleşme "ambiguous" olur ve
-- organization_id boş kalır (sipariş bekletilir).
CREATE VIEW glorian_dealer_matches AS
WITH base AS (
    SELECT p.id AS party_id, p.connection_id, p.active,
           CASE
               WHEN p.linked_organization_id IS NOT NULL THEN 'manual'
               WHEN c.n = 1 THEN 'code'
               WHEN ph.n = 1 THEN 'phone'
               WHEN ph.n > 1 THEN 'ambiguous'
               ELSE 'none'
           END AS match_type,
           CASE
               WHEN p.linked_organization_id IS NOT NULL THEN p.linked_organization_id
               WHEN c.n = 1 THEN c.id
               WHEN ph.n = 1 THEN ph.id
           END AS organization_id
    FROM integration_external_parties p
    LEFT JOIN LATERAL (
        SELECT count(*) AS n, min(o.id) AS id
        FROM organizations o
        WHERE p.dealer_code IS NOT NULL
          AND o.brand_id = p.brand_id AND o.deleted_at IS NULL
          AND lower(o.glorian_dealer_code) = lower(p.dealer_code)
          AND NOT EXISTS (SELECT 1 FROM integration_external_parties x
                          WHERE x.connection_id = p.connection_id AND x.linked_organization_id = o.id)
    ) c ON true
    LEFT JOIN LATERAL (
        SELECT count(*) AS n, min(o.id) AS id
        FROM organizations o
        WHERE p.phone_e164 IS NOT NULL
          AND o.brand_id = p.brand_id AND o.deleted_at IS NULL
          AND o.glorian_dealer_code IS NULL
          AND o.phone = p.phone_e164
          AND NOT EXISTS (SELECT 1 FROM integration_external_parties x
                          WHERE x.connection_id = p.connection_id AND x.linked_organization_id = o.id)
    ) ph ON true
), counted AS (
    SELECT base.*,
           count(*) FILTER (WHERE active AND organization_id IS NOT NULL)
               OVER (PARTITION BY connection_id, organization_id) AS org_parties
    FROM base
)
SELECT party_id, connection_id,
       CASE WHEN active AND org_parties > 1 THEN 'ambiguous' ELSE match_type END AS match_type,
       CASE WHEN active AND org_parties > 1 THEN NULL ELSE organization_id END AS organization_id
FROM counted;
