DROP TRIGGER IF EXISTS trg_oauth_provider_settings_set_updated_at ON oauth_provider_settings;
DROP TABLE IF EXISTS oauth_provider_settings;

DROP TRIGGER IF EXISTS trg_auth_settings_set_updated_at ON auth_settings;
DROP TABLE IF EXISTS auth_settings;
