package tools

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// parityDoc holds the parity table (moved from docs/ai.md by TEC-409).
const parityDoc = "docs/runbooks/f4-ai-cutover.md"

// TEC-386 acceptance: the parity table maps every legacy AI layer tool, hub
// /mcp/olex tool and M2M chatbot endpoint to new tool names or to "bilinçli
// olarak yok" with a reason, and every new name is a registered tool.
func TestParityTableNamesAreRegistered(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", parityDoc))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	start, end := strings.Index(doc, "<!-- parity:start -->"), strings.Index(doc, "<!-- parity:end -->")
	if start < 0 || end < start {
		t.Fatal(parityDoc + ": parity markers missing")
	}
	r := testRegistry(nil)
	name := regexp.MustCompile("`([^`]+)`")
	counts := map[string]int{}
	seen := map[string]bool{}
	mapped := 0
	for _, line := range strings.Split(doc[start:end], "\n") {
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Kaynak") || strings.HasPrefix(line, "|---") {
			continue
		}
		cols := strings.Split(strings.Trim(line, "| "), " | ")
		if len(cols) < 3 {
			t.Fatalf("parity row has %d columns: %s", len(cols), line)
		}
		source, legacy, target := cols[0], cols[1], cols[2]
		reason := ""
		if len(cols) > 3 {
			reason = strings.TrimSpace(cols[3])
		}
		key := source + " " + legacy
		if seen[key] {
			t.Fatalf("duplicate parity row: %s", key)
		}
		seen[key] = true
		counts[source]++
		if strings.Contains(target, "bilinçli olarak yok") {
			if reason == "" {
				t.Errorf("%s: 'bilinçli olarak yok' without a reason", key)
			}
			continue
		}
		names := name.FindAllStringSubmatch(target, -1)
		if len(names) == 0 {
			t.Errorf("%s: no new tool name and not marked 'bilinçli olarak yok': %q", key, target)
		}
		for _, m := range names {
			if _, ok := r.Get(m[1]); !ok {
				t.Errorf("%s: %s is not a registered tool", key, m[1])
			}
			mapped++
		}
	}
	// Legacy AI layer: 18 always registered + 3 document tools; hub
	// /mcp/olex: 12 tools; M2M chatbot: 16 endpoints.
	want := map[string]int{"eski-ai": 21, "hub-mcp": 12, "m2m": 16}
	for src, n := range want {
		if counts[src] != n {
			t.Errorf("%s rows = %d, want %d", src, counts[src], n)
		}
	}
	if len(counts) != len(want) || mapped == 0 {
		t.Errorf("unexpected sources %v (mapped %d)", counts, mapped)
	}
}

// TEC-1244: the assistant's order draft takes the same line fields as the
// REST order line (docs/openapi.yaml OrderItemInput) a buyer fills in:
// product, quantity / meters, the measure (box / meter) and the meter
// line's credits. Price, discount and gift fields stay out (the
// assistant never sets a price).
func TestOrderDraftSchemaParity(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "docs", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	start := strings.Index(doc, "\n    OrderItemInput:\n")
	if start < 0 {
		t.Fatal("OrderItemInput missing from docs/openapi.yaml")
	}
	block := doc[start+1:]
	if end := regexp.MustCompile(`\n    [A-Za-z]+:\n`).FindStringIndex(block[1:]); end != nil {
		block = block[:end[0]+1]
	}
	rest := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^        ([a-z_]+):`).FindAllStringSubmatch(block, -1) {
		rest[m[1]] = true
	}
	spec := CreateOrderDraft{}.Spec()
	items := spec.InputSchema["properties"].(map[string]any)["items"].(map[string]any)["items"].(map[string]any)
	props := items["properties"].(map[string]any)
	for _, f := range []string{"product_uuid", "quantity", "meters", "sale_measure", "line_credits"} {
		if !rest[f] {
			t.Errorf("OrderItemInput has no %s", f)
		}
		if _, ok := props[f]; !ok {
			t.Errorf("create_order_draft item schema has no %s", f)
		}
	}
	for _, f := range []string{"unit_price", "discount_pct", "is_gift"} {
		if _, ok := props[f]; ok {
			t.Errorf("create_order_draft must not take %s", f)
		}
	}
	m := props["sale_measure"].(map[string]any)["enum"].([]string)
	if strings.Join(m, ",") != "box,meter" {
		t.Errorf("sale_measure enum = %v", m)
	}
}
