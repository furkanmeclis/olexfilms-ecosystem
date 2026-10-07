// Package handler exposes the fleet management API (TEC-473, F5-02b):
// /v1/fleets for dealers, distributors and the center, and the link
// decision of the fleet portal (/v1/portal/fleet/links).
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/usecase"
	importusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/imports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Exporter queues I/O engine exports (exports usecase).
type Exporter interface {
	RequestExport(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ExportFormat, query ioengine.ExportQuery, locale string) (exportusecase.ExportJobView, error)
}

// Importer stores uploaded import files (imports usecase).
type Importer interface {
	Upload(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ImportFormat, locale string, filename string, r io.Reader) (importusecase.ImportJobView, error)
	UpdateMapping(ctx context.Context, jobUUID uuid.UUID, actorID int64, orgID *int64, mapping, defaults map[string]string) (importusecase.ImportJobView, error)
	Sample(ctx context.Context, resource string, format ioengine.ImportFormat, locale string) ([]byte, string, error)
}

// Handler serves the fleet endpoints.
type Handler struct {
	svc     *usecase.Service
	exports Exporter
	imports Importer
}

// New creates the handler; exports and imports may be nil (503).
func New(svc *usecase.Service, exports Exporter, imports Importer) *Handler {
	return &Handler{svc: svc, exports: exports, imports: imports}
}

func caller(r *http.Request) usecase.Caller {
	p := authctx.MustPrincipal(r.Context())
	org := orgctx.MustScope(r.Context())
	f, _ := scopefilter.From(r.Context())
	return usecase.Caller{UserID: p.UserInternal, OrgID: org.InternalID, BrandID: org.BrandID, OrgType: org.OrgType, Filter: f}
}

