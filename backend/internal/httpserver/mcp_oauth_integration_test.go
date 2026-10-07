package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TEC-400: the OAuth endpoints are mounted outside /v1 on the full server
// (brand / locale middleware included), metadata uses the frontend origin as
// issuer and registration is limited to 20 per hour per IP (the 21st is 429).
func TestIntegrationMCPOAuthRoutes(t *testing.T) {
	it := newIntegration(t)
	ip := fmt.Sprintf("198.18.%d.%d", time.Now().UnixNano()%250, time.Now().UnixNano()/1000%250)
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM oauth_clients WHERE created_ip = $1`, ip)
	})

	rec := httptest.NewRecorder()
	it.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("metadata status = %d %s", rec.Code, rec.Body)
	}
	var meta map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["issuer"] != "http://localhost:3000" || meta["token_endpoint"] != "http://localhost:3000/oauth/token" {
		t.Fatalf("metadata = %v", meta)
	}

	rec = httptest.NewRecorder()
	it.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource/mcp/dealer", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"resource":"http://localhost:3000/mcp/dealer"`) {
		t.Fatalf("resource metadata = %d %s", rec.Code, rec.Body)
	}

	register := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/oauth/register",
			strings.NewReader(`{"client_name":"Claude","redirect_uris":["https://claude.ai/api/mcp/auth_callback"]}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", ip)
		rec := httptest.NewRecorder()
		it.handler.ServeHTTP(rec, req)
		return rec
	}
	for i := 1; i <= 20; i++ {
		if rec := register(); rec.Code != http.StatusCreated {
			t.Fatalf("registration %d = %d %s", i, rec.Code, rec.Body)
		}
	}
	rec = register()
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), `"slow_down"`) {
		t.Fatalf("21st registration = %d %v %s", rec.Code, rec.Header(), rec.Body)
	}
}
