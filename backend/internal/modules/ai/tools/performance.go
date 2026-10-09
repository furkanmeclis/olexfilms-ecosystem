package tools

import (
	"context"
	"encoding/json"
	"strings"

	perfuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

type PerformanceReader interface {
	Dashboard(context.Context, perfuc.Caller, string) (perfuc.Dashboard, error)
	ListRanking(context.Context, perfuc.Caller, perfuc.RankingFilter) ([]perfuc.RankingRow, int64, error)
}

type performanceBase struct {
	performance PerformanceReader
	tree        scopefilter.TreeReader
}

func (b performanceBase) caller(ctx context.Context, p Principal) (perfuc.Caller, error) {
	f, err := resolveScope(ctx, b.tree, p, rbac.PermPerformanceRead)
	if err != nil {
		return perfuc.Caller{}, err
	}
	return perfuc.Caller{Principal: p.Auth, Org: *p.Org, Filter: f}, nil
}

func (performanceBase) errs(tool string) errCases {
	return errCases{tool: tool, what: "performance", notFound: []error{perfuc.ErrNotFound}, forbidden: []error{perfuc.ErrForbidden}}
}

func NewPerformanceTools(svc PerformanceReader, tree scopefilter.TreeReader) []Tool {
	if svc == nil {
		return nil
	}
	b := performanceBase{performance: svc, tree: tree}
	return []Tool{PerformanceDashboard{b}, PerformanceRanking{b}}
}

type PerformanceDashboard struct{ performanceBase }

func (PerformanceDashboard) Spec() Spec {
	return Spec{
		Name:        "performance_dashboard",
		Description: "Read the performance dashboard for the active organization: current metrics, previous-period deltas, 12-month trend, targets and subtree summary.",
		InputSchema: object(map[string]any{"period": str("Optional month in YYYY-MM.", 7)}),
		Kind:        KindRead, Realm: RealmPanel, Feature: features.ModulePerformance,
		Permissions: []string{rbac.PermPerformanceRead},
	}
}

func (t PerformanceDashboard) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Period string `json:"period"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	c, err := t.caller(ctx, env.Principal)
	if err != nil {
		return t.errs(t.Spec().Name).result(err)
	}
	item, err := t.performance.Dashboard(ctx, c, strings.TrimSpace(in.Period))
	if err != nil {
		return t.errs(t.Spec().Name).result(err)
	}
	return JSONResult(item)
}

type PerformanceRanking struct{ performanceBase }

func (PerformanceRanking) Spec() Spec {
	return Spec{
		Name:        "performance_ranking",
		Description: "List organization performance ranking for center or distributor users. Dealers should use the dashboard/benchmark UI, not named peer rankings.",
		InputSchema: object(map[string]any{
			"period": str("Optional month in YYYY-MM.", 7),
			"query":  str("Optional organization name search.", 100),
			"sort":   enum("Optional sort metric.", "services_count", "order_volume", "warranty_start_rate", "measurement_rate", "review_avg", "stock_turnover", "contract_days_left", "cari_overdue_amount", "cari_overdue_days", "certificate_coverage", "lead_conversion_rate", "waste_ratio", "name"),
			"limit":  limitProp(),
		}),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModulePerformance,
		Permissions: []string{rbac.PermPerformanceRead},
		OrgTypes:    []string{OrgCenter, OrgDistributor},
	}
}

func (t PerformanceRanking) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Period string `json:"period"`
		Query  string `json:"query"`
		Sort   string `json:"sort"`
		Limit  int    `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	c, err := t.caller(ctx, env.Principal)
	if err != nil {
		return t.errs(t.Spec().Name).result(err)
	}
	f := perfuc.RankingFilter{Period: strings.TrimSpace(in.Period), Q: strings.TrimSpace(in.Query), Limit: limitArg(in.Limit)}
	if in.Sort != "" {
		f.Sort = []apiquery.SortField{{Field: in.Sort, Desc: true}}
	}
	items, total, err := t.performance.ListRanking(ctx, c, f)
	if err != nil {
		return t.errs(t.Spec().Name).result(err)
	}
	return JSONResult(NewList(items, total))
}
