package activity

import (
	"net/url"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// SortSpec is the GET /v1/platform/activity sort contract (TEC-365); the
// export of the list applies the same sort.
var SortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"created_at": "created_at", "action": "action", "resource": "resource"},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// ListParams parses the activity list filters and sort shared by the list
// endpoint and its export: q, actor (user uuid), resource and action
// (multi-value), created_from / created_to and sort. Limit/offset are left
// to the caller. Bad input is an *apiquery.ValidationError.
func ListParams(values url.Values) (db.ListActivityEventsParams, error) {
	var out db.ListActivityEventsParams
	sort, err := apiquery.ResolveSort(apiquery.ParseSort(values.Get("sort")), SortSpec)
	if err != nil {
		return out, err
	}
	out.SortKey, out.SortDesc = sort.Key, sort.Desc
	if raw := strings.TrimSpace(values.Get("actor")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return out, &apiquery.ValidationError{Details: []apiquery.Detail{{
				Field: "actor", Message: "must be a user uuid", Code: "invalid",
			}}}
		}
		out.ActorUuid = pgtype.UUID{Bytes: id, Valid: true}
	}
	out.Resources = apiquery.CSVValues(values, "resource")
	out.Actions = apiquery.CSVValues(values, "action")
	created, err := apiquery.DateRange(values, "created")
	if err != nil {
		return out, err
	}
	if created.From != nil {
		out.CreatedFrom = pgtype.Timestamptz{Time: *created.From, Valid: true}
	}
	if created.Before != nil {
		out.CreatedBefore = pgtype.Timestamptz{Time: *created.Before, Valid: true}
	}
	if q := strings.TrimSpace(values.Get("q")); q != "" {
		out.Q = pgtype.Text{String: q, Valid: true}
	}
	return out, nil
}
