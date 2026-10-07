package handler

// TEC-348 (F3-07h): list endpoints of the dealer sales screens
// (docs/list-contract.md) and the quick-sale barcode lookup.

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	da "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealeraccounting/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

var (
	PriceCatalogSortSpec = apiquery.SortSpec{
		Columns: apiquery.SortColumns{
			"name": "name", "sku": "sku", "sale_price": "sale_price",
			"recommended_sale_price": "recommended_sale_price", "updated_at": "updated_at",
		},
		Default: apiquery.SortField{Field: "name"},
	}
	SalesSortSpec = apiquery.SortSpec{
		Columns: apiquery.SortColumns{"sold_at": "sold_at", "total": "total", "payment_method": "payment_method"},
		Default: apiquery.SortField{Field: "sold_at", Desc: true},
	}
	SuppliersSortSpec = apiquery.SortSpec{
		Columns: apiquery.SortColumns{"name": "name", "created_at": "created_at", "updated_at": "updated_at"},
		Default: apiquery.SortField{Field: "name"},
	}
	PurchasesSortSpec = apiquery.SortSpec{
		Columns: apiquery.SortColumns{
			"purchased_on": "purchased_on", "amount": "amount",
			"supplier_name": "supplier_name", "payment_method": "payment_method",
		},
		Default: apiquery.SortField{Field: "purchased_on", Desc: true},
	}
)

// purchaseVisible: pricing.purchase.read in the active (dealer)
// organization; same rule as the accounting reports.
func purchaseVisible(r *http.Request) bool {
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok {
		return false
	}
	need := rbac.ScopeManaged
	if orgctx.MustScope(r.Context()).OrgType == rbac.OrgTypeCenter {
		need = rbac.ScopeBrand
	}
	return p.Can(rbac.PermPricingPurchaseRead, need)
}

func writeQueryError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *apiquery.ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	writeError(w, r, err)
}

func listPage(values url.Values, spec apiquery.SortSpec) (da.ListPage, error) {
	q := apiquery.Parse(values)
	sort, err := apiquery.ResolveSort(q.Sort, spec)
	if err != nil {
		return da.ListPage{}, err
	}
	return da.ListPage{Limit: q.Limit, Offset: q.Offset, Q: strings.TrimSpace(q.Q), Sort: sort}, nil
}

// ListPriceCatalog (GET /v1/dealer-prices/catalog).
func (h *Handler) ListPriceCatalog(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	page, err := listPage(values, PriceCatalogSortSpec)
	if err != nil {
		writeQueryError(w, r, err)
		return
	}
	priced, err := apiquery.Bool(values, "priced")
	if err != nil {
		writeQueryError(w, r, err)
		return
	}
	out, err := h.svc.ListPriceCatalog(r.Context(), caller(r), da.PriceCatalogFilter{
		ListPage: page, Priced: priced, PurchaseVisible: purchaseVisible(r),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// LookupSaleItem (GET /v1/product-sales/lookup?barcode=|product_uuid=).
func (h *Handler) LookupSaleItem(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	var productUUID *uuid.UUID
	if raw := strings.TrimSpace(values.Get("product_uuid")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "product_uuid", Message: "must be a UUID"}})
			return
		}
		productUUID = &id
	}
	out, err := h.svc.LookupSaleItem(r.Context(), caller(r), values.Get("barcode"), productUUID, purchaseVisible(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

var salePaymentMethods = []string{da.PaymentCash, da.PaymentCard, da.PaymentCari}
var purchasePaymentMethods = []string{da.PaymentCash, da.PaymentCard, da.PaymentBankTransfer, da.PaymentCari}

// ListProductSales (GET /v1/product-sales).
func (h *Handler) ListProductSales(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	page, err := listPage(values, SalesSortSpec)
	if err != nil {
		writeQueryError(w, r, err)
		return
	}
	f := da.SaleListFilter{ListPage: page, PurchaseVisible: purchaseVisible(r)}
	if f.PaymentMethods, err = apiquery.EnumList(values, "payment_method", salePaymentMethods...); err != nil {
		writeQueryError(w, r, err)
		return
	}
	if f.Voided, err = apiquery.Bool(values, "voided"); err != nil {
		writeQueryError(w, r, err)
		return
	}
	if f.Sold, err = apiquery.DateRange(values, "sold"); err != nil {
		writeQueryError(w, r, err)
		return
	}
	total, err := apiquery.NumRange(values, "total")
	if err != nil {
		writeQueryError(w, r, err)
		return
	}
	f.TotalMin, f.TotalMax = total.Min, total.Max
	out, err := h.svc.ListProductSales(r.Context(), caller(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListSuppliers (GET /v1/suppliers).
func (h *Handler) ListSuppliers(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	page, err := listPage(values, SuppliersSortSpec)
	if err != nil {
		writeQueryError(w, r, err)
		return
	}
	f := da.SupplierListFilter{ListPage: page}
	if f.Active, err = apiquery.Bool(values, "active"); err != nil {
		writeQueryError(w, r, err)
		return
	}
	if f.Created, err = apiquery.DateRange(values, "created"); err != nil {
		writeQueryError(w, r, err)
		return
	}
	out, err := h.svc.ListSuppliersPage(r.Context(), caller(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListPurchases (GET /v1/purchases).
func (h *Handler) ListPurchases(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	page, err := listPage(values, PurchasesSortSpec)
	if err != nil {
		writeQueryError(w, r, err)
		return
	}
	f := da.PurchaseListFilter{ListPage: page}
	for _, raw := range apiquery.CSVValues(values, "supplier_uuid") {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "supplier_uuid", Message: "must be a UUID list"}})
			return
		}
		f.SupplierUUIDs = append(f.SupplierUUIDs, id)
	}
	if f.PaymentMethods, err = apiquery.EnumList(values, "payment_method", purchasePaymentMethods...); err != nil {
		writeQueryError(w, r, err)
		return
	}
	if f.Purchased, err = apiquery.DateRange(values, "purchased"); err != nil {
		writeQueryError(w, r, err)
		return
	}
	amount, err := apiquery.NumRange(values, "amount")
	if err != nil {
		writeQueryError(w, r, err)
		return
	}
	f.AmountMin, f.AmountMax = amount.Min, amount.Max
	out, err := h.svc.ListPurchases(r.Context(), caller(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
