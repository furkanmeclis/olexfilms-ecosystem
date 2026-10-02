package usecase

import (
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
)

func sampleCertificate() Certificate {
	return Certificate{
		ServiceNo: "DS-2026-000042", ServiceDate: "01.10.2026",
		Dealer:     CertificateDealer{Name: "Tech <Oto>", City: "İstanbul", Phone: "+902165550000"},
		Vehicle:    CertificateVehicle{Brand: "BMW", Model: "320i", Year: "2024", Plate: "34 ABC 123", VIN: "WBA00000000000001"},
		HolderName: "Ayşe Kaya",
		Items: []CertificateItem{
			{PublicCode: "AAAAAAAAAAAAAAAAAAAAAA", Product: "PPF Ultra", Barcode: "OLX-1", Kind: "full",
				StartDate: "01.10.2026", EndDate: "01.10.2036", VerifyURL: "https://olexfilms.app/garanti/AAAAAAAAAAAAAAAAAAAAAA"},
			{PublicCode: "BBBBBBBBBBBBBBBBBBBBBB", Product: "PPF Roll", Barcode: "OLX-R", Kind: "partial", Meters: "10.50",
				StartDate: "01.10.2026", EndDate: "01.10.2028", VerifyURL: "https://olexfilms.app/garanti/BBBBBBBBBBBBBBBBBBBBBB"},
		},
		GeneratedAt: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC),
		Letterhead:  ioengine.Letterhead{CompanyName: "Tech <Oto>", FooterText: "olexfilms.app"},
	}
}

func TestCertificateHTML(t *testing.T) {
	c := sampleCertificate()
	out, err := CertificateHTML(c, i18n.LocaleTR, "Garanti belgesi")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, `<img src="data:image/png;base64,`) != 2 {
		t.Fatal("want one QR image per warranty")
	}
	for _, want := range []string{
		`dir="ltr"`, "AAAAAAAAAAAAAAAAAAAAAA", "BBBBBBBBBBBBBBBBBBBBBB", "34 ABC 123", "WBA00000000000001",
		"https://olexfilms.app/garanti/AAAAAAAAAAAAAAAAAAAAAA", "Kısmi kesim (10.50 m)", "Tam birim",
		"Garanti koşulları", "Tech &lt;Oto&gt;", "Ayşe Kaya", "DS-2026-000042",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("certificate html misses %q", want)
		}
	}
	if strings.Contains(out, "Tech <Oto>") {
		t.Fatal("dealer name is not escaped")
	}
	if strings.Count(out, "<li>") != 5 {
		t.Fatalf("terms items = %d", strings.Count(out, "<li>"))
	}

	ar, err := CertificateHTML(c, i18n.LocaleAR, "شهادة الضمان")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ar, `dir="rtl"`) || !strings.Contains(ar, "شروط الضمان") {
		t.Fatal("arabic certificate is not rtl / translated")
	}
}

func TestCertificateDataset(t *testing.T) {
	ds := CertificateDataset(ResourceCertificate, sampleCertificate(), i18n.LocaleEN)
	if len(ds.Rows) != 2 || ds.Rows[1]["code"] != "BBBBBBBBBBBBBBBBBBBBBB" || len(ds.Info) != 7 {
		t.Fatalf("dataset = %+v", ds)
	}
	if _, ok := ds.Doc.(Certificate); !ok {
		t.Fatal("dataset carries no certificate")
	}
	html, err := NewCertificateAdapter(nil).DocumentHTML(ds, "en", nil, "Warranty certificate")
	if err != nil || !strings.Contains(html, "Warranty terms") {
		t.Fatalf("document html: %v", err)
	}
}

func TestFormatCertificateDate(t *testing.T) {
	ist, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		t.Skipf("tzdata: %v", err)
	}
	// 31 Dec 22:30 UTC is already 1 Jan in Istanbul.
	ts := time.Date(2026, 12, 31, 22, 30, 0, 0, time.UTC)
	if got := FormatCertificateDate(ts, ist, i18n.LocaleTR); got != "01.01.2027" {
		t.Fatalf("tr = %s", got)
	}
	if got := FormatCertificateDate(ts, ist, i18n.LocaleEN); got != "2027-01-01" {
		t.Fatalf("en = %s", got)
	}
	if got := FormatCertificateDate(ts, nil, i18n.LocaleEN); got != "2026-12-31" {
		t.Fatalf("utc = %s", got)
	}
}

func TestCertificateVerifyURL(t *testing.T) {
	s := NewCertificate(nil, nil, "https://olexfilms.app/", nil)
	if got := s.VerifyURL("abcdefghijkl"); got != "https://olexfilms.app/garanti/abcdefghijkl" {
		t.Fatalf("verify url = %s", got)
	}
}
