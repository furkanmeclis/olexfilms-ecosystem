package db_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-165: database-level guards of the order schema (migration 000049).
// Reuses the stock fixture (rolled-back transaction, savepoint per failure).

type orderFixture struct {
	*stockFixture
	currency string
	dist     db.Organization
	dealer   db.Organization
	dealer2  db.Organization
}

func newOrderFixture(t *testing.T) *orderFixture {
	t.Helper()
	sf := newStockFixture(t)
	brand, err := sf.q.GetBrandBySlug(sf.ctx, "olex")
	if err != nil {
		t.Fatal(err)
	}
	f := &orderFixture{stockFixture: sf, currency: brand.Currency}
	f.dist = f.org(t, "dist", "distributor", sf.centerID)
	f.dealer = f.org(t, "dealer", "dealer", f.dist.ID)
	f.dealer2 = f.org(t, "dealer2", "dealer", f.dist.ID)
	return f
}

func (f *orderFixture) org(t *testing.T, name, typ string, parent int64) db.Organization {
	t.Helper()
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("t165-%s-%d", name, time.Now().UnixNano()), Name: name, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ, ParentID: pgtype.Int8{Int64: parent, Valid: true},
		BrandID: f.brandID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	return o
}

func (f *orderFixture) orderParams(seller, buyer int64) db.CreateOrderParams {
	return db.CreateOrderParams{
		SellerOrgID: seller, BrandID: f.brandID, BuyerOrgID: buyer, Currency: f.currency,
	}
}

func (f *orderFixture) order(t *testing.T, seller, buyer int64) db.Order {
	t.Helper()
	o, err := f.q.CreateOrder(f.ctx, f.orderParams(seller, buyer))
	if err != nil {
		t.Fatalf("order %d->%d: %v", seller, buyer, err)
	}
	return o
}

func (f *orderFixture) item(t *testing.T, o db.Order, p db.Product) db.OrderItem {
	t.Helper()
	arg := db.CreateOrderItemParams{
		OrderID: o.ID, ProductID: p.ID, UnitPrice: numeric(t, "12.5000"),
		PriceSource: "list", LineTotal: numeric(t, "125.00"),
	}
	if p.UnitType == "roll_meter" {
		arg.Meters = numeric(t, "10")
	} else {
		arg.Quantity = pgtype.Int4{Int32: 10, Valid: true}
	}
	it, err := f.q.CreateOrderItem(f.ctx, arg)
	if err != nil {
		t.Fatalf("order item: %v", err)
	}
	return it
}

// expectConstraint is expectCode plus the violated constraint's name.
func (f *orderFixture) expectConstraint(t *testing.T, name, code, constraint string, fn func(sp pgx.Tx) error) {
	t.Helper()
	sp, err := f.tx.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = fn(sp)
	_ = sp.Rollback(f.ctx)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code || pgErr.ConstraintName != constraint {
		t.Fatalf("%s: want %s on %s, got %v", name, code, constraint, err)
	}
}

