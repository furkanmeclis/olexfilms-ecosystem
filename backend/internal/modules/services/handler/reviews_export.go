package handler

import (
	"net/http"
	"strings"

	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// RequestReviewsExport (POST /v1/reviews/export): queues a CSV/XLSX/PDF/JSON
// export of the filtered review list for the active organization. Poll and
// download through /v1/tenant/exports/{uuid}.
func (h *Handler) RequestReviewsExport(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not available")
		return
	}
	var in struct {
		Format string            `json:"format"`
		Query  map[string]string `json:"query"`
		Locale string            `json:"locale"`
	}
	if !decode(w, r, &in) {
		return
	}
	format := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(in.Format)))
	if in.Locale == "" {
		in.Locale = "tr"
	}
	query := ioengine.ExportQuery(in.Query)
	if query == nil {
		query = ioengine.ExportQuery{}
	}
	for _, key := range []string{"dealer_uuid", "product_uuid", "min_rating", "max_rating", "created_from", "created_to"} {
		if v := r.URL.Query().Get(key); v != "" {
			query[key] = v
		}
	}
	scope, err := svcuc.ReviewExportScope(caller(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	query["scope"] = scope
	p := authctx.MustPrincipal(r.Context())
	orgID := orgctx.MustScope(r.Context()).InternalID
	job, err := h.exports.RequestExport(r.Context(), p.UserInternal, &orgID, svcuc.ResourceReviewsExport, format, query, in.Locale)
	if err != nil {
		response.InternalErr(w, r, err, "review export failed")
		return
	}
	response.JSON(w, r, http.StatusAccepted, job)
}
