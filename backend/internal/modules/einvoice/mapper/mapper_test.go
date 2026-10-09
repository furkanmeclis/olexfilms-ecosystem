package mapper

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/ubl"
)

func num(s string) pgtype.Numeric {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		panic(err)
	}
	return n
}

func txt(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

var issueAt = time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)

func settings() db.EinvoiceSetting {
	return db.EinvoiceSetting{
		Vkn: "9000068418", TaxOffice: "Beşiktaş", LegalName: "Olexfilms Merkez A.Ş.",
		Address: "Papatya Cad. No:21", City: "İstanbul", District: "Beşiktaş", Country: "TR",
		Iban: txt("TR330006100519786457841326"), Email: txt("fatura@olexfilms.example"),
		MersisNo: txt("0123456789000015"), DefaultNote: txt("İrsaliye yerine geçmez."),
		EarchiveSeries: "EAR", EfaturaSeries: "EFN",
	}
}

func distributor() db.Organization {
	return db.Organization{
		ID: 7, Name: "Ege Dist", Type: "distributor", Address: "Kordon No:5", City: "İzmir", District: "Konak",
		Email: "info@ege.example", Currency: "TRY",
		InvoiceVkn: txt("1234567890"), InvoiceTaxOffice: txt("Konak"), InvoiceLegalName: txt("Ege Distribütör Ltd. Şti."),
	}
}

func order() OrderInput {
	return OrderInput{
		Settings: settings(),
		Order:    db.Order{OrderNo: "ORD-2026-0042", Currency: "TRY"},
		Lines: []OrderLine{
			{
				Item:    db.OrderItem{Quantity: pgtype.Int4{Int32: 3, Valid: true}, UnitPrice: num("250.00")},
				Product: db.Product{ID: 1, CategoryID: 10, Name: "Cam filmi kutu", Sku: "CF-1", UnitType: "piece"},
			},
			{
				Item:    db.OrderItem{Meters: num("12.5"), UnitPrice: num("80.00")},
				Product: db.Product{ID: 2, CategoryID: 20, Name: "PPF rulo", Sku: "PPF-152", UnitType: "roll_meter"},
			},
			{
				Item:    db.OrderItem{Quantity: pgtype.Int4{Int32: 2, Valid: true}, UnitPrice: num("49.90")},
				Product: db.Product{ID: 3, CategoryID: 30, Name: "Katalog", UnitType: "piece"},
			},
		},
		Buyer:        distributor(),
		BuyerCountry: "TR",
		VAT:          VATRates{ByProduct: map[int64]string{3: "1"}, ByCategory: map[int64]string{20: "10"}},
		IssueAt:      issueAt,
	}
}

func buildXML(t *testing.T, doc ubl.Document, registered string) (string, []byte, *ublInvoice) {
	t.Helper()
	profile := ubl.SelectProfile(doc.Buyer, registered)
	inv, err := ubl.Build(doc, profile, "EAR2026000000007", "0b9c1f9e-1a2b-4c3d-8e4f-5a6b7c8d9e0f")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := ubl.AttachXSLT(inv, ubl.DefaultXSLT()); err != nil {
		t.Fatal(err)
	}
	xml, err := ubl.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	v, err := ubl.Validate(context.Background(), xml, profile)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != ubl.StatusValid {
		t.Fatalf("validation %s: %v", v.Status, v.Messages)
	}
	return profile, xml, &ublInvoice{
		payable: inv.LegalMonetaryTotal.PayableAmount.Value,
		tax:     inv.TaxTotal[0].TaxAmount.Value,
		ext:     inv.LegalMonetaryTotal.LineExtensionAmount.Value,
	}
}

type ublInvoice struct{ payable, tax, ext string }

