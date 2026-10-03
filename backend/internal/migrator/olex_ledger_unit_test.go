package migrator

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
)

const (
	tCenter = int64(10)
	tDealer = int64(20)
	tBin    = int64(30)
)

func tOrg(id int64) ledger.Owner {
	return ledger.Owner{Type: ledger.OwnerOrganization, ID: id, OrgID: id}
}

func tLoc() ledger.Owner {
	return ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: tBin, OrgID: tCenter}
}

func tUnit(kind string) db.Unit {
	return db.Unit{ID: 7, Uuid: uuid.New(), OrganizationID: tCenter, BrandID: 1, ProductID: 3, Barcode: "B-1",
		UnitKind: kind, Status: "available", CreatedAt: pgtype.Timestamptz{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true}}
}

func tMove(t *testing.T, id string, mv legacyMove, ok bool, at time.Time) legacyMove {
	t.Helper()
	if !ok {
		t.Fatalf("move %s not known", id)
	}
	mv.Key = Key{System: "hub", Table: "stock_movements", ID: id, TargetTable: "stock_movements"}
	mv.At = at
	return mv
}

func hubT(t *testing.T, id, action string, dealer int64, day int) legacyMove {
	mv, ok := hubMove(action, dealer, tCenter)
	return tMove(t, id, mv, ok, time.Date(2025, 3, day, 9, 0, 0, 0, time.UTC))
}

// replayPlan replays the planned records like the rebuild does.
func replayPlan(t *testing.T, u db.Unit, plan unitPlan) *ledger.UnitProjection {
	t.Helper()
	var mvs []db.StockMovement
	for i, r := range plan.Records {
		mvs = append(mvs, recordedMovement(u, r, int64(i+1)))
	}
	up, err := ledger.NewProjection().Replay(u, mvs)
	if err != nil {
		t.Fatal(err)
	}
	return up
}

func TestHubAndWHMoveTranslation(t *testing.T) {
	cases := []struct {
		name   string
		mv     legacyMove
		ok     bool
		typ    ledger.MovementType
		intent ownerIntent
		to     ledger.Owner
		status ledger.Status
	}{
		{"imported", must2(hubMove("imported", 0, tCenter)), true, ledger.TypeEntry, intentTo, tOrg(tCenter), ledger.StatusAvailable},
		{"received", must2(hubMove("Received", 0, tCenter)), true, ledger.TypeReceived, intentTo, tOrg(tCenter), ledger.StatusAvailable},
		{"to dealer", must2(hubMove("transferred_to_dealer", tDealer, tCenter)), true, ledger.TypeTransferIn, intentTo, tOrg(tDealer), ledger.StatusAvailable},
		{"to unmapped dealer", must2(hubMove("transferred_to_dealer", 0, tCenter)), true, ledger.TypeTransferIn, intentUnknown, ledger.Owner{}, ledger.StatusAvailable},
		{"service", must2(hubMove("used_in_service", tDealer, tCenter)), true, ledger.TypeConsumption, intentUnknown, ledger.Owner{}, ledger.StatusUsed},
		{"external", must2(hubMove("external_outbound", 0, tCenter)), true, ledger.TypeExternalOutbound, intentTrashHolder, ledger.Owner{}, ledger.StatusUsed},
		{"hub unknown", must2(hubMove("teleported", 0, tCenter)), false, ledger.TypeCountAdjustment, intentKeep, ledger.Owner{}, ""},
		{"placement", must2(whMove("placement", 1, tBin, tCenter)), true, ledger.TypePlacement, intentTo, tLoc(), ledger.StatusPlaced},
		{"placement no bin", must2(whMove("placement", 1, 0, tCenter)), true, ledger.TypePlacement, intentTo, tOrg(tCenter), ledger.StatusAvailable},
		{"order out", must2(whMove("order_out", -1, tBin, tCenter)), true, ledger.TypeOrderOut, intentTo, tOrg(tCenter), ledger.StatusInTransit},
		{"void", must2(whMove("void", -1, tBin, tCenter)), true, ledger.TypeVoid, intentTo,
			ledger.Owner{Type: ledger.OwnerTrash, ID: tCenter, OrgID: tCenter}, ledger.StatusVoid},
		{"count minus", must2(whMove("count_adjustment", -1, tBin, tCenter)), true, ledger.TypeCountAdjustment, intentTo,
			ledger.Owner{Type: ledger.OwnerTrash, ID: tCenter, OrgID: tCenter}, ledger.StatusVoid},
		{"wh unknown", must2(whMove("teleport", 1, tBin, tCenter)), false, ledger.TypeCountAdjustment, intentKeep, ledger.Owner{}, ""},
	}
	for _, c := range cases {
		mv, ok := c.mv, c.ok
		if mv.Type != c.typ || mv.Intent != c.intent || mv.To != c.to || mv.Status != c.status || ok != okOf(c.name) {
			t.Errorf("%s: got %+v ok=%v", c.name, mv, ok)
		}
	}
}

