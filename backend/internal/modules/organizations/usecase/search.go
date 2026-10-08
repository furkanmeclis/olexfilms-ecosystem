package usecase

// TEC-210: Meilisearch organizations index. A document carries the name,
// the dealer code (slug, /bayi/{kod}), the type, the brand and the
// organization id; GET /v1/tenant/organizations?q= filters the index on
// the caller's organizations.read reach (center: whole brand, distributor:
// its subtree, dealer: itself) and the request brand (K1/K20), then loads
// the hits from Postgres with the same scope, so the index is never the
// only access check. The command palette does not search this spec
// (ListScoped). Writes publish organization.created / organization.updated
// to the outbox; the search sync refreshes the document from them.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SearchSpec is the Meilisearch spec (index suffix) of organizations.
const SearchSpec = searchengine.SpecOrganizations

// IndexStore is what the organizations search adapter reads.
type IndexStore interface {
	ListOrganizationsForIndex(ctx context.Context) ([]db.ListOrganizationsForIndexRow, error)
	GetOrganizationForIndex(ctx context.Context, id uuid.UUID) (db.GetOrganizationForIndexRow, error)
}

// SearchAdapter indexes organizations in Meilisearch.
type SearchAdapter struct{ q IndexStore }

// NewSearchAdapter creates the organizations search adapter.
func NewSearchAdapter(q IndexStore) *SearchAdapter { return &SearchAdapter{q: q} }

// Spec implements searchengine.Adapter.
func (a *SearchAdapter) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         SearchSpec,
		LabelKey:   "search.specs_organizations",
		Permission: rbac.PermOrganizationsRead,
		Icon:       "building",
		Searchable: []string{"title", "subtitle", "keywords"},
		Filterable: []string{"organization_ids", "brand_ids", "status", "org_type", "has_showcase"},
		ListScoped: true,
	}
}

// ListAll implements searchengine.Adapter.
func (a *SearchAdapter) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	rows, err := a.q.ListOrganizationsForIndex(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]searchengine.Document, 0, len(rows))
	for _, r := range rows {
		out = append(out, organizationDocument(db.GetOrganizationForIndexRow(r)))
	}
	return out, nil
}

// Document implements searchengine.Adapter. A missing or deleted
// organization is not indexable (the stale document is removed).
func (a *SearchAdapter) Document(ctx context.Context, id string) (searchengine.Document, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("organizations search: invalid uuid")
	}
	row, err := a.q.GetOrganizationForIndex(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return searchengine.Document{}, searchengine.ErrSkipDocument
	}
	if err != nil {
		return searchengine.Document{}, err
	}
	return organizationDocument(row), nil
}

func organizationDocument(r db.GetOrganizationForIndexRow) searchengine.Document {
	hasShowcase := r.HasShowcase
	doc := searchengine.Document{
		ID:              r.Uuid.String(),
		Spec:            SearchSpec,
		Title:           r.Name,
		Subtitle:        strings.Join(searchengine.Keywords(r.Slug, r.City, r.ParentName.String), " · "),
		Keywords:        searchengine.Keywords(r.Name, r.Slug, r.City, r.District, r.Phone),
		Href:            "/organizations/" + r.Uuid.String(),
		Icon:            "building",
		OrganizationIDs: []int64{r.ID},
		BrandIDs:        []int64{r.BrandID},
		Status:          r.Status,
		OrgType:         r.Type,
		HasShowcase:     &hasShowcase,
	}
	if p := r.Phone; strings.HasPrefix(p, "+") && len(p) > 4 {
		doc.Keywords = append(doc.Keywords, strings.TrimPrefix(p, "+"))
	}
	// TEC-473: a fleet is reached through its dealer links: its
	// organization_ids are the dealers with an active link, so a dealer's
	// search finds only its own fleets.
	if r.Type == "fleet" {
		doc.Href = "/fleets/" + r.Uuid.String()
		doc.Icon = "truck"
		doc.OrganizationIDs = r.LinkedOrgIds
		if doc.OrganizationIDs == nil {
			doc.OrganizationIDs = []int64{}
		}
	}
	return doc
}

