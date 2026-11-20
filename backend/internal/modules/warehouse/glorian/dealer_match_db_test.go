package glorian_test

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian/fake"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1265 (karar 6): the dealer of an order's buyer comes from
// glorian_dealer_matches: manual link, then dealer code, then phone (only
// for an organization without a code); several candidates hold.

func (f *pullFixture) codedParty(t *testing.T, remoteID, code, phone string) db.IntegrationExternalParty {
	t.Helper()
	arg := db.UpsertIntegrationExternalPartyParams{
		OrganizationID: f.conn.OrganizationID, BrandID: f.conn.BrandID, ConnectionID: f.conn.ID,
		RemoteID: remoteID, Name: f.name("T1265 Bayi " + remoteID), Active: true,
	}
	if code != "" {
		arg.DealerCode = pgtype.Text{String: code, Valid: true}
	}
	if phone != "" {
		arg.PhoneE164 = pgtype.Text{String: phone, Valid: true}
	}
	p, err := f.q.UpsertIntegrationExternalParty(f.ctx, arg)
	if err != nil {
		t.Fatalf("party %s: %v", remoteID, err)
	}
	return p
}

func (f *pullFixture) setCode(t *testing.T, org db.Organization, code string) {
	t.Helper()
	if _, err := f.q.SetOrganizationGlorianDealerCode(f.ctx, db.SetOrganizationGlorianDealerCodeParams{
		ID: org.ID, GlorianDealerCode: pgtype.Text{String: code, Valid: code != ""},
	}); err != nil {
		t.Fatalf("dealer code: %v", err)
	}
}

func (f *pullFixture) linkParty(t *testing.T, p db.IntegrationExternalParty, org *db.Organization) {
	t.Helper()
	arg := db.SetIntegrationExternalPartyLinkParams{ConnectionID: f.conn.ID, Uuid: p.Uuid}
	if org != nil {
		arg.LinkedOrganizationID = pgtype.Int8{Int64: org.ID, Valid: true}
	}
	if _, err := f.q.SetIntegrationExternalPartyLink(f.ctx, arg); err != nil {
		t.Fatalf("link: %v", err)
	}
}

// matched returns the remote ids of the dealers org matches.
func (f *pullFixture) matched(t *testing.T, org db.Organization) []string {
	t.Helper()
	rows, err := f.q.ListGlorianPartiesForOrganization(f.ctx, db.ListGlorianPartiesForOrganizationParams{
		ConnectionID: f.conn.ID, OrganizationID: org.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.RemoteID)
	}
	return out
}

