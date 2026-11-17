package usecase_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	stockmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	platstorage "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1225 acceptance against PostgreSQL: a container entry with a missing
// line, an extra, an unknown extra and a damaged line cannot be confirmed
// without decisions; with them the record is written, the missing line does
// not enter stock, the extra does, the damaged line needs a photo; the
// record PDF names the supplier; no DTO leaks a price. An undo is refused
// while a unit moved after the entry, and otherwise voids every line through
// the ledger with a clean rebuild check.

type fakeRenderer struct{ html string }

func (f *fakeRenderer) Convert(_ context.Context, req pdfrender.Request) ([]byte, error) {
	f.html = req.HTML
	return []byte("%PDF-1.7 fake"), nil
}

var pngBytes = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 13, 'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1, 8, 2, 0, 0, 0}

func exceptionCaller(t *testing.T, e *entryEnv, adjust bool) wh.EntryCaller {
	t.Helper()
	c := e.caller(e.center)
	user, err := e.q.CreateUser(e.ctx, db.CreateUserParams{
		Email:        pgtype.Text{String: "t1225-" + uuid.NewString() + "@example.test", Valid: true},
		PasswordHash: "x", Name: "Kabul", Surname: "Test", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	scopes := map[string]rbac.Scope{rbac.PermStockRead: rbac.ScopeAll}
	if adjust {
		scopes[rbac.PermStockAdjust] = rbac.ScopeAll
	}
	c.Principal = authctx.Principal{UserInternal: user.ID, PermissionScopes: scopes}
	return c
}

func noPriceKeys(t *testing.T, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	low := strings.ToLower(string(raw))
	for _, k := range []string{`"price`, `"unit_price`, `"total`, `"amount`, `"cost`, `"rate_snapshot`, `"currency`} {
		if strings.Contains(low, k) {
			t.Fatalf("DTO leaks %s: %s", k, raw)
		}
	}
}

func TestEntryDiscrepancyRecord(t *testing.T) {
	e := newEntryImportEnv(t)
	w, _ := e.location(t, e.center, "Q")
	bin := importBin(t, e, w)
	c := exceptionCaller(t, e, false)
	pdf := &fakeRenderer{}
	e.entries.WithFiles(platstorage.NewMemory(), pdf)

	sup, err := e.entries.CreateSupplier(e.ctx, c, wh.SupplierInput{Name: "Tedarikci " + e.suffix})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := e.bc.Create(e.ctx, e.stockCaller(e.center), stockusecase.BatchInput{ProductUUID: e.piece.Uuid.String(), Quantity: 4, Prefix: e.prefix("D")})
	if err != nil {
		t.Fatal(err)
	}
	a, b, miss, extra := batch.Units[0].Barcode, batch.Units[1].Barcode, batch.Units[2].Barcode, batch.Units[3].Barcode
	unknown := "UNKNOWN-" + e.suffix

	supUUID, container, binUUID := sup.UUID.String(), "MSCU"+e.suffix[:7], bin.UUID.String()
	entry, err := e.entries.Create(e.ctx, c, wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeWithExisting,
		Header: wh.EntryHeaderInput{SupplierUUID: &supUUID, ContainerNo: &container, DefaultLocationUUID: &binUUID}})
	if err != nil {
		t.Fatal(err)
	}
	// Manual lot and production date (item 3).
	entry, err = e.entries.AddLines(e.ctx, c, entry.UUID, wh.EntryLinesInput{Barcodes: []string{a, b, miss}, Lot: "LOT-9", ProductionDate: "2026-10-01"})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range entry.Lines {
		if l.Lot == nil || *l.Lot != "LOT-9" || l.ProductionDate == nil || *l.ProductionDate != "2026-10-01" || l.Location == nil {
			t.Fatalf("line details %+v", l)
		}
	}
	if _, err := e.entries.AddLines(e.ctx, c, entry.UUID, wh.EntryLinesInput{Barcodes: []string{extra}, ProductionDate: "2999-01-01"}); err == nil {
		t.Fatal("future production date accepted")
	}
	pd := ""
	if entry, err = e.entries.UpdateLine(e.ctx, c, entry.UUID, entry.Lines[0].UUID, wh.LineDetailsInput{ProductionDate: &pd}); err != nil || entry.Lines[0].ProductionDate != nil {
		t.Fatalf("clear production date: %v %+v", err, entry.Lines[0])
	}

	for _, code := range []string{a, strings.ToLower(b), extra, unknown} {
		if _, err := e.entries.Scan(e.ctx, c, entry.UUID, code); err != nil {
			t.Fatalf("scan %s: %v", code, err)
		}
	}

	// No decisions: the missing line and both extras are listed.
	_, err = e.entries.ConfirmWith(e.ctx, c, entry.UUID, wh.ConfirmInput{})
	var dr *wh.DiscrepancyRequiredError
	if !errors.As(err, &dr) || len(dr.Missing) != 1 || dr.Missing[0] != miss || len(dr.Extras) != 2 {
		t.Fatalf("confirm without decisions: %v %+v", err, dr)
	}

	photo, err := e.entries.AddAttachment(e.ctx, c, entry.UUID, wh.AttachmentInput{Kind: wh.AttachmentPhoto, Filename: "hasar.png", Body: bytes.NewReader(pngBytes), Size: int64(len(pngBytes))})
	if err != nil {
		t.Fatalf("photo: %v", err)
	}
	pdfDoc := []byte("%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF")
	waybill, err := e.entries.AddAttachment(e.ctx, c, entry.UUID, wh.AttachmentInput{Kind: wh.AttachmentWaybill, Filename: "irsaliye.pdf", Body: bytes.NewReader(pdfDoc), Size: int64(len(pdfDoc))})
	if err != nil || waybill.MimeType != "application/pdf" {
		t.Fatalf("waybill: %v %+v", err, waybill)
	}
	if _, err := e.entries.AddAttachment(e.ctx, c, entry.UUID, wh.AttachmentInput{Kind: wh.AttachmentPhoto, Filename: "x.txt", Body: strings.NewReader("hello"), Size: 5}); err == nil {
		t.Fatal("text accepted as attachment")
	}

	decisions := []wh.DiscrepancyDecision{
		{Barcode: miss, Kind: wh.DiscrepancyMissing, Decision: wh.DecisionLeftOut},
		{Barcode: extra, Kind: wh.DiscrepancyExtra, Decision: wh.DecisionAdded},
		{Barcode: unknown, Kind: wh.DiscrepancyExtra, Decision: wh.DecisionRejected},
		{Barcode: b, Kind: wh.DiscrepancyDamaged, Decision: wh.DecisionAccepted, Reason: "Koli ezik"},
	}
	// Damage needs a photo.
	if _, err := e.entries.ConfirmWith(e.ctx, c, entry.UUID, wh.ConfirmInput{Decisions: decisions}); err == nil {
		t.Fatal("damage without photo accepted")
	}
	decisions[3].AttachmentUUIDs = []uuid.UUID{waybill.UUID}
	if _, err := e.entries.ConfirmWith(e.ctx, c, entry.UUID, wh.ConfirmInput{Decisions: decisions}); err == nil {
		t.Fatal("damage with a PDF as photo accepted")
	}
	decisions[3].AttachmentUUIDs = []uuid.UUID{photo.UUID}
	// An unknown extra cannot be added.
	bad := append([]wh.DiscrepancyDecision{}, decisions...)
	bad[2].Decision = wh.DecisionAdded
	if _, err := e.entries.ConfirmWith(e.ctx, c, entry.UUID, wh.ConfirmInput{Decisions: bad}); err == nil {
		t.Fatal("unknown extra added")
	}

	got, err := e.entries.ConfirmWith(e.ctx, c, entry.UUID, wh.ConfirmInput{Decisions: decisions})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if got.Status != wh.EntryStatusConfirmed || len(got.Lines) != 3 || len(got.Discrepancies) != 4 || len(got.Attachments) != 2 {
		t.Fatalf("confirmed %+v", got)
	}
	seen := map[string]bool{}
	for _, l := range got.Lines {
		seen[l.Barcode] = true
		if l.UnitStatus != "placed" {
			t.Fatalf("line %s status %s", l.Barcode, l.UnitStatus)
		}
	}
	if !seen[a] || !seen[b] || !seen[extra] || seen[miss] {
		t.Fatalf("lines %+v", seen)
	}
	mu, err := e.q.GetUnitByBarcode(e.ctx, db.GetUnitByBarcodeParams{BrandID: e.brand.ID, Barcode: miss})
	if err != nil || (mu.Status != "reserved" && mu.Status != "printed") {
		t.Fatalf("missing unit entered stock: %s %v", mu.Status, err)
	}
	rec, err := e.entries.Receipt(e.ctx, c, entry.UUID)
	if err != nil {
		t.Fatal(err)
	}
	s := rec.Summary
	if s.Entered != 3 || s.Missing != 1 || s.ExtrasAdded != 1 || s.ExtrasRejected != 1 || s.DamagedAccepted != 1 || rec.Supplier == nil || rec.ConfirmedBy == nil {
		t.Fatalf("receipt %+v", rec)
	}
	for _, it := range rec.Items {
		if it.Kind == wh.DiscrepancyDamaged && (len(it.Attachments) != 1 || it.Reason == nil) {
			t.Fatalf("damage item %+v", it)
		}
	}
	noPriceKeys(t, rec)
	noPriceKeys(t, got)

	steps := map[string]bool{}
	for _, st := range got.Timeline {
		steps[st.Action] = true
		if st.Action == "scanned" && st.Details["count"] != float64(4) {
			t.Fatalf("collapsed scans %+v", st)
		}
	}
	for _, want := range []string{"created", "lines_added", "line_updated", "scanned", "attachment_added", "discrepancies_recorded", "confirmed"} {
		if !steps[want] {
			t.Fatalf("timeline misses %s: %+v", want, got.Timeline)
		}
	}

	if err := e.entries.DeleteAttachment(e.ctx, c, entry.UUID, photo.UUID); !errors.Is(err, wh.ErrEntryAttachmentInUse) {
		t.Fatalf("delete evidence: %v", err)
	}
	if err := e.entries.DeleteAttachment(e.ctx, c, entry.UUID, waybill.UUID); err != nil {
		t.Fatalf("delete waybill: %v", err)
	}
	rc, _, _, err := e.entries.AttachmentFile(e.ctx, c, entry.UUID, photo.UUID)
	if err != nil {
		t.Fatal(err)
	}
	_ = rc.Close()

	out, name, err := e.entries.ReceiptPDF(e.ctx, c, entry.UUID, "tr")
	if err != nil || len(out) == 0 || !strings.HasPrefix(name, "tutanak-") {
		t.Fatalf("pdf %v %s", err, name)
	}
	for _, want := range []string{sup.Name, container, "Mal kabul tutanağı", "Koli ezik", miss} {
		if !strings.Contains(pdf.html, want) {
			t.Fatalf("pdf html misses %q", want)
		}
	}
	if err := e.entries.RecordLabelPrint(e.ctx, c, entry.UUID, 3); err != nil {
		t.Fatal(err)
	}

	// The record is append-only.
	if _, err := e.pool.Exec(e.ctx, `UPDATE stock_entry_discrepancies SET reason = 'x' WHERE barcode = $1`, b); err == nil {
		t.Fatal("discrepancy record updated")
	}
}

func TestEntryUndo(t *testing.T) {
	e := newEntryImportEnv(t)
	w, loc := e.location(t, e.center, "U")
	_, other := e.location(t, e.center, "O")
	c := exceptionCaller(t, e, true)

	confirmEntry := func(prefix string, n int) (wh.StockEntry, []stockmodel.BarcodeBatchUnit) {
		t.Helper()
		batch, err := e.bc.Create(e.ctx, e.stockCaller(e.center), stockusecase.BatchInput{ProductUUID: e.piece.Uuid.String(), Quantity: n, Prefix: e.prefix(prefix)})
		if err != nil {
			t.Fatal(err)
		}
		codes := make([]string, 0, n)
		for _, u := range batch.Units {
			codes = append(codes, u.Barcode)
		}
		entry, err := e.entries.Create(e.ctx, c, wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeWithExisting})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.entries.AddLines(e.ctx, c, entry.UUID, wh.EntryLinesInput{Barcodes: codes}); err != nil {
			t.Fatal(err)
		}
		locID := loc.UUID
		if _, err := e.entries.Place(e.ctx, c, entry.UUID, wh.PlaceInput{LocationUUID: &locID}); err != nil {
			t.Fatal(err)
		}
		got, err := e.entries.Confirm(e.ctx, c, entry.UUID)
		if err != nil {
			t.Fatal(err)
		}
		return got, batch.Units
	}

	// A unit moved after the entry blocks the undo.
	moved, units := confirmEntry("M", 2)
	otherRow, err := e.q.GetTypedLocationByUUID(e.ctx, db.GetTypedLocationByUUIDParams{Uuid: other.UUID, OrganizationID: e.center.ID})
	if err != nil {
		t.Fatal(err)
	}
	u0, err := e.q.GetUnitByUUID(e.ctx, units[0].UUID)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := e.pool.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.New(e.q.WithTx(tx), nil).Post(e.ctx, tx, ledger.Movement{
		Type: ledger.TypePlacement, UnitID: u0.ID, Source: "test", RefType: "t1225", RefID: u0.ID,
		To: &ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: otherRow.ID, OrgID: e.center.ID},
	}); err != nil {
		_ = tx.Rollback(e.ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(e.ctx); err != nil {
		t.Fatal(err)
	}
	chk, err := e.entries.CheckUndo(e.ctx, c, moved.UUID)
	if err != nil || chk.Allowed || len(chk.Blockers) != 1 || chk.Blockers[0].Barcode != u0.Barcode || chk.Blockers[0].MovementType != "placement" {
		t.Fatalf("check %+v %v", chk, err)
	}
	noPriceKeys(t, chk)
	var ub *wh.UndoBlockedError
	if _, err := e.entries.Undo(e.ctx, c, moved.UUID, "yanlış giriş"); !errors.As(err, &ub) || len(ub.Blockers) != 1 {
		t.Fatalf("undo blocked: %v", err)
	}

	// Permission and reason.
	clean, cleanUnits := confirmEntry("N", 2)
	if _, err := e.entries.Undo(e.ctx, exceptionCaller(t, e, false), clean.UUID, "x"); !errors.Is(err, wh.ErrEntryAdjustForbidden) {
		t.Fatalf("undo without stock.adjust: %v", err)
	}
	if _, err := e.entries.Undo(e.ctx, c, clean.UUID, " "); err == nil {
		t.Fatal("undo without reason")
	}
	chk, err = e.entries.CheckUndo(e.ctx, c, clean.UUID)
	if err != nil || !chk.Allowed || chk.Lines != 2 {
		t.Fatalf("check clean %+v %v", chk, err)
	}
	undone, err := e.entries.Undo(e.ctx, c, clean.UUID, "yanlış depo")
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if undone.Status != wh.EntryStatusUndone || undone.UndoneAt == nil || undone.UndoReason == nil {
		t.Fatalf("undone %+v", undone)
	}
	barcodes := map[string]bool{}
	for _, l := range undone.Lines {
		if !l.Undone || l.UnitStatus != "void" {
			t.Fatalf("line %+v", l)
		}
		barcodes[l.Barcode] = true
	}
	for _, u := range cleanUnits {
		row, err := e.q.GetUnitByUUID(e.ctx, u.UUID)
		if err != nil {
			t.Fatal(err)
		}
		mv := e.movements(t, row.ID)
		if len(mv) != 3 || mv[2].Type != "void" || !strings.HasPrefix(mv[2].IdempotencyKey, "stock_entry_undo:") {
			t.Fatalf("movements %+v", mv)
		}
	}
	if _, err := e.entries.Undo(e.ctx, c, clean.UUID, "again"); !errors.Is(err, wh.ErrEntryNotConfirmed) {
		t.Fatalf("second undo: %v", err)
	}
	steps := map[string]bool{}
	for _, st := range undone.Timeline {
		steps[st.Action] = true
	}
	if !steps["undone"] {
		t.Fatalf("timeline %+v", undone.Timeline)
	}

	// Rebuild check: the undone units replay without drift.
	rep, err := rebuild.New(e.pool, e.q).Check(e.ctx, e.center.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range rep.Diffs {
		if barcodes[d.Barcode] {
			t.Fatalf("rebuild drift %+v", d)
		}
	}
	for _, a := range rep.Anomalies {
		for code := range barcodes {
			if strings.Contains(a, code) {
				t.Fatalf("rebuild anomaly %s", a)
			}
		}
	}
}
