// Package handler exposes contract template endpoints.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/otp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves contract template routes.
type Handler struct{ svc *usecase.Service }

// New creates a handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// Variables lists the allowed placeholders.
func (h *Handler) Variables(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, map[string]any{"items": h.svc.Variables()})
}

// List lists templates of the active brand.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	// TEC-369: kind (CSV), active and is_default (true|false; omitted = all).
	values := r.URL.Query()
	f := usecase.ListFilter{Kinds: apiquery.CSVValues(values, "kind")}
	var err error
	if f.Active, err = apiquery.Bool(values, "active"); err != nil {
		response.QueryValidation(w, r, err)
		return
	}
	if f.IsDefault, err = apiquery.Bool(values, "is_default"); err != nil {
		response.QueryValidation(w, r, err)
		return
	}
	items, err := h.svc.List(r.Context(), caller(r), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Get returns one template.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	item, err := h.svc.Get(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

type templateBody struct {
	Name              string `json:"name"`
	Kind              string `json:"kind"`
	IsDefault         *bool  `json:"is_default"`
	OTPRequired       *bool  `json:"otp_required"`
	SignatureRequired *bool  `json:"signature_required"`
	IsActive          *bool  `json:"is_active"`
}

func (b templateBody) input() usecase.Input {
	return usecase.Input{
		Name: b.Name, Kind: b.Kind, IsDefault: b.IsDefault, OTPRequired: b.OTPRequired,
		SignatureRequired: b.SignatureRequired, IsActive: b.IsActive,
	}
}

// Create creates a template.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in templateBody
	if !decode(w, r, &in) {
		return
	}
	item, err := h.svc.Create(r.Context(), caller(r), in.input())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

// Update patches a template.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in templateBody
	if !decode(w, r, &in) {
		return
	}
	item, err := h.svc.Update(r.Context(), caller(r), id, in.input())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

// Delete deletes unused templates and deactivates used templates.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	item, deleted, err := h.svc.Delete(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if deleted {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

// SetDefault makes one template default for its kind.
func (h *Handler) SetDefault(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	item, err := h.svc.SetDefault(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

type localeBody struct {
	LexicalJSON json.RawMessage `json:"lexical_json"`
	HTML        string          `json:"html"`
}

// PutLocale upserts one language version and bumps its version.
func (h *Handler) PutLocale(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in localeBody
	if !decode(w, r, &in) {
		return
	}
	item, err := h.svc.PutLocale(r.Context(), caller(r), id, usecase.LocaleInput{
		Locale: r.PathValue("locale"), LexicalJSON: in.LexicalJSON, HTML: in.HTML,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

type createContractBody struct {
	TemplateUUID *uuid.UUID `json:"template_uuid"`
	Locale       string     `json:"locale"`
}

// CreateForService creates a vehicle-intake contract from a service.
func (h *Handler) CreateForService(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body createContractBody
	if !decodeOptional(w, r, &body) {
		return
	}
	var tpl uuid.UUID
	if body.TemplateUUID != nil {
		tpl = *body.TemplateUUID
	}
	item, err := h.svc.CreateForService(r.Context(), caller(r), serviceID, usecase.CreateFromServiceInput{
		TemplateUUID: tpl, Locale: body.Locale,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

// GetContract returns content, signers and media.
func (h *Handler) GetContract(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	item, err := h.svc.GetContract(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

// PDF streams the ready executed contract PDF for panel users.
func (h *Handler) PDF(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	rc, name, err := h.svc.DownloadPDF(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+name+`"`)
	_, _ = io.Copy(w, rc)
}

// PortalPDF streams the ready executed contract PDF for the portal owner.
func (h *Handler) PortalPDF(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	rc, name, err := h.svc.DownloadPortalPDF(r.Context(), portalCaller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+name+`"`)
	_, _ = io.Copy(w, rc)
}

// RequestCustomerOTP sends a contract_sign OTP to the customer signer.
func (h *Handler) RequestCustomerOTP(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	res, err := h.svc.RequestCustomerOTP(r.Context(), caller(r), id, usecase.OTPInput{
		IP: clientIP(r), UserAgent: r.UserAgent(), Locale: r.Header.Get("Accept-Language"),
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, res)
}

type signBody struct {
	Code         string `json:"code"`
	SignaturePNG string `json:"signature_png"`
}

// SignCustomer verifies OTP when required and stores the canvas signature.
func (h *Handler) SignCustomer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body signBody
	if !decode(w, r, &body) {
		return
	}
	item, err := h.svc.SignCustomer(r.Context(), caller(r), id, usecase.SignatureInput{
		Code: body.Code, PNGBase64: body.SignaturePNG, IP: clientIP(r), UserAgent: r.UserAgent(),
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

// SignStaff stores the authenticated staff user's canvas signature.
func (h *Handler) SignStaff(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body signBody
	if !decode(w, r, &body) {
		return
	}
	item, err := h.svc.SignStaff(r.Context(), caller(r), id, usecase.SignatureInput{
		PNGBase64: body.SignaturePNG, IP: clientIP(r), UserAgent: r.UserAgent(),
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

// AddMedia attaches one jpeg/png/webp image.
func (h *Handler) AddMedia(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(usecase.MaxMediaBytes + (1 << 20)); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart body")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		file, header, err = r.FormFile("media")
	}
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	item, err := h.svc.AddMedia(r.Context(), caller(r), id, usecase.MediaInput{
		Body: file, Size: header.Size, Filename: header.Filename,
		Title: r.FormValue("title"), ContentType: header.Header.Get("Content-Type"),
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

// DeleteMedia removes one media object before execution.
func (h *Handler) DeleteMedia(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	mediaID, err := uuid.Parse(r.PathValue("media"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "media uuid is invalid")
		return
	}
	if err := h.svc.DeleteMedia(r.Context(), caller(r), id, mediaID); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type voidBody struct {
	Reason string `json:"reason"`
}

// Void voids a contract with a required reason.
func (h *Handler) Void(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body voidBody
	if !decode(w, r, &body) {
		return
	}
	item, err := h.svc.Void(r.Context(), caller(r), id, body.Reason)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func caller(r *http.Request) usecase.Caller {
	p := authctx.MustPrincipal(r.Context())
	org := orgctx.MustScope(r.Context())
	f, _ := scopefilter.From(r.Context())
	return usecase.Caller{UserID: p.UserInternal, OrganizationID: org.InternalID, BrandID: org.BrandID, Filter: f}
}

func portalCaller(r *http.Request) usecase.PortalCaller {
	p := authctx.MustPrincipal(r.Context())
	c := usecase.PortalCaller{UserID: p.UserInternal}
	if b, ok := brandctx.From(r.Context()); ok {
		c.BrandID = b.ID
	}
	return c
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid body")
		return false
	}
	return true
}

func decodeOptional(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		response.BadRequest(w, r, response.CodeValidationError, "invalid body")
		return false
	}
	return true
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	var unknown *usecase.UnknownVariablesError
	switch {
	case errors.As(err, &unknown):
		details := make([]response.Detail, 0, len(unknown.Keys))
		for _, k := range unknown.Keys {
			details = append(details, response.Detail{Field: "html", Message: "unknown variable: " + k})
		}
		response.ErrorWithDetails(w, r, http.StatusBadRequest, response.CodeValidationError, unknown.Error(), details)
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "contract not found")
	case errors.Is(err, usecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, usecase.ErrInvalidOTP), errors.Is(err, otp.ErrInvalidCode), errors.Is(err, otp.ErrTooManyAttempts):
		response.BadRequest(w, r, response.CodeValidationError, "invalid OTP code")
	case errors.Is(err, usecase.ErrWindowExpired):
		response.Error(w, r, http.StatusUnprocessableEntity, usecase.CodeContractSignWindowExpired, "contract sign window expired")
	case errors.Is(err, usecase.ErrUnsupportedStatus):
		response.Error(w, r, http.StatusUnprocessableEntity, usecase.CodeContractServiceStatus, "service status does not allow contract creation")
	case errors.Is(err, usecase.ErrAlreadySigned):
		response.Conflict(w, r, usecase.CodeContractAlreadySigned, "contract is already signed or locked")
	default:
		response.InternalErr(w, r, err, "contract request failed")
	}
}

func clientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		if i := strings.Index(xff, ","); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return xff
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		return strings.Trim(host[:i], "[]")
	}
	return host
}
