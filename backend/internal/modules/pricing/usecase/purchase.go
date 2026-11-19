package usecase

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

// PurchasePrice is the effective purchase price one organization pays for a
// product in one currency, with the price list it came from.
type PurchasePrice struct {
	Price  string
	Source string // SourceList, SourceOverride or SourceDistributor
	// MeterPrice (TEC-1238, K2) is the price of one meter of the product
	// for a meter line ("" when none can be computed). A roll_meter
	// product's price is already per meter. A piece product's meter price
	// carries the buyer's box price ratio: box price × (meter list ÷ box
	// list); without a meter list price, box price ÷ box length.
	MeterPrice string
}

// BuyerPurchasePrices returns the effective purchase prices of an
// organization (distributor or dealer) for products of its brand in one
// currency: the same rule as the effective view (K8: the purchase price of a
// level is the sale price of the level above), without the viewer's read
// grants, for server-side callers such as orders (TEC-166). Products without
// a price in that currency are absent from the map.
func (s *Service) BuyerPurchasePrices(ctx context.Context, buyerOrgID int64, buyerType string, brandID int64,
	productIDs []int64, currency string) (map[int64]PurchasePrice, error) {
	out := map[int64]PurchasePrice{}
	if len(productIDs) == 0 {
		return out, nil
	}
	if buyerType != OrgDistributor && buyerType != OrgDealer {
		return nil, ErrForbidden
	}
	v := Viewer{OrgID: buyerOrgID, OrgType: buyerType, BrandID: brandID, PurchaseRead: true}
	refs := make([]productRef, 0, len(productIDs))
	for _, id := range productIDs {
		refs = append(refs, productRef{ID: id})
	}
	views, err := s.views(ctx, v, refs)
	if err != nil {
		return nil, err
	}
	cur := strings.ToUpper(strings.TrimSpace(currency))
	for i, view := range views {
		for _, p := range view.Prices {
			if p.Currency == cur && p.PurchasePrice != nil {
				out[refs[i].ID] = PurchasePrice{Price: *p.PurchasePrice, Source: p.PurchasePriceSource}
			}
		}
	}
	if len(out) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(out))
	for id := range out {
		ids = append(ids, id)
	}
	terms, err := s.q.ListProductMeterTerms(ctx, db.ListProductMeterTermsParams{BrandID: brandID, Currency: cur, ProductIds: ids})
	if err != nil {
		return nil, fmt.Errorf("meter terms: %w", err)
	}
	for _, t := range terms {
		p, ok := out[t.ProductID]
		if !ok {
			continue
		}
		if m, ok := MeterPrice(t.UnitType, p.Price, t.ListBoxPrice, t.ListMeterPrice, t.DefaultRollMeters); ok {
			p.MeterPrice = m
			out[t.ProductID] = p
		}
	}
	return out, nil
}

// MeterPrice computes a buyer's price of one meter (TEC-1238, K2) from its
// box price (buyerBox), the center's box and meter list prices and the box
// length, rounded to four decimals. Example: box list 400, the buyer pays
// 360 (10 % off); meter list 30 -> the buyer pays 27. Without a meter list
// price (or a box list price to compare): 360 / 15 m = 24. ok is false when
// neither rule applies. A roll_meter product's price is already per meter.
func MeterPrice(unitType, buyerBox string, listBox, listMeter, boxMeters pgtype.Numeric) (string, bool) {
	box, ok := new(big.Rat).SetString(strings.TrimSpace(buyerBox))
	if !ok || box.Sign() < 0 {
		return "", false
	}
	if unitType == "roll_meter" {
		return box.FloatString(4), true
	}
	if lm, lb := ratOf(listMeter), ratOf(listBox); lm != nil && lb != nil && lb.Sign() > 0 {
		v := new(big.Rat).Mul(box, lm)
		return v.Quo(v, lb).FloatString(4), true
	}
	if m := ratOf(boxMeters); m != nil && m.Sign() > 0 {
		return new(big.Rat).Quo(box, m).FloatString(4), true
	}
	return "", false
}

func ratOf(n pgtype.Numeric) *big.Rat {
	p := numericText(n)
	if p == nil {
		return nil
	}
	r, ok := new(big.Rat).SetString(*p)
	if !ok {
		return nil
	}
	return r
}
