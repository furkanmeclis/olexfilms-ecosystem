// Package handler serves the F3-07d dealer accounting endpoints.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	da "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealeraccounting/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

const (
	CodeStockUnavailable  = "STOCK_UNAVAILABLE"
	CodeVoidWindowExpired = "VOID_WINDOW_EXPIRED"
	CodeSaleVoided        = "PRODUCT_SALE_VOIDED"
)

type Handler struct {
	svc *da.Service
}

func New(svc *da.Service) *Handler { return &Handler{svc: svc} }

func caller(r *http.Request) da.Caller {
	p := authctx.MustPrincipal(r.Context())
	f, _ := scopefilter.From(r.Context())
	// TEC-506: the recommended block needs pricing.recommended.read.
	return da.Caller{
		UserID: p.UserInternal, Org: orgctx.MustScope(r.Context()), Filter: f,
		RecommendedRead: p.Can(rbac.PermPricingRecommendedRead, rbac.ScopeManaged),
	}
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

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.NotFound(w, r, "Resource not found")
		return uuid.Nil, false
	}
	return id, true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *da.ValidationError
	switch {
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.Is(err, da.ErrForbidden):
		response.Forbidden(w, r, "This dealer accounting endpoint is not available")
	case errors.Is(err, da.ErrProductNotFound):
		response.NotFound(w, r, "Product not found")
	case errors.Is(err, da.ErrCustomerNotFound):
		response.NotFound(w, r, "Customer not found")
	case errors.Is(err, da.ErrSupplierNotFound):
		response.NotFound(w, r, "Supplier not found")
	case errors.Is(err, da.ErrSaleNotFound):
		response.NotFound(w, r, "Product sale not found")
	case errors.Is(err, da.ErrPurchaseNotFound):
		response.NotFound(w, r, "Purchase not found")
	case errors.Is(err, da.ErrStockUnavailable):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeStockUnavailable, "Requested stock is not available")
	case errors.Is(err, da.ErrSaleAlreadyVoided):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeSaleVoided, "Product sale is already voided")
	case errors.Is(err, da.ErrVoidWindowExpired):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeVoidWindowExpired, "Product sale can be voided only on the sale day")
	default:
		response.InternalErr(w, r, err, "dealer accounting request failed")
	}
}

func (h *Handler) ListDealerPrices(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListDealerPrices(r.Context(), caller(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type priceBody struct {
	ProductUUID string `json:"product_uuid"`
	SalePrice   string `json:"sale_price"`
}

func (h *Handler) SetDealerPrice(w http.ResponseWriter, r *http.Request) {
	var b priceBody
	if !decode(w, r, &b) {
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(b.ProductUUID))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "product_uuid", Message: "must be a UUID"}})
		return
	}
	v, err := h.svc.SetDealerPrice(r.Context(), caller(r), da.DealerPriceInput{ProductUUID: id, SalePrice: b.SalePrice})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

type supplierBody struct {
	Name      string  `json:"name"`
	TaxNo     *string `json:"tax_no"`
	PhoneE164 *string `json:"phone_e164"`
	Email     *string `json:"email"`
	Note      string  `json:"note"`
	Active    *bool   `json:"active"`
}

func (b supplierBody) input() da.SupplierInput {
	return da.SupplierInput{Name: b.Name, TaxNo: b.TaxNo, PhoneE164: b.PhoneE164, Email: b.Email, Note: b.Note, Active: b.Active}
}

func (h *Handler) CreateSupplier(w http.ResponseWriter, r *http.Request) {
	var b supplierBody
	if !decode(w, r, &b) {
		return
	}
	v, err := h.svc.CreateSupplier(r.Context(), caller(r), b.input())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

func (h *Handler) UpdateSupplier(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b supplierBody
	if !decode(w, r, &b) {
		return
	}
	v, err := h.svc.UpdateSupplier(r.Context(), caller(r), id, b.input())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

type saleLineBody struct {
	ProductUUID *string `json:"product_uuid"`
	Barcode     string  `json:"barcode"`
	Quantity    int32   `json:"quantity"`
	UnitPrice   string  `json:"unit_price"`
}

type saleBody struct {
	CustomerUUID  *string        `json:"customer_uuid"`
	PaymentMethod string         `json:"payment_method"`
	Note          string         `json:"note"`
	Lines         []saleLineBody `json:"lines"`
}

func (b saleBody) input() (da.ProductSaleInput, []response.Detail) {
	out := da.ProductSaleInput{PaymentMethod: b.PaymentMethod, Note: b.Note, Lines: make([]da.SaleLineInput, 0, len(b.Lines))}
	if b.CustomerUUID != nil && strings.TrimSpace(*b.CustomerUUID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*b.CustomerUUID))
		if err != nil {
			return out, []response.Detail{{Field: "customer_uuid", Message: "must be a UUID"}}
		}
		out.CustomerUUID = &id
	}
	for i, l := range b.Lines {
		line := da.SaleLineInput{Barcode: l.Barcode, Quantity: l.Quantity, UnitPrice: l.UnitPrice}
		if l.ProductUUID != nil && strings.TrimSpace(*l.ProductUUID) != "" {
			id, err := uuid.Parse(strings.TrimSpace(*l.ProductUUID))
			if err != nil {
				return out, []response.Detail{{Field: "lines." + strconv.Itoa(i) + ".product_uuid", Message: "must be a UUID"}}
			}
			line.ProductUUID = &id
		}
		out.Lines = append(out.Lines, line)
	}
	return out, nil
}

func (h *Handler) CreateProductSale(w http.ResponseWriter, r *http.Request) {
	var b saleBody
	if !decode(w, r, &b) {
		return
	}
	in, details := b.input()
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	v, err := h.svc.CreateProductSale(r.Context(), caller(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

func (h *Handler) VoidProductSale(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &b) {
		return
	}
	v, err := h.svc.VoidProductSale(r.Context(), caller(r), id, b.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

type purchaseBody struct {
	SupplierUUID  string `json:"supplier_uuid"`
	Amount        string `json:"amount"`
	PaymentMethod string `json:"payment_method"`
	Note          string `json:"note"`
	Description   string `json:"description"`
}

func (h *Handler) CreatePurchase(w http.ResponseWriter, r *http.Request) {
	var b purchaseBody
	if !decode(w, r, &b) {
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(b.SupplierUUID))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "supplier_uuid", Message: "must be a UUID"}})
		return
	}
	v, err := h.svc.CreatePurchase(r.Context(), caller(r), da.PurchaseInput{
		SupplierUUID: id, Amount: b.Amount, PaymentMethod: b.PaymentMethod, Note: b.Note, Description: b.Description,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}
