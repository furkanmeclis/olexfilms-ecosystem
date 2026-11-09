package usecase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type invoicingOutbox struct{}

func (invoicingOutbox) Enqueue(context.Context, pgx.Tx, events.Event) error { return nil }

// invoicingFixture: a received center → dealer order with two lines
// (TEC-1030), everything inside one rolled-back transaction.
type invoicingFixture struct {
	ctx    context.Context
	tx     pgx.Tx
	svc    *Service
	center Caller
	dealer Caller
	order  uuid.UUID
	items  []db.OrderItem
}

func newInvoicingFixture(t *testing.T) *invoicingFixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	q := db.New(tx)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	dealer, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: "t1030-dealer-" + suffix, Name: "T1030 Bayi", Status: "active", Type: "dealer",
		Address: "Kordon No:5", City: "İzmir", District: "Konak",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		ParentID:       pgtype.Int8{Int64: center.ID, Valid: true}, BrandID: brand.ID,
		Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("dealer: %v", err)
	}
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "T1030", Surname: "Accounting", Status: "active",
		Email: pgtype.Text{String: "t1030-" + suffix + "@example.test", Valid: true},
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	category, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: center.ID, BrandID: brand.ID, Name: "t1030-cat-" + suffix,
		AvailableParts: []byte(`[]`), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	var orderID int64
	var orderUUID uuid.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO orders (
		order_no, organization_id, brand_id, seller_org_id, buyer_org_id, status, currency, subtotal, total, received_at, created_by_seller
	) VALUES ($1, $2, $3, $2, $4, 'received', 'TRY', 300, 300, NOW(), true) RETURNING id, uuid`,
		"T1030-"+suffix, center.ID, brand.ID, dealer.ID).Scan(&orderID, &orderUUID); err != nil {
		t.Fatalf("order: %v", err)
	}
	f := &invoicingFixture{ctx: ctx, tx: tx, order: orderUUID}
	for i, price := range []string{"100.00", "0"} {
		product, err := q.CreateProduct(ctx, db.CreateProductParams{
			OrganizationID: center.ID, BrandID: brand.ID, CategoryID: category.ID,
			Sku: fmt.Sprintf("t1030-%s-%d", suffix, i), Name: "T1030 Film", Images: []byte("[]"), UnitType: "piece", Active: true,
		})
		if err != nil {
			t.Fatalf("product: %v", err)
		}
		source := "manual"
		if price == "0" {
			source = PriceSourceMissing
		}
		var it db.OrderItem
		if err := tx.QueryRow(ctx, `INSERT INTO order_items (
			order_id, organization_id, brand_id, product_id, quantity, unit_price, price_source, line_total
		) VALUES ($1, $2, $3, $4, 3, $5::numeric, $6, $5::numeric * 3) RETURNING id, uuid`,
			orderID, center.ID, brand.ID, product.ID, price, source).Scan(&it.ID, &it.Uuid); err != nil {
			t.Fatalf("order item: %v", err)
		}
		f.items = append(f.items, it)
	}
	f.svc = New(tx, q, invoicingOutbox{}, nil)
	caller := func(o db.Organization) Caller {
		return Caller{
			Principal: authctx.Principal{UserID: user.Uuid, UserInternal: user.ID},
			Org:       orgctx.Scope{InternalID: o.ID, UUID: o.Uuid, OrgType: o.Type, BrandID: brand.ID},
			Filter:    scopefilter.Filter{Permission: rbac.PermOrdersRead, Scope: rbac.ScopeManaged, OrgID: o.ID, OrgIDs: []int64{o.ID}},
		}
	}
	f.center, f.dealer = caller(center), caller(dealer)
	return f
}

func TestMarkInvoicedPerLineThenWhole(t *testing.T) {
	f := newInvoicingFixture(t)
	first := f.items[0].Uuid.String()

	// The buyer side cannot mark.
	if _, err := f.svc.MarkInvoiced(f.ctx, f.dealer, f.order, MarkInvoicedInput{InvoiceNo: "FTR-1", InvoiceDate: "2026-10-27"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("buyer mark: want ErrForbidden, got %v", err)
	}
	// One line → partial.
	v, err := f.svc.MarkInvoiced(f.ctx, f.center, f.order, MarkInvoicedInput{
		InvoiceNo: " FTR-2026-1 ", InvoiceDate: "2026-10-27", ItemUUIDs: []string{first},
	})
	if err != nil {
		t.Fatalf("mark line: %v", err)
	}
	if v.InvoiceStatus != InvoiceStatusPartial || len(v.Invoices) != 1 || v.Invoices[0].InvoiceNo != "FTR-2026-1" {
		t.Fatalf("partial view: status=%s invoices=%+v", v.InvoiceStatus, v.Invoices)
	}
	if !v.Items[0].Invoiced || v.Items[0].InvoiceNo == nil || *v.Items[0].InvoiceNo != "FTR-2026-1" || v.Items[1].Invoiced {
		t.Fatalf("line marks: %+v", v.Items)
	}
	// The same line again → conflict; the same number again → conflict.
	if _, err := f.svc.MarkInvoiced(f.ctx, f.center, f.order, MarkInvoicedInput{InvoiceNo: "FTR-2026-2", InvoiceDate: "2026-10-27", ItemUUIDs: []string{first}}); !errors.Is(err, ErrItemAlreadyInvoiced) {
		t.Fatalf("re-mark: want ErrItemAlreadyInvoiced, got %v", err)
	}
	if _, err := f.svc.MarkInvoiced(f.ctx, f.center, f.order, MarkInvoicedInput{InvoiceNo: "FTR-2026-1", InvoiceDate: "2026-10-27"}); !errors.Is(err, ErrInvoiceNoTaken) {
		t.Fatalf("same number: want ErrInvoiceNoTaken, got %v", err)
	}
	// Empty selection → every remaining line (the price_missing one too).
	v, err = f.svc.MarkInvoiced(f.ctx, f.center, f.order, MarkInvoicedInput{InvoiceNo: "FTR-2026-2", InvoiceDate: "2026-10-28"})
	if err != nil {
		t.Fatalf("mark rest: %v", err)
	}
	if v.InvoiceStatus != InvoiceStatusInvoiced || len(v.Invoices) != 2 || !v.Items[1].Invoiced {
		t.Fatalf("invoiced view: status=%s invoices=%d", v.InvoiceStatus, len(v.Invoices))
	}
	if _, err := f.svc.MarkInvoiced(f.ctx, f.center, f.order, MarkInvoicedInput{InvoiceNo: "FTR-2026-3", InvoiceDate: "2026-10-28"}); !errors.Is(err, ErrNothingToInvoice) {
		t.Fatalf("nothing left: want ErrNothingToInvoice, got %v", err)
	}
	// Undo the second record → partial again; the list filter sees it.
	v, err = f.svc.DeleteInvoice(f.ctx, f.center, f.order, v.Invoices[1].UUID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if v.InvoiceStatus != InvoiceStatusPartial || len(v.Invoices) != 1 || v.Items[1].Invoiced {
		t.Fatalf("after delete: status=%s invoices=%d", v.InvoiceStatus, len(v.Invoices))
	}
	if _, err := f.svc.DeleteInvoice(f.ctx, f.center, f.order, uuid.New()); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("delete unknown: want ErrInvoiceNotFound, got %v", err)
	}
	rows, _, err := f.svc.List(f.ctx, f.center, ListFilter{Side: SideSeller, InvoiceStatuses: []string{InvoiceStatusPartial}, Limit: 50})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, r := range rows {
		found = found || r.UUID == f.order
	}
	if !found {
		t.Fatal("partial order missing from invoice_status=partial list")
	}
	rows, _, err = f.svc.List(f.ctx, f.center, ListFilter{Side: SideSeller, InvoiceStatuses: []string{InvoiceStatusInvoiced}, Limit: 50})
	if err != nil {
		t.Fatalf("list invoiced: %v", err)
	}
	for _, r := range rows {
		if r.UUID == f.order {
			t.Fatal("partial order listed as invoiced")
		}
	}
}

func TestMarkInvoicedValidation(t *testing.T) {
	f := newInvoicingFixture(t)
	var ve *ValidationError
	if _, err := f.svc.MarkInvoiced(f.ctx, f.center, f.order, MarkInvoicedInput{InvoiceNo: "", InvoiceDate: "2026-10-27"}); !errors.As(err, &ve) || ve.Field != "invoice_no" {
		t.Fatalf("empty number: %v", err)
	}
	if _, err := f.svc.MarkInvoiced(f.ctx, f.center, f.order, MarkInvoicedInput{InvoiceNo: "X", InvoiceDate: "27.10.2026"}); !errors.As(err, &ve) || ve.Field != "invoice_date" {
		t.Fatalf("bad date: %v", err)
	}
	if _, err := f.svc.MarkInvoiced(f.ctx, f.center, f.order, MarkInvoicedInput{InvoiceNo: "X", InvoiceDate: "2026-10-27", ItemUUIDs: []string{uuid.New().String()}}); !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("unknown line: %v", err)
	}
}

// TEC-1070 (decision follow-up): like legacy, the invoice mark is allowed
// once the order is shipped (shipped, delivered, received), not before; an
// order with a mark is not cancelled and offers no cancel move.
func TestMarkInvoicedOnShippedAndDelivered(t *testing.T) {
	f := newInvoicingFixture(t)
	setStatus := func(status string) {
		t.Helper()
		if _, err := f.tx.Exec(f.ctx, `UPDATE orders SET status = $1, received_at = NULL WHERE uuid = $2`, status, f.order); err != nil {
			t.Fatalf("status %s: %v", status, err)
		}
	}
	for _, st := range []string{StatusApproved, StatusPreparing, StatusReady} {
		setStatus(st)
		if _, err := f.svc.MarkInvoiced(f.ctx, f.center, f.order, MarkInvoicedInput{InvoiceNo: "FTR-" + st, InvoiceDate: "2026-11-09"}); !errors.Is(err, ErrNotInvoiceable) {
			t.Fatalf("%s: want ErrNotInvoiceable, got %v", st, err)
		}
	}
	setStatus(StatusShipped)
	v, err := f.svc.MarkInvoiced(f.ctx, f.center, f.order, MarkInvoicedInput{
		InvoiceNo: "FTR-S1", InvoiceDate: "2026-11-09", ItemUUIDs: []string{f.items[0].Uuid.String()},
	})
	if err != nil || v.InvoiceStatus != InvoiceStatusPartial {
		t.Fatalf("shipped mark: %+v, %v", v.InvoiceStatus, err)
	}
	// The marked order is not cancelled (the seller may cancel otherwise).
	f.center.Principal.PermissionScopes = map[string]rbac.Scope{rbac.PermOrdersCancel: rbac.ScopeManaged}
	if _, err := f.svc.Transition(f.ctx, f.center, f.order, TransitionInput{Status: StatusCancelling}); !errors.Is(err, ErrOrderInvoiced) {
		t.Fatalf("cancel invoiced order: want ErrOrderInvoiced, got %v", err)
	}
	setStatus(StatusDelivered)
	v, err = f.svc.MarkInvoiced(f.ctx, f.center, f.order, MarkInvoicedInput{InvoiceNo: "FTR-D1", InvoiceDate: "2026-11-09"})
	if err != nil || v.InvoiceStatus != InvoiceStatusInvoiced {
		t.Fatalf("delivered mark: %+v, %v", v.InvoiceStatus, err)
	}
	for _, m := range v.AvailableTransitions {
		if m == StatusCancelling || m == StatusCancelled {
			t.Fatalf("invoiced order offers %s: %v", m, v.AvailableTransitions)
		}
	}
}

func TestInvoicedGuard(t *testing.T) {
	moves := []string{StatusReceived, StatusCancelling}
	if got := invoicedGuard(db.Order{InvoiceStatus: InvoiceStatusPending}, moves); len(got) != 2 {
		t.Fatalf("pending = %v", got)
	}
	if got := invoicedGuard(db.Order{InvoiceStatus: InvoiceStatusPartial}, moves); len(got) != 1 || got[0] != StatusReceived {
		t.Fatalf("partial = %v", got)
	}
	if !Invoiceable(StatusShipped) || !Invoiceable(StatusDelivered) || !Invoiceable(StatusReceived) || Invoiceable(StatusReady) {
		t.Fatal("invoiceable statuses")
	}
}
