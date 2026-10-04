package library

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/library/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

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
	// The library is part of the F3-05 announcements and document centre module.
	module := middleware.RequireFeature(checker, features.ModuleAnnouncements)
	read := middleware.RequireScope(q, rbac.PermLibraryRead)
	manage := middleware.RequireScope(q, rbac.PermLibraryManage)

	mux.Handle("GET /v1/library/folders", middleware.Chain(http.HandlerFunc(h.ListFolders), authn, org, module, read))
	mux.Handle("POST /v1/library/folders", middleware.Chain(http.HandlerFunc(h.CreateFolder), authn, org, module, manage))
	mux.Handle("PATCH /v1/library/folders/{uuid}", middleware.Chain(http.HandlerFunc(h.PatchFolder), authn, org, module, manage))
	mux.Handle("DELETE /v1/library/folders/{uuid}", middleware.Chain(http.HandlerFunc(h.DeleteFolder), authn, org, module, manage))

	mux.Handle("GET /v1/library", middleware.Chain(http.HandlerFunc(h.ListItems), authn, org, module, read))
	mux.Handle("POST /v1/library/items", middleware.Chain(http.HandlerFunc(h.CreateItem), authn, org, module, manage))
	mux.Handle("PATCH /v1/library/items/{uuid}", middleware.Chain(http.HandlerFunc(h.PatchItem), authn, org, module, manage))
	mux.Handle("DELETE /v1/library/items/{uuid}", middleware.Chain(http.HandlerFunc(h.ArchiveItem), authn, org, module, manage))
	mux.Handle("POST /v1/library/items/{uuid}/versions", middleware.Chain(http.HandlerFunc(h.AddVersion), authn, org, module, manage))
	mux.Handle("GET /v1/library/items/{uuid}/versions", middleware.Chain(http.HandlerFunc(h.ListVersions), authn, org, module, read))
	mux.Handle("GET /v1/library/versions/{uuid}/download", middleware.Chain(http.HandlerFunc(h.Download), authn, org, module, read))
}
