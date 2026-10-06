package usecase

// TEC-377 (DT-BE-7): list contract of GET /v1/portal/services
// (docs/list-contract.md).

import (
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ServiceStatuses are the service statuses the portal shows (drafts are
// dealer-internal and never listed).
var ServiceStatuses = []string{"pending", "processing", "ready", "completed", "cancelled"}

// ServiceSort is the sort contract of the portal service list. status
// sorts by the flow rank, organization by name; completed_at keeps open
// services last in both directions.
var ServiceSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"created_at": "created_at", "completed_at": "completed_at", "service_no": "service_no",
		"status": "status", "organization": "organization",
	},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// maxServiceQuery bounds q.
const maxServiceQuery = 100

// ServiceListFilter narrows GET /v1/portal/services. CreatedFrom is
// inclusive, CreatedBefore exclusive; a zero Sort is the ServiceSort
// default.
type ServiceListFilter struct {
	Page
	Q                 string
	Statuses          []string
	OrganizationUUIDs []uuid.UUID
	CreatedFrom       *time.Time
	CreatedBefore     *time.Time
	Sort              apiquery.ResolvedSort
}

// ParseServiceListFilter reads q, status (CSV), organization_uuid (CSV),
// created_from / created_to, sort, limit and offset. Errors are
// *apiquery.ValidationError (400).
func ParseServiceListFilter(values url.Values) (ServiceListFilter, error) {
	q := apiquery.Parse(values)
	f := ServiceListFilter{Page: Page{Limit: q.Limit, Offset: q.Offset}, Q: q.Q}
	if utf8.RuneCountInString(f.Q) > maxServiceQuery {
		return f, &apiquery.ValidationError{Details: []apiquery.Detail{{
			Field: "q", Message: "must be at most 100 characters", Code: "invalid",
		}}}
	}
	var err error
	if f.Statuses, err = apiquery.EnumList(values, "status", ServiceStatuses...); err != nil {
		return f, err
	}
	for _, raw := range apiquery.CSVValues(values, "organization_uuid") {
		id, err := uuid.Parse(raw)
		if err != nil {
			return f, &apiquery.ValidationError{Details: []apiquery.Detail{{
				Field: "organization_uuid", Message: "must be a list of uuids", Code: "invalid",
			}}}
		}
		f.OrganizationUUIDs = append(f.OrganizationUUIDs, id)
	}
	created, err := apiquery.DateRange(values, "created")
	if err != nil {
		return f, err
	}
	f.CreatedFrom, f.CreatedBefore = created.From, created.Before
	f.Sort, err = apiquery.ResolveSort(q.Sort, ServiceSort)
	return f, err
}

func (f ServiceListFilter) sortArgs() (string, bool) {
	if f.Sort.Key == "" {
		return ServiceSort.Columns[ServiceSort.Default.Field], ServiceSort.Default.Desc
	}
	return f.Sort.Key, f.Sort.Desc
}

func optTS(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// escapeLike escapes the LIKE wildcards of a user search term.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
