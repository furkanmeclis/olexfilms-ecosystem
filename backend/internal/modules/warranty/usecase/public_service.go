package usecase

// TEC-1005: the public warranty page is one page per service,
// /warranty/{service_no} (the legacy hub's page and the QR of the printed
// warranty cards), with every warranty of the service. A warranty public
// code (the per-warranty links before TEC-1005) or a legacy alias resolves
// to its service, so old links keep working; the page then moves to the
// service number. Like PublicWarranty it carries no personal data.

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// PublicService is the public page projection of a service.
type PublicService struct {
	// ServiceNo is the canonical address of the page (/warranty/{no}).
	ServiceNo   string                  `json:"service_no"`
	Package     *string                 `json:"package"`
	ServiceDate time.Time               `json:"service_date"`
	Brand       PublicWarrantyBrand     `json:"brand"`
	Dealer      PublicDealer            `json:"dealer"`
	Vehicle     PublicWarrantyCar       `json:"vehicle"`
	Warranties  []PublicServiceWarranty `json:"warranties"`
}

// PublicServiceWarranty is one warranty of the public service page.
type PublicServiceWarranty struct {
	PublicCode    string        `json:"public_code"`
	Status        string        `json:"status"`
	StartAt       time.Time     `json:"start_at"`
	EndAt         time.Time     `json:"end_at"`
	DaysRemaining int           `json:"days_remaining"`
	Product       PublicProduct `json:"product"`
	Category      string        `json:"category"`
	// Parts are the applied part keys (services.parts.<key>).
	Parts []string `json:"parts"`
}

// HasActive reports whether any warranty of the service is active (the
// PDF is offered only then).
func (p PublicService) HasActive() bool {
	for _, w := range p.Warranties {
		if w.Status == "active" {
			return true
		}
	}
	return false
}

// LookupService resolves a service number, warranty public code or legacy
// alias inside brand to its public service page. A malformed code is
// ErrPublicNotFound without a database round trip.
func (s *PublicLookup) LookupService(ctx context.Context, brand PublicWarrantyBrand, brandID int64, code string) (PublicService, error) {
	if !ValidPublicCode(code) {
		return PublicService{}, ErrPublicNotFound
	}
	id, err := s.q.ResolvePublicServiceID(ctx, db.ResolvePublicServiceIDParams{Code: code, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicService{}, ErrPublicNotFound
	}
	if err != nil {
		return PublicService{}, err
	}
	head, err := s.q.GetPublicService(ctx, db.GetPublicServiceParams{ServiceID: id, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicService{}, ErrPublicNotFound
	}
	if err != nil {
		return PublicService{}, err
	}
	rows, err := s.q.ListPublicServiceWarranties(ctx, db.ListPublicServiceWarrantiesParams{ServiceID: id, BrandID: brandID})
	if err != nil {
		return PublicService{}, err
	}
	return BuildPublicService(head, rows, brand, s.now()), nil
}

// BuildPublicService maps the rows to the public projection at now.
func BuildPublicService(head db.GetPublicServiceRow, rows []db.ListPublicServiceWarrantiesRow, brand PublicWarrantyBrand, now time.Time) PublicService {
	city := strings.TrimSpace(head.OrganizationProvince)
	if city == "" {
		city = strings.TrimSpace(head.OrganizationCity)
	}
	date := head.CreatedAt.Time
	if head.CompletedAt.Valid {
		date = head.CompletedAt.Time
	}
	out := PublicService{
		ServiceNo:   head.ServiceNo,
		ServiceDate: date,
		Brand:       brand,
		Dealer:      PublicDealer{Name: head.OrganizationName, City: city},
		Vehicle: publicCar(head.CarBrandName, head.CarModelName, head.CarBrandUuid, head.CarBrandHasLogo,
			firstInt2(head.ServiceModelYear, head.VehicleModelYear),
			head.ServicePlate, head.ServicePlateCountry, head.VehiclePlate, head.VehiclePlateCountry,
			firstText(head.ServiceVin, head.VehicleVin)),
		Warranties: make([]PublicServiceWarranty, 0, len(rows)),
	}
	if p := strings.TrimSpace(head.Package.String); head.Package.Valid && p != "" {
		out.Package = &p
	}
	for _, r := range rows {
		end := r.EndAt.Time
		status := r.Status
		if status == "active" && !now.Before(end) {
			status = "expired"
		}
		days := 0
		if status == "active" {
			days = int(math.Ceil(end.Sub(now).Hours() / 24))
		}
		parts := []string{}
		_ = json.Unmarshal(r.AppliedParts, &parts)
		if parts == nil {
			parts = []string{}
		}
		out.Warranties = append(out.Warranties, PublicServiceWarranty{
			PublicCode: r.PublicCode, Status: status, StartAt: r.StartAt.Time, EndAt: end, DaysRemaining: days,
			Product: PublicProduct{Name: r.ProductName}, Category: r.CategoryName, Parts: parts,
		})
	}
	return out
}

// publicCar is the masked vehicle: the service's plate snapshot, else the
// vehicle's; only the last four VIN characters.
func publicCar(brandName, modelName string, brandUUID uuid.UUID, hasLogo bool, year pgtype.Int2,
	plate, country, vehiclePlate, vehicleCountry, vin pgtype.Text) PublicWarrantyCar {
	car := PublicWarrantyCar{BrandName: brandName, ModelName: modelName}
	if hasLogo {
		id := brandUUID
		car.BrandLogoUUID = &id
	}
	if year.Valid {
		v := int(year.Int16)
		car.ModelYear = &v
	}
	if !plate.Valid || strings.TrimSpace(plate.String) == "" {
		plate, country = vehiclePlate, vehicleCountry
	}
	if plate.Valid {
		if m := geo.MaskPlate(country.String, plate.String); m != "" {
			car.PlateMasked = &m
		}
	}
	if vin.Valid {
		if l4 := geo.VINLast4(vin.String); l4 != "" {
			car.VINLast4 = &l4
		}
	}
	return car
}
