package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type f3LeadView struct {
	UUID         uuid.UUID `json:"uuid"`
	TargetType   string    `json:"target_type"`
	Source       string    `json:"source"`
	Status       string    `json:"status"`
	Organization struct {
		UUID uuid.UUID `json:"uuid"`
	} `json:"organization"`
}

type f3LeadPage struct {
	Items []f3LeadView `json:"items"`
	Total int64        `json:"total"`
}

type f3QuoteView struct {
	UUID       uuid.UUID `json:"uuid"`
	Status     string    `json:"status"`
	GrandTotal string    `json:"grand_total"`
	Lines      []struct {
		LineType    string `json:"line_type"`
		Description string `json:"description"`
		UnitPrice   string `json:"unit_price"`
	} `json:"lines"`
}

type f3QuoteSent struct {
	Quote     f3QuoteView `json:"quote"`
	PublicURL string      `json:"public_url"`
}

type f3PublicQuote struct {
	UUID             uuid.UUID `json:"uuid"`
	OrganizationName string    `json:"organization_name"`
	GrandTotal       string    `json:"grand_total"`
	Lines            []struct {
		LineType    string `json:"line_type"`
		Description string `json:"description"`
		UnitPrice   string `json:"unit_price"`
	} `json:"lines"`
}

type f3ConvertResult struct {
	Lead         f3LeadView `json:"lead"`
	Organization struct {
		UUID   uuid.UUID `json:"uuid"`
		Type   string    `json:"type"`
		Status string    `json:"status"`
	} `json:"organization"`
}

type f3LeadEvents struct {
	Items []struct {
		EventType string         `json:"event_type"`
		Payload   map[string]any `json:"payload"`
	} `json:"items"`
	Total int64 `json:"total"`
}

func f3Numeric(t *testing.T, s string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		t.Fatal(err)
	}
	return n
}

func f3Decode[T any](t *testing.T, code int, env envelope, want int) T {
	t.Helper()
	if code != want {
		t.Fatalf("HTTP %d, want %d, error=%s data=%s", code, want, errCode(env), env.Data)
	}
	var out T
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decode %T from %s: %v", out, env.Data, err)
	}
	return out
}

func f3LeadInPage(page f3LeadPage, id uuid.UUID) bool {
	for _, item := range page.Items {
		if item.UUID == id {
			return true
		}
	}
	return false
}

func f3EventKind(eventType string, payload map[string]any) string {
	switch eventType {
	case "created", "status_changed", "converted":
		return eventType
	}
	if kind, _ := payload["kind"].(string); kind != "" {
		return kind
	}
	return eventType
}

func f3AssertEventSubsequence(t *testing.T, evs f3LeadEvents, want []string) {
	t.Helper()
	var got []string
	for _, ev := range evs.Items {
		got = append(got, f3EventKind(ev.EventType, ev.Payload))
	}
	pos := 0
	for _, kind := range got {
		if pos < len(want) && kind == want[pos] {
			pos++
		}
	}
	if pos != len(want) {
		t.Fatalf("timeline kinds = %v, want subsequence %v", got, want)
	}
}

