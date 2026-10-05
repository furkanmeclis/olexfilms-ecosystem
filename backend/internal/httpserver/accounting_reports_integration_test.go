package httpserver

import (
	"net/http"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	accountingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
)

// TEC-346 (F3-07f) over HTTP: the dealer reads its own reports and queues
// their exports; the distributor reads its own P&L but never the dealer's
// (404); dealer staff without accounting.read gets 403.
func TestIntegrationAccountingReportsOwnBookOnly(t *testing.T) {
	mr := miniredis.RunT(t)
	qc := queue.NewClient(config.RedisConfig{Addr: mr.Addr()})
	t.Cleanup(func() { _ = qc.Close() })
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = storage.NewMemory(); d.Queue = qc })

	center := it.brandCenter("olex")
	dist := it.org("t346-dist", "distributor", center)
	dealer := it.org("t346-dealer", "dealer", dist)
	dOwner, dpw := it.user("t346-dealer-owner")
	it.member(dealer, dOwner, "owner")
	dStaff, spw := it.user("t346-dealer-staff")
	it.member(dealer, dStaff, "staff")
	distOwner, xpw := it.user("t346-dist-owner")
	it.member(dist, distOwner, "owner")
	dealerTok := it.loginOrg(dOwner, dpw, dealer)
	staffTok := it.loginOrg(dStaff, spw, dealer)
	distTok := it.loginOrg(distOwner, xpw, dist)

	type pnl struct {
		Organization struct {
			UUID string `json:"uuid"`
		} `json:"organization"`
		Group  string `json:"group"`
		Totals struct {
			Net string `json:"net"`
		} `json:"totals"`
	}
	own := decodeData[pnl](t, it.accDo("GET", "/v1/accounting/reports/pnl?group=category&from=2026-01-01", dealerTok, nil, http.StatusOK))
	if own.Organization.UUID != dealer.Uuid.String() || own.Group != "category" || own.Totals.Net != "0.00" {
		t.Fatalf("dealer pnl = %+v", own)
	}
	it.accDo("GET", "/v1/accounting/reports/pnl?organization_uuid="+dealer.Uuid.String(), dealerTok, nil, http.StatusOK)
	it.accDo("GET", "/v1/accounting/reports/pnl?group=week", dealerTok, nil, http.StatusBadRequest)
	it.accDo("GET", "/v1/accounting/reports/pnl?from=2026-02-01&to=2026-01-01", dealerTok, nil, http.StatusBadRequest)
	type margin struct {
		CostVisible bool `json:"cost_visible"`
		Total       struct {
			Cost *string `json:"cost"`
		} `json:"total"`
	}
	m := decodeData[margin](t, it.accDo("GET", "/v1/accounting/reports/margin", dealerTok, nil, http.StatusOK))
	if !m.CostVisible || m.Total.Cost == nil {
		t.Fatalf("dealer owner margin (pricing.purchase.read) = %+v", m)
	}
	it.accDo("GET", "/v1/accounting/reports/cari-aging?as_of=2026-10-01", dealerTok, nil, http.StatusOK)
	it.accDo("GET", "/v1/accounting/reports/staff-cost", dealerTok, nil, http.StatusOK)

	// The distributor: own P&L yes, the dealer's book no (no subtree read).
	it.accDo("GET", "/v1/accounting/reports/pnl", distTok, nil, http.StatusOK)
	for _, path := range []string{"pnl", "margin", "cari-aging", "staff-cost"} {
		it.accDo("GET", "/v1/accounting/reports/"+path+"?organization_uuid="+dealer.Uuid.String(), distTok, nil, http.StatusNotFound)
		it.accDo("POST", "/v1/accounting/reports/"+path+"/export", distTok, map[string]any{
			"format": "xlsx", "organization_uuid": dealer.Uuid.String(),
		}, http.StatusNotFound)
	}
	// Dealer staff holds no accounting.read.
	if code, env := it.do("GET", "/v1/accounting/reports/pnl", hostOlex, staffTok, nil); code != http.StatusForbidden {
		t.Fatalf("dealer staff pnl = %d %s, want 403", code, errCode(env))
	}

	// Exports land on the exports queue and read back through the
	// accounting export route.
	it.accDo("POST", "/v1/accounting/reports/pnl/export", dealerTok, map[string]any{"format": "xlsx", "group": "week"}, http.StatusBadRequest)
	want := map[string]string{
		"pnl": accountingusecase.ResourcePnl, "margin": accountingusecase.ResourceMargin,
		"cari-aging": accountingusecase.ResourceCariAging, "staff-cost": accountingusecase.ResourceStaffCost,
	}
	for path, resource := range want {
		job := decodeData[exportJob](t, it.accDo("POST", "/v1/accounting/reports/"+path+"/export", dealerTok, map[string]any{
			"format": "xlsx",
		}, http.StatusAccepted))
		if job.Resource != resource {
			t.Fatalf("%s export job = %+v", path, job)
		}
		got := decodeData[exportJob](t, it.accDo("GET", "/v1/accounting/exports/"+job.UUID, dealerTok, nil, http.StatusOK))
		if got.UUID != job.UUID {
			t.Fatalf("%s job read = %+v", path, got)
		}
	}
	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = insp.Close() })
	if tasks, err := insp.ListPendingTasks(queue.QueueExports); err != nil || len(tasks) != len(want) {
		t.Fatalf("pending export tasks = %d (%v)", len(tasks), err)
	}
}
