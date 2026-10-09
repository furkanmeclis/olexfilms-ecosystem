package photostandard

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/photostandard/handler"
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
	module := middleware.RequireFeature(checker, features.ModulePhotoStandard)
	manage := middleware.RequireScope(q, rbac.PermPhotoStandardManage)
	override := middleware.RequireScope(q, rbac.PermPhotoStandardOverride)
	serviceRead := middleware.RequireScope(q, rbac.PermServicesRead)
	serviceWrite := middleware.RequireScope(q, rbac.PermServicesWrite)

	mux.Handle("GET /v1/platform/photo-standard/angles", middleware.Chain(http.HandlerFunc(h.ListAngles), authn, org, module, manage))
	mux.Handle("POST /v1/platform/photo-standard/angles", middleware.Chain(http.HandlerFunc(h.CreateAngle), authn, org, module, manage))
	mux.Handle("PUT /v1/platform/photo-standard/angles/{uuid}", middleware.Chain(http.HandlerFunc(h.UpdateAngle), authn, org, module, manage))
	mux.Handle("DELETE /v1/platform/photo-standard/angles/{uuid}", middleware.Chain(http.HandlerFunc(h.DeleteAngle), authn, org, module, manage))
	mux.Handle("POST /v1/platform/photo-standard/angles/{uuid}/example", middleware.Chain(http.HandlerFunc(h.UploadExample), authn, org, module, manage))
	// TEC-500: the example image is shown to everyone taking intake photos.
	mux.Handle("GET /v1/photo-standard/angles/{uuid}/example", middleware.Chain(http.HandlerFunc(h.Example), authn, org, module, serviceRead))

	mux.Handle("GET /v1/photo-standard/overrides", middleware.Chain(http.HandlerFunc(h.GetOverrides), authn, org, module, override))
	mux.Handle("PUT /v1/photo-standard/overrides", middleware.Chain(http.HandlerFunc(h.PutOverrides), authn, org, module, override))

	mux.Handle("GET /v1/services/{uuid}/intake-photos", middleware.Chain(http.HandlerFunc(h.Intake), authn, org, module, serviceRead))
	mux.Handle("POST /v1/services/{uuid}/intake-photos/{angle_key}", middleware.Chain(http.HandlerFunc(h.Upload), authn, org, module, serviceWrite))
	mux.Handle("DELETE /v1/services/{uuid}/intake-photos/{angle_key}", middleware.Chain(http.HandlerFunc(h.Delete), authn, org, module, serviceWrite))
	mux.Handle("GET /v1/services/{uuid}/intake-photos/{angle_key}/file", middleware.Chain(http.HandlerFunc(h.IntakeFile), authn, org, module, serviceRead))
}
