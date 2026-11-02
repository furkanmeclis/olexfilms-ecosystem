package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	vcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Frequent vehicles of the active organization: the car brands (or, with a
// car brand, its models) the organization services most. The mobile vehicle
// picker lists them first under "Frequently used", like the legacy lookup
// popularity order (dealer counts only). Same reach as the home overview:
// the active organization's own services inside the domain brand (K20);
// own / assigned grants count the services they created.

// FrequentLimit caps each frequent list.
const FrequentLimit = 8

// FrequentCarBrand is one frequent car brand; UUID is also the logo id.
type FrequentCarBrand struct {
	UUID         uuid.UUID `json:"uuid"`
	Name         string    `json:"name"`
	HasLogo      bool      `json:"has_logo"`
	LogoURL      string    `json:"logo_url"`
	ServiceCount int64     `json:"service_count"`
}

// FrequentCarModel is one frequent car model of the requested car brand.
type FrequentCarModel struct {
	UUID         uuid.UUID `json:"uuid"`
	Name         string    `json:"name"`
	ServiceCount int64     `json:"service_count"`
}

// FrequentVehicles is the response of GET /v1/stats/frequent-vehicles.
// Without a car brand only Brands is filled; with one only Models.
type FrequentVehicles struct {
	Brands []FrequentCarBrand `json:"brands"`
	Models []FrequentCarModel `json:"models"`
}

// ParseCarBrandUUID reads the optional car_brand_uuid query value.
func ParseCarBrandUUID(raw string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, invalid("car_brand_uuid", "must be a uuid")
	}
	return &id, nil
}

// FrequentVehicles lists the most serviced car brands of the active
// organization, or the models of carBrand when it is set.
func (s *Service) FrequentVehicles(ctx context.Context, c Caller, carBrand *uuid.UUID) (FrequentVehicles, error) {
	orgID, err := overviewOrg(c)
	if err != nil {
		return FrequentVehicles{}, err
	}
	var createdBy pgtype.Int8
	if c.Filter.UserOnly() {
		createdBy = pgtype.Int8{Int64: c.Filter.UserID, Valid: true}
	}
	out := FrequentVehicles{Brands: []FrequentCarBrand{}, Models: []FrequentCarModel{}}
	if carBrand != nil {
		rows, err := s.q.FrequentServicedCarModels(ctx, db.FrequentServicedCarModelsParams{
			BrandID: c.Org.BrandID, OrgID: orgID, CreatedByUserID: createdBy, CarBrandUuid: *carBrand, RowLimit: FrequentLimit,
		})
		if err != nil {
			return FrequentVehicles{}, fmt.Errorf("services: frequent car models: %w", err)
		}
		for _, r := range rows {
			out.Models = append(out.Models, FrequentCarModel{UUID: r.CarModelUuid, Name: r.CarModelName, ServiceCount: r.ServiceCount})
		}
		return out, nil
	}
	rows, err := s.q.FrequentServicedCarBrands(ctx, db.FrequentServicedCarBrandsParams{
		BrandID: c.Org.BrandID, OrgID: orgID, CreatedByUserID: createdBy, RowLimit: FrequentLimit,
	})
	if err != nil {
		return FrequentVehicles{}, fmt.Errorf("services: frequent car brands: %w", err)
	}
	for _, r := range rows {
		key := ""
		if r.CarBrandLogoKey.Valid {
			key = r.CarBrandLogoKey.String
		}
		out.Brands = append(out.Brands, FrequentCarBrand{
			UUID: r.CarBrandUuid, Name: r.CarBrandName, HasLogo: key != "",
			LogoURL: vcuc.BrandLogoURL(r.CarBrandUuid, key), ServiceCount: r.ServiceCount,
		})
	}
	return out, nil
}