// TEC-321 / F3-03j: a Berlin public dealer application is routed to the
// German distributor, quoted, viewed through the public token, accepted and
// converted to a read-only dealer below that distributor.
func TestIntegrationF3DealerApplication(t *testing.T) {
	it := newIntegrationWith(t, func(c *config.Config) {
		c.Leads.ApplicationIPLimit = 20
		c.Leads.ApplicationPhoneLimit = 20
		c.Leads.ApplicationRateWindow = time.Hour
	})
	ctx := context.Background()
	key := sysconfig.KeyLeadsDealerApplicationEnabled
	cleanupSetting := func() { _, _ = it.pool.Exec(ctx, "DELETE FROM system_settings WHERE key = $1", key) }
	cleanupSetting()
	t.Cleanup(cleanupSetting)

	center := it.brandCenter("olex")
	dist := it.org("t321-de-dist", "distributor", center)
	distOwner, distPW := it.user("t321-dist-owner")
	it.member(dist, distOwner, "owner", rbac.RoleDistributorOwner)
	centerStaff, staffPW := it.user("t321-center-staff")
	it.member(center, centerStaff, "staff", rbac.RoleCenterStaff)
	centerSocial, socialPW := it.user("t321-center-social")
	it.member(center, centerSocial, "staff", rbac.RoleCenterSocial)

	de, err := it.q.GetCountryByISO2(ctx, "DE")
	if err != nil {
		t.Fatal(err)
	}
	prov, err := it.q.CreateProvince(ctx, db.CreateProvinceParams{
		CountryID: de.ID, Code: "T321" + it.suffix[len(it.suffix)-6:], Name: "Berlin " + it.suffix,
	})
	if err != nil {
		t.Fatalf("province: %v", err)
	}
	if _, err := it.q.CreateTerritory(ctx, db.CreateTerritoryParams{
		BrandID: center.BrandID, OrganizationID: dist.ID, CountryID: de.ID,
		ProvinceID: pgtype.Int8{Int64: prov.ID, Valid: true},
	}); err != nil {
		t.Fatalf("territory: %v", err)
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(ctx, "DELETE FROM territories WHERE organization_id = $1", dist.ID)
		_, _ = it.pool.Exec(ctx, "DELETE FROM provinces WHERE id = $1", prov.ID)
	})

	item, err := it.q.CreateServiceCatalogItem(ctx, db.CreateServiceCatalogItemParams{
		OrganizationID: center.ID, BrandID: center.BrandID,
		Name: "Dealer onboarding " + it.suffix, Description: "F3 integration test item",
		Category: "training", DefaultPrice: f3Numeric(t, "1200.00"), Currency: dist.Currency,
		Recurrence: "one_time", CancellationFee: f3Numeric(t, "0.00"), IsActive: true,
	})
	if err != nil {
		t.Fatalf("service catalog item: %v", err)
	}
	t.Cleanup(func() { _, _ = it.pool.Exec(ctx, "DELETE FROM service_catalog_items WHERE id = $1", item.ID) })

	admin := it.adminToken()
	if code, env := it.do("PUT", "/v1/platform/system-settings/"+key, hostOlex, admin, map[string]any{"value": true}); code != http.StatusOK {
		t.Fatalf("open dealer applications: %d %s", code, errCode(env))
	}

	company := "Berlin PPF Partner " + it.suffix
	phone := "+4930" + it.suffix[len(it.suffix)-8:]
	body := map[string]any{
		"company_name": company, "contact_name": "Max Berliner",
		"phone": phone, "email": "max.berliner." + it.suffix + "@example.test",
		"country_id": de.ID, "province_id": prov.ID,
		"message": "Wir möchten Olexfilms Händler werden.", "kvkk_consent": true, "language": "de",
	}
	if rec := it.dealerApplication("198.51.100.21", body); rec.Code != http.StatusAccepted {
		t.Fatalf("submit application = %d %s", rec.Code, rec.Body)
	}
	var lead db.Lead
	if err := it.pool.QueryRow(ctx, `SELECT id, uuid, organization_id, target_type, source, status
		FROM leads WHERE candidate_company_name = $1`, company).Scan(
		&lead.ID, &lead.Uuid, &lead.OrganizationID, &lead.TargetType, &lead.Source, &lead.Status); err != nil {
		t.Fatalf("lead row: %v", err)
	}
	if lead.OrganizationID != dist.ID || lead.TargetType != "dealer_candidate" || lead.Source != "application_form" || lead.Status != "new" {
		t.Fatalf("lead = %+v, want distributor application dealer candidate", lead)
	}

	staffTok := it.loginOrg(centerStaff, staffPW, center)
	socialTok := it.loginOrg(centerSocial, socialPW, center)
	distTok := it.loginOrg(distOwner, distPW, dist)
	code, env := it.do("GET", "/v1/leads?q="+url.QueryEscape(company), hostOlex, staffTok, nil)
	centerPage := f3Decode[f3LeadPage](t, code, env, http.StatusOK)
	if centerPage.Total != 0 || f3LeadInPage(centerPage, lead.Uuid) {
		t.Fatalf("center staff page = %+v, want no distributor lead", centerPage)
	}
	code, env = it.do("GET", "/v1/leads?q="+url.QueryEscape(company), hostOlex, socialTok, nil)
	socialPage := f3Decode[f3LeadPage](t, code, env, http.StatusOK)
	if socialPage.Total == 0 || !f3LeadInPage(socialPage, lead.Uuid) {
		t.Fatalf("center social page = %+v, want application lead", socialPage)
	}

	overridePrice := "1500.00"
	code, env = it.do("POST", "/v1/leads/"+lead.Uuid.String()+"/quotes", hostOlex, distTok, map[string]any{
		"lines": []map[string]any{{
			"line_type": "catalog_service", "service_catalog_item_uuid": item.Uuid.String(),
			"quantity": "1", "unit_price": overridePrice,
		}},
	})
	quote := f3Decode[f3QuoteView](t, code, env, http.StatusCreated)
	if quote.Status != "draft" || quote.GrandTotal != overridePrice || len(quote.Lines) != 1 || quote.Lines[0].UnitPrice != overridePrice {
		t.Fatalf("quote = %+v, want override catalog service line", quote)
	}

	// TEC-319: the lead's quotes for the panel "Teklifler" tab.
	code, env = it.do("GET", "/v1/leads/"+lead.Uuid.String()+"/quotes", hostOlex, distTok, nil)
	leadQuotes := f3Decode[struct {
		Items []f3QuoteView `json:"items"`
		Total int64         `json:"total"`
	}](t, code, env, http.StatusOK)
	if leadQuotes.Total != 1 || len(leadQuotes.Items) != 1 || leadQuotes.Items[0].UUID != quote.UUID || leadQuotes.Items[0].GrandTotal != overridePrice {
		t.Fatalf("lead quotes = %+v", leadQuotes)
	}

	code, env = it.do("POST", "/v1/quotes/"+quote.UUID.String()+"/send", hostOlex, distTok, nil)
	sent := f3Decode[f3QuoteSent](t, code, env, http.StatusOK)
	if sent.Quote.Status != "sent" || sent.PublicURL == "" {
		t.Fatalf("sent quote = %+v", sent)
	}
	var outboxed int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events
		WHERE event_name = 'quote.sent' AND payload->'data'->>'quote_uuid' = $1`, quote.UUID.String()).Scan(&outboxed); err != nil || outboxed != 1 {
		t.Fatalf("quote.sent outbox = %d %v", outboxed, err)
	}

	var publicToken uuid.UUID
	if err := it.pool.QueryRow(ctx, `SELECT public_token FROM quotes WHERE uuid = $1`, quote.UUID).Scan(&publicToken); err != nil {
		t.Fatalf("quote public token: %v", err)
	}
	code, env = it.do("GET", "/v1/public/quotes/"+publicToken.String(), hostOlex, "", nil)
	publicQuote := f3Decode[f3PublicQuote](t, code, env, http.StatusOK)
	if publicQuote.UUID != quote.UUID || publicQuote.OrganizationName != dist.Name || publicQuote.GrandTotal != overridePrice ||
		len(publicQuote.Lines) != 1 || publicQuote.Lines[0].UnitPrice != overridePrice {
		t.Fatalf("public quote = %+v", publicQuote)
	}

	code, env = it.do("POST", "/v1/quotes/"+quote.UUID.String()+"/accept", hostOlex, distTok, nil)
	accepted := f3Decode[f3QuoteView](t, code, env, http.StatusOK)
	if accepted.Status != "accepted" {
		t.Fatalf("accepted quote = %+v", accepted)
	}
	code, env = it.do("POST", "/v1/leads/"+lead.Uuid.String()+"/convert", hostOlex, distTok, map[string]any{
		"kind": "dealer_candidate",
	})
	converted := f3Decode[f3ConvertResult](t, code, env, http.StatusOK)
	if converted.Lead.Status != "won" || converted.Organization.Type != "dealer" || converted.Organization.Status != "read_only" {
		t.Fatalf("converted = %+v, want read-only dealer under distributor %d", converted, dist.ID)
	}
	var org db.Organization
	if err := it.pool.QueryRow(ctx, `SELECT id, type, parent_id, status, contract_pdf_key, contract_valid_until
		FROM organizations WHERE uuid = $1`, converted.Organization.UUID).Scan(
		&org.ID, &org.Type, &org.ParentID, &org.Status, &org.ContractPdfKey, &org.ContractValidUntil); err != nil {
		t.Fatalf("converted org row: %v", err)
	}
	if org.Type != "dealer" || !org.ParentID.Valid || org.ParentID.Int64 != dist.ID || org.Status != "read_only" ||
		org.ContractPdfKey.Valid || org.ContractValidUntil.Valid {
		t.Fatalf("converted org row = %+v, want K23 read-only without contract", org)
	}

	code, env = it.do("GET", "/v1/leads/"+lead.Uuid.String()+"/events", hostOlex, distTok, nil)
	events := f3Decode[f3LeadEvents](t, code, env, http.StatusOK)
	f3AssertEventSubsequence(t, events, []string{"created", "quote_sent", "quote_viewed", "status_changed", "converted"})

	var gotStatus string
	if err := it.pool.QueryRow(ctx, `SELECT status FROM leads WHERE id = $1`, lead.ID).Scan(&gotStatus); err != nil {
		t.Fatal(err)
	} else if !strings.EqualFold(gotStatus, "won") {
		t.Fatalf("lead status = %s, want won", gotStatus)
	}
}
