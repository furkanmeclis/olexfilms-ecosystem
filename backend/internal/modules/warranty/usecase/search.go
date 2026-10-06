package usecase

// TEC-209: Meilisearch warranties index. A document carries the warranty
// organization and brand, the status, the holder, the creator of the
// service (own / assigned scopes), the vehicle and the product as
// filterable fields. GET /v1/warranties?q= (and the portal list) filters
// the index on the caller's scope and the domain brand (K20) and then
// reloads the hits from Postgres with the same scope, so the index is never
// the only access check. The public code is always searchable; an
// anonymized holder's warranty keeps only the codes and the product (no
// name, phone, plate or VIN, K19).

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SearchSpec is the Meilisearch spec (index suffix) of warranties.
const SearchSpec = searchengine.SpecWarranties

// IndexStore is what the warranties search adapter reads.
type IndexStore interface {
	ListWarrantiesForIndex(ctx context.Context) ([]db.ListWarrantiesForIndexRow, error)
	GetWarrantyForIndex(ctx context.Context, id uuid.UUID) (db.GetWarrantyForIndexRow, error)
}

// SearchAdapter indexes warranties in Meilisearch.
type SearchAdapter struct{ q IndexStore }

// NewSearchAdapter creates the warranties search adapter.
func NewSearchAdapter(q IndexStore) *SearchAdapter { return &SearchAdapter{q: q} }

// Spec implements searchengine.Adapter.
func (a *SearchAdapter) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         SearchSpec,
		LabelKey:   "search.specs_warranties",
		Permission: rbac.PermWarrantiesRead,
		Icon:       "shield-check",
		Searchable: []string{"title", "subtitle", "keywords"},
		Filterable: []string{"organization_ids", "brand_ids", "status", "customer_user_id", "created_by_user_id", "vehicle_id", "product_id"},
		ListScoped: true,
	}
}

// ListAll implements searchengine.Adapter.
func (a *SearchAdapter) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	rows, err := a.q.ListWarrantiesForIndex(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]searchengine.Document, 0, len(rows))
	for _, r := range rows {
		out = append(out, warrantyDocument(db.GetWarrantyForIndexRow(r)))
	}
	return out, nil
}

// Document implements searchengine.Adapter. A missing warranty is not
// indexable (the stale document is removed).
func (a *SearchAdapter) Document(ctx context.Context, id string) (searchengine.Document, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("warranty search: invalid uuid")
	}
	row, err := a.q.GetWarrantyForIndex(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return searchengine.Document{}, searchengine.ErrSkipDocument
	}
	if err != nil {
		return searchengine.Document{}, err
	}
	return warrantyDocument(row), nil
}

func warrantyDocument(r db.GetWarrantyForIndexRow) searchengine.Document {
	doc := searchengine.Document{
		ID:              r.Uuid.String(),
		Spec:            SearchSpec,
		Title:           r.PublicCode,
		Subtitle:        strings.Join(searchengine.Keywords(r.ProductName, r.ServiceNo), " · "),
		Keywords:        searchengine.Keywords(r.PublicCode, r.ServiceNo, r.ProductSku, r.ProductName),
		Href:            "/warranties/" + r.Uuid.String(),
		Icon:            "shield-check",
		OrganizationIDs: []int64{r.OrganizationID},
		BrandIDs:        []int64{r.BrandID},
		Status:          r.Status,
		CustomerUserID:  r.HolderUserID,
		VehicleID:       r.VehicleID,
		ProductID:       r.ProductID,
	}
	if r.ServiceCreatedBy.Valid {
		doc.CreatedByUserID = r.ServiceCreatedBy.Int64
	}
	if r.HolderStatus == "anonymized" {
		// K19: the warranty stays, the person (and the vehicle identity) go.
		return doc
	}
	doc.Keywords = append(doc.Keywords, searchengine.Keywords(
		r.ServicePlate.String, geo.NormalizePlate(r.ServicePlate.String),
		r.VehiclePlate.String, r.VehiclePlateNormalized.String, r.VehicleVin.String,
		strings.TrimSpace(r.HolderName+" "+r.HolderSurname), r.HolderPhone.String,
	)...)
	if p := r.HolderPhone.String; strings.HasPrefix(p, "+") && len(p) > 4 {
		doc.Keywords = append(doc.Keywords, strings.TrimPrefix(p, "+"))
	}
	return doc
}

// SetFinder enables index search in the lists (nil: SQL search only).
func (r *Reader) SetFinder(f searchengine.ListFinder) { r.finder = f }

func (r *Reader) indexEnabled() bool { return r.finder != nil && r.finder.Enabled() }

// indexFilter is the Meilisearch filter of the scope: the domain brand
// (K20), the organizations of an organization relative scope, the holder
// (portal / customer scope), the service creator (own / assigned), plus
// the list filters. ok is false when the scope reaches no warranty.
func indexFilter(p db.ListWarrantyRowsParams) (string, bool) {
	if p.BrandID == 0 {
		return "", false
	}
	f := (&searchengine.Filter{}).Eq("brand_ids", p.BrandID)
	if p.OrgIds != nil {
		if len(p.OrgIds) == 0 {
			return "", false
		}
		f.In("organization_ids", p.OrgIds)
	}
	if p.HolderUserID.Valid {
		f.Eq("customer_user_id", p.HolderUserID.Int64)
	}
	if p.ServiceCreatedBy.Valid {
		f.Eq("created_by_user_id", p.ServiceCreatedBy.Int64)
	}
	if len(p.Statuses) > 0 {
		f.InStrings("status", p.Statuses)
	}
	if p.ProductID.Valid {
		f.Eq("product_id", p.ProductID.Int64)
	}
	if p.VehicleID.Valid {
		f.Eq("vehicle_id", p.VehicleID.Int64)
	}
	return f.String(), true
}

// searchIndexed answers a q search from the index: the hits are reloaded
// through ListWarrantyRows with the same scope and filters. handled is
// false when the caller must fall back to the SQL search.
func (r *Reader) searchIndexed(ctx context.Context, p db.ListWarrantyRowsParams, q string) ([]db.ListWarrantyRowsRow, int64, bool) {
	filter, ok := indexFilter(p)
	if !ok {
		return []db.ListWarrantyRowsRow{}, 0, true
	}
	ids, total, err := r.finder.SearchIDs(ctx, SearchSpec, q, filter, int(p.RowLimit), int(p.RowOffset))
	if err != nil {
		return nil, 0, false
	}
	uuids, rank := searchengine.ParseUUIDs(ids)
	if len(uuids) == 0 {
		return []db.ListWarrantyRowsRow{}, total, true
	}
	p.Q, p.QPlate = pgtype.Text{}, pgtype.Text{}
	p.Uuids = uuids
	p.RowLimit, p.RowOffset = int32(len(uuids)), 0
	rows, err := r.q.ListWarrantyRows(ctx, p)
	if err != nil {
		return nil, 0, false
	}
	return searchengine.Reorder(rows, func(row db.ListWarrantyRowsRow) uuid.UUID { return row.Uuid }, rank), total, true
}

var _ searchengine.Adapter = (*SearchAdapter)(nil)
