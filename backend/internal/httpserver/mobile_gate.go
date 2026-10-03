package httpserver

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
)

// mobileAppPolicy resolves the TEC-236 app version gate per request: the
// stored mobile.* system settings, each empty one falling back to the
// MOBILE_APP_* environment. A nil settings service (no database) uses the
// environment only.
func mobileAppPolicy(settings *sysconfig.Service, cfg config.MobileConfig) func(*http.Request) middleware.AppVersionPolicy {
	env := sysconfig.MobileApp{
		MinVersion:      cfg.AppMinVersion,
		StoreURLIOS:     cfg.AppStoreURLIOS,
		StoreURLAndroid: cfg.AppStoreURLAndroid,
		VersionRequired: cfg.AppVersionRequired,
	}
	return func(r *http.Request) middleware.AppVersionPolicy {
		eff := env
		if settings != nil {
			eff = settings.MobileApp(r.Context()).WithFallback(env)
		}
		return middleware.AppVersionPolicy{
			MinVersion:      eff.MinVersion,
			StoreURLIOS:     eff.StoreURLIOS,
			StoreURLAndroid: eff.StoreURLAndroid,
			VersionRequired: eff.VersionRequired,
		}
	}
}