func TestOrderSchemaConstraints(t *testing.T) {
	f := newOrderFixture(t)
	ctx := f.ctx

	o := f.order(t, f.centerID, f.dist.ID)
	if o.Status != "draft" || o.OrganizationID != f.centerID || o.OrderNo == "" {
		t.Fatalf("new order = %+v", o)
	}

	t.Run("seller equals buyer rejected", func(t *testing.T) {
		f.expectConstraint(t, "seller = buyer", "23514", "chk_orders_seller_not_buyer", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateOrder(ctx, f.orderParams(f.dist.ID, f.dist.ID))
			return err
		})
	})

	t.Run("invalid status rejected", func(t *testing.T) {
		f.expectConstraint(t, "status bogus", "23514", "chk_orders_status", func(sp pgx.Tx) error {
			_, err := db.New(sp).UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{ID: o.ID, Status: "bogus"})
			return err
		})
		f.expectConstraint(t, "history bogus", "23514", "chk_order_status_history_to", func(sp pgx.Tx) error {
			_, err := db.New(sp).InsertOrderStatusHistory(ctx, db.InsertOrderStatusHistoryParams{
				OrderID: o.ID, ToStatus: "bogus", Metadata: []byte("{}"),
			})
			return err
		})
		for _, st := range []string{"submitted", "cancelling", "cancelled"} {
			got, err := f.q.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{ID: o.ID, Status: st})
			if err != nil || got.Status != st {
				t.Fatalf("status %s: %+v, %v", st, got, err)
			}
		}
		got, _ := f.q.GetOrder(ctx, db.GetOrderParams{ID: o.ID, BrandID: f.brandID})
		if !got.SubmittedAt.Valid || !got.CancelRequestedAt.Valid || !got.CancelledAt.Valid {
			t.Fatalf("status timestamps not stamped: %+v", got)
		}
	})

	t.Run("hierarchy and currency", func(t *testing.T) {
		f.expectCode(t, "center -> dealer (K6)", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateOrder(ctx, f.orderParams(f.centerID, f.dealer.ID))
			return err
		}, "23514")
		f.expectCode(t, "dealer -> sibling dealer", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateOrder(ctx, f.orderParams(f.dealer.ID, f.dealer2.ID))
			return err
		}, "23514")
		f.expectCode(t, "upward order", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateOrder(ctx, f.orderParams(f.dist.ID, f.centerID))
			return err
		}, "23514")
		f.expectCode(t, "currency not brand currency", func(sp pgx.Tx) error {
			arg := f.orderParams(f.dist.ID, f.dealer.ID)
			arg.Currency = "XXX"
			if f.currency == "XXX" {
				arg.Currency = "YYY"
			}
			_, err := db.New(sp).CreateOrder(ctx, arg)
			return err
		}, "23514")
		down := f.order(t, f.dist.ID, f.dealer.ID)
		seller, err := f.q.ListOrdersBySeller(ctx, db.ListOrdersBySellerParams{
			BrandID: f.brandID, SellerOrgID: f.dist.ID, RowLimit: 10,
		})
		if err != nil || len(seller) != 1 || seller[0].ID != down.ID {
			t.Fatalf("seller list = %+v, %v", seller, err)
		}
		buyer, err := f.q.ListOrdersByBuyer(ctx, db.ListOrdersByBuyerParams{
			BrandID: f.brandID, BuyerOrgID: f.dist.ID, RowLimit: 10,
		})
		if err != nil || len(buyer) != 1 || buyer[0].ID != o.ID {
			t.Fatalf("buyer list = %+v, %v", buyer, err)
		}
		scoped, err := f.q.CountOrdersInScope(ctx, db.CountOrdersInScopeParams{
			BrandID: f.brandID, OrgIds: []int64{f.dist.ID},
		})
		if err != nil || scoped != 2 {
			t.Fatalf("scope count = %d, %v", scoped, err)
		}
	})

	t.Run("approval freezes the rate", func(t *testing.T) {
		draft := f.order(t, f.centerID, f.dist.ID)
		f.expectCode(t, "approved_at without snapshot", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE orders SET approved_at = NOW() WHERE id = $1`, draft.ID)
			return err
		}, "23514")
		got, err := f.q.ApproveOrder(ctx, db.ApproveOrderParams{
			ID: draft.ID, RateSnapshot: []byte(`{"base":"TRY","quote":"TRY","rate":"1"}`),
			TryRate: numeric(t, "1"),
		})
		if err != nil || got.Status != "approved" || !got.ApprovedAt.Valid {
			t.Fatalf("approve = %+v, %v", got, err)
		}
		locked, err := f.q.LockOrder(ctx, db.LockOrderParams{ID: draft.ID, BrandID: f.brandID})
		if err != nil || locked.ID != draft.ID {
			t.Fatalf("lock order = %+v, %v", locked, err)
		}
	})

	t.Run("line measure follows product", func(t *testing.T) {
		f.expectCode(t, "roll line in quantity", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateOrderItem(ctx, db.CreateOrderItemParams{
				OrderID: o.ID, ProductID: f.roll.ID, Quantity: pgtype.Int4{Int32: 1, Valid: true},
				UnitPrice: numeric(t, "1"), PriceSource: "list", LineTotal: numeric(t, "1"),
			})
			return err
		}, "23514")
		f.expectCode(t, "negative price", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateOrderItem(ctx, db.CreateOrderItemParams{
				OrderID: o.ID, ProductID: f.fixed.ID, Quantity: pgtype.Int4{Int32: 1, Valid: true},
				UnitPrice: numeric(t, "-1"), PriceSource: "list", LineTotal: numeric(t, "0"),
			})
			return err
		}, "23514")
	})

	t.Run("history is append-only", func(t *testing.T) {
		h, err := f.q.InsertOrderStatusHistory(ctx, db.InsertOrderStatusHistoryParams{
			OrderID: o.ID, ToStatus: "draft", Metadata: []byte("{}"),
		})
		if err != nil {
			t.Fatal(err)
		}
		f.expectCode(t, "history update", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE order_status_history SET reason = 'x' WHERE id = $1`, h.ID)
			return err
		}, "23001")
		f.expectCode(t, "history delete", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `DELETE FROM order_status_history WHERE id = $1`, h.ID)
			return err
		}, "23001")
	})
}

