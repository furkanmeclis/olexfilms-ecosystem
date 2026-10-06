package usecase

// TEC-375 (DT-BE-6): list contract of GET /v1/stock/barcodes
// (docs/list-contract.md).

import (
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// BarcodeBatchSort is the sort contract of the barcode batch list.
var BarcodeBatchSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"created_at": "created_at", "quantity": "quantity", "print_count": "print_count"},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// BatchListFilter is the parsed query of the barcode batch list.
type BatchListFilter struct {
	ProductUUIDs  []uuid.UUID
	Printed       *bool
	CreatedFrom   *time.Time
	CreatedBefore *time.Time
	// Q matches the prefix, the first or last barcode, the product name or
	// sku, or any barcode of the batch.
	Q             string
	Sort          apiquery.ResolvedSort
	Limit, Offset int32
}

// ParseBatchListFilter reads product_uuid (CSV), printed (true: printed at
// least once), created_from / created_to, q, sort, limit and offset. Errors
// are *apiquery.ValidationError (400).
func ParseBatchListFilter(values url.Values) (BatchListFilter, error) {
	q := apiquery.Parse(values)
	f := BatchListFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	for _, raw := range apiquery.CSVValues(values, "product_uuid") {
		id, err := uuid.Parse(raw)
		if err != nil {
			return f, &apiquery.ValidationError{Details: []apiquery.Detail{{
				Field: "product_uuid", Message: "must be a list of uuids", Code: "invalid",
			}}}
		}
		f.ProductUUIDs = append(f.ProductUUIDs, id)
	}
	var err error
	if f.Printed, err = apiquery.Bool(values, "printed"); err != nil {
		return f, err
	}
	created, err := apiquery.DateRange(values, "created")
	if err != nil {
		return f, err
	}
	f.CreatedFrom, f.CreatedBefore = created.From, created.Before
	f.Sort, err = apiquery.ResolveSort(q.Sort, BarcodeBatchSort)
	return f, err
}

func batchListTS(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// batchListQ is the ILIKE argument of q (LIKE wildcards escaped).
func batchListQ(q string) pgtype.Text {
	q = strings.TrimSpace(q)
	if q == "" {
		return pgtype.Text{}
	}
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return pgtype.Text{String: r.Replace(q), Valid: true}
}
