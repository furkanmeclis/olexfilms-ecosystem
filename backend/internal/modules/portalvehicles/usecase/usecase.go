// Package usecase serves the customer portal "my vehicles" reads (TEC-238,
// F2-03a): GET /v1/portal/vehicles, GET /v1/portal/vehicles/{uuid} and
// GET /v1/portal/services.
//
// Scope: every record belongs to the signed-in portal user (vehicle owner,
// warranty holder; a service is the user's when the user is its customer
// or holds one of its warranties, the TEC-239 rule) and to the domain
// brand (K20); Glorian
// rows never come back (K1/K2). Draft services are dealer-internal and stay
// out. Measurement data (has_measurement, measurement results) is not part
// of any portal view: the views below simply have no such field.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxVehicleServices caps the service list embedded in the vehicle detail;
// the full history is paged through GET /v1/portal/services.
const MaxVehicleServices = 50

// ErrVehicleNotFound answers a vehicle that is not the user's (404).
var ErrVehicleNotFound = errors.New("portal: vehicle not found")

// Store is the subset of db.Queries the use case reads.
type Store interface {
	ListPortalVehicles(ctx context.Context, arg db.ListPortalVehiclesParams) ([]db.ListPortalVehiclesRow, error)
	CountPortalVehicles(ctx context.Context, arg db.CountPortalVehiclesParams) (int64, error)
	GetPortalVehicle(ctx context.Context, arg db.GetPortalVehicleParams) (db.GetPortalVehicleRow, error)
	GetPortalVehicleServiceSummary(ctx context.Context, arg db.GetPortalVehicleServiceSummaryParams) (db.GetPortalVehicleServiceSummaryRow, error)
	ListPortalVehicleActiveWarranties(ctx context.Context, arg db.ListPortalVehicleActiveWarrantiesParams) ([]db.ListPortalVehicleActiveWarrantiesRow, error)
	ListPortalServices(ctx context.Context, arg db.ListPortalServicesParams) ([]db.ListPortalServicesRow, error)
	CountPortalServices(ctx context.Context, arg db.CountPortalServicesParams) (int64, error)
	ListPortalContracts(ctx context.Context, arg db.ListPortalContractsParams) ([]db.ListPortalContractsRow, error)
	CountPortalContracts(ctx context.Context, arg db.CountPortalContractsParams) (int64, error)
}

// Caller is the portal session: domain brand and user (internal ids).
type Caller struct {
	BrandID int64
	UserID  int64
}

func (c Caller) valid() bool { return c.BrandID > 0 && c.UserID > 0 }

// Page is a limit / offset window.
type Page struct {
	Limit  int32
	Offset int32
}

// NamedRef names a car brand or model.
type NamedRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// OrganizationRef is the organization that performed a service.
type OrganizationRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Type string    `json:"type"`
}

// VehicleView is one vehicle of the portal user.
type VehicleView struct {
	UUID                uuid.UUID  `json:"uuid"`
	CarBrand            *NamedRef  `json:"car_brand"`
	CarModel            *NamedRef  `json:"car_model"`
	ModelYear           *int16     `json:"model_year"`
	Plate               *string    `json:"plate"`
	PlateCountry        *string    `json:"plate_country"`
	VIN                 *string    `json:"vin"`
	ServiceCount        int64      `json:"service_count"`
	ActiveWarrantyCount int64      `json:"active_warranty_count"`
	LastServiceAt       *time.Time `json:"last_service_at"`
	CreatedAt           time.Time  `json:"created_at"`
}

// ServiceSummary counts the vehicle's services across organizations.
type ServiceSummary struct {
	Total             int64      `json:"total"`
	Completed         int64      `json:"completed"`
	OrganizationCount int64      `json:"organization_count"`
	LastServiceAt     *time.Time `json:"last_service_at"`
}

