package ubl

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	ubltr "github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/codelist"
)

var update = flag.Bool("update", false, "rewrite testdata golden files")

const (
	testNumber = "EAR2026000000001"
	testUUID   = "5f2a7d1e-6b43-4c1a-9d0e-2b8f3c4a5e61"
)

func seller() Party {
	return Party{
		Name: "Olexfilms Merkez A.Ş.", VKN: "9000068418", TaxOffice: "Büyük Mükellefler",
		Street: "Papatya Caddesi No:21", District: "Beşiktaş", City: "İstanbul", PostalZone: "34100",
		Email: "fatura@olexfilms.example", Phone: "+902120000000", MersisNo: "0123456789000015",
	}
}

func companyBuyer() Party {
	return Party{
		Name: "Ege Distribütör Ltd. Şti.", VKN: "1234567890", TaxOffice: "Konak",
		Street: "Kordon Boyu No:5", District: "Konak", City: "İzmir",
	}
}

// sampleOrder mixes %20, %10 and %1 KDV with a percent and an amount line
// discount. Expected totals by hand:
//
//	line 1: 2 × 1000.00 = 2000.00 − 10% = 1800.00; KDV 20% = 360.00
//	line 2: 12.5 m × 80.00 = 1000.00 − 50.00 = 950.00; KDV 10% = 95.00
//	line 3: 3 × 33.33 = 99.99; KDV 1% = 0.9999 → 1.00
//	line extension 2849.99, KDV 456.00, payable 3305.99
func sampleOrder() Document {
	return Document{
		Seller: seller(),
		Buyer:  companyBuyer(),
		Lines: []Line{
			{Name: "Seramik cam filmi 152 cm", Quantity: "2", UnitCode: codelist.UnitC62, UnitPrice: "1000.00", VATRate: "20", DiscountPercent: "10"},
			{Name: "PPF rulo (metre)", Quantity: "12.5", UnitCode: codelist.UnitMTR, UnitPrice: "80.00", VATRate: "10", DiscountAmount: "50.00", DiscountReason: "Kampanya"},
			{Name: "Uygulama kitapçığı", Quantity: "3", UnitPrice: "33.33", VATRate: "1"},
		},
		IssueAt:        time.Date(2026, 10, 9, 9, 30, 0, 0, time.UTC),
		Notes:          []string{"Teşekkür ederiz."},
		OrderReference: "ORD-2026-0042",
		Payment:        &Payment{IBAN: "TR33 0006 1005 1978 6457 8413 26", DueDate: time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)},
	}
}

func build(t *testing.T, doc Document) (string, []byte) {
	t.Helper()
	profile := SelectProfile(doc.Buyer, "")
	inv, err := Build(doc, profile, testNumber, testUUID)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := AttachXSLT(inv, DefaultXSLT()); err != nil {
		t.Fatalf("AttachXSLT: %v", err)
	}
	xml, err := Marshal(inv)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return profile, xml
}

func mustValid(t *testing.T, xml []byte, profile string) {
	t.Helper()
	v, err := Validate(context.Background(), xml, profile)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if v.Status != StatusValid || !v.Archivable() {
		t.Fatalf("validation = %s %v\n%s", v.Status, v.Messages, xml)
	}
}

func amount(t *testing.T, xml []byte, elem string) string {
	t.Helper()
	m := regexp.MustCompile(`<` + elem + `\b[^>]*currencyID="TRY">([0-9.]+)</` + elem + `>`).FindSubmatch(xml)
	if m == nil {
		t.Fatalf("%s not found", elem)
	}
	return string(m[1])
}

// hasElem matches <name ...attrs>value</name>; go-ubltr writes default
// namespaces instead of cbc: prefixes.
func hasElem(xml []byte, name, attrs, value string) bool {
	return regexp.MustCompile(`<` + name + `\b[^>]*` + attrs + `[^>]*>` + regexp.QuoteMeta(value) + `</` + name + `>`).Match(xml)
}

func assertElems(t *testing.T, xml []byte, wants ...[3]string) {
	t.Helper()
	for _, w := range wants {
		if !hasElem(xml, w[0], w[1], w[2]) {
			t.Errorf("xml lacks <%s %s>%s", w[0], w[1], w[2])
		}
	}
}

