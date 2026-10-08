package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrInvalidRequest = errors.New("performance: invalid request")

var periodRe = regexp.MustCompile(`^\d{4}-\d{2}$`)

const (
	LevelCountry  = "country"
	LevelProvince = "province"
	LevelDistrict = "district"
)

type Caller struct {
	Org orgctx.Scope
}

type MapFilter struct {
	Level      string `json:"level"`
	CountryISO string `json:"country"`
	Period     string `json:"period"`
	Metric     string `json:"metric"`
}

type RegionMap struct {
	Level              string        `json:"level"`
	Country            string        `json:"country,omitempty"`
	Period             string        `json:"period"`
	Metric             string        `json:"metric"`
	Items              []RegionItem  `json:"items"`
	EmptyRegions       []EmptyRegion `json:"empty_regions"`
	MissingCoordinates int64         `json:"missing_coordinates"`
}

type RegionItem struct {
	Level              string       `json:"level"`
	ID                 int64        `json:"id"`
	Code               string       `json:"code"`
	Name               string       `json:"name"`
	CountryISO         string       `json:"country_iso2"`
	CountryName        string       `json:"country_name"`
	ProvinceCode       string       `json:"province_code,omitempty"`
	ProvinceName       string       `json:"province_name,omitempty"`
	DealerCount        int64        `json:"dealer_count"`
	Distributor        *Distributor `json:"distributor,omitempty"`
	MetricAvg          *float64     `json:"metric_avg,omitempty"`
	Latitude           *float64     `json:"latitude,omitempty"`
	Longitude          *float64     `json:"longitude,omitempty"`
	MissingCoordinates int64        `json:"missing_coordinates"`
}

type Distributor struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

type EmptyRegion struct {
	Level       string       `json:"level"`
	ID          int64        `json:"id"`
	Code        string       `json:"code"`
	Name        string       `json:"name"`
	CountryISO  string       `json:"country_iso2"`
	Reason      string       `json:"empty_reason"`
	Distributor *Distributor `json:"distributor,omitempty"`
}

type DealerMap struct {
	Country            string        `json:"country,omitempty"`
	Period             string        `json:"period"`
	Metric             string        `json:"metric"`
	Items              []DealerPoint `json:"items"`
	MissingCoordinates int64         `json:"missing_coordinates"`
}

type DealerPoint struct {
	UUID        uuid.UUID `json:"uuid"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	CountryISO  string    `json:"country_iso2"`
	Province    string    `json:"province,omitempty"`
	District    string    `json:"district,omitempty"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	MetricValue *float64  `json:"metric_value,omitempty"`
	ShowcaseURL string    `json:"showcase_url"`
}

func ParseMapFilter(values url.Values, now time.Time) (MapFilter, error) {
	level := strings.TrimSpace(values.Get("level"))
	if level == "" {
		level = LevelProvince
	}
	switch level {
	case LevelCountry, LevelProvince, LevelDistrict:
	default:
		return MapFilter{}, fmt.Errorf("%w: level", ErrInvalidRequest)
	}
	metric := strings.TrimSpace(values.Get("metric"))
	if metric == "" {
		metric = model.MetricServicesCount
	}
	if !model.IsMetric(metric) {
		return MapFilter{}, fmt.Errorf("%w: metric", ErrInvalidRequest)
	}
	period := strings.TrimSpace(values.Get("period"))
	if period == "" {
		if now.IsZero() {
			now = time.Now()
		}
		period = now.UTC().Format("2006-01")
	}
	if !periodRe.MatchString(period) {
		return MapFilter{}, fmt.Errorf("%w: period", ErrInvalidRequest)
	}
	country := strings.ToUpper(strings.TrimSpace(values.Get("country")))
	if country != "" && len(country) != 2 {
		return MapFilter{}, fmt.Errorf("%w: country", ErrInvalidRequest)
	}
	return MapFilter{Level: level, CountryISO: country, Period: period, Metric: metric}, nil
}

