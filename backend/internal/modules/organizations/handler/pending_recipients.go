package handler

import (
	"net/http"

	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// TenantCreatePendingRecipient opens a recipient (dealer candidate or
// retail point) under the center and puts it in the waiting pool:
// POST /v1/tenant/pending-recipients (TEC-1062).
func (h *Handler) TenantCreatePendingRecipient(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	var in struct {
		Name       string `json:"name"`
		Phone      string `json:"phone"`
		City       string `json:"city"`
		District   string `json:"district"`
		Address    string `json:"address"`
		CountryID  *int64 `json:"country_id"`
		ProvinceID *int64 `json:"province_id"`
		DistrictID *int64 `json:"district_id"`
		TaxNo      string `json:"tax_no"`
		TaxOffice  string `json:"tax_office"`
		Kind       string `json:"kind"`
		Note       string `json:"note"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	out, err := h.svc.CreatePendingRecipient(r.Context(), scope.InternalID, actor, orgusecase.RecipientInput{
		Name: in.Name, Phone: in.Phone, City: in.City, District: in.District, Address: in.Address,
		Location: orgusecase.AddressInput{CountryID: in.CountryID, ProvinceID: in.ProvinceID, DistrictID: in.DistrictID},
		TaxNo:    in.TaxNo, TaxOffice: in.TaxOffice, Kind: in.Kind, Note: in.Note,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// TenantListPendingRecipients lists the center's waiting pool:
// GET /v1/tenant/pending-recipients (list contract; state, kind CSV,
// created_from/_to).
func (h *Handler) TenantListPendingRecipients(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	values := r.URL.Query()
	q := apiquery.Parse(values)
	sort, err := apiquery.ResolveSort(q.Sort, orgusecase.PendingRecipientSort)
	if err != nil {
		writeError(w, r, err)
		return
	}
	states, err := apiquery.EnumList(values, "state", orgusecase.RecipientStates...)
	if err != nil {
		writeError(w, r, err)
		return
	}
	kinds, err := apiquery.EnumList(values, "kind", orgusecase.RecipientKinds...)
	if err != nil {
		writeError(w, r, err)
		return
	}
	created, err := apiquery.DateRange(values, "created")
	if err != nil {
		writeError(w, r, err)
		return
	}
	if len(q.Q) > 200 {
		response.BadRequest(w, r, response.CodeValidationError, "q is too long")
		return
	}
	items, total, err := h.svc.ListPendingRecipients(r.Context(), scope.InternalID, orgusecase.RecipientListInput{
		Q: q.Q, States: states, Kinds: kinds, Created: created, Sort: sort, Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// TenantConvertPendingRecipient turns a recipient into an active dealer,
// the owner attached when given: POST /v1/tenant/pending-recipients/{uuid}/convert.
func (h *Handler) TenantConvertPendingRecipient(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	id, ok := recipientUUID(w, r)
	if !ok {
		return
	}
	var in struct {
		Owner *accountBody `json:"owner"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	var owner *orgusecase.AccountInput
	if in.Owner != nil {
		a := in.Owner.input()
		owner = &a
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	out, err := h.svc.ConvertPendingRecipient(r.Context(), scope.InternalID, actor, id, orgusecase.ConvertInput{Owner: owner})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// TenantKeepRecipientAsRetail takes a recipient out of the pool as a
// retail point: POST /v1/tenant/pending-recipients/{uuid}/keep-retail.
func (h *Handler) TenantKeepRecipientAsRetail(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	id, ok := recipientUUID(w, r)
	if !ok {
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	out, err := h.svc.KeepRecipientAsRetail(r.Context(), scope.InternalID, actor, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func recipientUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "recipient uuid is invalid")
		return uuid.UUID{}, false
	}
	return id, true
}
