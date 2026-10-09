// Package handler is the HTTP layer of the e-invoice API (TEC-503,
// F5-08c): /v1/einvoices and the buyer invoice profile.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/ubl"
	einvoiceuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Error codes of the e-invoice API (frontend errors.json codes.<CODE>).
const (
	CodeSettingsRequired       = "EINVOICE_SETTINGS_REQUIRED"
	CodeSourceNotBillable      = "EINVOICE_SOURCE_NOT_BILLABLE"
	CodeSourceAlreadyInvoiced  = "EINVOICE_SOURCE_ALREADY_INVOICED"
	CodeBuyerProfileIncomplete = "EINVOICE_BUYER_PROFILE_INCOMPLETE"
	CodeInvalidStatus          = "EINVOICE_INVALID_STATUS"
	CodeNotArchived            = "EINVOICE_NOT_ARCHIVED"
	CodePDFNotReady            = "EINVOICE_PDF_NOT_READY"
)

// InvoiceSortSpec is the list contract of GET /v1/einvoices.
var InvoiceSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"issue_date": "issue_date", "number": "number", "payable": "payable", "status": "status",
		"created_at": "created_at",
	},
	Default: apiquery.SortField{Field: "issue_date", Desc: true},
}

// BillableSortSpec is the list contract of GET /v1/einvoices/billable
// (shared with the bulk run query).
var BillableSortSpec = einvoiceuc.BillableSortSpec

// Handler serves the e-invoice API.
type Handler struct {
	svc *einvoiceuc.Service
	q   *db.Queries
}

// New builds the handler.
func New(svc *einvoiceuc.Service, q *db.Queries) *Handler { return &Handler{svc: svc, q: q} }

func caller(r *http.Request) einvoiceuc.Caller {
	return einvoiceuc.Caller{
		Principal: authctx.MustPrincipal(r.Context()), Org: orgctx.MustScope(r.Context()),
		Meta: activity.MetaFromRequest(r),
	}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var (
		ve *einvoiceuc.ValidationError
		pe *einvoiceuc.ProfileError
		ue *ubl.Error
	)
	switch {
	case response.QueryValidation(w, r, err):
	case errors.As(err, &ve):
		details := make([]response.Detail, 0, len(ve.Issues))
		for _, i := range ve.Issues {
			details = append(details, response.Detail{Field: i.Field, Message: i.Message})
		}
		response.ValidationError(w, r, details)
	case errors.As(err, &pe):
		details := make([]response.Detail, 0, len(pe.Fields))
		for _, f := range pe.Fields {
			details = append(details, response.Detail{Field: f, Message: "is required", Code: "required"})
		}
		response.ErrorWithData(w, r, http.StatusUnprocessableEntity, CodeBuyerProfileIncomplete,
			"The buyer invoice profile is incomplete", details,
			map[string]any{"organization_uuid": pe.OrganizationUUID, "missing_fields": pe.Fields})
	case errors.As(err, &ue):
		var details []response.Detail
		if ue.Field != "" {
			details = []response.Detail{{Field: ue.Field, Message: ue.Message}}
		}
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, ue.Code, ue.Message, details)
	case errors.Is(err, einvoiceuc.ErrNotFound):
		response.NotFound(w, r, "E-invoice not found")
	case errors.Is(err, einvoiceuc.ErrForbidden):
		response.Forbidden(w, r, "Only the brand center issues e-invoices")
	case errors.Is(err, einvoiceuc.ErrSettingsRequired):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeSettingsRequired, "E-invoice settings are not configured")
	case errors.Is(err, einvoiceuc.ErrSourceNotBillable):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeSourceNotBillable, "The source cannot be invoiced")
	case errors.Is(err, einvoiceuc.ErrAlreadyInvoiced):
		response.Conflict(w, r, CodeSourceAlreadyInvoiced, "The source already has an active e-invoice")
	case errors.Is(err, einvoiceuc.ErrInvalidStatus):
		response.Conflict(w, r, CodeInvalidStatus, "The e-invoice status does not allow this action")
	case errors.Is(err, einvoiceuc.ErrNotArchived):
		response.Conflict(w, r, CodeNotArchived, "The e-invoice is not archived")
	case errors.Is(err, einvoiceuc.ErrPDFNotReady):
		response.Conflict(w, r, CodePDFNotReady, "The e-invoice PDF is not ready")
	case errors.Is(err, einvoiceuc.ErrStorageUnavailable):
		response.ServiceUnavailable(w, r, response.CodeInternalError, "Storage is not configured")
	default:
		response.InternalErr(w, r, err, "e-invoice request failed")
	}
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "E-invoice not found")
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "body", Message: "invalid JSON: " + err.Error()}})
		return false
	}
	return true
}

