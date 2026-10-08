package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-240: dealer coordinates and the public nearby-dealers lookup.

const (
	// NearbyDefaultRadiusKm is used when radius_km is not given.
	NearbyDefaultRadiusKm = 100.0
	// NearbyMaxRadiusKm caps radius_km.
	NearbyMaxRadiusKm = 1000.0
	// NearbyMaxResults caps the result list.
	NearbyMaxResults = 50
)

// ErrInvalidCoordinates rejects a coordinate patch: only one of the pair,
// or a value out of the WGS84 range. It wraps ErrInvalidRequest.
var ErrInvalidCoordinates = fmt.Errorf("%w: latitude and longitude must be set together (latitude -90..90, longitude -180..180)", ErrInvalidRequest)

var e164 = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

// OptionalCoordinate tells an absent JSON field from an explicit null.
type OptionalCoordinate struct {
	Set   bool
	Value *float64
}

// UnmarshalJSON implements json.Unmarshaler.
func (o *OptionalCoordinate) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var v float64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// ValidLatitude reports whether v is a finite latitude in degrees.
func ValidLatitude(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= -90 && v <= 90
}

// ValidLongitude reports whether v is a finite longitude in degrees.
func ValidLongitude(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= -180 && v <= 180
}

// coordinatePatch validates a latitude/longitude patch. set is false when
// neither field was sent; otherwise lat/lng are both nil (clear) or both
// valid values.
func coordinatePatch(lat, lng OptionalCoordinate) (set bool, latN, lngN pgtype.Numeric, err error) {
	if !lat.Set && !lng.Set {
		return false, pgtype.Numeric{}, pgtype.Numeric{}, nil
	}
	if lat.Set != lng.Set || (lat.Value == nil) != (lng.Value == nil) {
		return false, pgtype.Numeric{}, pgtype.Numeric{}, ErrInvalidCoordinates
	}
	if lat.Value == nil {
		return true, pgtype.Numeric{}, pgtype.Numeric{}, nil
	}
	if !ValidLatitude(*lat.Value) || !ValidLongitude(*lng.Value) {
		return false, pgtype.Numeric{}, pgtype.Numeric{}, ErrInvalidCoordinates
	}
	latN, err = numeric6(*lat.Value)
	if err != nil {
		return false, pgtype.Numeric{}, pgtype.Numeric{}, ErrInvalidCoordinates
	}
	lngN, err = numeric6(*lng.Value)
	if err != nil {
		return false, pgtype.Numeric{}, pgtype.Numeric{}, ErrInvalidCoordinates
	}
	return true, latN, lngN, nil
}

func numeric6(v float64) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	err := n.Scan(strconv.FormatFloat(v, 'f', 6, 64))
	return n, err
}

// numericFloat returns a nullable NUMERIC as *float64.
func numericFloat(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	v := f.Float64
	return &v
}

// NearbyInput is a validated nearby-dealers query.
type NearbyInput struct {
	Lat      float64
	Lng      float64
	RadiusKm float64
}

// NearbyDealer is one row of GET /v1/public/dealers/nearby.
type NearbyDealer struct {
	// UUID is the organization uuid the portal books appointments with (TEC-327).
	UUID                uuid.UUID `json:"uuid"`
	Slug                string    `json:"slug"`
	Name                string    `json:"name"`
	City                string    `json:"city"`
	District            string    `json:"district"`
	Latitude            float64   `json:"latitude"`
	Longitude           float64   `json:"longitude"`
	DistanceKm          float64   `json:"distance_km"`
	AcceptsAppointments bool      `json:"accepts_appointments"`
	WhatsApp            *string   `json:"whatsapp"`
	// TEC-467: the dealer serves a published showcase (module on) and its
	// Google rating (null when unrated or without a showcase).
	HasShowcase  bool     `json:"has_showcase"`
	GoogleRating *float64 `json:"google_rating"`
}

// NearbyDealers lists the active dealers / distributors of a brand around
// a point, nearest first, at most NearbyMaxResults.
func (s *Service) NearbyDealers(ctx context.Context, brandID int64, in NearbyInput) ([]NearbyDealer, error) {
	rows, err := s.q.ListNearbyDealers(ctx, db.ListNearbyDealersParams{
		Lat: in.Lat, Lng: in.Lng, BrandID: brandID, RadiusKm: in.RadiusKm, LimitCount: NearbyMaxResults,
	})
	if err != nil {
		return nil, err
	}
	out := make([]NearbyDealer, 0, len(rows))
	for _, r := range rows {
		d := NearbyDealer{
			UUID: r.Uuid, Slug: r.Slug, Name: r.Name, City: r.City, District: r.District,
			Latitude: r.Latitude, Longitude: r.Longitude,
			DistanceKm:          math.Round(r.DistanceKm*100) / 100,
			AcceptsAppointments: r.AcceptsAppointments,
		}
		if e164.MatchString(r.Phone) {
			p := r.Phone
			d.WhatsApp = &p
		}
		out = append(out, d)
	}
	return s.withBadges(ctx, brandID, out)
}

// withBadges sets has_showcase / google_rating from the showcase data.
func (s *Service) withBadges(ctx context.Context, brandID int64, out []NearbyDealer) ([]NearbyDealer, error) {
	if s.showcases == nil || len(out) == 0 {
		return out, nil
	}
	ids := make([]uuid.UUID, len(out))
	for i, d := range out {
		ids[i] = d.UUID
	}
	badges, err := s.showcases.Badges(ctx, brandID, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if rating, ok := badges[out[i].UUID]; ok {
			out[i].HasShowcase, out[i].GoogleRating = true, rating
		}
	}
	return out, nil
}

// AreaInput is a validated dealers-in-area query (TEC-386): City is
// required, District optional.
type AreaInput struct {
	City     string
	District string
}

// AreaDealers lists the active, serving dealers / distributors of a brand
// in a city (and district), at most NearbyMaxResults. The rows have no
// distance or coordinates. It answers the AI visitor tool when the visitor
// names a place instead of sharing a location.
func (s *Service) AreaDealers(ctx context.Context, brandID int64, in AreaInput) ([]NearbyDealer, error) {
	arg := db.ListAreaDealersParams{BrandID: brandID, City: in.City, LimitCount: NearbyMaxResults}
	if in.District != "" {
		arg.District = pgtype.Text{String: in.District, Valid: true}
	}
	rows, err := s.q.ListAreaDealers(ctx, arg)
	if err != nil {
		return nil, err
	}
	out := make([]NearbyDealer, 0, len(rows))
	for _, r := range rows {
		d := NearbyDealer{
			UUID: r.Uuid, Slug: r.Slug, Name: r.Name, City: r.City, District: r.District,
			AcceptsAppointments: r.AcceptsAppointments,
		}
		if e164.MatchString(r.Phone) {
			p := r.Phone
			d.WhatsApp = &p
		}
		out = append(out, d)
	}
	return out, nil
}