func TestFromOrderBuildsValidEArchive(t *testing.T) {
	doc, err := FromOrder(order())
	if err != nil {
		t.Fatal(err)
	}
	// 3 × 250.00 = 750.00 @20% = 150.00; 12.5 m × 80.00 = 1000.00 @10% =
	// 100.00; 2 × 49.90 = 99.80 @1% = 1.00 → 1849.80 + 251.00 = 2100.80
	want := []ubl.Line{
		{Name: "Cam filmi kutu (CF-1)", Quantity: "3", UnitCode: "C62", UnitPrice: "250", VATRate: "20"},
		{Name: "PPF rulo (PPF-152)", Quantity: "12.5", UnitCode: "MTR", UnitPrice: "80", VATRate: "10"},
		{Name: "Katalog", Quantity: "2", UnitCode: "C62", UnitPrice: "49.9", VATRate: "1"},
	}
	for i, w := range want {
		if doc.Lines[i] != w {
			t.Errorf("line %d = %+v, want %+v", i, doc.Lines[i], w)
		}
	}
	if doc.Seller.VKN != "9000068418" || doc.Buyer.Name != "Ege Distribütör Ltd. Şti." || doc.OrderReference != "ORD-2026-0042" {
		t.Fatalf("parties/reference: %+v / %+v / %s", doc.Seller, doc.Buyer, doc.OrderReference)
	}
	if doc.Payment == nil || doc.Payment.IBAN != "TR330006100519786457841326" {
		t.Fatalf("payment = %+v", doc.Payment)
	}
	profile, _, inv := buildXML(t, doc, "")
	if profile != ubl.ProfileEArchive {
		t.Fatalf("profile = %s", profile)
	}
	if inv.ext != "1849.80" || inv.tax != "251.00" || inv.payable != "2100.80" {
		t.Fatalf("totals = %+v, want 1849.80/251.00/2100.80", inv)
	}
}

func TestFromOrderRegisteredDistributorGetsTicariFatura(t *testing.T) {
	in := order()
	in.Buyer.EinvoiceRegistered = true
	doc, err := FromOrder(in)
	if err != nil {
		t.Fatal(err)
	}
	if profile, _, _ := buildXML(t, doc, ""); profile != ubl.ProfileCommercial {
		t.Fatalf("profile = %s, want TICARIFATURA", profile)
	}
	if profile, _, _ := buildXML(t, doc, ubl.ProfileBasic); profile != ubl.ProfileBasic {
		t.Fatalf("profile = %s, want TEMELFATURA", profile)
	}
}

func TestFromOrderForeignBuyerUnsupported(t *testing.T) {
	in := order()
	in.BuyerCountry = "DE"
	if _, err := FromOrder(in); !errors.Is(err, ErrUnsupportedBuyer) {
		t.Fatalf("foreign buyer: %v", err)
	}
	in = order()
	in.Order.Currency = "EUR"
	if _, err := FromOrder(in); !errors.Is(err, ErrUnsupportedBuyer) {
		t.Fatalf("foreign currency: %v", err)
	}
}

func TestFromOrderInvalidBuyerVKNIs422(t *testing.T) {
	in := order()
	in.Buyer.InvoiceVkn = txt("1234567891")
	doc, err := FromOrder(in)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ubl.Build(doc, ubl.ProfileEArchive, "EAR2026000000007", "0b9c1f9e-1a2b-4c3d-8e4f-5a6b7c8d9e0f")
	var e *ubl.Error
	if !errors.As(err, &e) || e.Code != ubl.CodeInvalidTaxID || e.HTTPStatus() != 422 {
		t.Fatalf("err = %v", err)
	}
}

func TestVATRatesFallback(t *testing.T) {
	v := VATRates{ByProduct: map[int64]string{1: "1"}, ByCategory: map[int64]string{9: "10"}}
	if v.For(1, 9) != "1" || v.For(2, 9) != "10" || v.For(2, 8) != DefaultVATRate {
		t.Fatalf("For: %s %s %s", v.For(1, 9), v.For(2, 9), v.For(2, 8))
	}
	v.Default = "18"
	if v.For(2, 8) != "18" {
		t.Fatalf("sysconfig default not used: %s", v.For(2, 8))
	}
}

