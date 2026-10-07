package db_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-472: fleet schema (migration 000112). A fleet is an organization of
// type fleet outside the tree; the tree queries (subtree, scoped and
// platform lists, tree card, search index, public dealer lists with the
// contract check) never return it. Reuses the order fixture (center >
// dist > dealer, dealer2; rolled-back transaction, savepoint per failure).

func (f *orderFixture) fleetOrg(t *testing.T, name string) db.Organization {
	t.Helper()
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("t472-%s-%d", name, time.Now().UnixNano()), Name: name, Status: "active",
		City:           "Fleetcity",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "fleet", BrandID: f.brandID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("fleet org %s: %v", name, err)
	}
	return o
}

func fleetProfileParams(org db.Organization, tax string) db.CreateFleetProfileParams {
	return db.CreateFleetProfileParams{
		OrganizationID: org.ID, BrandID: org.BrandID, TaxNumber: tax, LegalName: org.Name + " A.Ş.",
		ReportFrequency: "monthly", ReportLocale: "tr",
	}
}

func fleetLinkParams(fleet, dealer db.Organization, status string) db.CreateFleetDealerLinkParams {
	return db.CreateFleetDealerLinkParams{
		FleetOrgID: fleet.ID, DealerOrgID: dealer.ID, BrandID: fleet.BrandID, Status: status,
		CreatedByOrgID: dealer.ID,
	}
}

