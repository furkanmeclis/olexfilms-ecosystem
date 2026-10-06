package warranty

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RegisterListRoutes mounts the TEC-191 warranty reads and the void.
//
// Panel (/v1/warranties): an organization session (RequireOrganization:
// suspended / expired = 403, read-only keeps GET) with the services module
// (warranties come from services) and warranties.read, scope resolved per
// request: center = brand, distributor = subtree, dealer = its
// organization; out of scope = 404. The void needs warranties.void (center
// roles only) and a fresh step-up.
//
// Portal (/v1/portal/warranties): a portal session (aud=portal) with
// warranties.read; the use case returns only the warranties the user holds
// in the domain brand.
//
// It returns the reader so the global search (TEC-213) runs the same list.
func RegisterListRoutes(
	mux *http.ServeMux,
	pool *pgxpool.Pool,
	q *db.Queries,
	frontendURL string,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	checker middleware.FeatureChecker,
	stepUp middleware.StepUpChecker,
	finder searchengine.ListFinder,
) *usecase.Reader {
	reader := usecase.NewReader(pool, q, outbox.NewStore(pool, q), frontendURL)
	// TEC-209: q searches the warranties index (nil finder: SQL only).
	if finder != nil {
		reader.SetFinder(finder)
	}
	h := handler.NewList(reader)
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleServices)
	read := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, rbac.PermWarrantiesRead))
	}
	void := middleware.Chain(http.HandlerFunc(h.Void), authn, org, module,
		middleware.RequireScope(q, rbac.PermWarrantiesVoid), middleware.RequireStepUp(stepUp))
	portal := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, authn, middleware.RequirePermission(rbac.PermWarrantiesRead))
	}

	mux.Handle("GET /v1/warranties", read(h.PanelList))
	mux.Handle("GET /v1/warranties/{uuid}", read(h.PanelGet))
	mux.Handle("POST /v1/warranties/{uuid}/void", void)
	mux.Handle("GET /v1/portal/warranties", portal(h.PortalList))
	mux.Handle("GET /v1/portal/warranties/{uuid}", portal(h.PortalGet))
	return reader
}

// RegisterListExportRoutes mounts POST /v1/warranties/export (TEC-377): the
// same session, module and warranties.read scope as GET /v1/warranties.
func RegisterListExportRoutes(
	mux *http.ServeMux,
	q *db.Queries,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	checker middleware.FeatureChecker,
	h *handler.ListExport,
) {
	mux.Handle("POST /v1/warranties/export", middleware.Chain(http.HandlerFunc(h.Request),
		middleware.Authenticate(tokens, loader), middleware.RequireOrganization(tokens, q),
		middleware.RequireFeature(checker, features.ModuleServices),
		middleware.RequireScope(q, rbac.PermWarrantiesRead)))
}
