package pipeline_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/pipeline"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm/fake"
	"github.com/jackc/pgx/v5/pgtype"
)

// --- TEC-397 fakes ---------------------------------------------------------------

// allModules opens every module (leads routing).
type allModules struct{}

func (allModules) Enabled(context.Context, int64, string) (bool, error) { return true, nil }
func (allModules) SystemEnabled(context.Context, string) (bool, error)  { return true, nil }

type fakeBools struct{}

func (fakeBools) Bool(context.Context, string) bool { return false }

type fakeVisitorSettings struct{ cap int64 }

func (s *fakeVisitorSettings) AIVisitorDailyTokenCap(context.Context) int64 { return s.cap }

// priceProbe is a visitor tool whose result carries prices (a careless
// new tool): the visitor whitelist must strip them.
type priceProbe struct{}

func (priceProbe) Spec() aitools.Spec {
	return aitools.Spec{
		Name: "price_probe", Description: "Products.", Kind: aitools.KindRead, Realm: aitools.RealmVisitor,
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}
}

func (priceProbe) Run(context.Context, aitools.Env, json.RawMessage) (aitools.Result, error) {
	return aitools.JSONResult(map[string]any{
		"items": []any{map[string]any{"name": "PPF Mat", "price": 45000, "list_price": 52000, "currency": "TRY"}},
		"note":  "x",
	})
}

// --- helpers -----------------------------------------------------------------------

func (f *fixture) location(c db.Conversation, lat, lng float64) db.Message {
	return f.message(c, "in", model.SenderContact, "", map[string]any{
		"type": "location", "latitude": lat, "longitude": lng, "caption": "Konumum",
	})
}

// dealerAt creates an active dealer of the brand at a position.
func (f *fixture) dealerAt(name string, lat, lng float64) db.Organization {
	f.t.Helper()
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: "tec397-" + f.uniq(), Name: name, Status: "active", City: "Testşehir",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "dealer", ParentID: pgtype.Int8{Int64: f.center.ID, Valid: true},
		BrandID: f.brand.ID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		f.t.Fatal(err)
	}
	num := func(v float64) pgtype.Numeric {
		var n pgtype.Numeric
		if err := n.Scan(fmt.Sprintf("%.6f", v)); err != nil {
			f.t.Fatal(err)
		}
		return n
	}
	if o, err = f.q.UpdateOrganizationCoordinates(f.ctx, db.UpdateOrganizationCoordinatesParams{
		ID: o.ID, Latitude: num(lat), Longitude: num(lng),
	}); err != nil {
		f.t.Fatal(err)
	}
	return o
}

// modelRuns adds n finished AI runs with a model call today.
func (f *fixture) modelRuns(c db.Conversation, n int) {
	f.t.Helper()
	for range n {
		m := f.inbound(c, "soru")
		r, err := f.q.CreateConversationAIRun(f.ctx, db.CreateConversationAIRunParams{
			ConversationID: c.ID, TriggerMessageID: m.ID, OrganizationID: c.OrganizationID, BrandID: c.BrandID,
		})
		if err != nil {
			f.t.Fatal(err)
		}
		if _, err := f.q.FinishConversationAIRun(f.ctx, db.FinishConversationAIRunParams{
			ID: r.ID, Status: model.RunCompleted, Stages: []byte("[]"), ToolCalls: []byte("[]"),
			Model: "claude-sonnet-5-5", InputTokens: 10, FinishedAt: pgtype.Timestamptz{Time: f.now, Valid: true},
		}); err != nil {
			f.t.Fatal(err)
		}
	}
}

func (f *fixture) leadsOf(phone string) []db.Lead {
	f.t.Helper()
	rows, err := f.tx.Query(f.ctx, `SELECT id, organization_id, target_type, source, candidate_contact_name, notes
		FROM leads WHERE candidate_phone_e164 = $1 AND deleted_at IS NULL ORDER BY id`, phone)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	var out []db.Lead
	for rows.Next() {
		var l db.Lead
		if err := rows.Scan(&l.ID, &l.OrganizationID, &l.TargetType, &l.Source, &l.CandidateContactName, &l.Notes); err != nil {
			f.t.Fatal(err)
		}
		out = append(out, l)
	}
	return out
}

