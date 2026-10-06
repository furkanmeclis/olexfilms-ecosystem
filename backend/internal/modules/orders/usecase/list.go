package usecase

// TEC-373 (DT-BE-5): list contract of GET /v1/orders (docs/list-contract.md).
// The list endpoint and the order list export read the same parameters
// with ParseListFilter, so both select the same rows.

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ListSort is the sort contract of the order list. status sorts by the
// flow rank (draft → cancelled), not alphabetically.
var ListSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"order_no": "order_no", "status": "status", "total": "total", "created_at": "created_at",
	},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// List query keys.
const (
	QuerySide          = "side"
	QueryStatus        = "status"
	QuerySellerOrgUUID = "seller_org_uuid"
	QueryBuyerOrgUUID  = "buyer_org_uuid"
)

// listKeys are the list parameters an export job carries.
var listKeys = []string{
	"q", "sort", QuerySide, QueryStatus, "created_from", "created_to",
	QuerySellerOrgUUID, QueryBuyerOrgUUID, "total_min", "total_max",
}

// ListValues keeps the list parameters of a job query (or export body).
func ListValues(q map[string]string) url.Values {
	out := url.Values{}
	for _, k := range listKeys {
		if v := strings.TrimSpace(q[k]); v != "" {
			out.Set(k, v)
		}
	}
	return out
}

// ParseListFilter reads side, status (CSV), created_from / created_to,
// seller_org_uuid / buyer_org_uuid (CSV), total_min / total_max, q, sort,
// limit and offset. Errors are *apiquery.ValidationError (400).
func ParseListFilter(values url.Values) (ListFilter, error) {
	q := apiquery.Parse(values)
	f := ListFilter{Side: strings.TrimSpace(values.Get(QuerySide)), Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.Statuses, err = apiquery.EnumList(values, QueryStatus, Statuses...); err != nil {
		return f, err
	}
	created, err := apiquery.DateRange(values, "created")
	if err != nil {
		return f, err
	}
	f.CreatedFrom, f.CreatedTo = created.From, created.Before
	if f.SellerOrgUUIDs, err = uuidList(values, QuerySellerOrgUUID); err != nil {
		return f, err
	}
	if f.BuyerOrgUUIDs, err = uuidList(values, QueryBuyerOrgUUID); err != nil {
		return f, err
	}
	total, err := apiquery.NumRange(values, "total")
	if err != nil {
		return f, err
	}
	f.TotalMin, f.TotalMax = total.Min, total.Max
	if f.Sort, err = apiquery.ResolveSort(q.Sort, ListSort); err != nil {
		return f, err
	}
	f.SortExplicit = len(q.Sort) > 0
	return f, nil
}

func uuidList(values url.Values, key string) ([]uuid.UUID, error) {
	var out []uuid.UUID
	for _, raw := range apiquery.CSVValues(values, key) {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, &apiquery.ValidationError{Details: []apiquery.Detail{{
				Field: key, Message: "must be a list of uuids", Code: "invalid",
			}}}
		}
		out = append(out, id)
	}
	return out, nil
}

// sqlOnly reports whether the filter uses a parameter the orders index
// cannot answer (TEC-210 keeps q on the index otherwise).
func (f ListFilter) sqlOnly() bool {
	return f.SortExplicit || f.CreatedFrom != nil || f.CreatedTo != nil ||
		len(f.SellerOrgUUIDs) > 0 || len(f.BuyerOrgUUIDs) > 0 || f.TotalMin != nil || f.TotalMax != nil
}

func sortArgs(f ListFilter) (string, bool) {
	if f.Sort.Key == "" {
		return ListSort.Columns[ListSort.Default.Field], ListSort.Default.Desc
	}
	return f.Sort.Key, f.Sort.Desc
}

func tsArg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func numArg(v *float64) pgtype.Numeric {
	var n pgtype.Numeric
	if v == nil {
		return n
	}
	if err := n.Scan(strconv.FormatFloat(*v, 'f', -1, 64)); err != nil {
		return pgtype.Numeric{}
	}
	return n
}