// ServiceView is one service of the portal user (no measurement data).
type ServiceView struct {
	UUID         uuid.UUID       `json:"uuid"`
	ServiceNo    string          `json:"service_no"`
	Status       string          `json:"status"`
	Package      *string         `json:"package"`
	Organization OrganizationRef `json:"organization"`
	VehicleUUID  uuid.UUID       `json:"vehicle_uuid"`
	CarBrandName string          `json:"car_brand_name"`
	CarModelName string          `json:"car_model_name"`
	ModelYear    *int16          `json:"model_year"`
	Plate        *string         `json:"plate"`
	PlateCountry *string         `json:"plate_country"`
	CompletedAt  *time.Time      `json:"completed_at"`
	CreatedAt    time.Time       `json:"created_at"`
}

// ProductRef is the product a warranty covers.
type ProductRef struct {
	UUID uuid.UUID `json:"uuid"`
	SKU  string    `json:"sku"`
	Name string    `json:"name"`
}

// ServiceRef is the service that issued a warranty.
type ServiceRef struct {
	UUID      uuid.UUID `json:"uuid"`
	ServiceNo string    `json:"service_no"`
}

// ActiveWarrantyView is an active warranty with the time left.
type ActiveWarrantyView struct {
	UUID         uuid.UUID       `json:"uuid"`
	PublicCode   string          `json:"public_code"`
	StartAt      time.Time       `json:"start_at"`
	EndAt        time.Time       `json:"end_at"`
	DaysLeft     int             `json:"days_left"`
	PercentLeft  int             `json:"percent_left"`
	Product      ProductRef      `json:"product"`
	Service      ServiceRef      `json:"service"`
	Organization OrganizationRef `json:"organization"`
}

// VehicleDetailView is the vehicle with its services across organizations
// and its active warranties.
type VehicleDetailView struct {
	VehicleView
	ServiceSummary   ServiceSummary       `json:"service_summary"`
	Services         []ServiceView        `json:"services"`
	ActiveWarranties []ActiveWarrantyView `json:"active_warranties"`
}

// Service is the portal vehicles use case.
type Service struct {
	q   Store
	now func() time.Time
}

// New creates the use case.
func New(q Store) *Service { return &Service{q: q, now: time.Now} }

// WithClock replaces the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := strings.TrimSpace(t.String)
	if v == "" {
		return nil
	}
	return &v
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func yearPtr(y pgtype.Int2) *int16 {
	if !y.Valid {
		return nil
	}
	v := y.Int16
	return &v
}

