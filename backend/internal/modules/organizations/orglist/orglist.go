// Package orglist parses and resolves the platform organizations list
// filters (TEC-365). The list endpoint, its export and its "select all
// matching" bulk target share it, so all three select the same rows.
package orglist

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Statuses are the organizations.status values (chk_organizations_status).
var Statuses = []string{"pending", "active", "read_only", "suspended", "expired"}

// Types are the organization tree types.
var Types = []string{"center", "distributor", "dealer"}

// QueryBrandID is the export / bulk query key the platform handlers stamp
// with the request brand; jobs run without a request and scope by it.
const QueryBrandID = "brand_id"

// Filter narrows and orders the platform organizations list.
type Filter struct {
	Q          string
	Statuses   []string
	Types      []string
	PlanCodes  []string
	ParentUUID *uuid.UUID
	AccessEnds apiquery.TimeRange
	Created    apiquery.TimeRange
	// SortKey/SortDesc come from apiquery.ResolveSort(…, TenantsSortSpec).
	SortKey  string
	SortDesc bool
}

// Parse reads q, status, type, plan_code (multi-value), parent_uuid,
// access_ends_from/_to, created_from/_to and sort. Bad input is an
// *apiquery.ValidationError.
func Parse(values url.Values) (Filter, error) {
	var f Filter
	sort, err := apiquery.ResolveSort(apiquery.ParseSort(values.Get("sort")), apiquery.TenantsSortSpec)
	if err != nil {
		return f, err
	}
	f.SortKey, f.SortDesc = sort.Key, sort.Desc
	if f.Statuses, err = apiquery.EnumList(values, "status", Statuses...); err != nil {
		return f, err
	}
	if f.Types, err = apiquery.EnumList(values, "type", Types...); err != nil {
		return f, err
	}
	f.PlanCodes = apiquery.CSVValues(values, "plan_code")
	if raw := strings.TrimSpace(values.Get("parent_uuid")); raw != "" {
		parent, err := uuid.Parse(raw)
		if err != nil {
			return f, &apiquery.ValidationError{Details: []apiquery.Detail{{
				Field: "parent_uuid", Message: "parent_uuid is invalid", Code: "invalid",
			}}}
		}
		f.ParentUUID = &parent
	}
	if f.AccessEnds, err = apiquery.DateRange(values, "access_ends"); err != nil {
		return f, err
	}
	if f.Created, err = apiquery.DateRange(values, "created"); err != nil {
		return f, err
	}
	f.Q = strings.TrimSpace(values.Get("q"))
	return f, nil
}

// noParent is a parent id no row has: a parent outside the brand (or
// unknown) matches nothing instead of widening the list.
const noParent int64 = -1

// Params resolves f inside brandID into list query params (limit/offset
// left to the caller).
func Params(ctx context.Context, q *db.Queries, brandID int64, f Filter) (db.ListOrganizationsFilteredParams, error) {
	p := db.ListOrganizationsFilteredParams{
		Statuses: f.Statuses, BrandID: pgtype.Int8{Int64: brandID, Valid: true},
		Types: f.Types, PlanCodes: f.PlanCodes,
		SortKey: f.SortKey, SortDesc: f.SortDesc,
	}
	if f.ParentUUID != nil {
		parent, err := q.GetOrganizationByUUID(ctx, *f.ParentUUID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			p.ParentID = pgtype.Int8{Int64: noParent, Valid: true}
		case err != nil:
			return p, err
		case parent.BrandID != brandID:
			p.ParentID = pgtype.Int8{Int64: noParent, Valid: true}
		default:
			p.ParentID = pgtype.Int8{Int64: parent.ID, Valid: true}
		}
	}
	p.AccessEndsFrom, p.AccessEndsBefore = ts(f.AccessEnds)
	p.CreatedFrom, p.CreatedBefore = ts(f.Created)
	if f.Q != "" {
		p.Q = pgtype.Text{String: f.Q, Valid: true}
	}
	return p, nil
}

// CountParams is the count query for the same filter.
func CountParams(p db.ListOrganizationsFilteredParams) db.CountOrganizationsParams {
	return db.CountOrganizationsParams{
		Statuses: p.Statuses, BrandID: p.BrandID, Types: p.Types, ParentID: p.ParentID,
		PlanCodes: p.PlanCodes, AccessEndsFrom: p.AccessEndsFrom, AccessEndsBefore: p.AccessEndsBefore,
		CreatedFrom: p.CreatedFrom, CreatedBefore: p.CreatedBefore, Q: p.Q,
	}
}

// QueryValues turns a stored export / bulk query map into url.Values.
func QueryValues(query map[string]string) url.Values {
	out := make(url.Values, len(query))
	for k, v := range query {
		out.Set(k, v)
	}
	return out
}

func ts(r apiquery.TimeRange) (from, before pgtype.Timestamptz) {
	if r.From != nil {
		from = pgtype.Timestamptz{Time: *r.From, Valid: true}
	}
	if r.Before != nil {
		before = pgtype.Timestamptz{Time: *r.Before, Valid: true}
	}
	return from, before
}
