package ledger

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

func TestPlanSerialSplit(t *testing.T) {
	cur := roll(held(StatusAvailable, distLoc), 5000, 5000)
	m := mv(TypeSplit, nil)
	m.Centimeters = 1200
	plan, err := planSerial(m, cur, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.metersDelta != -1200 || plan.toStatus != StatusAvailable || plan.toOwner != distLoc || plan.quantityDelta != 0 {
		t.Fatalf("plan = %+v", plan)
	}
	// The rest of the roll: used up, out of the stock count.
	m.Centimeters = 5000
	trash := Owner{Type: OwnerTrash, ID: dist, OrgID: dist}
	if plan, err = planSerial(m, cur, nil); err != nil || plan.toStatus != StatusUsed || plan.quantityDelta != -1 ||
		plan.metersDelta != -5000 || plan.toOwner != trash || !plan.ownerChanged {
		t.Fatalf("plan = %+v %v", plan, err)
	}
	m.Centimeters = 5001
	if _, err := planSerial(m, cur, nil); !errors.Is(err, ErrInsufficientMeters) {
		t.Fatalf("above remaining: %v", err)
	}
	m.Centimeters = 0
	if _, err := planSerial(m, cur, nil); !errors.Is(err, ErrInvalidMovement) {
		t.Fatalf("zero meters: %v", err)
	}
	m.Centimeters = 100
	if _, err := planSerial(m, held(StatusAvailable, distLoc), nil); !errors.Is(err, ErrTransitionNotAllowed) {
		t.Fatalf("piece split: %v", err)
	}
	if _, err := planSerial(m, roll(held(StatusInTransit, distOrg), 5000, 5000), nil); !errors.Is(err, ErrTransitionNotAllowed) {
		t.Fatalf("in transit split: %v", err)
	}
	if _, err := planFixed(Movement{Type: TypeSplit, Quantity: 1, To: ptr(distLoc)}, StatusAvailable); !errors.Is(err, ErrTransitionNotAllowed) {
		t.Fatalf("fixed split: %v", err)
	}
	if slices.Contains(PostableTypes, TypeSplit) {
		t.Fatal("split must not be postable")
	}
	if EventName(TypeSplit) != "stock.split" {
		t.Fatalf("event = %q", EventName(TypeSplit))
	}
}

func TestSplitBarcode(t *testing.T) {
	if got := SplitBarcode("OLX-ROLL-01", 3); got != "OLX-ROLL-01-S3" {
		t.Fatalf("barcode = %q", got)
	}
	long := strings.Repeat("A", 64)
	if got := SplitBarcode(long, 12); len(got) != 64 || !strings.HasSuffix(got, "-S12") {
		t.Fatalf("long barcode = %q (%d)", got, len(got))
	}
}

func TestReplaySplit(t *testing.T) {
	src := db.Unit{ID: 21, BrandID: 3, ProductID: 9, UnitKind: KindSerial, Status: "available",
		InitialMeters: cmToNumeric(5000), RemainingMeters: cmToNumeric(3800)}
	nu := db.Unit{ID: 22, BrandID: 3, ProductID: 9, UnitKind: KindSerial, Status: "available",
		InitialMeters: cmToNumeric(1200), RemainingMeters: cmToNumeric(1200)}
	srcRows := rows(src,
		mvRow{typ: TypeEntry, org: center, qty: 1, toT: OwnerWarehouseLocation, toID: cLoc, fromSt: StatusPrinted, toSt: StatusAvailable},
		mvRow{typ: TypeSplit, org: center, cm: -1200, fromT: OwnerWarehouseLocation, fromID: cLoc,
			toT: OwnerWarehouseLocation, toID: cLoc, fromSt: StatusAvailable, toSt: StatusAvailable,
			refType: SplitRefType, refID: 4, meta: `{"split_role":"source"}`},
	)
	tgt := mvRow{typ: TypeSplit, org: center, qty: 1, toT: OwnerWarehouseLocation, toID: cLoc, toSt: StatusAvailable,
		refType: SplitRefType, refID: 4, meta: `{"split_role":"target"}`}
	nuRows := rows(nu, tgt)
	p := NewProjection()
	for _, c := range []struct {
		u   db.Unit
		mvs []db.StockMovement
		cm  int64
	}{{src, srcRows, 3800}, {nu, nuRows, 1200}} {
		up, err := p.Replay(c.u, c.mvs)
		if err != nil {
			t.Fatal(err)
		}
		if len(up.Anomalies) != 0 {
			t.Fatalf("unit %d anomalies: %v", c.u.ID, up.Anomalies)
		}
		want := Owner{Type: OwnerWarehouseLocation, ID: cLoc, OrgID: center}
		if up.State == nil || up.State.Owner != want || up.State.Status != StatusAvailable || *up.RemainingCm != c.cm {
			t.Fatalf("unit %d state = %+v remaining %v", c.u.ID, up.State, up.RemainingCm)
		}
	}
	if a := p.SplitAnomalies(); len(a) != 0 {
		t.Fatalf("split anomalies: %v", a)
	}
	if got := p.OrgStocks[OrgStockKey{center, 9}]; got.Quantity != 2 || got.Centimeters != 5000 {
		t.Fatalf("center stock = %+v, want 2 units / 50 m", got)
	}

	// A target with more meters than the source lost is reported.
	bad := nu
	bad.ID, bad.InitialMeters, bad.RemainingMeters = 23, cmToNumeric(1300), cmToNumeric(1300)
	p2 := NewProjection()
	if _, err := p2.Replay(src, srcRows); err != nil {
		t.Fatal(err)
	}
	if _, err := p2.Replay(bad, rows(bad, tgt)); err != nil {
		t.Fatal(err)
	}
	if a := p2.SplitAnomalies(); len(a) != 1 {
		t.Fatalf("split anomalies = %v, want 1", a)
	}

	// A split without its role or that moves the roll is an anomaly.
	odd := rows(src,
		mvRow{typ: TypeEntry, org: center, qty: 1, toT: OwnerWarehouseLocation, toID: cLoc, fromSt: StatusPrinted, toSt: StatusAvailable},
		mvRow{typ: TypeSplit, org: center, cm: -1200, fromT: OwnerWarehouseLocation, fromID: cLoc,
			toT: OwnerWarehouseLocation, toID: cLoc + 1, fromSt: StatusAvailable, toSt: StatusAvailable,
			refType: SplitRefType, refID: 5, meta: `{"split_role":"source"}`},
		mvRow{typ: TypeSplit, org: center, cm: -100, fromT: OwnerWarehouseLocation, fromID: cLoc + 1,
			toT: OwnerWarehouseLocation, toID: cLoc + 1, fromSt: StatusAvailable, toSt: StatusAvailable,
			refType: SplitRefType, refID: 6},
	)
	up, err := NewProjection().Replay(src, odd)
	if err != nil {
		t.Fatal(err)
	}
	if len(up.Anomalies) < 2 {
		t.Fatalf("anomalies = %v, want the moved roll and the missing role", up.Anomalies)
	}
}
