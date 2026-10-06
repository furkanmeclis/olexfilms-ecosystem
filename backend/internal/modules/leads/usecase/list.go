package usecase

// TEC-371 (DT-BE-4): list contract of GET /v1/leads (docs/list-contract.md).
// The handler, the list export and "select all matching" bulk runs parse
// the same parameters with ParseListFilter, and a job carries the caller's
// resolved scope (EncodeScope) that the worker re-authorizes against the
// job organization (JobScope).

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

// LeadsSortSpec is the sort whitelist of GET /v1/leads.
var LeadsSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"created_at": "created_at", "follow_up_date": "follow_up_date",
		"status": "status", "temperature": "temperature", "name": "name",
	},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// Filter values of the lead list.
var (
	LeadStatuses     = []string{StatusNew, StatusContacted, StatusQuoted, StatusWon, StatusLost}
	LeadTargetTypes  = []string{"customer", "dealer_candidate", "distributor_candidate"}
	LeadSources      = []string{"incoming_call", "outgoing_call", "walk_in", "whatsapp", "social", "referral", "website", "application_form", "other"}
	LeadTemperatures = []string{"cold", "warm", "hot"}
)

// AssigneeNone is the assignee_user_id filter value of unassigned leads.
const AssigneeNone = "none"

// listKeys are the list parameters an export or a query bulk run carries.
var listKeys = []string{"q", "status", "target_type", "source", "temperature", "assignee_user_id",
	"created_from", "created_to", "follow_up", "sort"}

// ParseListFilter reads the GET /v1/leads filters: q, status /
// target_type / source / temperature CSV, assignee_user_id CSV (user ids or
// "none"), created_from/_to, follow_up=overdue|today and sort. Limit and
// offset are left to the caller. Bad values are *apiquery.ValidationError.
func ParseListFilter(values url.Values) (ListFilter, error) {
	f := ListFilter{Q: strings.TrimSpace(values.Get("q")), FollowUp: strings.TrimSpace(values.Get("follow_up"))}
	var err error
	if f.Statuses, err = apiquery.EnumList(values, "status", LeadStatuses...); err != nil {
		return f, err
	}
	if f.TargetTypes, err = apiquery.EnumList(values, "target_type", LeadTargetTypes...); err != nil {
		return f, err
	}
	if f.Sources, err = apiquery.EnumList(values, "source", LeadSources...); err != nil {
		return f, err
	}
	if f.Temperatures, err = apiquery.EnumList(values, "temperature", LeadTemperatures...); err != nil {
		return f, err
	}
	for _, v := range apiquery.CSVValues(values, "assignee_user_id") {
		if v == AssigneeNone {
			f.Unassigned = true
			continue
		}
		id, perr := strconv.ParseInt(v, 10, 64)
		if perr != nil || id <= 0 {
			return f, &apiquery.ValidationError{Details: []apiquery.Detail{{
				Field: "assignee_user_id", Message: `must be user ids or "none"`, Code: "invalid",
			}}}
		}
		f.AssigneeIDs = append(f.AssigneeIDs, id)
	}
	if f.Created, err = apiquery.DateRange(values, "created"); err != nil {
		return f, err
	}
	sorts := apiquery.ParseSort(values.Get("sort"))
	if f.Sort, err = apiquery.ResolveSort(sorts, LeadsSortSpec); err != nil {
		return f, err
	}
	f.SortExplicit = len(sorts) > 0
	return f, nil
}

// ListValues returns the list parameters of a stored job query (export or
// bulk target) as URL values for ParseListFilter.
func ListValues(q map[string]string) url.Values {
	out := url.Values{}
	for _, k := range listKeys {
		if v := strings.TrimSpace(q[k]); v != "" {
			out.Set(k, v)
		}
	}
	return out
}

// ScopeBrand is the encoded scope of a brand wide (or all) grant.
const ScopeBrand = bulkengine.ScopeBrand

// EncodeScope encodes a resolved leads scope for a job query: "brand" or
// the comma separated organization ids (bulkengine.EncodeScope).
// ErrForbidden when it reaches no organization.
func EncodeScope(f scopefilter.Filter) (string, error) {
	s, ok := bulkengine.EncodeScope(f)
	if !ok {
		return "", ErrForbidden
	}
	return s, nil
}

// errJobScope: the stored scope reaches no organization of the job.
var errJobScope = errors.New("leads: job scope is outside the job organization")

// JobCaller rebuilds the caller of a job (export or bulk run) of
// organization orgID from its encoded scope. The scope is re-authorized
// against the organization: a brand wide scope needs a center, organization
// ids are kept only when the organization covers them (itself or below).
// actorID is the requesting user (follow-up queue, timeline events).
func JobCaller(ctx context.Context, q *db.Queries, orgID int64, raw string, actorID int64) (Caller, error) {
	org, err := q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return Caller{}, fmt.Errorf("leads: job organization: %w", err)
	}
	c := Caller{}
	c.Principal.UserInternal = actorID
	c.Org.InternalID, c.Org.UUID, c.Org.BrandID, c.Org.OrgType, c.Org.Name = org.ID, org.Uuid, org.BrandID, org.Type, org.Name
	raw = strings.TrimSpace(raw)
	if raw == ScopeBrand && org.Type == rbac.OrgTypeCenter {
		c.Filter = scopefilter.Filter{Scope: rbac.ScopeBrand, OrgID: org.ID, BrandID: org.BrandID}
		return c, nil
	}
	below, err := q.Descendants(ctx, org.ID)
	if err != nil {
		return Caller{}, fmt.Errorf("leads: job descendants: %w", err)
	}
	covered := map[int64]bool{org.ID: true}
	tree := []int64{org.ID}
	for _, o := range below {
		covered[o.ID] = true
		tree = append(tree, o.ID)
	}
	var ids []int64
	if raw == ScopeBrand {
		ids = tree
	} else {
		for _, part := range strings.Split(raw, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err == nil && covered[id] {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return Caller{}, errJobScope
	}
	c.Filter = scopefilter.Filter{Scope: rbac.ScopeSubtree, OrgID: org.ID, OrgIDs: ids}
	return c, nil
}
