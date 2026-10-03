package glorian

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"

// ConnectionKey is the integration connection key of the Glorian hub.
const ConnectionKey = "glorian"

// OptionsFromConfig maps the env tuning onto client Options (BaseURL and
// APIKey are filled per connection by HTTPClientFactory).
func OptionsFromConfig(cfg config.GlorianConfig) Options {
	return Options{
		Timeout:        cfg.Timeout,
		WriteTimeout:   cfg.WriteTimeout,
		RetryBaseDelay: cfg.RetryBaseDelay,
		MaxRetryAfter:  cfg.MaxRetryAfter,
	}
}

// ConnectionFromConfig is the env-configured Glorian connection used by
// StaticSource until integration_connections exists (TEC-266). It is active
// only when explicitly enabled.
func ConnectionFromConfig(cfg config.GlorianConfig) Connection {
	return Connection{
		ID:      "env:" + ConnectionKey,
		Key:     ConnectionKey,
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Active:  cfg.Enabled,
	}
}
