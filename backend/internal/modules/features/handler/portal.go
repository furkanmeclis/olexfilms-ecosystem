package handler

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// portalFeaturesResponse is GET /v1/portal/features (TEC-1023).
type portalFeaturesResponse struct {
	Enabled []string `json:"enabled"`
}

// PortalList (GET /v1/portal/features): the customer-facing modules the
// portal session may use (features.Service.PortalModules has the rule).
func (h *Handler) PortalList(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	var brandID int64
	if b, ok := brandctx.From(r.Context()); ok {
		brandID = b.ID
	}
	keys, err := h.svc.PortalModules(r.Context(), p.UserInternal, brandID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, portalFeaturesResponse{Enabled: keys})
}
