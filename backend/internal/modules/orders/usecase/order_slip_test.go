package usecase

import (
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
)

func TestOrderSlipHTML(t *testing.T) {
	for _, lang := range []string{"tr", "en", "bg", "de", "el", "uk", "ru", "fr", "es", "it", "zh_CN", "az", "ar"} {
		t.Run(lang, func(t *testing.T) {
			d := OrderSlipDoc{ShippingDoc: ShippingDoc{Locale: i18n.Normalize(lang), OrderNo: "ORD-772", Zone: pdfrender.Zone("America/New_York"), GeneratedAt: time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC), Seller: ShippingParty{Name: "Seller <script>", TaxNo: "SELL-TAX"}, Buyer: ShippingParty{Name: "Buyer", TaxNo: "BUY-TAX"}, Letterhead: ioengine.Letterhead{CompanyName: "Seller", PrimaryColor: "#123456", TaxNumber: "SELL-TAX"}, Lines: []ShippingLine{{Name: "Film <b>", UnitPrice: "100.00", LineTotal: "200.00", SKU: "SKU-772", Amount: "2 m", Barcodes: []string{"BC-772"}}}}, CreatedAt: time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC), Currency: "EUR", Subtotal: "200.00", TaxTotal: "40.00", Total: "240.00", Notes: "Note <img>\nSecond line", Status: "Approved"}
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
