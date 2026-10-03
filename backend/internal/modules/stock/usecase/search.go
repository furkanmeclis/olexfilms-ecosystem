package usecase

// TEC-210: Meilisearch stock units index. A document carries the barcode,
// the product (SKU, name), the unit status and the bins it sits in
// (location full_code); the holding organizations (serial current state,
// fixed barcode holdings with quantity on hand), the brand, the status and
// the product are filterable. GET /v1/stock/organizations/{uuid}/units?q=
// filters the index on the organization the caller's stock.read scope
// reaches (TEC-216: a distributor reaches its dealers) and then loads the
// hits from Postgres with the same filters, so the index is never the only
// access check. Every ledger movement publishes a stock.* event with the
// unit uuid; the search sync refreshes the unit document from it. The
// command palette does not search this spec (ListScoped).

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SearchSpec is the Meilisearch spec (index suffix) of stock units.
const SearchSpec = searchengine.SpecStockUnits

// listedStatuses are the statuses the unit list shows without a status
// filter (stock on hand).
var listedStatuses = []string{"available", "placed"}

// IndexStore is what the stock units search adapter reads.
type IndexStore interface {
	ListStockUnitsForIndex(ctx context.Context) ([]db.ListStockUnitsForIndexRow, error)
	GetStockUnitForIndex(ctx context.Context, id uuid.UUID) (db.GetStockUnitForIndexRow, error)
}

// SearchAdapter indexes stock units in Meilisearch.
type SearchAdapter struct{ q IndexStore }

// NewSearchAdapter creates the stock units search adapter.
func NewSearchAdapter(q IndexStore) *SearchAdapter { return &SearchAdapter{q: q} }

// Spec implements searchengine.Adapter.
func (a *SearchAdapter) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         SearchSpec,
		LabelKey:   "search.specs_stock_units",
		Permission: rbac.PermStockRead,
		Icon:       "barcode",
		Searchable: []string{"title", "subtitle", "keywords"},
		Filterable: []string{"organization_ids", "brand_ids", "status", "product_id"},
		ListScoped: true,
	}
}

// ListAll implements searchengine.Adapter.
func (a *SearchAdapter) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	rows, err := a.q.ListStockUnitsForIndex(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]searchengine.Document, 0, len(rows))
	for _, r := range rows {
		out = append(out, unitDocument(db.GetStockUnitForIndexRow(r)))
	}
	return out, nil
}

// Document implements searchengine.Adapter. A missing unit is not
// indexable (the stale document is removed).
func (a *SearchAdapter) Document(ctx context.Context, id string) (searchengine.Document, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("stock units search: invalid uuid")
	}
	row, err := a.q.GetStockUnitForIndex(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return searchengine.Document{}, searchengine.ErrSkipDocument
	}
	if err != nil {
		return searchengine.Document{}, err
	}
	return unitDocument(row), nil
}

func unitDocument(r db.GetStockUnitForIndexRow) searchengine.Document {
	kw := searchengine.Keywords(r.Barcode, r.Sku, r.ProductName)
	kw = append(kw, searchengine.Keywords(r.LocationCodes...)...)
	return searchengine.Document{
		ID:              r.Uuid.String(),
		Spec:            SearchSpec,
		Title:           r.Barcode,
		Subtitle:        strings.Join(searchengine.Keywords(r.ProductName, r.Sku, strings.Join(r.LocationCodes, ", ")), " · "),
		Keywords:        kw,
		Href:            "/stock/units/" + r.Barcode,
		Icon:            "barcode",
		OrganizationIDs: r.HolderOrgIds,
		BrandIDs:        []int64{r.BrandID},
		Status:          r.Status,
		ProductID:       r.ProductID,
	}
}

// SetFinder enables index search in OrganizationUnits (nil: SQL only).
func (s *Service) SetFinder(f searchengine.ListFinder) { s.finder = f }

func (s *Service) indexEnabled() bool { return s.finder != nil && s.finder.Enabled() }

// unitIndexFilter is the Meilisearch filter of one organization's unit
// list: the holding organization, the brand of a brand scope, the product
// and the status (stock on hand without a status filter).
func unitIndexFilter(p db.ListOrganizationStockUnitRowsParams) string {
	f := (&searchengine.Filter{}).In("organization_ids", []int64{p.OrganizationID})
	if p.BrandID.Valid {
		f.Eq("brand_ids", p.BrandID.Int64)
	}
	if p.ProductID.Valid {
		f.Eq("product_id", p.ProductID.Int64)
	}
	if p.Status.Valid {
		f.EqString("status", p.Status.String)
	} else {
		f.InStrings("status", listedStatuses)
	}
	return f.String()
}

// searchUnitsIndexed answers a q search from the index; the hits are
// reloaded through ListOrganizationStockUnitRows with the same filters.
// handled is false when the caller must fall back to the SQL search.
func (s *Service) searchUnitsIndexed(ctx context.Context, p db.ListOrganizationStockUnitRowsParams) ([]db.ListOrganizationStockUnitRowsRow, int64, bool) {
	ids, total, err := s.finder.SearchIDs(ctx, SearchSpec, p.Q.String, unitIndexFilter(p), int(p.LimitCount), int(p.OffsetCount))
	if err != nil {
		return nil, 0, false
	}
	uuids, rank := searchengine.ParseUUIDs(ids)
	if len(uuids) == 0 {
		return []db.ListOrganizationStockUnitRowsRow{}, total, true
	}
	p.Q = pgtype.Text{}
	p.Uuids = uuids
	p.LimitCount, p.OffsetCount = int32(len(uuids)), 0
	rows, err := s.q.ListOrganizationStockUnitRows(ctx, p)
	if err != nil {
		return nil, 0, false
	}
	return searchengine.Reorder(rows, func(r db.ListOrganizationStockUnitRowsRow) uuid.UUID { return r.Uuid }, rank), total, true
}