func TestStockReservationConstraints(t *testing.T) {
	f := newOrderFixture(t)
	ctx := f.ctx

	o1 := f.order(t, f.centerID, f.dist.ID)
	o2 := f.order(t, f.centerID, f.dist.ID)
	roll1, roll2 := f.item(t, o1, f.roll), f.item(t, o2, f.roll)
	fixed1, fixed2 := f.item(t, o1, f.fixed), f.item(t, o2, f.fixed)

	rollUnit, err := f.q.CreateUnit(ctx, f.unitParams(t, f.roll, "15", "15"))
	if err != nil {
		t.Fatalf("roll unit: %v", err)
	}
	fixedUnit, err := f.q.CreateUnit(ctx, f.unitParams(t, f.fixed, "", ""))
	if err != nil {
		t.Fatalf("fixed unit: %v", err)
	}
	meters := func(item db.OrderItem) db.CreateStockReservationParams {
		return db.CreateStockReservationParams{UnitID: rollUnit.ID, OrderItemID: item.ID, Meters: numeric(t, "10")}
	}

	first, err := f.q.CreateStockReservation(ctx, meters(roll1))
	if err != nil || first.Status != "active" || first.UnitKind != "serial" || first.OrganizationID != f.centerID {
		t.Fatalf("first reservation = %+v, %v", first, err)
	}

	t.Run("second active reservation of a serial unit rejected", func(t *testing.T) {
		f.expectConstraint(t, "other order", "23505", "uq_stock_reservations_active_serial", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateStockReservation(ctx, meters(roll2))
			return err
		})
	})

	t.Run("released unit can be reserved again", func(t *testing.T) {
		rel, err := f.q.ReleaseStockReservation(ctx, first.ID)
		if err != nil || rel.Status != "released" || !rel.ReleasedAt.Valid {
			t.Fatalf("release = %+v, %v", rel, err)
		}
		again, err := f.q.CreateStockReservation(ctx, meters(roll2))
		if err != nil {
			t.Fatalf("reserve after release: %v", err)
		}
		con, err := f.q.ConsumeStockReservation(ctx, again.ID)
		if err != nil || con.Status != "consumed" {
			t.Fatalf("consume = %+v, %v", con, err)
		}
	})

	t.Run("fixed barcode may be reserved by many lines", func(t *testing.T) {
		for _, it := range []db.OrderItem{fixed1, fixed2} {
			r, err := f.q.CreateStockReservation(ctx, db.CreateStockReservationParams{
				UnitID: fixedUnit.ID, OrderItemID: it.ID, Quantity: pgtype.Int4{Int32: 4, Valid: true},
			})
			if err != nil || r.UnitKind != "fixed" {
				t.Fatalf("fixed reservation = %+v, %v", r, err)
			}
		}
		sum, err := f.q.SumActiveReservedQuantityByUnit(ctx, fixedUnit.ID)
		if err != nil || sum != 8 {
			t.Fatalf("reserved sum = %d, %v", sum, err)
		}
		f.expectConstraint(t, "same line twice", "23505", "uq_stock_reservations_active_item_unit", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateStockReservation(ctx, db.CreateStockReservationParams{
				UnitID: fixedUnit.ID, OrderItemID: fixed1.ID, Quantity: pgtype.Int4{Int32: 1, Valid: true},
			})
			return err
		})
		n, err := f.q.ReleaseReservationsByOrder(ctx, o1.ID)
		if err != nil || n != 1 {
			t.Fatalf("release by order = %d, %v", n, err)
		}
	})

	t.Run("unit must match the line", func(t *testing.T) {
		f.expectCode(t, "roll unit on fixed line", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateStockReservation(ctx, db.CreateStockReservationParams{
				UnitID: rollUnit.ID, OrderItemID: fixed2.ID, Meters: numeric(t, "1"),
			})
			return err
		}, "23514")
		f.expectConstraint(t, "invalid reservation status", "23514", "chk_stock_reservations_status", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE stock_reservations SET status = 'bogus' WHERE order_item_id = $1`, fixed2.ID)
			return err
		})
	})

	t.Run("assigned units", func(t *testing.T) {
		u, err := f.q.CreateOrderItemUnit(ctx, db.CreateOrderItemUnitParams{
			OrderItemID: roll1.ID, UnitID: rollUnit.ID, Meters: numeric(t, "10"),
		})
		if err != nil || u.OrganizationID != f.centerID {
			t.Fatalf("assign = %+v, %v", u, err)
		}
		f.expectConstraint(t, "same unit twice", "23505", "uq_order_item_units_item_unit", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateOrderItemUnit(ctx, db.CreateOrderItemUnitParams{
				OrderItemID: roll1.ID, UnitID: rollUnit.ID, Meters: numeric(t, "1"),
			})
			return err
		})
		rows, err := f.q.ListOrderItemUnitsByOrder(ctx, o1.ID)
		if err != nil || len(rows) != 1 || rows[0].Barcode != rollUnit.Barcode {
			t.Fatalf("units by order = %+v, %v", rows, err)
		}
	})
}

func TestStockTransferRequestConstraints(t *testing.T) {
	f := newOrderFixture(t)
	ctx := f.ctx
	arg := db.CreateStockTransferRequestParams{
		FromOrgID: f.dealer.ID, BrandID: f.brandID, ToOrgID: f.dealer2.ID, ApproverOrgID: f.dist.ID,
		ProductID: f.fixed.ID, Quantity: pgtype.Int4{Int32: 2, Valid: true}, Currency: f.currency,
	}
	req, err := f.q.CreateStockTransferRequest(ctx, arg)
	if err != nil || req.Status != "requested" || req.OrganizationID != f.dealer.ID {
		t.Fatalf("transfer = %+v, %v", req, err)
	}

	f.expectCode(t, "approver is not the parent", func(sp pgx.Tx) error {
		bad := arg
		bad.ApproverOrgID = f.centerID
		_, err := db.New(sp).CreateStockTransferRequest(ctx, bad)
		return err
	}, "23514")
	f.expectConstraint(t, "to self", "23514", "chk_stock_transfer_requests_parties", func(sp pgx.Tx) error {
		bad := arg
		bad.ToOrgID = f.dealer.ID
		_, err := db.New(sp).CreateStockTransferRequest(ctx, bad)
		return err
	})
	f.expectConstraint(t, "invalid status", "23514", "chk_stock_transfer_requests_status", func(sp pgx.Tx) error {
		_, err := sp.Exec(ctx, `UPDATE stock_transfer_requests SET status = 'bogus' WHERE id = $1`, req.ID)
		return err
	})

	ok, err := f.q.ApproveStockTransferRequest(ctx, db.ApproveStockTransferRequestParams{
		ID: req.ID, UnitPrice: numeric(t, "10.0000"), LineTotal: numeric(t, "20.00"),
	})
	if err != nil || ok.Status != "approved" || !ok.DecidedAt.Valid {
		t.Fatalf("approve = %+v, %v", ok, err)
	}
	done, err := f.q.CompleteStockTransferRequest(ctx, req.ID)
	if err != nil || done.Status != "completed" {
		t.Fatalf("complete = %+v, %v", done, err)
	}
	list, err := f.q.ListStockTransferRequestsInScope(ctx, db.ListStockTransferRequestsInScopeParams{
		BrandID: f.brandID, OrgIds: []int64{f.dist.ID}, RowLimit: 10,
	})
	if err != nil || len(list) != 1 {
		t.Fatalf("scope list = %+v, %v", list, err)
	}
}
