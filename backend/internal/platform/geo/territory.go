package geo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	// ErrTerritoryConflict: the area (or an ancestor/descendant of it) already
	// belongs to a distributor of the brand (K5).
	ErrTerritoryConflict = errors.New("geo: territory conflict")
	// ErrNotDistributor: territories are assigned to distributors only.
	ErrNotDistributor = errors.New("geo: organization is not a distributor")
)

// ConflictError carries the territories that block an assignment.
type ConflictError struct {
	Conflicts []TerritoryRef
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("geo: territory conflict with %d territory(ies)", len(e.Conflicts))
}

// Unwrap lets errors.Is(err, ErrTerritoryConflict) match.
func (e *ConflictError) Unwrap() error { return ErrTerritoryConflict }

// TerritoryRef identifies a blocking territory.
type TerritoryRef struct {
	UUID             uuid.UUID `json:"uuid"`
	Level            string    `json:"level"`
	OrganizationUUID uuid.UUID `json:"organization_uuid"`
	OrganizationName string    `json:"organization_name"`
}

// Territory is the public territory projection.
type Territory struct {
	UUID             uuid.UUID `json:"uuid"`
	Level            string    `json:"level"`
	OrganizationUUID uuid.UUID `json:"organization_uuid"`
	OrganizationName string    `json:"organization_name"`
	CountryID        int64     `json:"country_id"`
	CountryISO2      string    `json:"country_iso2"`
	CountryNameEn    string    `json:"country_name_en"`
	CountryNameTr    string    `json:"country_name_tr"`
	ProvinceID       *int64    `json:"province_id,omitempty"`
	ProvinceName     *string   `json:"province_name,omitempty"`
	DistrictID       *int64    `json:"district_id,omitempty"`
	DistrictName     *string   `json:"district_name,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// AssignInput is one territory assignment.
type AssignInput struct {
	BrandID       int64
	DistributorID int64
	CountryID     int64
	ProvinceID    *int64
	DistrictID    *int64
	ActorID       int64
}

func int8Ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}

func ptrInt8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

// Assign gives an area to a distributor. Inside one transaction it takes an
// advisory lock per (brand, country), so two concurrent assignments of
// overlapping areas cannot both pass the overlap check, then rejects the
// same area and any ancestor/descendant overlap (NL with A blocks Amsterdam
// for B and vice versa) with a *ConflictError.
func (s *Service) Assign(ctx context.Context, in AssignInput) (Territory, error) {
	org, err := s.q.GetOrganizationByID(ctx, in.DistributorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Territory{}, fmt.Errorf("%w: distributor", ErrNotFound)
		}
		return Territory{}, err
	}
	if org.BrandID != in.BrandID {
		return Territory{}, fmt.Errorf("%w: distributor", ErrNotFound)
	}
	if org.Type != "distributor" {
		return Territory{}, ErrNotDistributor
	}
	if _, err := s.ValidateAddress(ctx, in.CountryID, in.ProvinceID, in.DistrictID); err != nil {
		return Territory{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Territory{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)
	if err := qtx.LockTerritoryArea(ctx, db.LockTerritoryAreaParams{BrandID: in.BrandID, CountryID: in.CountryID}); err != nil {
		return Territory{}, err
	}
	overlaps, err := qtx.ListOverlappingTerritories(ctx, db.ListOverlappingTerritoriesParams{
		BrandID: in.BrandID, CountryID: in.CountryID,
		ProvinceID: ptrInt8(in.ProvinceID), DistrictID: ptrInt8(in.DistrictID),
	})
	if err != nil {
		return Territory{}, err
	}
	if len(overlaps) > 0 {
		ce := &ConflictError{}
		for _, o := range overlaps {
			ce.Conflicts = append(ce.Conflicts, TerritoryRef{
				UUID: o.Uuid, Level: o.Level.String,
				OrganizationUUID: o.OrganizationUuid, OrganizationName: o.OrganizationName,
			})
		}
		return Territory{}, ce
	}
	row, err := qtx.CreateTerritory(ctx, db.CreateTerritoryParams{
		BrandID: in.BrandID, OrganizationID: org.ID, CountryID: in.CountryID,
		ProvinceID: ptrInt8(in.ProvinceID), DistrictID: ptrInt8(in.DistrictID),
		CreatedByUserID: pgtype.Int8{Int64: in.ActorID, Valid: in.ActorID != 0},
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Territory{}, &ConflictError{}
		}
		return Territory{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Territory{}, err
	}
	return s.territoryByUUID(ctx, in.BrandID, row.Uuid)
}

func (s *Service) territoryByUUID(ctx context.Context, brandID int64, id uuid.UUID) (Territory, error) {
	items, err := s.Territories(ctx, brandID, nil)
	if err != nil {
		return Territory{}, err
	}
	for _, t := range items {
		if t.UUID == id {
			return t, nil
		}
	}
	return Territory{}, fmt.Errorf("%w: territory", ErrNotFound)
}

// Territories lists the brand's territories, optionally of one distributor.
func (s *Service) Territories(ctx context.Context, brandID int64, distributorID *int64) ([]Territory, error) {
	rows, err := s.q.ListTerritories(ctx, db.ListTerritoriesParams{BrandID: brandID, OrganizationID: ptrInt8(distributorID)})
	if err != nil {
		return nil, err
	}
	out := make([]Territory, 0, len(rows))
	for _, r := range rows {
		out = append(out, Territory{
			UUID: r.Uuid, Level: r.Level.String,
			OrganizationUUID: r.OrganizationUuid, OrganizationName: r.OrganizationName,
			CountryID: r.CountryID, CountryISO2: r.CountryIso2, CountryNameEn: r.CountryNameEn, CountryNameTr: r.CountryNameTr,
			ProvinceID: int8Ptr(r.ProvinceID), ProvinceName: textPtr(r.ProvinceName),
			DistrictID: int8Ptr(r.DistrictID), DistrictName: textPtr(r.DistrictName),
			CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

// DeleteTerritory removes a territory of the brand. Existing dealers keep
// their parent; only new dealers resolve differently.
func (s *Service) DeleteTerritory(ctx context.Context, brandID int64, id uuid.UUID) error {
	n, err := s.q.DeleteTerritory(ctx, db.DeleteTerritoryParams{Uuid: id, BrandID: brandID})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: territory", ErrNotFound)
	}
	return nil
}

// Match is the distributor that covers an address.
type Match struct {
	TerritoryUUID    uuid.UUID `json:"territory_uuid"`
	Level            string    `json:"level"`
	OrganizationID   int64     `json:"-"`
	OrganizationUUID uuid.UUID `json:"organization_uuid"`
	OrganizationName string    `json:"organization_name"`
}

// ResolveDistributor returns the most specific territory (district >
// province > country) covering the address, or nil when the area belongs to
// no distributor (the dealer stays under the center).
func (s *Service) ResolveDistributor(ctx context.Context, brandID, countryID int64, provinceID, districtID *int64) (*Match, error) {
	row, err := s.q.ResolveTerritory(ctx, db.ResolveTerritoryParams{
		BrandID: brandID, CountryID: countryID, ProvinceID: ptrInt8(provinceID), DistrictID: ptrInt8(districtID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &Match{
		TerritoryUUID: row.Uuid, Level: row.Level.String, OrganizationID: row.OrganizationID,
		OrganizationUUID: row.OrganizationUuid, OrganizationName: row.OrganizationName,
	}, nil
}