func (f *pullFixture) matchType(t *testing.T, p db.IntegrationExternalParty) (string, string) {
	t.Helper()
	rows, err := f.q.ListGlorianDealerMatches(f.ctx, db.ListGlorianDealerMatchesParams{
		ConnectionID: f.conn.ID,
		MatchTypes:   []string{}, SortKey: "name", PageLimit: 500,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Uuid == p.Uuid {
			return r.MatchType, r.OrgName.String
		}
	}
	t.Fatalf("party %s not listed", p.RemoteID)
	return "", ""
}

func sameIDs(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestDealerMatchOrder(t *testing.T) {
	f := newPullFixture(t, true)
	code := "BK-" + f.suffix

	// Phone only (no code anywhere): the fallback match.
	phoneOrg := f.distributor(t, f.glorian, f.phone(1))
	phoneParty := f.codedParty(t, "p1", "", phoneOrg.Phone)
	if got := f.matched(t, phoneOrg); !sameIDs(got, "p1") {
		t.Fatalf("phone match = %v", got)
	}
	if mt, org := f.matchType(t, phoneParty); mt != "phone" || org != phoneOrg.Name {
		t.Fatalf("phone party = %s %q", mt, org)
	}

	// Code beats phone: the coded organization matches the coded dealer
	// even though another dealer carries its phone.
	codeOrg := f.distributor(t, f.glorian, f.phone(2))
	f.codedParty(t, "p2-phone", "", codeOrg.Phone)
	codeParty := f.codedParty(t, "p2-code", code, "")
	f.setCode(t, codeOrg, code)
	if got := f.matched(t, codeOrg); !sameIDs(got, "p2-code") {
		t.Fatalf("code match = %v", got)
	}
	if mt, _ := f.matchType(t, codeParty); mt != "code" {
		t.Fatalf("code party = %s", mt)
	}

	// Case-insensitive code; a coded organization never falls back to
	// the phone when its code has no dealer.
	f.setCode(t, codeOrg, "YOK-"+f.suffix)
	if got := f.matched(t, codeOrg); len(got) != 0 {
		t.Fatalf("unknown code fell back to %v", got)
	}
	f.setCode(t, codeOrg, "bk-"+f.suffix)
	if got := f.matched(t, codeOrg); !sameIDs(got, "p2-code") {
		t.Fatalf("lower-case code = %v", got)
	}

	// Manual link beats code; the coded dealer then matches nobody.
	manualParty := f.codedParty(t, "p3-manual", "", "")
	f.linkParty(t, manualParty, &codeOrg)
	if got := f.matched(t, codeOrg); !sameIDs(got, "p3-manual") {
		t.Fatalf("manual match = %v", got)
	}
	if mt, _ := f.matchType(t, codeParty); mt != "none" {
		t.Fatalf("code party after manual link = %s", mt)
	}
	if mt, org := f.matchType(t, manualParty); mt != "manual" || org != codeOrg.Name {
		t.Fatalf("manual party = %s %q", mt, org)
	}
	f.linkParty(t, manualParty, nil)
	if got := f.matched(t, codeOrg); !sameIDs(got, "p2-code") {
		t.Fatalf("after unlink = %v", got)
	}

	// Two active dealers on one organization's phone: ambiguous, held.
	f.codedParty(t, "p1-twin", "", phoneOrg.Phone)
	if got := f.matched(t, phoneOrg); len(got) != 0 {
		t.Fatalf("ambiguous phone matched %v", got)
	}
	if mt, _ := f.matchType(t, phoneParty); mt != "ambiguous" {
		t.Fatalf("twin party = %s", mt)
	}

	// Two organizations on one dealer phone: ambiguous too.
	shared := f.phone(3)
	a := f.distributor(t, f.glorian, shared)
	f.distributor(t, f.glorian, shared)
	sharedParty := f.codedParty(t, "p4", "", shared)
	if got := f.matched(t, a); len(got) != 0 {
		t.Fatalf("shared phone matched %v", got)
	}
	if mt, _ := f.matchType(t, sharedParty); mt != "ambiguous" {
		t.Fatalf("shared party = %s", mt)
	}

	// Filters: match type and active.
	rows, err := f.q.ListGlorianDealerMatches(f.ctx, db.ListGlorianDealerMatchesParams{
		ConnectionID: f.conn.ID, MatchTypes: []string{"code"}, SortKey: "name", PageLimit: 500,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Uuid != codeParty.Uuid {
		t.Fatalf("code filter = %d rows", len(rows))
	}
}

// The dealer pull stores the hub dealer code.
func TestPullStoresDealerCode(t *testing.T) {
	f := newPullFixture(t, true)
	d := f.dealer("1", "Parazit Dizayn", "+905550000001", true, "2026-09-01T09:00:00+00:00")
	d["dealer_code"] = " PD-" + f.suffix + " "
	f.srv.SetFixture(fake.FixtureDealers, []fake.Row{d})
	if err := f.puller().Run(f.ctx); err != nil {
		t.Fatalf("pull: %v", err)
	}
	p, err := f.q.GetIntegrationExternalPartyByRemoteID(f.ctx, db.GetIntegrationExternalPartyByRemoteIDParams{
		ConnectionID: f.conn.ID, RemoteID: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.DealerCode.String != "PD-"+f.suffix {
		t.Fatalf("dealer code = %q", p.DealerCode.String)
	}
}
