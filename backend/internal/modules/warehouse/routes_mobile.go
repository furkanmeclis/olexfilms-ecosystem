package warehouse

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	whhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// MobileHandlers are the warehouse handlers mirrored on the mobile API.
type MobileHandlers struct {
	Tree      *whhandler.Handler
	Scan      *whhandler.ScanHandler
	Counts    *whhandler.Counts
	Transfers *whhandler.Transfers
}

// RegisterMobileRoutes mounts the TEC-235 mobile warehouse routes under
// /v1/mobile/warehouse/*: the warehouse lookup, the TEC-203 scan resolver,
// the TEC-206 stock counts and the TEC-205 warehouse transfers. A mobile
// token (aud=mobile) is accepted on /v1/mobile/* only (RealmAllows), so these
// are thin mirrors of the panel routes: the same handlers and use cases, the
// same permissions, warehouse module gate and active organization scope (a
// distributor reaches its own warehouses only, never the center's: K4, K20).
// Every route checks X-Mobile-Api-Version first.
func RegisterMobileRoutes(
	mux *http.ServeMux,
	h MobileHandlers,
	checker middleware.FeatureChecker,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	q *db.Queries,
	minVersion, maxVersion int,
) {
	version := middleware.MobileAPIVersion(minVersion, maxVersion)
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleWarehouse)
	scoped := func(fn http.HandlerFunc, slug string) http.Handler {
		return middleware.Chain(fn, version, authn, org, module, middleware.RequireScope(q, slug))
	}
	read := func(fn http.HandlerFunc) http.Handler { return scoped(fn, rbac.PermWarehouseRead) }
	write := func(fn http.HandlerFunc) http.Handler { return scoped(fn, rbac.PermWarehouseWrite) }

	const p = "/v1/mobile/warehouse"
	mux.Handle("GET "+p+"/warehouses", read(h.Tree.ListWarehouses))
	mux.Handle("POST "+p+"/scan", read(h.Scan.Scan))

	mux.Handle("GET "+p+"/stock-counts", read(h.Counts.List))
	mux.Handle("POST "+p+"/stock-counts", write(h.Counts.Create))
	mux.Handle("GET "+p+"/stock-counts/{uuid}", read(h.Counts.Get))
	mux.Handle("POST "+p+"/stock-counts/{uuid}/approve-start", write(h.Counts.ApproveStart))
	mux.Handle("POST "+p+"/stock-counts/{uuid}/start", write(h.Counts.Start))
	mux.Handle("GET "+p+"/stock-counts/{uuid}/scans", read(h.Counts.Scans))
	mux.Handle("POST "+p+"/stock-counts/{uuid}/scans", write(h.Counts.Scan))
	mux.Handle("DELETE "+p+"/stock-counts/{uuid}/scans/{scan_uuid}", write(h.Counts.DeleteScan))
	mux.Handle("POST "+p+"/stock-counts/{uuid}/complete", write(h.Counts.Complete))
	mux.Handle("GET "+p+"/stock-counts/{uuid}/report", read(h.Counts.Report))
	mux.Handle("POST "+p+"/stock-counts/{uuid}/approve", write(h.Counts.Approve))
	mux.Handle("POST "+p+"/stock-counts/{uuid}/cancel", write(h.Counts.Cancel))

	mux.Handle("GET "+p+"/transfers", read(h.Transfers.List))
	mux.Handle("POST "+p+"/transfers", write(h.Transfers.Create))
	mux.Handle("GET "+p+"/transfers/{uuid}", read(h.Transfers.Get))
	mux.Handle("POST "+p+"/transfers/{uuid}/lines", write(h.Transfers.AddLines))
	mux.Handle("DELETE "+p+"/transfers/{uuid}/lines/{line_uuid}", write(h.Transfers.DeleteLine))
	mux.Handle("POST "+p+"/transfers/{uuid}/place", write(h.Transfers.Place))
	mux.Handle("POST "+p+"/transfers/{uuid}/ship", write(h.Transfers.Ship))
	mux.Handle("POST "+p+"/transfers/{uuid}/complete", write(h.Transfers.Complete))
	mux.Handle("POST "+p+"/transfers/{uuid}/cancel", write(h.Transfers.Cancel))
}
