package usecase

import (
	"context"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// TEC-1238 (K2): a buyer's meter price keeps its box price ratio.
func TestMeterPrice(t *testing.T) {
	cases := []struct {
		name, unit, box    string
		listBox, listMeter string
		boxMeters, want    string
		ok                 bool
	}{
		{"discounted box, meter list", "piece", "360", "400", "30", "15", "27.0000", true},
		{"list buyer pays the meter list", "piece", "400", "400", "30", "15", "30.0000", true},
		{"no meter list: box / length", "piece", "400", "400", "", "15", "26.6667", true},
		{"no box list: box / length", "piece", "360", "", "30", "15", "24.0000", true},
		{"no meter list nor length", "piece", "400", "400", "", "", "", false},
		{"roll price is per meter", "roll_meter", "12.5", "", "", "", "12.5000", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := MeterPrice(c.unit, c.box, num(c.listBox), num(c.listMeter), num(c.boxMeters))
			if ok != c.ok || got != c.want {
				t.Fatalf("MeterPrice = %q, %v; want %q, %v", got, ok, c.want, c.ok)
			}
		})
	}
}

type meterFake struct {
	*fakeQ
	terms []db.ListProductMeterTermsRow
}

func (f meterFake) ListProductMeterTerms(context.Context, db.ListProductMeterTermsParams) ([]db.ListProductMeterTermsRow, error) {
	return f.terms, nil
}

// The dealer's meter price follows its distributor's dealer price.
func TestBuyerPurchasePricesMeter(t *testing.T) {
	f := newFake()
	f.prices[key{product: productID, cur: "USD"}] = db.ProductPrice{SaleToDistributorPrice: num("400"), MeterPrice: num("30")}
	f.dealer[key{product: productID, org: distributorID, cur: "USD"}] = "360"
	mf := meterFake{fakeQ: f, terms: []db.ListProductMeterTermsRow{{
		ProductID: productID, UnitType: "piece", DefaultRollMeters: num("15"),
		ListBoxPrice: num("400"), ListMeterPrice: num("30"),
	}}}
	got, err := New(mf).BuyerPurchasePrices(context.Background(), dealerID, OrgDealer, brandID, []int64{productID}, "USD")
	if err != nil {
		t.Fatal(err)
	}
	p := got[productID]
	if p.Price != "360" || p.MeterPrice != "27.0000" || p.Source != SourceDistributor {
		t.Fatalf("price = %+v", p)
	}
}
