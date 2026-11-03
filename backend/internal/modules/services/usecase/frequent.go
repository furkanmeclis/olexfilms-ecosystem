package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	vcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/popularity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Frequent vehicles of the active organization: the car brands (or, with a
// car brand, its models) the mobile vehicle picker lists first under
// "Frequently used". TEC-1043: ranked by the popularity engine's mixed
// score (60 % the organization's usage, 40 % the usage of every
// organization of the domain brand, K20), so a new organization without
// services still gets a list. The organization part has the home overview
// reach: its own services and measurements; own / assigned grants count the
// records they created.

// FrequentLimit caps each frequent list.
const FrequentLimit = 8

// FrequentCarBrand is one frequent car brand; UUID is also the logo id.
// ServiceCount is the caller organization's own usage only (0 when only
// other organizations use it); other organizations' counts never leave.
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

// Popularity scores car brands / models (*popularity.Service).
type Popularity interface {
	CarBrands(ctx context.Context, scope popularity.Scope) (popularity.Result, error)
	CarModels(ctx context.Context, scope popularity.Scope, carBrandID int64) (popularity.Result, error)
}

// WithPopularity wires the popularity engine of the frequent list.
func (s *Service) WithPopularity(p Popularity) *Service {
	s.pop = p
	return s
}

func (s *Service) popularity() Popularity {
	if s.pop != nil {
		return s.pop
	}
	return popularity.New(s.q, nil)
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

// FrequentVehicles lists the most used car brands for the active
// organization, or the models of carBrand when it is set.
func (s *Service) FrequentVehicles(ctx context.Context, c Caller, carBrand *uuid.UUID) (FrequentVehicles, error) {
	orgID, err := overviewOrg(c)
	if err != nil {
		return FrequentVehicles{}, err
	}
	scope := popularity.Scope{BrandID: c.Org.BrandID, OrgID: orgID}
	if c.Filter.UserOnly() {
		scope.UserID = c.Filter.UserID
	}
	out := FrequentVehicles{Brands: []FrequentCarBrand{}, Models: []FrequentCarModel{}}
	if carBrand != nil {
		cb, err := s.q.GetCarBrandByUUID(ctx, *carBrand)
		if errors.Is(err, pgx.ErrNoRows) {
			return out, nil
		}
		if err != nil {
			return FrequentVehicles{}, fmt.Errorf("services: frequent car brand: %w", err)
		}
		res, err := s.popularity().CarModels(ctx, scope, cb.ID)
		if err != nil {
			return FrequentVehicles{}, err
		}
		ids := usedIDs(res.Scores)
		if len(ids) == 0 {
			return out, nil
		}
		rows, err := s.q.ActiveCarModelsByIDs(ctx, db.ActiveCarModelsByIDsParams{Ids: ids, CarBrandID: cb.ID})
		if err != nil {
			return FrequentVehicles{}, fmt.Errorf("services: frequent car models: %w", err)
		}
		out.Models = frequentModels(rows, res, FrequentLimit)
		return out, nil
	}
	res, err := s.popularity().CarBrands(ctx, scope)
	if err != nil {
		return FrequentVehicles{}, err
	}
	ids := usedIDs(res.Scores)
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.ActiveCarBrandsByIDs(ctx, ids)
	if err != nil {
		return FrequentVehicles{}, fmt.Errorf("services: frequent car brands: %w", err)
	}
	out.Brands = frequentBrands(rows, res, FrequentLimit)
	return out, nil
}

// usedIDs are the ids with a positive score (used at least once).
func usedIDs(scores map[int64]float64) []int64 {
	ids, vals := popularity.Arrays(scores)
	out := ids[:0:0]
	for i, id := range ids {
		if vals[i] > 0 {
			out = append(out, id)
		}
	}
	return out
}

// frequentBrands ranks the active catalog rows by score (then name, id) and
// keeps the first limit; ServiceCount is the organization's own count.
func frequentBrands(rows []db.ActiveCarBrandsByIDsRow, res popularity.Result, limit int) []FrequentCarBrand {
	byID := make(map[int64]db.ActiveCarBrandsByIDsRow, len(rows))
	entries := make([]popularity.Entry, 0, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
		entries = append(entries, popularity.Entry{ID: r.ID, Name: r.Name})
	}
	out := []FrequentCarBrand{}
	for _, e := range popularity.Rank(entries, res.Scores) {
		if len(out) == limit {
			break
		}
		if res.Scores[e.ID] <= 0 {
			continue
		}
		r := byID[e.ID]
		key := ""
		if r.LogoObjectKey.Valid {
			key = r.LogoObjectKey.String
		}
		out = append(out, FrequentCarBrand{
			UUID: r.Uuid, Name: r.Name, HasLogo: key != "",
			LogoURL: vcuc.BrandLogoURL(r.Uuid, key), ServiceCount: res.OrgCounts[e.ID],
		})
	}
	return out
}

// frequentModels is frequentBrands for the models of one car brand.
func frequentModels(rows []db.ActiveCarModelsByIDsRow, res popularity.Result, limit int) []FrequentCarModel {
	byID := make(map[int64]db.ActiveCarModelsByIDsRow, len(rows))
	entries := make([]popularity.Entry, 0, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
		entries = append(entries, popularity.Entry{ID: r.ID, Name: r.Name})
	}
	out := []FrequentCarModel{}
	for _, e := range popularity.Rank(entries, res.Scores) {
		if len(out) == limit {
			break
		}
		if res.Scores[e.ID] <= 0 {
			continue
		}
		r := byID[e.ID]
		out = append(out, FrequentCarModel{UUID: r.Uuid, Name: r.Name, ServiceCount: res.OrgCounts[e.ID]})
	}
	return out
}