func namedRef(id pgtype.UUID, name pgtype.Text) *NamedRef {
	if !id.Valid {
		return nil
	}
	return &NamedRef{UUID: uuid.UUID(id.Bytes), Name: name.String}
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

// TimeLeft returns the whole days left (rounded up, never negative) and the
// share of the warranty period still left in percent (0-100).
func TimeLeft(start, end, now time.Time) (days, percent int) {
	left := end.Sub(now)
	if left <= 0 {
		return 0, 0
	}
	days = int(math.Ceil(left.Hours() / 24))
	total := end.Sub(start)
	if total <= 0 {
		return days, 0
	}
	percent = int(math.Round(float64(left) / float64(total) * 100))
	if percent > 100 {
		percent = 100
	}
	return days, percent
}

// ListVehicles returns one page of the user's vehicles.
func (s *Service) ListVehicles(ctx context.Context, c Caller, p Page) ([]VehicleView, int64, error) {
	if !c.valid() {
		return []VehicleView{}, 0, nil
	}
	rows, err := s.q.ListPortalVehicles(ctx, db.ListPortalVehiclesParams{
		Now: ts(s.now()), UserID: c.UserID, BrandID: c.BrandID, RowLimit: p.Limit, RowOffset: p.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("portal: vehicles: %w", err)
	}
	total, err := s.q.CountPortalVehicles(ctx, db.CountPortalVehiclesParams{UserID: c.UserID, BrandID: c.BrandID})
	if err != nil {
		return nil, 0, fmt.Errorf("portal: count vehicles: %w", err)
	}
	out := make([]VehicleView, 0, len(rows))
	for _, r := range rows {
		out = append(out, VehicleView{
			UUID: r.Uuid, CarBrand: namedRef(r.CarBrandUuid, r.CarBrandName), CarModel: namedRef(r.CarModelUuid, r.CarModelName),
			ModelYear: yearPtr(r.ModelYear), Plate: textPtr(r.Plate), PlateCountry: textPtr(r.PlateCountry), VIN: textPtr(r.Vin),
			ServiceCount: r.ServiceCount, ActiveWarrantyCount: r.ActiveWarrantyCount,
			LastServiceAt: timePtr(r.LastServiceAt), CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, total, nil
}

// GetVehicle returns one vehicle of the user with its services across
// organizations and its active warranties; any other vehicle is 404.
func (s *Service) GetVehicle(ctx context.Context, c Caller, id uuid.UUID) (VehicleDetailView, error) {
	if !c.valid() {
		return VehicleDetailView{}, ErrVehicleNotFound
	}
	v, err := s.q.GetPortalVehicle(ctx, db.GetPortalVehicleParams{Uuid: id, UserID: c.UserID, BrandID: c.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return VehicleDetailView{}, ErrVehicleNotFound
	}
	if err != nil {
		return VehicleDetailView{}, fmt.Errorf("portal: vehicle: %w", err)
	}
	now := s.now()
	sum, err := s.q.GetPortalVehicleServiceSummary(ctx, db.GetPortalVehicleServiceSummaryParams{
		VehicleID: v.ID, UserID: c.UserID, BrandID: c.BrandID,
	})
	if err != nil {
		return VehicleDetailView{}, fmt.Errorf("portal: vehicle summary: %w", err)
	}
	svcRows, err := s.q.ListPortalServices(ctx, db.ListPortalServicesParams{
		UserID: c.UserID, BrandID: c.BrandID, VehicleID: pgtype.Int8{Int64: v.ID, Valid: true},
		RowLimit: MaxVehicleServices,
	})
	if err != nil {
		return VehicleDetailView{}, fmt.Errorf("portal: vehicle services: %w", err)
	}
	wRows, err := s.q.ListPortalVehicleActiveWarranties(ctx, db.ListPortalVehicleActiveWarrantiesParams{
		VehicleID: v.ID, UserID: c.UserID, BrandID: c.BrandID, Now: ts(now),
	})
	if err != nil {
		return VehicleDetailView{}, fmt.Errorf("portal: vehicle warranties: %w", err)
	}
	out := VehicleDetailView{
		VehicleView: VehicleView{
			UUID: v.Uuid, CarBrand: namedRef(v.CarBrandUuid, v.CarBrandName), CarModel: namedRef(v.CarModelUuid, v.CarModelName),
			ModelYear: yearPtr(v.ModelYear), Plate: textPtr(v.Plate), PlateCountry: textPtr(v.PlateCountry), VIN: textPtr(v.Vin),
			ServiceCount: sum.Total, ActiveWarrantyCount: int64(len(wRows)),
			LastServiceAt: timePtr(sum.LastServiceAt), CreatedAt: v.CreatedAt.Time,
		},
		ServiceSummary: ServiceSummary{
			Total: sum.Total, Completed: sum.Completed, OrganizationCount: sum.OrganizationCount,
			LastServiceAt: timePtr(sum.LastServiceAt),
		},
		Services:         serviceViews(svcRows),
		ActiveWarranties: make([]ActiveWarrantyView, 0, len(wRows)),
	}
	for _, w := range wRows {
		days, pct := TimeLeft(w.StartAt.Time, w.EndAt.Time, now)
		out.ActiveWarranties = append(out.ActiveWarranties, ActiveWarrantyView{
			UUID: w.Uuid, PublicCode: w.PublicCode, StartAt: w.StartAt.Time, EndAt: w.EndAt.Time,
			DaysLeft: days, PercentLeft: pct,
			Product:      ProductRef{UUID: w.ProductUuid, SKU: w.ProductSku, Name: w.ProductName},
			Service:      ServiceRef{UUID: w.ServiceUuid, ServiceNo: w.ServiceNo},
			Organization: OrganizationRef{UUID: w.OrganizationUuid, Name: w.OrganizationName, Type: w.OrganizationType},
		})
	}
	return out, nil
}

// ListServices returns one page of the user's services across every
// organization of the brand.
func (s *Service) ListServices(ctx context.Context, c Caller, p Page) ([]ServiceView, int64, error) {
	if !c.valid() {
		return []ServiceView{}, 0, nil
	}
	rows, err := s.q.ListPortalServices(ctx, db.ListPortalServicesParams{
		UserID: c.UserID, BrandID: c.BrandID, RowLimit: p.Limit, RowOffset: p.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("portal: services: %w", err)
	}
	total, err := s.q.CountPortalServices(ctx, db.CountPortalServicesParams{UserID: c.UserID, BrandID: c.BrandID})
	if err != nil {
		return nil, 0, fmt.Errorf("portal: count services: %w", err)
	}
	return serviceViews(rows), total, nil
}

func serviceViews(rows []db.ListPortalServicesRow) []ServiceView {
	out := make([]ServiceView, 0, len(rows))
	for _, r := range rows {
		out = append(out, ServiceView{
			UUID: r.Uuid, ServiceNo: r.ServiceNo, Status: r.Status, Package: textPtr(r.Package),
			Organization: OrganizationRef{UUID: r.OrganizationUuid, Name: r.OrganizationName, Type: r.OrganizationType},
			VehicleUUID:  r.VehicleUuid, CarBrandName: r.CarBrandName, CarModelName: r.CarModelName,
			ModelYear: yearPtr(r.ModelYear), Plate: textPtr(r.Plate), PlateCountry: textPtr(r.PlateCountry),
			CompletedAt: timePtr(r.CompletedAt), CreatedAt: r.CreatedAt.Time,
		})
	}
	return out
}

// ContractView is one executed vehicle intake contract of the portal user
// (TEC-288). The legacy service field stays for TEC-245 clients; F3 adds the
// contract identity, execution time and PDF readiness.
type ContractView struct {
	ContractUUID uuid.UUID       `json:"contract_uuid"`
	ContractNo   int64           `json:"contract_no"`
	Service      ServiceRef      `json:"service"`
	Status       string          `json:"status"`
	Organization OrganizationRef `json:"organization"`
	VehicleUUID  uuid.UUID       `json:"vehicle_uuid"`
	CarBrandName string          `json:"car_brand_name"`
	CarModelName string          `json:"car_model_name"`
	ModelYear    *int16          `json:"model_year"`
	Plate        *string         `json:"plate"`
	PlateCountry *string         `json:"plate_country"`
	ExecutedAt   *time.Time      `json:"executed_at"`
	PDFReady     bool            `json:"pdf_ready"`
	CreatedAt    time.Time       `json:"created_at"`
}

// ListContracts returns one page of the user's signed vehicle intake
// contracts (an empty list until F3 records contracts).
func (s *Service) ListContracts(ctx context.Context, c Caller, p Page) ([]ContractView, int64, error) {
	if !c.valid() {
		return []ContractView{}, 0, nil
	}
	rows, err := s.q.ListPortalContracts(ctx, db.ListPortalContractsParams{
		UserID: c.UserID, BrandID: c.BrandID, RowLimit: p.Limit, RowOffset: p.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("portal: contracts: %w", err)
	}
	total, err := s.q.CountPortalContracts(ctx, db.CountPortalContractsParams{UserID: c.UserID, BrandID: c.BrandID})
	if err != nil {
		return nil, 0, fmt.Errorf("portal: count contracts: %w", err)
	}
	out := make([]ContractView, 0, len(rows))
	for _, r := range rows {
		out = append(out, ContractView{
			ContractUUID: r.ContractUuid, ContractNo: r.ContractNo,
			Service: ServiceRef{UUID: r.Uuid, ServiceNo: r.ServiceNo}, Status: r.Status,
			Organization: OrganizationRef{UUID: r.OrganizationUuid, Name: r.OrganizationName, Type: r.OrganizationType},
			VehicleUUID:  r.VehicleUuid, CarBrandName: r.CarBrandName, CarModelName: r.CarModelName,
			ModelYear: yearPtr(r.ModelYear), Plate: textPtr(r.Plate), PlateCountry: textPtr(r.PlateCountry),
			ExecutedAt: timePtr(r.ExecutedAt), PDFReady: r.PdfReady,
			CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, total, nil
}
