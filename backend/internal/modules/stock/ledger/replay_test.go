package ledger

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

// Pure replay tests (TEC-156): the movement rows are built the way Post
// records them.

// center and dist come from transitions_test.go.
const (
	cLoc = int64(10)
	dLoc = int64(20)
)

type mvRow struct {
	typ          MovementType
	org          int64
	qty          int32
	cm           int64
	fromT        OwnerType
	fromID       int64
	toT          OwnerType
	toID         int64
	fromSt, toSt Status
	refType      string
	refID        int64
}

func rows(unit int64, in ...mvRow) []db.StockMovement {
	out := make([]db.StockMovement, len(in))
	for i, r := range in {
		m := db.StockMovement{
			ID: int64(100 + i), UnitID: unit, OrganizationID: r.org, Type: string(r.typ),
			QuantityDelta: r.qty, MetersDelta: cmToNumeric(r.cm),
			FromStatus: text(string(r.fromSt)), ToStatus: text(string(r.toSt)),
			ReferenceType: text(r.refType), ReferenceID: pgtype.Int8{Int64: r.refID, Valid: r.refID != 0},
		}
		if r.fromT != "" {
			m.FromOwnerType, m.FromOwnerID = text(string(r.fromT)), i8(r.fromID)
		}
		if r.toT != "" {
			m.ToOwnerType, m.ToOwnerID = text(string(r.toT)), i8(r.toID)
		}
		out[i] = m
	}
	return out
}

func TestReplaySerialRollChain(t *testing.T) {
	u := db.Unit{ID: 7, BrandID: 3, ProductID: 9, UnitKind: KindSerial, Status: "printed",
		InitialMeters: cmToNumeric(1500), RemainingMeters: cmToNumeric(600)}
	mvs := rows(u.ID,
		mvRow{typ: TypeEntry, org: center, qty: 1, toT: OwnerWarehouseLocation, toID: cLoc, fromSt: StatusPrinted, toSt: StatusAvailable},
		mvRow{typ: TypeTransferOut, org: center, qty: -1, fromT: OwnerWarehouseLocation, fromID: cLoc,
			toT: OwnerOrganization, toID: dist, fromSt: StatusAvailable, toSt: StatusInTransit, refType: "t", refID: 1},
		mvRow{typ: TypeTransferIn, org: dist, qty: 1, fromT: OwnerOrganization, fromID: dist,
			toT: OwnerWarehouseLocation, toID: dLoc, fromSt: StatusInTransit, toSt: StatusAvailable, refType: "t", refID: 1},
		mvRow{typ: TypePartialConsumption, org: dist, cm: -500, fromT: OwnerWarehouseLocation, fromID: dLoc,
			toT: OwnerWarehouseLocation, toID: dLoc, fromSt: StatusAvailable, toSt: StatusAvailable},
		mvRow{typ: TypePartialConsumption, org: dist, cm: -400, fromT: OwnerWarehouseLocation, fromID: dLoc,
			toT: OwnerWarehouseLocation, toID: dLoc, fromSt: StatusAvailable, toSt: StatusAvailable},
	)
	p := NewProjection()
	up, err := p.Replay(u, mvs)
	if err != nil {
		t.Fatal(err)
	}
	if len(up.Anomalies) != 0 {
		t.Fatalf("anomalies: %v", up.Anomalies)
	}
	want := Owner{Type: OwnerWarehouseLocation, ID: dLoc, OrgID: dist}
	if up.State == nil || up.State.Owner != want || up.State.Status != StatusAvailable || up.State.LastMovementID != 104 {
		t.Fatalf("state = %+v", up.State)
	}
	if up.RemainingCm == nil || *up.RemainingCm != 600 {
		t.Fatalf("remaining = %v, want 6.00 m (15-5-4)", up.RemainingCm)
	}
	if got := p.OrgStocks[OrgStockKey{dist, 9}]; got.Quantity != 1 || got.Centimeters != 600 || got.BrandID != 3 {
		t.Fatalf("dist stock = %+v", got)
	}
	if got := p.BinStocks[BinStockKey{dLoc, 9}]; got.Quantity != 1 || got.OrganizationID != dist {
		t.Fatalf("dist bin = %+v", got)
	}
	if _, ok := p.OrgStocks[OrgStockKey{center, 9}]; ok {
		t.Fatal("center keeps no stock of the unit")
	}
}

