package usecase

// TEC-377 (DT-BE-7): list contract of GET /v1/services
// (docs/list-contract.md). The list endpoint and the service list export
// read the same parameters with ParseListFilter, so both select the same
// rows.

import (
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ListSort is the sort contract of the service list. status sorts by the
// flow rank (draft → cancelled), organization by the organization name;
// plate and completed_at keep empty values last in both directions.
var ListSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"service_no": "service_no", "status": "status", "created_at": "created_at",
		"updated_at": "updated_at", "completed_at": "completed_at", "plate": "plate",
		"organization": "organization",
	},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// List query keys.
const (
	QueryStatus           = "status"
	QueryOrganizationUUID = "organization_uuid"
	QueryCustomerUUID     = "customer_uuid"
	QueryVehicleUUID      = "vehicle_uuid"
)

// listKeys are the list parameters an export job carries.
var listKeys = []string{
	"q", "sort", QueryStatus, QueryOrganizationUUID, QueryCustomerUUID, QueryVehicleUUID,
	"created_from", "created_to", "completed_from", "completed_to",
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

// ParseListFilter reads q, status (CSV), organization_uuid (CSV),
// customer_uuid, vehicle_uuid, created_from / created_to,
// completed_from / completed_to, sort, limit and offset. Errors are
// *apiquery.ValidationError (400).
func ParseListFilter(values url.Values) (ListFilter, error) {
	q := apiquery.Parse(values)
	f := ListFilter{
		Q: q.Q, CustomerUUID: strings.TrimSpace(values.Get(QueryCustomerUUID)),
		VehicleUUID: strings.TrimSpace(values.Get(QueryVehicleUUID)), Limit: q.Limit, Offset: q.Offset,
	}
	var err error
	if f.Statuses, err = apiquery.EnumList(values, QueryStatus, Statuses...); err != nil {
		return f, err
	}
	if f.OrganizationUUIDs, err = UUIDList(values, QueryOrganizationUUID); err != nil {
		return f, err
	}
	created, err := apiquery.DateRange(values, "created")
	if err != nil {
		return f, err
	}
	f.CreatedFrom, f.CreatedTo = created.From, created.Before
	completed, err := apiquery.DateRange(values, "completed")
	if err != nil {
		return f, err
	}
	f.CompletedFrom, f.CompletedTo = completed.From, completed.Before
	if f.Sort, err = apiquery.ResolveSort(q.Sort, ListSort); err != nil {
		return f, err
	}
	f.SortExplicit = len(q.Sort) > 0
	return f, nil
}

// UUIDList reads a CSV list of uuids (400 on a bad value).
func UUIDList(values url.Values, key string) ([]uuid.UUID, error) {
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

// sqlOnly reports whether the filter uses a parameter the services index
// cannot answer (TEC-209 keeps q on the index otherwise).
func (f ListFilter) sqlOnly() bool {
	return f.SortExplicit || f.CreatedFrom != nil || f.CreatedTo != nil ||
		f.CompletedFrom != nil || f.CompletedTo != nil || len(f.OrganizationUUIDs) > 0
}

// sortArgs is the resolved sort (the default when the filter has none).
func (f ListFilter) sortArgs() (string, bool) {
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
