// Package handler serves the price list endpoints of TEC-146 (K8).
package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	pricing "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves pricing endpoints.
type Handler struct {
	svc      *pricing.Service
	activity *activity.Recorder
}

// New creates the handler. rec may be nil.
func New(svc *pricing.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func viewer(r *http.Request) pricing.Viewer {
	return pricing.ViewerFrom(authctx.MustPrincipal(r.Context()), orgctx.MustScope(r.Context()))
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *pricing.ValidationError
	switch {
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.Is(err, pricing.ErrForbidden):
		response.Forbidden(w, r, "This price is not writable or readable by your organization")
	case errors.Is(err, pricing.ErrProductNotFound):
		response.NotFound(w, r, "Product not found")
	case errors.Is(err, pricing.ErrDistributorNotFound):
		response.NotFound(w, r, "Distributor not found")
	case errors.Is(err, pricing.ErrPriceNotFound):
		response.NotFound(w, r, "Price not found")
	default:
		response.InternalErr(w, r, err, "pricing request failed")
	}
}

func (h *Handler) record(r *http.Request, action string, product uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	var actor *int64
	if p, ok := authctx.PrincipalFrom(r.Context()); ok && p.UserInternal != 0 {
		id := p.UserInternal
		actor = &id
	}
	if org, ok := orgctx.ScopeFrom(r.Context()); ok {
		payload["organization_uuid"] = org.UUID.String()
	}
	h.activity.Record(r.Context(), actor, action, "products", &product, payload, r)
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.NotFound(w, r, "Resource not found")
		return uuid.Nil, false
	}
	return id, true
}

func queryUUID(w http.ResponseWriter, r *http.Request, name string) (*uuid.UUID, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: "must be a UUID"}})
		return nil, false
	}
	return &id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

// ListProducts returns the effective price view of the brand's products
// (GET /v1/tenant/pricing/products?q&active&limit&offset).
func (h *Handler) ListProducts(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	f := pricing.ListFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	switch strings.TrimSpace(r.URL.Query().Get("active")) {
	case "":
	case "true":
		t := true
		f.Active = &t
	case "false":
		b := false
		f.Active = &b
	default:
		response.ValidationError(w, r, []response.Detail{{Field: "active", Message: "must be true or false"}})
		return
	}
	items, total, err := h.svc.ListViews(r.Context(), viewer(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetProduct returns the effective price view of one product
// (GET /v1/tenant/pricing/products/{uuid}).
func (h *Handler) GetProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	view, err := h.svc.ProductView(r.Context(), viewer(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, view)
}

// optionalPrice decodes a field that may be absent, null or a decimal string.
type optionalPrice struct {
	pricing.OptionalPrice
}

func (o *optionalPrice) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	o.Value = &s
	return nil
}

type listPriceBody struct {
	PurchasePrice          optionalPrice `json:"purchase_price"`
	SaleToDistributorPrice optionalPrice `json:"sale_to_distributor_price"`
	RecommendedSalePrice   optionalPrice `json:"recommended_sale_price"`
}

// SetListPrice writes the center list price of one currency
// (PUT /v1/tenant/pricing/products/{uuid}/prices/{currency}).
func (h *Handler) SetListPrice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in listPriceBody
	if !decode(w, r, &in) {
		return
	}
	cur := r.PathValue("currency")
	view, err := h.svc.SetListPrice(r.Context(), viewer(r), id, cur, pricing.ListPriceInput{
		Purchase:          in.PurchasePrice.OptionalPrice,
		SaleToDistributor: in.SaleToDistributorPrice.OptionalPrice,
		Recommended:       in.RecommendedSalePrice.OptionalPrice,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "pricing.list_price_set", id, map[string]any{"currency": strings.ToUpper(cur)})
	response.JSON(w, r, http.StatusOK, view)
}

// DeleteListPrice removes the center list price of one currency
// (DELETE /v1/tenant/pricing/products/{uuid}/prices/{currency}).
func (h *Handler) DeleteListPrice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	cur := r.PathValue("currency")
	if err := h.svc.DeleteListPrice(r.Context(), viewer(r), id, cur); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "pricing.list_price_deleted", id, map[string]any{"currency": strings.ToUpper(cur)})
	response.JSON(w, r, http.StatusOK, map[string]bool{"deleted": true})
}

