package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-240: dealer coordinates (tenant settings) and the public nearby
// dealers lookup. The fixtures sit in the middle of the Pacific so no other
// row of the shared test database falls inside the radius.
func TestIntegrationDealersNearby(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	olexCenter := it.brandCenter("olex")
	glorianCenter := it.brandCenter("glorian")

	near := it.org("near", "dealer", olexCenter)
	mid := it.org("mid", "distributor", olexCenter)
	far := it.org("far", "dealer", olexCenter)
	noCoords := it.org("nocoords", "dealer", olexCenter)
	passive := it.org("passive", "dealer", olexCenter)
	glorian := it.org("glorian", "dealer", glorianCenter)

	setCoords := func(o db.Organization, lat, lng float64) {
		t.Helper()
		if _, err := it.pool.Exec(ctx,
			"UPDATE organizations SET latitude = $2, longitude = $3, phone = '+905321112233' WHERE id = $1",
			o.ID, lat, lng); err != nil {
			t.Fatalf("coords %s: %v", o.Slug, err)
		}
	}
	setCoords(mid, 10.05, -150.0)
	setCoords(far, 10.10, -150.0)
	setCoords(passive, 10.02, -150.0)
	setCoords(glorian, 10.03, -150.0)
	if _, err := it.pool.Exec(ctx, "UPDATE organizations SET status = 'suspended' WHERE id = $1", passive.ID); err != nil {
		t.Fatal(err)
	}
	_ = noCoords

	// Only one of the pair violates the CHECK constraint.
	if _, err := it.pool.Exec(ctx, "UPDATE organizations SET latitude = 1 WHERE id = $1", noCoords.ID); err == nil {
		t.Fatal("latitude without longitude must violate chk_organizations_coordinates_pair")
	}

	// The owner sets the near dealer's coordinates through tenant settings.
	owner, ownerPw := it.user("geo-owner")
	it.member(near, owner, "owner")
	ownerTok := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": owner.Email.String, "password": ownerPw, "organization_slug": near.Slug,
	})).AccessToken
	code, env := it.do("PATCH", "/v1/tenant/settings", hostOlex, ownerTok, map[string]any{"latitude": 10.01, "longitude": -150.0})
	if code != http.StatusOK {
		t.Fatalf("owner coordinates: %d %s", code, errCode(env))
	}
	var settings struct {
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	}
	if err := json.Unmarshal(env.Data, &settings); err != nil || settings.Latitude == nil || *settings.Latitude != 10.01 ||
		settings.Longitude == nil || *settings.Longitude != -150.0 {
		t.Fatalf("settings coordinates: %s", env.Data)
	}
	if code, env := it.do("PATCH", "/v1/tenant/settings", hostOlex, ownerTok, map[string]any{"latitude": 95.0, "longitude": 10.0}); code != http.StatusBadRequest ||
		errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("out of range latitude: %d %s", code, errCode(env))
	}
	if code, env := it.do("PATCH", "/v1/tenant/settings", hostOlex, ownerTok, map[string]any{"latitude": 10.0}); code != http.StatusBadRequest ||
		errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("latitude alone: %d %s", code, errCode(env))
	}

	// A staff member without tenant.settings.write gets 403.
	staff, staffPw := it.user("geo-staff")
	it.member(near, staff, "staff")
	staffTok := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": staff.Email.String, "password": staffPw, "organization_slug": near.Slug,
	})).AccessToken
	if code, _ := it.do("PATCH", "/v1/tenant/settings", hostOlex, staffTok, map[string]any{"latitude": 1.0, "longitude": 1.0}); code != http.StatusForbidden {
		t.Fatalf("staff coordinates: %d, want 403", code)
	}

	type item struct {
		UUID                string  `json:"uuid"`
		Slug                string  `json:"slug"`
		DistanceKm          float64 `json:"distance_km"`
		WhatsApp            *string `json:"whatsapp"`
		Latitude            float64 `json:"latitude"`
		Longitude           float64 `json:"longitude"`
		City                string  `json:"city"`
		Name                string  `json:"name"`
		District            string  `json:"district"`
		AcceptsAppointments bool    `json:"accepts_appointments"`
	}
	nearby := func(host, query string) []item {
		t.Helper()
		code, env := it.do("GET", "/v1/public/dealers/nearby"+query, host, "", nil)
		if code != http.StatusOK {
			t.Fatalf("nearby %s: %d %s", query, code, errCode(env))
		}
		var out struct {
			Items []item `json:"items"`
		}
		if err := json.Unmarshal(env.Data, &out); err != nil {
			t.Fatalf("nearby body: %s", env.Data)
		}
		return out.Items
	}

	got := nearby(hostOlex, "?lat=10.0&lng=-150.0&radius_km=50")
	want := []string{near.Slug, mid.Slug, far.Slug}
	if len(got) != len(want) {
		t.Fatalf("nearby olex = %+v, want %v", got, want)
	}
	for i, w := range want {
		if got[i].Slug != w {
			t.Fatalf("nearby[%d] = %s, want %s (%+v)", i, got[i].Slug, w, got)
		}
	}
	// TEC-327: the portal books appointments with the organization uuid.
	if got[0].UUID != near.Uuid.String() {
		t.Fatalf("nearby[0] uuid = %q, want %s", got[0].UUID, near.Uuid)
	}
	if !(got[0].DistanceKm < got[1].DistanceKm && got[1].DistanceKm < got[2].DistanceKm) {
		t.Fatalf("distances not ascending: %+v", got)
	}
	// 0.01 degree of latitude is about 1.11 km.
	if got[0].DistanceKm < 1.0 || got[0].DistanceKm > 1.25 {
		t.Fatalf("near distance %.3f km", got[0].DistanceKm)
	}
	if got[1].WhatsApp == nil || *got[1].WhatsApp != "+905321112233" {
		t.Fatalf("whatsapp: %+v", got[1].WhatsApp)
	}
	for _, org := range []db.Organization{near, mid, far} {
		if _, err := it.q.UpsertAppointmentSettings(ctx, db.UpsertAppointmentSettingsParams{
			OrganizationID: org.ID, BrandID: org.BrandID, DailyVehicleCapacity: 3,
			DefaultEstimatedMinutes: 60, SlotIntervalMinutes: 60, WorkingHours: []byte(`{}`),
			PortalAppointmentsEnabled: true,
		}); err != nil {
			t.Fatalf("appointment settings %s: %v", org.Slug, err)
		}
	}
	if _, err := it.q.UpsertOrgModuleFlag(ctx, db.UpsertOrgModuleFlagParams{
		Scope: "org", OrganizationID: pgtype.Int8{Int64: far.ID, Valid: true},
		ModuleKey: features.ModuleAppointments, Enabled: false, Source: "admin",
	}); err != nil {
		t.Fatalf("disable appointments feature: %v", err)
	}
	got = nearby(hostOlex, "?lat=10.0&lng=-150.0&radius_km=50")
	if !got[0].AcceptsAppointments || !got[1].AcceptsAppointments || got[2].AcceptsAppointments {
		t.Fatalf("accepts_appointments = %+v, want enabled near/mid and disabled far", got)
	}

	// The radius cuts the far dealer.
	if got := nearby(hostOlex, "?lat=10.0&lng=-150.0&radius_km=8"); len(got) != 2 {
		t.Fatalf("radius 8 km = %+v, want near and mid", got)
	}
	// The Glorian domain only sees the Glorian dealer.
	if got := nearby(hostGlorian, "?lat=10.0&lng=-150.0&radius_km=50"); len(got) != 1 || got[0].Slug != glorian.Slug {
		t.Fatalf("nearby glorian = %+v", got)
	}

	if code, env := it.do("GET", "/v1/public/dealers/nearby?lat=91&lng=-150", hostOlex, "", nil); code != http.StatusBadRequest ||
		errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("invalid lat: %d %s", code, errCode(env))
	}
}