func TestFleetSchemaConstraints(t *testing.T) {
	f := newOrderFixture(t)
	ctx := f.ctx
	fleet := f.fleetOrg(t, "filo")
	if fleet.ParentID.Valid || fleet.Type != "fleet" {
		t.Fatalf("fleet org = %+v", fleet)
	}

	t.Run("organization: fleet has no parent and is never a parent", func(t *testing.T) {
		f.expectConstraint(t, "fleet with parent", "23514", "chk_organizations_parent", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateOrganization(ctx, db.CreateOrganizationParams{
				Slug: fmt.Sprintf("t472-fp-%d", time.Now().UnixNano()), Name: "x", Status: "active",
				AccessStartsAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				Type:           "fleet", ParentID: pgtype.Int8{Int64: f.dist.ID, Valid: true}, BrandID: f.brandID,
				Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
			})
			return err
		})
		f.expectConstraint(t, "dealer under a fleet", "23514", "", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateOrganization(ctx, db.CreateOrganizationParams{
				Slug: fmt.Sprintf("t472-df-%d", time.Now().UnixNano()), Name: "x", Status: "active",
				AccessStartsAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				Type:           "dealer", ParentID: pgtype.Int8{Int64: fleet.ID, Valid: true}, BrandID: f.brandID,
				Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
			})
			return err
		})
		f.expectConstraint(t, "re-parent under a fleet", "23514", "", func(sp pgx.Tx) error {
			_, err := db.New(sp).UpdateOrganizationParent(ctx, db.UpdateOrganizationParentParams{
				ID: f.dealer.ID, ParentID: pgtype.Int8{Int64: fleet.ID, Valid: true},
			})
			return err
		})
		f.expectConstraint(t, "dealer turns into a fleet", "23514", "", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, "UPDATE organizations SET type = 'fleet', parent_id = NULL WHERE id = $1", f.dealer2.ID)
			return err
		})
	})

	t.Run("profile: fleet org only, tax number shape, contact and report fields", func(t *testing.T) {
		f.expectConstraint(t, "profile on a dealer", "23514", "", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateFleetProfile(ctx, fleetProfileParams(f.dealer, "1089325650"))
			return err
		})
		for name, tax := range map[string]string{"short": "123", "letters": "10893256AB", "nine": "123456789"} {
			f.expectConstraint(t, name, "23514", "chk_fleet_profiles_tax_number", func(sp pgx.Tx) error {
				_, err := db.New(sp).CreateFleetProfile(ctx, fleetProfileParams(fleet, tax))
				return err
			})
		}
		f.expectConstraint(t, "phone not E.164", "23514", "chk_fleet_profiles_contact_phone", func(sp pgx.Tx) error {
			arg := fleetProfileParams(fleet, "1089325650")
			arg.ContactPhone = text("0532 000 00 00")
			_, err := db.New(sp).CreateFleetProfile(ctx, arg)
			return err
		})
		f.expectConstraint(t, "weekly report", "23514", "chk_fleet_profiles_report_frequency", func(sp pgx.Tx) error {
			arg := fleetProfileParams(fleet, "1089325650")
			arg.ReportFrequency = "weekly"
			_, err := db.New(sp).CreateFleetProfile(ctx, arg)
			return err
		})
		p, err := f.q.CreateFleetProfile(ctx, fleetProfileParams(fleet, "1089325650"))
		if err != nil || p.ReportFrequency != "monthly" {
			t.Fatalf("profile = %+v, %v", p, err)
		}
		f.expectConstraint(t, "second profile", "23505", "uq_fleet_profiles_org", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateFleetProfile(ctx, fleetProfileParams(fleet, "9249799759"))
			return err
		})
		up, err := f.q.UpdateFleetProfile(ctx, db.UpdateFleetProfileParams{
			OrganizationID: fleet.ID, ReportFrequency: text("quarterly"), BillingEmail: text("fatura@example.test"),
		})
		if err != nil || up.ReportFrequency != "quarterly" || up.BillingEmail.String != "fatura@example.test" || up.LegalName != p.LegalName {
			t.Fatalf("update = %+v, %v", up, err)
		}
		up, err = f.q.UpdateFleetProfile(ctx, db.UpdateFleetProfileParams{OrganizationID: fleet.ID, ClearBillingEmail: true})
		if err != nil || up.BillingEmail.Valid || up.ReportFrequency != "quarterly" {
			t.Fatalf("clear e-mail = %+v, %v", up, err)
		}
	})

	t.Run("links: fleet to dealer or distributor, creator is a party, one open per pair", func(t *testing.T) {
		f.expectConstraint(t, "fleet side is a dealer", "23514", "", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateFleetDealerLink(ctx, fleetLinkParams(f.dealer, f.dealer2, "pending"))
			return err
		})
		f.expectConstraint(t, "dealer side is the center", "23514", "", func(sp pgx.Tx) error {
			center, err := db.New(sp).GetOrganizationByID(ctx, f.centerID)
			if err != nil {
				return err
			}
			_, err = db.New(sp).CreateFleetDealerLink(ctx, fleetLinkParams(fleet, center, "pending"))
			return err
		})
		f.expectConstraint(t, "creator outside the link", "23514", "chk_fleet_dealer_links_creator", func(sp pgx.Tx) error {
			arg := fleetLinkParams(fleet, f.dealer, "pending")
			arg.CreatedByOrgID = f.dist.ID
			_, err := db.New(sp).CreateFleetDealerLink(ctx, arg)
			return err
		})
		viaDist, err := f.q.CreateFleetDealerLink(ctx, fleetLinkParams(fleet, f.dist, "active"))
		if err != nil || !viaDist.StartedAt.Valid {
			t.Fatalf("distributor link = %+v, %v", viaDist, err)
		}
		l, err := f.q.CreateFleetDealerLink(ctx, fleetLinkParams(fleet, f.dealer, "pending"))
		if err != nil || l.StartedAt.Valid {
			t.Fatalf("pending link = %+v, %v", l, err)
		}
		f.expectConstraint(t, "second open link", "23505", "uq_fleet_dealer_links_open", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateFleetDealerLink(ctx, fleetLinkParams(fleet, f.dealer, "active"))
			return err
		})
		f.expectConstraint(t, "ended without ended_at", "23514", "chk_fleet_dealer_links_ended", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, "UPDATE fleet_dealer_links SET status = 'ended' WHERE id = $1", l.ID)
			return err
		})

		// The cari is the fleet's cari in the dealer's ledger.
		other := f.fleetOrg(t, "filo2")
		wrong, err := f.q.CreateCariAccount(ctx, db.CreateCariAccountParams{
			OrganizationID: f.dealer.ID, BrandID: f.brandID, CounterpartyType: "organization",
			CounterpartyOrgID: pgtype.Int8{Int64: other.ID, Valid: true}, Currency: f.dealer.Currency,
		})
		if err != nil {
			t.Fatalf("cari: %v", err)
		}
		f.expectConstraint(t, "cari of another fleet", "23514", "", func(sp pgx.Tx) error {
			_, err := db.New(sp).SetFleetDealerLinkCari(ctx, db.SetFleetDealerLinkCariParams{
				ID: l.ID, CariAccountID: pgtype.Int8{Int64: wrong.ID, Valid: true},
			})
			return err
		})
		distCari, err := f.q.CreateCariAccount(ctx, db.CreateCariAccountParams{
			OrganizationID: f.dist.ID, BrandID: f.brandID, CounterpartyType: "organization",
			CounterpartyOrgID: pgtype.Int8{Int64: fleet.ID, Valid: true}, Currency: f.dist.Currency,
		})
		if err != nil {
			t.Fatalf("dist cari: %v", err)
		}
		f.expectConstraint(t, "cari of another ledger", "23503", "fk_fleet_dealer_links_cari", func(sp pgx.Tx) error {
			_, err := db.New(sp).SetFleetDealerLinkCari(ctx, db.SetFleetDealerLinkCariParams{
				ID: l.ID, CariAccountID: pgtype.Int8{Int64: distCari.ID, Valid: true},
			})
			return err
		})
		right, err := f.q.CreateCariAccount(ctx, db.CreateCariAccountParams{
			OrganizationID: f.dealer.ID, BrandID: f.brandID, CounterpartyType: "organization",
			CounterpartyOrgID: pgtype.Int8{Int64: fleet.ID, Valid: true}, Currency: f.dealer.Currency,
		})
		if err != nil {
			t.Fatalf("fleet cari: %v", err)
		}
		got, err := f.q.SetFleetDealerLinkCari(ctx, db.SetFleetDealerLinkCariParams{
			ID: l.ID, CariAccountID: pgtype.Int8{Int64: right.ID, Valid: true},
		})
		if err != nil || got.CariAccountID.Int64 != right.ID {
			t.Fatalf("set cari = %+v, %v", got, err)
		}
		// Set once: a second call matches no row.
		if _, err := f.q.SetFleetDealerLinkCari(ctx, db.SetFleetDealerLinkCariParams{
			ID: l.ID, CariAccountID: pgtype.Int8{Int64: right.ID, Valid: true},
		}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("second set cari: err = %v", err)
		}
		links, err := f.q.ListFleetDealerLinks(ctx, db.ListFleetDealerLinksParams{FleetOrgID: fleet.ID, Statuses: []string{"pending"}})
		if err != nil || len(links) != 1 || links[0].DealerUuid != f.dealer.Uuid || links[0].DealerType != "dealer" {
			t.Fatalf("pending links = %+v, %v", links, err)
		}
	})

	t.Run("vehicles: fleet_org_id points at a fleet", func(t *testing.T) {
		u := f.campaignUser(t, "")
		v, err := f.q.CreateVehicle(ctx, db.CreateVehicleParams{UserID: u.ID, BrandID: f.brandID})
		if err != nil {
			t.Fatal(err)
		}
		f.expectConstraint(t, "vehicle of a dealer org as fleet", "23514", "", func(sp pgx.Tx) error {
			_, err := db.New(sp).SetVehicleFleet(ctx, db.SetVehicleFleetParams{
				ID: v.ID, FleetOrgID: pgtype.Int8{Int64: f.dealer.ID, Valid: true},
			})
			return err
		})
		got, err := f.q.SetVehicleFleet(ctx, db.SetVehicleFleetParams{ID: v.ID, FleetOrgID: pgtype.Int8{Int64: fleet.ID, Valid: true}})
		if err != nil || got.FleetOrgID.Int64 != fleet.ID {
			t.Fatalf("vehicle fleet = %+v, %v", got, err)
		}
		if n, err := f.q.CountFleetVehicles(ctx, db.CountFleetVehiclesParams{FleetOrgID: fleet.ID}); err != nil || n != 1 {
			t.Fatalf("fleet vehicles = %d, %v", n, err)
		}
	})

	t.Run("reports: one per period, ready needs a file, e-mail after ready", func(t *testing.T) {
		start := pgtype.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Valid: true}
		end := pgtype.Date{Time: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), Valid: true}
		arg := db.UpsertFleetReportParams{
			FleetOrgID: fleet.ID, BrandID: f.brandID, PeriodKind: "monthly", PeriodStart: start, PeriodEnd: end, Locale: "tr",
		}
		r, err := f.q.UpsertFleetReport(ctx, arg)
		if err != nil || r.Status != "pending" {
			t.Fatalf("report = %+v, %v", r, err)
		}
		f.expectConstraint(t, "ready without file", "23514", "chk_fleet_reports_storage_key", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, "UPDATE fleet_reports SET status = 'ready' WHERE id = $1", r.ID)
			return err
		})
		if _, err := f.q.MarkFleetReportEmailed(ctx, r.ID); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("e-mail a pending report: err = %v", err)
		}
		if _, err := f.q.MarkFleetReportFailed(ctx, db.MarkFleetReportFailedParams{ID: r.ID, Error: text("gotenberg down")}); err != nil {
			t.Fatal(err)
		}
		again, err := f.q.UpsertFleetReport(ctx, arg)
		if err != nil || again.ID != r.ID || again.Status != "pending" || again.Error.Valid {
			t.Fatalf("rerun of a failed period = %+v, %v", again, err)
		}
		ready, err := f.q.MarkFleetReportReady(ctx, db.MarkFleetReportReadyParams{ID: r.ID, StorageKey: text("fleet-reports/x.pdf")})
		if err != nil || ready.Status != "ready" {
			t.Fatalf("ready = %+v, %v", ready, err)
		}
		if kept, err := f.q.UpsertFleetReport(ctx, arg); err != nil || kept.Status != "ready" || kept.StorageKey.String != "fleet-reports/x.pdf" {
			t.Fatalf("rerun of a ready period = %+v, %v", kept, err)
		}
		mailed, err := f.q.MarkFleetReportEmailed(ctx, r.ID)
		if err != nil || !mailed.EmailedAt.Valid {
			t.Fatalf("emailed = %+v, %v", mailed, err)
		}
		if n, err := f.q.CountFleetReports(ctx, db.CountFleetReportsParams{FleetOrgID: fleet.ID, Statuses: []string{"ready"}}); err != nil || n != 1 {
			t.Fatalf("ready reports = %d, %v", n, err)
		}
		f.expectConstraint(t, "fleet report of a dealer", "23514", "", func(sp pgx.Tx) error {
			bad := arg
			bad.FleetOrgID = f.dealer.ID
			_, err := db.New(sp).UpsertFleetReport(ctx, bad)
			return err
		})
	})
}

