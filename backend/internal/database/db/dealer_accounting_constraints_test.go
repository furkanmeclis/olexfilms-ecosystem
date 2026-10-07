package db_test

import (
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-341: database-level guards of the dealer accounting schema (migration
// 000093). Reuses the service fixture (rolled-back transaction, savepoint
// per failure).

func date(t time.Time) pgtype.Date { return pgtype.Date{Time: t, Valid: true} }

func TestDealerAccountingSchemaConstraints(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.ctx
	dealer := f.dealer

	t.Run("dealer product price is unique per organization and product", func(t *testing.T) {
		arg := db.UpsertDealerProductPriceParams{
			OrganizationID: dealer.ID, BrandID: dealer.BrandID, ProductID: f.film.ID,
			SalePrice: numeric(t, "100.00"), Currency: "TRY",
		}
		if _, err := f.q.UpsertDealerProductPrice(ctx, arg); err != nil {
			t.Fatalf("price: %v", err)
		}
		arg.SalePrice = numeric(t, "120.50")
		p, err := f.q.UpsertDealerProductPrice(ctx, arg)
		if err != nil || numericFloat(t, p.SalePrice) != 120.5 {
			t.Fatalf("upsert = %+v, %v", p, err)
		}
		f.expectCode(t, "duplicate price", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `INSERT INTO dealer_product_prices
				(organization_id, brand_id, product_id, sale_price, currency) VALUES ($1, $2, $3, 1, 'TRY')`,
				dealer.ID, dealer.BrandID, f.film.ID)
			return err
		}, "23505")
		// Another dealer prices the same product on its own.
		if _, err := f.q.UpsertDealerProductPrice(ctx, db.UpsertDealerProductPriceParams{
			OrganizationID: f.dealer2.ID, BrandID: f.dealer2.BrandID, ProductID: f.film.ID,
			SalePrice: numeric(t, "90.00"), Currency: "TRY",
		}); err != nil {
			t.Fatalf("dealer2 price: %v", err)
		}
		f.expectCode(t, "negative price", func(sp pgx.Tx) error {
			_, err := db.New(sp).UpsertDealerProductPrice(ctx, db.UpsertDealerProductPriceParams{
				OrganizationID: dealer.ID, BrandID: dealer.BrandID, ProductID: f.piece.ID,
				SalePrice: numeric(t, "-1"), Currency: "TRY",
			})
			return err
		}, "23514")
	})

	t.Run("product sale lines reject non-positive quantities", func(t *testing.T) {
		sale, err := f.q.CreateProductSale(ctx, db.CreateProductSaleParams{
			OrganizationID: dealer.ID, BrandID: dealer.BrandID, PaymentMethod: "cash",
			Currency: "TRY", Total: numeric(t, "200.00"), SoldAt: timestamptz(time.Now()),
		})
		if err != nil {
			t.Fatalf("sale: %v", err)
		}
		line := func(qty string) db.CreateProductSaleLineParams {
			return db.CreateProductSaleLineParams{
				SaleID: sale.ID, OrganizationID: dealer.ID, BrandID: dealer.BrandID, ProductID: f.piece.ID,
				Quantity: numeric(t, qty), UnitPrice: numeric(t, "100.00"), LineTotal: numeric(t, "200.00"),
				PurchaseUnitCost: numeric(t, "60.00"),
			}
		}
		if _, err := f.q.CreateProductSaleLine(ctx, line("2")); err != nil {
			t.Fatalf("line: %v", err)
		}
		for _, qty := range []string{"-1", "0"} {
			f.expectCode(t, "quantity "+qty, func(sp pgx.Tx) error {
				_, err := db.New(sp).CreateProductSaleLine(ctx, line(qty))
				return err
			}, "23514")
		}
		f.expectCode(t, "line of another organization", func(sp pgx.Tx) error {
			arg := line("1")
			arg.OrganizationID, arg.BrandID = f.dealer2.ID, f.dealer2.BrandID
			_, err := db.New(sp).CreateProductSaleLine(ctx, arg)
			return err
		}, "23503")
		f.expectCode(t, "cari sale without cari", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateProductSale(ctx, db.CreateProductSaleParams{
				OrganizationID: dealer.ID, BrandID: dealer.BrandID, PaymentMethod: "cari",
				CustomerUserID: int8p(f.customer.ID), Currency: "TRY", Total: numeric(t, "1"),
				SoldAt: timestamptz(time.Now()),
			})
			return err
		}, "23514")
		f.expectCode(t, "bad payment method", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateProductSale(ctx, db.CreateProductSaleParams{
				OrganizationID: dealer.ID, BrandID: dealer.BrandID, PaymentMethod: "barter",
				Currency: "TRY", Total: numeric(t, "1"), SoldAt: timestamptz(time.Now()),
			})
			return err
		}, "23514")
	})

	t.Run("salary is unique per staff and period", func(t *testing.T) {
		staff, err := f.q.CreateStaffProfile(ctx, db.CreateStaffProfileParams{
			OrganizationID: dealer.ID, BrandID: dealer.BrandID, Name: "Usta",
			MonthlySalary: numeric(t, "30000.00"), Currency: "TRY", Active: true,
		})
		if err != nil {
			t.Fatalf("staff: %v", err)
		}
		pay := func(q *db.Queries, typ, period string) (db.StaffPayment, error) {
			return q.CreateStaffPayment(ctx, db.CreateStaffPaymentParams{
				OrganizationID: dealer.ID, BrandID: dealer.BrandID, StaffID: staff.ID, Type: typ,
				Period: period, Amount: numeric(t, "1000.00"), Currency: "TRY", PaidOn: date(time.Now()), Status: "posted",
			})
		}
		salary, err := pay(f.q, "salary", "2026-09")
		if err != nil {
			t.Fatalf("salary: %v", err)
		}
		f.expectCode(t, "second salary", func(sp pgx.Tx) error {
			_, err := pay(db.New(sp), "salary", "2026-09")
			return err
		}, "23505")
		for _, typ := range []string{"advance", "advance", "bonus"} {
			if _, err := pay(f.q, typ, "2026-09"); err != nil {
				t.Fatalf("%s: %v", typ, err)
			}
		}
		if _, err := pay(f.q, "salary", "2026-10"); err != nil {
			t.Fatalf("next period salary: %v", err)
		}
		for _, period := range []string{"2026-13", "2026-9", "26-09"} {
			f.expectCode(t, "period "+period, func(sp pgx.Tx) error {
				_, err := pay(db.New(sp), "advance", period)
				return err
			}, "23514")
		}
		f.expectCode(t, "bad type", func(sp pgx.Tx) error {
			_, err := pay(db.New(sp), "tip", "2026-09")
			return err
		}, "23514")
		// A voided salary frees its period.
		if _, err := f.q.VoidStaffPayment(ctx, db.VoidStaffPaymentParams{ID: salary.ID, OrganizationID: dealer.ID}); err != nil {
			t.Fatalf("void: %v", err)
		}
		if _, err := pay(f.q, "salary", "2026-09"); err != nil {
			t.Fatalf("salary after void: %v", err)
		}
		f.expectCode(t, "staff of another organization", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateStaffPayment(ctx, db.CreateStaffPaymentParams{
				OrganizationID: f.dealer2.ID, BrandID: f.dealer2.BrandID, StaffID: staff.ID, Type: "bonus",
				Period: "2026-09", Amount: numeric(t, "1"), Currency: "TRY", PaidOn: date(time.Now()), Status: "posted",
			})
			return err
		}, "23503")
	})

	t.Run("suppliers and purchases", func(t *testing.T) {
		sup, err := f.q.CreateSupplier(ctx, db.CreateSupplierParams{
			OrganizationID: dealer.ID, BrandID: dealer.BrandID, Name: "Tedarikci",
			PhoneE164: text("+905551112233"),
		})
		if err != nil {
			t.Fatalf("supplier: %v", err)
		}
		f.expectCode(t, "bad phone", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateSupplier(ctx, db.CreateSupplierParams{
				OrganizationID: dealer.ID, BrandID: dealer.BrandID, Name: "X", PhoneE164: text("05551112233"),
			})
			return err
		}, "23514")
		p, err := f.q.CreatePurchase(ctx, db.CreatePurchaseParams{
			OrganizationID: dealer.ID, BrandID: dealer.BrandID, SupplierID: sup.ID,
			PurchasedOn: date(time.Now()), Currency: "TRY", Amount: numeric(t, "500.00"), PaymentMethod: "cash",
		})
		if err != nil {
			t.Fatalf("purchase: %v", err)
		}
		if _, err := f.q.CreatePurchaseLine(ctx, db.CreatePurchaseLineParams{
			PurchaseID: p.ID, OrganizationID: dealer.ID, BrandID: dealer.BrandID, Description: "Temizlik malzemesi",
			Quantity: numeric(t, "1"), UnitPrice: numeric(t, "500.00"), LineTotal: numeric(t, "500.00"),
		}); err != nil {
			t.Fatalf("purchase line: %v", err)
		}
		f.expectCode(t, "supplier of another organization", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreatePurchase(ctx, db.CreatePurchaseParams{
				OrganizationID: f.dealer2.ID, BrandID: f.dealer2.BrandID, SupplierID: sup.ID,
				PurchasedOn: date(time.Now()), Currency: "TRY", Amount: numeric(t, "1"), PaymentMethod: "cash",
			})
			return err
		}, "23503")
	})

	t.Run("customer cari requires a served customer", func(t *testing.T) {
		arg := db.CreateCariForUserIfMissingParams{
			OrganizationID: f.dealer2.ID, BrandID: f.dealer2.BrandID,
			CounterpartyUserID: f.customer.ID, Currency: f.dealer2.Currency,
		}
		f.expectCode(t, "not a customer", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateCariForUserIfMissing(ctx, arg)
			return err
		}, "23514")
		if _, err := f.tx.Exec(ctx, `INSERT INTO customer_organizations (user_id, organization_id, brand_id)
			VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, f.customer.ID, f.dealer2.ID, f.dealer2.BrandID); err != nil {
			t.Fatalf("link: %v", err)
		}
		c, err := f.q.CreateCariForUserIfMissing(ctx, arg)
		if err != nil || c.CounterpartyType != "user" {
			t.Fatalf("cari = %+v, %v", c, err)
		}
		if _, err := f.q.CreateCariForUserIfMissing(ctx, arg); err != pgx.ErrNoRows {
			t.Fatalf("second cari: %v", err)
		}
	})
}
