package usecase_test

import (
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	acc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// cariOf returns the uuid of book's cari with counterparty.
func (e *disputeEnv) cariOf(t *testing.T, book, counterparty db.Organization) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.pool.QueryRow(e.ctx,
		`SELECT uuid FROM cari_accounts WHERE organization_id = $1 AND counterparty_org_id = $2`,
		book.ID, counterparty.ID).Scan(&id); err != nil {
		t.Fatalf("cari: %v", err)
	}
	return id
}

// TEC-1070: billing details of a cari organization, filled by the book above.
func TestCariBilling(t *testing.T) {
	e := newDisputeEnv(t)
	e.sale(t, "100.00")
	distCari := e.cariOf(t, e.dist, e.dealer)
	dealerCari := e.cariOf(t, e.dealer, e.dist)
	seller := caller(e.dist, rbac.PermAccountingWrite)

	got, err := e.svc.GetCariBilling(e.ctx, caller(e.dist, rbac.PermAccountingRead), nil, distCari)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Editable || got.Title != nil || got.IBAN != nil {
		t.Fatalf("empty profile = %+v", got)
	}

	var ve *acc.ValidationError
	for _, bad := range []acc.BillingInput{
		{IBAN: "TR00 0000 0000 0000 0000 0000 00"},
		{Email: "not-an-email"},
		{TaxNo: "12 34"},
	} {
		if _, err := e.svc.UpdateCariBilling(e.ctx, seller, distCari, bad); !errors.As(err, &ve) {
			t.Fatalf("%+v: err = %v, want a validation error", bad, err)
		}
	}

	got, err = e.svc.UpdateCariBilling(e.ctx, seller, distCari, acc.BillingInput{
		Title: " Bayi Ltd. Şti. ", TaxOffice: "Kadıköy", TaxNo: "1234567890",
		Address: "Moda Cd. 1, İstanbul", Email: "muhasebe@bayi.example",
		IBAN: "tr33 0006 1005 1978 6457 8413 26",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title == nil || *got.Title != "Bayi Ltd. Şti." || got.IBAN == nil || *got.IBAN != "TR330006100519786457841326" ||
		got.UpdatedAt == nil {
		t.Fatalf("saved profile = %+v", got)
	}

	// The dealer reads its supplier's cari but cannot edit the supplier.
	read, err := e.svc.GetCariBilling(e.ctx, caller(e.dealer, rbac.PermAccountingRead), nil, dealerCari)
	if err != nil {
		t.Fatal(err)
	}
	if read.Editable {
		t.Fatal("the dealer must not edit its supplier's billing details")
	}
	if _, err := e.svc.UpdateCariBilling(e.ctx, caller(e.dealer, rbac.PermAccountingWrite), dealerCari,
		acc.BillingInput{Title: "x"}); !errors.Is(err, acc.ErrBillingNotEditable) && !errors.Is(err, acc.ErrForbidden) {
		t.Fatalf("dealer edit: err = %v", err)
	}
	// Clearing a field.
	got, err = e.svc.UpdateCariBilling(e.ctx, seller, distCari, acc.BillingInput{Title: "Bayi Ltd."})
	if err != nil {
		t.Fatal(err)
	}
	if got.IBAN != nil || got.TaxNo != nil {
		t.Fatalf("cleared profile = %+v", got)
	}
}

// TEC-1070: a received return waits in the receiver's queue until processed.
func TestQueueReturns(t *testing.T) {
	e := newDisputeEnv(t)
	var retUUID uuid.UUID
	if err := e.pool.QueryRow(e.ctx, `
		INSERT INTO stock_transfer_requests
		    (organization_id, brand_id, from_org_id, to_org_id, approver_org_id, kind, transfer_no,
		     currency, status, decided_at, shipped_at, received_at, total)
		VALUES ($1, $2, $1, $3, $3, 'return', $4, 'TRY', 'received', NOW(), NOW(), NOW(), 50)
		RETURNING uuid`,
		e.dealer.ID, e.brand, e.dist.ID, "RT-"+e.suffix).Scan(&retUUID); err != nil {
		t.Fatalf("return: %v", err)
	}
	reader := caller(e.dist, rbac.PermAccountingRead)
	writer := caller(e.dist, rbac.PermAccountingWrite)

	sum, err := e.svc.QueueSummary(e.ctx, reader)
	if err != nil {
		t.Fatal(err)
	}
	if sum.ReturnsPending != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	waiting := false
	items, total, err := e.svc.ListQueueReturns(e.ctx, reader, acc.QueueReturnFilter{Processed: &waiting, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].UUID != retUUID || items[0].Sender.Name != "dealer" ||
		items[0].Total == nil || *items[0].Total != "50.00" {
		t.Fatalf("queue = %d %+v", total, items)
	}

	// Another book does not reach it.
	if err := e.svc.MarkReturnProcessed(e.ctx, caller(e.other, rbac.PermAccountingWrite), retUUID, true, ""); !errors.Is(err, acc.ErrReturnNotFound) {
		t.Fatalf("other book: err = %v", err)
	}
	if err := e.svc.MarkReturnProcessed(e.ctx, writer, retUUID, true, "İade faturası IF-1"); err != nil {
		t.Fatal(err)
	}
	if sum, _ = e.svc.QueueSummary(e.ctx, reader); sum.ReturnsPending != 0 {
		t.Fatalf("summary after mark = %+v", sum)
	}
	done := true
	items, _, err = e.svc.ListQueueReturns(e.ctx, reader, acc.QueueReturnFilter{Processed: &done, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ProcessedAt == nil || items[0].Note == nil || *items[0].Note != "İade faturası IF-1" {
		t.Fatalf("processed = %+v", items)
	}
	if err := e.svc.MarkReturnProcessed(e.ctx, writer, retUUID, false, ""); err != nil {
		t.Fatal(err)
	}
	if sum, _ = e.svc.QueueSummary(e.ctx, reader); sum.ReturnsPending != 1 {
		t.Fatalf("summary after undo = %+v", sum)
	}
}

func TestParseQueueReturnFilter(t *testing.T) {
	f, err := acc.ParseQueueReturnFilter(map[string][]string{"processed": {"true"}, "sort": {"-transfer_no"}})
	if err != nil || f.Processed == nil || !*f.Processed || f.Sort.Key != "transfer_no" || !f.Sort.Desc {
		t.Fatalf("filter = %+v, err = %v", f, err)
	}
	if _, err := acc.ParseQueueReturnFilter(map[string][]string{"processed": {"maybe"}}); err == nil {
		t.Fatal("processed=maybe must fail")
	}
}
