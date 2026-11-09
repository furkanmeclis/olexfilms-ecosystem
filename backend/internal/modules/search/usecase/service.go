package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Service coordinates search queries and spec listing.
type Service struct {
	client *searchengine.Client
	reg    *searchengine.Registry
	q      *db.Queries
	log    *slog.Logger
	// TEC-213 global search (SetGroups).
	finder   searchengine.ListFinder
	tree     scopefilter.TreeReader
	features FeatureChecker
	groups   []Group
	// TEC-1061: products closed to dealer orders (nil = no check).
	closed ClosedProductLister
}

// ClosedProductLister returns the given products that are closed to dealer
// orders (*db.Queries).
type ClosedProductLister interface {
	ListDealerClosedProductUUIDs(ctx context.Context, uuids []uuid.UUID) ([]uuid.UUID, error)
}

// New creates a search service.
func New(client *searchengine.Client, reg *searchengine.Registry, q *db.Queries, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	s := &Service{client: client, reg: reg, q: q, log: log}
	if q != nil {
		s.closed = q
	}
	return s
}

// Enabled reports whether remote search is available.
func (s *Service) Enabled() bool {
	return s != nil && s.client != nil && s.client.Enabled()
}

// ListSpecs returns specs visible to the principal.
func (s *Service) ListSpecs(ctx context.Context) []searchengine.Spec {
	if s == nil || s.reg == nil {
		return nil
	}
	p, ok := authctx.PrincipalFrom(ctx)
	if !ok {
		return nil
	}
	tenant := s.tenantScope(ctx, p)
	out := make([]searchengine.Spec, 0, len(s.reg.Specs()))
	for _, spec := range s.reg.Specs() {
		if spec.ListScoped {
			// TEC-164: only the module list applies its scope; the global
			// search (TEC-213) reaches these specs through their list.
			continue
		}
		if spec.Permission != "" && !p.HasPermission(spec.Permission) {
			continue
		}
		if spec.TenantScoped && tenant.slug == "" {
			continue
		}
		if spec.BrandScoped && tenant.brandID == 0 {
			continue
		}
		out = append(out, spec)
	}
	return out
}

// Search queries indexed records for allowed specs.
func (s *Service) Search(ctx context.Context, q, spec string, limit int) []searchengine.Hit {
	if !s.Enabled() {
		return nil
	}
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	specs, filters := s.allowedSpecs(ctx, spec)
	if len(specs) == 0 {
		return nil
	}
	hits, err := s.client.Search(ctx, specs, q, fetchLimit(specs, limit), filters)
	if err != nil {
		s.log.Warn("search_query_failed", "error", err)
		return nil
	}
	return s.VisibleHits(ctx, hits, limit)
}

// fetchLimit asks the index for more hits when product hits may be
// dropped afterwards (VisibleHits), so the caller still gets up to limit.
func fetchLimit(specs []string, limit int) int {
	if !slices.Contains(specs, searchengine.SpecProducts) {
		return limit
	}
	if limit <= 0 {
		limit = 20
	}
	return limit * 2
}

// VisibleHits keeps at most limit hits and, outside the brand center,
// drops the products closed to dealer orders (TEC-1061, as the catalog
// list does). Search and Global run every product hit through it.
func (s *Service) VisibleHits(ctx context.Context, hits []searchengine.Hit, limit int) []searchengine.Hit {
	if !slices.ContainsFunc(hits, func(h searchengine.Hit) bool { return h.Spec == searchengine.SpecProducts }) {
		return trimHits(hits, limit)
	}
	if p, ok := authctx.PrincipalFrom(ctx); ok && s.tenantScope(ctx, p).center {
		return trimHits(hits, limit)
	}
	return s.dropClosedProducts(ctx, hits, limit)
}

func trimHits(hits []searchengine.Hit, limit int) []searchengine.Hit {
	if limit > 0 && len(hits) > limit {
		return hits[:limit]
	}
	return hits
}

