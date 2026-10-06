package usecase

// TEC-373 (DT-BE-5): list contract of GET /v1/stock-transfers
// (docs/list-contract.md).

import (
	"net/url"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ListSort is the sort contract of the request list. status sorts by the
// flow rank (requested, approved, shipped, received, rejected, cancelled).
var ListSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"transfer_no": "transfer_no", "status": "status", "created_at": "created_at"},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// ParseListFilter reads kind, direction, status (all CSV), created_from /
// created_to, organization_uuid (CSV: sender or receiver), q, sort, limit
// and offset. Errors are *apiquery.ValidationError (400).
func ParseListFilter(values url.Values) (ListFilter, error) {
	q := apiquery.Parse(values)
	f := ListFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.Kinds, err = apiquery.EnumList(values, "kind", KindSibling, KindReturn); err != nil {
		return f, err
	}
	if f.Directions, err = apiquery.EnumList(values, "direction",
		DirectionOutgoing, DirectionIncoming, DirectionApproval); err != nil {
		return f, err
	}
	if f.Statuses, err = apiquery.EnumList(values, "status", Statuses...); err != nil {
		return f, err
	}
	created, err := apiquery.DateRange(values, "created")
	if err != nil {
		return f, err
	}
	f.CreatedFrom, f.CreatedBefore = created.From, created.Before
	for _, raw := range apiquery.CSVValues(values, "organization_uuid") {
		id, perr := uuid.Parse(raw)
		if perr != nil {
			return f, &apiquery.ValidationError{Details: []apiquery.Detail{{
				Field: "organization_uuid", Message: "must be a list of uuids", Code: "invalid",
			}}}
		}
		f.OrganizationUUIDs = append(f.OrganizationUUIDs, id)
	}
	if f.Sort, err = apiquery.ResolveSort(q.Sort, ListSort); err != nil {
		return f, err
	}
	return f, nil
}

func tsArg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