func toolResults(req llm.Request) []llm.Block {
	var out []llm.Block
	for _, m := range req.Messages {
		for _, b := range m.Content {
			if b.Type == llm.BlockToolResult {
				out = append(out, b)
			}
		}
	}
	return out
}

func leadCall(id string) fake.Turn {
	return fake.ToolCall(id, "request_dealer_contact", map[string]any{
		"name": "Mehmet Kaya", "city": "İstanbul", "vehicle": "Toyota Corolla 2023", "intent": "appointment",
	}, llm.Usage{InputTokens: 50, OutputTokens: 10})
}

// --- acceptance --------------------------------------------------------------------

// TEC-397 acceptance: a visitor answer carries no price field. A visitor
// tool returning prices reaches the model only through the whitelist.
func TestVisitorToolResultHasNoPrice(t *testing.T) {
	f := newFixture(t)
	c := f.consented(f.conversation(f.phone()))
	f.inbound(c, "Mat PPF fiyatı ne kadar?")
	f.llm.Push(
		fake.ToolCall("t1", "price_probe", map[string]any{}, llm.Usage{InputTokens: 10}),
		fake.Text("Fiyat için size en yakın bayimize yönlendirebilirim.", llm.Usage{InputTokens: 10}),
	)
	f.process(c)

	reqs := f.llm.Requests()
	if len(reqs) != 2 {
		t.Fatalf("model calls = %d, want 2", len(reqs))
	}
	res := toolResults(reqs[1])
	if len(res) != 1 || res[0].IsError {
		t.Fatalf("tool results = %+v", res)
	}
	if strings.Contains(strings.ToLower(res[0].Content), "price") || strings.Contains(res[0].Content, "45000") ||
		strings.Contains(res[0].Content, "currency") || !strings.Contains(res[0].Content, "PPF Mat") {
		t.Fatalf("visitor tool result = %s", res[0].Content)
	}
}

