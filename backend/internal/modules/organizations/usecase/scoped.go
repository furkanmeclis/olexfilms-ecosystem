package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ScopedListInput filters GET /v1/tenant/organizations.
type ScopedListInput struct {
	Type   string
	Limit  int32
	Offset int32
}

// ListInScope lists the organizations an organizations.read filter reaches
// (a distributor owner: itself and its dealers). Results never leave the
// request brand (K1/K20), whatever the scope.
func (s *Service) ListInScope(ctx context.Context, f scopefilter.Filter, in ScopedListInput) ([]Organization, error) {
	brand, err := RequestBrand(ctx)
	if err != nil {
		return nil, err
	}
	typ := pgtype.Text{}
	if t := strings.TrimSpace(in.Type); t != "" {
		if t != TypeCenter && t != TypeDistributor && t != TypeDealer {
			return nil, fmt.Errorf("%w: invalid type", ErrInvalidRequest)
		}
		typ = pgtype.Text{String: t, Valid: true}
	}
	if in.Limit <= 0 || in.Limit > 200 {
		in.Limit = 50
	}
	if in.Offset < 0 {
		in.Offset = 0
	}
	rows, err := s.q.ListOrganizationsInScope(ctx, db.ListOrganizationsInScopeParams{
		OrgIds:      f.OrgIDsArg(),
		BrandID:     pgtype.Int8{Int64: brand.ID, Valid: true},
		Type:        typ,
		LimitCount:  in.Limit,
		OffsetCount: in.Offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Organization, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapTree(row.Organization, row.BrandSlug, row.ParentUuid, row.ParentName))
	}
	return out, nil
}

// GetInScope returns one organization when the filter reaches it. Anything
// outside the scope reads as not found, so other distributors' dealers are
// not even confirmed to exist.
func (s *Service) GetInScope(ctx context.Context, f scopefilter.Filter, id uuid.UUID) (Organization, error) {
	row, err := s.brandOrg(ctx, id)
	if err != nil {
		return Organization{}, err
	}
	if !f.AllowsOrg(row.Organization.ID, row.Organization.BrandID) {
		return Organization{}, ErrNotFound
	}
	return mapTree(row.Organization, row.BrandSlug, row.ParentUuid, row.ParentName), nil
}
