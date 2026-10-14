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
	Type string
	// Q searches name and dealer code (slug); the index answers it when
	// it is up (TEC-210).
	Q      string
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
	typ, err := scopedType(in.Type)
	if err != nil {
		return nil, err
	}
	if in.Limit <= 0 || in.Limit > 200 {
		in.Limit = 50
	}
	if in.Offset < 0 {
		in.Offset = 0
	}
	p := db.ListOrganizationsInScopeParams{
		OrgIds:      f.OrgIDsArg(),
		BrandID:     pgtype.Int8{Int64: brand.ID, Valid: true},
		Type:        typ,
		Q:           scopedQuery(in.Q),
		LimitCount:  in.Limit,
		OffsetCount: in.Offset,
	}
	var (
		rows    []db.ListOrganizationsInScopeRow
		indexed bool
	)
	if p.Q.Valid && s.indexEnabled() {
		rows, indexed = s.searchIndexed(ctx, brand.ID, p)
	}
	if !indexed {
		rows, err = s.q.ListOrganizationsInScope(ctx, p)
		if err != nil {
			return nil, err
		}
	}
	out := make([]Organization, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapTree(row.Organization, row.BrandSlug, row.ParentUuid, row.ParentName))
	}
	return out, nil
}

// CountInScope counts what ListInScope matches without paging, minus the
// exclude organization (the AI tools leave the caller's own row out). Q is
// matched by SQL (name / dealer code), not the index (TEC-647).
func (s *Service) CountInScope(ctx context.Context, f scopefilter.Filter, in ScopedListInput, exclude uuid.UUID) (int64, error) {
	brand, err := RequestBrand(ctx)
	if err != nil {
		return 0, err
	}
	typ, err := scopedType(in.Type)
	if err != nil {
		return 0, err
	}
	return s.q.CountOrganizationsInScope(ctx, db.CountOrganizationsInScopeParams{
		OrgIds:      f.OrgIDsArg(),
		BrandID:     pgtype.Int8{Int64: brand.ID, Valid: true},
		Type:        typ,
		Q:           scopedQuery(in.Q),
		ExcludeUuid: pgtype.UUID{Bytes: exclude, Valid: exclude != uuid.Nil},
	})
}

// scopedType validates the optional organization type filter.
func scopedType(t string) (pgtype.Text, error) {
	t = strings.TrimSpace(t)
	if t == "" {
		return pgtype.Text{}, nil
	}
	if t != TypeCenter && t != TypeDistributor && t != TypeDealer {
		return pgtype.Text{}, fmt.Errorf("%w: invalid type", ErrInvalidRequest)
	}
	return pgtype.Text{String: t, Valid: true}, nil
}

// GetInScope returns// GetInScope returns one organization when the filter reaches it. Anything
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
