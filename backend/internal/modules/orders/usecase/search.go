package usecase

// TEC-210: Meilisearch orders index. A document carries the order number,
// the parties (seller and buyer name + dealer code), the tracking number
// and the external reference; the seller / buyer organization ids, the
// brand and the status are filterable. GET /v1/orders?q= filters the index
// on the caller's orders.read scope (seller or buyer inside the scope, the
// domain brand K20, side=seller|buyer) and then loads the hits from
// Postgres with the same scope, so the index is never the only access
// check. The command palette does not search this spec (ListScoped).

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

// SearchSpec is the Meilisearch spec (index suffix) of orders.
const SearchSpec = searchengine.SpecOrders

// IndexStore is what the orders search adapter reads.
type IndexStore interface {
	ListOrdersForIndex(ctx context.Context) ([]db.ListOrdersForIndexRow, error)
	GetOrderForIndex(ctx context.Context, id uuid.UUID) (db.GetOrderForIndexRow, error)
}

// SearchAdapter indexes orders in Meilisearch.
type SearchAdapter struct{ q IndexStore }

// NewSearchAdapter creates the orders search adapter.
func NewSearchAdapter(q IndexStore) *SearchAdapter { return &SearchAdapter{q: q} }

// Spec implements searchengine.Adapter.
func (a *SearchAdapter) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         SearchSpec,
		LabelKey:   "search.specs_orders",
		Permission: rbac.PermOrdersRead,
		Icon:       "shopping-cart",
		Searchable: []string{"title", "subtitle", "keywords"},
		Filterable: []string{"organization_ids", "brand_ids", "status", "seller_org_id", "buyer_org_id"},
		ListScoped: true,
	}
}

// ListAll implements searchengine.Adapter.
func (a *SearchAdapter) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	rows, err := a.q.ListOrdersForIndex(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]searchengine.Document, 0, len(rows))
	for _, r := range rows {
		out = append(out, orderDocument(db.GetOrderForIndexRow(r)))
	}
	return out, nil
}

// Document implements searchengine.Adapter. A missing order is not
// indexable (the stale document is removed).
func (a *SearchAdapter) Document(ctx context.Context, id string) (searchengine.Document, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("orders search: invalid uuid")
	}
	row, err := a.q.GetOrderForIndex(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return searchengine.Document{}, searchengine.ErrSkipDocument
	}
	if err != nil {
		return searchengine.Document{}, err
	}
	return orderDocument(row), nil
}

func orderDocument(r db.GetOrderForIndexRow) searchengine.Document {
	return searchengine.Document{
		ID:       r.Uuid.String(),
		Spec:     SearchSpec,
		Title:    r.OrderNo,
		Subtitle: r.SellerName + " → " + r.BuyerName,
		Keywords: searchengine.Keywords(r.OrderNo, r.SellerName, r.SellerSlug, r.BuyerName, r.BuyerSlug,
			r.TrackingNo.String, r.ExternalReference.String),
		Href:            "/orders/" + r.Uuid.String(),
		Icon:            "shopping-cart",
		OrganizationIDs: []int64{r.OrganizationID, r.BuyerOrgID},
		BrandIDs:        []int64{r.BrandID},
		Status:          r.Status,
		SellerOrgID:     r.OrganizationID,
		BuyerOrgID:      r.BuyerOrgID,
	}
}

// SetFinder enables index search in List (nil: SQL search only).
func (s *Service) SetFinder(f searchengine.ListFinder) { s.finder = f }

func (s *Service) indexEnabled() bool { return s.finder != nil && s.finder.Enabled() }

// orderScope is the resolved list scope shared by the index filter and the
// Postgres reload.
type orderScope struct {
	side   string
	brand  int64
	orgID  int64   // the active organization (side seller / buyer)
	orgIDs []int64 // side all: nil = whole brand
	status pgtype.Text
}

// indexFilter is the Meilisearch filter of the scope. ok is false when the
// scope reaches no order.
func indexFilter(sc orderScope) (string, bool) {
	if sc.brand == 0 {
		return "", false
	}
	f := (&searchengine.Filter{}).Eq("brand_ids", sc.brand)
	switch sc.side {
	case SideSeller:
		f.Eq("seller_org_id", sc.orgID)
	case SideBuyer:
		f.Eq("buyer_org_id", sc.orgID)
	default:
		if sc.orgIDs != nil {
			if len(sc.orgIDs) == 0 {
				return "", false
			}
			f.In("organization_ids", sc.orgIDs)
		}
	}
	if sc.status.Valid {
		f.EqString("status", sc.status.String)
	}
	return f.String(), true
}

// searchIndexed answers a q search from the index; the hits are reloaded
// through the side's list query with the same scope. handled is false when
// the caller must fall back to the SQL search.
func (s *Service) searchIndexed(ctx context.Context, sc orderScope, q string, limit, offset int32) ([]db.Order, int64, bool) {
	filter, ok := indexFilter(sc)
	if !ok {
		return []db.Order{}, 0, true
	}
	ids, total, err := s.finder.SearchIDs(ctx, SearchSpec, q, filter, int(limit), int(offset))
	if err != nil {
		return nil, 0, false
	}
	uuids, rank := searchengine.ParseUUIDs(ids)
	if len(uuids) == 0 {
		return []db.Order{}, total, true
	}
	n := int32(len(uuids))
	var rows []db.Order
	switch sc.side {
	case SideSeller:
		rows, err = s.q.ListOrdersBySeller(ctx, db.ListOrdersBySellerParams{
			BrandID: sc.brand, SellerOrgID: sc.orgID, Status: sc.status, Uuids: uuids, RowLimit: n,
		})
	case SideBuyer:
		rows, err = s.q.ListOrdersByBuyer(ctx, db.ListOrdersByBuyerParams{
			BrandID: sc.brand, BuyerOrgID: sc.orgID, Status: sc.status, Uuids: uuids, RowLimit: n,
		})
	default:
		rows, err = s.q.ListOrdersInScope(ctx, db.ListOrdersInScopeParams{
			BrandID: sc.brand, OrgIds: sc.orgIDs, Status: sc.status, Uuids: uuids, RowLimit: n,
		})
	}
	if err != nil {
		return nil, 0, false
	}
	return searchengine.Reorder(rows, func(r db.Order) uuid.UUID { return r.Uuid }, rank), total, true
}
