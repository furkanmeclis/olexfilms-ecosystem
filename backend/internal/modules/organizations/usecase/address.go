package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/jackc/pgx/v5/pgtype"
)

// GeoResolver validates addresses and finds the distributor of an area
// (TEC-84, K5). *geo.Service implements it.
type GeoResolver interface {
	ValidateAddress(ctx context.Context, countryID int64, provinceID, districtID *int64) (geo.Address, error)
	ResolveDistributor(ctx context.Context, brandID, countryID int64, provinceID, districtID *int64) (*geo.Match, error)
}

// SetGeo enables structured addresses and territory placement.
func (s *Service) SetGeo(g GeoResolver) { s.geo = g }

// AddressInput is an optional country > province > district chain.
type AddressInput struct {
	CountryID  *int64
	ProvinceID *int64
	DistrictID *int64
}

func (a AddressInput) empty() bool {
	return a.CountryID == nil && a.ProvinceID == nil && a.DistrictID == nil
}

// address validates the chain; an empty input is no address (nil).
func (s *Service) address(ctx context.Context, in AddressInput) (*geo.Address, error) {
	if in.empty() {
		return nil, nil
	}
	if in.CountryID == nil {
		return nil, fmt.Errorf("%w: country_id is required with province_id/district_id", ErrInvalidRequest)
	}
	if s.geo == nil {
		return nil, fmt.Errorf("%w: structured addresses are not available", ErrInvalidRequest)
	}
	a, err := s.geo.ValidateAddress(ctx, *in.CountryID, in.ProvinceID, in.DistrictID)
	if err != nil {
		if errors.Is(err, geo.ErrInvalid) {
			return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
		}
		return nil, err
	}
	return &a, nil
}

// territoryParent returns the distributor whose territory covers the address
// (district > province > country), or nil when no distributor covers it.
func (s *Service) territoryParent(ctx context.Context, brandID int64, a *geo.Address) (*db.Organization, error) {
	if a == nil || s.geo == nil {
		return nil, nil
	}
	var province, district *int64
	if a.Province != nil {
		province = &a.Province.ID
	}
	if a.District != nil {
		district = &a.District.ID
	}
	m, err := s.geo.ResolveDistributor(ctx, brandID, a.Country.ID, province, district)
	if err != nil || m == nil {
		return nil, err
	}
	org, err := s.q.GetOrganizationByID(ctx, m.OrganizationID)
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// addressText fills the display city/district from the chain when the
// caller left them empty.
func addressText(a *geo.Address, city, district string) (string, string) {
	city, district = strings.TrimSpace(city), strings.TrimSpace(district)
	if a == nil {
		return city, district
	}
	if city == "" && a.Province != nil {
		city = a.Province.Name
	}
	if district == "" && a.District != nil {
		district = a.District.Name
	}
	return city, district
}

func addressIDs(a *geo.Address) (country, province, district pgtype.Int8) {
	if a == nil {
		return
	}
	return a.IDs()
}

func int8Ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}
