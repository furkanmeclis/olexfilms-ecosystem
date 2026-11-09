package usecase

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

// TEC-1076: the contract PDF lists the used products with their codes.
func TestProductsHTMLListsCodesAndParts(t *testing.T) {
	if got := productsHTML(i18n.Locale("tr"), nil); got != "" {
		t.Fatalf("no products = %q, want empty", got)
	}
	rows := []db.ListContractServiceProductsRow{
		{ID: 1, ProductName: "Olex PPF <Gloss>", Sku: "PPF-190", Barcode: "OLX-ROLL-001",
			AppliedParts: []byte(`["body_kaput","custom_part"]`)},
		{ID: 2, ProductName: "Cam filmi", Sku: "WF-35", Barcode: "OLX-WF-9", AppliedParts: []byte(`[]`)},
	}
	got := productsHTML(i18n.Locale("tr"), rows)
	for _, want := range []string{
		"Kullanılan ürünler", "Ürün kodu", "Olex PPF &lt;Gloss&gt;", "PPF-190", "OLX-ROLL-001",
		"WF-35", "OLX-WF-9", "custom_part", `<td>2</td>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("products HTML missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "<Gloss>") || strings.Contains(got, "body_kaput") {
		t.Fatalf("products HTML not escaped / part not translated:\n%s", got)
	}
	en := productsHTML(i18n.Locale("en"), rows)
	if !strings.Contains(en, "Products used") || !strings.Contains(en, "Product code") {
		t.Fatalf("en products HTML:\n%s", en)
	}
}

func TestWithEvidenceBlocksAddsProducts(t *testing.T) {
	got := withEvidenceBlocks(`<p>{{contract_body_html}}</p>`, "SHA")
	if !strings.Contains(got, `<section class="doc-evidence">{{products_html}}{{signatures_html}}`) {
		t.Fatalf("products not placed before the signatures:\n%s", got)
	}
	placed := withEvidenceBlocks(`{{products_html}}{{signatures_html}}{{otp_proof_html}}{{media_html}}`, "SHA")
	if strings.Count(placed, "{{products_html}}") != 1 {
		t.Fatalf("products block duplicated:\n%s", placed)
	}
}
