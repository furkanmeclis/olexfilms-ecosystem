package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakePoster struct {
	sales []posting.Sale
	voids []posting.Source
	err   error
}

func (f *fakePoster) PostHierarchicalSaleTx(_ context.Context, _ pgx.Tx, s posting.Sale) (posting.SaleResult, error) {
	f.sales = append(f.sales, s)
	return posting.SaleResult{}, f.err
}

func (f *fakePoster) VoidBySourceTx(_ context.Context, _ pgx.Tx, src posting.Source, _ string, _ *int64) (posting.VoidResult, error) {
	f.voids = append(f.voids, src)
	return posting.VoidResult{}, f.err
}

func testOrder(t *testing.T, total string, snapshot string) db.Order {
	t.Helper()
	n, err := numeric(total)
	if err != nil {
		t.Fatal(err)
	}
	o := db.Order{
		Uuid: uuid.New(), OrderNo: "SIP-2026-000001", SellerOrgID: 1, BuyerOrgID: 2,
		Currency: "EUR", Total: n,
		ApprovedAt: pgtype.Timestamptz{Time: time.Date(2026, 9, 30, 15, 0, 0, 0, time.UTC), Valid: true},
	}
	if snapshot != "" {
		o.RateSnapshot = []byte(snapshot)
	}
	return o
}

func TestAccountingBridge_BooksOrderTotalAtFrozenRate(t *testing.T) {
	f := &fakePoster{}
	o := testOrder(t, "1250.5", `{"base":"EUR","quote":"TRY","rate":"35.1","rate_date":"2026-09-30","source":"tcmb"}`)
	actor := int64(7)
	if err := NewAccountingBridge(f).OrderReceived(context.Background(), nil, nil, o, &actor); err != nil {
		t.Fatal(err)
	}
	if len(f.sales) != 1 {
		t.Fatalf("sales = %d", len(f.sales))
	}
	s := f.sales[0]
	if s.Source != (posting.Source{Type: "order", UUID: o.Uuid}) || s.SellerOrgID != 1 || s.BuyerOrgID != 2 {
		t.Fatalf("sale parties = %+v", s)
	}
	if s.Amount != "1250.50" || s.Currency != "EUR" || s.ActorUserID == nil || *s.ActorUserID != 7 {
		t.Fatalf("sale amount = %+v", s)
	}
	if s.RateSnapshot == nil || s.RateSnapshot.Rate != "35.1" || s.RateSnapshot.Base != "EUR" || !s.RateDate.IsZero() {
		t.Fatalf("sale rate = %+v %v", s.RateSnapshot, s.RateDate)
	}
}

func TestAccountingBridge_NoSnapshotUsesApprovalDay(t *testing.T) {
	f := &fakePoster{}
	o := testOrder(t, "10", "")
	if err := NewAccountingBridge(f).OrderReceived(context.Background(), nil, nil, o, nil); err != nil {
		t.Fatal(err)
	}
	if s := f.sales[0]; s.RateSnapshot != nil || s.RateDate.Format(time.DateOnly) != "2026-09-30" {
		t.Fatalf("rate = %+v %v", s.RateSnapshot, s.RateDate)
	}
}

func TestAccountingBridge_ZeroTotalBooksNothing(t *testing.T) {
	f := &fakePoster{}
	if err := NewAccountingBridge(f).OrderReceived(context.Background(), nil, nil, testOrder(t, "0", ""), nil); err != nil {
		t.Fatal(err)
	}
	if len(f.sales) != 0 {
		t.Fatalf("sales = %d", len(f.sales))
	}
}

func TestAccountingBridge_ErrorsPropagate(t *testing.T) {
	boom := errors.New("boom")
	f := &fakePoster{err: boom}
	b := NewAccountingBridge(f)
	o := testOrder(t, "10", "")
	if err := b.OrderReceived(context.Background(), nil, nil, o, nil); !errors.Is(err, boom) {
		t.Fatalf("received err = %v", err)
	}
	if _, err := b.VoidOrder(context.Background(), nil, o, "return", nil); !errors.Is(err, boom) {
		t.Fatalf("void err = %v", err)
	}
	if len(f.voids) != 1 || f.voids[0] != AccountingSource(o) {
		t.Fatalf("voids = %+v", f.voids)
	}
}

func TestAccountingBridge_BadSnapshot(t *testing.T) {
	if _, _, err := SaleFor(testOrder(t, "10", "{"), nil); err == nil {
		t.Fatal("bad snapshot accepted")
	}
}
