package usecase

// List contract of GET /v1/campaigns (docs/list-contract.md): limit /
// offset through apiquery.Parse, sort created_at | scheduled_at | name |
// status (flow rank), default -created_at with an id tiebreak; status and
// channel are multi-valued (channel matches on overlap); scheduled_from /
// scheduled_to as a date or RFC3339; q matches the name.

import (
	"context"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/jackc/pgx/v5/pgtype"
)

// ListSort is the sort contract of the campaign list.
var ListSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"created_at": "created_at", "scheduled_at": "scheduled_at", "name": "name", "status": "status",
	},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

const maxListQuery = 100

// ListFilter is the parsed list query.
type ListFilter struct {
	Q             string
	Statuses      []string
	Channels      []string
	ScheduledFrom *time.Time
	ScheduledTo   *time.Time
	SortKey       string
	SortDesc      bool
	Limit         int32
	Offset        int32
}

// ParseListFilter reads the list parameters; errors are
// *apiquery.ValidationError (400).
func ParseListFilter(values url.Values) (ListFilter, error) {
	q := apiquery.Parse(values)
	f := ListFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	if utf8.RuneCountInString(f.Q) > maxListQuery {
		return f, &apiquery.ValidationError{Details: []apiquery.Detail{{Field: "q", Message: "must be at most 100 characters", Code: "invalid"}}}
	}
	var err error
	if f.Statuses, err = apiquery.EnumList(values, "status", Statuses...); err != nil {
		return f, err
	}
	if f.Channels, err = apiquery.EnumList(values, "channel", Channels...); err != nil {
		return f, err
	}
	scheduled, err := apiquery.DateRange(values, "scheduled")
	if err != nil {
		return f, err
	}
	f.ScheduledFrom, f.ScheduledTo = scheduled.From, scheduled.Before
	sort, err := apiquery.ResolveSort(q.Sort, ListSort)
	if err != nil {
		return f, err
	}
	f.SortKey, f.SortDesc = sort.Key, sort.Desc
	return f, nil
}

// List returns the campaigns of the caller's read scope in the brand.
func (s *Service) List(ctx context.Context, c Caller, f ListFilter) (apiquery.Page[Campaign], error) {
	var q pgtype.Text
	if f.Q != "" {
		q = pgtype.Text{String: escapeLike(f.Q), Valid: true}
	}
	orgIDs := c.Filter.OrgIDsArg()
	rows, err := s.q.ListCampaigns(ctx, db.ListCampaignsParams{
		BrandID: c.BrandID, OrganizationIds: orgIDs, Statuses: f.Statuses, Channels: f.Channels,
		ScheduledFrom: pgTime(f.ScheduledFrom), ScheduledTo: pgTime(f.ScheduledTo), Q: q,
		SortKey: f.SortKey, SortDesc: f.SortDesc, PageLimit: f.Limit, PageOffset: f.Offset,
	})
	if err != nil {
		return apiquery.Page[Campaign]{}, err
	}
	total, err := s.q.CountCampaigns(ctx, db.CountCampaignsParams{
		BrandID: c.BrandID, OrganizationIds: orgIDs, Statuses: f.Statuses, Channels: f.Channels,
		ScheduledFrom: pgTime(f.ScheduledFrom), ScheduledTo: pgTime(f.ScheduledTo), Q: q,
	})
	if err != nil {
		return apiquery.Page[Campaign]{}, err
	}
	items, err := s.views(ctx, rows)
	if err != nil {
		return apiquery.Page[Campaign]{}, err
	}
	return apiquery.NewPage(items, total, f.Limit, f.Offset), nil
}

// escapeLike escapes the LIKE wildcards of a user search term.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
