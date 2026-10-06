package usecase

// TEC-371 (DT-BE-4): list contract of GET /v1/customers and GET
// /v1/vehicles (docs/list-contract.md). The handlers, the customer list
// export and its worker parse the request with the same functions, so all
// of them select the same rows in the same order.

import (
	"net/url"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// CustomersSortSpec is the sort whitelist of GET /v1/customers.
var CustomersSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"name": "name", "email": "email", "status": "status",
		"linked_at": "linked_at", "first_service_at": "first_service_at",
	},
	Default: apiquery.SortField{Field: "linked_at", Desc: true},
}

// VehiclesSortSpec is the sort whitelist of GET /v1/vehicles.
var VehiclesSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"plate": "plate", "brand": "brand", "model": "model",
		"model_year": "model_year", "created_at": "created_at",
	},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// CustomerStatuses are the values of the status filter.
var CustomerStatuses = []string{"active", "pending", "disabled", StatusAnonymized}

// CustomerTypes are the values of the type filter.
var CustomerTypes = []string{TypeIndividual, "corporate"}

// List query keys shared by the list, the export body and the export job.
const (
	QueryType             = "type"
	QueryLinkedFrom       = "linked_from"
	QueryLinkedTo         = "linked_to"
	QueryOrganizationUUID = "organization_uuid"
	QuerySort             = "sort"
)

// customerListKeys are the list parameters an export carries over.
var customerListKeys = []string{QueryQ, QueryStatus, QueryType, QueryLinkedFrom, QueryLinkedTo, QueryOrganizationUUID, QuerySort}

// ParseListFilter reads the GET /v1/customers filters (q, status CSV, type
// CSV, linked_from/_to, organization_uuid CSV, sort). Limit and offset are
// left to the caller. Bad values are *apiquery.ValidationError (400).
func ParseListFilter(values url.Values) (ListFilter, error) {
	f := ListFilter{Q: strings.TrimSpace(values.Get(QueryQ))}
	var err error
	if f.Statuses, err = apiquery.EnumList(values, QueryStatus, CustomerStatuses...); err != nil {
		return f, err
	}
	if f.Types, err = apiquery.EnumList(values, QueryType, CustomerTypes...); err != nil {
		return f, err
	}
	if f.Linked, err = apiquery.DateRange(values, "linked"); err != nil {
		return f, err
	}
	if f.OrganizationUUIDs, err = UUIDList(values, QueryOrganizationUUID); err != nil {
		return f, err
	}
	sorts := apiquery.ParseSort(values.Get(QuerySort))
	if f.Sort, err = apiquery.ResolveSort(sorts, CustomersSortSpec); err != nil {
		return f, err
	}
	f.SortExplicit = len(sorts) > 0
	return f, nil
}

// ParseVehicleFilter reads the GET /v1/vehicles list filters added by
// TEC-371 (car_brand_uuid / car_model_uuid / organization_uuid CSV, sort)
// into f.
func ParseVehicleFilter(values url.Values, f *VehicleFilter) error {
	var err error
	if f.CarBrandUUIDs, err = UUIDList(values, "car_brand_uuid"); err != nil {
		return err
	}
	if f.CarModelUUIDs, err = UUIDList(values, "car_model_uuid"); err != nil {
		return err
	}
	if f.OrganizationUUIDs, err = UUIDList(values, QueryOrganizationUUID); err != nil {
		return err
	}
	sorts := apiquery.ParseSort(values.Get(QuerySort))
	if f.Sort, err = apiquery.ResolveSort(sorts, VehiclesSortSpec); err != nil {
		return err
	}
	f.SortExplicit = len(sorts) > 0
	return nil
}

// UUIDList parses a multi-value UUID filter (CSV or repeated key). It
// returns nil when the parameter is absent.
func UUIDList(values url.Values, key string) ([]uuid.UUID, error) {
	vals := apiquery.CSVValues(values, key)
	if len(vals) == 0 {
		return nil, nil
	}
	out := make([]uuid.UUID, 0, len(vals))
	for _, v := range vals {
		id, err := uuid.Parse(v)
		if err != nil {
			return nil, &apiquery.ValidationError{Details: []apiquery.Detail{{
				Field: key, Message: "must be a comma separated list of UUIDs", Code: "invalid",
			}}}
		}
		out = append(out, id)
	}
	return out, nil
}

func tsArg(t *apiquery.TimeRange, from bool) pgtype.Timestamptz {
	v := t.Before
	if from {
		v = t.From
	}
	if v == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *v, Valid: true}
}

// sortKey is the resolved sort, the spec default when the filter carries
// none (a zero ListFilter from an internal caller).
func sortKey(s apiquery.ResolvedSort, spec apiquery.SortSpec) apiquery.ResolvedSort {
	if s.Key != "" {
		return s
	}
	def, _ := apiquery.ResolveSort(nil, spec)
	return def
}
