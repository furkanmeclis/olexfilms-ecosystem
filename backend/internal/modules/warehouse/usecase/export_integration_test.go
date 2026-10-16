package usecase_test

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"testing"

	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// TEC-761 acceptance against a migrated PostgreSQL (TEST_DATABASE_URL):
// the warehouse exports keep the list filters and the row selection,
// print names instead of ids and translated enums, and the worker
// re-authorizes the stored scope against the job organization.

func exportAdapter(t *testing.T, x *wh.Exporter, resource string) ioengine.ResourceAdapter {
	t.Helper()
	for _, a := range x.Adapters() {
		if a.Resource() == resource {
			return a
		}
	}
	t.Fatalf("no adapter %s", resource)
	return nil
}

// runExport runs the worker side of a job of organization org.
func runExport(t *testing.T, x *wh.Exporter, resource string, q ioengine.ExportQuery, org int64) ioengine.Dataset {
	t.Helper()
	q[ioengine.QueryOrganizationID] = strconv.FormatInt(org, 10)
	ds, err := exportAdapter(t, x, resource).Export(t.Context(), q, i18n.LocaleTR)
	if err != nil {
		t.Fatalf("export %s: %v", resource, err)
	}
	return ds
}

func rowsBy(ds ioengine.Dataset, key string) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, r := range ds.Rows {
		if s, ok := r[key].(string); ok {
			out[s] = r
		}
	}
	return out
}

func infoValue(ds ioengine.Dataset, labelKey string) string {
	for _, l := range ds.Info {
		if l.LabelKey == labelKey {
			return l.Value
		}
	}
	return ""
}