// Regression: the organization tree queries never return a fleet.
func TestFleetOutsideOrganizationTree(t *testing.T) {
	f := newOrderFixture(t)
	ctx := f.ctx
	fleet := f.fleetOrg(t, "t472-agac-filo")
	center, err := f.q.GetOrganizationByID(ctx, f.centerID)
	if err != nil {
		t.Fatal(err)
	}
	ids := func(orgs []db.Organization) []int64 {
		out := make([]int64, 0, len(orgs))
		for _, o := range orgs {
			out = append(out, o.ID)
		}
		return out
	}

	// Subtree of the center: the dealers are there, the fleet is not.
	desc, err := f.q.Descendants(ctx, center.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(desc); !slices.Contains(got, f.dealer.ID) || slices.Contains(got, fleet.ID) {
		t.Fatalf("center descendants contain dealer=%v fleet=%v", slices.Contains(got, f.dealer.ID), slices.Contains(got, fleet.ID))
	}

	// Brand scope (center) and an explicit id set.
	brand := pgtype.Int8{Int64: f.brandID, Valid: true}
	scoped, err := f.q.ListOrganizationsInScope(ctx, db.ListOrganizationsInScopeParams{
		BrandID: brand, Q: text("t472-agac"), LimitCount: 50,
	})
	if err != nil || len(scoped) != 0 {
		t.Fatalf("brand scope = %d rows, %v", len(scoped), err)
	}
	scoped, err = f.q.ListOrganizationsInScope(ctx, db.ListOrganizationsInScopeParams{
		OrgIds: []int64{fleet.ID, f.dealer.ID}, LimitCount: 50,
	})
	if err != nil || len(scoped) != 1 || scoped[0].Organization.ID != f.dealer.ID {
		t.Fatalf("id scope = %+v, %v", scoped, err)
	}
	scoped, err = f.q.ListOrganizationsInScope(ctx, db.ListOrganizationsInScopeParams{
		BrandID: brand, Type: text("fleet"), LimitCount: 50,
	})
	if err != nil || len(scoped) != 0 {
		t.Fatalf("type=fleet scope = %d rows, %v", len(scoped), err)
	}

	// Platform list and count, even when asked for the fleet type.
	for _, types := range [][]string{nil, {"fleet"}} {
		list, err := f.q.ListOrganizationsFiltered(ctx, db.ListOrganizationsFilteredParams{
			BrandID: brand, Types: types, Q: text("t472-agac"), SortKey: "name", LimitCount: 50,
		})
		if err != nil || len(list) != 0 {
			t.Fatalf("platform list types=%v = %d rows, %v", types, len(list), err)
		}
		n, err := f.q.CountOrganizations(ctx, db.CountOrganizationsParams{BrandID: brand, Types: types, Q: text("t472-agac")})
		if err != nil || n != 0 {
			t.Fatalf("platform count types=%v = %d, %v", types, n, err)
		}
	}

	// Tree card, children of the center and the search index.
	if _, err := f.q.GetOrganizationTreeByUUID(ctx, fleet.Uuid); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("tree card of a fleet: err = %v", err)
	}
	children, err := f.q.ListOrganizationChildren(ctx, pgtype.Int8{Int64: center.ID, Valid: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range children {
		if c.Organization.ID == fleet.ID {
			t.Fatal("fleet listed as a child of the center")
		}
	}
	// TEC-473: a fleet is indexed with type fleet; its organization ids are
	// the dealers with an active link (none here), never the tree.
	doc, err := f.q.GetOrganizationForIndex(ctx, fleet.Uuid)
	if err != nil || doc.Type != "fleet" || len(doc.LinkedOrgIds) != 0 {
		t.Fatalf("index document of a fleet = %+v, %v", doc, err)
	}
	all, err := f.q.ListOrganizationsForIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range all {
		if r.ID == fleet.ID && (r.Type != "fleet" || len(r.LinkedOrgIds) != 0) {
			t.Fatalf("fleet index row = %+v", r)
		}
	}

	// Public dealer list with the access window and contract check.
	area, err := f.q.ListAreaDealers(ctx, db.ListAreaDealersParams{BrandID: f.brandID, City: "Fleetcity", LimitCount: 50})
	if err != nil || len(area) != 0 {
		t.Fatalf("area dealers = %+v, %v", area, err)
	}
}