func TestBuildOrderValidXSDAndEArchiveSchematron(t *testing.T) {
	profile, xml := build(t, sampleOrder())
	if profile != codelist.ProfileEArsivFatura {
		t.Fatalf("profile = %s, want EARSIVFATURA", profile)
	}
	if err := ubltr.ValidateXSD(context.Background(), xml); err != nil {
		t.Fatalf("ValidateXSD: %v", err)
	}
	if err := ubltr.ValidateSchematronType(context.Background(), xml, ubltr.TypeEArchive); err != nil {
		t.Fatalf("e-Arşiv Schematron: %v", err)
	}
	mustValid(t, xml, profile)
	assertElems(t, xml,
		[3]string{"ProfileID", "", "EARSIVFATURA"},
		[3]string{"ID", "", testNumber},
		[3]string{"UUID", "", testUUID},
		[3]string{"IssueDate", "", "2026-10-09"},
		[3]string{"IssueTime", "", "12:30:00"},
		[3]string{"Note", "", "Sipariş No: ORD-2026-0042"},
		[3]string{"InvoicedQuantity", `unitCode="MTR"`, "12.5"},
		[3]string{"ID", "", "TR330006100519786457841326"},
		[3]string{"PaymentDueDate", "", "2026-10-23"},
		[3]string{"DocumentType", "", "XSLT"},
		[3]string{"ID", `schemeID="MERSISNO"`, "0123456789000015"},
	)
}

func TestBuildTotalsMatchHandComputed(t *testing.T) {
	_, xml := build(t, sampleOrder())
	for elem, want := range map[string]string{
		"LineExtensionAmount": "2849.99", // LegalMonetaryTotal precedes the lines
		"TaxExclusiveAmount":  "2849.99",
		"TaxInclusiveAmount":  "3305.99",
		"PayableAmount":       "3305.99",
	} {
		if got := amount(t, xml, elem); got != want {
			t.Errorf("%s = %s, want %s", elem, got, want)
		}
	}
	inv, err := Build(sampleOrder(), ProfileEArchive, testNumber, testUUID)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"1800.00", "950.00", "99.99"} {
		if got := inv.InvoiceLine[i].LineExtensionAmount.Value; got != want {
			t.Errorf("line %d extension = %s, want %s", i+1, got, want)
		}
	}
	if len(inv.TaxTotal) != 1 || inv.TaxTotal[0].TaxAmount.Value != "456.00" {
		t.Fatalf("tax total = %+v, want 456.00", inv.TaxTotal)
	}
	got := map[string]string{}
	for _, s := range inv.TaxTotal[0].TaxSubtotal {
		got[s.Percent.Value] = s.TaxableAmount.Value + "/" + s.TaxAmount.Value
	}
	want := map[string]string{"20": "1800.00/360.00", "10": "950.00/95.00", "1": "99.99/1.00"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("KDV %%%s = %q, want %q (all %v)", k, got[k], v, got)
		}
	}
}

func TestBuildTCKNBuyerIsPerson(t *testing.T) {
	doc := sampleOrder()
	doc.Buyer = Party{Name: "Ayşe Nur Yılmaz", TCKN: "10000000146", District: "Çankaya", City: "Ankara"}
	profile, xml := build(t, doc)
	mustValid(t, xml, profile)
	assertElems(t, xml,
		[3]string{"ID", `schemeID="TCKN"`, "10000000146"},
		[3]string{"FirstName", "", "Ayşe Nur"},
		[3]string{"FamilyName", "", "Yılmaz"},
	)
}

