package warranty

import (
	"net/http"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterPublicRoutes mounts the public warranty lookup (TEC-189). The
// frontend page /garanti/{public_code} reads it; no authentication, a per-IP
// limit of limit hits per window.
func RegisterPublicRoutes(mux *http.ServeMux, q *db.Queries, limiter *ratelimit.Limiter, limit int, window time.Duration) {
	h := handler.NewPublic(usecase.NewPublicLookup(q), limiter, limit, window)
	mux.HandleFunc("GET /v1/public/warranties/{public_code}", h.Get)
}

// RegisterCertificateRoutes mounts the warranty certificate PDF (TEC-188).
//
// Panel: POST /v1/services/{uuid}/warranty-certificate queues the export
// job; the service must be inside the caller's warranties.read scope
// (dealer: its organization, distributor: its subtree, center: the brand;
// else 404) and the organization gate applies (RequireOrganization).
// GET /v1/warranty-certificates/{uuid}[/download] polls and downloads the
// job of the active organization.
//
// Portal: POST /v1/portal/services/{uuid}/warranty-certificate is the
// signed-in customer's certificate (only the warranties they hold); the job
// is polled and downloaded through /v1/portal/exports/{uuid}.
func RegisterCertificateRoutes(
	mux *http.ServeMux,
	h *handler.Certificate,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	checker middleware.FeatureChecker,
) {
	authn := middleware.Authenticate(tokens, loader)
	tenant := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, middleware.RequireOrganization(tokens, q),
			middleware.RequireFeature(checker, features.ModuleServices),
			middleware.RequireScope(q, rbac.PermWarrantiesRead))
	}
	mux.Handle("POST /v1/services/{uuid}/warranty-certificate", tenant(h.RequestTenant))
	mux.Handle("GET /v1/warranty-certificates/{uuid}", tenant(h.GetTenant))
	mux.Handle("GET /v1/warranty-certificates/{uuid}/download", tenant(h.DownloadTenant))
	mux.Handle("POST /v1/portal/services/{uuid}/warranty-certificate", middleware.Chain(
		http.HandlerFunc(h.RequestPortal), authn, middleware.RequirePermission(rbac.PermWarrantiesRead)))
}
