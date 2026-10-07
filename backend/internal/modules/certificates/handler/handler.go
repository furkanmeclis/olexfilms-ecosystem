package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	certuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/certificates/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const CodeUnsupportedPDF = "CERTIFICATE_PDF_REQUIRED"

type Handler struct {
	svc *certuc.Service
	q   *db.Queries
}

func New(svc *certuc.Service, q *db.Queries) *Handler { return &Handler{svc: svc, q: q} }

func caller(r *http.Request) certuc.Caller {
	f, _ := scopefilter.From(r.Context())
	return certuc.Caller{Principal: authctx.MustPrincipal(r.Context()), Org: orgctx.MustScope(r.Context()), Filter: f}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, certuc.ErrNotFound):
		response.NotFound(w, r, "Certificate not found")
	case errors.Is(err, certuc.ErrForbidden):
		response.Forbidden(w, r, "This organization cannot perform this certificate action")
	case errors.Is(err, certuc.ErrUnsupportedMediaType):
		response.Error(w, r, http.StatusUnsupportedMediaType, CodeUnsupportedPDF, "Only PDF files are accepted")
	case errors.Is(err, certuc.ErrFileTooLarge):
		response.BadRequest(w, r, response.CodeValidationError, "PDF must be at most 10 MB")
	case errors.Is(err, certuc.ErrStorageRequired):
		response.ServiceUnavailable(w, r, response.CodeInternalError, "Storage is not configured")
	default:
		response.InternalErr(w, r, err, "certificate request failed")
	}
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.NotFound(w, r, "Certificate not found")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	c := caller(r)
	qp := apiquery.Parse(r.URL.Query())
	rows, err := h.q.ListCertificates(r.Context(), db.ListCertificatesParams{
		BrandID: c.Org.BrandID, OrgIds: c.Filter.OrgIDsArg(), Q: text(strings.TrimSpace(r.URL.Query().Get("q"))),
		SortKey: "expires_at", RowLimit: qp.Limit, RowOffset: qp.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	total, _ := h.q.CountCertificates(r.Context(), db.CountCertificatesParams{
		BrandID: c.Org.BrandID, OrgIds: c.Filter.OrgIDsArg(), Q: text(strings.TrimSpace(r.URL.Query().Get("q"))),
	})
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(rows, total, qp.Limit, qp.Offset))
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	const limit = certuc.MaxPDFBytes + (1 << 20)
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(limit); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid multipart form")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	userID, err := uuid.Parse(r.FormValue("user_uuid"))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "user_uuid", Message: "must be a UUID"}})
		return
	}
	typeID, err := uuid.Parse(r.FormValue("type_uuid"))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "type_uuid", Message: "must be a UUID"}})
		return
	}
	issued := time.Now().UTC()
	if raw := strings.TrimSpace(r.FormValue("issued_at")); raw != "" {
		issued, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "issued_at", Message: "must be RFC3339"}})
			return
		}
	}
	var expires *time.Time
	if raw := strings.TrimSpace(r.FormValue("expires_at")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "expires_at", Message: "must be RFC3339"}})
			return
		}
		expires = &t
	}
	v, err := h.svc.Upload(r.Context(), caller(r), certuc.UploadInput{
		UserUUID: userID, TypeUUID: typeID, IssuedAt: issued, ExpiresAt: expires,
		Filename: header.Filename, Body: file, Size: header.Size,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	rc, size, err := h.svc.Download(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Content-Disposition", `inline; filename="certificate.pdf"`)
	_, _ = io.Copy(w, rc)
}

func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) { h.status(w, r, "verify") }
func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) { h.status(w, r, "reject") }
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) { h.status(w, r, "revoke") }