// must2 drops the known flag (checked through okOf).
func must2(mv legacyMove, _ bool) legacyMove { return mv }

func okOf(name string) bool { return !strings.HasSuffix(name, "unknown") }

func TestPlanOpeningHistoryMatches(t *testing.T) {
	u := tUnit(ledger.KindSerial)
	moves := []legacyMove{hubT(t, "1", "imported", tDealer, 1), hubT(t, "2", "transferred_to_dealer", tDealer, 5)}
	target := openingTarget{State: &ledger.UnitState{Owner: tOrg(tDealer), Status: ledger.StatusAvailable}}
	plan, err := planOpening(u, nil, moves, target)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Legacy != 2 || len(plan.Records) != 2 || plan.Unreachable || plan.Ownerless {
		t.Fatalf("plan = %+v", plan)
	}
	r := plan.Records[1]
	if r.From == nil || *r.From != tOrg(tCenter) || r.To == nil || *r.To != tOrg(tDealer) || r.OrganizationID != tCenter ||
		r.Key != "legacy:hub:stock_movements:2" || !r.CreatedAt.Equal(moves[1].At) || r.QuantityDelta != 0 {
		t.Errorf("transfer record = %+v", r)
	}
	if first := plan.Records[0]; first.From != nil || first.QuantityDelta != 1 || first.OrganizationID != tCenter {
		t.Errorf("entry record = %+v", first)
	}
	if up := replayPlan(t, u, plan); !sameState(up.State, target.State) || len(up.Anomalies) != 0 {
		t.Errorf("replayed %+v anomalies %v", up.State, up.Anomalies)
	}
}

func TestPlanOpeningCorrection(t *testing.T) {
	u := tUnit(ledger.KindSerial)
	trash := ledger.Owner{Type: ledger.OwnerTrash, ID: tCenter, OrgID: tCenter}
	target := openingTarget{State: &ledger.UnitState{Owner: trash, Status: ledger.StatusUsed}}
	moves := []legacyMove{hubT(t, "1", "imported", 0, 1)}
	plan, err := planOpening(u, nil, moves, target)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Legacy != 1 || len(plan.Records) != 2 || plan.Unreachable {
		t.Fatalf("plan = %+v", plan)
	}
	c := plan.Records[1]
	if c.Type != ledger.TypeCountAdjustment || c.Reason != openingReason || *c.From != tOrg(tCenter) || *c.To != trash ||
		c.QuantityDelta != -1 || !strings.HasPrefix(c.Key, "legacy:opening:") || !c.CreatedAt.Equal(moves[0].At) {
		t.Errorf("correction = %+v", c)
	}
	if up := replayPlan(t, u, plan); !sameState(up.State, target.State) {
		t.Errorf("replayed %+v", up.State)
	}
}

func TestPlanOpeningWithoutHistory(t *testing.T) {
	u := tUnit(ledger.KindSerial)
	target := openingTarget{State: &ledger.UnitState{Owner: tLoc(), Status: ledger.StatusPlaced}}
	plan, err := planOpening(u, nil, nil, target)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Legacy != 0 || len(plan.Records) != 1 || plan.Unreachable {
		t.Fatalf("plan = %+v", plan)
	}
	c := plan.Records[0]
	if c.From != nil || *c.To != tLoc() || c.OrganizationID != tCenter || c.QuantityDelta != 1 || !c.CreatedAt.Equal(u.CreatedAt.Time) {
		t.Errorf("opening = %+v", c)
	}
	if up := replayPlan(t, u, plan); !sameState(up.State, target.State) {
		t.Errorf("replayed %+v", up.State)
	}
}

