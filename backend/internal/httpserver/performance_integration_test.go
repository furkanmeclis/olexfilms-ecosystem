package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	perfuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
)

func TestIntegrationPerformanceAPIAndRules(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	distA := it.org("t492-dist-a", "distributor", center)
	distB := it.org("t492-dist-b", "distributor", center)
	dealerA := it.org("t492-dealer-a", "dealer", distA)
	dealerB := it.org("t492-dealer-b", "dealer", distB)
	for _, org := range []db.Organization{center, distA, distB, dealerA, dealerB} {
		if _, err := it.srv.features.SetByAdmin(ctx, 0, org.ID, features.ModulePerformance, true); err != nil {
			t.Fatal(err)
		}
	}
	centerOwner, cpw := it.user("t492-center-owner")
	it.member(center, centerOwner, "owner")
	centerTok := it.loginOrg(centerOwner, cpw, center)
	distOwner, dpw := it.user("t492-dist-owner")
	it.member(distA, distOwner, "owner")
	distTok := it.loginOrg(distOwner, dpw, distA)
	dealerOwner, opw := it.user("t492-dealer-owner")
	it.member(dealerA, dealerOwner, "owner")
	dealerTok := it.loginOrg(dealerOwner, opw, dealerA)
	otherOwner, _ := it.user("t492-other-owner")
	it.member(distB, otherOwner, "owner")

	seedPerformanceMetric(t, it.q, dealerA, "2026-10", "services_count", "2.00")
	seedPerformanceMetric(t, it.q, dealerB, "2026-10", "services_count", "9.00")

	code, env := it.do("POST", "/v1/performance/targets", hostOlex, distTok, map[string]any{
		"target_organization_uuid": dealerB.Uuid.String(), "metric": "services_count",
		"period_kind": "monthly", "period_start": "2026-10-01", "value": "10",
	})
	if code != http.StatusNotFound {
		t.Fatalf("foreign target = %d %s, want 404", code, errCode(env))
	}
	code, env = it.do("POST", "/v1/performance/targets", hostOlex, centerTok, map[string]any{
		"target_organization_uuid": dealerA.Uuid.String(), "metric": "services_count",
		"period_kind": "monthly", "period_start": "2026-10-01", "value": "4",
	})
	if code != http.StatusCreated {
		t.Fatalf("center target = %d %s", code, errCode(env))
	}
	var target struct {
		AchievementPct *string `json:"achievement_pct"`
	}
	if err := json.Unmarshal(env.Data, &target); err != nil {
		t.Fatal(err)
	}
	if target.AchievementPct == nil || *target.AchievementPct != "50.00" {
		t.Fatalf("achievement = %v, want 50.00", target.AchievementPct)
	}

	code, env = it.do("GET", "/v1/performance/ranking?period=2026-10&sort=services_count", hostOlex, dealerTok, nil)
	if code != http.StatusForbidden {
		t.Fatalf("dealer ranking = %d %s, want 403", code, errCode(env))
	}
	code, env = it.do("GET", "/v1/performance/benchmark?period=2026-10", hostOlex, dealerTok, nil)
	if code != http.StatusOK {
		t.Fatalf("benchmark = %d %s", code, errCode(env))
	}
	if string(env.Data) == "" || json.Valid(env.Data) == false {
		t.Fatalf("benchmark payload invalid: %s", string(env.Data))
	}
	code, env = it.do("GET", "/v1/performance/ranking?period=2026-10&sort=bogus", hostOlex, centerTok, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("unknown sort = %d %s, want 400", code, errCode(env))
	}

	svc := perfuc.New(it.pool, it.q, outbox.NewStore(it.pool, it.q))
	centerRule, err := it.q.CreateWeakDealerRule(ctx, db.CreateWeakDealerRuleParams{
		OrganizationID: center.ID, BrandID: center.BrandID, Name: "center weak", Metric: "services_count",
		Operator: "lt", Threshold: scanNum("5"), CreateTask: true, Notify: true, Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	distRule, err := it.q.CreateWeakDealerRule(ctx, db.CreateWeakDealerRuleParams{
		OrganizationID: distA.ID, BrandID: center.BrandID, Name: "dist weak", Metric: "services_count",
		Operator: "lt", Threshold: scanNum("5"), CreateTask: false, Notify: true, Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunRules(ctx, center.BrandID, []int64{center.ID, distA.ID}, "2026-10"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunRules(ctx, center.BrandID, []int64{center.ID, distA.ID}, "2026-10"); err != nil {
		t.Fatal(err)
	}
	var centerTasks, distTasks int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE auto_rule_id = $1`, centerRule.ID).Scan(&centerTasks); err != nil {
		t.Fatal(err)
	}
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE auto_rule_id = $1`, distRule.ID).Scan(&distTasks); err != nil {
		t.Fatal(err)
	}
	if centerTasks != 1 || distTasks != 0 {
		t.Fatalf("auto tasks center=%d dist=%d, want 1/0", centerTasks, distTasks)
	}
	var notifications int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_name IN ('performance.weak_dealer','performance.below_target')`).Scan(&notifications); err != nil {
		t.Fatal(err)
	}
	if notifications == 0 {
		t.Fatal("performance notifications were not enqueued")
	}

	if _, err := it.srv.features.SetByAdmin(ctx, 0, dealerA.ID, features.ModuleDealerAccounting, false); err != nil {
		t.Fatal(err)
	}
	code, env = it.do("GET", "/v1/performance/bonuses?period=2026-10", hostOlex, dealerTok, nil)
	if code != http.StatusForbidden {
		t.Fatalf("dealer_accounting off bonuses = %d %s, want 403", code, errCode(env))
	}

	if _, err := it.srv.features.SetByAdmin(ctx, 0, dealerA.ID, features.ModulePerformance, false); err != nil {
		t.Fatal(err)
	}
	code, env = it.do("GET", "/v1/performance/dashboard?period=2026-10", hostOlex, dealerTok, nil)
	if code != http.StatusForbidden {
		t.Fatalf("feature off dashboard = %d %s, want 403", code, errCode(env))
	}
}

func seedPerformanceMetric(t *testing.T, q *db.Queries, org db.Organization, period, metric, value string) {
	t.Helper()
	if _, err := q.UpsertPerformanceMetric(context.Background(), db.UpsertPerformanceMetricParams{
		OrganizationID: org.ID, BrandID: org.BrandID, Period: period, Scope: "org", Metric: metric,
		Value: scanNum(value),
	}); err != nil {
		t.Fatal(err)
	}
}
