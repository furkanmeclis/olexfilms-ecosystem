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
	// product overrides the unit's product (movements before a
	// reclassification); meta is the metadata JSON.
	product int64
	meta    string
}

func rows(u db.Unit, in ...mvRow) []db.StockMovement {
	out := make([]db.StockMovement, len(in))
	for i, r := range in {
		m := db.StockMovement{
			ID: int64(100 + i), UnitID: u.ID, ProductID: u.ProductID, OrganizationID: r.org, Type: string(r.typ),
			QuantityDelta: r.qty, MetersDelta: cmToNumeric(r.cm),
			FromStatus: text(string(r.fromSt)), ToStatus: text(string(r.toSt)),
			ReferenceType: text(r.refType), ReferenceID: pgtype.Int8{Int64: r.refID, Valid: r.refID != 0},
		}
		if r.product != 0 {
			m.ProductID = r.product
		}
		if r.meta != "" {
			m.Metadata = []byte(r.meta)
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
	mvs := rows(u,
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
	mvs := rows(u,
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
	mvs := rows(u,
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
	mvs := rows(u,
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
	if _, err := p.Replay(db.Unit{ID: 12}, rows(db.Unit{ID: 13}, mvRow{typ: TypeEntry})); err == nil {
		t.Fatal("a movement of another unit must be rejected")
	}
}

// TEC-157: a reclassification keeps owner and status; the unit's stock
// counts under the new product and the movements after it carry the new
// product.
func reclassChain(newProduct int64) []mvRow {
	return []mvRow{
		{typ: TypeEntry, org: center, qty: 1, toT: OwnerWarehouseLocation, toID: cLoc, fromSt: StatusPrinted, toSt: StatusAvailable, product: 9},
		{typ: TypeTransferOut, org: center, qty: -1, fromT: OwnerWarehouseLocation, fromID: cLoc,
			toT: OwnerOrganization, toID: dist, fromSt: StatusAvailable, toSt: StatusInTransit, refType: "t", refID: 1, product: 9},
		{typ: TypeTransferIn, org: dist, qty: 1, fromT: OwnerOrganization, fromID: dist,
			toT: OwnerWarehouseLocation, toID: dLoc, fromSt: StatusInTransit, toSt: StatusAvailable, refType: "t", refID: 1, product: 9},
		{typ: TypeReclassification, org: dist, fromT: OwnerWarehouseLocation, fromID: dLoc,
			toT: OwnerWarehouseLocation, toID: dLoc, fromSt: StatusAvailable, toSt: StatusAvailable,
			refType: ReclassificationRefType, refID: 4, product: newProduct, meta: `{"from_product_id": 9, "to_product_id": 12}`},
		{typ: TypePartialConsumption, org: dist, cm: -300, fromT: OwnerWarehouseLocation, fromID: dLoc,
			toT: OwnerWarehouseLocation, toID: dLoc, fromSt: StatusAvailable, toSt: StatusAvailable, product: newProduct},
	}
}

func TestReplayReclassification(t *testing.T) {
	u := db.Unit{ID: 14, BrandID: 3, ProductID: 12, UnitKind: KindSerial, Status: "available",
		InitialMeters: cmToNumeric(1500), RemainingMeters: cmToNumeric(1200)}
	p := NewProjection()
	up, err := p.Replay(u, rows(u, reclassChain(12)...))
	if err != nil {
		t.Fatal(err)
	}
	if len(up.Anomalies) != 0 {
		t.Fatalf("anomalies: %v", up.Anomalies)
	}
	want := Owner{Type: OwnerWarehouseLocation, ID: dLoc, OrgID: dist}
	if up.ProductID != 12 || up.State == nil || up.State.Owner != want || up.Status != StatusAvailable {
		t.Fatalf("unit = %+v state %+v", up, up.State)
	}
	if up.RemainingCm == nil || *up.RemainingCm != 1200 {
		t.Fatalf("remaining = %v", up.RemainingCm)
	}
	if got := p.OrgStocks[OrgStockKey{dist, 12}]; got.Quantity != 1 || got.Centimeters != 1200 || got.BrandID != 3 {
		t.Fatalf("dist stock of the new product = %+v", got)
	}
	if got := p.BinStocks[BinStockKey{dLoc, 12}]; got.Quantity != 1 || got.OrganizationID != dist {
		t.Fatalf("dist bin of the new product = %+v", got)
	}
	if _, ok := p.OrgStocks[OrgStockKey{dist, 9}]; ok {
		t.Fatal("the old product keeps no stock of the unit")
	}
}

func TestReplayReclassificationAnomalies(t *testing.T) {
	// units.product_id was not changed with the movement.
	u := db.Unit{ID: 15, BrandID: 3, ProductID: 9, UnitKind: KindSerial, Status: "available",
		InitialMeters: cmToNumeric(1500), RemainingMeters: cmToNumeric(1200)}
	up, err := NewProjection().Replay(u, rows(u, reclassChain(12)...))
	if err != nil {
		t.Fatal(err)
	}
	if len(up.Anomalies) != 1 || up.ProductID != 12 {
		t.Fatalf("anomalies = %v product %d, want the units.product_id break", up.Anomalies, up.ProductID)
	}

	// A movement after the reclassification still on the old product, and
	// a reclassification without its source product.
	chain := reclassChain(12)
	chain[4].product = 9
	chain[3].meta = `{}`
	u.ID = 16
	up, err = NewProjection().Replay(u, rows(u, chain...))
	if err != nil {
		t.Fatal(err)
	}
	if len(up.Anomalies) != 2 {
		t.Fatalf("anomalies = %v, want missing from_product_id and product break", up.Anomalies)
	}
}
