package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrPublicNotFound: the public code is malformed, unknown or belongs to
// another brand. The three cases are indistinguishable to the caller.
var ErrPublicNotFound = errors.New("warranty: public code not found")

// publicCodeRe is the chk_warranties_public_code constraint of migration
// 000051 (new codes are 22 base64url characters). Anything else cannot be
// stored, so it is rejected without a database round trip.
var publicCodeRe = regexp.MustCompile(`^[A-Za-z0-9_-]{12,32}$`)

// ValidPublicCode reports whether code has the shape of a stored public code.
func ValidPublicCode(code string) bool { return publicCodeRe.MatchString(code) }

// PublicWarranty is the public page projection (TEC-189). It carries no
// personal data: no holder name, phone, e-mail or address, a masked plate
// and only the last four VIN characters.
type PublicWarranty struct {
	PublicCode    string              `json:"public_code"`
	Status        string              `json:"status"`
	StartAt       time.Time           `json:"start_at"`
	EndAt         time.Time           `json:"end_at"`
	DaysRemaining int                 `json:"days_remaining"`
	Product       PublicProduct       `json:"product"`
	Brand         PublicWarrantyBrand `json:"brand"`
	Dealer        PublicDealer        `json:"dealer"`
	Vehicle       PublicWarrantyCar   `json:"vehicle"`
}

// PublicProduct is the covered product.
type PublicProduct struct {
	Name string `json:"name"`
}

// PublicWarrantyBrand is the brand of the request domain (K3, K20).
type PublicWarrantyBrand struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// PublicDealer is the organization that performed the service.
type PublicDealer struct {
	Name string `json:"name"`
	City string `json:"city"`
}

// PublicWarrantyCar is the covered vehicle. BrandLogoUUID is set when the
// car brand has a logo (served at /brand-logos/{uuid}).
type PublicWarrantyCar struct {
	BrandName     string     `json:"brand_name"`
	BrandLogoUUID *uuid.UUID `json:"brand_logo_uuid"`
	ModelName     string     `json:"model_name"`
	ModelYear     *int       `json:"model_year"`
	PlateMasked   *string    `json:"plate_masked"`
	VINLast4      *string    `json:"vin_last4"`
}

// PublicLookup answers the public warranty page.
type PublicLookup struct {
	q   *db.Queries
	now func() time.Time
}

// NewPublicLookup builds the lookup over the generated queries.
func NewPublicLookup(q *db.Queries) *PublicLookup {
	return &PublicLookup{q: q, now: time.Now}
}

// WithClock replaces the clock (tests).
func (s *PublicLookup) WithClock(now func() time.Time) *PublicLookup {
	s.now = now
	return s
}

// Lookup returns the warranty with code inside brand. A malformed code is
// ErrPublicNotFound without touching the database.
func (s *PublicLookup) Lookup(ctx context.Context, brand PublicWarrantyBrand, brandID int64, code string) (PublicWarranty, error) {
	if !ValidPublicCode(code) {
		return PublicWarranty{}, ErrPublicNotFound
	}
	row, err := s.q.GetPublicWarrantyByCode(ctx, db.GetPublicWarrantyByCodeParams{PublicCode: code, BrandID: brandID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PublicWarranty{}, ErrPublicNotFound
		}
		return PublicWarranty{}, err
	}
	return BuildPublicWarranty(row, brand, s.now()), nil
}

// BuildPublicWarranty maps a row to the public projection at now. An active
// warranty past its end (the expiry cron has not run yet) reads expired.
func BuildPublicWarranty(row db.GetPublicWarrantyByCodeRow, brand PublicWarrantyBrand, now time.Time) PublicWarranty {
	end := row.EndAt.Time
	status := row.Status
	if status == "active" && !now.Before(end) {
		status = "expired"
	}
	days := 0
	if status == "active" {
		days = int(math.Ceil(end.Sub(now).Hours() / 24))
	}
	city := strings.TrimSpace(row.OrganizationProvince)
	if city == "" {
		city = strings.TrimSpace(row.OrganizationCity)
	}
	car := PublicWarrantyCar{BrandName: row.CarBrandName, ModelName: row.CarModelName}
	if row.CarBrandHasLogo {
		id := row.CarBrandUuid
		car.BrandLogoUUID = &id
	}
	if y := firstInt2(row.ServiceModelYear, row.VehicleModelYear); y.Valid {
		v := int(y.Int16)
		car.ModelYear = &v
	}
	plate, country := row.ServicePlate, row.ServicePlateCountry
	if !plate.Valid || strings.TrimSpace(plate.String) == "" {
		plate, country = row.VehiclePlate, row.VehiclePlateCountry
	}
	if plate.Valid {
		if m := geo.MaskPlate(country.String, plate.String); m != "" {
			car.PlateMasked = &m
		}
	}
	if vin := firstText(row.ServiceVin, row.VehicleVin); vin.Valid {
		if l4 := geo.VINLast4(vin.String); l4 != "" {
			car.VINLast4 = &l4
		}
	}
	return PublicWarranty{
		PublicCode:    row.PublicCode,
		Status:        status,
		StartAt:       row.StartAt.Time,
		EndAt:         end,
		DaysRemaining: days,
		Product:       PublicProduct{Name: row.ProductName},
		Brand:         brand,
		Dealer:        PublicDealer{Name: row.OrganizationName, City: city},
		Vehicle:       car,
	}
}

func firstInt2(a, b pgtype.Int2) pgtype.Int2 {
	if a.Valid {
		return a
	}
	return b
}

func firstText(a, b pgtype.Text) pgtype.Text {
	if a.Valid && strings.TrimSpace(a.String) != "" {
		return a
	}
	return b
}

// piiKeys are JSON keys that would carry personal data. The public
// projection never has them; FindPIIKeys lets tests prove it on the wire.
var piiKeys = map[string]bool{
	"surname": true, "first_name": true, "last_name": true, "full_name": true,
	"customer": true, "customer_name": true, "customer_uuid": true,
	"holder": true, "holder_name": true, "holder_user_id": true, "holder_uuid": true,
	"owner": true, "owner_name": true, "user": true, "user_uuid": true, "user_id": true,
	"phone": true, "phone_e164": true, "phone_national": true, "email": true,
	"address": true, "national_id": true, "tax_no": true, "company_name": true,
	"plate": true, "vin": true,
}

// FindPIIKeys returns the personal data keys found anywhere in a JSON
// document (nil when there are none or the document is not JSON).
func FindPIIKeys(doc []byte) []string {
	var v any
	if err := json.Unmarshal(doc, &v); err != nil {
		return nil
	}
	var found []string
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case map[string]any:
			for k, child := range t {
				if piiKeys[strings.ToLower(k)] {
					found = append(found, k)
				}
				walk(child)
			}
		case []any:
			for _, child := range t {
				walk(child)
			}
		}
	}
	walk(v)
	return found
}
