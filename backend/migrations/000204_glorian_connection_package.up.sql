-- TEC-1265: Glorian bağlantı paketi ve anahtar döndürme.
--
-- Paket üretilince yeni API anahtarı pending_api_key_enc'e yazılır; eski
-- anahtar (api_key_enc) Glorian'da yeni paket etkinleşene kadar kullanılır.
-- İstemci eski anahtarla 401 alınca bekleyen anahtarı dener; tutarsa bekleyen
-- anahtar etkin olur ve eskisi silinir. Bağlantının henüz anahtarı yoksa yeni
-- anahtar doğrudan api_key_enc'e yazılır.
ALTER TABLE integration_connections
    ADD COLUMN pending_api_key_enc   TEXT        NOT NULL DEFAULT '',
    ADD COLUMN pending_key_issued_at TIMESTAMPTZ NULL,
    ADD COLUMN package_issued_at     TIMESTAMPTZ NULL,
    ADD COLUMN package_expires_at    TIMESTAMPTZ NULL,
    ADD CONSTRAINT chk_integration_connections_package_window
        CHECK (package_expires_at IS NULL OR package_issued_at IS NULL OR package_expires_at > package_issued_at);
