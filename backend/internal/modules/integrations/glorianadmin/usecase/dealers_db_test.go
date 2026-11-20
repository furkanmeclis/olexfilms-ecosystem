package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/glorianadmin/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-1265 (karar 6) against a migrated PostgreSQL (TEST_DATABASE_URL), in
// a rolled-back transaction: the dealer match list, the manual link, the
// organization dealer code and their conflicts.
func TestDealerMatchAdmin(t *testing.T) {
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
	box, err := crypto.NewSecretBox("test-encryption-key-32-bytes!!!!")
	if err != nil {
		t.Fatal(err)
	}
	brand, err := q.GetBrandBySlug(ctx, "glorian")
	if err != nil {
		t.Fatalf("glorian brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("glorian center: %v", err)
	}
	olexBrand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatal(err)
	}
	olexCenter, err := q.GetBrandCenter(ctx, olexBrand.ID)
	if err != nil {
		t.Fatal(err)
	}
	svc := usecase.New(q, box, nil, nil, nil)
	sp := func(fn func(s *usecase.Service) error) error {
		t.Helper()
		stx, err := tx.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := fn(usecase.New(db.New(stx), box, nil, nil, nil)); err != nil {
			_ = stx.Rollback(ctx)
			return err
		}
		return stx.Commit(ctx)
	}

	if _, _, err := svc.ListDealers(ctx, brand, usecase.DealerFilter{}); !errors.Is(err, usecase.ErrNotConfigured) {
		t.Fatalf("list without connection = %v", err)
	}
	conn, err := glorian.NewStore(q, box).Create(ctx, glorian.ConnectionInput{
		OrganizationID: center.ID, BrandID: brand.ID, Key: glorian.ConnectionKey, BaseURL: "https://hub.example.test",
	})
	if err != nil {
		t.Fatalf("connection: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)
	newOrg := func(parent db.Organization, name, phone string) db.Organization {
		t.Helper()
		b, err := q.GetBrandByID(ctx, parent.BrandID)
		if err != nil {
			t.Fatal(err)
		}
		o, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
			Slug: fmt.Sprintf("t1265-%s-%d", suffix, time.Now().UnixNano()), Name: name + " " + suffix, Status: "active",
			Phone: phone, AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
			Type: "distributor", ParentID: pgtype.Int8{Int64: parent.ID, Valid: true},
			BrandID: parent.BrandID, Currency: b.Currency, Locale: "tr", Timezone: "Europe/Istanbul",
			Settings: []byte("{}"),
		})
		if err != nil {
			t.Fatalf("org: %v", err)
		}
		return o
	}
	newParty := func(remote, name, code, phone string) db.IntegrationExternalParty {
		t.Helper()
		p, err := q.UpsertIntegrationExternalParty(ctx, db.UpsertIntegrationExternalPartyParams{
			OrganizationID: conn.OrganizationID, BrandID: conn.BrandID, ConnectionID: conn.ID,
			RemoteID: remote, Name: name + " " + suffix, Active: true,
			DealerCode: pgtype.Text{String: code, Valid: code != ""},
			PhoneE164:  pgtype.Text{String: phone, Valid: phone != ""},
		})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	phone := fmt.Sprintf("+90532%07d", time.Now().UnixNano()%10_000_000)
	a := newOrg(center, "T1265 Ankara", phone)
	b := newOrg(center, "T1265 Bursa", "")
	foreign := newOrg(olexCenter, "T1265 Olex", "")
	pa := newParty("d1", "Ankara Bayi", "", phone)
	pb := newParty("d2", "Bursa Bayi", "BRS-"+suffix, "")
	newParty("d3", "Çorum Bayi", "", "")

	list := func(f usecase.DealerFilter) map[string]usecase.DealerView {
		t.Helper()
		items, total, err := svc.ListDealers(ctx, brand, f)
		if err != nil {
			t.Fatal(err)
		}
		if int(total) != len(items) {
			t.Fatalf("total %d != %d items", total, len(items))
		}
		out := map[string]usecase.DealerView{}
		for _, v := range items {
			out[v.RemoteID] = v
		}
		return out
	}
	got := list(usecase.DealerFilter{SortKey: "name"})
	if len(got) != 3 || got["d1"].MatchType != usecase.MatchPhone || got["d1"].Organization.UUID != a.Uuid ||
		got["d2"].MatchType != usecase.MatchNone || got["d3"].Organization != nil {
		t.Fatalf("initial list = %+v", got)
	}

	// Dealer code on Bursa: d2 matches by code.
	code := "brs-" + suffix
	org, err := svc.SetDealerCode(ctx, brand, b.Uuid, usecase.DealerCodeInput{GlorianDealerCode: &code})
	if err != nil || org.GlorianDealerCode == nil || *org.GlorianDealerCode != code {
		t.Fatalf("set code = %+v, %v", org, err)
	}
	if v := list(usecase.DealerFilter{MatchTypes: []string{usecase.MatchCode}}); len(v) != 1 || v["d2"].Organization.UUID != b.Uuid {
		t.Fatalf("code filter = %+v", v)
	}
	// Same code on another organization of the brand (any case): 409.
	upper := "BRS-" + suffix
	if err := sp(func(s *usecase.Service) error {
		_, err := s.SetDealerCode(ctx, brand, a.Uuid, usecase.DealerCodeInput{GlorianDealerCode: &upper})
		return err
	}); !errors.Is(err, usecase.ErrDealerCodeTaken) {
		t.Fatalf("duplicate code = %v", err)
	}
	// Another brand's organization: 404.
	if _, err := svc.SetDealerCode(ctx, brand, foreign.Uuid, usecase.DealerCodeInput{GlorianDealerCode: &upper}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("foreign org code = %v", err)
	}
	long := fmt.Sprintf("%065d", 1)
	var verr *usecase.ValidationError
	if _, err := svc.SetDealerCode(ctx, brand, a.Uuid, usecase.DealerCodeInput{GlorianDealerCode: &long}); !errors.As(err, &verr) {
		t.Fatalf("long code = %v", err)
	}

	// Manual link: d3 -> Ankara; Ankara leaves the phone match.
	v, err := svc.LinkDealer(ctx, brand, newPartyUUID(t, q, conn, "d3"), usecase.DealerLinkInput{OrganizationUUID: &a.Uuid})
	if err != nil || v.MatchType != usecase.MatchManual || v.Organization == nil || v.Organization.UUID != a.Uuid || v.LinkedAt == nil {
		t.Fatalf("link = %+v, %v", v, err)
	}
	if got := list(usecase.DealerFilter{}); got["d1"].MatchType != usecase.MatchNone {
		t.Fatalf("phone party after manual link = %s", got["d1"].MatchType)
	}
	// The same organization on a second dealer: 409.
	if err := sp(func(s *usecase.Service) error {
		_, err := s.LinkDealer(ctx, brand, pa.Uuid, usecase.DealerLinkInput{OrganizationUUID: &a.Uuid})
		return err
	}); !errors.Is(err, usecase.ErrOrganizationLinked) {
		t.Fatalf("second link = %v", err)
	}
	// Another brand's organization: 422.
	if _, err := svc.LinkDealer(ctx, brand, pb.Uuid, usecase.DealerLinkInput{OrganizationUUID: &foreign.Uuid}); !errors.As(err, &verr) {
		t.Fatalf("foreign link = %v", err)
	}
	// Unlink: d1 matches Ankara by phone again.
	v, err = svc.LinkDealer(ctx, brand, newPartyUUID(t, q, conn, "d3"), usecase.DealerLinkInput{})
	if err != nil || v.MatchType != usecase.MatchNone || v.LinkedAt != nil {
		t.Fatalf("unlink = %+v, %v", v, err)
	}
	if got := list(usecase.DealerFilter{Q: "Ankara"}); len(got) != 1 || got["d1"].MatchType != usecase.MatchPhone {
		t.Fatalf("q list = %+v", got)
	}
	inactive := false
	if got := list(usecase.DealerFilter{Active: &inactive}); len(got) != 0 {
		t.Fatalf("inactive filter = %+v", got)
	}

	// Picker: the brand's organizations only.
	opts, err := svc.DealerOrganizations(ctx, brand, "T1265")
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range opts {
		if o.UUID == foreign.Uuid {
			t.Fatal("picker lists another brand's organization")
		}
	}
	if len(opts) != 2 {
		t.Fatalf("picker = %d options", len(opts))
	}
}

func newPartyUUID(t *testing.T, q *db.Queries, conn db.IntegrationConnection, remote string) uuid.UUID {
	t.Helper()
	p, err := q.GetIntegrationExternalPartyByRemoteID(context.Background(), db.GetIntegrationExternalPartyByRemoteIDParams{
		ConnectionID: conn.ID, RemoteID: remote,
	})
	if err != nil {
		t.Fatal(err)
	}
	return p.Uuid
}
