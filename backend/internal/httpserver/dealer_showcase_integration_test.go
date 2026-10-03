package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"
)

// TEC-250: GET /v1/public/dealers/{code}, the public dealer showcase.
func TestIntegrationDealerShowcase(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	olexCenter := it.brandCenter("olex")
	glorianCenter := it.brandCenter("glorian")

	active := it.org("show", "dealer", olexCenter)
	distributor := it.org("showdist", "distributor", olexCenter)
	passive := it.org("showpassive", "dealer", olexCenter)
	expired := it.org("showexpired", "dealer", olexCenter)
	glorian := it.org("showglorian", "dealer", glorianCenter)

	if _, err := it.pool.Exec(ctx,
		`UPDATE organizations SET latitude = 41.0082, longitude = 28.9784, phone = '+905321112233',
		        address = 'Moda Cd. 1', city = 'İstanbul', district = 'Kadıköy',
		        email = 'secret@example.test', website = 'https://internal.example.test'
		 WHERE id = $1`, active.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := it.pool.Exec(ctx, "UPDATE organizations SET status = 'suspended' WHERE id = $1", passive.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := it.pool.Exec(ctx, "UPDATE organizations SET access_ends_at = NOW() - interval '1 minute' WHERE id = $1", expired.ID); err != nil {
		t.Fatal(err)
	}

	get := func(host, code string) (int, envelope) {
		t.Helper()
		return it.do("GET", "/v1/public/dealers/"+code, host, "", nil)
	}

	// Active dealer: 200 with the showcase fields only.
	code, env := get(hostOlex, active.Slug)
	if code != http.StatusOK {
		t.Fatalf("active dealer: %d %s", code, errCode(env))
	}
	var body map[string]any
	if err := json.Unmarshal(env.Data, &body); err != nil {
		t.Fatalf("body: %s", env.Data)
	}
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if got, want := strings.Join(keys, ","), "address,city,code,district,latitude,logo_url,longitude,name,whatsapp"; got != want {
		t.Fatalf("keys = %s, want %s", got, want)
	}
	if body["code"] != active.Slug || body["whatsapp"] != "+905321112233" || body["latitude"] != 41.0082 ||
		body["longitude"] != 28.9784 || body["city"] != "İstanbul" || body["district"] != "Kadıköy" || body["address"] != "Moda Cd. 1" {
		t.Fatalf("active dealer body: %s", env.Data)
	}
	// No logo here, so the organization uuid must not appear either.
	for _, secret := range []string{"secret@example.test", "internal.example.test", active.Uuid.String()} {
		if strings.Contains(string(env.Data), secret) {
			t.Fatalf("body leaks %s: %s", secret, env.Data)
		}
	}

	// A distributor without coordinates or WhatsApp: 200 with nulls.
	code, env = get(hostOlex, distributor.Slug)
	if code != http.StatusOK {
		t.Fatalf("distributor: %d %s", code, errCode(env))
	}
	var dist struct {
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
		WhatsApp  *string  `json:"whatsapp"`
	}
	if err := json.Unmarshal(env.Data, &dist); err != nil || dist.Latitude != nil || dist.Longitude != nil || dist.WhatsApp != nil {
		t.Fatalf("distributor body: %s", env.Data)
	}

	// Passive, expired, other-brand, non-dealer and unknown codes: 404.
	for name, c := range map[string]string{
		"passive":   passive.Slug,
		"expired":   expired.Slug,
		"glorian":   glorian.Slug,
		"center":    olexCenter.Slug,
		"unknown":   "t83-nope-" + active.Slug,
		"malformed": "Bad%20Code",
	} {
		if code, env := get(hostOlex, c); code != http.StatusNotFound || errCode(env) != "NOT_FOUND" {
			t.Fatalf("%s: %d %s, want 404", name, code, errCode(env))
		}
	}
	// The Glorian domain sees its own dealer, not the Olex one.
	if code, _ := get(hostGlorian, glorian.Slug); code != http.StatusOK {
		t.Fatalf("glorian on glorian domain: %d", code)
	}
	if code, _ := get(hostGlorian, active.Slug); code != http.StatusNotFound {
		t.Fatalf("olex dealer on glorian domain: %d", code)
	}

	// The literal nearby route still wins over {code}.
	if code, env := get(hostOlex, "nearby?lat=91&lng=0"); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("nearby route: %d %s", code, errCode(env))
	}
}
