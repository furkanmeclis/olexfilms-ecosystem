// Package geo mounts the TEC-84 geography routes: pickers, territories and
// plate formats.
package geo

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	geohandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/geo/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts geo routes. Pickers and plate validation are open to
// every signed-in user; writes need the platform permissions.
func RegisterRoutes(mux *http.ServeMux, h *geohandler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	authn := middleware.Authenticate(tokens, loader)
	signedIn := func(fn http.HandlerFunc) http.Handler { return middleware.Chain(fn, authn) }
	platform := func(fn http.HandlerFunc, slug string) http.Handler {
		return middleware.Chain(fn, authn, middleware.RequirePermission(slug))
	}

	mux.Handle("GET /v1/geo/countries", signedIn(h.Countries))
	mux.Handle("GET /v1/geo/countries/{iso2}/provinces", signedIn(h.Provinces))
	mux.Handle("GET /v1/geo/provinces/{id}/districts", signedIn(h.Districts))

	// Anonymous pickers (TEC-320) for the public dealer application form:
	// active countries only, reference data without personal fields.
	mux.HandleFunc("GET /v1/public/geo/countries", h.PublicCountries)
	mux.HandleFunc("GET /v1/public/geo/countries/{iso2}/provinces", h.Provinces)
	mux.HandleFunc("GET /v1/public/geo/provinces/{id}/districts", h.Districts)

	mux.Handle("PATCH /v1/platform/geo/countries/{iso2}", platform(h.PatchCountry, rbac.PermPlatformGeoWrite))
	mux.Handle("POST /v1/platform/geo/countries/{iso2}/provinces", platform(h.CreateProvince, rbac.PermPlatformGeoWrite))
	mux.Handle("DELETE /v1/platform/geo/provinces/{id}", platform(h.DeleteProvince, rbac.PermPlatformGeoWrite))
	mux.Handle("POST /v1/platform/geo/provinces/{id}/districts", platform(h.CreateDistrict, rbac.PermPlatformGeoWrite))
	mux.Handle("DELETE /v1/platform/geo/districts/{id}", platform(h.DeleteDistrict, rbac.PermPlatformGeoWrite))

	mux.Handle("GET /v1/platform/territories", platform(h.ListTerritories, rbac.PermPlatformTerritoriesRead))
	mux.Handle("GET /v1/platform/territories/resolve", platform(h.ResolveTerritory, rbac.PermPlatformTerritoriesRead))
	mux.Handle("POST /v1/platform/territories", platform(h.AssignTerritory, rbac.PermPlatformTerritoriesWrite))
	mux.Handle("DELETE /v1/platform/territories/{uuid}", platform(h.DeleteTerritory, rbac.PermPlatformTerritoriesWrite))

	mux.Handle("GET /v1/plate-formats", signedIn(h.PlateFormats))
	mux.Handle("POST /v1/plate-formats/validate", signedIn(h.ValidatePlate))
	mux.Handle("GET /v1/platform/plate-formats", platform(h.PlatformPlateFormats, rbac.PermPlatformSettingsRead))
	mux.Handle("POST /v1/platform/plate-formats", platform(h.CreatePlateFormat, rbac.PermPlatformSettingsWrite))
	mux.Handle("PATCH /v1/platform/plate-formats/{iso2}", platform(h.UpdatePlateFormat, rbac.PermPlatformSettingsWrite))
	mux.Handle("PUT /v1/platform/plate-formats/order", platform(h.ReorderPlateFormats, rbac.PermPlatformSettingsWrite))
	mux.Handle("DELETE /v1/platform/plate-formats/{iso2}", platform(h.DeletePlateFormat, rbac.PermPlatformSettingsWrite))
}
