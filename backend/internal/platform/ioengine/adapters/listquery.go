package adapters

import (
	"net/url"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/jackc/pgx/v5/pgtype"
)

// List query helpers (TEC-365): "select all matching" and export read the
// list filters from the stored query map with the same parsers as the list
// endpoint, so both resolve the same rows.

func queryValues(query map[string]string) url.Values {
	out := make(url.Values, len(query))
	for k, v := range query {
		out.Set(k, v)
	}
	return out
}

// rangeArgs parses <prefix>_from / <prefix>_to into from / before args.
func rangeArgs(query map[string]string, prefix string) (pgtype.Timestamptz, pgtype.Timestamptz, error) {
	var from, before pgtype.Timestamptz
	r, err := apiquery.DateRange(queryValues(query), prefix)
	if err != nil {
		return from, before, err
	}
	if r.From != nil {
		from = pgtype.Timestamptz{Time: *r.From, Valid: true}
	}
	if r.Before != nil {
		before = pgtype.Timestamptz{Time: *r.Before, Valid: true}
	}
	return from, before, nil
}

// boolArg parses a true|false filter.
func boolArg(query map[string]string, key string) (pgtype.Bool, error) {
	b, err := apiquery.Bool(queryValues(query), key)
	if err != nil || b == nil {
		return pgtype.Bool{}, err
	}
	return pgtype.Bool{Bool: *b, Valid: true}, nil
}
