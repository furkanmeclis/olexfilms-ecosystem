package usecase_test

import (
	"errors"
	"testing"

	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
)

// TEC-1227 acceptance: the target warehouse completes a transfer with one
// of three units missing and a surplus barcode. The missing unit gets no
// transfer_in, stays in transit and keeps its lock under the record; the
// arrived units are placed. Later one missing unit is found (transfer_in +
// placement) and, on a second transfer, another is returned to its source
// (transfer_cancel_restore). Surplus rows are closed only.
func TestWarehouseTransferCompleteWithDiscrepancies(t *testing.T) {
	e := newEntryEnv(t)
	tr := wh.NewWarehouseTransfers(e.pool, e.q, outbox.NewStore(e.pool, e.q))
	c := e.caller(e.center)
	src := e.site(t, "M")
	dst := e.site(t, "N")
	lines := e.stocked(t, src.w, src.a, 4, "K")
	codes := []string{lines[0].Barcode, lines[1].Barcode, lines[2].Barcode}

	doc, err := tr.CreateTransfer(e.ctx, c, wh.TransferInput{FromWarehouseUUID: src.w.UUID.String(), ToWarehouseUUID: dst.w.UUID.String(), ToLocationUUID: &dst.a.UUID})
	if err != nil {
		t.Fatal(err)
	}
	if doc, err = tr.AddTransferLines(e.ctx, c, doc.UUID, codes); err != nil {
		t.Fatal(err)
	}
	if doc, err = tr.Ship(e.ctx, c, doc.UUID); err != nil {
		t.Fatal(err)
	}
	byCode := map[string]wh.TransferLine{}
	for _, l := range doc.Lines {
		byCode[l.Barcode] = l
	}
	// Every line missing is refused; a surplus barcode on the transfer too.
	all := []wh.TransferMissingInput{{LineUUID: byCode[codes[0]].UUID}, {LineUUID: byCode[codes[1]].UUID}, {LineUUID: byCode[codes[2]].UUID}}
	if _, err := tr.Complete(e.ctx, c, doc.UUID, wh.CompleteInput{Missing: all}); !isValidation(err) {
		t.Fatalf("all missing: %v", err)
	}
	if _, err := tr.Complete(e.ctx, c, doc.UUID, wh.CompleteInput{Surplus: []wh.TransferSurplusInput{{Barcode: codes[0]}}}); !isValidation(err) {
		t.Fatalf("surplus on transfer: %v", err)
	}

	doc, err = tr.Complete(e.ctx, c, doc.UUID, wh.CompleteInput{
		Missing: []wh.TransferMissingInput{{LineUUID: byCode[codes[0]].UUID, Note: "koli eksik"}, {LineUUID: byCode[codes[1]].UUID}},
		Surplus: []wh.TransferSurplusInput{{Barcode: lines[3].Barcode, Note: "fazla geldi"}, {Barcode: "UNKNOWN-1227"}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if doc.Status != wh.TransferStatusCompleted || doc.OpenDiscrepancies != 4 || len(doc.Discrepancies) != 4 {
		t.Fatalf("completed = %s open %d rows %d", doc.Status, doc.OpenDiscrepancies, len(doc.Discrepancies))
	}
	if _, st := e.state(t, byCode[codes[2]].UnitUUID); st.Status != "placed" || st.OwnerID != dst.aID {
		t.Fatalf("arrived unit = %+v", st)
	}
	_, st := e.state(t, byCode[codes[0]].UnitUUID)
	if st.Status != "in_transit" {
		t.Fatalf("missing unit = %+v", st)
	}
	// The missing unit keeps its lock.
	other, err := tr.CreateTransfer(e.ctx, c, wh.TransferInput{FromWarehouseUUID: src.w.UUID.String(), ToWarehouseUUID: dst.w.UUID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.AddTransferLines(e.ctx, c, other.UUID, codes[:1]); !errors.Is(err, wh.ErrUnitBusy) {
		t.Fatalf("missing unit picked again: %v", err)
	}
	rows := map[string]wh.TransferDiscrepancy{}
	for _, d := range doc.Discrepancies {
		rows[d.Barcode] = d
	}
	if rows[codes[0]].Kind != "missing" || rows[codes[0]].Note == nil || *rows[codes[0]].Note != "koli eksik" ||
		rows[lines[3].Barcode].Kind != "surplus" || rows[lines[3].Barcode].UnitUUID == nil || rows["UNKNOWN-1227"].UnitUUID != nil {
		t.Fatalf("rows = %+v", rows)
	}

	// Resolutions: a surplus row is only closed; a missing row is not closed.
	if _, err := tr.ResolveTransferDiscrepancy(e.ctx, c, doc.UUID, rows["UNKNOWN-1227"].UUID, wh.ResolveTransferDiscrepancyInput{Resolution: "found"}); !isValidation(err) {
		t.Fatalf("surplus found: %v", err)
	}
	if _, err := tr.ResolveTransferDiscrepancy(e.ctx, c, doc.UUID, rows[codes[0]].UUID, wh.ResolveTransferDiscrepancyInput{Resolution: "closed"}); !isValidation(err) {
		t.Fatalf("missing closed: %v", err)
	}
	if doc, err = tr.ResolveTransferDiscrepancy(e.ctx, c, doc.UUID, rows["UNKNOWN-1227"].UUID, wh.ResolveTransferDiscrepancyInput{Resolution: "closed"}); err != nil {
		t.Fatalf("surplus closed: %v", err)
	}
	// Found: transfer_in + placement onto bin b of the target.
	if doc, err = tr.ResolveTransferDiscrepancy(e.ctx, c, doc.UUID, rows[codes[0]].UUID, wh.ResolveTransferDiscrepancyInput{
		Resolution: "found", LocationUUID: &dst.b.UUID,
	}); err != nil {
		t.Fatalf("found: %v", err)
	}
	if _, st := e.state(t, byCode[codes[0]].UnitUUID); st.Status != "placed" || st.OwnerID != dst.bID {
		t.Fatalf("found unit = %+v", st)
	}
	// Returned to source: transfer_cancel_restore back to bin a of the source.
	if doc, err = tr.ResolveTransferDiscrepancy(e.ctx, c, doc.UUID, rows[codes[1]].UUID, wh.ResolveTransferDiscrepancyInput{
		Resolution: "returned_to_source",
	}); err != nil {
		t.Fatalf("returned: %v", err)
	}
	if _, st := e.state(t, byCode[codes[1]].UnitUUID); st.Status != "placed" || st.OwnerID != src.aID {
		t.Fatalf("returned unit = %+v", st)
	}
	if _, err := tr.ResolveTransferDiscrepancy(e.ctx, c, doc.UUID, rows[codes[1]].UUID, wh.ResolveTransferDiscrepancyInput{Resolution: "found"}); !errors.Is(err, wh.ErrTransferDiscrepancyClosed) {
		t.Fatalf("second resolve: %v", err)
	}
	if doc.OpenDiscrepancies != 1 {
		t.Fatalf("open = %d", doc.OpenDiscrepancies)
	}
	// The unit is free again once resolved.
	if _, err := tr.AddTransferLines(e.ctx, c, other.UUID, codes[1:2]); err != nil {
		t.Fatalf("returned unit on a new transfer: %v", err)
	}
}
