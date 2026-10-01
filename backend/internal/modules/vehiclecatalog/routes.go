// Package vehiclecatalog mounts the TEC-149 vehicle catalog routes. Car
// brands and models are global reference data: reads need
// vehicle_catalog.read (every organization role, portal customers and
// fleets), writes vehicle_catalog.write (super_admin only). No organization
// context is required. Logo and hero images are public, unsigned and
// cacheable (ETag + Cache-Control) so the fixed /brand-logos/{uuid} URL works
// in <img> tags, e-mails and PDFs.
package vehiclecatalog

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the vehicle catalog routes.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	authn := middleware.Authenticate(tokens, loader)
	read := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, middleware.RequirePermission(rbac.PermVehicleCatalogRead))
	}
	write := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, middleware.RequirePermission(rbac.PermVehicleCatalogWrite))
	}

	// Public images (no auth).
	mux.HandleFunc("GET /v1/public/brand-logos/{uuid}", h.PublicBrandLogo)
	mux.HandleFunc("GET /v1/public/vehicle-heroes/default", h.PublicDefaultHero)
	mux.HandleFunc("GET /v1/public/vehicle-heroes/brands/{uuid}", h.PublicBrandHero)
	mux.HandleFunc("GET /v1/public/vehicle-heroes/models/{uuid}", h.PublicModelHero)

	// Panel reads (any organization role; super_admin also sees inactive rows).
	mux.Handle("GET /v1/vehicle-catalog/brands", read(h.ListBrands))
	mux.Handle("GET /v1/vehicle-catalog/brands/{uuid}", read(h.GetBrand))
	mux.Handle("GET /v1/vehicle-catalog/models", read(h.ListModels))
	mux.Handle("GET /v1/vehicle-catalog/models/{uuid}", read(h.GetModel))

	// Portal reads (customer / fleet sessions; active rows only).
	mux.Handle("GET /v1/portal/vehicle-catalog/brands", read(h.ListBrands))
	mux.Handle("GET /v1/portal/vehicle-catalog/models", read(h.ListModels))

	// super_admin writes.
	mux.Handle("POST /v1/platform/vehicle-catalog/brands", write(h.CreateBrand))
	mux.Handle("PATCH /v1/platform/vehicle-catalog/brands/{uuid}", write(h.UpdateBrand))
	mux.Handle("DELETE /v1/platform/vehicle-catalog/brands/{uuid}", write(h.DeleteBrand))
	mux.Handle("PUT /v1/platform/vehicle-catalog/brands/{uuid}/logo", write(h.UploadBrandLogo))
	mux.Handle("DELETE /v1/platform/vehicle-catalog/brands/{uuid}/logo", write(h.UploadBrandLogo))
	mux.Handle("PUT /v1/platform/vehicle-catalog/brands/{uuid}/hero", write(h.UploadBrandHero))
	mux.Handle("DELETE /v1/platform/vehicle-catalog/brands/{uuid}/hero", write(h.UploadBrandHero))
	mux.Handle("POST /v1/platform/vehicle-catalog/models", write(h.CreateModel))
	mux.Handle("PATCH /v1/platform/vehicle-catalog/models/{uuid}", write(h.UpdateModel))
	mux.Handle("DELETE /v1/platform/vehicle-catalog/models/{uuid}", write(h.DeleteModel))
	mux.Handle("PUT /v1/platform/vehicle-catalog/models/{uuid}/hero", write(h.UploadModelHero))
	mux.Handle("DELETE /v1/platform/vehicle-catalog/models/{uuid}/hero", write(h.UploadModelHero))
}