func (h *Handler) status(w http.ResponseWriter, r *http.Request, action string) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var v certuc.CertificateView
	var err error
	switch action {
	case "verify":
		v, err = h.svc.Verify(r.Context(), caller(r), id)
	case "reject":
		v, err = h.svc.Reject(r.Context(), caller(r), id, r.FormValue("reason"))
	case "revoke":
		v, err = h.svc.Revoke(r.Context(), caller(r), id)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

func (h *Handler) ListWarnings(w http.ResponseWriter, r *http.Request) {
	c := caller(r)
	qp := apiquery.Parse(r.URL.Query())
	rows, err := h.q.ListServiceCertificateWarnings(r.Context(), db.ListServiceCertificateWarningsParams{
		BrandID: c.Org.BrandID, OrgIds: c.Filter.OrgIDsArg(), Q: text(strings.TrimSpace(r.URL.Query().Get("q"))),
		SortKey: "created_at", RowLimit: qp.Limit, RowOffset: qp.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	total, _ := h.q.CountServiceCertificateWarnings(r.Context(), db.CountServiceCertificateWarningsParams{
		BrandID: c.Org.BrandID, OrgIds: c.Filter.OrgIDsArg(), Q: text(strings.TrimSpace(r.URL.Query().Get("q"))),
	})
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(rows, total, qp.Limit, qp.Offset))
}

func (h *Handler) ApproveWarning(w http.ResponseWriter, r *http.Request) { h.decideWarning(w, r, true) }
func (h *Handler) RejectWarning(w http.ResponseWriter, r *http.Request)  { h.decideWarning(w, r, false) }

func (h *Handler) decideWarning(w http.ResponseWriter, r *http.Request, approve bool) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	row, err := h.svc.DecideWarning(r.Context(), caller(r), id, approve, r.FormValue("note"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, row)
}

func (h *Handler) Coverage(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, map[string]any{"items": []any{}})
}

type typeBody struct {
	Name           json.RawMessage `json:"name"`
	Description    json.RawMessage `json:"description"`
	ValidityMonths *int32          `json:"validity_months"`
	Active         *bool           `json:"active"`
	SortOrder      int32           `json:"sort_order"`
	CategoryUUIDs  []string        `json:"category_uuids"`
	ProductUUIDs   []string        `json:"product_uuids"`
}

func (h *Handler) ListTypes(w http.ResponseWriter, r *http.Request) {
	c := caller(r)
	qp := apiquery.Parse(r.URL.Query())
	rows, err := h.q.ListCertificateTypes(r.Context(), db.ListCertificateTypesParams{
		BrandID: c.Org.BrandID, Q: text(qp.Q), SortKey: "sort_order", RowLimit: qp.Limit, RowOffset: qp.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	total, _ := h.q.CountCertificateTypes(r.Context(), db.CountCertificateTypesParams{BrandID: c.Org.BrandID, Q: text(qp.Q)})
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(rows, total, qp.Limit, qp.Offset))
}

func (h *Handler) CreateType(w http.ResponseWriter, r *http.Request) {
	var b typeBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	c := caller(r)
	center, err := h.q.GetBrandCenter(r.Context(), c.Org.BrandID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	active := true
	if b.Active != nil {
		active = *b.Active
	}
	row, err := h.q.CreateCertificateType(r.Context(), db.CreateCertificateTypeParams{
		OrganizationID: center.ID, BrandID: c.Org.BrandID, Name: b.Name, Description: emptyObject(b.Description),
		ValidityMonths: int4ptr(b.ValidityMonths), Active: active, SortOrder: b.SortOrder,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := h.replaceBindings(r, center.ID, row.ID, b); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, row)
}

func (h *Handler) UpdateType(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b typeBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	c := caller(r)
	cur, err := h.q.GetCertificateTypeByUUID(r.Context(), db.GetCertificateTypeByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "Certificate type not found")
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	active := cur.Active
	if b.Active != nil {
		active = *b.Active
	}
	row, err := h.q.UpdateCertificateType(r.Context(), db.UpdateCertificateTypeParams{
		ID: cur.ID, BrandID: cur.BrandID, Name: b.Name, Description: emptyObject(b.Description),
		ValidityMonths: int4ptr(b.ValidityMonths), Active: active, SortOrder: b.SortOrder,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := h.replaceBindings(r, cur.OrganizationID, row.ID, b); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, row)
}

func (h *Handler) DeleteType(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	c := caller(r)
	cur, err := h.q.GetCertificateTypeByUUID(r.Context(), db.GetCertificateTypeByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "Certificate type not found")
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	if _, err := h.q.DeleteCertificateType(r.Context(), db.DeleteCertificateTypeParams{ID: cur.ID, BrandID: cur.BrandID}); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) replaceBindings(r *http.Request, orgID, typeID int64, b typeBody) error {
	c := caller(r)
	if len(b.CategoryUUIDs)+len(b.ProductUUIDs) == 0 {
		return certuc.ErrNotFound
	}
	_, _ = h.q.DeleteCertificateTypeCategories(r.Context(), db.DeleteCertificateTypeCategoriesParams{TypeID: typeID, BrandID: c.Org.BrandID})
	_, _ = h.q.DeleteCertificateTypeProducts(r.Context(), db.DeleteCertificateTypeProductsParams{TypeID: typeID, BrandID: c.Org.BrandID})
	for _, raw := range b.CategoryUUIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return err
		}
		cat, err := h.q.GetProductCategoryByUUID(r.Context(), db.GetProductCategoryByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
		if err != nil {
			return err
		}
		if _, err := h.q.AddCertificateTypeCategory(r.Context(), db.AddCertificateTypeCategoryParams{TypeID: typeID, OrganizationID: orgID, BrandID: c.Org.BrandID, CategoryID: cat.ID}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	for _, raw := range b.ProductUUIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return err
		}
		p, err := h.q.GetProductByUUID(r.Context(), db.GetProductByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
		if err != nil {
			return err
		}
		if _, err := h.q.AddCertificateTypeProduct(r.Context(), db.AddCertificateTypeProductParams{TypeID: typeID, OrganizationID: orgID, BrandID: c.Org.BrandID, ProductID: p.ID}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	return nil
}

func emptyObject(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte(`{}`)
	}
	return raw
}

func int4ptr(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

func text(v string) pgtype.Text {
	if strings.TrimSpace(v) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strings.TrimSpace(v), Valid: true}
}
