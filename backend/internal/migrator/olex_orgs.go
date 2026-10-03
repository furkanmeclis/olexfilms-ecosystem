package migrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/normalize"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/slug"
)

// Olex organization tree defaults (TEC-254, K1/K5).
const (
	OlexBrandSlug = "olex"
	// TRCountry is the country of the Türkiye distributor and the default
	// country of legacy addresses and phones.
	TRCountry = "TR"
	// TRDistributorSlug and TRDistributorName name the Türkiye distributor
	// the migrator creates when the brand has none.
	TRDistributorSlug = "olex-turkiye"
	TRDistributorName = "Olex Türkiye Distribütörü"
	olexCenterSlug    = "olex-merkez"
	olexCenterName    = "Olex Merkez"
)

// Report count keys shared by the steps. Keys with a ":" suffix carry the
// offending value (a role name, a dealer id) so the run report lists them.
const (
	cntRead      = "read"
	cntCreated   = "created"
	cntUpdated   = "updated"
	cntUnchanged = "unchanged"
)

type counts map[string]int64

func (c counts) inc(key string)          { c[key]++ }
func (c counts) add(key string, n int64) { c[key] += n }

// olexTree is the fixed top of the Olex organization tree.
type olexTree struct {
	BrandID       int64
	CenterID      int64
	CountryID     int64 // Türkiye
	DistributorID int64 // Türkiye distributor
}

// resolveOlexTree finds the Olex center and the Türkiye distributor. With
// create it adds whichever is missing (K5: the distributor gets the TR
// country territory); without it a missing one is an error.
func resolveOlexTree(ctx context.Context, q *db.Queries, create bool, c counts) (olexTree, error) {
	var t olexTree
	brand, err := q.GetBrandBySlug(ctx, OlexBrandSlug)
	if err != nil {
		return t, fmt.Errorf("brand %q: %w", OlexBrandSlug, err)
	}
	t.BrandID = brand.ID
	country, err := q.GetCountryByISO2(ctx, TRCountry)
	if err != nil {
		return t, fmt.Errorf("country %s: %w", TRCountry, err)
	}
	t.CountryID = country.ID

	center, err := q.GetBrandCenter(ctx, brand.ID)
	switch {
	case err == nil:
		t.CenterID = center.ID
	case errors.Is(err, pgx.ErrNoRows) && create:
		t.CenterID, err = q.MigratorInsertOrganization(ctx, db.MigratorInsertOrganizationParams{
			Uuid: uuid.New(), Slug: olexCenterSlug, Name: olexCenterName, Status: "active",
			Type: "center", BrandID: brand.ID, CountryID: pgInt8(country.ID),
		})
		if err != nil {
			return t, fmt.Errorf("create center: %w", err)
		}
		c.inc("center_created")
	default:
		return t, fmt.Errorf("olex center: %w", err)
	}

	id, err := q.MigratorCountryDistributor(ctx, db.MigratorCountryDistributorParams{BrandID: brand.ID, CountryID: country.ID})
	if err == nil {
		t.DistributorID = id
		return t, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return t, fmt.Errorf("TR distributor: %w", err)
	}
	// No country territory yet: a distributor created by an earlier run (or
	// by hand) under the reserved slug is reused.
	id, err = q.MigratorDistributorBySlug(ctx, db.MigratorDistributorBySlugParams{Slug: TRDistributorSlug, BrandID: brand.ID})
	switch {
	case err == nil:
		t.DistributorID = id
	case errors.Is(err, pgx.ErrNoRows) && create:
		t.DistributorID, err = q.MigratorInsertOrganization(ctx, db.MigratorInsertOrganizationParams{
			Uuid: uuid.New(), Slug: TRDistributorSlug, Name: TRDistributorName, Status: "active",
			Type: "distributor", ParentID: pgInt8(t.CenterID), BrandID: brand.ID, CountryID: pgInt8(country.ID),
		})
		if err != nil {
			return t, fmt.Errorf("create TR distributor: %w", err)
		}
		c.inc("distributor_created")
	case errors.Is(err, pgx.ErrNoRows):
		return t, errors.New("TR distributor is missing; run the organizations step first")
	default:
		return t, fmt.Errorf("TR distributor: %w", err)
	}
	if create {
		if err := assignCountryTerritory(ctx, q, t, c); err != nil {
			return t, err
		}
	}
	return t, nil
}

