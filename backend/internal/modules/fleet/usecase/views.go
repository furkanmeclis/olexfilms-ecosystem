package usecase

import (
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ProfileView is the fleet profile.
type ProfileView struct {
	TaxNumber       string  `json:"tax_number"`
	TaxOffice       *string `json:"tax_office"`
	LegalName       string  `json:"legal_name"`
	ContactName     *string `json:"contact_name"`
	ContactPhone    *string `json:"contact_phone"`
	BillingEmail    *string `json:"billing_email"`
	ReportFrequency string  `json:"report_frequency"`
	ReportLocale    string  `json:"report_locale"`
}

// LinkView is a fleet-dealer link.
type LinkView struct {
	UUID       uuid.UUID  `json:"uuid"`
	Status     string     `json:"status"`
	DealerUUID uuid.UUID  `json:"dealer_uuid"`
	DealerName string     `json:"dealer_name"`
	StartedAt  *time.Time `json:"started_at"`
	EndedAt    *time.Time `json:"ended_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// FleetView is the answer of POST /v1/fleets (and of a link request).
type FleetView struct {
	UUID      uuid.UUID   `json:"uuid"`
	Name      string      `json:"name"`
	Status    string      `json:"status"`
	Profile   ProfileView `json:"profile"`
	Link      *LinkView   `json:"link"`
	CreatedAt time.Time   `json:"created_at"`
}

// FleetListItem is one row of GET /v1/fleets (one per link).
type FleetListItem struct {
	UUID          uuid.UUID  `json:"uuid"`
	Name          string     `json:"name"`
	LegalName     string     `json:"legal_name"`
	TaxNumber     string     `json:"tax_number"`
	Status        string     `json:"status"`
	VehicleCount  int64      `json:"vehicle_count"`
	LastServiceAt *time.Time `json:"last_service_at"`
	Link          LinkView   `json:"link"`
}

// FleetUserView is a fleet user.
type FleetUserView struct {
	UUID        uuid.UUID  `json:"uuid"`
	UserUUID    uuid.UUID  `json:"user_uuid"`
	Email       *string    `json:"email"`
	Name        string     `json:"name"`
	Surname     string     `json:"surname"`
	IsPrimary   bool       `json:"is_primary"`
	Status      string     `json:"status"`
	LastLoginAt *time.Time `json:"last_login_at"`
	DisabledAt  *time.Time `json:"disabled_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// CatalogRef is a car brand or model.
type CatalogRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// VehicleView is a fleet vehicle. last_service_at and
// active_warranty_count count only the caller's own work (a dealer sees its
// services and warranties; the center all).
type VehicleView struct {
	UUID                uuid.UUID   `json:"uuid"`
	Plate               *string     `json:"plate"`
	PlateCountry        *string     `json:"plate_country"`
	VIN                 *string     `json:"vin"`
	ModelYear           *int        `json:"model_year"`
	CarBrand            *CatalogRef `json:"car_brand"`
	CarModel            *CatalogRef `json:"car_model"`
	LastServiceAt       *time.Time  `json:"last_service_at"`
	ActiveWarrantyCount int64       `json:"active_warranty_count"`
	CreatedAt           time.Time   `json:"created_at"`
}

func profileView(p db.FleetProfile) ProfileView {
	return ProfileView{
		TaxNumber: p.TaxNumber, TaxOffice: textPtr(p.TaxOffice), LegalName: p.LegalName,
		ContactName: textPtr(p.ContactName), ContactPhone: textPtr(p.ContactPhone),
		BillingEmail: textPtr(p.BillingEmail), ReportFrequency: p.ReportFrequency, ReportLocale: p.ReportLocale,
	}
}

func linkView(l db.FleetDealerLink, dealer db.Organization) LinkView {
	return LinkView{
		UUID: l.Uuid, Status: l.Status, DealerUUID: dealer.Uuid, DealerName: dealer.Name,
		StartedAt: timePtr(l.StartedAt), EndedAt: timePtr(l.EndedAt), CreatedAt: l.CreatedAt.Time,
	}
}

func fleetView(o db.Organization, p db.FleetProfile, link *LinkView) FleetView {
	return FleetView{
		UUID: o.Uuid, Name: o.Name, Status: o.Status, Profile: profileView(p), Link: link,
		CreatedAt: o.CreatedAt.Time,
	}
}

func vehicleRowView(r db.ListFleetVehiclesRow) VehicleView {
	v := VehicleView{
		UUID: r.Uuid, Plate: textPtr(r.Plate), PlateCountry: textPtr(r.PlateCountry), VIN: textPtr(r.Vin),
		LastServiceAt: timePtr(r.LastServiceAt), ActiveWarrantyCount: r.ActiveWarrantyCount,
		CreatedAt: r.CreatedAt.Time,
	}
	if r.ModelYear.Valid {
		y := int(r.ModelYear.Int16)
		v.ModelYear = &y
	}
	if r.CarBrandUuid.Valid {
		v.CarBrand = &CatalogRef{UUID: r.CarBrandUuid.Bytes, Name: r.CarBrandName.String}
	}
	if r.CarModelUuid.Valid {
		v.CarModel = &CatalogRef{UUID: r.CarModelUuid.Bytes, Name: r.CarModelName.String}
	}
	return v
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
