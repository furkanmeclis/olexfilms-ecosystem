package usecase

// TEC-213: Cmd+K global search over several indexes. Every list scoped
// spec (customers, vehicles, services, warranties, orders, organizations,
// stock units) is answered by its module list (Group.Search), so the
// index filter and the Postgres reload of the list scope apply exactly as
// on GET /v1/<module>?q=. A list scoped spec without a registered Group
// is never searched; of the other specs only the tenant / brand scoped
// ones (they carry an index filter) join, platform-wide specs (users,
// roles) stay on GET /v1/search. The groups run in parallel; a group the
// caller lacks the permission or the module feature for is not queried at
// all.

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
)

// Global search limits.
const (
	DefaultGroupLimit = 5
	MaxGroupLimit     = 20
	MaxQueryLength    = 100
	// InfoSearchDisabled: Meilisearch is off; the groups are empty.
	InfoSearchDisabled = "search_disabled"
)

// ErrQueryTooLong: q is longer than MaxQueryLength runes.
var ErrQueryTooLong = errors.New("search: q must be at most 100 characters")

// Caller is the request principal in its active organization plus the
// resolved scope of one group's permission (what RequireScope stores for
// the module list route).
type Caller struct {
	Principal authctx.Principal
	Org       orgctx.Scope
	Filter    scopefilter.Filter
}

// Group answers one list scoped spec for the palette.
type Group struct {
	Spec string
	// Permission is the list route's scope slug (e.g. services.read).
	Permission string
	// Feature is the list route's module key ("" = none).
	Feature string
	// Search runs the module list with q and returns the matching record
	// uuids in rank order (at most limit).
	Search func(ctx context.Context, c Caller, q string, limit int32) ([]uuid.UUID, error)
}

// FeatureChecker reports whether a module is on for an organization.
type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

// GroupResult is one spec's hits.
type GroupResult struct {
	Spec     string             `json:"spec"`
	LabelKey string             `json:"label_key"`
	Icon     string             `json:"icon,omitempty"`
	Items    []searchengine.Hit `json:"items"`
}

// GlobalResult is the GET /v1/search/global payload.
type GlobalResult struct {
	Enabled bool          `json:"enabled"`
	Info    *string       `json:"info"`
	Groups  []GroupResult `json:"groups"`
}

// SetGroups wires the TEC-213 global search: finder tells whether the
// index is up, tree resolves the scopes, features the module gates.
func (s *Service) SetGroups(finder searchengine.ListFinder, tree scopefilter.TreeReader, features FeatureChecker, groups ...Group) {
	s.finder, s.tree, s.features, s.groups = finder, tree, features, groups
}

func (s *Service) globalEnabled() bool { return s.finder != nil && s.finder.Enabled() }

// pending is one group to query.
type pending struct {
	res GroupResult
	run func(ctx context.Context) []searchengine.Hit
}

// Global searches every group the caller may read in org; spec narrows
// to one group. limit is per group.
func (s *Service) Global(ctx context.Context, org orgctx.Scope, q, spec string, limit int) (GlobalResult, error) {
	out := GlobalResult{Enabled: s.globalEnabled(), Groups: []GroupResult{}}
	if !out.Enabled {
		info := InfoSearchDisabled
		out.Info = &info
	}
	q, spec = strings.TrimSpace(q), strings.TrimSpace(spec)
	if len([]rune(q)) > MaxQueryLength {
		return out, ErrQueryTooLong
	}
	if limit <= 0 {
		limit = DefaultGroupLimit
	}
	if limit > MaxGroupLimit {
		limit = MaxGroupLimit
	}
	p, ok := authctx.PrincipalFrom(ctx)
	if !ok {
		return out, nil
	}
	jobs := s.recordJobs(ctx, p, org, q, spec, limit)
	jobs = append(jobs, s.specJobs(ctx, q, spec, limit)...)
	results := make([][]searchengine.Hit, len(jobs))
	if q != "" && out.Enabled {
		var wg sync.WaitGroup
		for i, j := range jobs {
			if j.run == nil {
				continue
			}
			wg.Add(1)
			go func(i int, j pending) {
				defer wg.Done()
				results[i] = j.run(ctx)
			}(i, j)
		}
		wg.Wait()
	}
	for i, j := range jobs {
		j.res.Items = results[i]
		if j.res.Items == nil {
			j.res.Items = []searchengine.Hit{}
		}
		out.Groups = append(out.Groups, j.res)
	}
	return out, nil
}

