package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// leakyVisitorTool is a visitor tool whose DTO carries prices and an
// internal id, as a careless new tool could.
type leakyVisitorTool struct{}

func (leakyVisitorTool) Spec() Spec {
	return visitorSpec("leaky_products", "Products.", object(map[string]any{}))
}

func (leakyVisitorTool) Run(context.Context, Env, json.RawMessage) (Result, error) {
	return JSONResult(map[string]any{
		"items": []any{map[string]any{
			"name": "PPF Gloss", "price": 1200.5, "list_price": 1500, "unit_price": map[string]any{"amount": 9},
			"Retail_Price": 1, "id": 42, "warranty_months": 120,
		}},
		"price_note": "VAT included", "note": "ok",
	})
}

// TEC-397 acceptance: a visitor answer carries no price field. Whatever a
// visitor tool returns, the result the model sees holds only whitelisted
// keys; the same tool for a panel user is untouched.
func TestVisitorResultWhitelistDropsPrices(t *testing.T) {
	r := testRegistry(nil)
	r.Register(leakyVisitorTool{})
	res := call(t, r, realmPrincipal(RealmVisitor), "leaky_products", `{}`)
	if res.IsError {
		t.Fatalf("result: %+v", res)
	}
	if strings.Contains(strings.ToLower(res.Content), "price") || strings.Contains(res.Content, `"id"`) {
		t.Fatalf("visitor result leaks: %s", res.Content)
	}
	var doc any
	if err := json.Unmarshal([]byte(res.Content), &doc); err != nil {
		t.Fatal(err)
	}
	for _, k := range allKeys(doc) {
		if !VisitorOutputKeys[k] {
			t.Fatalf("key %q is not on the whitelist: %s", k, res.Content)
		}
	}
	if !strings.Contains(res.Content, `"PPF Gloss"`) || !strings.Contains(res.Content, `"warranty_months":120`) {
		t.Fatalf("whitelisted fields lost: %s", res.Content)
	}

	// A whitelisted key never names a price.
	for k := range VisitorOutputKeys {
		if strings.Contains(k, "price") {
			t.Fatalf("whitelist holds a price key %q", k)
		}
	}
}
