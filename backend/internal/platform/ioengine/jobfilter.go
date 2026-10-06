package ioengine

// TEC-373 (DT-BE-5): list exports store the caller's resolved permission
// scope in the job query (EncodeScopeFilter) and the worker rebuilds it
// against the job organization (JobScopeFilter), the pattern of the
// customer list export (TEC-164) shared by the order and stock unit list
// exports.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
)

// QueryScopeFilter is the job query key of the encoded scope.
const QueryScopeFilter = "_scope"

// ScopeFilterBrand is the encoded scope of a brand wide (or all) grant.
const ScopeFilterBrand = "brand"

// ErrJobScope: the stored scope reaches no organization of the job.
var ErrJobScope = errors.New("export scope: the stored scope is outside the job organization")

// EncodeScopeFilter encodes a resolved scope for a job query: "brand" or
// the comma separated organization ids. ok is false when the scope reaches
// no organization (customer scope or an empty set).
func EncodeScopeFilter(f scopefilter.Filter) (string, bool) {
	ids := f.OrgIDsArg()
	if ids == nil {
		return ScopeFilterBrand, true
	}
	if len(ids) == 0 {
		return "", false
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ","), true
}

// JobScopeFilter rebuilds the scope of a job of organization org from its
// encoded form, re-authorized against the organization: a brand wide scope
// needs a center (elsewhere it falls back to the organization's subtree),
// organization ids are kept only when org covers them (itself or below).
func JobScopeFilter(ctx context.Context, q *db.Queries, org db.Organization, raw string) (scopefilter.Filter, error) {
	raw = strings.TrimSpace(raw)
	if raw == ScopeFilterBrand && org.Type == rbac.OrgTypeCenter {
		return scopefilter.Filter{Scope: rbac.ScopeBrand, OrgID: org.ID, BrandID: org.BrandID}, nil
	}
	below, err := q.Descendants(ctx, org.ID)
	if err != nil {
		return scopefilter.Filter{}, fmt.Errorf("export scope: descendants: %w", err)
	}
	covered := map[int64]bool{org.ID: true}
	tree := []int64{org.ID}
	for _, o := range below {
		covered[o.ID] = true
		tree = append(tree, o.ID)
	}
	var ids []int64
	if raw == ScopeFilterBrand {
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
		return scopefilter.Filter{}, ErrJobScope
	}
	return scopefilter.Filter{Scope: rbac.ScopeSubtree, OrgID: org.ID, OrgIDs: ids}, nil
}