// SetFinder enables index search in ListInScope (nil: SQL search only).
func (s *Service) SetFinder(f searchengine.ListFinder) { s.finder = f }

// SetOutbox makes organization writes publish organization.* events
// (TEC-210; nil: no events).
func (s *Service) SetOutbox(out outbox.Enqueuer) { s.out = out }

func (s *Service) indexEnabled() bool { return s.finder != nil && s.finder.Enabled() }

// indexFilter is the Meilisearch filter of the scoped list: the request
// brand, the organizations of an id-set scope and the type filter. ok is
// false when the scope reaches no organization.
func indexFilter(brandID int64, p db.ListOrganizationsInScopeParams) (string, bool) {
	if brandID == 0 {
		return "", false
	}
	f := (&searchengine.Filter{}).Eq("brand_ids", brandID)
	if ids := p.OrgIds; ids != nil {
		if len(ids) == 0 {
			return "", false
		}
		f.In("organization_ids", ids)
	}
	if p.Type.Valid {
		f.EqString("org_type", p.Type.String)
	} else {
		// TEC-473: fleets share the index but live outside the tree.
		f.InStrings("org_type", []string{"center", "distributor", "dealer"})
	}
	return f.String(), true
}

// searchIndexed answers a q search from the index; the hits are reloaded
// through ListOrganizationsInScope with the same scope. handled is false
// when the caller must fall back to the SQL search.
func (s *Service) searchIndexed(ctx context.Context, brandID int64, p db.ListOrganizationsInScopeParams) ([]db.ListOrganizationsInScopeRow, bool) {
	filter, ok := indexFilter(brandID, p)
	if !ok {
		return []db.ListOrganizationsInScopeRow{}, true
	}
	ids, _, err := s.finder.SearchIDs(ctx, SearchSpec, p.Q.String, filter, int(p.LimitCount), int(p.OffsetCount))
	if err != nil {
		return nil, false
	}
	uuids, rank := searchengine.ParseUUIDs(ids)
	if len(uuids) == 0 {
		return []db.ListOrganizationsInScopeRow{}, true
	}
	p.Q = pgtype.Text{}
	p.Uuids = uuids
	p.LimitCount, p.OffsetCount = int32(len(uuids)), 0
	rows, err := s.q.ListOrganizationsInScope(ctx, p)
	if err != nil {
		return nil, false
	}
	return searchengine.Reorder(rows, func(r db.ListOrganizationsInScopeRow) uuid.UUID { return r.Organization.Uuid }, rank), true
}

// emit writes an organization.* event in tx (no-op without an outbox).
func (s *Service) emit(ctx context.Context, tx pgx.Tx, name string, o db.Organization) error {
	if s.out == nil || tx == nil {
		return nil
	}
	id, uid := o.ID, o.Uuid
	payload := map[string]any{
		"organization_uuid": o.Uuid.String(),
		"organization_id":   o.ID,
		"brand_id":          o.BrandID,
		"type":              o.Type,
		"slug":              o.Slug,
	}
	if o.ParentID.Valid {
		payload["parent_id"] = o.ParentID.Int64
	}
	ev := events.New(name).WithTenant(o.ID).WithEntity("organization", &id, &uid).WithPayload(payload)
	if err := s.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("organizations: outbox: %w", err)
	}
	return nil
}

// updateWithEvent runs an organization update and its organization.updated
// event in one transaction. Without an outbox (or pool) it is the plain
// update.
func (s *Service) updateWithEvent(ctx context.Context, update func(q *db.Queries) (db.Organization, error)) (db.Organization, error) {
	if s.out == nil || s.pool == nil {
		return update(s.q)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Organization{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	o, err := update(s.q.WithTx(tx))
	if err != nil {
		return db.Organization{}, err
	}
	if err := s.emit(ctx, tx, events.OrganizationUpdated, o); err != nil {
		return db.Organization{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Organization{}, err
	}
	return o, nil
}

// scopedQuery trims the q of the scoped list.
func scopedQuery(q string) pgtype.Text {
	if q = strings.TrimSpace(q); q != "" {
		return pgtype.Text{String: q, Valid: true}
	}
	return pgtype.Text{}
}
