package rebuild_test

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/credits"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
)

// TEC-1229: the rebuild check compares the unit credit balances with the
// sum of their credit movements and the repair restores them.
func TestRebuildCreditBalances(t *testing.T) {
	e := newEnv(t)
	piece, _, _ := e.chain(t)
	if _, err := credits.Increase(e.ctx, e.q, piece, credits.Actor{OrgID: e.dist.ID}, 3, "t1229"); err != nil {
		t.Fatal(err)
	}
	rep, err := e.svc.Check(e.ctx, e.dist.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hasDiff(rep, rebuild.TableCreditBalances, "credits_total", piece.ID) {
		t.Fatalf("clean credits reported: %+v", rep.Diffs)
	}
	e.exec(t, `UPDATE unit_credit_balances SET credits_total = credits_total + 4, credits_remaining = credits_remaining + 1 WHERE unit_id = $1`, piece.ID)
	rep, err = e.svc.Check(e.ctx, e.dist.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDiff(rep, rebuild.TableCreditBalances, "credits_total", piece.ID) || !hasDiff(rep, rebuild.TableCreditBalances, "credits_remaining", piece.ID) {
		t.Fatalf("credit drift not reported: %+v", rep.Diffs)
	}
	if _, err := e.svc.Run(e.ctx, rebuild.Options{OrganizationID: e.dist.ID, Apply: true, Source: "t1229"}); err != nil {
		t.Fatal(err)
	}
	rep, err = e.svc.Check(e.ctx, e.dist.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hasDiff(rep, rebuild.TableCreditBalances, "credits_total", piece.ID) {
		t.Fatalf("credit drift after repair: %+v", rep.Diffs)
	}
}