func TestBuildRejectsBadTaxIDWith422(t *testing.T) {
	for name, mutate := range map[string]func(*Document){
		"buyer vkn check digit":  func(d *Document) { d.Buyer.VKN = "1234567891" },
		"buyer tckn check digit": func(d *Document) { d.Buyer.VKN = ""; d.Buyer.TCKN = "10000000147" },
		"buyer vkn length":       func(d *Document) { d.Buyer.VKN = "123456789" },
		"seller vkn":             func(d *Document) { d.Seller.VKN = "9000068419" },
		"buyer no id":            func(d *Document) { d.Buyer.VKN = "" },
	} {
		t.Run(name, func(t *testing.T) {
			doc := sampleOrder()
			mutate(&doc)
			_, err := Build(doc, ProfileEArchive, testNumber, testUUID)
			var e *Error
			if !errors.Is(err, ErrInvalidTaxID) || !errors.As(err, &e) {
				t.Fatalf("err = %v, want ErrInvalidTaxID", err)
			}
			if e.Code != CodeInvalidTaxID || e.HTTPStatus() != http.StatusUnprocessableEntity {
				t.Fatalf("code/status = %s/%d", e.Code, e.HTTPStatus())
			}
		})
	}
}

func TestTaxIDCheckDigits(t *testing.T) {
	for _, s := range []string{"9000068418", "1234567890", "9876543217"} {
		if !ValidVKN(s) {
			t.Errorf("ValidVKN(%s) = false", s)
		}
	}
	for _, s := range []string{"9000068419", "1288331521", "12883315", "12883315a1"} {
		if ValidVKN(s) {
			t.Errorf("ValidVKN(%s) = true", s)
		}
	}
	for _, s := range []string{"10000000146", "12345678950", FinalConsumerTCKN} {
		if !ValidTCKN(s) {
			t.Errorf("ValidTCKN(%s) = false", s)
		}
	}
	for _, s := range []string{"10000000147", "01234567890", "1234567895"} {
		if ValidTCKN(s) {
			t.Errorf("ValidTCKN(%s) = true", s)
		}
	}
	if err := ValidateTaxID("vkn", "9000068418"); err != nil {
		t.Errorf("ValidateTaxID: %v", err)
	}
	if err := ValidateTaxID("vkn", "12345"); !errors.Is(err, ErrInvalidTaxID) {
		t.Errorf("ValidateTaxID short = %v", err)
	}
}

func TestEInvoiceRegisteredBuyerGetsTicariFatura(t *testing.T) {
	doc := sampleOrder()
	doc.Buyer.EInvoiceRegistered = true
	profile, xml := build(t, doc)
	if profile != codelist.ProfileTicariFatura {
		t.Fatalf("profile = %s, want TICARIFATURA", profile)
	}
	if !hasElem(xml, "ProfileID", "", "TICARIFATURA") {
		t.Fatal("xml profile is not TICARIFATURA")
	}
	mustValid(t, xml, profile)
	if got := SelectProfile(doc.Buyer, ProfileBasic); got != ProfileBasic {
		t.Fatalf("SelectProfile(TEMELFATURA setting) = %s", got)
	}
	if _, err := Build(doc, ProfileEArchive, testNumber, testUUID); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("e-Arşiv for a registered buyer: err = %v", err)
	}
}

func TestForeignBuyerOrCurrencyUnsupported(t *testing.T) {
	for name, mutate := range map[string]func(*Document){
		"foreign buyer": func(d *Document) { d.Buyer.CountryCode = "DE" },
		"foreign currency": func(d *Document) {
			d.Currency = "EUR"
		},
	} {
		t.Run(name, func(t *testing.T) {
			doc := sampleOrder()
			mutate(&doc)
			_, err := Build(doc, ProfileEArchive, testNumber, testUUID)
			var e *Error
			if !errors.Is(err, ErrUnsupportedBuyer) || !errors.As(err, &e) || e.Code != CodeUnsupportedBuyer {
				t.Fatalf("err = %v, want ErrUnsupportedBuyer", err)
			}
		})
	}
}

