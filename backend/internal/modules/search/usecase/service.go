package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
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
}

// New creates a search service.
func New(client *searchengine.Client, reg *searchengine.Registry, q *db.Queries, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{client: client, reg: reg, q: q, log: log}
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
	hits, err := s.client.Search(ctx, specs, q, limit, filters)
	if err != nil {
		s.log.Warn("search_query_failed", "error", err)
		return nil
	}
	return hits
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
	out := tenant{slug: strings.TrimSpace(row.OrganizationSlug)}
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
