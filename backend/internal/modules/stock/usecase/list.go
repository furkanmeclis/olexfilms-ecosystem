package usecase

// TEC-373 (DT-BE-5): list contract of the organization unit and product
// stock lists (docs/list-contract.md). The unit list endpoint and its
// export read the same parameters with ParseUnitFilter.

import (
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// UnitSort is the sort contract of GET /v1/stock/organizations/{uuid}/units.
// status sorts by the unit flow rank (reserved → void); meters is the
// remaining meters (pieces last); product keeps barcode order inside a
// product (the former fixed order, the default).
var UnitSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"product": "product", "barcode": "barcode", "status": "status",
		"quantity": "quantity", "meters": "meters", "updated_at": "updated_at",
	},
	Default: apiquery.SortField{Field: "product"},
}

// ProductStockSort is the sort contract of the product stock lists
// (organization and location).
var ProductStockSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"product": "product", "sku": "sku", "category": "category",
		"quantity": "quantity", "meters": "meters", "updated_at": "updated_at",
	},
	Default: apiquery.SortField{Field: "product"},
}

// Unit list query keys.
const (
	QueryBarcode      = "barcode"
	QueryBarcodeMatch = "barcode_match"
	// BarcodeMatchPrefix makes barcode a prefix match (default: exact).
	BarcodeMatchPrefix = "prefix"
	BarcodeMatchExact  = "exact"
)

// unitListKeys are the unit list parameters an export job carries.
var unitListKeys = []string{
	"q", "sort", "product_uuid", "status", QueryBarcode, QueryBarcodeMatch,
	"location_uuid", "updated_from", "updated_to",
}

// UnitListValues keeps the unit list parameters of a job query (or body).
func UnitListValues(q map[string]string) url.Values {
	out := url.Values{}
	for _, k := range unitListKeys {
		if v := strings.TrimSpace(q[k]); v != "" {
			out.Set(k, v)
		}
	}
	return out
}

func invalidParam(field, msg string) error {
	return &apiquery.ValidationError{Details: []apiquery.Detail{{Field: field, Message: msg, Code: "invalid"}}}
}

// ParseUnitFilter reads q, product_uuid, status (CSV), barcode,
// barcode_match (exact | prefix), location_uuid (CSV), updated_from /
// updated_to, sort, limit and offset. Errors are *apiquery.ValidationError.
func ParseUnitFilter(values url.Values) (model.UnitFilter, error) {
	q := apiquery.Parse(values)
	in := model.UnitFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset, Barcode: strings.TrimSpace(values.Get(QueryBarcode))}
	if len(in.Barcode) > 64 {
		return in, invalidParam(QueryBarcode, "barcode is too long")
	}
	switch strings.TrimSpace(values.Get(QueryBarcodeMatch)) {
	case "", BarcodeMatchExact:
	case BarcodeMatchPrefix:
		in.BarcodePrefix = true
	default:
		return in, invalidParam(QueryBarcodeMatch, "must be exact or prefix")
	}
	var err error
	if in.Statuses, err = apiquery.EnumList(values, "status", model.UnitStatuses...); err != nil {
		return in, err
	}
	if raw := strings.TrimSpace(values.Get("product_uuid")); raw != "" {
		pid, perr := uuid.Parse(raw)
		if perr != nil {
			return in, invalidParam("product_uuid", "product_uuid is invalid")
		}
		in.ProductUUID = &pid
	}
	for _, raw := range apiquery.CSVValues(values, "location_uuid") {
		id, perr := uuid.Parse(raw)
		if perr != nil {
			return in, invalidParam("location_uuid", "must be a list of uuids")
		}
		in.LocationUUIDs = append(in.LocationUUIDs, id)
	}
	updated, err := apiquery.DateRange(values, "updated")
	if err != nil {
		return in, err
	}
	in.UpdatedFrom, in.UpdatedBefore = updated.From, updated.Before
	if in.Sort, err = apiquery.ResolveSort(q.Sort, UnitSort); err != nil {
		return in, err
	}
	in.SortExplicit = len(q.Sort) > 0
	return in, nil
}

// unitSQLOnly reports whether the unit filter uses a parameter the stock
// units index cannot answer (q then searches SQL, TEC-373).
func unitSQLOnly(in model.UnitFilter) bool {
	return in.Barcode != "" || in.SortExplicit || len(in.LocationUUIDs) > 0 ||
		in.UpdatedFrom != nil || in.UpdatedBefore != nil
}

func resolvedSort(r apiquery.ResolvedSort, spec apiquery.SortSpec) (string, bool) {
	if r.Key == "" {
		return spec.Columns[spec.Default.Field], spec.Default.Desc
	}
	return r.Key, r.Desc
}

// likePrefix escapes the LIKE wildcards of a barcode prefix.
func likePrefix(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func tsArg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
