package handler

// TEC-164: customer list export (I/O engine). Same permission and scope as
// GET /v1/customers (customers.read, RequireScope); the job stores the
// resolved scope and the worker re-authorizes it.

import (
	"net/http"
	"strings"

	cu "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

type listExportBody struct {
	Format string            `json:"format"`
	Query  map[string]string `json:"query"`
	Locale string            `json:"locale"`
}

func listExportFormat(raw string) (ioengine.ExportFormat, error) {
	f := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(raw)))
	switch f {
	case ioengine.ExportCSV, ioengine.ExportXLSX, ioengine.ExportPDF:
		return f, nil
	default:
		return "", &cu.ValidationError{Field: "format", Message: "must be csv, xlsx or pdf"}
	}
}

// RequestListExport (POST /v1/customers/export, 202): exports the customers
// of the caller's scope with the list filters and sort of GET /v1/customers.
func (h *Handler) RequestListExport(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	var b listExportBody
	if !decode(w, r, &b) {
		return
	}
	format, err := listExportFormat(b.Format)
	if err != nil {
		writeError(w, r, err)
		return
	}
	c := caller(r)
	scope, err := cu.ExportScope(c)
	if err != nil {
		writeError(w, r, err)
		return
	}
	// TEC-371: every list filter and the sort travel with the job; they are
	// validated here so a bad value is a 400 at request time.
	values := cu.ListExportValues(b.Query)
	f, err := cu.ParseListFilter(values)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if len(f.Q) > 100 {
		writeError(w, r, &cu.ValidationError{Field: "q", Message: "must be at most 100 characters"})
		return
	}
	query := ioengine.ExportQuery{cu.QueryScope: scope}
	for k := range values {
		query[k] = values.Get(k)
	}
	orgID := c.Org.InternalID
	job, err := h.exports.RequestExport(r.Context(), c.UserID, &orgID, cu.ResourceListExport, format, query,
		exportLocale(r, b.Locale))
	if err != nil {
		writeExportError(w, r, err)
		return
	}
	h.record(r, "customers.list_exported", "customers", &job.UUID, map[string]any{
		"format": string(format), "status": query[cu.QueryStatus], "has_query": query[cu.QueryQ] != "",
	})
	response.JSON(w, r, http.StatusAccepted, listExportJob(job))
}

// GetListExport (GET /v1/customer-list-exports/{uuid}).
func (h *Handler) GetListExport(w http.ResponseWriter, r *http.Request) {
	job, ok := h.listJob(w, r)
	if !ok {
		return
	}
	response.JSON(w, r, http.StatusOK, listExportJob(job))
}

// DownloadListExport (GET /v1/customer-list-exports/{uuid}/download).
func (h *Handler) DownloadListExport(w http.ResponseWriter, r *http.Request) {
	job, ok := h.listJob(w, r)
	if !ok {
		return
	}
	file, ct, filename, err := h.exports.DownloadOrg(r.Context(), job.UUID, orgctx.MustScope(r.Context()).InternalID)
	if err != nil {
		writeExportError(w, r, err)
		return
	}
	h.record(r, "customers.list_export_downloaded", "customers", &job.UUID, nil)
	sendFile(w, file, ct, filename)
}

func (h *Handler) listJob(w http.ResponseWriter, r *http.Request) (exportusecase.ExportJobView, bool) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return exportusecase.ExportJobView{}, false
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return exportusecase.ExportJobView{}, false
	}
	job, err := h.exports.GetOrgJob(r.Context(), id, orgctx.MustScope(r.Context()).InternalID)
	if err == nil && job.Resource != cu.ResourceListExport {
		err = exportusecase.ErrNotFound
	}
	if err != nil {
		writeExportError(w, r, err)
		return exportusecase.ExportJobView{}, false
	}
	return job, true
}

// listExportJob points the download link at the customers route.
func listExportJob(j exportusecase.ExportJobView) exportusecase.ExportJobView {
	if j.Download != nil {
		u := "/v1/customer-list-exports/" + j.UUID.String() + "/download"
		j.Download = &u
	}
	return j
}
