package usecase

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
)

func strp(s string) *string { return &s }

func TestBonusRulesCRUDValidationAndSettings(t *testing.T) {
	f := newPerfFixture(t)
	owner := f.member(t, f.dealer1, "rule-owner")
	svc := New(f.tx, f.q, outbox.NewMemory(), featureMap{})
	c := callerFor(f.dealer1, owner)

	bad := []BonusRuleInput{
		{Name: "", Metric: model.MetricServicesCount, ThresholdPct: "100", Kind: model.BonusFixed, Amount: strp("500"), Currency: strp("TRY")},
		{Name: "x", Metric: "order_volume", ThresholdPct: "100", Kind: model.BonusFixed, Amount: strp("500"), Currency: strp("TRY")},
		{Name: "x", Metric: model.MetricServicesCount, ThresholdPct: "1001", Kind: model.BonusFixed, Amount: strp("500"), Currency: strp("TRY")},
		{Name: "x", Metric: model.MetricServicesCount, ThresholdPct: "100", Kind: model.BonusFixed, Amount: strp("500")},
		{Name: "x", Metric: model.MetricServiceRevenue, ThresholdPct: "100", Kind: model.BonusPercentOfRevenue, Percent: strp("101")},
		{Name: "x", Metric: model.MetricServiceRevenue, ThresholdPct: "100", Kind: "other"},
	}
	for i, in := range bad {
		var ve *ValidationError
		if _, err := svc.CreateBonusRule(f.ctx, c, in); !errors.As(err, &ve) {
			t.Fatalf("bad input %d error = %v, want validation", i, err)
		}
	}

	created, err := svc.CreateBonusRule(f.ctx, c, BonusRuleInput{
		Name: " Hedef %100 ", Metric: model.MetricServicesCount, ThresholdPct: "100", Kind: model.BonusFixed,
		Amount: strp("500"), Currency: strp("try"), Active: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Name != "Hedef %100" || created.Amount == nil || *created.Amount != "500.00" || created.Currency == nil || *created.Currency != "TRY" || created.Percent != nil {
		t.Fatalf("created = %+v", created)
	}
	updated, err := svc.UpdateBonusRule(f.ctx, c, created.UUID, BonusRuleInput{
		Name: "Gelir %5", Metric: model.MetricServiceRevenue, ThresholdPct: "120", Kind: model.BonusPercentOfRevenue,
		Percent: strp("5"), Active: true,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Kind != model.BonusPercentOfRevenue || updated.Amount != nil || updated.Currency != nil || updated.Percent == nil {
		t.Fatalf("updated = %+v", updated)
	}
	rules, err := svc.ListBonusRules(f.ctx, c, nil)
	if err != nil || len(rules) != 1 || rules[0].UUID != created.UUID {
		t.Fatalf("list = %+v, %v", rules, err)
	}
	// Another dealer does not see or reach the rule.
	other := callerFor(f.dealer2, owner)
	if _, err := svc.UpdateBonusRule(f.ctx, other, created.UUID, BonusRuleInput{
		Name: "x", Metric: model.MetricServicesCount, ThresholdPct: "100", Kind: model.BonusFixed, Amount: strp("1"), Currency: strp("TRY"),
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other dealer update error = %v, want ErrNotFound", err)
	}
	if err := svc.DeleteBonusRule(f.ctx, c, created.UUID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if rules, _ := svc.ListBonusRules(f.ctx, c, nil); len(rules) != 0 {
		t.Fatalf("after delete = %+v", rules)
	}

	// Only dealers with dealer_accounting have bonus rules.
	if _, err := svc.ListBonusRules(f.ctx, callerFor(f.dist, owner), nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("distributor list error = %v, want ErrForbidden", err)
	}
	off := New(f.tx, f.q, outbox.NewMemory(), featureMap{features.ModuleDealerAccounting: false})
	if _, err := off.ListBonusRules(f.ctx, c, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("dealer_accounting off error = %v, want ErrForbidden", err)
	}

	settings, err := svc.GetBonusSettings(f.ctx, c)
	if err != nil || settings.PayoutDay != DefaultBonusPayoutDay {
		t.Fatalf("default settings = %+v, %v", settings, err)
	}
	var ve *ValidationError
	if _, err := svc.UpdateBonusSettings(f.ctx, c, BonusSettingsInput{PayoutDay: 29}); !errors.As(err, &ve) {
		t.Fatalf("payout day 29 error = %v, want validation", err)
	}
	if settings, err = svc.UpdateBonusSettings(f.ctx, c, BonusSettingsInput{PayoutDay: 10}); err != nil || settings.PayoutDay != 10 {
		t.Fatalf("update settings = %+v, %v", settings, err)
	}
	if settings, err = svc.GetBonusSettings(f.ctx, c); err != nil || settings.PayoutDay != 10 {
		t.Fatalf("read settings = %+v, %v", settings, err)
	}
}

func TestBonusBulkApproveAndRuleWithAccrualIsDeactivated(t *testing.T) {
	f := newPerfFixture(t)
	staffUser := f.member(t, f.dealer1, "bulk-staff")
	f.staffProfile(t, f.dealer1, staffUser, "Toplu Usta")
	period := "2026-10"
	if _, err := f.q.UpsertStaffTarget(f.ctx, db.UpsertStaffTargetParams{
		OrganizationID: f.dealer1.ID, BrandID: f.dealer1.BrandID, UserID: staffUser, Period: period,
		Metric: model.MetricServicesCount, Value: mustNum(t, "1"),
	}); err != nil {
		t.Fatalf("target: %v", err)
	}
	svc := New(f.tx, f.q, outbox.NewMemory(), featureMap{}, posting.New(f.q, nil, nil)).
		WithClock(func() time.Time { return time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC) })
	c := callerFor(f.dealer1, staffUser)
	rule, err := svc.CreateBonusRule(f.ctx, c, BonusRuleInput{
		Name: "Bir hizmet", Metric: model.MetricServicesCount, ThresholdPct: "100", Kind: model.BonusFixed,
		Amount: strp("300"), Currency: strp("TRY"), Active: true,
	})
	if err != nil {
		t.Fatalf("rule: %v", err)
	}
	f.service(t, f.dealer1, time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC), false, 0, false, staffUser)
	if res, err := svc.CalculateBonuses(f.ctx, f.dealer1.ID, period); err != nil || res.Accrued != 1 {
		t.Fatalf("calculate = %+v, %v", res, err)
	}

	adapter := NewBonusBulkAdapter(svc)
	query := map[string]string{
		"organization_uuid":         f.dealer1.Uuid.String(),
		bulkengine.QueryActorUserID: strconv.FormatInt(staffUser, 10),
		"period":                    period,
	}
	ids, err := adapter.ResolveTargets(f.ctx, BulkApproveBonus, bulkengine.BulkTarget{Scope: "query", Query: query})
	if err != nil || len(ids) != 1 {
		t.Fatalf("resolve = %v, %v", ids, err)
	}
	// A distributor run resolves nothing.
	distQuery := map[string]string{"organization_uuid": f.dist.Uuid.String(), bulkengine.QueryActorUserID: "1"}
	if _, err := adapter.ResolveTargets(f.ctx, BulkApproveBonus, bulkengine.BulkTarget{Scope: "query", Query: distQuery}); err == nil {
		t.Fatal("distributor resolve succeeded")
	}
	ctx := bulkengine.WithRun(f.ctx, bulkengine.Run{Query: query})
	res, err := adapter.ApplyItem(ctx, BulkApproveBonus, ids[0])
	if err != nil || !res.OK {
		t.Fatalf("apply = %+v, %v", res, err)
	}
	assertBonusPayment(t, f, f.dealer1.ID, "planned", "2026-11-05", 0)
	again, err := adapter.ApplyItem(ctx, BulkApproveBonus, ids[0])
	if err != nil || again.OK {
		t.Fatalf("second apply = %+v, %v, want item failure", again, err)
	}
	if ids, err := adapter.ResolveTargets(f.ctx, BulkApproveBonus, bulkengine.BulkTarget{Scope: "query", Query: query}); err != nil || len(ids) != 0 {
		t.Fatalf("resolve after approval = %v, %v", ids, err)
	}

	// The rule has an accrual: delete keeps it, inactive.
	if err := svc.DeleteBonusRule(f.ctx, c, rule.UUID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rules, err := svc.ListBonusRules(f.ctx, c, nil)
	if err != nil || len(rules) != 1 || rules[0].Active {
		t.Fatalf("rules after delete = %+v, %v", rules, err)
	}
}

func TestTargetListCarriesTargetOrganizationUUID(t *testing.T) {
	f := newPerfFixture(t)
	owner := f.member(t, f.dist, "target-owner")
	svc := New(f.tx, f.q, outbox.NewMemory(), featureMap{})
	c := callerFor(f.dist, owner)
	created, err := svc.CreateTarget(f.ctx, c, TargetInput{
		TargetOrganizationUUID: f.dealer1.Uuid, Metric: model.MetricServicesCount, PeriodKind: model.PeriodMonthly,
		PeriodStart: "2026-10-01", Value: "40",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.TargetOrganizationUUID != f.dealer1.Uuid {
		t.Fatalf("created target org = %s, want %s", created.TargetOrganizationUUID, f.dealer1.Uuid)
	}
	items, _, err := svc.ListTargets(f.ctx, c, TargetFilter{Limit: 10})
	if err != nil || len(items) != 1 || items[0].TargetOrganizationUUID != f.dealer1.Uuid {
		t.Fatalf("list = %+v, %v", items, err)
	}
}
