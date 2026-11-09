package usecase

import (
	"math/big"
	"testing"
)

func TestDiscountedPrice(t *testing.T) {
	cases := []struct {
		list string
		pct  *big.Rat
		want string
	}{
		{"100", nil, "100.0000"},
		{"100", big.NewRat(0, 1), "100.0000"},
		{"100", big.NewRat(15, 1), "85.0000"},
		{"33.3333", big.NewRat(20, 1), "26.6666"},
		{"10", big.NewRat(125, 10), "8.7500"},
		{"10", big.NewRat(100, 1), "0.0000"},
	}
	for _, c := range cases {
		got, err := discountedPrice(c.list, c.pct)
		if err != nil || got != c.want {
			t.Errorf("discountedPrice(%s, %v) = %s, %v; want %s", c.list, c.pct, got, err, c.want)
		}
	}
}

func TestCharge(t *testing.T) {
	list := "40"
	manual := "12.5"
	two := big.NewRat(2, 1)
	pct := big.NewRat(20, 1)

	gift, err := charge(chargeInput{list: &list, source: "list", amount: two, discount: pct, gift: true})
	if err != nil || !gift.gift || numericText(gift.unitPrice, 2) != "0.00" || numericText(gift.lineTotal, 2) != "0.00" ||
		gift.discount.Valid || numericText(gift.listPrice, 2) != "40.00" || gift.source != "list" {
		t.Fatalf("gift = %+v %v", gift, err)
	}
	disc, err := charge(chargeInput{list: &list, source: "list", amount: two, discount: pct})
	if err != nil || numericText(disc.unitPrice, 2) != "32.00" || numericText(disc.lineTotal, 2) != "64.00" ||
		numericText(disc.discount, 2) != "20.00" {
		t.Fatalf("discounted = %+v %v", disc, err)
	}
	typed, err := charge(chargeInput{list: &list, source: "list", amount: two, discount: pct, manual: &manual})
	if err != nil || typed.source != PriceSourceManual || numericText(typed.unitPrice, 2) != "12.50" ||
		typed.discount.Valid || numericText(typed.lineTotal, 2) != "25.00" {
		t.Fatalf("manual = %+v %v", typed, err)
	}
	missing, err := charge(chargeInput{amount: two, discount: pct, source: "list"})
	if err != nil || missing.source != PriceSourceMissing || missing.listPrice.Valid || numericText(missing.lineTotal, 2) != "0.00" {
		t.Fatalf("missing = %+v %v", missing, err)
	}
	giftMissing, err := charge(chargeInput{amount: two, gift: true, source: "list"})
	if err != nil || giftMissing.source != PriceSourceMissing || !giftMissing.gift {
		t.Fatalf("gift without a price = %+v %v", giftMissing, err)
	}
}