func TestReplayCancelRestoreGoesBackToSourceHolder(t *testing.T) {
	u := db.Unit{ID: 8, BrandID: 3, ProductID: 9, UnitKind: KindSerial, Status: "printed"}
	mvs := rows(u.ID,
		mvRow{typ: TypeEntry, org: center, qty: 1, toT: OwnerWarehouseLocation, toID: cLoc, fromSt: StatusPrinted, toSt: StatusAvailable},
		mvRow{typ: TypePlacement, org: center, fromT: OwnerWarehouseLocation, fromID: cLoc,
			toT: OwnerWarehouseLocation, toID: cLoc + 1, fromSt: StatusAvailable, toSt: StatusPlaced},
		mvRow{typ: TypeOrderOut, org: center, qty: -1, fromT: OwnerWarehouseLocation, fromID: cLoc + 1,
			toT: OwnerOrganization, toID: dist, fromSt: StatusPlaced, toSt: StatusInTransit, refType: "o", refID: 5},
		mvRow{typ: TypeOrderCancelRestore, org: dist, qty: 1, fromT: OwnerOrganization, fromID: dist,
			toT: OwnerWarehouseLocation, toID: cLoc + 1, fromSt: StatusInTransit, toSt: StatusPlaced, refType: "o", refID: 5},
	)
	p := NewProjection()
	up, err := p.Replay(u, mvs)
	if err != nil {
		t.Fatal(err)
	}
	want := Owner{Type: OwnerWarehouseLocation, ID: cLoc + 1, OrgID: center}
	if len(up.Anomalies) != 0 || up.State.Owner != want || up.Status != StatusPlaced {
		t.Fatalf("state = %+v anomalies %v", up.State, up.Anomalies)
	}
	if got := p.OrgStocks[OrgStockKey{center, 9}]; got.Quantity != 1 {
		t.Fatalf("center stock = %+v", got)
	}
	if got := p.OrgStocks[OrgStockKey{dist, 9}]; got.Quantity != 0 {
		t.Fatalf("dist stock = %+v", got)
	}
}

func TestReplayFixedHoldings(t *testing.T) {
	u := db.Unit{ID: 9, BrandID: 3, ProductID: 11, UnitKind: KindFixed, Status: "printed"}
	mvs := rows(u.ID,
		mvRow{typ: TypeEntry, org: center, qty: 10, toT: OwnerWarehouseLocation, toID: cLoc, fromSt: StatusPrinted, toSt: StatusAvailable},
		mvRow{typ: TypeTransferOut, org: center, qty: -4, fromT: OwnerWarehouseLocation, fromID: cLoc,
			fromSt: StatusAvailable, toSt: StatusAvailable, refType: "t", refID: 2},
		mvRow{typ: TypeTransferIn, org: dist, qty: 4, toT: OwnerOrganization, toID: dist,
			fromSt: StatusAvailable, toSt: StatusAvailable, refType: "t", refID: 2},
	)
	p := NewProjection()
	up, err := p.Replay(u, mvs)
	if err != nil {
		t.Fatal(err)
	}
	if up.State != nil || up.Status != StatusAvailable || len(up.Holdings) != 2 || len(up.Anomalies) != 0 {
		t.Fatalf("unit = %+v", up)
	}
	if h := up.Holdings[0]; h.Quantity != 6 || h.Owner.OrgID != center || h.LastMovementID != 101 {
		t.Fatalf("center holding = %+v", h)
	}
	if h := up.Holdings[1]; h.Quantity != 4 || h.Owner != (Owner{OwnerOrganization, dist, dist}) {
		t.Fatalf("dist holding = %+v", h)
	}
	if got := p.OrgStocks[OrgStockKey{center, 11}]; got.Quantity != 6 {
		t.Fatalf("center = %+v", got)
	}
	if got := p.BinStocks[BinStockKey{cLoc, 11}]; got.Quantity != 6 {
		t.Fatalf("center bin = %+v", got)
	}
	if got := p.OrgStocks[OrgStockKey{dist, 11}]; got.Quantity != 4 {
		t.Fatalf("dist = %+v", got)
	}
}

func TestReplayReportsChainBreak(t *testing.T) {
	u := db.Unit{ID: 10, BrandID: 3, ProductID: 9, UnitKind: KindSerial, Status: "printed"}
	mvs := rows(u.ID,
		mvRow{typ: TypeEntry, org: center, qty: 1, toT: OwnerWarehouseLocation, toID: cLoc, fromSt: StatusPrinted, toSt: StatusAvailable},
		// Recorded as leaving a location it never reached.
		mvRow{typ: TypeTransferOut, org: center, qty: -1, fromT: OwnerWarehouseLocation, fromID: cLoc + 5,
			toT: OwnerOrganization, toID: dist, fromSt: StatusPlaced, toSt: StatusInTransit, refType: "t", refID: 3},
	)
	up, err := NewProjection().Replay(u, mvs)
	if err != nil {
		t.Fatal(err)
	}
	if len(up.Anomalies) != 2 {
		t.Fatalf("anomalies = %v, want from_status and from_owner", up.Anomalies)
	}
}

func TestReplayNoMovements(t *testing.T) {
	p := NewProjection()
	up, err := p.Replay(db.Unit{ID: 11, UnitKind: KindSerial, Status: "reserved"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if up.State != nil || up.Status != "" || len(p.OrgStocks) != 0 {
		t.Fatalf("unit = %+v", up)
	}
	if _, err := p.Replay(db.Unit{ID: 12}, rows(13, mvRow{typ: TypeEntry})); err == nil {
		t.Fatal("a movement of another unit must be rejected")
	}
}
