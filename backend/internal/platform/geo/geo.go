// Package geo holds the country > province > district tree, distributor
// territories (K5) and licence plate formats (TEC-84).
package geo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound: the country, province, district or territory is unknown.
	ErrNotFound = errors.New("geo: not found")
	// ErrInvalid: the request is malformed (an address chain that does not
	// hold, a bad regex, a missing field).
	ErrInvalid = errors.New("geo: invalid request")
	// ErrDuplicate: a province/district/plate format with the same key exists.
	ErrDuplicate = errors.New("geo: already exists")
	// ErrInUse: the row is referenced (organizations, territories).
	ErrInUse = errors.New("geo: in use")
)

// Service reads and writes the geography tables.
type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// New creates the geo service.
func New(pool *pgxpool.Pool, q *db.Queries) *Service {
	return &Service{pool: pool, q: q}
}

// Country is the public country projection.
type Country struct {
	ID              int64   `json:"id"`
	ISO2            string  `json:"iso2"`
	ISO3            string  `json:"iso3"`
	NameEn          string  `json:"name_en"`
	NameTr          string  `json:"name_tr"`
	PhoneCode       *string `json:"phone_code,omitempty"`
	DefaultCurrency *string `json:"default_currency,omitempty"`
	DefaultLocale   *string `json:"default_locale,omitempty"`
	Timezone        *string `json:"timezone,omitempty"`
	IsActive        bool    `json:"is_active"`
	HasProvinces    bool    `json:"has_provinces"`
}