// recordJobs builds the list scoped groups the caller may read: the
// permission resolved like RequireScope, the module feature like
// RequireFeature. Anything else is skipped without a query.
func (s *Service) recordJobs(ctx context.Context, p authctx.Principal, org orgctx.Scope, q, spec string, limit int) []pending {
	var out []pending
	for _, g := range s.groups {
		if spec != "" && g.Spec != spec {
			continue
		}
		meta, ok := s.specMeta(g.Spec)
		if !ok || g.Search == nil {
			continue
		}
		if _, granted := p.ScopeFor(g.Permission); !granted {
			continue
		}
		if g.Feature != "" && s.features != nil {
			on, err := s.features.Enabled(ctx, org.InternalID, g.Feature)
			if err != nil {
				s.log.Warn("search_global_feature_check_failed", "spec", g.Spec, "error", err)
				continue
			}
			if !on {
				continue
			}
		}
		orgCopy := org
		f, err := scopefilter.Resolve(ctx, s.tree, p, &orgCopy, g.Permission)
		if err != nil {
			if !errors.Is(err, scopefilter.ErrForbidden) && !errors.Is(err, scopefilter.ErrOrganizationRequired) {
				s.log.Warn("search_global_scope_failed", "spec", g.Spec, "error", err)
			}
			continue
		}
		c := Caller{Principal: p, Org: org, Filter: f}
		g := g
		out = append(out, pending{res: meta, run: func(ctx context.Context) []searchengine.Hit {
			ids, err := g.Search(ctx, c, q, int32(limit))
			if err != nil {
				s.log.Warn("search_global_group_failed", "spec", g.Spec, "error", err)
				return nil
			}
			return s.hits(ctx, g.Spec, ids, limit)
		}})
	}
	return out
}

// specJobs adds the non list scoped specs that carry an index filter
// (tenant / brand scoped, e.g. catalog products) with the filter of
// GET /v1/search.
func (s *Service) specJobs(ctx context.Context, q, spec string, limit int) []pending {
	var specs []searchengine.Spec
	for _, sp := range s.ListSpecs(ctx) {
		if spec != "" && sp.ID != spec {
			continue
		}
		if sp.TenantScoped || sp.BrandScoped {
			specs = append(specs, sp)
		}
	}
	if len(specs) == 0 {
		return nil
	}
	ids, filters := s.specsWithFilters(ctx, specs)
	out := make([]pending, 0, len(ids))
	for _, id := range ids {
		meta, _ := s.specMeta(id)
		id := id
		filter := filters[id]
		var run func(context.Context) []searchengine.Hit
		if s.Enabled() {
			run = func(ctx context.Context) []searchengine.Hit {
				hits, err := s.client.Search(ctx, []string{id}, q, limit, map[string]string{id: filter})
				if err != nil {
					s.log.Warn("search_global_group_failed", "spec", id, "error", err)
					return nil
				}
				return hits
			}
		}
		out = append(out, pending{res: meta, run: run})
	}
	return out
}

func (s *Service) specMeta(id string) (GroupResult, bool) {
	if s.reg == nil {
		return GroupResult{}, false
	}
	a, err := s.reg.Get(id)
	if err != nil {
		return GroupResult{}, false
	}
	sp := a.Spec()
	return GroupResult{Spec: sp.ID, LabelKey: sp.LabelKey, Icon: sp.Icon}, true
}

// hits renders the records the module list returned with their index
// document (title, subtitle, link). A record that is no longer indexable
// is left out.
func (s *Service) hits(ctx context.Context, spec string, ids []uuid.UUID, limit int) []searchengine.Hit {
	a, err := s.reg.Get(spec)
	if err != nil {
		return nil
	}
	out := make([]searchengine.Hit, 0, len(ids))
	for _, id := range ids {
		if len(out) >= limit {
			break
		}
		doc, err := a.Document(ctx, id.String())
		if err != nil {
			if !errors.Is(err, searchengine.ErrSkipDocument) {
				s.log.Warn("search_global_document_failed", "spec", spec, "error", err)
			}
			continue
		}
		icon := doc.Icon
		if icon == "" {
			icon = a.Spec().Icon
		}
		out = append(out, searchengine.Hit{
			Spec: spec, ID: id.String(), Title: doc.Title, Subtitle: doc.Subtitle, Href: doc.Href, Icon: icon,
		})
	}
	return out
}
