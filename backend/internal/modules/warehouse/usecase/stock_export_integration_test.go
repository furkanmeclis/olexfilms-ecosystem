package usecase_test

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-761: the stock exports of the warehouse screens (product stock,
// location stock, barcode history, barcode batches) read through the
// screen use cases with the stored stock.read scope.

func runStockExport(t *testing.T, x *stockusecase.StockExporter, resource string, q ioengine.ExportQuery, org int64) ioengine.Dataset {
	t.Helper()
	q[ioengine.QueryOrganizationID] = strconv.FormatInt(org, 10)
	for _, a := range x.Adapters() {
		if a.Resource() == resource {
			ds, err := a.Export(t.Context(), q, i18n.LocaleTR)
			if err != nil {
				t.Fatalf("export %s: %v", resource, err)
			}
			return ds
		}
	}
	t.Fatalf("no adapter %s", resource)
	return ioengine.Dataset{}
}

func TestStockScreenExports(t *testing.T) {
	e := newCountEnv(t)
	x := stockusecase.NewStockExporter(e.pool, stockusecase.New(e.q))
	_, _, locs := e.warehouse(t, "S", "A1")
	l1 := locs[0]
	a := e.unit(t, e.piece, "", &l1, 0)
	e.unit(t, e.piece, "", &l1, 0)
	center := e.caller(false).Caller
	v := stockusecase.UnitViewer{Org: center.Org, Filter: center.Filter}

	// Location stock: the bin holds two units of the piece product.
	q, err := x.LocationStockExportQuery(e.ctx, v.Filter, l1.view.UUID, nil)
	if err != nil {
		t.Fatalf("location query: %v", err)
	}
	ds := runStockExport(t, x, stockusecase.ResourceLocationStockExport, q, e.center.ID)
	if len(ds.Rows) != 1 || ds.Rows[0]["sku"] != e.piece.Sku || ds.Rows[0]["quantity"] != "2" {
		t.Fatalf("location stock = %+v", ds.Rows)
	}

	// Product stock of the center, narrowed to the product (the list filter).
	q, err = x.ProductStockExportQuery(e.ctx, v.Filter, e.center.Uuid, map[string]string{"product_uuid": e.piece.Uuid.String()})
	if err != nil {
		t.Fatalf("product query: %v", err)
	}
	ds = runStockExport(t, x, stockusecase.ResourceProductStockExport, q, e.center.ID)
	if len(ds.Rows) != 1 || ds.Rows[0]["product"] != e.piece.Name {
		t.Fatalf("product stock = %+v", ds.Rows)
	}
	if _, err := x.ProductStockExportQuery(e.ctx, v.Filter, e.center.Uuid, map[string]string{"status": "bogus"}); err == nil {
		t.Fatal("bad status accepted")
	}

	// Barcode history: entry + placement, translated, with the unit header.
	q, err = x.UnitHistoryExportQuery(e.ctx, v, a.Barcode)
	if err != nil {
		t.Fatalf("history query: %v", err)
	}
	ds = runStockExport(t, x, stockusecase.ResourceUnitHistoryExport, q, e.center.ID)
	if len(ds.Rows) != 2 {
		t.Fatalf("history rows = %+v", ds.Rows)
	}
	types := map[any]bool{ds.Rows[0]["type"]: true, ds.Rows[1]["type"]: true}
	if !types["Stok girişi"] || !types["Yerleştirme"] {
		t.Fatalf("history types = %v", types)
	}
	if ds.Info[0].Value != a.Barcode {
		t.Fatalf("history info = %+v", ds.Info)
	}

	// A distributor outside the reach finds neither the unit nor the bin.
	dist, err := e.q.CreateOrganization(e.ctx, db.CreateOrganizationParams{
		Slug: "t761-dist-" + e.suffix, Name: "dist", Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "distributor", ParentID: pgtype.Int8{Int64: e.center.ID, Valid: true},
		BrandID: e.brand.ID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatal(err)
	}
	dv := stockusecase.UnitViewer{
		Org:    orgctx.Scope{InternalID: dist.ID, UUID: dist.Uuid, OrgType: dist.Type, BrandID: dist.BrandID},
		Filter: scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{dist.ID}, OrgID: dist.ID},
	}
	if _, err := x.UnitHistoryExportQuery(e.ctx, dv, a.Barcode); !errors.Is(err, stockusecase.ErrNotFound) {
		t.Fatalf("distributor history: %v", err)
	}
	if _, err := x.LocationStockExportQuery(e.ctx, dv.Filter, l1.view.UUID, nil); !errors.Is(err, stockusecase.ErrNotFound) {
		t.Fatalf("distributor location stock: %v", err)
	}

	// Barcode batches: center only; the selection keeps one batch.
	bc := stockusecase.NewBarcodes(e.pool, e.q)
	sc := stockusecase.Caller{Org: center.Org, Filter: center.Filter}
	batch, err := bc.Create(e.ctx, sc, stockusecase.BatchInput{ProductUUID: e.piece.Uuid.String(), Quantity: 3,
		Prefix: "E" + e.suffix[len(e.suffix)-7:]})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	q, err = x.BatchesExportQuery(sc, map[string]string{"uuid": batch.UUID.String()})
	if err != nil {
		t.Fatalf("batches query: %v", err)
	}
	ds = runStockExport(t, x, stockusecase.ResourceBatchesExport, q, e.center.ID)
	if len(ds.Rows) != 1 || ds.Rows[0]["first_barcode"] != batch.FirstBarcode || ds.Rows[0]["quantity"] != "3" {
		t.Fatalf("batches = %+v", ds.Rows)
	}
	if _, err := x.BatchesExportQuery(stockusecase.Caller{Org: dv.Org, Filter: dv.Filter}, nil); !errors.Is(err, stockusecase.ErrBarcodesCenterOnly) {
		t.Fatalf("distributor batches: %v", err)
	}
}
