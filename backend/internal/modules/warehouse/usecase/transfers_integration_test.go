package usecase_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/google/uuid"
)

// TEC-205 acceptance against a migrated PostgreSQL (TEST_DATABASE_URL; CI
// runs PG18): bin <-> bin move as one placement; warehouse -> warehouse
// transfer draft -> in_transit -> completed with transfer_out, transfer_in
// and placement; cancel after shipping restores with
// transfer_cancel_restore; a unit in transit (or on an open transfer)
// cannot be picked by another operation.

type site struct {
	w    wh.Warehouse
	a, b wh.Location // two aisles of one room
	aID  int64
	bID  int64
}

// site creates warehouse -> room -> two aisles for the center.
func (e *entryEnv) site(t *testing.T, code string) site {
	t.Helper()
	c := e.caller(e.center).Caller
	w, err := e.tree.CreateWarehouse(e.ctx, c, wh.WarehouseInput{Code: code + e.suffix[len(e.suffix)-6:], Name: "t205 " + code})
	if err != nil {
		t.Fatalf("warehouse: %v", err)
	}
	r, err := e.tree.CreateRoom(e.ctx, c, w.UUID, wh.RoomInput{Code: "R1", Name: "room"})
	if err != nil {
		t.Fatalf("room: %v", err)
	}
	s := site{w: w}
	for i, dst := range []*wh.Location{&s.a, &s.b} {
		l, err := e.tree.CreateLocation(e.ctx, c, wh.LocationInput{RoomUUID: r.UUID, Type: wh.TypeAisle, Code: fmt.Sprintf("A%d", i+1), Name: "aisle"})
		if err != nil {
			t.Fatalf("location: %v", err)
		}
		*dst = l
	}
	s.aID = e.locID(t, s.a.UUID)
	s.bID = e.locID(t, s.b.UUID)
	return s
}

func (e *entryEnv) locID(t *testing.T, id uuid.UUID) int64 {
	t.Helper()
	l, err := e.q.GetTypedLocationByUUID(e.ctx, db.GetTypedLocationByUUIDParams{Uuid: id, OrganizationID: e.center.ID})
	if err != nil {
		t.Fatal(err)
	}
	return l.ID
}

// stocked enters n new serial units at the center and places them on loc.
func (e *entryEnv) stocked(t *testing.T, w wh.Warehouse, loc wh.Location, n int, letter string) []wh.EntryLine {
	t.Helper()
	c := e.caller(e.center)
	entry, err := e.entries.Create(e.ctx, c, wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeGenerateNew})
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	if _, err = e.entries.AddLines(e.ctx, c, entry.UUID, wh.EntryLinesInput{ProductUUID: e.piece.Uuid.String(), Count: n, Prefix: e.prefix(letter)}); err != nil {
		t.Fatalf("entry lines: %v", err)
	}
	id := loc.UUID
	if _, err = e.entries.Place(e.ctx, c, entry.UUID, wh.PlaceInput{LocationUUID: &id}); err != nil {
		t.Fatalf("entry place: %v", err)
	}
	entry, err = e.entries.Confirm(e.ctx, c, entry.UUID)
	if err != nil {
		t.Fatalf("entry confirm: %v", err)
	}
	return entry.Lines
}