func TestFromSubscriptionPeriodBuildsValidXML(t *testing.T) {
	dealer := distributor()
	dealer.Type = "dealer"
	dealer.InvoiceVkn = pgtype.Text{}
	dealer.InvoiceTckn = txt("10000000146")
	dealer.InvoiceLegalName = txt("Mehmet Can Demir")
	doc, err := FromSubscriptionPeriod(SubscriptionInput{
		Settings: settings(),
		Period: db.ServiceSubscriptionPeriod{
			PeriodStart: pgtype.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			PeriodEnd:   pgtype.Date{Time: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), Valid: true},
		},
		Subscription: db.ServiceSubscription{Price: num("1500.00"), Currency: "TRY"},
		Item:         db.ServiceCatalogItem{Name: "Pazarlama paketi"},
		Buyer:        dealer,
		IssueAt:      issueAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Lines[0].Name != "Pazarlama paketi (01.09.2026 – 30.09.2026)" || doc.Lines[0].VATRate != "20" {
		t.Fatalf("line = %+v", doc.Lines[0])
	}
	_, _, inv := buildXML(t, doc, "")
	if inv.payable != "1800.00" {
		t.Fatalf("payable = %s, want 1800.00", inv.payable)
	}
}

func dealerSeller() DealerSeller {
	o := distributor()
	o.Type = "dealer"
	o.Name = "Kadıköy Oto Film"
	o.InvoiceVkn = txt("9876543217")
	o.InvoiceLegalName = txt("Kadıköy Oto Film Ltd. Şti.")
	o.EinvoiceRegistered = true
	return DealerSeller{Org: o, Country: "TR", IBAN: "TR330006100519786457841326"}
}

func consumer() ubl.Party {
	return ubl.Party{Name: "Ali Veli", TCKN: ubl.FinalConsumerTCKN, District: "Kadıköy", City: "İstanbul"}
}

func TestFromProductSaleBuildsValidXML(t *testing.T) {
	in := ProductSaleInput{
		Seller: dealerSeller(),
		Sale:   db.ProductSale{Currency: "TRY", SoldAt: pgtype.Timestamptz{Time: issueAt, Valid: true}, Note: "Kasa satışı"},
		Lines: []ProductSaleLine{
			{Line: db.ProductSaleLine{Quantity: num("2"), UnitPrice: num("600.00")}, Product: db.Product{ID: 1, Name: "Cam filmi", UnitType: "piece"}},
			{Line: db.ProductSaleLine{Quantity: num("1.5"), UnitPrice: num("110.00")}, Product: db.Product{ID: 2, CategoryID: 20, Name: "PPF", UnitType: "roll_meter"}},
		},
		Buyer:       consumer(),
		VAT:         VATRates{ByCategory: map[int64]string{20: "10"}},
		VATIncluded: true,
	}
	doc, err := FromProductSale(in)
	if err != nil {
		t.Fatal(err)
	}
	// KDV-inclusive 600.00 @20% → 500; 110.00 @10% → 100.
	if doc.Lines[0].UnitPrice != "500" || doc.Lines[1].UnitPrice != "100" || doc.Lines[1].UnitCode != "MTR" {
		t.Fatalf("lines = %+v", doc.Lines)
	}
	if doc.Seller.EInvoiceRegistered || doc.Notes[0] != "Kasa satışı" || !doc.IssueAt.Equal(issueAt) {
		t.Fatalf("doc = %+v", doc)
	}
	profile, _, inv := buildXML(t, doc, "")
	if profile != ubl.ProfileEArchive {
		t.Fatalf("profile = %s", profile)
	}
	// Sale total 2 × 600 + 1.5 × 110 = 1365.00 incl. KDV.
	if inv.payable != "1365.00" || inv.ext != "1150.00" || inv.tax != "215.00" {
		t.Fatalf("totals = %+v", inv)
	}

	in.VATIncluded = false
	doc, err = FromProductSale(in)
	if err != nil || doc.Lines[0].UnitPrice != "600" {
		t.Fatalf("VAT-exclusive price = %+v (%v)", doc.Lines, err)
	}

	in.Seller.Country = "NL"
	if _, err := FromProductSale(in); !errors.Is(err, ErrUnsupportedBuyer) {
		t.Fatalf("foreign dealer: %v", err)
	}
}

func TestFromServiceIncomeBuildsValidXML(t *testing.T) {
	doc, err := FromServiceIncome(ServiceIncomeInput{
		Seller:      dealerSeller(),
		Service:     db.Service{ServiceNo: "SRV-0099", IncomeAmount: num("2400.00"), CompletedAt: pgtype.Timestamptz{Time: issueAt, Valid: true}},
		Buyer:       consumer(),
		VATIncluded: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Lines[0].Name != "Hizmet SRV-0099" || doc.Lines[0].UnitPrice != "2000" {
		t.Fatalf("line = %+v", doc.Lines[0])
	}
	if _, _, inv := buildXML(t, doc, ""); inv.payable != "2400.00" {
		t.Fatalf("payable = %s", inv.payable)
	}
}

func TestNumericString(t *testing.T) {
	for in, want := range map[string]string{"12.50": "12.5", "1000": "1000", "0.0001": "0.0001"} {
		if got := NumericString(num(in)); got != want {
			t.Errorf("NumericString(%s) = %s, want %s", in, got, want)
		}
	}
	if NumericString(pgtype.Numeric{}) != "" {
		t.Error("NULL is not empty")
	}
	if got := NumericString(pgtype.Numeric{Int: big.NewInt(125), Exp: -1, Valid: true}); got != "12.5" {
		t.Errorf("got %s", got)
	}
}
