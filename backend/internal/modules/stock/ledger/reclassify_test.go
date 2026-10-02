package ledger

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
)

// TEC-157: the routing rules of a reclassification (pure).
func TestCheckReclassification(t *testing.T) {
	from := db.Product{ID: 9, BrandID: 3, UnitType: "piece", Active: true}
	to := db.Product{ID: 12, BrandID: 3, UnitType: "piece", Active: true}
	unit := db.Unit{ID: 7, BrandID: 3, ProductID: 9, UnitKind: KindSerial, Status: "available"}
	held := &db.UnitCurrentState{UnitID: 7, OwnerType: string(OwnerWarehouseLocation), OwnerID: 20,
		HolderOrgID: dist, Status: string(StatusAvailable)}
	with := func(f func(u *db.Unit, st *db.UnitCurrentState, to *db.Product)) (db.Unit, *db.UnitCurrentState, db.Product) {
		u, st, p := unit, *held, to
		f(&u, &st, &p)
		return u, &st, p
	}

	if err := CheckReclassification(unit, held, from, to, dist); err != nil {
		t.Fatalf("valid reclassification: %v", err)
	}
	placed := *held
	placed.Status = string(StatusPlaced)
	if err := CheckReclassification(unit, &placed, from, to, dist); err != nil {
		t.Fatalf("placed unit: %v", err)
	}

	cases := []struct {
		name string
		mut  func(u *db.Unit, st *db.UnitCurrentState, to *db.Product)
		org  int64
		want error
	}{
		{"fixed barcode", func(u *db.Unit, _ *db.UnitCurrentState, _ *db.Product) { u.UnitKind = KindFixed }, dist, ErrReclassifyFixedBarcode},
		{"product changed", func(u *db.Unit, _ *db.UnitCurrentState, _ *db.Product) { u.ProductID = 10 }, dist, ErrReclassifyProductChanged},
		{"same product", func(_ *db.Unit, _ *db.UnitCurrentState, p *db.Product) { p.ID = 9 }, dist, ErrReclassifySameProduct},
		{"other brand", func(_ *db.Unit, _ *db.UnitCurrentState, p *db.Product) { p.BrandID = 4 }, dist, ErrReclassifyBrandMismatch},
		{"roll target", func(_ *db.Unit, _ *db.UnitCurrentState, p *db.Product) { p.UnitType = "roll_meter" }, dist, ErrReclassifyUnitTypeMismatch},
		{"fixed target", func(_ *db.Unit, _ *db.UnitCurrentState, p *db.Product) { p.UsesFixedBarcode = true }, dist, ErrReclassifyUnitTypeMismatch},
		{"inactive target", func(_ *db.Unit, _ *db.UnitCurrentState, p *db.Product) { p.Active = false }, dist, ErrReclassifyProductInactive},
		{"used", func(_ *db.Unit, st *db.UnitCurrentState, _ *db.Product) {
			st.Status, st.OwnerType = string(StatusUsed), string(OwnerService)
		}, dist, ErrReclassifyUnitConsumed},
		{"void", func(_ *db.Unit, st *db.UnitCurrentState, _ *db.Product) {
			st.Status, st.OwnerType = string(StatusVoid), string(OwnerTrash)
		}, dist, ErrReclassifyUnitConsumed},
		{"in transit", func(_ *db.Unit, st *db.UnitCurrentState, _ *db.Product) { st.Status = string(StatusInTransit) }, dist, ErrReclassifyUnitElsewhere},
		{"other holder", func(_ *db.Unit, _ *db.UnitCurrentState, _ *db.Product) {}, center, ErrReclassifyUnitElsewhere},
	}
	for _, tc := range cases {
		u, st, p := with(tc.mut)
		if err := CheckReclassification(u, st, from, p, tc.org); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
	// A printed label has no state yet: not in stock.
	if err := CheckReclassification(db.Unit{ID: 8, BrandID: 3, ProductID: 9, UnitKind: KindSerial, Status: "printed"},
		nil, from, to, center); !errors.Is(err, ErrReclassifyUnitElsewhere) {
		t.Fatalf("printed label: %v", err)
	}
}

// fakeTx is a non-nil transaction that is never used.
type fakeTx struct{ pgx.Tx }

// Post never writes a reclassification: only an approved request does.
func TestPostRefusesReclassification(t *testing.T) {
	if slices.Contains(PostableTypes, TypeReclassification) {
		t.Fatal("reclassification must not be postable")
	}
	_, err := New(nil, nil).Post(context.Background(), fakeTx{}, Movement{Type: TypeReclassification, UnitID: 1})
	if !errors.Is(err, ErrInvalidMovement) {
		t.Fatalf("Post(reclassification) = %v, want ErrInvalidMovement", err)
	}
	if EventName(TypeReclassification) != "stock.reclassification" {
		t.Fatalf("event = %q", EventName(TypeReclassification))
	}
}
