package httpserver

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	pricingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/jackc/pgx/v5/pgtype"
)

type recommendedRefResp struct {
	Price       string `json:"price"`
	Currency    string `json:"currency"`
	CountryISO2 string `json:"country_iso2"`
	Scope       string `json:"scope"`
}

// TEC-506 acceptance (HTTP): publication needs the center, the permission
// and a step-up; the history and the price in force follow the list
// contract; the dealer's price screens carry the recommended block; the
// discipline reads follow the pricing.discipline.read scope.
func TestIntegrationRecommendedPrices(t *testing.T) {
	mr := miniredis.RunT(t)
	qc := queue.NewClient(config.RedisConfig{Addr: mr.Addr()})
	t.Cleanup(func() { _ = qc.Close() })
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = storage.NewMemory(); d.Queue = qc })
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t506-dist", "distributor", center)
	dealer := it.org("t506-dealer", "dealer", dist)

	acc, apw := it.user("t506-acc")
	it.member(center, acc, "staff", rbac.RoleCenterAccounting)
	staff, spw := it.user("t506-staff")
	it.member(center, staff, "staff", rbac.RoleCenterStaff)
	distOwner, dpw := it.user("t506-dist-owner")
	it.member(dist, distOwner, "owner")
	dealerOwner, rpw := it.user("t506-dealer-owner")
	it.member(dealer, dealerOwner, "owner")

	p := it.product(center, "T506")
	pid := p.Uuid.String()
	accTok := it.loginOrg(acc, apw, center)
	staffTok := it.loginOrg(staff, spw, center)
	distTok := it.loginOrg(distOwner, dpw, dist)
	dealerTok := it.loginOrg(dealerOwner, rpw, dealer)
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM dealer_product_prices WHERE product_id = $1", p.ID)
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM price_discipline_snapshots WHERE product_id = $1", p.ID)
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM product_prices WHERE product_id = $1", p.ID)
	})

	today := pricingusecase.LocalDay(time.Now(), center.Timezone)
	body := map[string]any{"rows": []map[string]any{{"product_uuid": pid, "currency": "TRY", "price": "1000"}}}
	if code, env := it.do("POST", "/v1/tenant/pricing/recommended/publish", hostOlex, accTok, body); code != http.StatusForbidden || errCode(env) != "STEP_UP_REQUIRED" {
		t.Fatalf("publish without step-up = %d %s", code, errCode(env))
	}
	if code, _ := it.do("POST", "/v1/tenant/pricing/recommended/publish", hostOlex, distTok, body); code != http.StatusForbidden {
		t.Fatalf("distributor publish = %d, want 403", code)
	}
	it.stepUp(acc.Uuid)
	past := map[string]any{"effective_from": today.AddDate(0, 0, -1).Format(time.DateOnly), "rows": body["rows"]}
	if code, env := it.do("POST", "/v1/tenant/pricing/recommended/publish", hostOlex, accTok, past); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("past publish = %d %s", code, errCode(env))
	}
	res := decodeData[struct {
		BatchID      string `json:"batch_id"`
		PriceCount   int    `json:"price_count"`
		AppliedCount int    `json:"applied_count"`
	}](t, mustDo(t, it, "POST", "/v1/tenant/pricing/recommended/publish", accTok, body, http.StatusCreated))
	if res.PriceCount != 1 || res.AppliedCount != 1 || res.BatchID == "" {
		t.Fatalf("publish = %+v", res)
	}
	future := map[string]any{"effective_from": today.AddDate(0, 0, 7).Format(time.DateOnly),
		"rows": []map[string]any{{"product_uuid": pid, "currency": "TRY", "price": "1100.50"}}}
	mustDo(t, it, "POST", "/v1/tenant/pricing/recommended/publish", accTok, future, http.StatusCreated)

	// History: list contract (filter, sort, validation); center only.
	type versionResp struct {
		Price         string `json:"price"`
		EffectiveFrom string `json:"effective_from"`
		Status        string `json:"status"`
	}
	hist := decodeData[listPage[versionResp]](t, mustDo(t, it, "GET", "/v1/tenant/pricing/recommended/versions?product="+pid+"&sort=effective_from", staffTok, nil, http.StatusOK))
	if hist.Total != 2 || hist.Items[0].Price != "1000.00" || hist.Items[0].Status != "current" || hist.Items[1].Status != "scheduled" {
		t.Fatalf("history = %+v", hist)
	}
	if code, env := it.do("GET", "/v1/tenant/pricing/recommended/versions?sort=bogus", hostOlex, staffTok, nil); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("bad sort = %d %s", code, errCode(env))
	}
	if code, _ := it.do("GET", "/v1/tenant/pricing/recommended/versions", hostOlex, distTok, nil); code != http.StatusForbidden {
		t.Fatalf("distributor history = %d", code)
	}
	cur := decodeData[listPage[struct {
		ProductUUID string `json:"product_uuid"`
		Price       string `json:"price"`
		Scope       string `json:"scope"`
	}]](t, mustDo(t, it, "GET", "/v1/tenant/pricing/recommended/current?currency=TRY&product="+pid, dealerTok, nil, http.StatusOK))
	if cur.Total != 1 || cur.Items[0].Price != "1000.00" || cur.Items[0].Scope != "currency" {
		t.Fatalf("current = %+v", cur)
	}

	// The dealer's price screens carry the block and the deviation.
	mustDo(t, it, "PUT", "/v1/dealer-prices", dealerTok, map[string]any{"product_uuid": pid, "sale_price": "1150"}, http.StatusOK)
	prices := decodeData[struct {
		Items []struct {
			ProductUUID  string              `json:"product_uuid"`
			Recommended  *recommendedRefResp `json:"recommended"`
			DeviationPct *string             `json:"deviation_pct"`
		} `json:"items"`
	}](t, mustDo(t, it, "GET", "/v1/dealer-prices", dealerTok, nil, http.StatusOK)).Items
	if len(prices) != 1 || prices[0].Recommended == nil || prices[0].Recommended.Price != "1000.00" || strv(prices[0].DeviationPct) != "15.00" {
		t.Fatalf("dealer prices = %+v", prices)
	}
	_, raw := it.priceView(dealerTok, pid)
	if !strings.Contains(raw, `"recommended":{"price":"1000.00"`) || !strings.Contains(raw, `"deviation_pct":"15.00"`) {
		t.Fatalf("dealer tenant price view = %s", raw)
	}

	// Discipline: snapshot of today, center (brand) and distributor (subtree).
	if _, err := it.q.RefreshPriceDisciplineSnapshots(ctx, db.RefreshPriceDisciplineSnapshotsParams{
		SnapshotDate: pgtype.Date{Time: today, Valid: true}, BrandID: center.BrandID,
		SalesFrom: pgtype.Timestamptz{Time: time.Now().AddDate(0, 0, -30), Valid: true},
		SalesTo:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	day := today.Format(time.DateOnly)
	type discResp struct {
		OrganizationUUID string  `json:"organization_uuid"`
		DeviationPct     *string `json:"deviation_pct"`
		OverThreshold    bool    `json:"over_threshold"`
	}
	path := "/v1/pricing/discipline?date=" + day + "&q=" + url.QueryEscape(dealer.Name) + "&sort=-deviation_pct"
	for _, tok := range []string{staffTok, distTok} {
		got := decodeData[listPage[discResp]](t, mustDo(t, it, "GET", path, tok, nil, http.StatusOK))
		if got.Total != 1 || got.Items[0].OrganizationUUID != dealer.Uuid.String() || strv(got.Items[0].DeviationPct) != "15.00" || !got.Items[0].OverThreshold {
			t.Fatalf("discipline = %+v", got)
		}
	}
	if code, _ := it.do("GET", path, hostOlex, dealerTok, nil); code != http.StatusForbidden {
		t.Fatalf("dealer discipline = %d, want 403", code)
	}
	if code, env := it.do("GET", "/v1/pricing/discipline?sort=nope", hostOlex, staffTok, nil); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("bad discipline sort = %d %s", code, errCode(env))
	}
	sum := decodeData[struct {
		SnapshotDate *string `json:"snapshot_date"`
		ThresholdPct int     `json:"threshold_pct"`
		Countries    []struct {
			Currency              string `json:"currency"`
			OverThresholdOrgCount int64  `json:"over_threshold_org_count"`
		} `json:"countries"`
	}](t, mustDo(t, it, "GET", "/v1/pricing/discipline/summary?date="+day, distTok, nil, http.StatusOK))
	if strv(sum.SnapshotDate) != day || sum.ThresholdPct != 15 || len(sum.Countries) != 1 || sum.Countries[0].OverThresholdOrgCount != 1 {
		t.Fatalf("distributor summary = %+v", sum)
	}
	job := decodeData[struct {
		Resource string `json:"resource"`
	}](t, mustDo(t, it, "POST", "/v1/pricing/discipline/export", staffTok, map[string]any{"format": "csv", "query": map[string]string{"date": day}}, http.StatusAccepted))
	if job.Resource != pricingusecase.ResourceDisciplineExport {
		t.Fatalf("export job = %+v", job)
	}
	if code, env := it.do("POST", "/v1/pricing/discipline/export", hostOlex, staffTok, map[string]any{"format": "doc"}); code != http.StatusBadRequest {
		t.Fatalf("bad export format = %d %s", code, errCode(env))
	}
}
