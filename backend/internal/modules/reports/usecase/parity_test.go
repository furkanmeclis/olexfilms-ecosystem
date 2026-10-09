package usecase

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestParityTableCoversLegacyEndpoints: docs/mobile-reports.md maps all 22
// legacy endpoints and documents every catalog key.
func TestParityTableCoversLegacyEndpoints(t *testing.T) {
	raw, err := os.ReadFile("../../../../../docs/mobile-reports.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	section := doc[strings.Index(doc, "## 4. Parite tablosu"):]
	rows := regexp.MustCompile(`(?m)^\| (\d+) \| `+"`").FindAllStringSubmatch(section, -1)
	if len(rows) != 22 {
		t.Fatalf("parity rows = %d, want 22", len(rows))
	}
	for _, legacy := range []string{
		"/layout", "/overview", "/services/trend", "/services/status-distribution", "/services/top-car-brands",
		"/services/top-car-models", "/services/top-products", "/orders/trend", "/orders/status-distribution",
		"/customers/trend", "/customers/type-distribution", "/stock/summary", "/stock/status-distribution",
		"/warranty/summary", "/warranty/trend", "/warranty/status-distribution", "/operations/monthly-trends",
		"/activities/recent-services", "/dealers/performance", "/dealers/top-by-warranty", "/nexptg/summary",
		"/users/role-distribution",
	} {
		if !strings.Contains(section, legacy+"`") {
			t.Errorf("legacy %s missing from the parity table", legacy)
		}
	}
	for _, d := range Definitions {
		if !strings.Contains(doc, "`"+d.Key+"`") {
			t.Errorf("report %s not documented", d.Key)
		}
	}
}
