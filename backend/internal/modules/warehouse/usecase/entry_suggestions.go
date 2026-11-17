package usecase

// Placement suggestions (TEC-1230): where to put the unplaced lines of a
// stock entry. Like the picking suggestion (orders/usecase/picking.go,
// placed units first), the product goes where the entry's warehouse already
// keeps it, most stocked location first; empty leaf locations of the
// warehouse follow as the fallback. Suggestions are advisory: Place still
// accepts any active location of the warehouse.

import (
	"context"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
)

// Suggestion limits.
const (
	maxSuggestedPerProduct = 3
	maxSuggestedEmpty      = 5
)

// SuggestedLocation is one location to place into. Quantity is what the
// location already holds of the product (nil for an empty location).
type SuggestedLocation struct {
	UUID     uuid.UUID `json:"uuid"`
	Code     string    `json:"code"`
	FullCode *string   `json:"full_code"`
	Name     string    `json:"name"`
	Quantity *int32    `json:"quantity"`
}

// ProductPlacement lists the locations already holding one product.
type ProductPlacement struct {
	Product   EntryProduct        `json:"product"`
	Locations []SuggestedLocation `json:"locations"`
}

// PlacementSuggestions is the answer of
// GET /v1/warehouse/stock-entries/{uuid}/placement-suggestions.
type PlacementSuggestions struct {
	Products       []ProductPlacement  `json:"products"`
	EmptyLocations []SuggestedLocation `json:"empty_locations"`
}

// PlacementSuggestions returns the suggestions for the unplaced lines of an
// entry of the active organization.
func (s *StockEntries) PlacementSuggestions(ctx context.Context, c EntryCaller, id uuid.UUID) (PlacementSuggestions, error) {
	out := PlacementSuggestions{Products: []ProductPlacement{}, EmptyLocations: []SuggestedLocation{}}
	org, err := guard(c.Caller)
	if err != nil {
		return out, err
	}
	e, err := s.q.GetStockEntryByUUID(ctx, db.GetStockEntryByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return out, notFound(err, ErrEntryNotFound)
	}
	if !e.WarehouseID.Valid {
		return out, nil
	}
	products, err := s.q.ListEntryLineProducts(ctx, e.ID)
	if err != nil {
		return out, err
	}
	if len(products) > 0 {
		rows, err := s.q.ListWarehouseProductLocations(ctx, db.ListWarehouseProductLocationsParams{
			OrganizationID: org, WarehouseID: e.WarehouseID, ProductIds: products,
		})
		if err != nil {
			return out, err
		}
		byProduct := map[int64][]SuggestedLocation{}
		for _, r := range rows {
			if len(byProduct[r.ProductID]) >= maxSuggestedPerProduct {
				continue
			}
			qty := r.Quantity
			byProduct[r.ProductID] = append(byProduct[r.ProductID], SuggestedLocation{
				UUID: r.LocationUuid, Code: r.LocationCode, FullCode: textPtr(r.LocationFullCode),
				Name: r.LocationName, Quantity: &qty,
			})
		}
		for _, pid := range products {
			p, err := s.q.GetProductByIDAnyBrand(ctx, pid)
			if err != nil {
				return out, err
			}
			locs := byProduct[pid]
			if locs == nil {
				locs = []SuggestedLocation{}
			}
			out.Products = append(out.Products, ProductPlacement{
				Product:   EntryProduct{UUID: p.Uuid, SKU: p.Sku, Name: p.Name},
				Locations: locs,
			})
		}
	}
	empty, err := s.q.ListEmptyWarehouseLocations(ctx, db.ListEmptyWarehouseLocationsParams{
		OrganizationID: org, WarehouseID: e.WarehouseID, LimitCount: maxSuggestedEmpty,
	})
	if err != nil {
		return out, err
	}
	for _, r := range empty {
		out.EmptyLocations = append(out.EmptyLocations, SuggestedLocation{
			UUID: r.LocationUuid, Code: r.LocationCode, FullCode: textPtr(r.LocationFullCode), Name: r.LocationName,
		})
	}
	return out, nil
}