// assignCountryTerritory gives Türkiye to the distributor under the same
// advisory lock the territory service takes. An overlapping territory of
// another distributor is left alone and reported (K5 forbids overlaps).
func assignCountryTerritory(ctx context.Context, q *db.Queries, t olexTree, c counts) error {
	if err := q.LockTerritoryArea(ctx, db.LockTerritoryAreaParams{BrandID: t.BrandID, CountryID: t.CountryID}); err != nil {
		return fmt.Errorf("lock territory: %w", err)
	}
	overlaps, err := q.ListOverlappingTerritories(ctx, db.ListOverlappingTerritoriesParams{BrandID: t.BrandID, CountryID: t.CountryID})
	if err != nil {
		return fmt.Errorf("territory overlaps: %w", err)
	}
	if len(overlaps) > 0 {
		c.add("territory_conflict", int64(len(overlaps)))
		return nil
	}
	if _, err := q.CreateTerritory(ctx, db.CreateTerritoryParams{
		BrandID: t.BrandID, OrganizationID: t.DistributorID, CountryID: t.CountryID,
	}); err != nil {
		return fmt.Errorf("create TR territory: %w", err)
	}
	c.inc("territory_created")
	return nil
}

// OrganizationsStep imports hub dealers as Olex dealer organizations under
// the Türkiye distributor, creating the center and the distributor when they
// are missing (TEC-254).
type OrganizationsStep struct {
	// System is the migration_map source system; empty means SourceHub.
	System string
}

// Name implements Step.
func (OrganizationsStep) Name() string { return "organizations" }

func (s OrganizationsStep) system() string {
	if s.System == "" {
		return SourceHub
	}
	return s.System
}

const dealersQuery = `SELECT id, COALESCE(dealer_code, ''), name, COALESCE(email, ''), COALESCE(phone, ''),
	COALESCE(address, ''), COALESCE(city, ''), COALESCE(district, ''), COALESCE(country, ''),
	COALESCE(website_url, ''), is_active, created_at, updated_at
FROM dealers`

type legacyDealer struct {
	ID                                int64
	Code, Name, Email, Phone, Address string
	City, District, Country, Website  string
	Active                            bool
	CreatedAt, UpdatedAt              sql.NullTime
}

// Run implements Step.
func (s OrganizationsStep) Run(ctx context.Context, src Sources, dst *Target, m *Mapper) (StepResult, error) {
	c := counts{}
	hub, err := src.Get(SourceHub)
	if err != nil {
		return StepResult{}, err
	}
	tree, err := resolveOlexTree(ctx, dst.Q, true, c)
	if err != nil {
		return StepResult{Counts: c}, err
	}

	query, args := dealersQuery, []any{}
	if dst.Mode == ModeDelta && !dst.Since.IsZero() {
		query += " WHERE COALESCE(updated_at, created_at) > ?"
		args = append(args, dst.Since)
	}
	rows, err := hub.Query(ctx, query+" ORDER BY id", args...)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	var dealers []legacyDealer
	for rows.Next() {
		var d legacyDealer
		if err := rows.Scan(&d.ID, &d.Code, &d.Name, &d.Email, &d.Phone, &d.Address, &d.City, &d.District,
			&d.Country, &d.Website, &d.Active, &d.CreatedAt, &d.UpdatedAt); err != nil {
			_ = rows.Close()
			return StepResult{Counts: c}, fmt.Errorf("scan dealer: %w", err)
		}
		dealers = append(dealers, d)
	}
	if err := rows.Close(); err != nil {
		return StepResult{Counts: c}, fmt.Errorf("read dealers: %w", err)
	}

	var watermark time.Time
	countries := map[string]int64{TRCountry: tree.CountryID}
	for _, d := range dealers {
		c.inc(cntRead)
		if ts := latest(d.CreatedAt, d.UpdatedAt); ts.After(watermark) {
			watermark = ts
		}
		if err := s.importDealer(ctx, dst.Q, m, tree, countries, d, c); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("dealer %d: %w", d.ID, err)
		}
	}
	return StepResult{Counts: c, Watermark: watermark}, nil
}

