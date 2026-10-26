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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgmerge"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/slug"
)

// Olex organization tree defaults (TEC-254, K1/K5; TEC-1003).
const (
	OlexBrandSlug = "olex"
	// TRCountry is the default country of legacy dealers, addresses and
	// phones.
	TRCountry = "TR"
	// TRDistributorSlug names the Türkiye distributor earlier migrator runs
	// created. Since TEC-1003 Türkiye has no distributor: the TR dealers hang
	// under the center, and a distributor left with this slug is merged into
	// the center.
	TRDistributorSlug = "olex-turkiye"
	// OlexServicePointDealerID is the legacy hub dealer that is the center's
	// own service point (AFM Florya, the same legal entity as the center):
	// it is merged into the center and lends the center its customer-facing
	// name and logo (TEC-1003).
	OlexServicePointDealerID = 1
	olexCenterSlug           = "olex-merkez"
	olexCenterName           = "Olex Merkez"
)

// mergeActor is org_merge_runs.actor of merges the migrator runs.
const mergeActor = "migrator"

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
	BrandID   int64
	CenterID  int64
	CountryID int64 // Türkiye
}

// resolveOlexTree finds the Olex center. With create it adds a missing
// center; without it a missing one is an error.
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
	return t, nil
}

// mergeIntoCenter merges an organization into the center (orgmerge) and
// adds the run to the step counts.
func mergeIntoCenter(ctx context.Context, dst *Target, tree olexTree, orgID int64, servicePoint bool, label string, c counts) error {
	keep := []orgmerge.Column{}
	if servicePoint {
		// The orders the service point bought from the center stay its
		// history (TEC-1003): the buyer keeps pointing at it.
		keep = append(keep, orgmerge.Column{Table: "orders", Column: "buyer_org_id"})
	}
	rep, err := orgmerge.Merge(ctx, dst.Tx, orgmerge.Options{
		SourceID: orgID, TargetID: tree.CenterID, Actor: mergeActor, Keep: keep, ServicePoint: servicePoint,
	})
	if err != nil {
		return fmt.Errorf("merge %s into the center: %w", label, err)
	}
	c.inc(label + "_merged")
	c.add(label+"_merge_moved", rep.Moved())
	for _, t := range rep.Tables {
		if t.Kept > 0 {
			c.add(label+"_merge_kept:"+t.String(), t.Kept)
		}
	}
	return nil
}

// mergeTRDistributor merges the Türkiye distributor of earlier runs into
// the center: its dealers, orders and territory move to the center.
func mergeTRDistributor(ctx context.Context, dst *Target, tree olexTree, c counts) error {
	id, err := dst.Q.MigratorDistributorBySlug(ctx, db.MigratorDistributorBySlugParams{Slug: TRDistributorSlug, BrandID: tree.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("TR distributor: %w", err)
	}
	return mergeIntoCenter(ctx, dst, tree, id, false, "distributor", c)
}

// mergeServicePoint merges the organization of the service point dealer
// (OlexServicePointDealerID) into the center once.
func (s OrganizationsStep) mergeServicePoint(ctx context.Context, dst *Target, m *Mapper, tree olexTree, c counts) error {
	if s.ServicePointDealerID == 0 {
		return nil
	}
	target, ok, err := m.LookupRaw(ctx, s.system(), "dealers", strconv.FormatInt(s.ServicePointDealerID, 10))
	if err != nil || !ok {
		return err
	}
	org, err := dst.Q.MigratorOrganizationStateByUUID(ctx, target)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("service point dealer: %w", err)
	}
	if org.DeletedAt.Valid || org.Type != "dealer" {
		return nil // merged by an earlier run
	}
	return mergeIntoCenter(ctx, dst, tree, org.ID, true, "service_point", c)
}

// OrganizationsStep imports hub dealers as Olex dealer organizations under
// the brand center, creating the center when it is missing (TEC-254). Since
// TEC-1003 it also merges the Türkiye distributor of earlier runs and the
// service point dealer into the center (idempotent: a merged organization
// is not merged again, and later steps resolve it to the center).
type OrganizationsStep struct {
	// System is the migration_map source system; empty means SourceHub.
	System string
	// ServicePointDealerID is the legacy hub dealer merged into the center
	// (0: none).
	ServicePointDealerID int64
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
	if err := mergeTRDistributor(ctx, dst, tree, c); err != nil {
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
	if err := s.mergeServicePoint(ctx, dst, m, tree, c); err != nil {
		return StepResult{Counts: c}, err
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
			Type: "dealer", ParentID: pgInt8(tree.CenterID), BrandID: tree.BrandID,
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