func qText(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func numeric(f *float64) pgtype.Numeric {
	var n pgtype.Numeric
	if f == nil {
		return n
	}
	_ = n.Scan(strconv.FormatFloat(*f, 'f', -1, 64))
	return n
}

func uuidList(values []string, field string) ([]uuid.UUID, error) {
	var out []uuid.UUID
	for _, v := range values {
		id, err := uuid.Parse(strings.TrimSpace(v))
		if err != nil {
			return nil, &einvoiceuc.ValidationError{Issues: []einvoiceuc.FieldIssue{{Field: field, Message: "must be UUIDs"}}}
		}
		out = append(out, id)
	}
	return out, nil
}

// List is GET /v1/einvoices.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	c := caller(r)
	vals := r.URL.Query()
	qp := apiquery.Parse(vals)
	sort, err := apiquery.ResolveSort(qp.Sort, InvoiceSortSpec)
	if err != nil {
		writeError(w, r, err)
		return
	}
	statuses, err := apiquery.EnumList(vals, "status", einvoiceuc.Statuses...)
	if err != nil {
		writeError(w, r, err)
		return
	}
	profiles, err := apiquery.EnumList(vals, "profile", einvoiceuc.Profiles...)
	if err != nil {
		writeError(w, r, err)
		return
	}
	issued, err := apiquery.DateRange(vals, "issue_date")
	if err != nil {
		writeError(w, r, err)
		return
	}
	payable, err := apiquery.NumRange(vals, "payable")
	if err != nil {
		writeError(w, r, err)
		return
	}
	buyerUUIDs, err := uuidList(apiquery.CSVValues(vals, "buyer"), "buyer")
	if err != nil {
		writeError(w, r, err)
		return
	}
	p := db.ListEinvoicesParams{
		Statuses: statuses, Profiles: profiles, PayableMin: numeric(payable.Min), PayableMax: numeric(payable.Max),
		Q: qText(qp.Q), SortKey: sort.Key, SortDesc: sort.Desc, RowLimit: qp.Limit, RowOffset: qp.Offset,
	}
	if issued.From != nil {
		p.IssueDateFrom = pgtype.Date{Time: *issued.From, Valid: true}
	}
	if issued.Before != nil {
		// Before is exclusive; issue_date is a date column.
		p.IssueDateTo = pgtype.Date{Time: issued.Before.AddDate(0, 0, -1), Valid: true}
	}
	if len(buyerUUIDs) > 0 {
		p.BuyerOrgIds = []int64{0} // unknown buyers match nothing
		for _, id := range buyerUUIDs {
			if o, err := h.q.GetOrganizationByUUID(r.Context(), id); err == nil && o.BrandID == c.Org.BrandID {
				p.BuyerOrgIds = append(p.BuyerOrgIds, o.ID)
			}
		}
	}
	items, total, err := h.svc.List(r.Context(), c, p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, qp.Limit, qp.Offset))
}

// Billable is GET /v1/einvoices/billable.
func (h *Handler) Billable(w http.ResponseWriter, r *http.Request) {
	qp := apiquery.Parse(r.URL.Query())
	p, err := einvoiceuc.ParseBillableQuery(r.URL.Query())
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.svc.ListBillable(r.Context(), caller(r), p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, qp.Limit, qp.Offset))
}

type createRequest struct {
	SourceType string `json:"source_type"`
	SourceUUID string `json:"source_uuid"`
}

