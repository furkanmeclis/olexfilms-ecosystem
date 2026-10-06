package usecase

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

// SearchSpec is the Meilisearch spec id of leads.
const SearchSpec = searchengine.SpecLeads

// IndexStore is what the leads search adapter reads.
type IndexStore interface {
	ListLeadsForIndex(ctx context.Context) ([]db.Lead, error)
	GetLeadForIndex(ctx context.Context, id uuid.UUID) (db.Lead, error)
}

// SearchAdapter indexes lead name/company/phone fields.
type SearchAdapter struct{ q IndexStore }

// NewSearchAdapter creates the lead search adapter.
func NewSearchAdapter(q IndexStore) *SearchAdapter { return &SearchAdapter{q: q} }

// Spec implements searchengine.Adapter.
func (a *SearchAdapter) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         SearchSpec,
		LabelKey:   "search.specs_leads",
		Permission: rbac.PermLeadsRead,
		Icon:       "target",
		Searchable: []string{"title", "subtitle", "keywords"},
		Filterable: []string{"organization_ids", "brand_ids", "status", "created_by_user_id"},
		ListScoped: true,
	}
}

// ListAll implements searchengine.Adapter.
func (a *SearchAdapter) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	rows, err := a.q.ListLeadsForIndex(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]searchengine.Document, 0, len(rows))
	for _, r := range rows {
		out = append(out, leadDocument(r))
	}
	return out, nil
}

// Document implements searchengine.Adapter.
func (a *SearchAdapter) Document(ctx context.Context, id string) (searchengine.Document, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("leads search: invalid uuid")
	}
	row, err := a.q.GetLeadForIndex(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return searchengine.Document{}, searchengine.ErrSkipDocument
	}
	if err != nil {
		return searchengine.Document{}, err
	}
	return leadDocument(row), nil
}

func leadDocument(r db.Lead) searchengine.Document {
	title := strings.TrimSpace(r.CandidateContactName.String)
	if title == "" {
		title = strings.TrimSpace(r.CandidateCompanyName.String)
	}
	if title == "" {
		title = r.CandidatePhoneE164.String
	}
	if title == "" {
		title = r.TargetType
	}
	subtitle := strings.Join(searchengine.Keywords(r.CandidateCompanyName.String, r.CandidatePhoneE164.String), " · ")
	doc := searchengine.Document{
		ID:              r.Uuid.String(),
		Spec:            SearchSpec,
		Title:           title,
		Subtitle:        subtitle,
		Keywords:        searchengine.Keywords(r.CandidateCompanyName.String, r.CandidateContactName.String, r.CandidatePhoneE164.String, r.CandidateEmail.String),
		Href:            "/leads/" + r.Uuid.String(),
		Icon:            "target",
		OrganizationIDs: []int64{r.OrganizationID},
		BrandIDs:        []int64{r.BrandID},
		Status:          r.Status,
	}
	if r.CreatedByUserID.Valid {
		doc.CreatedByUserID = r.CreatedByUserID.Int64
	}
	return doc
}

func leadIndexFilter(c Caller, p db.ListLeadsInScopeParams) (string, bool) {
	if c.Org.BrandID == 0 {
		return "", false
	}
	f := (&searchengine.Filter{}).Eq("brand_ids", c.Org.BrandID)
	if ids := p.OrganizationIds; ids != nil {
		if len(ids) == 0 {
			return "", false
		}
		f.In("organization_ids", ids)
	}
	if len(p.Statuses) == 1 {
		f.EqString("status", p.Statuses[0])
	}
	if len(p.Statuses) > 1 || len(p.TargetTypes) > 0 {
		// several statuses and target_type are not filterable in the
		// index; fall back to SQL.
		return "", false
	}
	if c.Filter.UserOnly() {
		f.Eq("created_by_user_id", c.Principal.UserInternal)
	}
	return f.String(), true
}

func (s *Service) searchIndexed(ctx context.Context, c Caller, p db.ListLeadsInScopeParams) ([]db.Lead, int64, bool) {
	filter, ok := leadIndexFilter(c, p)
	if !ok {
		return nil, 0, false
	}
	ids, total, err := s.finder.SearchIDs(ctx, SearchSpec, p.Q.String, filter, int(p.PageLimit), int(p.PageOffset))
	if err != nil {
		return nil, 0, false
	}
	uuids, rank := searchengine.ParseUUIDs(ids)
	if len(uuids) == 0 {
		return []db.Lead{}, total, true
	}
	p.Q = pgtype.Text{}
	p.Uuids = uuids
	p.PageLimit, p.PageOffset = int32(len(uuids)), 0
	rows, err := s.q.ListLeadsInScope(ctx, p)
	if err != nil {
		return nil, 0, false
	}
	return searchengine.Reorder(rows, func(r db.Lead) uuid.UUID { return r.Uuid }, rank), total, true
}

var _ searchengine.Adapter = (*SearchAdapter)(nil)