// dropClosedProducts removes the product hits closed to dealer orders and
// keeps at most limit hits. The check reads Postgres, so a product closed
// by a bulk action before its index document is refreshed never shows; on
// a lookup error every product hit is dropped (fail closed).
func (s *Service) dropClosedProducts(ctx context.Context, hits []searchengine.Hit, limit int) []searchengine.Hit {
	var ids []uuid.UUID
	for _, h := range hits {
		if h.Spec != searchengine.SpecProducts {
			continue
		}
		if id, err := uuid.Parse(h.ID); err == nil {
			ids = append(ids, id)
		}
	}
	closed := map[string]bool{}
	failed := false
	if len(ids) > 0 {
		if s.closed == nil {
			failed = true
		} else if rows, err := s.closed.ListDealerClosedProductUUIDs(ctx, ids); err != nil {
			s.log.Warn("search_closed_products_failed", "error", err)
			failed = true
		} else {
			for _, id := range rows {
				closed[id.String()] = true
			}
		}
	}
	out := make([]searchengine.Hit, 0, len(hits))
	for _, h := range hits {
		if limit > 0 && len(out) >= limit {
			break
		}
		if h.Spec == searchengine.SpecProducts {
			id, err := uuid.Parse(h.ID)
			if failed || err != nil || closed[id.String()] {
				continue
			}
		}
		out = append(out, h)
	}
	return out
}

func (s *Service) allowedSpecs(ctx context.Context, requested string) ([]string, map[string]string) {
	requested = strings.TrimSpace(requested)
	all := s.ListSpecs(ctx)
	if requested != "" {
		for _, spec := range all {
			if spec.ID == requested {
				return s.specsWithFilters(ctx, []searchengine.Spec{spec})
			}
		}
		return nil, nil
	}
	return s.specsWithFilters(ctx, all)
}

func (s *Service) specsWithFilters(ctx context.Context, specs []searchengine.Spec) ([]string, map[string]string) {
	p, ok := authctx.PrincipalFrom(ctx)
	if !ok {
		return nil, nil
	}
	tenant := s.tenantScope(ctx, p)
	filters := make(map[string]string)
	out := make([]string, 0, len(specs))
	for _, spec := range specs {
		var parts []string
		if spec.TenantScoped {
			if tenant.slug == "" {
				continue
			}
			parts = append(parts, organizationSlugFilter(tenant.slug))
		}
		if spec.BrandScoped {
			if tenant.brandID == 0 {
				continue
			}
			parts = append(parts, brandFilter(tenant.brandID))
		}
		if len(parts) > 0 {
			filters[spec.ID] = strings.Join(parts, " AND ")
		}
		out = append(out, spec.ID)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, filters
}

// tenant is the active organization as search sees it.
type tenant struct {
	slug    string
	brandID int64
	// center: the active organization is the brand center (TEC-1061).
	center bool
}

// tenantScope resolves the active organization. brandID stays 0 when the
// organization does not belong to the request domain's brand (K1/K20), so
// brand scoped specs are never searched across brands.
func (s *Service) tenantScope(ctx context.Context, p authctx.Principal) tenant {
	if s == nil || s.q == nil || p.OrganizationUUID == nil || *p.OrganizationUUID == uuid.Nil {
		return tenant{}
	}
	row, err := s.q.GetOrganizationMemberByUserAndOrgUUID(ctx, db.GetOrganizationMemberByUserAndOrgUUIDParams{
		UserID: p.UserInternal,
		Uuid:   *p.OrganizationUUID,
	})
	if err != nil {
		if err != pgx.ErrNoRows {
			s.log.Warn("search_tenant_scope_failed", "error", err)
		}
		return tenant{}
	}
	if row.OrganizationStatus == "suspended" {
		return tenant{}
	}
	out := tenant{slug: strings.TrimSpace(row.OrganizationSlug), center: row.OrganizationType == "center"}
	if b, ok := brandctx.From(ctx); ok && b.ID == row.OrganizationBrandID {
		out.brandID = row.OrganizationBrandID
	}
	return out
}

// brandFilter is the Meilisearch filter of brand scoped specs.
func brandFilter(brandID int64) string {
	return fmt.Sprintf("brand_id = %d", brandID)
}

func organizationSlugFilter(slug string) string {
	slug = strings.ReplaceAll(slug, `\`, `\\`)
	slug = strings.ReplaceAll(slug, `"`, `\"`)
	return fmt.Sprintf(`organization_slug = "%s"`, slug)
}
