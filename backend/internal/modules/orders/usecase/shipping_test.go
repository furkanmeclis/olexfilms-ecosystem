package usecase

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

func TestTrackingURL(t *testing.T) {
	cases := []struct{ tpl, no, want string }{
		{"https://t.example/q?no={tracking_no}", "AB 12/3", "https://t.example/q?no=AB%2012%2F3"},
		{"https://t.example/{tracking_no}/x", "X1", "https://t.example/X1/x"},
		{"https://t.example/static", "X1", "https://t.example/static"},
		{"", "X1", ""},
		{"https://t.example/{tracking_no}", "  ", ""},
	}
	for _, c := range cases {
		if got := TrackingURL(c.tpl, c.no); got != c.want {
			t.Errorf("TrackingURL(%q, %q) = %q, want %q", c.tpl, c.no, got, c.want)
		}
	}
}

func TestDeliveryMethodInputValidate(t *testing.T) {
	ptr := func(s string) *string { return &s }
	ok := DeliveryMethodInput{Code: " Aras_1 ", Name: " Aras ", TrackingURLTemplate: ptr("https://a.example/{tracking_no}")}
	f, err := ok.validate()
	if err != nil || f.code != "aras_1" || f.name != "Aras" || !f.template.Valid || !f.active {
		t.Fatalf("valid input = %+v %v", f, err)
	}
	if f, err := (DeliveryMethodInput{Code: "x", Name: "X", TrackingURLTemplate: ptr("  ")}).validate(); err != nil || f.template.Valid {
		t.Fatalf("blank template = %+v %v", f, err)
	}
	bad := []DeliveryMethodInput{
		{Code: "", Name: "X"},
		{Code: "bad code", Name: "X"},
		{Code: SystemDeliveryMethodCode, Name: "X"},
		{Code: "x", Name: " "},
		{Code: "x", Name: "X", TrackingURLTemplate: ptr("ftp://a.example/{tracking_no}")},
		{Code: "x", Name: "X", TrackingURLTemplate: ptr("not a url")},
		{Code: "x", Name: "X", TrackingURLTemplate: ptr("https://a.example/{tracking_no} x")},
	}
	for _, in := range bad {
		var ve *ValidationError
		if _, err := in.validate(); !errors.As(err, &ve) {
			t.Errorf("%+v: want a validation error, got %v", in, err)
		}
	}
}

func TestShippingDocumentHTML(t *testing.T) {
	d := ShippingDoc{
		Locale: i18n.LocaleAR, OrderNo: "ORD-1", ShippedAt: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), Zone: time.UTC,
		Seller: ShippingParty{Name: "Center <b>"}, Buyer: ShippingParty{Name: "Dist", TaxNo: "123"},
		DeliveryMethod: "Aras", TrackingNo: "T1", TrackingURL: "https://t.example/T1",
		Lines:       []ShippingLine{{SKU: "S1", Name: "Film", Amount: "2 pcs", Barcodes: []string{"B1", "B2"}}},
		GeneratedAt: time.Date(2026, 10, 1, 9, 5, 0, 0, time.UTC),
	}
	out := ShippingDocumentHTML(d)
	for _, want := range []string{`dir="rtl"`, "ORD-1", "Center &lt;b&gt;", "B1", "B2", "Aras", "T1", "123",
		i18n.Translate(i18n.LocaleAR, "orders.pdf.shipping_note")} {
		if !strings.Contains(out, want) {
			t.Errorf("document misses %q", want)
		}
	}
	if strings.Contains(out, "Center <b>") {
		t.Fatal("values must be escaped")
	}
}
