package usecase_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1230: a hand cut shorter than the product's min_cut_meters is
// refused unless it takes the whole rest; a roll left under the minimum is
// a remnant, flagged on the split and filtered apart in the units list.
func TestSplitMinCutAndRemnant(t *testing.T) {
	e := newLabelEnv(t)
	ctx, q := e.ctx, e.q
	var minCut pgtype.Numeric
	if err := minCut.Scan("1.5"); err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateProductRollSettings(ctx, db.UpdateProductRollSettingsParams{
		MinCutMeters: minCut, Uuid: e.roll.Uuid, BrandID: e.brand.ID,
	}); err != nil {
		t.Fatalf("roll settings: %v", err)
	}
	var meters pgtype.Numeric
	if err := meters.Scan("5"); err != nil {
		t.Fatal(err)
	}
	u, err := q.CreateUnit(ctx, db.CreateUnitParams{
		OrganizationID: e.center.ID, BrandID: e.brand.ID, ProductID: e.roll.ID,
		Barcode:  fmt.Sprintf("T1230-%s", e.suffix),
		UnitKind: ledger.KindSerial, Source: "generated", Status: string(ledger.StatusPrinted),
		InitialMeters: meters, RemainingMeters: meters,
	})
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	l := ledger.New(q, outbox.NewStore(e.pool, q))
	if _, err := l.Post(ctx, tx, ledger.Movement{
		Type: ledger.TypeEntry, UnitID: u.ID, To: &ledger.Owner{Type: ledger.OwnerOrganization, ID: e.center.ID, OrgID: e.center.ID},
		Source: "test", RefType: "t1230", RefID: u.ID,
	}); err != nil {
		t.Fatalf("entry: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	sp := stockusecase.NewSplits(e.pool, q, outbox.NewStore(e.pool, q))
	c := e.caller(e.center)
	if _, _, err := sp.Split(ctx, c, stockusecase.SplitInput{Barcode: u.Barcode, Meters: "1", IdempotencyKey: "t1230-a-" + e.suffix}); !errors.Is(err, ledger.ErrBelowMinCut) {
		t.Fatalf("below minimum: %v", err)
	}
	// 5 - 4 = 1 m left: under the 1.5 m minimum, a remnant.
	out, _, err := sp.Split(ctx, c, stockusecase.SplitInput{Barcode: u.Barcode, Meters: "4", IdempotencyKey: "t1230-b-" + e.suffix})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if !out.Source.Remnant || out.NewUnit.Remnant || out.Source.RemainingMeters != "1.00" {
		t.Fatalf("split view = %+v", out)
	}
	// The whole remnant may still be taken.
	if _, _, err := sp.Split(ctx, c, stockusecase.SplitInput{Barcode: out.NewUnit.Barcode, Meters: "1", IdempotencyKey: "t1230-c-" + e.suffix}); !errors.Is(err, ledger.ErrBelowMinCut) {
		t.Fatalf("below minimum on the new roll: %v", err)
	}

	svc := stockusecase.New(q)
	v := stockusecase.UnitViewer{Org: c.Org, Filter: c.Filter}
	yes, no := true, false
	list := func(remnant *bool) []model.StockUnitRow {
		rows, _, err := svc.OrganizationUnits(ctx, v, e.center.Uuid, model.UnitFilter{
			ProductUUID: &e.roll.Uuid, Remnant: remnant, Limit: 50,
		})
		if err != nil {
			t.Fatalf("units: %v", err)
		}
		return rows
	}
	rem := list(&yes)
	if len(rem) != 1 || rem[0].Barcode != u.Barcode || !rem[0].Remnant {
		t.Fatalf("remnants = %+v", rem)
	}
	rest := list(&no)
	if len(rest) != 1 || rest[0].Barcode != out.NewUnit.Barcode || rest[0].Remnant {
		t.Fatalf("non remnants = %+v", rest)
	}
	if all := list(nil); len(all) != 2 {
		t.Fatalf("all = %d", len(all))
	}
}
