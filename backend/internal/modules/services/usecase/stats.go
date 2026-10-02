package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	vcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-151 (F1-09c): top-10 car brands / models by completed services for
// the center dashboard. Brand scoped (K20): a center sees its own domain
// brand, super_admin the brand of the organization it selected. Dealers and
// distributors (organization / subtree scope) get ErrForbidden.

// Statistic periods (query parameter period).
const (
	StatsPeriod30d = "30d"
	StatsPeriod90d = "90d"
	StatsPeriod12m = "12m"
	StatsPeriodAll = "all"
)

// Statistic groupings (query parameter group).
const (
	StatsGroupBrand = "brand"
	StatsGroupModel = "model"
)

// TopVehicleLimit is the number of rows of the top list.
const TopVehicleLimit = 10

// StatsCarBrand is the car brand of a top-list row; UUID is also the logo id
// (/brand-logos/{uuid}).
type StatsCarBrand struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	HasLogo bool      `json:"has_logo"`
	LogoURL string    `json:"logo_url"`
}

// StatsCarModel is the car model of a top-list row (group=model only).
type StatsCarModel struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// TopVehicle is one row of the top list.
type TopVehicle struct {
	ServiceCount int64          `json:"service_count"`
	CarBrand     StatsCarBrand  `json:"car_brand"`
	CarModel     *StatsCarModel `json:"car_model"`
}

// TopVehicles is the response of GET /v1/stats/top-vehicle-models.
type TopVehicles struct {
	Period string       `json:"period"`
	Group  string       `json:"group"`
	Since  *time.Time   `json:"since"`
	Items  []TopVehicle `json:"items"`
}

// StatsSince returns the lower completed_at bound of period at now (nil for
// all time). 12m is twelve calendar months.
func StatsSince(period string, now time.Time) (*time.Time, error) {
	var t time.Time
	switch period {
	case StatsPeriod30d:
		t = now.AddDate(0, 0, -30)
	case StatsPeriod90d:
		t = now.AddDate(0, 0, -90)
	case StatsPeriod12m:
		t = now.AddDate(0, -12, 0)
	case StatsPeriodAll:
		return nil, nil
	default:
		return nil, invalid("period", "must be one of 30d, 90d, 12m, all")
	}
	return &t, nil
}

// statsBrand returns the domain brand the caller may read statistics of:
// the center with a brand-wide services.read grant, or a platform operator
// (scope all) for the brand of the selected organization.
func statsBrand(c Caller) (int64, error) {
	scope, ok := c.Principal.ScopeFor(rbac.PermServicesRead)
	if !ok || c.Org.BrandID == 0 {
		return 0, ErrForbidden
	}
	switch scope {
	case rbac.ScopeAll:
		return c.Org.BrandID, nil
	case rbac.ScopeBrand:
		if c.isCenter() {
			return c.Org.BrandID, nil
		}
	}
	return 0, ErrForbidden
}

// TopVehicleModels lists the top car brands (group=brand) or models
// (group=model) by completed services of the caller's brand in period.
func (s *Service) TopVehicleModels(ctx context.Context, c Caller, period, group string, now time.Time) (TopVehicles, error) {
	period = strings.TrimSpace(period)
	if period == "" {
		period = StatsPeriod30d
	}
	group = strings.TrimSpace(group)
	if group == "" {
		group = StatsGroupModel
	}
	if group != StatsGroupBrand && group != StatsGroupModel {
		return TopVehicles{}, invalid("group", "must be brand or model")
	}
	since, err := StatsSince(period, now)
	if err != nil {
		return TopVehicles{}, err
	}
	brandID, err := statsBrand(c)
	if err != nil {
		return TopVehicles{}, err
	}
	sinceArg := pgtype.Timestamptz{}
	if since != nil {
		sinceArg = pgtype.Timestamptz{Time: *since, Valid: true}
	}
	out := TopVehicles{Period: period, Group: group, Since: since, Items: []TopVehicle{}}
	brand := func(id uuid.UUID, name string, key pgtype.Text) StatsCarBrand {
		k := ""
		if key.Valid {
			k = key.String
		}
		return StatsCarBrand{UUID: id, Name: name, HasLogo: k != "", LogoURL: vcuc.BrandLogoURL(id, k)}
	}
	if group == StatsGroupBrand {
		rows, err := s.q.TopServicedCarBrands(ctx, db.TopServicedCarBrandsParams{BrandID: brandID, Since: sinceArg})
		if err != nil {
			return TopVehicles{}, fmt.Errorf("services: top car brands: %w", err)
		}
		for _, r := range rows {
			out.Items = append(out.Items, TopVehicle{
				ServiceCount: r.ServiceCount,
				CarBrand:     brand(r.CarBrandUuid, r.CarBrandName, r.CarBrandLogoKey),
			})
		}
		return out, nil
	}
	rows, err := s.q.TopServicedCarModels(ctx, db.TopServicedCarModelsParams{BrandID: brandID, Since: sinceArg})
	if err != nil {
		return TopVehicles{}, fmt.Errorf("services: top car models: %w", err)
	}
	for _, r := range rows {
		out.Items = append(out.Items, TopVehicle{
			ServiceCount: r.ServiceCount,
			CarBrand:     brand(r.CarBrandUuid, r.CarBrandName, r.CarBrandLogoKey),
			CarModel:     &StatsCarModel{UUID: r.CarModelUuid, Name: r.CarModelName},
		})
	}
	return out, nil
}