// Create is POST /v1/einvoices.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if !decode(w, r, &req) {
		return
	}
	var details []response.Detail
	st := strings.TrimSpace(req.SourceType)
	if st != einvoiceuc.SourceOrder && st != einvoiceuc.SourceSubscription {
		details = append(details, response.Detail{Field: "source_type", Message: "must be one of " + strings.Join(einvoiceuc.SourceTypes, ", ")})
	}
	id, err := uuid.Parse(strings.TrimSpace(req.SourceUUID))
	if err != nil {
		details = append(details, response.Detail{Field: "source_uuid", Message: "must be a UUID"})
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	v, err := h.svc.CreateDraft(r.Context(), caller(r), einvoiceuc.DraftInput{SourceType: st, SourceUUID: id})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

// Get is GET /v1/einvoices/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Get(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

func writeHTML(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The invoice HTML is rendered from an XSLT; it runs no script.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src data:; style-src 'unsafe-inline'; font-src data:")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// Preview is GET /v1/einvoices/{uuid}/preview.
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	html, err := h.svc.Preview(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeHTML(w, html)
}

// Archive is POST /v1/einvoices/{uuid}/archive (step-up).
func (h *Handler) Archive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Archive(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

type voidRequest struct {
	Reason string `json:"reason"`
}

// Void is POST /v1/einvoices/{uuid}/void (step-up).
func (h *Handler) Void(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var req voidRequest
	if !decode(w, r, &req) {
		return
	}
	v, err := h.svc.Void(r.Context(), caller(r), id, einvoiceuc.VoidInput{Reason: req.Reason})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// RetryPDF is POST /v1/einvoices/{uuid}/pdf/retry.
func (h *Handler) RetryPDF(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.RetryPDF(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, v)
}

func stream(w http.ResponseWriter, f einvoiceuc.File) {
	defer func() { _ = f.Body.Close() }()
	w.Header().Set("Content-Type", f.ContentType)
	if f.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+f.Filename+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f.Body)
}

// XML is GET /v1/einvoices/{uuid}/xml.
func (h *Handler) XML(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	f, err := h.svc.XML(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	stream(w, f)
}

// PDF is GET /v1/einvoices/{uuid}/pdf.
func (h *Handler) PDF(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	f, err := h.svc.PDF(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	stream(w, f)
}

// HTML is GET /v1/einvoices/{uuid}/html.
func (h *Handler) HTML(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	html, err := h.svc.HTML(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeHTML(w, html)
}

// GetSettings is GET /v1/einvoices/settings.
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.GetSettings(r.Context(), caller(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// SamplePreview is GET /v1/einvoices/settings/xslt/preview.
func (h *Handler) SamplePreview(w http.ResponseWriter, r *http.Request) {
	html, err := h.svc.SamplePreview(r.Context(), caller(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeHTML(w, html)
}

type settingsRequest struct {
	VKN             string `json:"vkn"`
	TaxOffice       string `json:"tax_office"`
	LegalName       string `json:"legal_name"`
	Address         string `json:"address"`
	City            string `json:"city"`
	District        string `json:"district"`
	IBAN            string `json:"iban"`
	Email           string `json:"email"`
	Phone           string `json:"phone"`
	Website         string `json:"website"`
	TradeRegistryNo string `json:"trade_registry_no"`
	MersisNo        string `json:"mersis_no"`
	DefaultNote     string `json:"default_note"`
	EArchiveSeries  string `json:"earchive_series"`
	EFaturaSeries   string `json:"efatura_series"`
	PDFEnabled      *bool  `json:"pdf_enabled"`
}

// PutSettings is PUT /v1/einvoices/settings.
func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsRequest
	if !decode(w, r, &req) {
		return
	}
	pdf := true
	if req.PDFEnabled != nil {
		pdf = *req.PDFEnabled
	}
	v, err := h.svc.PutSettings(r.Context(), caller(r), einvoiceuc.SettingsInput{
		VKN: req.VKN, TaxOffice: req.TaxOffice, LegalName: req.LegalName, Address: req.Address, City: req.City,
		District: req.District, IBAN: req.IBAN, Email: req.Email, Phone: req.Phone, Website: req.Website,
		TradeRegistryNo: req.TradeRegistryNo, MersisNo: req.MersisNo, DefaultNote: req.DefaultNote,
		EArchiveSeries: req.EArchiveSeries, EFaturaSeries: req.EFaturaSeries, PDFEnabled: pdf,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// UploadXSLT is POST /v1/einvoices/settings/xslt (multipart "file", ≤ 1 MB).
func (h *Handler) UploadXSLT(w http.ResponseWriter, r *http.Request) {
	const limit = einvoiceuc.MaxXSLTBytes + (64 << 10)
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(limit); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "must be a multipart upload of at most 1 MB"}})
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "is required"}})
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, einvoiceuc.MaxXSLTBytes+1))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "could not be read"}})
		return
	}
	v, err := h.svc.UploadXSLT(r.Context(), caller(r), data)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// ResetXSLT is DELETE /v1/einvoices/settings/xslt.
func (h *Handler) ResetXSLT(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.ResetXSLT(r.Context(), caller(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// GetBuyerProfile is GET /v1/platform/organizations/{uuid}/invoice-profile.
func (h *Handler) GetBuyerProfile(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Organization not found")
		return
	}
	v, err := h.svc.GetBuyerProfile(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

type buyerProfileRequest struct {
	VKN                string `json:"invoice_vkn"`
	TCKN               string `json:"invoice_tckn"`
	TaxOffice          string `json:"invoice_tax_office"`
	LegalName          string `json:"invoice_legal_name"`
	EInvoiceRegistered bool   `json:"einvoice_registered"`
	EInvoiceAlias      string `json:"einvoice_alias"`
	Email              string `json:"invoice_email"`
}

// PutBuyerProfile is PUT /v1/platform/organizations/{uuid}/invoice-profile.
func (h *Handler) PutBuyerProfile(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Organization not found")
		return
	}
	var req buyerProfileRequest
	if !decode(w, r, &req) {
		return
	}
	v, err := h.svc.UpdateBuyerProfile(r.Context(), caller(r), id, einvoiceuc.BuyerProfileInput{
		VKN: req.VKN, TCKN: req.TCKN, TaxOffice: req.TaxOffice, LegalName: req.LegalName,
		EInvoiceRegistered: req.EInvoiceRegistered, EInvoiceAlias: req.EInvoiceAlias, Email: req.Email,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}