func (e *entryEnv) state(t *testing.T, unit uuid.UUID) (db.Unit, db.UnitCurrentState) {
	t.Helper()
	u, err := e.q.GetUnitByUUID(e.ctx, unit)
	if err != nil {
		t.Fatal(err)
	}
	st, err := e.q.GetUnitCurrentState(e.ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	return u, st
}

func types(mv []db.StockMovement) []string {
	out := make([]string, len(mv))
	for i, m := range mv {
		out[i] = m.Type
	}
	return out
}

func sameTypes(got []db.StockMovement, want ...string) bool {
	g := types(got)
	if len(g) != len(want) {
		return false
	}
	for i := range g {
		if g[i] != want[i] {
			return false
		}
	}
	return true
}

func isValidation(err error) bool {
	var ve *wh.ValidationError
	return errors.As(err, &ve)
}

// Bin -> bin inside one warehouse is one placement keyed by the unit's last
// movement; a move to another warehouse or onto the same bin is refused.
func TestWarehouseMoveBinToBin(t *testing.T) {
	e := newEntryEnv(t)
	tr := wh.NewWarehouseTransfers(e.pool, e.q, outbox.NewStore(e.pool, e.q))
	c := e.caller(e.center)
	s := e.site(t, "M")
	other := e.site(t, "N")
	lines := e.stocked(t, s.w, s.a, 2, "M")

	res, err := tr.Move(e.ctx, c, wh.MoveInput{
		Barcodes:     []string{"OFW:UNIT:" + lines[0].Barcode, lines[1].Barcode},
		LocationCode: "OFW:LOC:" + s.b.FullCode,
	})
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if res.Location.UUID != s.b.UUID || len(res.Units) != 2 {
		t.Fatalf("move result = %+v", res)
	}
	for _, mu := range res.Units {
		if mu.FromLocation == nil || mu.FromLocation.UUID != s.a.UUID || mu.MovementUUID == nil {
			t.Fatalf("moved unit = %+v", mu)
		}
	}
	for _, l := range lines {
		u, st := e.state(t, l.UnitUUID)
		if st.Status != "placed" || st.OwnerType != "warehouse_location" || st.OwnerID != s.bID {
			t.Fatalf("state after move = %+v", st)
		}
		mv := e.movements(t, u.ID)
		if !sameTypes(mv, "entry", "placement", "placement") {
			t.Fatalf("movements = %v", types(mv))
		}
		if mv[2].ToOwnerID.Int64 != s.bID {
			t.Fatalf("move target = %+v", mv[2])
		}
		var prev int64
		if err := e.pool.QueryRow(e.ctx, `SELECT id FROM stock_movements WHERE unit_id = $1 AND type = 'placement' ORDER BY id LIMIT 1`, u.ID).Scan(&prev); err != nil {
			t.Fatal(err)
		}
		if want := fmt.Sprintf("warehouse_move:stock_movement:%d:placement:%s", prev, u.Barcode); mv[2].IdempotencyKey != want {
			t.Fatalf("move key = %s, want %s", mv[2].IdempotencyKey, want)
		}
	}

	// Same bin again, another warehouse: refused, nothing written.
	if _, err := tr.Move(e.ctx, c, wh.MoveInput{Barcodes: []string{lines[0].Barcode}, LocationCode: s.b.FullCode}); !isValidation(err) {
		t.Fatalf("move onto the same bin: %v", err)
	}
	ol := other.a.UUID
	if _, err := tr.Move(e.ctx, c, wh.MoveInput{Barcodes: []string{lines[0].Barcode}, LocationUUID: &ol}); !isValidation(err) {
		t.Fatalf("move to another warehouse: %v", err)
	}
	u, _ := e.state(t, lines[0].UnitUUID)
	if n := len(e.movements(t, u.ID)); n != 3 {
		t.Fatalf("movements after refused moves = %d", n)
	}
	// Back to the first bin.
	back := s.a.UUID
	if _, err := tr.Move(e.ctx, c, wh.MoveInput{Barcodes: []string{lines[0].Barcode}, LocationUUID: &back}); err != nil {
		t.Fatalf("move back: %v", err)
	}
	if _, st := e.state(t, lines[0].UnitUUID); st.OwnerID != s.aID {
		t.Fatalf("state after move back = %+v", st)
	}
}

// Warehouse -> warehouse: draft -> in_transit (transfer_out) -> completed
// (transfer_in + placement). In transit the unit is locked for every other
// operation; a repeated step is refused and writes nothing.
func TestWarehouseTransferFullFlow(t *testing.T) {
	e := newEntryEnv(t)
	tr := wh.NewWarehouseTransfers(e.pool, e.q, outbox.NewStore(e.pool, e.q))
	c := e.caller(e.center)
	src := e.site(t, "S")
	dst := e.site(t, "D")
	lines := e.stocked(t, src.w, src.a, 2, "T")
	codes := []string{lines[0].Barcode, lines[1].Barcode}

	if _, err := tr.CreateTransfer(e.ctx, c, wh.TransferInput{FromWarehouseUUID: src.w.UUID.String(), ToWarehouseUUID: src.w.UUID.String()}); !isValidation(err) {
		t.Fatalf("same warehouse: %v", err)
	}
	doc, err := tr.CreateTransfer(e.ctx, c, wh.TransferInput{FromWarehouseUUID: src.w.UUID.String(), ToWarehouseUUID: dst.w.UUID.String()})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if doc.Status != wh.TransferStatusDraft || doc.TransferNo == "" || doc.FromWarehouse.UUID != src.w.UUID {
		t.Fatalf("draft = %+v", doc)
	}
	if _, err := tr.Ship(e.ctx, c, doc.UUID); !errors.Is(err, wh.ErrTransferEmpty) {
		t.Fatalf("ship empty: %v", err)
	}
	// A unit outside the source warehouse is refused.
	foreign := e.stocked(t, dst.w, dst.a, 1, "F")
	if _, err := tr.AddTransferLines(e.ctx, c, doc.UUID, []string{foreign[0].Barcode}); !errors.Is(err, wh.ErrUnitUnavailable) {
		t.Fatalf("unit outside the source warehouse: %v", err)
	}
	doc, err = tr.AddTransferLines(e.ctx, c, doc.UUID, codes)
	if err != nil {
		t.Fatalf("add lines: %v", err)
	}
	if len(doc.Lines) != 2 {
		t.Fatalf("lines = %+v", doc.Lines)
	}

	doc, err = tr.Ship(e.ctx, c, doc.UUID)
	if err != nil {
		t.Fatalf("ship: %v", err)
	}
	if doc.Status != wh.TransferStatusInTransit || doc.ShippedAt == nil {
		t.Fatalf("shipped = %+v", doc)
	}
	for _, l := range doc.Lines {
		if l.OutMovementUUID == nil || l.SourceLocation == nil || l.SourceLocation.UUID != src.a.UUID {
			t.Fatalf("shipped line = %+v", l)
		}
		_, st := e.state(t, l.UnitUUID)
		if st.Status != "in_transit" || st.OwnerType != "organization" || st.OwnerID != e.center.ID {
			t.Fatalf("in transit state = %+v", st)
		}
	}

	// In transit: not picked by a move, another transfer or a second ship.
	if _, err := tr.Move(e.ctx, c, wh.MoveInput{Barcodes: codes[:1], LocationCode: src.b.FullCode}); !errors.Is(err, wh.ErrUnitBusy) {
		t.Fatalf("move in transit: %v", err)
	}
	other, err := tr.CreateTransfer(e.ctx, c, wh.TransferInput{FromWarehouseUUID: src.w.UUID.String(), ToWarehouseUUID: dst.w.UUID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.AddTransferLines(e.ctx, c, other.UUID, codes[:1]); !errors.Is(err, wh.ErrUnitBusy) {
		t.Fatalf("add in-transit unit: %v", err)
	}
	if _, err := tr.Ship(e.ctx, c, doc.UUID); !errors.Is(err, wh.ErrTransferState) {
		t.Fatalf("second ship: %v", err)
	}
	if _, err := tr.AddTransferLines(e.ctx, c, doc.UUID, []string{foreign[0].Barcode}); !errors.Is(err, wh.ErrTransferState) {
		t.Fatalf("add to in-transit transfer: %v", err)
	}

	// No target location: refused, nothing written.
	if _, err := tr.Complete(e.ctx, c, doc.UUID, wh.CompleteInput{}); !errors.Is(err, wh.ErrTransferUnplaced) {
		t.Fatalf("complete unplaced: %v", err)
	}
	// A location of the source warehouse is not a target.
	if _, err := tr.Complete(e.ctx, c, doc.UUID, wh.CompleteInput{LocationCode: src.b.FullCode}); !isValidation(err) {
		t.Fatalf("complete into the source warehouse: %v", err)
	}
	// One line on bin b by scan, the other on the completion location a.
	if _, err := tr.PlaceTransferLines(e.ctx, c, doc.UUID, wh.PlaceInput{Barcodes: codes[1:], LocationCode: "OFW:LOC:" + dst.b.FullCode}); err != nil {
		t.Fatalf("place: %v", err)
	}
	doc, err = tr.Complete(e.ctx, c, doc.UUID, wh.CompleteInput{LocationCode: dst.a.FullCode})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if doc.Status != wh.TransferStatusCompleted || doc.CompletedAt == nil {
		t.Fatalf("completed = %+v", doc)
	}
	want := map[string]int64{codes[0]: dst.aID, codes[1]: dst.bID}
	var lineIDs = map[string]int64{}
	rows, err := e.pool.Query(e.ctx, `SELECT l.id, u.barcode, l.is_open FROM warehouse_transfer_lines l JOIN units u ON u.id = l.unit_id
		JOIN warehouse_transfers t ON t.id = l.transfer_id WHERE t.uuid = $1`, doc.UUID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		var bc string
		var open bool
		if err := rows.Scan(&id, &bc, &open); err != nil {
			t.Fatal(err)
		}
		if open {
			t.Fatalf("line %s still open", bc)
		}
		lineIDs[bc] = id
	}
	rows.Close()
	for _, l := range doc.Lines {
		if l.InMovementUUID == nil || l.PlacementMovementUUID == nil || l.TargetLocation == nil {
			t.Fatalf("completed line = %+v", l)
		}
		u, st := e.state(t, l.UnitUUID)
		if st.Status != "placed" || st.OwnerType != "warehouse_location" || st.OwnerID != want[u.Barcode] || u.Status != "placed" {
			t.Fatalf("state after complete of %s = %+v", u.Barcode, st)
		}
		mv := e.movements(t, u.ID)
		if !sameTypes(mv, "entry", "placement", "transfer_out", "transfer_in", "placement") {
			t.Fatalf("movements of %s = %v", u.Barcode, types(mv))
		}
		for i, typ := range []string{"transfer_out", "transfer_in", "placement"} {
			key := fmt.Sprintf("warehouse_transfer:warehouse_transfer_line:%d:%s:%s", lineIDs[u.Barcode], typ, u.Barcode)
			if mv[2+i].IdempotencyKey != key {
				t.Fatalf("key = %s, want %s", mv[2+i].IdempotencyKey, key)
			}
		}
	}
	// Completed: refused, nothing new; the lock is released.
	if _, err := tr.Complete(e.ctx, c, doc.UUID, wh.CompleteInput{}); !errors.Is(err, wh.ErrTransferState) {
		t.Fatalf("second complete: %v", err)
	}
	if _, err := tr.Cancel(e.ctx, c, doc.UUID); !errors.Is(err, wh.ErrTransferState) {
		t.Fatalf("cancel completed: %v", err)
	}
	back, err := tr.CreateTransfer(e.ctx, c, wh.TransferInput{FromWarehouseUUID: dst.w.UUID.String(), ToWarehouseUUID: src.w.UUID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.AddTransferLines(e.ctx, c, back.UUID, codes); err != nil {
		t.Fatalf("units free after completion: %v", err)
	}
	list, total, err := tr.ListTransfers(e.ctx, c, wh.TransferListFilter{Statuses: []string{wh.TransferStatusCompleted}, Limit: 50})
	if err != nil || total < 1 || len(list) < 1 || list[0].Lines != nil {
		t.Fatalf("list = %d %v", total, err)
	}
}

// Cancel after shipping restores every unit to its source bin with
// transfer_cancel_restore; a draft holds its units against other transfers
// and moves until it is cancelled.
func TestWarehouseTransferCancelRestore(t *testing.T) {
	e := newEntryEnv(t)
	tr := wh.NewWarehouseTransfers(e.pool, e.q, outbox.NewStore(e.pool, e.q))
	c := e.caller(e.center)
	src := e.site(t, "C")
	dst := e.site(t, "E")
	lines := e.stocked(t, src.w, src.b, 2, "C")
	codes := []string{lines[0].Barcode, lines[1].Barcode}
	in := wh.TransferInput{FromWarehouseUUID: src.w.UUID.String(), ToWarehouseUUID: dst.w.UUID.String()}

	// Draft lock.
	draft, err := tr.CreateTransfer(e.ctx, c, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.AddTransferLines(e.ctx, c, draft.UUID, codes); err != nil {
		t.Fatalf("add lines: %v", err)
	}
	second, err := tr.CreateTransfer(e.ctx, c, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.AddTransferLines(e.ctx, c, second.UUID, codes[:1]); !errors.Is(err, wh.ErrUnitBusy) {
		t.Fatalf("unit on an open draft: %v", err)
	}
	if _, err := tr.Move(e.ctx, c, wh.MoveInput{Barcodes: codes[:1], LocationCode: src.a.FullCode}); !errors.Is(err, wh.ErrUnitBusy) {
		t.Fatalf("move a unit on an open draft: %v", err)
	}
	if draft, err = tr.Cancel(e.ctx, c, draft.UUID); err != nil || draft.Status != wh.TransferStatusCancelled {
		t.Fatalf("cancel draft: %+v %v", draft, err)
	}
	u0, _ := e.state(t, lines[0].UnitUUID)
	if n := len(e.movements(t, u0.ID)); n != 2 {
		t.Fatalf("draft cancel wrote movements: %d", n)
	}

	// Ship, then cancel: restore.
	if _, err := tr.AddTransferLines(e.ctx, c, second.UUID, codes); err != nil {
		t.Fatalf("units free after draft cancel: %v", err)
	}
	if _, err := tr.Ship(e.ctx, c, second.UUID); err != nil {
		t.Fatalf("ship: %v", err)
	}
	doc, err := tr.Cancel(e.ctx, c, second.UUID)
	if err != nil {
		t.Fatalf("cancel in transit: %v", err)
	}
	if doc.Status != wh.TransferStatusCancelled || doc.CancelledAt == nil {
		t.Fatalf("cancelled = %+v", doc)
	}
	for _, l := range doc.Lines {
		if l.RestoreMovementUUID == nil || l.InMovementUUID != nil {
			t.Fatalf("restored line = %+v", l)
		}
		u, st := e.state(t, l.UnitUUID)
		if st.Status != "placed" || st.OwnerType != "warehouse_location" || st.OwnerID != src.bID || u.Status != "placed" {
			t.Fatalf("state after restore = %+v", st)
		}
		mv := e.movements(t, u.ID)
		if !sameTypes(mv, "entry", "placement", "transfer_out", "transfer_cancel_restore") {
			t.Fatalf("movements = %v", types(mv))
		}
		if mv[3].ToOwnerType.String != "warehouse_location" || mv[3].ToOwnerID.Int64 != src.bID {
			t.Fatalf("restore target = %+v", mv[3])
		}
	}
	if _, err := tr.Cancel(e.ctx, c, second.UUID); !errors.Is(err, wh.ErrTransferState) {
		t.Fatalf("second cancel: %v", err)
	}
	if _, err := tr.Complete(e.ctx, c, second.UUID, wh.CompleteInput{}); !errors.Is(err, wh.ErrTransferState) {
		t.Fatalf("complete cancelled: %v", err)
	}
	// Restored units move again.
	if _, err := tr.Move(e.ctx, c, wh.MoveInput{Barcodes: codes, LocationCode: src.a.FullCode}); err != nil {
		t.Fatalf("move after restore: %v", err)
	}
	// An order that does not exist for the buyer.
	if _, err := tr.PlaceOrder(e.ctx, c, uuid.New(), wh.MoveInput{LocationCode: src.a.FullCode}); !errors.Is(err, wh.ErrOrderNotFound) {
		t.Fatalf("place unknown order: %v", err)
	}
}
