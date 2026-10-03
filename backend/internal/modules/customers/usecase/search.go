package usecase

// TEC-164: Meilisearch customers index. A document carries the
// organizations (customer_organizations) and brands a customer is linked
// to; GET /v1/customers?q= filters it on the caller's permission scope
// (organization_ids IN [...]) and the domain brand (brand_ids = X, K20) and
// then loads the hits from Postgres with the same scope filter, so the index
// is never the only access check. The command palette does not search this
// spec (ListScoped). Anonymized, merged and deleted customers never enter
// the index: Document answers searchengine.ErrSkipDocument and the indexer
// deletes the stale document.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SearchSpec is the Meilisearch spec (index suffix) of customers.
const SearchSpec = "customers"

// IndexStore is what the customers search adapter reads.
type IndexStore interface {
	ListCustomersForIndex(ctx context.Context) ([]db.ListCustomersForIndexRow, error)
	GetCustomerForIndex(ctx context.Context, id uuid.UUID) (db.GetCustomerForIndexRow, error)
}

// SearchAdapter indexes customers in Meilisearch.
type SearchAdapter struct{ q IndexStore }

// NewSearchAdapter creates the customers search adapter.
func NewSearchAdapter(q IndexStore) *SearchAdapter { return &SearchAdapter{q: q} }

// Spec implements searchengine.Adapter.
func (a *SearchAdapter) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         SearchSpec,
		LabelKey:   "search.specs_customers",
		Permission: rbac.PermCustomersRead,
		Icon:       "users",
		Searchable: []string{"title", "subtitle", "keywords"},
		Filterable: []string{"organization_ids", "brand_ids", "status"},
		ListScoped: true,
	}
}

// ListAll implements searchengine.Adapter.
func (a *SearchAdapter) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	rows, err := a.q.ListCustomersForIndex(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]searchengine.Document, 0, len(rows))
	for _, r := range rows {
		out = append(out, customerDocument(db.GetCustomerForIndexRow(r)))
	}
	return out, nil
}

// Document implements searchengine.Adapter. A customer that is anonymized,
// merged, deleted or linked to no organization is not indexable.
func (a *SearchAdapter) Document(ctx context.Context, id string) (searchengine.Document, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("customers search: invalid uuid")
	}
	row, err := a.q.GetCustomerForIndex(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return searchengine.Document{}, searchengine.ErrSkipDocument
	}
	if err != nil {
		return searchengine.Document{}, err
	}
	return customerDocument(row), nil
}

func customerDocument(r db.GetCustomerForIndexRow) searchengine.Document {
	title := strings.TrimSpace(r.Name + " " + r.Surname)
	keywords := []string{}
	for _, v := range []string{r.PhoneE164.String, r.Email.String, r.CompanyName.String} {
		if v = strings.TrimSpace(v); v != "" {
			keywords = append(keywords, v)
		}
	}
	// The national number without the country code ("5551234567") is what
	// staff usually type.
	if p := r.PhoneE164.String; strings.HasPrefix(p, "+") && len(p) > 4 {
		keywords = append(keywords, strings.TrimPrefix(p, "+"))
	}
	subtitle := firstNonEmpty(r.PhoneE164.String, r.Email.String)
	if company := strings.TrimSpace(r.CompanyName.String); company != "" {
		if subtitle != "" {
			subtitle += " · "
		}
		subtitle += company
	}
	return searchengine.Document{
		ID:              r.Uuid.String(),
		Spec:            SearchSpec,
		Title:           title,
		Subtitle:        subtitle,
		Keywords:        keywords,
		Href:            "/customers/" + r.Uuid.String(),
		Icon:            "users",
		OrganizationIDs: r.OrganizationIds,
		BrandIDs:        r.BrandIds,
		Status:          r.Status,
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// CustomerFinder runs a filtered index query (*searchengine.Client).
type CustomerFinder interface {
	Enabled() bool
	SearchIDs(ctx context.Context, spec, q, filter string, limit, offset int) ([]string, int64, error)
}

// SetFinder enables index search in ListCustomers (nil: SQL search only).
func (s *Service) SetFinder(f CustomerFinder) { s.finder = f }

// indexFilter is the Meilisearch filter of the caller's scope: always the
// domain brand (K20), plus the organizations of an organization relative
// scope. ok is false when the scope reaches no customer at all.
func indexFilter(c Caller, status string) (string, bool) {
	if c.Org.BrandID == 0 {
		return "", false
	}
	parts := []string{"brand_ids = " + strconv.FormatInt(c.Org.BrandID, 10)}
	if ids := c.orgIDs(); ids != nil {
		if len(ids) == 0 {
			return "", false
		}
		strs := make([]string, len(ids))
		for i, id := range ids {
			strs[i] = strconv.FormatInt(id, 10)
		}
		parts = append(parts, "organization_ids IN ["+strings.Join(strs, ", ")+"]")
	}
	if status != "" {
		parts = append(parts, "status = "+strconv.Quote(status))
	}
	return strings.Join(parts, " AND "), true
}

// indexEnabled reports whether the list can search the index.
func (s *Service) indexEnabled() bool {
	return s.finder != nil && s.finder.Enabled()
}

// searchIndexed answers a q search from the index. handled is false when
// the caller must fall back to the SQL search (index unavailable).
func (s *Service) searchIndexed(ctx context.Context, c Caller, status, q string, limit, offset int32) ([]db.ListOrganizationCustomersRow, int64, bool) {
	filter, ok := indexFilter(c, status)
	if !ok {
		return []db.ListOrganizationCustomersRow{}, 0, true
	}
	ids, total, err := s.finder.SearchIDs(ctx, SearchSpec, q, filter, int(limit), int(offset))
	if err != nil {
		return nil, 0, false
	}
	uuids := make([]uuid.UUID, 0, len(ids))
	rank := make(map[uuid.UUID]int, len(ids))
	for _, id := range ids {
		u, err := uuid.Parse(id)
		if err != nil {
			continue
		}
		rank[u] = len(uuids)
		uuids = append(uuids, u)
	}
	if len(uuids) == 0 {
		return []db.ListOrganizationCustomersRow{}, total, true
	}
	rows, err := s.q.ListOrganizationCustomers(ctx, db.ListOrganizationCustomersParams{
		OrgIds: c.orgIDs(), BrandID: c.brand(), Status: text(status), Uuids: uuids,
		LimitCount: int32(len(uuids)), OffsetCount: 0,
	})
	if err != nil {
		return nil, 0, false
	}
	ordered := make([]db.ListOrganizationCustomersRow, len(uuids))
	present := make([]bool, len(uuids))
	for _, r := range rows {
		if i, ok := rank[r.Uuid]; ok {
			ordered[i], present[i] = r, true
		}
	}
	out := make([]db.ListOrganizationCustomersRow, 0, len(rows))
	for i := range ordered {
		if present[i] {
			out = append(out, ordered[i])
		}
	}
	return out, total, true
}

// IndexCustomer refreshes one customer document (fail-soft, async).
func (s *Service) indexCustomer(ctx context.Context, id uuid.UUID) {
	if s.search != nil && id != uuid.Nil {
		s.search.EnqueueUpsert(ctx, SearchSpec, id.String())
		// TEC-209: the customer's name, phone and organization links are
		// part of its services, vehicles and warranties documents.
		s.indexCustomerRecords(ctx, id)
	}
}

var _ searchengine.Adapter = (*SearchAdapter)(nil)
