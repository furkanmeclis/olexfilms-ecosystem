package ledger_test

import (
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
	"github.com/jackc/pgx/v5/pgtype"
)

func mustMeters(t *testing.T, n pgtype.Numeric) int64 {
	t.Helper()
	cm, err := ledger.NumericToCentimeters(n)
	if err != nil {
		t.Fatal(err)
	}
	return cm
}

// split runs one ledger.Split in its own committed transaction.
func (e *env) split(c ledger.SplitCommand) (ledger.SplitResult, error) {
	tx, err := e.pool.Begin(e.ctx)
	if err != nil {
		return ledger.SplitResult{}, err
	}
	defer func() { _ = tx.Rollback(e.ctx) }()
	res, err := e.l.Split(e.ctx, tx, c)
	if err != nil {
		return res, err
	}
	return res, tx.Commit(e.ctx)
}

// TEC-184 acceptance: a 50 m roll at the distributor is split 12 m -> the
// roll keeps 38 m, the new unit holds 12 m at the same owner, the product
// stock keeps 50 m; the same key writes nothing again; N > remaining,
// pieces and fixed barcodes are refused; cutting the rest uses the roll up;
// the replay and the rebuild check of the distributor find no drift.
func TestRollSplit(t *testing.T) {
	e := newEnv(t)
	u := e.unit(t, e.roll, "50")
	e.mustPost(t, mv(ledger.TypeEntry, u, nextRef(), ptr(e.cLoc)))
	tr := nextRef()
	e.mustPost(t, mv(ledger.TypeTransferOut, u, tr, orgOwner(e.dist)))
	e.mustPost(t, mv(ledger.TypeTransferIn, u, tr, ptr(e.dLoc)))

	key := "t184-" + e.suffix
	cmd := ledger.SplitCommand{SourceUnitID: u.ID, Centimeters: 1200, HolderOrgID: e.dist.ID, IdempotencyKey: key}
	res, err := e.split(cmd)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if res.Replayed || res.New.Barcode != u.Barcode+"-S1" || res.New.ProductID != e.roll.ID {
		t.Fatalf("split result = %+v", res)
	}
	src, srcUnit := e.state(t, u)
	if got := mustMeters(t, srcUnit.RemainingMeters); got != 3800 || src.Status != "available" || src.OwnerID != e.dLoc.ID {
		t.Fatalf("source = %d cm, %+v", got, src)
	}
	nst, nu := e.state(t, res.New)
	if mustMeters(t, nu.RemainingMeters) != 1200 || mustMeters(t, nu.InitialMeters) != 1200 ||
		nst.OwnerType != string(ledger.OwnerWarehouseLocation) || nst.OwnerID != e.dLoc.ID ||
		nst.HolderOrgID != e.dist.ID || nst.Status != "available" {
		t.Fatalf("new unit = %+v %+v", nst, nu)
	}
	if qty, meters := e.orgStock(t, e.dist.ID, e.roll); qty != 2 || meters != "50.00" {
		t.Fatalf("distributor roll stock = %d / %s, want 2 / 50.00", qty, meters)
	}

	// The same key returns the earlier split and writes nothing.
	again, err := e.split(cmd)
	if err != nil || !again.Replayed || again.New.ID != res.New.ID {
		t.Fatalf("replayed split = %+v %v", again, err)
	}
	if n := e.count(t, `SELECT count(*) FROM stock_movements WHERE type = 'split' AND reference_type = 'stock_split' AND reference_id = $1`, res.Split.ID); n != 2 {
		t.Fatalf("split movements = %d, want 2", n)
	}
	if n := e.count(t, `SELECT count(*) FROM stock_splits WHERE source_unit_id = $1`, u.ID); n != 1 {
		t.Fatalf("splits = %d, want 1", n)
	}
	if qty, meters := e.orgStock(t, e.dist.ID, e.roll); qty != 2 || meters != "50.00" {
		t.Fatalf("distributor roll stock after replay = %d / %s", qty, meters)
	}
	if _, err := e.split(ledger.SplitCommand{SourceUnitID: u.ID, Centimeters: 500, HolderOrgID: e.dist.ID, IdempotencyKey: key}); !errors.Is(err, ledger.ErrIdempotencyConflict) {
		t.Fatalf("key reuse for other meters: %v", err)
	}

	// Refusals.
	if _, err := e.split(ledger.SplitCommand{SourceUnitID: u.ID, Centimeters: 3900, HolderOrgID: e.dist.ID, IdempotencyKey: key + "-b"}); !errors.Is(err, ledger.ErrInsufficientMeters) {
		t.Fatalf("split above remaining: %v", err)
	}
	if _, err := e.split(ledger.SplitCommand{SourceUnitID: u.ID, Centimeters: 100, HolderOrgID: e.center.ID, IdempotencyKey: key + "-c"}); !errors.Is(err, ledger.ErrSplitUnitElsewhere) {
		t.Fatalf("split by a non-holder: %v", err)
	}
	piece := e.unit(t, e.piece, "")
	e.mustPost(t, mv(ledger.TypeEntry, piece, nextRef(), ptr(e.cLoc)))
	fixed := e.unit(t, e.fixed, "")
	fe := mv(ledger.TypeEntry, fixed, nextRef(), ptr(e.cLoc))
	fe.Quantity = 5
	e.mustPost(t, fe)
	for _, x := range []int64{piece.ID, fixed.ID} {
		if _, err := e.split(ledger.SplitCommand{SourceUnitID: x, Centimeters: 100, HolderOrgID: e.center.ID, IdempotencyKey: key + "-d"}); !errors.Is(err, ledger.ErrSplitNotRoll) {
			t.Fatalf("split of unit %d: %v", x, err)
		}
	}
	// Post refuses a split movement: only Ledger.Split writes it.
	direct := mv(ledger.TypeSplit, u, nextRef(), nil)
	direct.Centimeters = 100
	e.mustFail(t, direct, ledger.ErrInvalidMovement)

	// Cutting the rest uses the roll up; the stock still adds up to 50 m.
	rest, err := e.split(ledger.SplitCommand{SourceUnitID: u.ID, Centimeters: 3800, HolderOrgID: e.dist.ID, IdempotencyKey: key + "-e"})
	if err != nil || rest.New.Barcode != u.Barcode+"-S2" {
		t.Fatalf("split the rest: %+v %v", rest, err)
	}
	src, srcUnit = e.state(t, u)
	if src.Status != "used" || src.OwnerType != string(ledger.OwnerTrash) || src.OwnerID != e.dist.ID ||
		mustMeters(t, srcUnit.RemainingMeters) != 0 {
		t.Fatalf("used-up roll = %+v", src)
	}
	if qty, meters := e.orgStock(t, e.dist.ID, e.roll); qty != 2 || meters != "50.00" {
		t.Fatalf("distributor roll stock after the last cut = %d / %s, want 2 / 50.00", qty, meters)
	}

	// Replay: no anomaly, the projection equals the stored state and the
	// meters of each split match.
	ids := []int64{u.ID, res.New.ID, rest.New.ID}
	units, err := e.q.ListUnitsByIDs(e.ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	mvs, err := e.q.ListStockMovementsByUnitIDs(e.ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	p := ledger.NewProjection()
	for _, unit := range units {
		var own []int
		for i := range mvs {
			if mvs[i].UnitID == unit.ID {
				own = append(own, i)
			}
		}
		list := make([]db.StockMovement, 0, len(own))
		for _, i := range own {
			list = append(list, mvs[i])
		}
		up, err := p.Replay(unit, list)
		if err != nil {
			t.Fatal(err)
		}
		if len(up.Anomalies) != 0 {
			t.Fatalf("unit %s anomalies: %v", unit.Barcode, up.Anomalies)
		}
		st, _ := e.state(t, unit)
		if up.State == nil || string(up.State.Status) != st.Status || up.State.Owner.ID != st.OwnerID ||
			up.State.Owner.OrgID != st.HolderOrgID || up.State.LastMovementID != st.LastMovementID.Int64 {
			t.Fatalf("unit %s replayed state %+v, stored %+v", unit.Barcode, up.State, st)
		}
		if up.RemainingCm == nil || *up.RemainingCm != mustMeters(t, unit.RemainingMeters) {
			t.Fatalf("unit %s replayed remaining %v", unit.Barcode, up.RemainingCm)
		}
	}
	if a := p.SplitAnomalies(); len(a) != 0 {
		t.Fatalf("split anomalies: %v", a)
	}
	got := p.OrgStocks[ledger.OrgStockKey{OrganizationID: e.dist.ID, ProductID: e.roll.ID}]
	if got.Quantity != 2 || got.Centimeters != 5000 {
		t.Fatalf("replayed distributor stock = %+v, want 2 / 5000 cm", got)
	}

	// Rebuild check of the distributor (a fresh organization): no drift.
	rep, err := rebuild.New(e.pool, e.q).Check(e.ctx, e.dist.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.DiffCount != 0 || len(rep.Anomalies) != 0 {
		t.Fatalf("rebuild check: diffs %+v anomalies %v", rep.Diffs, rep.Anomalies)
	}
}