func (s *Service) RegionMap(ctx context.Context, caller Caller, f MapFilter) (RegionMap, error) {
	rows, err := s.q.ListPerformanceMapRegions(ctx, db.ListPerformanceMapRegionsParams{
		BrandID: caller.Org.BrandID, ActorType: caller.Org.OrgType, ActorOrgID: caller.Org.InternalID,
		CountryIso2: text(f.CountryISO), Level: f.Level, Period: f.Period, Metric: f.Metric,
	})
	if err != nil {
		return RegionMap{}, fmt.Errorf("performance map: regions: %w", err)
	}
	out := RegionMap{Level: f.Level, Country: f.CountryISO, Period: f.Period, Metric: f.Metric, Items: []RegionItem{}, EmptyRegions: []EmptyRegion{}}
	for _, row := range rows {
		item := RegionItem{
			Level: row.Level, ID: row.AreaID, Code: row.Code, Name: row.Name,
			CountryISO: row.CountryIso2, CountryName: row.CountryName,
			ProvinceCode: row.ProvinceCode, ProvinceName: row.ProvinceName.String,
			DealerCount: row.DealerCount, MissingCoordinates: row.MissingCoordinates,
			MetricAvg: numeric(row.MetricAvg), Latitude: numeric(row.Latitude), Longitude: numeric(row.Longitude),
		}
		if row.DistributorID > 0 {
			item.Distributor = &Distributor{UUID: row.DistributorUuid, Name: row.DistributorName}
		}
		out.MissingCoordinates += row.MissingCoordinates
		out.Items = append(out.Items, item)
		if row.EmptyReason != "" {
			out.EmptyRegions = append(out.EmptyRegions, EmptyRegion{
				Level: item.Level, ID: item.ID, Code: item.Code, Name: item.Name,
				CountryISO: item.CountryISO, Reason: row.EmptyReason, Distributor: item.Distributor,
			})
		}
	}
	return out, nil
}

func (s *Service) DealerMap(ctx context.Context, caller Caller, f MapFilter) (DealerMap, error) {
	arg := db.ListPerformanceMapDealersParams{
		BrandID: caller.Org.BrandID, ActorType: caller.Org.OrgType, ActorOrgID: caller.Org.InternalID,
		CountryIso2: text(f.CountryISO), Period: f.Period, Metric: f.Metric,
	}
	missing, err := s.q.CountPerformanceMapDealerMissingCoordinates(ctx, db.CountPerformanceMapDealerMissingCoordinatesParams{
		BrandID: arg.BrandID, ActorType: arg.ActorType, ActorOrgID: arg.ActorOrgID, CountryIso2: arg.CountryIso2,
	})
	if err != nil {
		return DealerMap{}, fmt.Errorf("performance map: dealer missing coordinates: %w", err)
	}
	rows, err := s.q.ListPerformanceMapDealers(ctx, arg)
	if err != nil {
		return DealerMap{}, fmt.Errorf("performance map: dealers: %w", err)
	}
	out := DealerMap{Country: f.CountryISO, Period: f.Period, Metric: f.Metric, MissingCoordinates: missing, Items: []DealerPoint{}}
	for _, row := range rows {
		lat, lon := numeric(row.Latitude), numeric(row.Longitude)
		if lat == nil || lon == nil {
			continue
		}
		out.Items = append(out.Items, DealerPoint{
			UUID: row.OrganizationUuid, Code: row.Slug, Name: row.Name,
			CountryISO: row.CountryIso2, Province: row.ProvinceName.String, District: row.DistrictName.String,
			Latitude: *lat, Longitude: *lon, MetricValue: numeric(row.MetricValue),
			ShowcaseURL: "/bayi/" + row.Slug,
		})
	}
	return out, nil
}

func text(v string) pgtype.Text {
	return pgtype.Text{String: v, Valid: v != ""}
}

func numeric(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	v, err := n.Float64Value()
	if err != nil || !v.Valid {
		return nil
	}
	return &v.Float64
}
