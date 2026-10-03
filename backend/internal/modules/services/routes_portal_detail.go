package services

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	serviceshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterPortalDetailRoutes mounts the portal service detail and PDF
// (TEC-239). /v1/portal/* accepts portal sessions only (aud=portal, realm
// middleware); the customer / fleet role holds services.read with scope
// customer and the use case returns only services the user owns in the
// domain brand (else 404). The PDF job is polled and downloaded through
// /v1/portal/exports/{uuid}.
func RegisterPortalDetailRoutes(
	mux *http.ServeMux,
	h *serviceshandler.PortalDetail,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	portal := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn, middleware.Authenticate(tokens, loader),
			middleware.RequirePermission(rbac.PermServicesRead))
	}
	mux.Handle("GET /v1/portal/services/{uuid}", portal(h.Get))
	mux.Handle("GET /v1/portal/services/{uuid}/pdf", portal(h.PDF))
}