func TestBuildRejectsBadLines(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*Line)
		want   error
	}{
		"zero quantity":       {func(l *Line) { l.Quantity = "0" }, ErrInvalidDocument},
		"unknown unit":        {func(l *Line) { l.UnitCode = "XYZ1" }, ErrInvalidDocument},
		"bad vat":             {func(l *Line) { l.VATRate = "abc" }, ErrInvalidVATRate},
		"zero vat no reason":  {func(l *Line) { l.VATRate = "0" }, ErrInvalidVATRate},
		"discount both kinds": {func(l *Line) { l.DiscountAmount = "1" }, ErrInvalidDocument},
		"discount over 100%":  {func(l *Line) { l.DiscountPercent = "120" }, ErrInvalidDocument},
	} {
		t.Run(name, func(t *testing.T) {
			doc := sampleOrder()
			tc.mutate(&doc.Lines[0])
			if _, err := Build(doc, ProfileEArchive, testNumber, testUUID); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestValidateReportsInvalidAndRuleWarnings(t *testing.T) {
	_, xml := build(t, sampleOrder())

	// An element the Invoice schema does not know.
	broken := bytes.Replace(xml, []byte("<InvoiceTypeCode "), []byte("<Bogus/><InvoiceTypeCode "), 1)
	v, err := Validate(context.Background(), broken, ProfileEArchive)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != StatusInvalid || v.Archivable() || len(v.Messages) == 0 {
		t.Fatalf("broken xsd: %+v", v)
	}

	// The e-Arşiv XML checked against the e-Fatura rules: XSD passes but GİB
	// Schematron rejects the EARSIVFATURA profile for type=efatura.
	v, err = Validate(context.Background(), xml, ProfileCommercial)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != StatusRuleWarnings || v.Archivable() || len(v.Messages) == 0 {
		t.Fatalf("schematron: %+v", v)
	}
}

func TestAttachXSLTEmbedsStylesheet(t *testing.T) {
	custom := []byte(`<?xml version="1.0"?><xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform"><xsl:template match="/"><html><body>ozel</body></html></xsl:template></xsl:stylesheet>`)
	if got := Stylesheet(custom); !bytes.Equal(got, custom) {
		t.Fatal("uploaded XSLT is not preferred")
	}
	if got := Stylesheet(nil); !bytes.Equal(got, defaultXSLT) {
		t.Fatal("default XSLT is not used without an upload")
	}
	inv, err := Build(sampleOrder(), ProfileEArchive, testNumber, testUUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := AttachXSLT(inv, custom); err != nil {
		t.Fatal(err)
	}
	ref := inv.AdditionalDocumentReference[len(inv.AdditionalDocumentReference)-1]
	data, err := base64.StdEncoding.DecodeString(ref.Attachment.EmbeddedDocumentBinaryObject.Value)
	if err != nil || !bytes.Equal(data, custom) || ref.DocumentType.Value != "XSLT" {
		t.Fatalf("attachment = %+v (%v)", ref, err)
	}
	if err := AttachXSLT(inv, nil); !errors.Is(err, ErrInvalidStylesheet) {
		t.Fatalf("empty xslt: %v", err)
	}
}

func TestRenderHTMLGolden(t *testing.T) {
	_, xml := build(t, sampleOrder())
	html, err := RenderHTML(context.Background(), xml, Stylesheet(nil))
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	if len(bytes.TrimSpace(html)) == 0 || !bytes.Contains(html, []byte(testNumber)) {
		t.Fatalf("html is empty or lacks the invoice number (%d bytes)", len(html))
	}
	golden := filepath.Join("testdata", "order_earchive.html")
	if *update {
		if err := os.WriteFile(golden, html, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run with -update): %v", err)
	}
	if !bytes.Equal(html, want) {
		t.Fatalf("html differs from %s; run go test -run TestRenderHTMLGolden -update", golden)
	}
}

func TestRenderHTMLBrokenStylesheet(t *testing.T) {
	_, xml := build(t, sampleOrder())
	_, err := RenderHTML(context.Background(), xml, []byte("<xsl:stylesheet"))
	if !errors.Is(err, ErrInvalidStylesheet) {
		t.Fatalf("err = %v, want ErrInvalidStylesheet", err)
	}
	if _, err := RenderHTML(context.Background(), xml, nil); !errors.Is(err, ErrInvalidStylesheet) {
		t.Fatalf("empty stylesheet: %v", err)
	}
	if !strings.Contains(string(DefaultXSLT()), "xsl:stylesheet") {
		t.Fatal("embedded general.xslt is missing")
	}
}