type priceBody struct {
	Price string `json:"price"`
}

// SetDistributorOverride writes a distributor-specific price
// (PUT /v1/tenant/pricing/products/{uuid}/distributor-prices/{distributor_uuid}/{currency}).
func (h *Handler) SetDistributorOverride(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	dist, ok := pathUUID(w, r, "distributor_uuid")
	if !ok {
		return
	}
	var in priceBody
	if !decode(w, r, &in) {
		return
	}
	row, err := h.svc.SetDistributorOverride(r.Context(), viewer(r), id, dist, r.PathValue("currency"), in.Price)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "pricing.distributor_price_set", id, map[string]any{
		"currency": row.Currency, "distributor_uuid": dist.String(),
	})
	response.JSON(w, r, http.StatusOK, row)
}

// DeleteDistributorOverride removes a distributor-specific price
// (DELETE /v1/tenant/pricing/products/{uuid}/distributor-prices/{distributor_uuid}/{currency}).
func (h *Handler) DeleteDistributorOverride(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	dist, ok := pathUUID(w, r, "distributor_uuid")
	if !ok {
		return
	}
	cur := r.PathValue("currency")
	if err := h.svc.DeleteDistributorOverride(r.Context(), viewer(r), id, dist, cur); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "pricing.distributor_price_deleted", id, map[string]any{
		"currency": strings.ToUpper(cur), "distributor_uuid": dist.String(),
	})
	response.JSON(w, r, http.StatusOK, map[string]bool{"deleted": true})
}

// ListDistributorOverrides lists distributor-specific prices of the brand
// (GET /v1/tenant/pricing/distributor-prices?product_uuid&distributor_uuid).
func (h *Handler) ListDistributorOverrides(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	product, ok := queryUUID(w, r, "product_uuid")
	if !ok {
		return
	}
	dist, ok := queryUUID(w, r, "distributor_uuid")
	if !ok {
		return
	}
	f := pricing.OverrideFilter{
		ProductUUID: product, DistributorUUID: dist, Q: q.Q, Limit: q.Limit, Offset: q.Offset,
	}
	// TEC-369: currency (CSV of ISO-4217 codes), q and sort.
	for _, raw := range apiquery.CSVValues(r.URL.Query(), "currency") {
		cur, err := pricing.NormalizeCurrency(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "currency", Message: "must be a list of three-letter ISO-4217 codes", Code: "invalid"}})
			return
		}
		f.Currencies = append(f.Currencies, cur)
	}
	sort, err := apiquery.ResolveSort(q.Sort, pricing.DistributorPriceSort)
	if response.QueryValidation(w, r, err) {
		return
	}
	f.Sort = sort
	items, total, err := h.svc.ListDistributorOverrides(r.Context(), viewer(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// SetDealerPrice writes the caller distributor's price to its dealers
// (PUT /v1/tenant/pricing/products/{uuid}/dealer-prices/{currency}).
func (h *Handler) SetDealerPrice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in priceBody
	if !decode(w, r, &in) {
		return
	}
	cur := r.PathValue("currency")
	view, err := h.svc.SetDealerPrice(r.Context(), viewer(r), id, cur, in.Price)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "pricing.dealer_price_set", id, map[string]any{"currency": strings.ToUpper(cur)})
	response.JSON(w, r, http.StatusOK, view)
}

// DeleteDealerPrice removes the caller distributor's dealer price
// (DELETE /v1/tenant/pricing/products/{uuid}/dealer-prices/{currency}).
func (h *Handler) DeleteDealerPrice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	cur := r.PathValue("currency")
	if err := h.svc.DeleteDealerPrice(r.Context(), viewer(r), id, cur); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "pricing.dealer_price_deleted", id, map[string]any{"currency": strings.ToUpper(cur)})
	response.JSON(w, r, http.StatusOK, map[string]bool{"deleted": true})
}
