package usecase

import (
	"net/url"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/jackc/pgx/v5/pgtype"
)

// SortSpec is the exports job list sort contract (TEC-365), platform and
// tenant lists alike.
var SortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"created_at": "created_at", "updated_at": "updated_at",
		"status": "status", "resource": "resource", "format": "format",
	},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// JobStatuses and JobFormats are the export_jobs check constraint values.
var (
	JobStatuses = []string{"queued", "processing", "completed", "failed", "expired"}
	JobFormats  = []string{"pdf", "xlsx", "csv", "json"}
)

// JobListFilter narrows and orders a job list (TEC-365).
type JobListFilter struct {
	// Q matches the resource (ILIKE).
	Q         string
	Statuses  []string
	Resources []string
	Formats   []string
	Created   apiquery.TimeRange
	SortKey   string
	SortDesc  bool
}

// ParseJobListFilter reads q, status, resource, format (multi-value),
// created_from / created_to and sort. Bad input is an
// *apiquery.ValidationError.
func ParseJobListFilter(values url.Values) (JobListFilter, error) {
	var f JobListFilter
	sort, err := apiquery.ResolveSort(apiquery.ParseSort(values.Get("sort")), SortSpec)
	if err != nil {
		return f, err
	}
	f.SortKey, f.SortDesc = sort.Key, sort.Desc
	if f.Statuses, err = apiquery.EnumList(values, "status", JobStatuses...); err != nil {
		return f, err
	}
	if f.Formats, err = apiquery.EnumList(values, "format", JobFormats...); err != nil {
		return f, err
	}
	f.Resources = apiquery.CSVValues(values, "resource")
	if f.Created, err = apiquery.DateRange(values, "created"); err != nil {
		return f, err
	}
	f.Q = strings.TrimSpace(values.Get("q"))
	return f, nil
}

func (f JobListFilter) qArg() pgtype.Text {
	if f.Q == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: f.Q, Valid: true}
}

func (f JobListFilter) createdArgs() (from, before pgtype.Timestamptz) {
	if f.Created.From != nil {
		from = pgtype.Timestamptz{Time: *f.Created.From, Valid: true}
	}
	if f.Created.Before != nil {
		before = pgtype.Timestamptz{Time: *f.Created.Before, Valid: true}
	}
	return from, before
}
