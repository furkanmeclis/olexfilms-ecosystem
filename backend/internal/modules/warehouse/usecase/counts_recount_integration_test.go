package usecase_test

import (
	"errors"
	"testing"

	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
)

// TEC-1229: a missing unit and a fixed quantity difference go back for a
// recount; the recount finds the unit (matched) and confirms the fixed
// difference; a partial approval resolves the fixed line and the count is
// approved only when nothing is open.
func TestStockCountRecountAndPartialApproval(t *testing.T) {
	e := newCountEnv(t)
	c := e.caller(true)
	w, _, locs := e.warehouse(t, "Q", "A1")
	l1 := locs[0]
	found := e.unit(t, e.piece, "", &l1, 0)
	lost := e.unit(t, e.piece, "", &l1, 0)
	fixed := e.unit(t, e.fixed, "", &l1, 10)

	cnt, err := e.counts.Create(e.ctx, c, wh.CountInput{WarehouseUUID: w.UUID.String(), Method: wh.CountMethodLocationFirst, Visibility: wh.CountVisibilityBlind})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.counts.Start(e.ctx, c, cnt.UUID); err != nil {
		t.Fatal(err)
	}
	qty := int32(8)
	mustScan(t, e, c, cnt.UUID, wh.CountScanInput{Code: l1.qr()})
	mustScan(t, e, c, cnt.UUID, wh.CountScanInput{Code: found.Barcode})
	mustScan(t, e, c, cnt.UUID, wh.CountScanInput{Code: fixed.Barcode, Quantity: &qty})
	if _, err := e.counts.Complete(e.ctx, c, cnt.UUID); err != nil {
		t.Fatal(err)
	}
	approver := e.userCaller(t)
	rep, err := e.counts.Report(e.ctx, approver, cnt.UUID)
	if err != nil {
		t.Fatal(err)
	}
	lostLine, fixedLine := lineFor(t, rep, lost.Barcode), lineFor(t, rep, fixed.Barcode)
	if lostLine.Result != wh.CountResultMissing || fixedLine.Result != wh.CountResultQtyVariance {
		t.Fatalf("lines = %+v / %+v", lostLine, fixedLine)
	}
	// A matched line cannot be recounted.
	if _, err := e.counts.RequestRecount(e.ctx, approver, cnt.UUID, []string{lineFor(t, rep, found.Barcode).UUID.String()}); err == nil {
		t.Fatal("recount of a matched line accepted")
	}
	rep, err = e.counts.RequestRecount(e.ctx, approver, cnt.UUID, []string{lostLine.UUID.String(), fixedLine.UUID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if l := lineFor(t, rep, lost.Barcode); l.RecountStatus == nil || *l.RecountStatus != wh.CountRecountRequested || len(l.AllowedResolutions) != 0 {
		t.Fatalf("requested line = %+v", l)
	}
	// A line waiting for a recount cannot be resolved.
	if _, err := e.counts.ApprovePartial(e.ctx, approver, cnt.UUID, []wh.CountResolution{{LineUUID: lostLine.UUID.String(), Resolution: wh.CountResolveVoidMissing}}); err == nil {
		t.Fatal("resolution of a requested line accepted")
	}
	// Recount: the lost unit is on A1 after all; the fixed barcode is 8.
	loc := l1.view.UUID.String()
	if _, err := e.counts.Recount(e.ctx, c, cnt.UUID, lostLine.UUID, wh.CountRecountInput{CountedQuantity: 1, LocationUUID: &loc}); err != nil {
		t.Fatal(err)
	}
	rep, err = e.counts.Recount(e.ctx, c, cnt.UUID, fixedLine.UUID, wh.CountRecountInput{CountedQuantity: 8})
	if err != nil {
		t.Fatal(err)
	}
	l := lineFor(t, rep, lost.Barcode)
	if l.Result != wh.CountResultMatched || l.RecountRounds != 1 || l.First == nil || l.First.Result != wh.CountResultMissing {
		t.Fatalf("recounted lost line = %+v", l)
	}
	if _, err := e.counts.Recount(e.ctx, c, cnt.UUID, lostLine.UUID, wh.CountRecountInput{CountedQuantity: 1, LocationUUID: &loc}); !errors.Is(err, wh.ErrCountStatus) {
		t.Fatalf("second recount without request: %v", err)
	}
	if l := lineFor(t, rep, fixed.Barcode); l.Result != wh.CountResultQtyVariance || l.CountedQuantity != 8 {
		t.Fatalf("recounted fixed line = %+v", l)
	}
	// Partial approval: the fixed line is ignored -> nothing open -> approved.
	rep, err = e.counts.ApprovePartial(e.ctx, approver, cnt.UUID, []wh.CountResolution{{LineUUID: fixedLine.UUID.String(), Resolution: wh.CountResolveIgnore}})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Count.Status != wh.CountStatusApproved {
		t.Fatalf("status after the last open line = %s", rep.Count.Status)
	}
	if _, _, st := e.state(t, lost); st != "placed" {
		t.Fatalf("lost unit status = %s", st)
	}
}

// A partial approval leaves the count pending while a difference is open.
func TestStockCountPartialApprovalKeepsPending(t *testing.T) {
	e := newCountEnv(t)
	c := e.caller(true)
	w, _, locs := e.warehouse(t, "P", "A1")
	l1 := locs[0]
	kept := e.unit(t, e.piece, "", &l1, 0)
	lost := e.unit(t, e.piece, "", &l1, 0)
	fixed := e.unit(t, e.fixed, "", &l1, 5)
	cnt, err := e.counts.Create(e.ctx, c, wh.CountInput{WarehouseUUID: w.UUID.String(), Method: wh.CountMethodLocationFirst, Visibility: wh.CountVisibilityBlind})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.counts.Start(e.ctx, c, cnt.UUID); err != nil {
		t.Fatal(err)
	}
	qty := int32(4)
	mustScan(t, e, c, cnt.UUID, wh.CountScanInput{Code: l1.qr()})
	mustScan(t, e, c, cnt.UUID, wh.CountScanInput{Code: kept.Barcode})
	mustScan(t, e, c, cnt.UUID, wh.CountScanInput{Code: fixed.Barcode, Quantity: &qty})
	if _, err := e.counts.Complete(e.ctx, c, cnt.UUID); err != nil {
		t.Fatal(err)
	}
	approver := e.userCaller(t)
	rep, err := e.counts.Report(e.ctx, approver, cnt.UUID)
	if err != nil {
		t.Fatal(err)
	}
	fl := lineFor(t, rep, fixed.Barcode)
	rep, err = e.counts.ApprovePartial(e.ctx, approver, cnt.UUID, []wh.CountResolution{{LineUUID: fl.UUID.String(), Resolution: wh.CountResolveIgnore}})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Count.Status != wh.CountStatusPendingReview {
		t.Fatalf("status with an open line = %s", rep.Count.Status)
	}
	if _, err := e.counts.ApprovePartial(e.ctx, approver, cnt.UUID, []wh.CountResolution{{LineUUID: fl.UUID.String(), Resolution: wh.CountResolveIgnore}}); err == nil {
		t.Fatal("resolved line resolved again")
	}
	// The full approval still needs the open line.
	if _, err := e.counts.Approve(e.ctx, approver, cnt.UUID, nil); err == nil {
		t.Fatal("approval with an open line accepted")
	}
	rep, err = e.counts.Approve(e.ctx, approver, cnt.UUID, []wh.CountResolution{{LineUUID: lineFor(t, rep, lost.Barcode).UUID.String(), Resolution: wh.CountResolveIgnore}})
	if err != nil || rep.Count.Status != wh.CountStatusApproved {
		t.Fatalf("final approval: %+v %v", rep.Count.Status, err)
	}
}
