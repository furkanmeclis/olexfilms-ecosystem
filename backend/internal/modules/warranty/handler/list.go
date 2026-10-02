package handler

// TEC-191 (F1-06g): GET /v1/warranties, GET /v1/warranties/{uuid},
// POST /v1/warranties/{uuid}/void and the portal reads
// GET /v1/portal/warranties, GET /v1/portal/warranties/{uuid}.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// CodeAlreadyVoid answers a void of a warranty that is already void.
const CodeAlreadyVoid = "WARRANTY_ALREADY_VOID"

// List serves the panel and portal warranty reads and the center void.
type List struct {
	r *usecase.Reader
}

// NewList creates the handler.
func NewList(r *usecase.Reader) *List { return &List{r: r} }

func panelCaller(r *http.Request) usecase.Caller {
	f, _ := scopefilter.From(r.Context())
	return usecase.Caller{Principal: authctx.MustPrincipal(r.Context()), Org: orgctx.MustScope(r.Context()), Filter: f}
}

func writeListError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	switch {
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.Is(err, usecase.ErrWarrantyNotFound):
		response.NotFound(w, r, "Warranty not found")
	case errors.Is(err, usecase.ErrAlreadyVoid):
		response.Conflict(w, r, CodeAlreadyVoid, "The warranty is already void")
	default:
		response.InternalErr(w, r, err, "warranty request failed")
	}
}

func warrantyUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Warranty not found")
		return uuid.Nil, false
	}
	return id, true
}

func optInt(raw, field string) (*int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil, &usecase.ValidationError{Field: field, Message: "must be an integer"}
	}
	return &n, nil
}

// listFilter parses ?status&q&product_uuid&vehicle_uuid&days_left_min&days_left_max&limit&offset.
func listFilter(r *http.Request) (usecase.ListFilter, apiquery.Query, error) {
	pq := apiquery.Parse(r.URL.Query())
	v := r.URL.Query()
	minDays, err := optInt(v.Get("days_left_min"), "days_left_min")
	if err != nil {
		return usecase.ListFilter{}, pq, err
	}
	maxDays, err := optInt(v.Get("days_left_max"), "days_left_max")
	if err != nil {
		return usecase.ListFilter{}, pq, err
	}
	return usecase.ListFilter{
		Status: v.Get("status"), Q: v.Get("q"), ProductUUID: v.Get("product_uuid"),
		VehicleUUID: v.Get("vehicle_uuid"), DaysLeftMin: minDays, DaysLeftMax: maxDays,
		Limit: pq.Limit, Offset: pq.Offset,
	}, pq, nil
}

// PanelList (GET /v1/warranties).
func (h *List) PanelList(w http.ResponseWriter, r *http.Request) {
	f, pq, err := listFilter(r)
	if err != nil {
		writeListError(w, r, err)
		return
	}
	items, total, err := h.r.List(r.Context(), panelCaller(r), f)
	if err != nil {
		writeListError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, pq.Limit, pq.Offset))
}

// PanelGet (GET /v1/warranties/{uuid}).
func (h *List) PanelGet(w http.ResponseWriter, r *http.Request) {
	id, ok := warrantyUUID(w, r)
	if !ok {
		return
	}
	v, err := h.r.Get(r.Context(), panelCaller(r), id)
	if err != nil {
		writeListError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

type voidBody struct {
	Reason string `json:"reason"`
}

// Void (POST /v1/warranties/{uuid}/void).
func (h *List) Void(w http.ResponseWriter, r *http.Request) {
	id, ok := warrantyUUID(w, r)
	if !ok {
		return
	}
	var b voidBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	v, err := h.r.Void(r.Context(), panelCaller(r), id, b.Reason, activity.MetaFromRequest(r))
	if err != nil {
		writeListError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

func portalBrand(r *http.Request) int64 {
	if b, ok := brandctx.From(r.Context()); ok {
		return b.ID
	}
	return 0
}

// PortalList (GET /v1/portal/warranties): the signed-in user's warranties.
func (h *List) PortalList(w http.ResponseWriter, r *http.Request) {
	f, pq, err := listFilter(r)
	if err != nil {
		writeListError(w, r, err)
		return
	}
	p := authctx.MustPrincipal(r.Context())
	items, total, err := h.r.PortalList(r.Context(), portalBrand(r), p.UserInternal, f)
	if err != nil {
		writeListError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, pq.Limit, pq.Offset))
}

// PortalGet (GET /v1/portal/warranties/{uuid}).
func (h *List) PortalGet(w http.ResponseWriter, r *http.Request) {
	id, ok := warrantyUUID(w, r)
	if !ok {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	v, err := h.r.PortalGet(r.Context(), portalBrand(r), p.UserInternal, id)
	if err != nil {
		writeListError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}