// List is GET /v1/fleets (docs/list-contract.md): q, status (link status,
// CSV), vehicle_count_min/_max, sort name | vehicle_count |
// last_service_at | created_at (default name).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	q := apiquery.Parse(values)
	statuses, err := apiquery.EnumList(values, "status", model.LinkPending, model.LinkActive, model.LinkEnded)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	rng, err := apiquery.NumRange(values, "vehicle_count")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	f := usecase.ListFilter{Statuses: statuses, Q: q.Q, Sort: q.Sort, Limit: q.Limit, Offset: q.Offset}
	if rng.Min != nil {
		v := int64(*rng.Min)
		f.VehicleCountMin = &v
	}
	if rng.Max != nil {
		v := int64(*rng.Max)
		f.VehicleCountMax = &v
	}
	items, total, err := h.svc.List(r.Context(), caller(r), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Open is POST /v1/fleets.
func (h *Handler) Open(w http.ResponseWriter, r *http.Request) {
	var in usecase.OpenInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Open(r.Context(), caller(r), in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// Card is GET /v1/fleets/{uuid}.
func (h *Handler) Card(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.Card(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// RequestLink is POST /v1/fleets/{uuid}/links.
func (h *Handler) RequestLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.RequestLink(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// ListUsers is GET /v1/fleets/{uuid}/users: q, status (CSV), sort name |
// email | status | created_at (default created_at).
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	values := r.URL.Query()
	q := apiquery.Parse(values)
	statuses, err := apiquery.EnumList(values, "status", model.UserActive, model.UserDisabled)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	items, total, err := h.svc.ListUsers(r.Context(), caller(r), id, usecase.UserFilter{
		Q: q.Q, Statuses: statuses, Sort: q.Sort, Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// InviteUser is POST /v1/fleets/{uuid}/users.
func (h *Handler) InviteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in usecase.InviteInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.InviteUser(r.Context(), caller(r), id, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// DisableUser is POST /v1/fleets/{uuid}/users/{user_uuid}/disable.
func (h *Handler) DisableUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	uid, ok := pathUUID(w, r, "user_uuid")
	if !ok {
		return
	}
	if err := h.svc.DisableUser(r.Context(), caller(r), id, uid); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListVehicles is GET /v1/fleets/{uuid}/vehicles: q, sort plate |
// car_brand | last_service_at | active_warranty_count | created_at
// (default plate).
func (h *Handler) ListVehicles(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.ListVehicles(r.Context(), caller(r), id, usecase.VehicleFilter{
		Q: q.Q, Sort: q.Sort, Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// AddVehicle is POST /v1/fleets/{uuid}/vehicles: 201 for a new vehicle,
// 200 when an existing vehicle was linked.
func (h *Handler) AddVehicle(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in usecase.AddVehicleInput
	if !decode(w, r, &in) {
		return
	}
	out, created, err := h.svc.AddVehicle(r.Context(), caller(r), id, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	response.JSON(w, r, status, out)
}

// UpdateVehicle is PATCH /v1/fleets/{uuid}/vehicles/{vehicle_uuid}.
func (h *Handler) UpdateVehicle(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	vid, ok := pathUUID(w, r, "vehicle_uuid")
	if !ok {
		return
	}
	var in usecase.UpdateVehicleInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.UpdateVehicle(r.Context(), caller(r), id, vid, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// RemoveVehicle is DELETE /v1/fleets/{uuid}/vehicles/{vehicle_uuid}.
func (h *Handler) RemoveVehicle(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	vid, ok := pathUUID(w, r, "vehicle_uuid")
	if !ok {
		return
	}
	if err := h.svc.RemoveVehicle(r.Context(), caller(r), id, vid); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

const maxImportFile = 10 << 20

// ImportVehicles is POST /v1/fleets/{uuid}/vehicles/import (multipart:
// file, format csv | tsv | xlsx | json, locale). The job targets the fleet
// (defaults.fleet_uuid; keep it when the mapping is saved); preview (dry
// run), confirm and undo run on /v1/tenant/imports/{uuid}.
func (h *Handler) ImportVehicles(w http.ResponseWriter, r *http.Request) {
	if h.imports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "imports are not configured")
		return
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	c := caller(r)
	if err := h.svc.Check(r.Context(), c, id); err != nil {
		writeErr(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImportFile+(1<<20))
	if err := r.ParseMultipartForm(maxImportFile); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	format, ok := importFormat(w, r, r.FormValue("format"), ioengine.ImportCSV)
	if !ok {
		return
	}
	locale := strings.TrimSpace(r.FormValue("locale"))
	if locale == "" {
		locale = "tr"
	}
	orgID := c.OrgID
	job, err := h.imports.Upload(r.Context(), c.UserID, &orgID, usecase.ImportResource, format, locale, header.Filename, file)
	if err != nil {
		writeImportErr(w, r, err)
		return
	}
	mapping := job.Mapping
	if mapping == nil {
		mapping = map[string]string{}
	}
	job, err = h.imports.UpdateMapping(r.Context(), job.UUID, c.UserID, &orgID, mapping,
		map[string]string{usecase.ImportDefaultFleetUUID: id.String()})
	if err != nil {
		writeImportErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, job)
}

// ImportSample is GET /v1/fleets/vehicle-import/sample?format=&locale=.
func (h *Handler) ImportSample(w http.ResponseWriter, r *http.Request) {
	if h.imports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "imports are not configured")
		return
	}
	format, ok := importFormat(w, r, r.URL.Query().Get("format"), ioengine.ImportXLSX)
	if !ok {
		return
	}
	locale := strings.TrimSpace(r.URL.Query().Get("locale"))
	if locale == "" {
		locale = "tr"
	}
	data, ct, err := h.imports.Sample(r.Context(), usecase.ImportResource, format, locale)
	if err != nil {
		writeImportErr(w, r, err)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", "attachment; filename=\"fleet-vehicles-sample."+string(format)+"\"")
	_, _ = w.Write(data)
}

func importFormat(w http.ResponseWriter, r *http.Request, raw string, def ioengine.ImportFormat) (ioengine.ImportFormat, bool) {
	format := ioengine.ImportFormat(strings.ToLower(strings.TrimSpace(raw)))
	if format == "" {
		format = def
	}
	switch format {
	case ioengine.ImportCSV, ioengine.ImportTSV, ioengine.ImportXLSX, ioengine.ImportJSON:
		return format, true
	}
	response.BadRequest(w, r, response.CodeValidationError, "format is invalid")
	return "", false
}

func writeImportErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, importusecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, importusecase.ErrForbidden):
		response.Forbidden(w, r, "")
	default:
		response.InternalErr(w, r, err, "fleet import failed")
	}
}

// Statement is GET /v1/fleets/{uuid}/statement?period_from=&period_to=.
func (h *Handler) Statement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	p, err := usecase.ParsePeriod(r.URL.Query().Get("period_from"), r.URL.Query().Get("period_to"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out, err := h.svc.Statement(r.Context(), caller(r), id, p)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type statementExportBody struct {
	Format     string `json:"format"`
	PeriodFrom string `json:"period_from"`
	PeriodTo   string `json:"period_to"`
	Locale     string `json:"locale"`
}

// ExportStatement is POST /v1/fleets/{uuid}/statement/export (pdf | xlsx |
// csv): 202 with the export job; poll /v1/tenant/exports/{uuid}.
func (h *Handler) ExportStatement(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in statementExportBody
	if !decode(w, r, &in) {
		return
	}
	format := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(in.Format)))
	if format != ioengine.ExportPDF && format != ioengine.ExportXLSX && format != ioengine.ExportCSV {
		response.ValidationError(w, r, []response.Detail{{Field: "format", Message: "must be pdf, xlsx or csv"}})
		return
	}
	p, err := usecase.ParsePeriod(in.PeriodFrom, in.PeriodTo)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	c := caller(r)
	// Authorize now (own active link); the worker re-checks it.
	if _, err := h.svc.Statement(r.Context(), c, id, p); err != nil {
		writeErr(w, r, err)
		return
	}
	if in.Locale == "" {
		in.Locale = "tr"
	}
	orgID := c.OrgID
	job, err := h.exports.RequestExport(r.Context(), c.UserID, &orgID, usecase.ResourceStatement, format, ioengine.ExportQuery{
		usecase.QueryFleetUUID:  id.String(),
		usecase.QueryPeriodFrom: p.From.Format("2006-01-02"),
		usecase.QueryPeriodTo:   p.To.Format("2006-01-02"),
	}, in.Locale)
	if err != nil {
		if errors.Is(err, exportusecase.ErrInvalidRequest) {
			response.BadRequest(w, r, response.CodeValidationError, err.Error())
			return
		}
		response.InternalErr(w, r, err, "fleet statement export failed")
		return
	}
	response.JSON(w, r, http.StatusAccepted, job)
}

// --- Portal --------------------------------------------------------------------

// PortalLinks is GET /v1/portal/fleet/links.
func (h *Handler) PortalLinks(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	out, err := h.svc.PortalLinks(r.Context(), p.UserInternal)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(out, int64(len(out)), int32(len(out)), 0))
}

// AcceptLink is POST /v1/portal/fleet/links/{uuid}/accept.
func (h *Handler) AcceptLink(w http.ResponseWriter, r *http.Request) { h.decide(w, r, true) }

// RejectLink is POST /v1/portal/fleet/links/{uuid}/reject.
func (h *Handler) RejectLink(w http.ResponseWriter, r *http.Request) { h.decide(w, r, false) }

func (h *Handler) decide(w http.ResponseWriter, r *http.Request, accept bool) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	out, err := h.svc.DecideLink(r.Context(), p.UserInternal, id, accept)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// --- helpers -------------------------------------------------------------------

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	var (
		qe  *apiquery.ValidationError
		ve  *usecase.ValidationError
		fe  *usecase.FleetExistsError
		vx  *usecase.VehicleExistsError
		rul *usecase.RuleError
	)
	switch {
	case errors.As(err, &qe):
		details := make([]response.Detail, 0, len(qe.Details))
		for _, d := range qe.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.As(err, &fe):
		response.ErrorWithData(w, r, http.StatusConflict, model.CodeFleetExists,
			"a fleet with this tax number already exists; request a link", nil, fe)
	case errors.As(err, &vx):
		response.ErrorWithData(w, r, http.StatusConflict, model.CodeVehicleExists,
			"a vehicle with this plate or VIN already belongs to the fleet's users", nil, vx)
	case errors.As(err, &rul):
		response.Error(w, r, rul.Status(), rul.Code, rul.Message)
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "fleet not found")
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "only a dealer or a distributor does this")
	case errors.Is(err, usecase.ErrReportFilesUnavailable):
		response.ServiceUnavailable(w, r, response.CodeInternalError, "report storage is not configured")
	case errors.Is(err, repository.ErrLinkStale):
		response.Conflict(w, r, model.CodeLinkStale, "the link changed concurrently")
	default:
		response.InternalErr(w, r, err, "fleet request failed")
	}
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, name+" is invalid")
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			response.BadRequest(w, r, response.CodeValidationError, "body is required")
			return false
		}
		response.BadRequest(w, r, response.CodeValidationError, "invalid body")
		return false
	}
	return true
}