func TestCountReportExport(t *testing.T) {
	e := newCountEnv(t)
	x := wh.NewExporter(e.pool, e.q)
	c := e.caller(true)
	w, _, locs := e.warehouse(t, "X", "A1", "A2")
	l1, l2 := locs[0], locs[1]
	a := e.unit(t, e.piece, "", &l1, 0)
	moved := e.unit(t, e.piece, "", &l2, 0)
	missing := e.unit(t, e.piece, "", &l1, 0)
	roll := e.unit(t, e.roll, "10", &l1, 0)
	fixed := e.unit(t, e.fixed, "", &l1, 10)

	cnt, err := e.counts.Create(e.ctx, c, wh.CountInput{WarehouseUUID: w.UUID.String(), Method: wh.CountMethodLocationFirst, Visibility: wh.CountVisibilityBlind})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := x.CountReportExportQuery(e.ctx, c, cnt.UUID); !errors.Is(err, wh.ErrCountStatus) {
		t.Fatalf("report export of an open count: %v", err)
	}
	if _, err = e.counts.Start(e.ctx, c, cnt.UUID); err != nil {
		t.Fatal(err)
	}
	meters, qty := "8.5", int32(7)
	for _, in := range []wh.CountScanInput{
		{Code: l1.qr()}, {Code: a.Barcode}, {Code: moved.Barcode}, {Code: roll.Barcode, Meters: &meters}, {Code: fixed.Barcode, Quantity: &qty},
	} {
		mustScan(t, e, c, cnt.UUID, in)
	}
	if _, err = e.counts.Complete(e.ctx, c, cnt.UUID); err != nil {
		t.Fatalf("complete: %v", err)
	}

	q, err := x.CountReportExportQuery(e.ctx, c, cnt.UUID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if q[ioengine.QueryScopeFilter] == "" {
		t.Fatalf("scope not stored: %v", q)
	}
	ds := runExport(t, x, wh.ResourceCountReportExport, q, e.center.ID)
	rows := rowsBy(ds, "barcode")
	if len(ds.Rows) != 5 {
		t.Fatalf("rows = %d: %+v", len(ds.Rows), ds.Rows)
	}
	if r := rows[moved.Barcode]; r["result"] != "Yanlış konum" || r["location"] != l1.view.FullCode || r["expected_location"] != l2.view.FullCode {
		t.Fatalf("moved row = %+v", r)
	}
	if r := rows[missing.Barcode]; r["result"] != "Eksik" || r["quantity_diff"] != "-1" {
		t.Fatalf("missing row = %+v", r)
	}
	if r := rows[roll.Barcode]; r["meters_diff"] != "-1.50" || r["counted_meters"] != "8.50" {
		t.Fatalf("roll row = %+v", r)
	}
	if r := rows[fixed.Barcode]; r["quantity_diff"] != "-3" || r["result"] != "Adet farkı" {
		t.Fatalf("fixed row = %+v", r)
	}
	if r := rows[a.Barcode]; r["result"] != "Eşleşti" || r["quantity_diff"] != "0" || r["product"] != e.piece.Name {
		t.Fatalf("matched row = %+v", r)
	}
	if got := infoValue(ds, "warehouse.export.method"); got != "Önce konum" {
		t.Fatalf("method info = %q", got)
	}
	if got := infoValue(ds, "warehouse.export.status"); got != "Onay bekliyor" {
		t.Fatalf("status info = %q", got)
	}
	if got := infoValue(ds, "warehouse.export.warehouse"); !strings.Contains(got, w.Name) {
		t.Fatalf("warehouse info = %q", got)
	}

	// CSV: UTF-8 BOM and Turkish headers; XLSX encodes.
	csv, err := ioengine.EncodeCSV(ds, "tr")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(csv, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("csv has no BOM")
	}
	head := strings.SplitN(string(csv[3:]), "\n", 2)[0]
	for _, want := range []string{"Barkod", "Beklenen adet", "Sayılan adet", "Adet farkı", "Metre farkı", "Sonuç", "Çözüm"} {
		if !strings.Contains(head, want) {
			t.Fatalf("csv header %q misses %q", head, want)
		}
	}
	if b, err := ioengine.EncodeXLSX(ds, "tr", nil); err != nil || len(b) == 0 {
		t.Fatalf("xlsx: %v", err)
	}

	// Count list: the selected count only, with its line totals.
	lq, err := x.ListExportQuery(c.Caller, wh.ResourceCountsExport, map[string]string{wh.QueryExportUUIDs: cnt.UUID.String()})
	if err != nil {
		t.Fatal(err)
	}
	list := runExport(t, x, wh.ResourceCountsExport, lq, e.center.ID)
	if len(list.Rows) != 1 || list.Rows[0]["lines"] != "5" || list.Rows[0]["differences"] != "4" || list.Rows[0]["status"] != "Onay bekliyor" {
		t.Fatalf("count list = %+v", list.Rows)
	}
}

func TestWarehouseListExports(t *testing.T) {
	e := newEntryEnv(t)
	x := wh.NewExporter(e.pool, e.q)
	tr := wh.NewWarehouseTransfers(e.pool, e.q, outbox.NewStore(e.pool, e.q))
	c := e.caller(e.center)
	src := e.site(t, "XS")
	dst := e.site(t, "XD")
	lines := e.stocked(t, src.w, src.a, 2, "X")

	// Stock entries: the selected entry, one row per line.
	entries, _, err := e.entries.List(e.ctx, c, wh.EntryListFilter{Limit: 1, WarehouseUUIDs: []uuid.UUID{src.w.UUID}})
	if err != nil || len(entries) == 0 {
		t.Fatalf("entries: %v", err)
	}
	entry := entries[0]
	q, err := x.ListExportQuery(c.Caller, wh.ResourceEntriesExport, map[string]string{wh.QueryExportUUIDs: entry.UUID.String()})
	if err != nil {
		t.Fatal(err)
	}
	ds := runExport(t, x, wh.ResourceEntriesExport, q, e.center.ID)
	byBarcode := rowsBy(ds, "barcode")
	if len(ds.Rows) != 2 || len(byBarcode) != 2 {
		t.Fatalf("entry rows = %+v", ds.Rows)
	}
	r := byBarcode[lines[0].Barcode]
	if r["status"] != "Onaylandı" || r["mode"] != "Yeni barkod" || r["placed"] != true || r["location"] != src.a.FullCode ||
		r["quantity"] != "1" || !strings.Contains(r["warehouse"].(string), src.w.Name) {
		t.Fatalf("entry row = %+v", r)
	}
	// The list filters stay: a draft-only export drops the confirmed entry.
	q, _ = x.ListExportQuery(c.Caller, wh.ResourceEntriesExport, map[string]string{
		wh.QueryExportUUIDs: entry.UUID.String(), "status": wh.EntryStatusDraft,
	})
	if ds := runExport(t, x, wh.ResourceEntriesExport, q, e.center.ID); len(ds.Rows) != 0 {
		t.Fatalf("draft filter kept %d rows", len(ds.Rows))
	}
	// A bad filter is refused at request time.
	var qe *apiquery.ValidationError
	if _, err := x.ListExportQuery(c.Caller, wh.ResourceEntriesExport, map[string]string{"status": "bogus"}); !errors.As(err, &qe) {
		t.Fatalf("bad status: %v", err)
	}
	if _, err := x.ListExportQuery(c.Caller, wh.ResourceTransfersExport, map[string]string{wh.QueryExportUUIDs: "nope"}); !errors.As(err, &qe) {
		t.Fatalf("bad selection: %v", err)
	}

	// Transfers: shipped document, one row per line, translated states.
	doc, err := tr.CreateTransfer(e.ctx, c, wh.TransferInput{FromWarehouseUUID: src.w.UUID.String(), ToWarehouseUUID: dst.w.UUID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tr.AddTransferLines(e.ctx, c, doc.UUID, []string{lines[0].Barcode, lines[1].Barcode}); err != nil {
		t.Fatal(err)
	}
	if _, err = tr.Ship(e.ctx, c, doc.UUID); err != nil {
		t.Fatal(err)
	}
	q, _ = x.ListExportQuery(c.Caller, wh.ResourceTransfersExport, map[string]string{wh.QueryExportUUIDs: doc.UUID.String()})
	ds = runExport(t, x, wh.ResourceTransfersExport, q, e.center.ID)
	if len(ds.Rows) != 2 {
		t.Fatalf("transfer rows = %+v", ds.Rows)
	}
	for _, r := range ds.Rows {
		if r["status"] != "Yolda" || r["line_status"] != "Yolda" || r["transfer_no"] != doc.TransferNo ||
			r["source_location"] != src.a.FullCode || !strings.Contains(r["to_warehouse"].(string), dst.w.Name) {
			t.Fatalf("transfer row = %+v", r)
		}
	}

	// Transfer summary: per product with the product and warehouse filters.
	q, err = x.ListExportQuery(c.Caller, wh.ResourceTransferSummaryExport, map[string]string{
		"product_uuid": e.piece.Uuid.String(), "from_warehouse_uuid": src.w.UUID.String(), wh.QueryExportUUIDs: doc.UUID.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	ds = runExport(t, x, wh.ResourceTransferSummaryExport, q, e.center.ID)
	if len(ds.Rows) != 1 {
		t.Fatalf("summary rows = %+v", ds.Rows)
	}
	if r := ds.Rows[0]; r["sku"] != e.piece.Sku || r["transfer_count"] != "1" || r["unit_count"] != "2" ||
		r["shipped"] != "2" || r["delivered"] != "0" || r["in_transit"] != "2" {
		t.Fatalf("summary row = %+v", r)
	}
	if got := infoValue(ds, "warehouse.export.products"); !strings.Contains(got, e.piece.Name) {
		t.Fatalf("summary product info = %q", got)
	}
	q, _ = x.ListExportQuery(c.Caller, wh.ResourceTransferSummaryExport, map[string]string{
		"product_uuid": e.fixed.Uuid.String(), wh.QueryExportUUIDs: doc.UUID.String(),
	})
	if ds := runExport(t, x, wh.ResourceTransferSummaryExport, q, e.center.ID); len(ds.Rows) != 0 {
		t.Fatalf("other product kept %d rows", len(ds.Rows))
	}

	// Locations: the source warehouse's aisles with their QR payload.
	q, _ = x.ListExportQuery(c.Caller, wh.ResourceLocationsExport, map[string]string{"warehouse_uuid": src.w.UUID.String()})
	ds = runExport(t, x, wh.ResourceLocationsExport, q, e.center.ID)
	locs := rowsBy(ds, "full_code")
	if len(ds.Rows) != 2 {
		t.Fatalf("location rows = %+v", ds.Rows)
	}
	if r := locs[src.a.FullCode]; r["qr"] != "OFW:LOC:"+src.a.FullCode || r["aisle"] != "A1" || r["type"] != "Koridor" || r["active"] != true {
		t.Fatalf("location row = %+v", r)
	}

	// The module guard: a dealer gets nothing.
	dealer := c.Caller
	dealer.Org.OrgType = rbac.OrgTypeDealer
	if _, err := x.ListExportQuery(dealer, wh.ResourceEntriesExport, nil); !errors.Is(err, wh.ErrForbidden) {
		t.Fatalf("dealer export: %v", err)
	}
	// The worker re-authorizes the stored scope against the job
	// organization: the center's scope is not the distributor's.
	q, _ = x.ListExportQuery(c.Caller, wh.ResourceEntriesExport, map[string]string{wh.QueryExportUUIDs: entry.UUID.String()})
	q[ioengine.QueryOrganizationID] = strconv.FormatInt(e.dist.ID, 10)
	if _, err := exportAdapter(t, x, wh.ResourceEntriesExport).Export(e.ctx, q, i18n.LocaleTR); !errors.Is(err, ioengine.ErrJobScope) {
		t.Fatalf("distributor job with the center scope: %v", err)
	}
}