// Province is one first-level subdivision.
type Province struct {
	ID           int64  `json:"id"`
	CountryID    int64  `json:"country_id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	HasDistricts bool   `json:"has_districts"`
}

// District is one second-level subdivision.
type District struct {
	ID         int64   `json:"id"`
	ProvinceID int64   `json:"province_id"`
	Code       *string `json:"code,omitempty"`
	Name       string  `json:"name"`
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func mapCountry(c db.Country, hasProvinces bool) Country {
	return Country{
		ID: c.ID, ISO2: c.Iso2, ISO3: c.Iso3, NameEn: c.NameEn, NameTr: c.NameTr,
		PhoneCode: textPtr(c.PhoneCode), DefaultCurrency: textPtr(c.DefaultCurrency),
		DefaultLocale: textPtr(c.DefaultLocale), Timezone: textPtr(c.Timezone),
		IsActive: c.IsActive, HasProvinces: hasProvinces,
	}
}

func mapDistrict(d db.District) District {
	return District{ID: d.ID, ProvinceID: d.ProvinceID, Code: textPtr(d.Code), Name: d.Name}
}

// NormalizeISO2 upper-cases and validates a country code.
func NormalizeISO2(raw string) (string, error) {
	v := strings.ToUpper(strings.TrimSpace(raw))
	if len(v) != 2 || v[0] < 'A' || v[0] > 'Z' || v[1] < 'A' || v[1] > 'Z' {
		return "", fmt.Errorf("%w: country must be an ISO 3166-1 alpha-2 code", ErrInvalid)
	}
	return v, nil
}

// Countries lists countries (active only unless all is set).
func (s *Service) Countries(ctx context.Context, all bool) ([]Country, error) {
	rows, err := s.q.ListCountries(ctx, !all)
	if err != nil {
		return nil, err
	}
	out := make([]Country, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapCountry(db.Country{
			ID: r.ID, Iso2: r.Iso2, Iso3: r.Iso3, NumericCode: r.NumericCode, NameEn: r.NameEn, NameTr: r.NameTr,
			PhoneCode: r.PhoneCode, DefaultCurrency: r.DefaultCurrency, DefaultLocale: r.DefaultLocale,
			Timezone: r.Timezone, IsActive: r.IsActive,
		}, r.HasProvinces))
	}
	return out, nil
}

// CountryByISO2 loads one country.
func (s *Service) CountryByISO2(ctx context.Context, iso2 string) (db.Country, error) {
	code, err := NormalizeISO2(iso2)
	if err != nil {
		return db.Country{}, err
	}
	c, err := s.q.GetCountryByISO2(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Country{}, fmt.Errorf("%w: country %s", ErrNotFound, code)
	}
	return c, err
}

// SetCountryActive shows or hides a country in pickers.
func (s *Service) SetCountryActive(ctx context.Context, iso2 string, active bool) (Country, error) {
	code, err := NormalizeISO2(iso2)
	if err != nil {
		return Country{}, err
	}
	c, err := s.q.SetCountryActive(ctx, db.SetCountryActiveParams{Iso2: code, IsActive: active})
	if errors.Is(err, pgx.ErrNoRows) {
		return Country{}, fmt.Errorf("%w: country %s", ErrNotFound, code)
	}
	if err != nil {
		return Country{}, err
	}
	return mapCountry(c, false), nil
}

// Provinces lists the provinces of a country.
func (s *Service) Provinces(ctx context.Context, iso2 string) ([]Province, error) {
	c, err := s.CountryByISO2(ctx, iso2)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListProvincesByCountry(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	out := make([]Province, 0, len(rows))
	for _, r := range rows {
		out = append(out, Province{ID: r.ID, CountryID: r.CountryID, Code: r.Code, Name: r.Name, HasDistricts: r.HasDistricts})
	}
	return out, nil
}

// Districts lists the districts of a province.
func (s *Service) Districts(ctx context.Context, provinceID int64) ([]District, error) {
	if _, err := s.q.GetProvinceByID(ctx, provinceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: province", ErrNotFound)
		}
		return nil, err
	}
	rows, err := s.q.ListDistrictsByProvince(ctx, provinceID)
	if err != nil {
		return nil, err
	}
	out := make([]District, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapDistrict(r))
	}
	return out, nil
}

// CreateProvince adds a province to a country (platform.geo.write).
func (s *Service) CreateProvince(ctx context.Context, iso2, code, name string) (Province, error) {
	c, err := s.CountryByISO2(ctx, iso2)
	if err != nil {
		return Province{}, err
	}
	code, name = strings.TrimSpace(code), strings.TrimSpace(name)
	if code == "" || name == "" || len(code) > 16 || len(name) > 120 {
		return Province{}, fmt.Errorf("%w: code (max 16) and name (max 120) are required", ErrInvalid)
	}
	p, err := s.q.CreateProvince(ctx, db.CreateProvinceParams{CountryID: c.ID, Code: code, Name: name})
	if err != nil {
		return Province{}, mapWriteErr(err)
	}
	return Province{ID: p.ID, CountryID: p.CountryID, Code: p.Code, Name: p.Name}, nil
}

// DeleteProvince removes a province and its districts. Provinces used by an
// organization or a territory cannot be removed.
func (s *Service) DeleteProvince(ctx context.Context, id int64) error {
	n, err := s.q.DeleteProvince(ctx, id)
	if err != nil {
		return mapWriteErr(err)
	}
	if n == 0 {
		return fmt.Errorf("%w: province", ErrNotFound)
	}
	return nil
}

// CreateDistrict adds a district to a province (platform.geo.write).
func (s *Service) CreateDistrict(ctx context.Context, provinceID int64, code, name string) (District, error) {
	if _, err := s.q.GetProvinceByID(ctx, provinceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return District{}, fmt.Errorf("%w: province", ErrNotFound)
		}
		return District{}, err
	}
	code, name = strings.TrimSpace(code), strings.TrimSpace(name)
	if name == "" || len(name) > 120 || len(code) > 16 {
		return District{}, fmt.Errorf("%w: name (max 120) is required, code max 16", ErrInvalid)
	}
	d, err := s.q.CreateDistrict(ctx, db.CreateDistrictParams{
		ProvinceID: provinceID, Code: pgtype.Text{String: code, Valid: code != ""}, Name: name,
	})
	if err != nil {
		return District{}, mapWriteErr(err)
	}
	return mapDistrict(d), nil
}

// DeleteDistrict removes a district that nothing references.
func (s *Service) DeleteDistrict(ctx context.Context, id int64) error {
	n, err := s.q.DeleteDistrict(ctx, id)
	if err != nil {
		return mapWriteErr(err)
	}
	if n == 0 {
		return fmt.Errorf("%w: district", ErrNotFound)
	}
	return nil
}

// Address is a validated country > province > district chain.
type Address struct {
	Country  db.Country
	Province *db.Province
	District *db.District
}

// IDs returns the chain as nullable ids.
func (a Address) IDs() (country, province, district pgtype.Int8) {
	country = pgtype.Int8{Int64: a.Country.ID, Valid: a.Country.ID != 0}
	if a.Province != nil {
		province = pgtype.Int8{Int64: a.Province.ID, Valid: true}
	}
	if a.District != nil {
		district = pgtype.Int8{Int64: a.District.ID, Valid: true}
	}
	return country, province, district
}

// ValidateAddress loads an address chain and checks that the province is in
// the country and the district in the province. provinceID/districtID may
// be nil (a country- or province-level address).
func (s *Service) ValidateAddress(ctx context.Context, countryID int64, provinceID, districtID *int64) (Address, error) {
	c, err := s.q.GetCountryByID(ctx, countryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Address{}, fmt.Errorf("%w: country_id is unknown", ErrInvalid)
		}
		return Address{}, err
	}
	out := Address{Country: c}
	if districtID != nil && provinceID == nil {
		return Address{}, fmt.Errorf("%w: district_id needs province_id", ErrInvalid)
	}
	if provinceID != nil {
		p, err := s.q.GetProvinceByID(ctx, *provinceID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Address{}, fmt.Errorf("%w: province_id is unknown", ErrInvalid)
			}
			return Address{}, err
		}
		if p.CountryID != c.ID {
			return Address{}, fmt.Errorf("%w: province is not in the country", ErrInvalid)
		}
		out.Province = &p
	}
	if districtID != nil {
		d, err := s.q.GetDistrictByID(ctx, *districtID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Address{}, fmt.Errorf("%w: district_id is unknown", ErrInvalid)
			}
			return Address{}, err
		}
		if d.ProvinceID != out.Province.ID {
			return Address{}, fmt.Errorf("%w: district is not in the province", ErrInvalid)
		}
		out.District = &d
	}
	return out, nil
}

func mapWriteErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: %s", ErrDuplicate, pgErr.ConstraintName)
		case "23503":
			return fmt.Errorf("%w: %s", ErrInUse, pgErr.ConstraintName)
		}
	}
	return err
}
