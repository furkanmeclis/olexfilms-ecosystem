package db_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-305: database-level guards of the service catalog schema (migration
// 000084). Reuses the customer fixture (rolled-back transaction, savepoint
// per failure).

type serviceCatalogFixture struct {
	*customerFixture
	dist   db.Organization
	dealer db.Organization
}

func newServiceCatalogFixture(t *testing.T) *serviceCatalogFixture {
	t.Helper()
	f := &serviceCatalogFixture{customerFixture: newCustomerFixture(t)}
	f.dist = f.org(t, "dist", "distributor", f.olex.ID)
	f.dealer = f.org(t, "dealer", "dealer", f.dist.ID)
	return f
}

func (f *serviceCatalogFixture) org(t *testing.T, name, typ string, parent int64) db.Organization {
	t.Helper()
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("t305-%s-%d", name, time.Now().UnixNano()), Name: name, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ, ParentID: pgtype.Int8{Int64: parent, Valid: true},
		BrandID: f.olex.BrandID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	return o
}

func (f *serviceCatalogFixture) itemParams(t *testing.T, owner db.Organization, category string) db.CreateServiceCatalogItemParams {
	return db.CreateServiceCatalogItemParams{
		OrganizationID: owner.ID, BrandID: owner.BrandID, Name: "Yazilim " + category,
		Category: category, DefaultPrice: numeric(t, "1000.00"), Currency: "TRY",
		Recurrence: "monthly", CancellationFee: numeric(t, "250.00"), IsActive: true,
	}
}

func (f *serviceCatalogFixture) item(t *testing.T, category string) db.ServiceCatalogItem {
	t.Helper()
	it, err := f.q.CreateServiceCatalogItem(f.ctx, f.itemParams(t, f.olex, category))
	if err != nil {
		t.Fatalf("catalog item: %v", err)
	}
	return it
}

func day(y int, m time.Month, d int) pgtype.Date {
	return pgtype.Date{Time: time.Date(y, m, d, 0, 0, 0, 0, time.UTC), Valid: true}
}

func (f *serviceCatalogFixture) subParams(t *testing.T, it db.ServiceCatalogItem) db.CreateServiceSubscriptionParams {
	return db.CreateServiceSubscriptionParams{
		OrganizationID: f.dealer.ID, BrandID: it.BrandID, SellerOrgID: f.olex.ID, ItemID: it.ID,
		AssignedByOrgID: f.dist.ID, StartsOn: day(2026, 10, 1), EndsOn: day(2027, 9, 30),
		Recurrence: it.Recurrence, Price: it.DefaultPrice, Currency: it.Currency,
		RateSnapshot: []byte(`{}`), CancellationFee: it.CancellationFee, Status: "active",
	}
}

func TestServiceCatalogSchemaConstraints(t *testing.T) {
	f := newServiceCatalogFixture(t)
	ctx := f.ctx

	t.Run("catalog item only in the center", func(t *testing.T) {
		for _, owner := range []db.Organization{f.dist, f.dealer} {
			f.expectCode(t, "item owned by "+owner.Type, func(sp pgx.Tx) error {
				_, err := db.New(sp).CreateServiceCatalogItem(ctx, f.itemParams(t, owner, "training"))
				return err
			}, "23514")
		}
		// Center of another brand cannot own an Olex item.
		f.expectCode(t, "item owned by another brand's center", func(sp pgx.Tx) error {
			arg := f.itemParams(t, f.glorian, "training")
			arg.BrandID = f.olex.BrandID
			_, err := db.New(sp).CreateServiceCatalogItem(ctx, arg)
			return err
		}, "23514")
		f.expectOK(t, "center item", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateServiceCatalogItem(ctx, f.itemParams(t, f.olex, "training"))
			return err
		})
	})

	t.Run("modules only on module_bundle items", func(t *testing.T) {
		plain := f.item(t, "software")
		f.expectCode(t, "module on software item", func(sp pgx.Tx) error {
			return db.New(sp).AddServiceCatalogModule(ctx, db.AddServiceCatalogModuleParams{ItemID: plain.ID, ModuleKey: "catalog"})
		}, "23503")
		bundle := f.item(t, "module_bundle")
		if err := f.q.AddServiceCatalogModule(ctx, db.AddServiceCatalogModuleParams{ItemID: bundle.ID, ModuleKey: "catalog"}); err != nil {
			t.Fatalf("module on bundle: %v", err)
		}
		// A bundle with modules cannot be turned into another category.
		f.expectCode(t, "bundle category change", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE service_catalog_items SET category = 'other' WHERE id = $1`, bundle.ID)
			return err
		}, "23503")
	})

	t.Run("subscription without end rejected", func(t *testing.T) {
		it := f.item(t, "advertising")
		f.expectCode(t, "no ends_on", func(sp pgx.Tx) error {
			arg := f.subParams(t, it)
			arg.EndsOn = pgtype.Date{}
			_, err := db.New(sp).CreateServiceSubscription(ctx, arg)
			return err
		}, "23502")
		f.expectCode(t, "ends before start", func(sp pgx.Tx) error {
			arg := f.subParams(t, it)
			arg.EndsOn = day(2026, 9, 1)
			_, err := db.New(sp).CreateServiceSubscription(ctx, arg)
			return err
		}, "23514")
		f.expectCode(t, "seller not the center", func(sp pgx.Tx) error {
			arg := f.subParams(t, it)
			arg.SellerOrgID = f.dist.ID
			_, err := db.New(sp).CreateServiceSubscription(ctx, arg)
			return err
		}, "23514")
		f.expectOK(t, "valid subscription", func(sp pgx.Tx) error {
			s, err := db.New(sp).CreateServiceSubscription(ctx, f.subParams(t, it))
			if err == nil && s.Status != "active" {
				err = fmt.Errorf("status = %q", s.Status)
			}
			return err
		})
	})

	t.Run("same period written twice", func(t *testing.T) {
		it := f.item(t, "software")
		s, err := f.q.CreateServiceSubscription(ctx, f.subParams(t, it))
		if err != nil {
			t.Fatalf("subscription: %v", err)
		}
		period := db.InsertServiceSubscriptionPeriodParams{
			SubscriptionID: s.ID, OrganizationID: s.OrganizationID, BrandID: s.BrandID,
			PeriodStart: day(2026, 10, 1), PeriodEnd: day(2026, 10, 31),
		}
		if _, err := f.q.InsertServiceSubscriptionPeriod(ctx, period); err != nil {
			t.Fatalf("first period: %v", err)
		}
		// The idempotent query writes nothing the second time.
		if _, err := f.q.InsertServiceSubscriptionPeriod(ctx, period); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("replay: want no row, got %v", err)
		}
		f.expectCode(t, "duplicate period", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `INSERT INTO service_subscription_periods
				(subscription_id, organization_id, brand_id, period_start, period_end)
				VALUES ($1, $2, $3, $4, $5)`,
				s.ID, s.OrganizationID, s.BrandID, period.PeriodStart, period.PeriodEnd)
			return err
		}, "23505")
	})

	t.Run("price override only for distributors", func(t *testing.T) {
		it := f.item(t, "setup")
		arg := db.UpsertServicePriceOverrideParams{
			ItemID: it.ID, OrganizationID: f.dealer.ID, BrandID: it.BrandID,
			Price: numeric(t, "800.00"), Currency: "TRY",
		}
		f.expectCode(t, "override for dealer", func(sp pgx.Tx) error {
			_, err := db.New(sp).UpsertServicePriceOverride(ctx, arg)
			return err
		}, "23514")
		arg.OrganizationID = f.dist.ID
		if _, err := f.q.UpsertServicePriceOverride(ctx, arg); err != nil {
			t.Fatalf("override: %v", err)
		}
		f.expectCode(t, "second override row", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `INSERT INTO service_price_overrides
				(item_id, organization_id, brand_id, price, currency) VALUES ($1, $2, $3, 1, 'TRY')`,
				it.ID, f.dist.ID, it.BrandID)
			return err
		}, "23505")
	})

	t.Run("one pending cancel request", func(t *testing.T) {
		it := f.item(t, "other")
		s, err := f.q.CreateServiceSubscription(ctx, f.subParams(t, it))
		if err != nil {
			t.Fatalf("subscription: %v", err)
		}
		req := db.CreateServiceSubscriptionCancelRequestParams{
			SubscriptionID: s.ID, OrganizationID: s.OrganizationID, BrandID: s.BrandID,
			RequestedByOrgID: f.dealer.ID, Reason: "kapanis", CancellationFee: s.CancellationFee, Currency: s.Currency,
		}
		r, err := f.q.CreateServiceSubscriptionCancelRequest(ctx, req)
		if err != nil {
			t.Fatalf("cancel request: %v", err)
		}
		f.expectCode(t, "second pending", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateServiceSubscriptionCancelRequest(ctx, req)
			return err
		}, "23505")
		if _, err := f.q.DecideServiceSubscriptionCancelRequest(ctx, db.DecideServiceSubscriptionCancelRequestParams{
			ID: r.ID, BrandID: r.BrandID, Status: "rejected",
		}); err != nil {
			t.Fatalf("decide: %v", err)
		}
		f.expectCode(t, "decision is final", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE service_subscription_cancel_requests
				SET status = 'approved' WHERE id = $1`, r.ID)
			return err
		}, "23514")
	})
}
