// Package dealershowcase mounts the dealer showcase API (TEC-467, F5-01b).
package dealershowcase

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealershowcase/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/showcase (panel editor of the active
// organization or, with ?org=, a dealer of the caller's scope; module
// dealer_showcase), /v1/platform/showcases (center review queue) and the
// public gallery files.
func RegisterRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	checker middleware.FeatureChecker,
) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleDealerShowcase)
	read := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, rbac.PermShowcaseRead))
	}
	write := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, rbac.PermShowcaseWrite))
	}
	review := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, middleware.RequireScope(q, rbac.PermPlatformShowcaseReview))
	}

	mux.Handle("GET /v1/showcase", read(h.Get))
	mux.Handle("PUT /v1/showcase", write(h.Save))
	mux.Handle("POST /v1/showcase/submit", write(h.Submit))
	// TEC-469: manual Google rating (Places not configured or no place id).
	mux.Handle("PUT /v1/showcase/google-rating", write(h.SetGoogleRating))
	mux.Handle("GET /v1/showcase/services", read(h.ListServices))
	mux.Handle("POST /v1/showcase/services", write(h.CreateService))
	mux.Handle("PUT /v1/showcase/services/order", write(h.ReorderServices))
	mux.Handle("PUT /v1/showcase/services/{uuid}", write(h.UpdateService))
	mux.Handle("DELETE /v1/showcase/services/{uuid}", write(h.DeleteService))
	mux.Handle("POST /v1/showcase/photos", write(h.UploadPhoto))
	mux.Handle("PUT /v1/showcase/photos/order", write(h.ReorderPhotos))
	mux.Handle("PUT /v1/showcase/photos/{uuid}", write(h.UpdatePhoto))
	mux.Handle("DELETE /v1/showcase/photos/{uuid}", write(h.DeletePhoto))
	mux.Handle("GET /v1/showcase/photos/{uuid}/file", read(h.PhotoFile))

	mux.Handle("GET /v1/platform/showcases", review(h.ListReviews))
	mux.Handle("GET /v1/platform/showcases/{org_uuid}", review(h.ReviewDetail))
	mux.Handle("POST /v1/platform/showcases/{org_uuid}/review", review(h.Review))

	mux.HandleFunc("GET /v1/public/dealers/{code}/photos/{uuid}", h.PublicPhoto)
}
