package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
)

// TEC-495 (F5-05f): the /v1/reports routes end to end — catalog without a
// disabled module, 403 FEATURE_DISABLED, 404 unknown report, 400 period,
// layout default + atomic PUT.
func TestIntegrationReportsAPI(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t495-dist", "distributor", center)
	dealer := it.org("t495-dealer", "dealer", dist)
	owner, pw := it.user("t495-dealer-owner")
	it.member(dealer, owner, "owner")
	tok := it.loginOrg(owner, pw, dealer)

	if _, err := it.srv.features.SetByAdmin(ctx, 0, dealer.ID, features.ModulePerformance, false); err != nil {
		t.Fatal(err)
	}

	code, env := it.do("GET", "/v1/reports/catalog?locale=en", hostOlex, tok, nil)
	if code != http.StatusOK {
		t.Fatalf("catalog = %d %s", code, errCode(env))
	}
	var catalog struct {
		Items []struct {
			Key   string `json:"key"`
			Title string `json:"title"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &catalog); err != nil {
		t.Fatal(err)
	}
	keys := map[string]string{}
	for _, i := range catalog.Items {
		keys[i.Key] = i.Title
	}
	if keys["services.trend"] != "Service trend" || keys["overview"] == "" {
		t.Fatalf("catalog = %v", keys)
	}
	if _, ok := keys["dealers.performance"]; ok {
		t.Fatal("dealers.performance listed with the performance module off")
	}
	if _, ok := keys["dealers.top_by_warranty"]; ok {
		t.Fatal("network report listed for a dealer")
	}

	code, env = it.do("GET", "/v1/reports/dealers.performance", hostOlex, tok, nil)
	if code != http.StatusForbidden || errCode(env) != "FEATURE_DISABLED" {
		t.Fatalf("disabled report = %d %s, want 403 FEATURE_DISABLED", code, errCode(env))
	}
	code, env = it.do("GET", "/v1/reports/dealers.top_by_warranty", hostOlex, tok, nil)
	if code != http.StatusForbidden || errCode(env) != "FORBIDDEN" {
		t.Fatalf("network report = %d %s, want 403 FORBIDDEN", code, errCode(env))
	}
	code, env = it.do("GET", "/v1/reports/services.nope", hostOlex, tok, nil)
	if code != http.StatusNotFound {
		t.Fatalf("unknown report = %d %s, want 404", code, errCode(env))
	}
	code, env = it.do("GET", "/v1/reports/services.trend?period=5y", hostOlex, tok, nil)
	if code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("bad period = %d %s, want 400", code, errCode(env))
	}
	code, env = it.do("GET", "/v1/reports/services.trend?period=7d&locale=de", hostOlex, tok, nil)
	if code != http.StatusOK {
		t.Fatalf("services.trend = %d %s", code, errCode(env))
	}
	var report struct {
		Report      string  `json:"report"`
		Title       string  `json:"title"`
		Granularity *string `json:"granularity"`
		Series      []struct {
			Key    string            `json:"key"`
			Points []json.RawMessage `json:"points"`
		} `json:"series"`
	}
	if err := json.Unmarshal(env.Data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Report != "services.trend" || report.Title != "Serviceverlauf" || report.Granularity == nil ||
		*report.Granularity != "day" || len(report.Series) != 2 || len(report.Series[0].Points) != 7 {
		t.Fatalf("services.trend = %s", env.Data)
	}
	code, env = it.do("GET", "/v1/reports/overview", hostOlex, tok, nil)
	if code != http.StatusOK {
		t.Fatalf("overview = %d %s", code, errCode(env))
	}

	code, env = it.do("GET", "/v1/reports/layout", hostOlex, tok, nil)
	if code != http.StatusOK {
		t.Fatalf("layout = %d %s", code, errCode(env))
	}
	var layout struct {
		Widgets []struct {
			Report string `json:"report"`
		} `json:"widgets"`
	}
	if err := json.Unmarshal(env.Data, &layout); err != nil || len(layout.Widgets) != 1 || layout.Widgets[0].Report != "overview" {
		t.Fatalf("default layout = %s", env.Data)
	}
	code, env = it.do("PUT", "/v1/reports/layout", hostOlex, tok, map[string]any{"version": 1, "widgets": []any{
		map[string]any{"id": "550e8400-e29b-41d4-a716-446655440000", "report": "services.trend", "period": "7d"},
	}})
	if code != http.StatusOK {
		t.Fatalf("put layout = %d %s", code, errCode(env))
	}
	code, env = it.do("PUT", "/v1/reports/layout", hostOlex, tok, map[string]any{"widgets": []any{
		map[string]any{"id": "550e8400-e29b-41d4-a716-446655440000", "report": "overview"},
		map[string]any{"id": "660e8400-e29b-41d4-a716-446655440001", "report": "dealers.performance"},
	}})
	if code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("invalid layout = %d %s, want 400", code, errCode(env))
	}
	_, env = it.do("GET", "/v1/reports/layout", hostOlex, tok, nil)
	if err := json.Unmarshal(env.Data, &layout); err != nil || len(layout.Widgets) != 1 || layout.Widgets[0].Report != "services.trend" {
		t.Fatalf("layout after rejected PUT = %s", env.Data)
	}
}
