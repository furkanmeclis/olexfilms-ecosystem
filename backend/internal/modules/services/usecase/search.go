package usecase

// TEC-209: Meilisearch services index. A document carries the service
// organization and brand, the status, the creator (own / assigned scopes)
// and the customer / vehicle ids as filterable fields; GET /v1/services?q=
// filters the index on the caller's services.read scope and the domain
// brand (K20) and then loads the hits from Postgres with the same scope,
// so the index is never the only access check. The command palette does
// not search this spec (ListScoped). An anonymized customer's service
// keeps only the service number in the index: no name, phone, plate or
// VIN (K19).

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

// SearchSpec is the Meilisearch spec (index suffix) of services.
const SearchSpec = searchengine.SpecServices

// IndexStore is what the services search adapter reads.
type IndexStore interface {
	ListServicesForIndex(ctx context.Context) ([]db.ListServicesForIndexRow, error)
	GetServiceForIndex(ctx context.Context, id uuid.UUID) (db.GetServiceForIndexRow, error)
}

// SearchAdapter indexes services in Meilisearch.
type SearchAdapter struct{ q IndexStore }

// NewSearchAdapter creates the services search adapter.
func NewSearchAdapter(q IndexStore) *SearchAdapter { return &SearchAdapter{q: q} }

// Spec implements searchengine.Adapter.
func (a *SearchAdapter) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         SearchSpec,
		LabelKey:   "search.specs_services",
		Permission: rbac.PermServicesRead,
		Icon:       "wrench",
		Searchable: []string{"title", "subtitle", "keywords"},
		Filterable: []string{"organization_ids", "brand_ids", "status", "customer_user_id", "created_by_user_id", "vehicle_id"},
		ListScoped: true,
	}
}

// ListAll implements searchengine.Adapter.
func (a *SearchAdapter) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	rows, err := a.q.ListServicesForIndex(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]searchengine.Document, 0, len(rows))
	for _, r := range rows {
		out = append(out, serviceDocument(db.GetServiceForIndexRow(r)))
	}
	return out, nil
}

// Document implements searchengine.Adapter. A missing service is not
// indexable (the stale document is removed).
func (a *SearchAdapter) Document(ctx context.Context, id string) (searchengine.Document, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("services search: invalid uuid")
	}
	row, err := a.q.GetServiceForIndex(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return searchengine.Document{}, searchengine.ErrSkipDocument
	}
	if err != nil {
		return searchengine.Document{}, err
	}
	return serviceDocument(row), nil
}

func serviceDocument(r db.GetServiceForIndexRow) searchengine.Document {
	doc := searchengine.Document{
		ID:              r.Uuid.String(),
		Spec:            SearchSpec,
		Title:           r.ServiceNo,
		Keywords:        searchengine.Keywords(r.ServiceNo),
		Href:            "/services/" + r.Uuid.String(),
		Icon:            "wrench",
		OrganizationIDs: []int64{r.OrganizationID},
		BrandIDs:        []int64{r.BrandID},
		Status:          r.Status,
		CustomerUserID:  r.CustomerUserID,
		VehicleID:       r.VehicleID,
	}
	if r.CreatedByUserID.Valid {
		doc.CreatedByUserID = r.CreatedByUserID.Int64
	}
	if r.CustomerStatus == "anonymized" {
		// K19: the service stays, the person (and the vehicle identity) go.
		return doc
	}
	car := strings.TrimSpace(r.CarBrandName + " " + r.CarModelName)
	doc.Subtitle = strings.Join(searchengine.Keywords(r.Plate.String, car), " · ")
	doc.Keywords = append(doc.Keywords, searchengine.Keywords(
		r.Plate.String, geo.NormalizePlate(r.Plate.String), r.Vin.String,
		strings.TrimSpace(r.CustomerName+" "+r.CustomerSurname), r.CustomerPhone.String, car, r.Package.String,
	)...)
	if p := r.CustomerPhone.String; strings.HasPrefix(p, "+") && len(p) > 4 {
		doc.Keywords = append(doc.Keywords, strings.TrimPrefix(p, "+"))
	}
	return doc
}

// SetFinder enables index search in List (nil: SQL search only).
func (s *Service) SetFinder(f searchengine.ListFinder) { s.finder = f }

func (s *Service) indexEnabled() bool { return s.finder != nil && s.finder.Enabled() }

// indexFilter is the Meilisearch filter of the caller's scope: the domain
// brand (K20), the organizations of an organization relative scope, the
// creator for own / assigned, plus the list filters. ok is false when the
// scope reaches no service.
func indexFilter(c Caller, p db.ListServicesInScopeParams) (string, bool) {
	if c.Org.BrandID == 0 {
		return "", false
	}
	f := (&searchengine.Filter{}).Eq("brand_ids", c.Org.BrandID)
	if ids := p.OrgIds; ids != nil {
		if len(ids) == 0 {
			return "", false
		}
		f.In("organization_ids", ids)
	}
	if p.CreatedByUserID.Valid {
		f.Eq("created_by_user_id", p.CreatedByUserID.Int64)
	}
	if p.CustomerUserID.Valid {
		f.Eq("customer_user_id", p.CustomerUserID.Int64)
	}
	if p.VehicleID.Valid {
		f.Eq("vehicle_id", p.VehicleID.Int64)
	}
	if p.Status.Valid {
		f.EqString("status", p.Status.String)
	}
	return f.String(), true
}

// searchIndexed answers a q search from the index: the hits are reloaded
// through ListServicesInScope with the same scope and filters. handled is
// false when the caller must fall back to the SQL search.
func (s *Service) searchIndexed(ctx context.Context, c Caller, p db.ListServicesInScopeParams) ([]db.Service, int64, bool) {
	filter, ok := indexFilter(c, p)
	if !ok {
		return []db.Service{}, 0, true
	}
	ids, total, err := s.finder.SearchIDs(ctx, SearchSpec, p.Q.String, filter, int(p.RowLimit), int(p.RowOffset))
	if err != nil {
		return nil, 0, false
	}
	uuids, rank := searchengine.ParseUUIDs(ids)
	if len(uuids) == 0 {
		return []db.Service{}, total, true
	}
	p.Q = pgtype.Text{}
	p.Uuids = uuids
	p.RowLimit, p.RowOffset = int32(len(uuids)), 0
	rows, err := s.q.ListServicesInScope(ctx, p)
	if err != nil {
		return nil, 0, false
	}
	return searchengine.Reorder(rows, func(r db.Service) uuid.UUID { return r.Uuid }, rank), total, true
}
