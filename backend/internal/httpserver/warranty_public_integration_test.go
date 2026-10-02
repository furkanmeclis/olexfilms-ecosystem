package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	warrantymodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
)

type publicWarrantyView struct {
	PublicCode    string `json:"public_code"`
	Status        string `json:"status"`
	DaysRemaining int    `json:"days_remaining"`
	Product       struct {
		Name string `json:"name"`
	} `json:"product"`
	Brand struct {
		Slug string `json:"slug"`
	} `json:"brand"`
	Dealer struct {
		Name string `json:"name"`
	} `json:"dealer"`
	Vehicle struct {
		BrandName   string  `json:"brand_name"`
		ModelName   string  `json:"model_name"`
		PlateMasked *string `json:"plate_masked"`
		VINLast4    *string `json:"vin_last4"`
	} `json:"vehicle"`
}

// publicWarranty calls the public lookup as the BFF does (client IP in
// X-Forwarded-For, browser host in X-Forwarded-Host) and returns the raw body.
func (it *itest) publicWarranty(code, host, ip string) *httptest.ResponseRecorder {
	it.t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/public/warranties/"+code, nil)
	req.Host = "backend:8080"
	req.Header.Set("X-Forwarded-Host", host)
	req.Header.Set("X-Forwarded-For", ip)
	rec := httptest.NewRecorder()
	it.handler.ServeHTTP(rec, req)
	return rec
}

// publicWarrantyFixture opens one warranty on a vehicle with a Turkish plate
// and a VIN through the service.completed listener.
func (it *itest) publicWarrantyFixture(prefix string) (db.Warranty, db.User, db.Organization) {
	it.t.Helper()
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org(prefix+"-dist", "distributor", center)
	dealer := it.org(prefix+"-dealer", "dealer", dist)
	cust, veh := it.svcCustomer(dealer, prefix+"-cust", "34ABC112")
	if _, err := it.pool.Exec(ctx, `UPDATE vehicles SET plate = '34 ABC 112', vin = 'WVWZZZ1JZ3W386752' WHERE id = $1`, veh.ID); err != nil {
		it.t.Fatalf("vehicle: %v", err)
	}
	p := it.product(center, strings.ToUpper(prefix)+"P")
	it.setWarrantyMonths(p, 12)
	u := it.stockChain().unit(center, p, 18901)
	svc := it.directService(dealer, cust, veh, 1, db.CreateServiceItemParams{ProductID: p.ID, UnitID: u.ID, Kind: "full"})
	listener := warrantymodule.NewListener(it.pool, it.q, "http://localhost:3000", nil)
	if err := listener.HandleServiceCompleted(ctx, completedEvent(svc)); err != nil {
		it.t.Fatalf("listener: %v", err)
	}
	ws := it.serviceWarranties(svc.ID, svc.BrandID)
	if len(ws) != 1 {
		it.t.Fatalf("warranties = %d, want 1", len(ws))
	}
	return ws[0], cust, dealer
}

// TEC-189 acceptance: a valid code returns the masked projection with no
// personal data; the warranty of an anonymized customer is still found;
// malformed, unknown and other-brand codes are the same 404.
func TestIntegrationPublicWarranty(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	w, cust, dealer := it.publicWarrantyFixture("t189")

	check := func(label string) {
		t.Helper()
		rec := it.publicWarranty(w.PublicCode, hostOlex, "203.0.113.10")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d %s", label, rec.Code, rec.Body)
		}
		body := rec.Body.Bytes()
		if keys := warrantyusecase.FindPIIKeys(body); len(keys) > 0 {
			t.Fatalf("%s: personal data keys %v in %s", label, keys, body)
		}
		for _, leak := range []string{cust.Name, cust.Email.String, "34ABC112", "34 ABC 112", "WVWZZZ1JZ3W386752"} {
			if leak != "" && strings.Contains(string(body), leak) {
				t.Fatalf("%s: %q leaked in %s", label, leak, body)
			}
		}
		var env envelope
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatal(err)
		}
		var v publicWarrantyView
		if err := json.Unmarshal(env.Data, &v); err != nil {
			t.Fatal(err)
		}
		if v.PublicCode != w.PublicCode || v.Status != "active" || v.DaysRemaining < 360 ||
			v.Brand.Slug != "olex" || v.Dealer.Name != dealer.Name || !strings.HasPrefix(v.Product.Name, "Film ") ||
			v.Vehicle.BrandName == "" || v.Vehicle.ModelName == "" {
			t.Fatalf("%s: view = %+v", label, v)
		}
		if v.Vehicle.PlateMasked == nil || *v.Vehicle.PlateMasked != "34 *** 12" {
			t.Fatalf("%s: plate = %v", label, v.Vehicle.PlateMasked)
		}
		if v.Vehicle.VINLast4 == nil || *v.Vehicle.VINLast4 != "6752" {
			t.Fatalf("%s: vin = %v", label, v.Vehicle.VINLast4)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: cache-control %q", label, rec.Header().Get("Cache-Control"))
		}
	}
	check("before anonymization")

	// K19 / TEC-161: anonymizing the holder keeps the warranty readable.
	if _, err := it.q.AnonymizeUser(ctx, db.AnonymizeUserParams{ID: cust.ID, PasswordHash: "x-anonymized"}); err != nil {
		t.Fatalf("anonymize: %v", err)
	}
	check("after anonymization")

	// Misses: malformed (no DB round trip), unknown, another brand's domain.
	var first envelope
	for i, tc := range []struct{ code, host string }{
		{"bad", hostOlex},
		{"has.dot.in.it.000000000", hostOlex},
		{"ZZZZZZZZZZZZZZZZZZZZZZ", hostOlex},
		{w.PublicCode, hostGlorian},
	} {
		rec := it.publicWarranty(tc.code, tc.host, "203.0.113.11")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%q on %s: status %d", tc.code, tc.host, rec.Code)
		}
		var env envelope
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		if i == 0 {
			first = env
		} else if errCode(env) != errCode(first) {
			t.Fatalf("%q: 404 code %s differs from %s", tc.code, errCode(env), errCode(first))
		}
	}
}

// TEC-189 acceptance: over the per-IP limit the lookup answers 429 with
// Retry-After; another IP is not affected.
func TestIntegrationPublicWarrantyRateLimit(t *testing.T) {
	it := newIntegrationWith(t, func(c *config.Config) {
		c.Warranty.PublicRateLimit = 3
		c.Warranty.PublicRateWindow = time.Minute
	})
	w, _, _ := it.publicWarrantyFixture("t189r")
	for i := 0; i < 3; i++ {
		if rec := it.publicWarranty(w.PublicCode, hostOlex, "198.51.100.7"); rec.Code != http.StatusOK {
			t.Fatalf("hit %d: %d", i+1, rec.Code)
		}
	}
	rec := it.publicWarranty(w.PublicCode, hostOlex, "198.51.100.7")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("over limit: %d retry-after %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if rec := it.publicWarranty("bad", hostOlex, "198.51.100.7"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("malformed over limit: %d", rec.Code)
	}
	if rec := it.publicWarranty(w.PublicCode, hostOlex, "198.51.100.8"); rec.Code != http.StatusOK {
		t.Fatalf("other ip: %d", rec.Code)
	}
}
