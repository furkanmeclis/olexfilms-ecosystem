package usecase

// TEC-377 (DT-BE-7): list contract of GET /v1/warranty-claims
// (docs/list-contract.md): limit / offset through apiquery.Parse (default
// 20, at most 100), q, service_uuid, multi-value status and
// organization_uuid, created_from / created_to as a date or RFC3339.

import (
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// ListStatuses are the claim statuses (chk_warranty_claims_status).
var ListStatuses = []string{
	StatusOpen, StatusDealerReview, StatusCenterReview, StatusApproved, StatusRejected, StatusReapplied, StatusClosed,
}

// ListSort is the sort contract of the claim list. status sorts by the
// flow rank (open → closed); decided_at keeps undecided claims last in
// both directions.
var ListSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"claim_no": "claim_no", "status": "status", "created_at": "created_at",
		"updated_at": "updated_at", "decided_at": "decided_at",
	},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

const maxListQuery = 100

func listInvalid(field, msg string) error {
	return &apiquery.ValidationError{Details: []apiquery.Detail{{Field: field, Message: msg, Code: "invalid"}}}
}

func optUUID(values url.Values, key string) (uuid.UUID, error) {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, listInvalid(key, "must be a uuid")
	}
	return id, nil
}

// ParseListFilter reads the list parameters. Errors are
// *apiquery.ValidationError (400).
func ParseListFilter(values url.Values) (model.ListFilter, error) {
	q := apiquery.Parse(values)
	f := model.ListFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	if utf8.RuneCountInString(f.Q) > maxListQuery {
		return f, listInvalid("q", "must be at most 100 characters")
	}
	var err error
	if f.Statuses, err = apiquery.EnumList(values, "status", ListStatuses...); err != nil {
		return f, err
	}
	if f.WarrantyID, err = optUUID(values, "warranty_uuid"); err != nil {
		return f, err
	}
	if f.ServiceID, err = optUUID(values, "service_uuid"); err != nil {
		return f, err
	}
	if f.VehicleID, err = optUUID(values, "vehicle_uuid"); err != nil {
		return f, err
	}
	for _, raw := range apiquery.CSVValues(values, "organization_uuid") {
		id, err := uuid.Parse(raw)
		if err != nil {
			return f, listInvalid("organization_uuid", "must be a list of uuids")
		}
		f.OrganizationUUIDs = append(f.OrganizationUUIDs, id)
	}
	created, err := apiquery.DateRange(values, "created")
	if err != nil {
		return f, err
	}
	f.CreatedFrom, f.CreatedTo = created.From, created.Before
	sort, err := apiquery.ResolveSort(q.Sort, ListSort)
	if err != nil {
		return f, err
	}
	f.SortKey, f.SortDesc = sort.Key, sort.Desc
	return f, nil
}

// escapeLike escapes the LIKE wildcards of a user search term.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
