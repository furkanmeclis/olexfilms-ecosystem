package usecase

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func (a *appFixture) visitorInput(phone string) VisitorLeadInput {
	return VisitorLeadInput{
		PhoneE164: phone, Name: "Ayşe Yılmaz", City: strings.ToUpper(a.berlin.Name), Vehicle: "BMW 3.20i 2022",
		Intent: IntentAppointment, Language: "de", ConversationUUID: "c0ffee",
		Notice: &KVKKNotice{Locale: "en", Version: 1, SentAt: time.Now().Add(-time.Minute)},
	}
}

func (a *appFixture) countVisitorLeads(phone string) int {
	a.t.Helper()
	var n int
	if err := a.tx.QueryRow(a.ctx, `SELECT COUNT(*) FROM leads WHERE brand_id = $1 AND candidate_phone_e164 = $2`,
		a.brand, phone).Scan(&n); err != nil {
		a.t.Fatal(err)
	}
	return n
}

// TEC-397 acceptance: no lead is written before the KVKK notice was sent.
func TestVisitorLeadNeedsKVKKNotice(t *testing.T) {
	a := newAppFixture(t)
	phone := "+4915112" + time.Now().Format("150405")
	in := a.visitorInput(phone)
	in.Notice = nil
	if _, err := a.apps.UpsertVisitorLead(a.ctx, a.brand, in); !errors.Is(err, ErrKVKKNoticeRequired) {
		t.Fatalf("err = %v, want ErrKVKKNoticeRequired", err)
	}
	if n := a.countVisitorLeads(phone); n != 0 {
		t.Fatalf("leads = %d, want 0", n)
	}
}

// TEC-397 acceptance: a visitor lead is a whatsapp customer lead routed by
// the territory rule; a second request of the number within 30 days
// updates it instead of opening a second one; after 30 days a new lead
// opens.
func TestVisitorLeadOncePer30Days(t *testing.T) {
	a := newAppFixture(t)
	phone := "+4915113" + time.Now().Format("150405")
	first, err := a.apps.UpsertVisitorLead(a.ctx, a.brand, a.visitorInput(phone))
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	l := first.Lead
	if !first.Created || first.RoutedBy != RoutedByTerritory || l.OrganizationID != a.dist.ID ||
		l.TargetType != TargetCustomer || l.Source != SourceWhatsApp || l.ProvinceID.Int64 != a.berlin.ID ||
		l.CandidateContactName.String != "Ayşe Yılmaz" || !strings.Contains(l.Notes, "BMW 3.20i") {
		t.Fatalf("first = %+v", first)
	}

	in := a.visitorInput(phone)
	in.Name, in.Intent, in.Vehicle = "Ayşe Y.", IntentQuote, "Audi A4"
	second, err := a.apps.UpsertVisitorLead(a.ctx, a.brand, in)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Created || second.Lead.ID != l.ID || second.Lead.CandidateContactName.String != "Ayşe Y." ||
		!strings.Contains(second.Lead.Notes, "BMW") || !strings.Contains(second.Lead.Notes, "Audi A4") {
		t.Fatalf("second = %+v, want an update of lead %d", second, l.ID)
	}
	if n := a.countVisitorLeads(phone); n != 1 {
		t.Fatalf("leads = %d, want 1", n)
	}
	var events int
	if err := a.tx.QueryRow(a.ctx, `SELECT COUNT(*) FROM lead_events WHERE lead_id = $1 AND event_type = 'message'
		AND payload->>'kind' = 'whatsapp' AND payload->>'kvkk_notice_version' = '1'`, l.ID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 2 {
		t.Fatalf("whatsapp timeline events = %d, want 2", events)
	}

	// 31 days later the number opens a new lead.
	if _, err := a.tx.Exec(a.ctx, `UPDATE leads SET created_at = NOW() - INTERVAL '31 days' WHERE id = $1`, l.ID); err != nil {
		t.Fatal(err)
	}
	third, err := a.apps.UpsertVisitorLead(a.ctx, a.brand, a.visitorInput(phone))
	if err != nil {
		t.Fatalf("third: %v", err)
	}
	if !third.Created || third.Lead.ID == l.ID || a.countVisitorLeads(phone) != 2 {
		t.Fatalf("third = %+v, want a new lead", third)
	}
}

// An unknown city still stores the lead, at the brand center.
func TestVisitorLeadUnknownCityGoesToCenter(t *testing.T) {
	a := newAppFixture(t)
	in := a.visitorInput("+4915114" + time.Now().Format("150405"))
	in.City = "Atlantis Nowhere"
	res, err := a.apps.UpsertVisitorLead(a.ctx, a.brand, in)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created || res.RoutedBy != RoutedByCenter || res.OrganizationID != a.center.ID || res.Lead.CountryID.Valid {
		t.Fatalf("result = %+v", res)
	}
	in.Name = ""
	if _, err := a.apps.UpsertVisitorLead(a.ctx, a.brand, in); err == nil {
		t.Fatal("empty name accepted")
	}
}