func (s OrganizationsStep) importDealer(ctx context.Context, q *db.Queries, m *Mapper, tree olexTree,
	countries map[string]int64, d legacyDealer, c counts,
) error {
	iso := strings.ToUpper(strings.TrimSpace(d.Country))
	if iso == "" {
		iso = TRCountry
	}
	countryID, ok := countries[iso]
	if !ok {
		country, err := q.GetCountryByISO2(ctx, iso)
		switch {
		case err == nil:
			countryID = country.ID
		case errors.Is(err, pgx.ErrNoRows):
			countryID = tree.CountryID
			c.inc("country_unmatched:" + iso)
		default:
			return fmt.Errorf("country %s: %w", iso, err)
		}
		countries[iso] = countryID
	}
	provinceID, districtID, err := matchAddress(ctx, q, countryID, d.City, d.District)
	if err != nil {
		return err
	}
	if (strings.TrimSpace(d.City) != "" && !provinceID.Valid) || (strings.TrimSpace(d.District) != "" && !districtID.Valid) {
		c.inc("geo_unmatched")
		c.inc("geo_unmatched:dealer:" + strconv.FormatInt(d.ID, 10))
	}

	ph := normalize.NormalizePhone(d.Phone, iso)
	phone, phoneRaw := "", pgtype.Text{}
	if ph.Verified {
		phone = ph.E164
	} else if ph.Raw != "" {
		phoneRaw = pgtype.Text{String: truncate(ph.Raw, 32), Valid: true}
		c.inc("phone_unverified")
	}
	status := "active"
	if !d.Active {
		status = "suspended"
	}
	name := truncate(strings.TrimSpace(d.Name), 200)
	email := truncate(normalize.NormalizeEmail(d.Email), 255)
	address := strings.TrimSpace(d.Address)
	city, district := truncate(strings.TrimSpace(d.City), 100), truncate(strings.TrimSpace(d.District), 100)
	website := truncate(strings.TrimSpace(d.Website), 512)

	key := Key{System: s.system(), Table: "dealers", ID: strconv.FormatInt(d.ID, 10), TargetTable: "organizations"}
	sum := Checksum(d.Code, d.Name, d.Email, d.Phone, d.Address, d.City, d.District, d.Country, d.Website, d.Active)
	res, err := m.Upsert(ctx, key, sum)
	if err != nil {
		return err
	}
	orgID, err := q.MigratorOrganizationIDByUUID(ctx, res.UUID)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read organization: %w", err)
	}

	if !exists {
		slugValue, err := dealerSlug(ctx, q, d)
		if err != nil {
			return err
		}
		_, err = q.MigratorInsertOrganization(ctx, db.MigratorInsertOrganizationParams{
			Uuid: res.UUID, Slug: slugValue, Name: name, Email: email, Phone: phone, PhoneRaw: phoneRaw,
			Address: address, City: city, District: district, Website: website, Status: status,
			Type: "dealer", ParentID: pgInt8(tree.DistributorID), BrandID: tree.BrandID,
			CountryID: pgInt8(countryID), ProvinceID: provinceID, DistrictID: districtID,
			CreatedAt: pgTime(d.CreatedAt),
		})
		if err != nil {
			return fmt.Errorf("insert organization: %w", err)
		}
		c.inc(cntCreated)
		return nil
	}
	if !res.Changed {
		c.inc(cntUnchanged)
		return nil
	}
	if err := q.MigratorUpdateOrganization(ctx, db.MigratorUpdateOrganizationParams{
		ID: orgID, Name: name, Email: email, Phone: phone, PhoneRaw: phoneRaw, Address: address,
		City: city, District: district, Website: website, Status: status,
		CountryID: pgInt8(countryID), ProvinceID: provinceID, DistrictID: districtID,
	}); err != nil {
		return fmt.Errorf("update organization: %w", err)
	}
	c.inc(cntUpdated)
	return nil
}

// matchAddress maps legacy city / district text to the geo tables.
func matchAddress(ctx context.Context, q *db.Queries, countryID int64, city, district string) (pgtype.Int8, pgtype.Int8, error) {
	var province, dist pgtype.Int8
	if strings.TrimSpace(city) == "" {
		return province, dist, nil
	}
	id, err := q.MigratorMatchProvince(ctx, db.MigratorMatchProvinceParams{CountryID: countryID, Name: city})
	if errors.Is(err, pgx.ErrNoRows) {
		return province, dist, nil
	}
	if err != nil {
		return province, dist, fmt.Errorf("match province: %w", err)
	}
	province = pgInt8(id)
	if strings.TrimSpace(district) == "" {
		return province, dist, nil
	}
	id, err = q.MigratorMatchDistrict(ctx, db.MigratorMatchDistrictParams{ProvinceID: province.Int64, Name: district})
	if errors.Is(err, pgx.ErrNoRows) {
		return province, dist, nil
	}
	if err != nil {
		return province, dist, fmt.Errorf("match district: %w", err)
	}
	return province, pgInt8(id), nil
}

// dealerSlug picks a free slug from the dealer name, suffixed with the
// dealer code (or legacy id) when the plain one is taken. It is computed once,
// on insert; later runs keep the stored slug.
func dealerSlug(ctx context.Context, q *db.Queries, d legacyDealer) (string, error) {
	base := strings.Trim(truncate(slug.FromName(d.Name), 48), "-")
	if base == "" || base == "item" {
		base = "bayi"
	}
	suffix := slug.FromName(d.Code)
	if strings.TrimSpace(d.Code) == "" || suffix == "item" {
		suffix = strconv.FormatInt(d.ID, 10)
	}
	candidates := []string{base, base + "-" + truncate(suffix, 15), base + "-h" + strconv.FormatInt(d.ID, 10)}
	for _, cand := range candidates {
		taken, err := q.SlugExists(ctx, cand)
		if err != nil {
			return "", fmt.Errorf("slug: %w", err)
		}
		if !taken {
			return cand, nil
		}
	}
	return "", fmt.Errorf("no free slug for %q", base)
}

func latest(ts ...sql.NullTime) time.Time {
	var out time.Time
	for _, t := range ts {
		if t.Valid && t.Time.After(out) {
			out = t.Time
		}
	}
	return out
}

func pgInt8(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: true} }

func pgTime(t sql.NullTime) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.Time, Valid: t.Valid}
}

// truncate cuts s to at most n runes (VARCHAR limits count characters).
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}
