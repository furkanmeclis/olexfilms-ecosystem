package usecase

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/documentstest"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
)

func samplePDF(items, warranties int) PDFDoc {
	d := PDFDoc{
		ServiceNo: "DS-2026-000196", Status: "Tamamlandı", CreatedAt: "01.10.2026", CompletedAt: "02.10.2026",
		Dealer:   PDFDealer{Name: "Tech <Oto>", City: "İstanbul", Phone: "+902165550000"},
		Customer: PDFCustomer{Name: "Ayşe Kaya", Phone: "********4567"},
		Vehicle:  PDFVehicle{Brand: "BMW", Model: "320i", Year: "2024", Plate: "34 ABC 196", VIN: "WBA00000000000196"},
		KM:       "12000", Notes: "Ön tampon <dikkat>",
		GeneratedAt: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC),
		Letterhead:  ioengine.Letterhead{CompanyName: "Tech <Oto>", FooterText: "olexfilms.app"},
	}
	for i := 0; i < items; i++ {
		it := PDFItem{Product: "PPF Ultra " + strconv.Itoa(i), SKU: "PPF-" + strconv.Itoa(i), Barcode: "OLX-" + strconv.Itoa(i),
			Kind: "full", Parts: []string{"body_kaput", "body_tavan"}}
		if i%2 == 1 {
			it.Kind, it.Meters, it.Parts = "partial", "10.50", []string{"window_on_cam", "custom_part"}
		}
		d.Items = append(d.Items, it)
	}
	for i := 0; i < warranties; i++ {
		code := strings.Repeat(string(rune('A'+i%26)), 22)
		d.Warranties = append(d.Warranties, PDFWarranty{PublicCode: code, Product: "PPF Ultra", Kind: "full", Status: "active",
			StartDate: "02.10.2026", EndDate: "02.10.2036", VerifyURL: "https://olexfilms.app/garanti/" + code})
	}
	return d
}

func TestPDFHTML(t *testing.T) {
	d := samplePDF(2, 2)
	out, err := PDFHTML(d, i18n.LocaleTR, "Hizmet raporu")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out, `<img src="data:image/png;base64,`); n != 2 {
		t.Fatalf("QR images = %d, want one per warranty", n)
	}
	for _, want := range []string{
		`dir="ltr"`, "DS-2026-000196", "34 ABC 196", "WBA00000000000196", "PPF Ultra 0", "OLX-1",
		"Kaput", "Tavan", "custom_part", "10.50 m", "Birimin tamamı", "Tam birim", "Kısmi kesim",
		"********4567", "12000 km", "Tech &lt;Oto&gt;", "Ön tampon &lt;dikkat&gt;", "Garanti özeti", "Aktif",
		"https://olexfilms.app/garanti/AAAAAAAAAAAAAAAAAAAAAA",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("html misses %q", want)
		}
	}
	if strings.Contains(out, "<dikkat>") || strings.Contains(out, "Tech <Oto>") {
		t.Fatal("values must be escaped")
	}

	ar, err := PDFHTML(d, i18n.LocaleAR, "تقرير الخدمة")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ar, `dir="rtl"`) || !strings.Contains(ar, "ملخص الضمان") || !strings.Contains(ar, "غطاء المحرك") {
		t.Fatal("ar document must be RTL and localized")
	}
}

func TestPDFHTMLWithoutWarranties(t *testing.T) {
	d := samplePDF(0, 0)
	d.CompletedAt = ""
	out, err := PDFHTML(d, i18n.LocaleEN, "Service report")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "data:image/png") {
		t.Fatal("no warranty, no QR")
	}
	for _, want := range []string{"Warranties are issued when the service is completed.", "No product added yet."} {
		if !strings.Contains(out, want) {
			t.Errorf("html misses %q", want)
		}
	}
}

func TestPDFDatasetRenders(t *testing.T) {
	ds := PDFDataset(samplePDF(1, 1), i18n.LocaleTR)
	if len(ds.Rows) != 1 || ds.Rows[0]["parts"] != "Kaput, Tavan" || ds.Rows[0]["amount"] != "Birimin tamamı" {
		t.Fatalf("rows = %+v", ds.Rows)
	}
	out, err := NewPDFAdapter(nil).DocumentHTML(ds, "tr", nil, "Hizmet raporu")
	if err != nil || !strings.Contains(out, "DS-2026-000196") {
		t.Fatalf("document html: %v", err)
	}
	if _, err := NewPDFAdapter(nil).DocumentHTML(ioengine.Dataset{}, "tr", nil, "x"); err == nil {
		t.Fatal("dataset without document must fail")
	}
}

// TEC-196 acceptance (time budget): the export path after the data load,
// dataset -> HTML -> Gotenberg (mock), stays far below 3 s for a large
// service (30 items, 15 warranties with QR codes); the HTML itself is built
// in milliseconds. The real Gotenberg duration is a manual check.
func TestPDFRenderTimeBudget(t *testing.T) {
	gotb := documentstest.NewGotenberg(t, 0)
	client := pdfrender.New(gotb.URL)
	adapter := NewPDFAdapter(nil)
	d := samplePDF(30, 15)

	for _, loc := range []i18n.Locale{i18n.LocaleTR, i18n.LocaleAR} {
		start := time.Now()
		ds := PDFDataset(d, loc)
		html, err := adapter.DocumentHTML(ds, string(loc), nil, i18n.Translate(loc, "export.title."+ResourcePDF))
		if err != nil {
			t.Fatal(err)
		}
		htmlTook := time.Since(start)
		if _, err := client.HTMLToPDF(context.Background(), html); err != nil {
			t.Fatal(err)
		}
		total := time.Since(start)
		t.Logf("%s: html %s, total %s", loc, htmlTook, total)
		if htmlTook > 250*time.Millisecond {
			t.Errorf("%s: html took %s, want milliseconds", loc, htmlTook)
		}
		if total >= 3*time.Second {
			t.Errorf("%s: render took %s, budget 3s", loc, total)
		}
		if h := gotb.LastHTML(); !strings.Contains(h, d.ServiceNo) || !strings.Contains(h, d.Vehicle.Plate) {
			t.Fatalf("%s: gotenberg did not receive the document", loc)
		}
	}
}

func BenchmarkPDFHTML(b *testing.B) {
	d := samplePDF(30, 15)
	for b.Loop() {
		if _, err := PDFHTML(d, i18n.LocaleTR, "Hizmet raporu"); err != nil {
			b.Fatal(err)
		}
	}
}