// A dealer's unit that ends in a center location: the rebuild keeps the
// holder when a unit moves into a location, so the opening goes through
// the center first.
func TestPlanOpeningHolderChange(t *testing.T) {
	u := tUnit(ledger.KindSerial)
	target := openingTarget{State: &ledger.UnitState{Owner: tLoc(), Status: ledger.StatusPlaced}}
	moves := []legacyMove{hubT(t, "1", "imported", tDealer, 1), hubT(t, "2", "transferred_to_dealer", tDealer, 2)}
	plan, err := planOpening(u, nil, moves, target)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Legacy != 2 || len(plan.Records) != 4 || plan.Unreachable {
		t.Fatalf("plan = %+v", plan)
	}
	if up := replayPlan(t, u, plan); !sameState(up.State, target.State) {
		t.Errorf("replayed %+v", up.State)
	}
}

// A unit used in a service has no owner until F2-01h: its history is
// recorded without owners and the ledger gives it none.
func TestPlanOpeningOwnerless(t *testing.T) {
	u := tUnit(ledger.KindSerial)
	moves := []legacyMove{
		hubT(t, "1", "imported", tDealer, 1), hubT(t, "2", "transferred_to_dealer", tDealer, 2),
		hubT(t, "3", "used_in_service", tDealer, 3),
	}
	plan, err := planOpening(u, nil, moves, openingTarget{})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Ownerless || plan.Legacy != 3 || len(plan.Records) != 3 || plan.Unreachable {
		t.Fatalf("plan = %+v", plan)
	}
	for _, r := range plan.Records {
		if r.From != nil || r.To != nil || r.QuantityDelta != 0 || r.OrganizationID != tCenter {
			t.Errorf("ownerless record = %+v", r)
		}
	}
	if plan.Records[2].ToStatus != ledger.StatusUsed {
		t.Errorf("status = %s", plan.Records[2].ToStatus)
	}
	if up := replayPlan(t, u, plan); up.State != nil || up.Status != "" {
		t.Errorf("replayed %+v", up.State)
	}
}

// Fixed barcodes: owner-less history, then one correction per owner.
func TestPlanOpeningFixed(t *testing.T) {
	u := tUnit(ledger.KindFixed)
	mv, ok := whMove("placement", 5, tBin, tCenter)
	moves := []legacyMove{tMove(t, "a", mv, ok, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))}
	target := openingTarget{Holdings: []ledger.FixedHolding{
		{Owner: tLoc(), Quantity: 4}, {Owner: tOrg(tDealer), Quantity: 2},
	}}
	plan, err := planOpening(u, nil, moves, target)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Legacy != 1 || len(plan.Records) != 3 || plan.Unreachable {
		t.Fatalf("plan = %+v", plan)
	}
	if r := plan.Records[0]; r.To != nil || r.From != nil {
		t.Errorf("fixed history record = %+v", r)
	}
	up := replayPlan(t, u, plan)
	if len(fixedCorrections(up.Holdings, target.Holdings)) != 0 {
		t.Errorf("holdings = %+v", up.Holdings)
	}
	if c := plan.Records[2]; *c.To != tOrg(tDealer) || c.QuantityDelta != 2 || c.OrganizationID != tDealer {
		t.Errorf("dealer correction = %+v", c)
	}
}

// A rerun with the history already in the ledger plans nothing.
func TestPlanOpeningRerun(t *testing.T) {
	u := tUnit(ledger.KindSerial)
	target := openingTarget{State: &ledger.UnitState{Owner: tOrg(tDealer), Status: ledger.StatusAvailable}}
	moves := []legacyMove{hubT(t, "1", "imported", tDealer, 1), hubT(t, "2", "transferred_to_dealer", tDealer, 5)}
	first, err := planOpening(u, nil, moves, target)
	if err != nil {
		t.Fatal(err)
	}
	var existing []db.StockMovement
	for i, r := range first.Records {
		existing = append(existing, recordedMovement(u, r, int64(100+i)))
	}
	again, err := planOpening(u, existing, nil, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Records) != 0 || again.Unreachable {
		t.Errorf("rerun plan = %+v", again)
	}
}
