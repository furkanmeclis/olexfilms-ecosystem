package handler

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/glorianadmin/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// TEC-1265 (karar 6): Glorian dealer <-> organization match.

// Conflict codes of the dealer match endpoints.
const (
	CodeDealerCodeTaken    = "GLORIAN_DEALER_CODE_TAKEN"
	CodeOrganizationLinked = "GLORIAN_ORGANIZATION_LINKED"
)

const (
	activityDealerLinked   = "integrations.glorian.dealer_linked"
	activityDealerUnlinked = "integrations.glorian.dealer_unlinked"
	activityDealerCodeSet  = "integrations.glorian.dealer_code_set"
)

// ListDealers (GET .../dealers?q=&match_type=&active=&sort=&limit=&offset=).
func (h *Handler) ListDealers(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianView)
	if !ok {
		return
	}
	limit, offset, ok := pageParams(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	srt, err := apiquery.ResolveSort(apiquery.Parse(q).Sort, usecase.DealersSortSpec)
	if err != nil {
		writeError(w, r, err)
		return
	}
	types, err := apiquery.EnumList(q, "match_type", usecase.MatchTypes...)
	if err != nil {
		writeError(w, r, err)
		return
	}
	active, err := apiquery.Bool(q, "active")
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.svc.ListDealers(r.Context(), b, usecase.DealerFilter{
		Q: q.Get("q"), MatchTypes: types, Active: active,
		SortKey: srt.Key, SortDesc: srt.Desc, Limit: limit, Offset: offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, int32(limit), int32(offset)))
}

// LinkDealer (PUT .../dealers/{uuid}/link): manual link; a null
// organization_uuid clears it.
func (h *Handler) LinkDealer(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianManage)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "uuid")
	if !ok {
		return
	}
	var in usecase.DealerLinkInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.LinkDealer(r.Context(), b, id, in)
	if err != nil {
		writeDealerError(w, r, err)
		return
	}
	if in.OrganizationUUID != nil {
		h.record(r, activityDealerLinked, &v.UUID, map[string]any{
			"remote_id": v.RemoteID, "organization_uuid": in.OrganizationUUID.String(),
		})
	} else {
		h.record(r, activityDealerUnlinked, &v.UUID, map[string]any{"remote_id": v.RemoteID})
	}
	response.JSON(w, r, http.StatusOK, v)
}

// DealerOrganizations (GET .../dealers/organizations?q=): picker options.
func (h *Handler) DealerOrganizations(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianView)
	if !ok {
		return
	}
	items, err := h.svc.DealerOrganizations(r.Context(), b, r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// SetDealerCode (PUT .../organizations/{uuid}/dealer-code).
func (h *Handler) SetDealerCode(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianManage)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "uuid")
	if !ok {
		return
	}
	var in usecase.DealerCodeInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.SetDealerCode(r.Context(), b, id, in)
	if err != nil {
		writeDealerError(w, r, err)
		return
	}
	h.record(r, activityDealerCodeSet, &v.UUID, map[string]any{"glorian_dealer_code": v.GlorianDealerCode})
	response.JSON(w, r, http.StatusOK, v)
}

func writeDealerError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrDealerCodeTaken):
		response.Conflict(w, r, CodeDealerCodeTaken, "Another organization of the brand has this Glorian dealer code")
	case errors.Is(err, usecase.ErrOrganizationLinked):
		response.Conflict(w, r, CodeOrganizationLinked, "The organization is already linked to another Glorian dealer")
	default:
		writeError(w, r, err)
	}
}
