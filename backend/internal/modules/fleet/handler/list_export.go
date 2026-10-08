package handler

// TEC-477: fleet list export (I/O engine). Same permission and scope as GET
// /v1/fleets (fleets.read, fleet module, RequireScope); the job stores the
// resolved scope and the worker re-authorizes it. Poll and download through
// /v1/tenant/exports/{uuid}.

import (
	"errors"
	"net/http"
	"strings"

	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

type listExportBody struct {
	Format string            `json:"format"`
	Query  map[string]string `json:"query"`
	Locale string            `json:"locale"`
}

// RequestListExport (POST /v1/fleets/export, 202): exports the fleets of
// the caller's scope with the filters, q and sort of GET /v1/fleets.
func (h *Handler) RequestListExport(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	var b listExportBody
	if !decode(w, r, &b) {
		return
	}
	format := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(b.Format)))
	switch format {
	case ioengine.ExportCSV, ioengine.ExportXLSX, ioengine.ExportPDF:
	default:
		response.ValidationError(w, r, []response.Detail{{Field: "format", Message: "must be csv, xlsx or pdf"}})
		return
	}
	c := caller(r)
	query, err := usecase.ListExportQuery(c, b.Query)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	locale := string(i18n.FromContext(r.Context()).Locale)
	if l, ok := i18n.Parse(b.Locale); ok {
		locale = string(l)
	}
	orgID := c.OrgID
	job, err := h.exports.RequestExport(r.Context(), c.UserID, &orgID, usecase.ResourceList, format, query, locale)
	if errors.Is(err, exportusecase.ErrInvalidRequest) {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "fleet list export failed")
		return
	}
	response.JSON(w, r, http.StatusAccepted, job)
}
