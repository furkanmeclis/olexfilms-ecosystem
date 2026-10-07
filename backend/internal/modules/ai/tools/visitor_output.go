package tools

import (
	"context"
	"encoding/json"
	"strings"
)

// TEC-397 (F4-02e): the visitor realm result whitelist. Every JSON key of a
// visitor tool result must be on VisitorOutputKeys; Registry.Call drops any
// other key, and every key naming a price, before the result reaches the
// model. The tool DTOs already return only these fields (TEC-386); the
// filter keeps a new or changed tool from leaking a price or a personal
// field to an unidentified WhatsApp contact.
var VisitorOutputKeys = map[string]bool{
	// envelopes
	"items": true, "returned": true, "total": true, "truncated": true, "hint": true, "note": true,
	// recommend_products
	"categories": true, "products": true, "name": true, "category": true, "warranty_months": true,
	"micron_thickness": true, "description": true,
	// find_nearest_dealers
	"city": true, "district": true, "distance_km": true, "whatsapp": true, "accepts_appointments": true, "page_url": true,
	// search_knowledge
	"passages": true,
	// lookup_warranty
	"public_code": true, "status": true, "start_at": true, "end_at": true, "days_remaining": true, "product": true,
	"dealer": true, "dealer_city": true, "vehicle": true, "brand": true, "model": true, "model_year": true,
	"plate_masked": true, "vin_last4": true,
}

// visitorKeyAllowed: on the whitelist and not a price.
func visitorKeyAllowed(k string) bool {
	return VisitorOutputKeys[k] && !strings.Contains(strings.ToLower(k), "price")
}

// visitorResult filters a visitor tool result through the whitelist. A
// non-JSON result (plain text, an error message) is returned unchanged.
func (r *Registry) visitorResult(ctx context.Context, name string, res Result) Result {
	var doc any
	if res.IsError || json.Unmarshal([]byte(res.Content), &doc) != nil {
		return res
	}
	var dropped []string
	doc = filterVisitorKeys(doc, &dropped)
	if len(dropped) == 0 {
		return res
	}
	r.log.WarnContext(ctx, "ai visitor tool result filtered", "tool", name, "keys", dropped)
	b, err := json.Marshal(doc)
	if err != nil {
		return failed()
	}
	res.Content = string(b)
	return res
}

func filterVisitorKeys(v any, dropped *[]string) any {
	switch t := v.(type) {
	case map[string]any:
		for k, c := range t {
			if !visitorKeyAllowed(k) {
				*dropped = append(*dropped, k)
				delete(t, k)
				continue
			}
			t[k] = filterVisitorKeys(c, dropped)
		}
	case []any:
		for i, c := range t {
			t[i] = filterVisitorKeys(c, dropped)
		}
	}
	return v
}
