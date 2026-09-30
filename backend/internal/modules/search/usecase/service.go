package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
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
	tenantSlug := s.tenantSlug(ctx, p)
	out := make([]searchengine.Spec, 0, len(s.reg.Specs()))
	for _, spec := range s.reg.Specs() {
		if spec.Permission != "" && !p.HasPermission(spec.Permission) {
			continue
		}
		if spec.TenantScoped && tenantSlug == "" {
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
	tenantSlug := s.tenantSlug(ctx, p)
	filters := make(map[string]string)
	out := make([]string, 0, len(specs))
	for _, spec := range specs {
		if spec.TenantScoped {
			if tenantSlug == "" {
				continue
			}
			filters[spec.ID] = organizationSlugFilter(tenantSlug)
		}
		out = append(out, spec.ID)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, filters
}

func (s *Service) tenantSlug(ctx context.Context, p authctx.Principal) string {
	if s == nil || s.q == nil || p.OrganizationUUID == nil || *p.OrganizationUUID == uuid.Nil {
		return ""
	}
	row, err := s.q.GetOrganizationMemberByUserAndOrgUUID(ctx, db.GetOrganizationMemberByUserAndOrgUUIDParams{
		UserID: p.UserInternal,
		Uuid:   *p.OrganizationUUID,
	})
	if err != nil {
		if err != pgx.ErrNoRows {
			s.log.Warn("search_tenant_scope_failed", "error", err)
		}
		return ""
	}
	if row.OrganizationStatus == "suspended" {
		return ""
	}
	return strings.TrimSpace(row.OrganizationSlug)
}

func organizationSlugFilter(slug string) string {
	slug = strings.ReplaceAll(slug, `\`, `\\`)
	slug = strings.ReplaceAll(slug, `"`, `\"`)
	return fmt.Sprintf(`organization_slug = "%s"`, slug)
}
