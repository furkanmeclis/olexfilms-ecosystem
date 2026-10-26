package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
)

// TEC-1005 acceptance: /v1/public/service-warranties/{code} answers the
// service page for the service number, for a warranty public code (the
// per-warranty links before TEC-1005) and for nothing else; the projection
// carries no personal data.
func TestIntegrationPublicServiceWarranties(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	w, _, dealer := it.publicWarrantyFixture("t1005")
	var serviceNo string
	if err := it.pool.QueryRow(ctx, `SELECT service_no FROM services WHERE id = $1`, w.ServiceID).Scan(&serviceNo); err != nil {
		t.Fatal(err)
	}

	get := func(code, host string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/v1/public/service-warranties/"+code, nil)
		req.Host = "backend:8080"
		req.Header.Set("X-Forwarded-Host", host)
		req.Header.Set("X-Forwarded-For", "203.0.113.20")
		rec := httptest.NewRecorder()
		it.handler.ServeHTTP(rec, req)
		return rec
	}
	for _, code := range []string{serviceNo, w.PublicCode} {
		rec := get(code, hostOlex)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d %s", code, rec.Code, rec.Body)
		}
		body := rec.Body.Bytes()
		if keys := warrantyusecase.FindPIIKeys(body); len(keys) > 0 {
			t.Fatalf("%s: personal data keys %v", code, keys)
		}
		for _, leak := range []string{"34ABC112", "34 ABC 112", "WVWZZZ1JZ3W386752"} {
			if strings.Contains(string(body), leak) {
				t.Fatalf("%s: %q leaked", code, leak)
			}
		}
		var env envelope
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatal(err)
		}
		var v struct {
			ServiceNo string `json:"service_no"`
			Dealer    struct {
				Name string `json:"name"`
			} `json:"dealer"`
			Warranties []struct {
				PublicCode string `json:"public_code"`
				Status     string `json:"status"`
			} `json:"warranties"`
		}
		if err := json.Unmarshal(env.Data, &v); err != nil {
			t.Fatal(err)
		}
		if v.ServiceNo != serviceNo || v.Dealer.Name != dealer.Name || len(v.Warranties) != 1 ||
			v.Warranties[0].PublicCode != w.PublicCode || v.Warranties[0].Status != "active" {
			t.Fatalf("%s: view = %+v", code, v)
		}
	}
	for _, tc := range []struct{ code, host string }{
		{"bad", hostOlex}, {"ZZZZZZZZZZZZZZZZZZZZZZ", hostOlex}, {serviceNo, hostGlorian},
	} {
		if rec := get(tc.code, tc.host); rec.Code != http.StatusNotFound {
			t.Fatalf("%q on %s: status %d", tc.code, tc.host, rec.Code)
		}
	}
}