// TEC-397 acceptance: no lead is written before the KVKK notice was sent.
// The first request only sends the notice (in the visitor's language); a
// second call in the same turn stores nothing either; the request after
// the visitor's reply stores the customer lead and links it to the
// conversation.
func TestVisitorLeadAfterKVKKNotice(t *testing.T) {
	f := newFixture(t)
	phone := f.phone()
	c := f.consented(f.conversation(phone))
	f.inbound(c, "Randevu almak istiyorum. Adım Mehmet Kaya, İstanbul, Toyota Corolla 2023.")
	f.llm.Push(leadCall("l1"), leadCall("l2"), fake.Text("Bilgilendirme metnini gönderdim; onaylıyor musunuz?", llm.Usage{InputTokens: 10}))
	f.process(c)

	if leads := f.leadsOf(phone); len(leads) != 0 {
		t.Fatalf("leads before the notice was answered = %+v", leads)
	}
	notice, err := f.q.GetLatestKVKKNotice(f.ctx, "tr")
	if err != nil {
		t.Fatal(err)
	}
	out := f.outgoing(c)
	if len(out) != 2 || out[0].SenderType != model.SenderSystem || !strings.Contains(out[0].Body.String, notice.Body) ||
		out[1].SenderType != model.SenderAI {
		t.Fatalf("outgoing = %q, want the Turkish KVKK notice then the answer", bodies(out))
	}
	res := toolResults(f.llm.Requests()[2])
	if len(res) != 2 || !strings.Contains(res[0].Content, pipeline.LeadStatusNoticeSent) ||
		!strings.Contains(res[1].Content, pipeline.LeadStatusAwaitingReply) {
		t.Fatalf("tool results = %+v", res)
	}
	if conv, _ := f.q.GetConversationByID(f.ctx, c.ID); conv.VisitorLeadID.Valid {
		t.Fatalf("visitor_lead_id set before the lead: %+v", conv.VisitorLeadID)
	}

	// The visitor confirms; the next call stores the lead.
	f.inbound(c, "Evet, onaylıyorum.")
	f.llm.Push(leadCall("l3"), fake.Text("Talebiniz bayimize iletildi.", llm.Usage{InputTokens: 10}))
	f.process(c)
	leads := f.leadsOf(phone)
	if len(leads) != 1 || leads[0].TargetType != "customer" || leads[0].Source != "whatsapp" ||
		leads[0].CandidateContactName.String != "Mehmet Kaya" || !strings.Contains(leads[0].Notes, "Toyota Corolla") {
		t.Fatalf("leads = %+v", leads)
	}
	conv, err := f.q.GetConversationByID(f.ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !conv.VisitorLeadID.Valid || conv.VisitorLeadID.Int64 != leads[0].ID {
		t.Fatalf("visitor_lead_id = %+v, want %d", conv.VisitorLeadID, leads[0].ID)
	}
	if n := strings.Count(strings.Join(bodies(f.outgoing(c)), "\n"), notice.Body); n != 1 {
		t.Fatalf("KVKK notice sent %d times, want once", n)
	}
}

// TEC-397 acceptance: a second request of the number within 30 days opens
// no new lead; the existing one is updated.
func TestVisitorSecondRequestWithin30DaysUpdatesLead(t *testing.T) {
	f := newFixture(t)
	phone := f.phone()
	c := f.consented(f.conversation(phone))
	turn := func(msg string, calls ...fake.Turn) {
		f.inbound(c, msg)
		f.llm.Push(append(calls, fake.Text("Tamam.", llm.Usage{InputTokens: 10}))...)
		f.process(c)
	}
	turn("Teklif istiyorum", leadCall("a"))
	turn("Onaylıyorum", leadCall("b"))
	first := f.leadsOf(phone)
	if len(first) != 1 {
		t.Fatalf("leads after the first request = %+v", first)
	}
	turn("Bir de seramik kaplama için teklif alabilir miyim?", fake.ToolCall("c", "request_dealer_contact", map[string]any{
		"name": "Mehmet Kaya", "city": "İstanbul", "vehicle": "Toyota Corolla 2023", "intent": "quote", "note": "Seramik kaplama",
	}, llm.Usage{InputTokens: 10}))
	leads := f.leadsOf(phone)
	if len(leads) != 1 || leads[0].ID != first[0].ID || !strings.Contains(leads[0].Notes, "Seramik kaplama") {
		t.Fatalf("leads after the second request = %+v, want lead %d updated", leads, first[0].ID)
	}
	res := toolResults(f.llm.Requests()[len(f.llm.Requests())-1])
	if len(res) != 1 || !strings.Contains(res[0].Content, pipeline.LeadStatusUpdated) {
		t.Fatalf("tool result = %+v", res)
	}
}

// TEC-397 acceptance: a shared location is answered with the three nearest
// dealers in distance order, without a model call.
func TestLocationReturnsThreeNearestDealers(t *testing.T) {
	f := newFixture(t)
	// A remote point no other test data is near.
	const lat, lng = -47.5, -128.25
	far := f.dealerAt("TEC397 Uzak Bayi", lat+0.9, lng)      // ~100 km
	mid := f.dealerAt("TEC397 Orta Bayi", lat+0.2, lng)      // ~22 km
	near := f.dealerAt("TEC397 Yakın Bayi", lat+0.01, lng)   // ~1 km
	second := f.dealerAt("TEC397 İkinci Bayi", lat, lng+0.1) // ~7.5 km
	c := f.conversation(f.phone())                           // no consent needed: no AI
	f.location(c, lat, lng)
	f.process(c)

	if n := len(f.llm.Requests()); n != 0 {
		t.Fatalf("model calls = %d, want 0", n)
	}
	out := f.outgoing(c)
	if len(out) != 1 || out[0].SenderType != model.SenderSystem {
		t.Fatalf("outgoing = %q", bodies(out))
	}
	body := out[0].Body.String
	iNear, iSecond, iMid := strings.Index(body, near.Name), strings.Index(body, second.Name), strings.Index(body, mid.Name)
	if iNear < 0 || iSecond < 0 || iMid < 0 || iNear > iSecond || iSecond > iMid || strings.Contains(body, far.Name) {
		t.Fatalf("dealers not the 3 nearest in order:\n%s", body)
	}
	if !strings.Contains(body, "1. *"+near.Name) || !strings.Contains(body, "https://app.example.test/bayi/"+near.Slug) ||
		!strings.Contains(body, "(1.1 km)") {
		t.Fatalf("dealer line format:\n%s", body)
	}
	runs := f.runs(c)
	if len(runs) != 1 || runs[0].Model != "" {
		t.Fatalf("runs = %+v", runs)
	}
}

// TEC-397 acceptance: the 31st AI turn of a visitor number in a day gets
// the fixed text with the dealer finder link and no model call.
func TestVisitorDailyTurnLimit(t *testing.T) {
	f := newFixture(t)
	c := f.consented(f.conversation(f.phone()))
	f.modelRuns(c, pipeline.VisitorDailyTurns-1)

	f.inbound(c, "30. soru")
	f.llm.Push(fake.Text("30. yanıt", llm.Usage{InputTokens: 10}))
	f.process(c)
	if n := len(f.llm.Requests()); n != 1 {
		t.Fatalf("30th turn: model calls = %d, want 1", n)
	}

	f.inbound(c, "31. soru")
	f.process(c)
	if n := len(f.llm.Requests()); n != 1 {
		t.Fatalf("31st turn called the model (%d calls)", n)
	}
	out := f.outgoing(c)
	last := out[len(out)-1]
	if last.SenderType != model.SenderSystem || !strings.Contains(last.Body.String, "https://app.example.test/portal/dealers") ||
		!strings.Contains(last.Body.String, "Bugünlük otomatik yanıt sınırına") {
		t.Fatalf("31st turn answer = %q", last.Body.String)
	}
	runs := f.runs(c)
	if runs[0].Status != model.RunSkipped || !strings.Contains(string(runs[0].Stages), pipeline.ReasonVisitorTurns) {
		t.Fatalf("31st run = %s %s", runs[0].Status, runs[0].Stages)
	}
	if !strings.Contains(string(runs[1].Stages), `"turns": 29`) {
		t.Fatalf("30th run stages = %s", runs[1].Stages)
	}
}

// TEC-397: above ai.visitor_daily_token_cap of the system pool every
// visitor gets the fixed text.
func TestVisitorDailyTokenCap(t *testing.T) {
	f := newFixture(t)
	f.visitor.cap = 1000
	f.exec(`INSERT INTO ai_usage (organization_id, brand_id, pool, channel, purpose, model, input_tokens)
		VALUES ($1, $2, 'system', 'whatsapp', 'chat', 'claude-sonnet-5-5', 1000)`, f.center.ID, f.brand.ID)
	c := f.consented(f.conversation(f.phone()))
	f.inbound(c, "Merhaba")
	f.process(c)
	if n := len(f.llm.Requests()); n != 0 {
		t.Fatalf("model calls = %d, want 0", n)
	}
	out := f.outgoing(c)
	if len(out) != 1 || !strings.Contains(out[0].Body.String, "/portal/dealers") {
		t.Fatalf("outgoing = %q", bodies(out))
	}
	if runs := f.runs(c); !strings.Contains(string(runs[0].Stages), pipeline.ReasonVisitorTokens) {
		t.Fatalf("stages = %s", runs[0].Stages)
	}
}
