package usecase

import (
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestOrderSlipHTML(t *testing.T) {
	for _, lang := range []string{"tr", "en", "bg", "de", "el", "uk", "ru", "fr", "es", "it", "zh_CN", "az", "ar"} {
		t.Run(lang, func(t *testing.T) {
			d := OrderSlipDoc{ShippingDoc: ShippingDoc{Locale: i18n.Normalize(lang), OrderNo: "ORD-772", Zone: pdfrender.Zone("America/New_York"), GeneratedAt: time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC), Seller: ShippingParty{Name: "Seller <script>", TaxNo: "SELL-TAX"}, Buyer: ShippingParty{Name: "Buyer", TaxNo: "BUY-TAX"}, Letterhead: ioengine.Letterhead{CompanyName: "Seller", PrimaryColor: "#123456", TaxNumber: "SELL-TAX"}}, Lines: []OrderSlipLine{{ShippingLine: ShippingLine{Name: "Film <b>", SKU: "SKU-772", Amount: "2 m", Barcodes: []string{"BC-772"}}, UnitPrice: "100.00", LineTotal: "200.00"}}, CreatedAt: time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC), Currency: "EUR", Subtotal: "200.00", TaxTotal: "40.00", Total: "240.00", Notes: "Note <img>\nSecond line", Status: "Approved"}
			out := OrderSlipHTML(d)
			for _, want := range []string{"ORD-772", "Seller &lt;script&gt;", "Buyer", "SELL-TAX", "BUY-TAX", "Film &lt;b&gt;", "SKU-772", "BC-772", "2 m", "100.00 EUR", "200.00 EUR", "40.00 EUR", "240.00 EUR", "Note &lt;img&gt;", "2026-10-16 22:00 (America/New_York)", "2026-10-15 23:00 (America/New_York)", "--primary:#123456", i18n.Translate(d.Locale, "orders.pdf.order_slip"), i18n.Translate(d.Locale, "orders.pdf.buyer_signature")} {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q", want)
				}
			}
			for _, unwanted := range []string{"Seller <script>", "Film <b>", "Other org", "Other order"} {
				if strings.Contains(out, unwanted) {
					t.Errorf("unexpected data %q", unwanted)
				}
			}
			if lang == "ar" && !strings.Contains(out, `dir="rtl"`) {
				t.Error("Arabic must be RTL")
			}
		})
	}
}

// TEC-1239: a meter line reads "3,0 m" in Turkish and carries its usage
// credits in a note on the slip and the shipping note.
func TestMeterLineDocuments(t *testing.T) {
	num := func(s string) pgtype.Numeric {
		var n pgtype.Numeric
		if err := n.Scan(s); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, tc := range []struct {
		loc  i18n.Locale
		in   string
		want string
	}{
		{i18n.LocaleTR, "3", "3,0 m"},
		{i18n.LocaleTR, "3.00", "3,0 m"},
		{i18n.LocaleTR, "2.5", "2,5 m"},
		{i18n.LocaleDE, "2.25", "2,25 m"},
		{i18n.LocaleEN, "3", "3.0 m"},
		{i18n.LocaleEN, "12.40", "12.4 m"},
		{i18n.LocaleFA, "0.5", "0.5 m"},
	} {
		if got := meterAmount(tc.loc, num(tc.in)); got != tc.want {
			t.Errorf("meterAmount(%s, %s) = %q, want %q", tc.loc, tc.in, got, tc.want)
		}
	}
	if got := lineCreditsNote(i18n.LocaleTR, pgtype.Int4{}); got != "" {
		t.Errorf("no credits: %q", got)
	}
	note := lineCreditsNote(i18n.LocaleTR, pgtype.Int4{Int32: 5, Valid: true})
	if note != "Kullanım kredisi: parça başına 5" {
		t.Fatalf("note = %q", note)
	}
	for _, loc := range i18n.Supported {
		if n := lineCreditsNote(loc, pgtype.Int4{Int32: 7, Valid: true}); !strings.Contains(n, "7") || strings.Contains(n, "{{") {
			t.Errorf("%s: note %q", loc, n)
		}
	}
	line := ShippingLine{Name: "Black PPF", SKU: "BLK", Amount: "3,0 m", Note: note, Barcodes: []string{"CUT-1 (3,0 m)"}}
	ship := ShippingDocumentHTML(ShippingDoc{Locale: i18n.LocaleTR, OrderNo: "ORD-1", Zone: time.UTC, Lines: []ShippingLine{line}})
	slip := OrderSlipHTML(OrderSlipDoc{ShippingDoc: ShippingDoc{Locale: i18n.LocaleTR, OrderNo: "ORD-1", Zone: time.UTC}, Lines: []OrderSlipLine{{ShippingLine: line}}})
	for name, out := range map[string]string{"shipping": ship, "slip": slip} {
		for _, want := range []string{"3,0 m", "Kullanım kredisi: parça başına 5", "CUT-1 (3,0 m)"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: missing %q", name, want)
			}
		}
	}
}
