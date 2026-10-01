package usecase

import (
	"context"
	"strings"
)

// PurchasePrice is the effective purchase price one organization pays for a
// product in one currency, with the price list it came from.
type PurchasePrice struct {
	Price  string
	Source string // SourceList, SourceOverride or SourceDistributor
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
	return out, nil
}
