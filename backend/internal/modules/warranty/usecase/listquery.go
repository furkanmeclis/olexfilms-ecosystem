package usecase

// TEC-377 (DT-BE-7): list contract of GET /v1/warranties and
// GET /v1/portal/warranties (docs/list-contract.md). The panel list, the
// portal list and the warranty list export read the same parameters with
// ParseListFilter.

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Statuses lists the warranty statuses.
var Statuses = []string{StatusActive, StatusExpired, StatusVoid}

// ListSort is the sort contract of the warranty lists. expiry (the
// default) lists active warranties by the soonest end first, then the
// rest by the latest end; -expiry reverses it. status sorts by rank
// (active, expired, void), product and organization by name.
var ListSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"expiry": "expiry", "end_at": "end_at", "start_at": "start_at", "created_at": "created_at",
		"public_code": "public_code", "service_no": "service_no", "status": "status",
		"product": "product", "organization": "organization",
	},
	Default: apiquery.SortField{Field: "expiry"},
}

// List query keys.
const (
	QueryStatus           = "status"
	QueryOrganizationUUID = "organization_uuid"
	QueryProductUUID      = "product_uuid"
	QueryVehicleUUID      = "vehicle_uuid"
	QueryDaysLeftMin      = "days_left_min"
	QueryDaysLeftMax      = "days_left_max"
)

// listKeys are the list parameters an export job carries.
var listKeys = []string{
	"q", "sort", QueryStatus, QueryOrganizationUUID, QueryProductUUID, QueryVehicleUUID,
	QueryDaysLeftMin, QueryDaysLeftMax, "start_from", "start_to", "end_from", "end_to",
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
// product_uuid, vehicle_uuid, days_left_min / days_left_max,
// start_from / start_to, end_from / end_to, sort, limit and offset. Errors
// are *apiquery.ValidationError or *ValidationError (both 400).
func ParseListFilter(values url.Values) (ListFilter, error) {
	q := apiquery.Parse(values)
	f := ListFilter{
		Q: q.Q, ProductUUID: strings.TrimSpace(values.Get(QueryProductUUID)),
		VehicleUUID: strings.TrimSpace(values.Get(QueryVehicleUUID)), Limit: q.Limit, Offset: q.Offset,
	}
	var err error
	if f.Statuses, err = apiquery.EnumList(values, QueryStatus, Statuses...); err != nil {
		return f, err
	}
	for _, raw := range apiquery.CSVValues(values, QueryOrganizationUUID) {
		id, err := uuid.Parse(raw)
		if err != nil {
			return f, &apiquery.ValidationError{Details: []apiquery.Detail{{
				Field: QueryOrganizationUUID, Message: "must be a list of uuids", Code: "invalid",
			}}}
		}
		f.OrganizationUUIDs = append(f.OrganizationUUIDs, id)
	}
	if f.DaysLeftMin, err = optInt(values.Get(QueryDaysLeftMin), QueryDaysLeftMin); err != nil {
		return f, err
	}
	if f.DaysLeftMax, err = optInt(values.Get(QueryDaysLeftMax), QueryDaysLeftMax); err != nil {
		return f, err
	}
	start, err := apiquery.DateRange(values, "start")
	if err != nil {
		return f, err
	}
	f.StartFrom, f.StartBefore = start.From, start.Before
	end, err := apiquery.DateRange(values, "end")
	if err != nil {
		return f, err
	}
	f.EndFrom, f.EndBefore = end.From, end.Before
	if f.Sort, err = apiquery.ResolveSort(q.Sort, ListSort); err != nil {
		return f, err
	}
	f.SortExplicit = len(q.Sort) > 0
	return f, nil
}

func optInt(raw, field string) (*int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil, invalidField(field, "must be an integer")
	}
	return &n, nil
}

// sqlOnly reports whether the filter uses a parameter the warranties index
// cannot answer.
func (f ListFilter) sqlOnly() bool {
	return f.SortExplicit || len(f.OrganizationUUIDs) > 0 || f.StartFrom != nil || f.StartBefore != nil ||
		f.EndFrom != nil || f.EndBefore != nil
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
