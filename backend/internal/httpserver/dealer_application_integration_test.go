package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/jackc/pgx/v5/pgtype"
)

func (it *itest) dealerApplication(ip string, body any) *httptest.ResponseRecorder {
	it.t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/public/dealer-applications", bytes.NewReader(b))
	req.Host = "backend:8080"
	req.Header.Set("X-Forwarded-Host", hostOlex)
	req.Header.Set("X-Forwarded-For", ip)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	it.handler.ServeHTTP(rec, req)
	return rec
}

// TEC-317 acceptance (HTTP): closed by default (404, config false); the
// platform admin opens it; a Berlin application lands in the German
// distributor's leads with an outbox notification event; over the per-IP
// limit the form answers 429 with Retry-After.
func TestIntegrationDealerApplication(t *testing.T) {
	it := newIntegrationWith(t, func(c *config.Config) {
		c.Leads.ApplicationIPLimit = 3
		c.Leads.ApplicationPhoneLimit = 10
		c.Leads.ApplicationRateWindow = time.Hour
	})
	ctx := context.Background()
	key := sysconfig.KeyLeadsDealerApplicationEnabled
	cleanup := func() { _, _ = it.pool.Exec(ctx, "DELETE FROM system_settings WHERE key = $1", key) }
	cleanup()
	t.Cleanup(cleanup)

	center := it.brandCenter("olex")
	dist := it.org("t317-de-dist", "distributor", center)
	de, err := it.q.GetCountryByISO2(ctx, "DE")
	if err != nil {
		t.Fatal(err)
	}
	prov, err := it.q.CreateProvince(ctx, db.CreateProvinceParams{
		CountryID: de.ID, Code: "T317" + it.suffix[len(it.suffix)-6:], Name: "Berlin " + it.suffix,
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

	body := map[string]any{
		"company_name": "Kuzey Folien " + it.suffix, "contact_name": "Max Mustermann",
		"phone": "030 12345678", "email": "max@kuzey.example", "country_id": de.ID, "province_id": prov.ID,
		"message": "Wir möchten Händler werden.", "kvkk_consent": true, "language": "de",
	}

	// Closed by default.
	if code, env := it.do("GET", "/v1/public/dealer-applications/config", hostOlex, "", nil); code != http.StatusOK ||
		string(env.Data) != `{"enabled":false}` {
		t.Fatalf("config = %d %s", code, env.Data)
	}
	if rec := it.dealerApplication("192.0.2.10", body); rec.Code != http.StatusNotFound {
		t.Fatalf("closed = %d %s", rec.Code, rec.Body)
	}

	// The platform admin opens it (leads is open system wide).
	admin := it.adminToken()
	if code, env := it.do("PUT", "/v1/platform/system-settings/"+key, hostOlex, admin, map[string]any{"value": true}); code != http.StatusOK {
		t.Fatalf("open = %d %s", code, errCode(env))
	}
	if code, env := it.do("GET", "/v1/public/dealer-applications/config", hostOlex, "", nil); code != http.StatusOK ||
		string(env.Data) != `{"enabled":true}` {
		t.Fatalf("config after open = %d %s", code, env.Data)
	}

	rec := it.dealerApplication("192.0.2.11", body)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("submit = %d %s", rec.Code, rec.Body)
	}
	var lead db.Lead
	if err := it.pool.QueryRow(ctx, `SELECT id, organization_id, target_type, source, candidate_phone_e164
		FROM leads WHERE candidate_company_name = $1`, body["company_name"]).Scan(
		&lead.ID, &lead.OrganizationID, &lead.TargetType, &lead.Source, &lead.CandidatePhoneE164); err != nil {
		t.Fatalf("lead: %v", err)
	}
	t.Cleanup(func() {
		// lead_events is append-only: the throwaway lead is only soft
		// deleted (the test database is reset between runs).
		_, _ = it.pool.Exec(ctx, "UPDATE leads SET deleted_at = NOW() WHERE id = $1", lead.ID)
	})
	if lead.OrganizationID != dist.ID || lead.TargetType != "dealer_candidate" || lead.Source != "application_form" ||
		lead.CandidatePhoneE164.String != "+493012345678" {
		t.Fatalf("lead = %+v, want DE distributor %d", lead, dist.ID)
	}
	var outboxed int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_name = $1
		AND payload->'data'->>'lead_uuid' = (SELECT uuid::text FROM leads WHERE id = $2)`,
		events.LeadsApplicationReceived, lead.ID).Scan(&outboxed); err != nil || outboxed != 1 {
		t.Fatalf("outbox = %d %v", outboxed, err)
	}

	// Validation: invalid phone, missing consent.
	bad := map[string]any{}
	for k, v := range body {
		bad[k] = v
	}
	bad["phone"] = "12"
	if rec := it.dealerApplication("192.0.2.11", bad); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad phone = %d", rec.Code)
	}
	// Third hit of the IP passes the limit, the fourth is 429.
	bad["phone"], bad["kvkk_consent"] = body["phone"], false
	if rec := it.dealerApplication("192.0.2.11", bad); rec.Code != http.StatusBadRequest {
		t.Fatalf("no consent = %d", rec.Code)
	}
	rec = it.dealerApplication("192.0.2.11", body)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("over limit = %d retry-after %q", rec.Code, rec.Header().Get("Retry-After"))
	}
}
