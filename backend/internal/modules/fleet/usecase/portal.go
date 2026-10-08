package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-474 (F5-02c): the fleet portal reads (/v1/portal/fleet/*). A fleet
// user sees only their own fleet (fleet_users: one fleet per user). The
// data comes from the dealers the fleet worked with (an active or ended
// link) whose fleet module is on; a dealer that turns the module off drops
// out (its history stays in the database). The portal opens while at least
// one actively linked dealer has the module (else 403 FEATURE_DISABLED); the
// link decision (/links) stays open regardless.

// ModuleChecker resolves the fleet module of a dealer (features.Service).
type ModuleChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

// ReportFiles reads a stored fleet report PDF (storage driver).
type ReportFiles interface {
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
}

// SetModules sets the module resolver of the portal reads (nil: every
// linked dealer counts as enabled).
func (s *Service) SetModules(m ModuleChecker) { s.modules = m }

// SetReportFiles sets the storage the report downloads read from.
func (s *Service) SetReportFiles(f ReportFiles) { s.files = f }

// ErrPortalClosed: no actively linked dealer has the fleet module (403).
var ErrPortalClosed = &RuleError{
	Code: response.CodeFeatureDisabled, Message: "no linked dealer has the fleet module enabled",
	status: http.StatusForbidden,
}

// PortalCaller is the portal session: the domain brand and the user.
type PortalCaller struct {
	UserID  int64
	BrandID int64
}

// Portal list sort contracts (docs/list-contract.md).
var (
	PortalVehicleSort = apiquery.SortSpec{
		Columns: apiquery.SortColumns{"plate": "plate", "last_service_at": "last_service_at", "warranty_until": "warranty_until"},
		Default: apiquery.SortField{Field: "plate"},
	}
	PortalServiceSort = apiquery.SortSpec{
		Columns: apiquery.SortColumns{
			"created_at": "created_at", "completed_at": "completed_at", "service_no": "service_no", "status": "status",
		},
		Default: apiquery.SortField{Field: "created_at", Desc: true},
	}
	PortalWarrantySort = apiquery.SortSpec{
		Columns: apiquery.SortColumns{"end_at": "end_at", "start_at": "start_at"},
		Default: apiquery.SortField{Field: "end_at"},
	}
	PortalReportSort = apiquery.SortSpec{
		Columns: apiquery.SortColumns{"period_start": "period_start"},
		Default: apiquery.SortField{Field: "period_start", Desc: true},
	}
)

// Portal filter values.
var (
	// PortalServiceStatuses are the service statuses the portal shows
	// (drafts are dealer internal).
	PortalServiceStatuses = []string{"pending", "processing", "ready", "completed", "cancelled"}
	// PortalWarrantyStates are the effective warranty states.
	PortalWarrantyStates = []string{"active", "expired", "void"}
)

// maxPortalVehicleServices caps the history embedded in the vehicle
// detail; the full list pages through GET /v1/portal/fleet/services.
const maxPortalVehicleServices = 100

// portalDealer is one dealer the portal shows.
type portalDealer struct {
	id         int64
	uuid       uuid.UUID
	name       string
	linkStatus string // latest link: active | ended
	startedAt  pgtype.Timestamptz
	cariID     pgtype.Int8
}

// portalScope is the signed-in user's fleet and its visible dealers.
type portalScope struct {
	fleet   db.Organization
	profile db.FleetProfile
	dealers []portalDealer
	ids     []int64
	pending int64
}

func (p portalScope) dealer(id int64) (portalDealer, bool) {
	for _, d := range p.dealers {
		if d.id == id {
			return d, true
		}
	}
	return portalDealer{}, false
}

// resolvePortal loads the user's fleet (ErrNotFound: none, or another
// brand) and its visible dealers (ErrPortalClosed: no active one).
func (s *Service) resolvePortal(ctx context.Context, q *db.Queries, c PortalCaller) (portalScope, error) {
	fu, err := portalFleet(ctx, q, c.UserID)
	if err != nil {
		return portalScope{}, err
	}
	f, err := q.GetFleetByUUID(ctx, fu.FleetUuid)
	if errors.Is(err, pgx.ErrNoRows) {
		return portalScope{}, ErrNotFound
	}
	if err != nil {
		return portalScope{}, fmt.Errorf("fleet: portal fleet: %w", err)
	}
	if c.BrandID != 0 && f.Organization.BrandID != c.BrandID {
		return portalScope{}, ErrNotFound
	}
	out := portalScope{fleet: f.Organization, profile: f.FleetProfile}
	dealers, open, pending, err := s.visibleDealers(ctx, q, f.Organization.ID)
	if err != nil {
		return portalScope{}, err
	}
	if !open {
		return portalScope{}, ErrPortalClosed
	}
	out.dealers, out.pending = dealers, pending
	out.ids = make([]int64, 0, len(out.dealers))
	for _, d := range out.dealers {
		out.ids = append(out.ids, d.id)
	}
	return out, nil
}

// visibleDealers lists the dealers whose data a fleet sees (the portal and
// the periodic report): every dealer the fleet worked with (a link that was
// active once) whose fleet module is on, sorted by name. open reports an
// active link among them; pending counts the pending link requests.
func (s *Service) visibleDealers(ctx context.Context, q *db.Queries, fleetOrgID int64) ([]portalDealer, bool, int64, error) {
	links, err := q.ListFleetDealerLinks(ctx, db.ListFleetDealerLinksParams{FleetOrgID: fleetOrgID})
	if err != nil {
		return nil, false, 0, fmt.Errorf("fleet: portal links: %w", err)
	}
	var (
		dealers []portalDealer
		pending int64
	)
	enabled := map[int64]bool{}
	seen := map[int64]int{}
	open := false
	// Newest link first: the first row of a dealer is its current link.
	for _, r := range links {
		l := r.FleetDealerLink
		if l.Status == model.LinkPending {
			pending++
		}
		if !l.StartedAt.Valid {
			continue // never active (a rejected request): nothing to show
		}
		on, ok := enabled[l.DealerOrgID]
		if !ok {
			if on, err = s.moduleOn(ctx, l.DealerOrgID); err != nil {
				return nil, false, 0, err
			}
			enabled[l.DealerOrgID] = on
		}
		if !on {
			continue
		}
		if l.Status == model.LinkActive {
			open = true
		}
		if i, dup := seen[l.DealerOrgID]; dup {
			if !dealers[i].cariID.Valid {
				dealers[i].cariID = l.CariAccountID
			}
			continue
		}
		seen[l.DealerOrgID] = len(dealers)
		dealers = append(dealers, portalDealer{
			id: l.DealerOrgID, uuid: r.DealerUuid, name: r.DealerName, linkStatus: l.Status,
			startedAt: l.StartedAt, cariID: l.CariAccountID,
		})
	}
	sort.SliceStable(dealers, func(i, j int) bool { return dealers[i].name < dealers[j].name })
	return dealers, open, pending, nil
}

func (s *Service) moduleOn(ctx context.Context, orgID int64) (bool, error) {
	if s.modules == nil {
		return true, nil
	}
	on, err := s.modules.Enabled(ctx, orgID, features.ModuleFleet)
	if err != nil {
		return false, fmt.Errorf("fleet: module: %w", err)
	}
	return on, nil
}

// --- Views -----------------------------------------------------------------------

// PortalDealerView is a dealer the fleet works (or worked) with.
type PortalDealerView struct {
	UUID       uuid.UUID  `json:"uuid"`
	Name       string     `json:"name"`
	LinkStatus string     `json:"link_status"`
	StartedAt  *time.Time `json:"started_at"`
}

// PortalAppointmentView is an upcoming appointment of a fleet vehicle.
type PortalAppointmentView struct {
	UUID        uuid.UUID `json:"uuid"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	Status      string    `json:"status"`
	Dealer      PartyRef  `json:"dealer"`
	VehicleUUID uuid.UUID `json:"vehicle_uuid"`
	Plate       *string   `json:"plate"`
}

// PortalOverview is GET /v1/portal/fleet/overview.
type PortalOverview struct {
	Fleet                    PartyRef                `json:"fleet"`
	PeriodFrom               string                  `json:"period_from"`
	PeriodTo                 string                  `json:"period_to"`
	VehicleCount             int64                   `json:"vehicle_count"`
	ServiceCount             int64                   `json:"service_count"`
	ActiveWarrantyCount      int64                   `json:"active_warranty_count"`
	UpcomingAppointmentCount int64                   `json:"upcoming_appointment_count"`
	UpcomingAppointments     []PortalAppointmentView `json:"upcoming_appointments"`
	Dealers                  []PortalDealerView      `json:"dealers"`
	PendingLinkCount         int64                   `json:"pending_link_count"`
}

// PortalVehicleView is one fleet vehicle with the visible dealers' work.
type PortalVehicleView struct {
	UUID                uuid.UUID   `json:"uuid"`
	Plate               *string     `json:"plate"`
	PlateCountry        *string     `json:"plate_country"`
	VIN                 *string     `json:"vin"`
	ModelYear           *int        `json:"model_year"`
	CarBrand            *CatalogRef `json:"car_brand"`
	CarModel            *CatalogRef `json:"car_model"`
	ServiceCount        int64       `json:"service_count"`
	LastServiceAt       *time.Time  `json:"last_service_at"`
	ActiveWarrantyCount int64       `json:"active_warranty_count"`
	WarrantyUntil       *time.Time  `json:"warranty_until"`
	CreatedAt           time.Time   `json:"created_at"`
}

// PortalServiceView is a service on a fleet vehicle.
type PortalServiceView struct {
	UUID         uuid.UUID  `json:"uuid"`
	ServiceNo    string     `json:"service_no"`
	Status       string     `json:"status"`
	Package      *string    `json:"package"`
	Dealer       PartyRef   `json:"dealer"`
	VehicleUUID  uuid.UUID  `json:"vehicle_uuid"`
	Plate        *string    `json:"plate"`
	CarBrandName string     `json:"car_brand_name"`
	CarModelName string     `json:"car_model_name"`
	CompletedAt  *time.Time `json:"completed_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

// PortalWarrantyView is a warranty on a fleet vehicle. state is the
// effective status (active | expired | void).
type PortalWarrantyView struct {
	UUID        uuid.UUID `json:"uuid"`
	PublicCode  string    `json:"public_code"`
	State       string    `json:"state"`
	StartAt     time.Time `json:"start_at"`
	EndAt       time.Time `json:"end_at"`
	DaysLeft    int       `json:"days_left"`
	ProductName string    `json:"product_name"`
	ProductSKU  string    `json:"product_sku"`
	ServiceUUID uuid.UUID `json:"service_uuid"`
	ServiceNo   string    `json:"service_no"`
	Dealer      PartyRef  `json:"dealer"`
	VehicleUUID uuid.UUID `json:"vehicle_uuid"`
	Plate       *string   `json:"plate"`
}

// PortalVehicleDetail is GET /v1/portal/fleet/vehicles/{uuid}: the vehicle
// with its service history and warranties from every visible dealer.
type PortalVehicleDetail struct {
	PortalVehicleView
	Services   []PortalServiceView  `json:"services"`
	Warranties []PortalWarrantyView `json:"warranties"`
}

// PortalReportView is a ready fleet report.
type PortalReportView struct {
	UUID        uuid.UUID `json:"uuid"`
	PeriodKind  string    `json:"period_kind"`
	PeriodStart string    `json:"period_start"`
	PeriodEnd   string    `json:"period_end"`
	Locale      string    `json:"locale"`
	CreatedAt   time.Time `json:"created_at"`
}

// --- Overview --------------------------------------------------------------------

// maxOverviewAppointments caps the appointments listed in the overview.
const maxOverviewAppointments = 5

// PortalOverview is GET /v1/portal/fleet/overview: the service count runs
// over the period (default: the current month in the fleet's timezone).
func (s *Service) PortalOverview(ctx context.Context, c PortalCaller, period apiquery.TimeRange) (PortalOverview, error) {
	q := db.New(s.conn)
	p, err := s.resolvePortal(ctx, q, c)
	if err != nil {
		return PortalOverview{}, err
	}
	now := s.now()
	from, before := periodBounds(period, p.fleet.Timezone, now)
	out := PortalOverview{
		Fleet:      PartyRef{UUID: p.fleet.Uuid, Name: p.fleet.Name, LegalName: p.profile.LegalName, TaxNumber: p.profile.TaxNumber},
		PeriodFrom: from.Format(time.DateOnly), PeriodTo: before.Add(-time.Nanosecond).Format(time.DateOnly),
		UpcomingAppointments: []PortalAppointmentView{}, Dealers: []PortalDealerView{}, PendingLinkCount: p.pending,
	}
	if out.VehicleCount, err = q.CountFleetPortalVehicles(ctx, db.CountFleetPortalVehiclesParams{
		FleetOrgID: p.fleet.ID, DealerIds: p.ids,
	}); err != nil {
		return PortalOverview{}, fmt.Errorf("fleet: portal vehicles: %w", err)
	}
	if out.ServiceCount, err = q.CountFleetPortalServices(ctx, db.CountFleetPortalServicesParams{
		FleetOrgID: p.fleet.ID, DealerIds: p.ids,
		CreatedFrom: ts(from), CreatedBefore: ts(before),
	}); err != nil {
		return PortalOverview{}, fmt.Errorf("fleet: portal services: %w", err)
	}
	if out.ActiveWarrantyCount, err = q.CountFleetPortalWarranties(ctx, db.CountFleetPortalWarrantiesParams{
		FleetOrgID: p.fleet.ID, DealerIds: p.ids, States: []string{"active"},
	}); err != nil {
		return PortalOverview{}, fmt.Errorf("fleet: portal warranties: %w", err)
	}
	if out.UpcomingAppointmentCount, err = q.CountFleetPortalUpcomingAppointments(ctx, db.CountFleetPortalUpcomingAppointmentsParams{
		FleetOrgID: p.fleet.ID, DealerIds: p.ids, After: ts(now),
	}); err != nil {
		return PortalOverview{}, fmt.Errorf("fleet: portal appointments: %w", err)
	}
	appts, err := q.ListFleetPortalUpcomingAppointments(ctx, db.ListFleetPortalUpcomingAppointmentsParams{
		FleetOrgID: p.fleet.ID, DealerIds: p.ids, After: ts(now), LimitCount: maxOverviewAppointments,
	})
	if err != nil {
		return PortalOverview{}, fmt.Errorf("fleet: portal appointments: %w", err)
	}
	for _, a := range appts {
		out.UpcomingAppointments = append(out.UpcomingAppointments, PortalAppointmentView{
			UUID: a.Uuid, StartsAt: a.StartsAt.Time, EndsAt: a.EndsAt.Time, Status: a.Status,
			Dealer:      PartyRef{UUID: a.OrganizationUuid, Name: a.OrganizationName},
			VehicleUUID: a.VehicleUuid, Plate: textPtr(a.Plate),
		})
	}
	for _, d := range p.dealers {
		out.Dealers = append(out.Dealers, PortalDealerView{
			UUID: d.uuid, Name: d.name, LinkStatus: d.linkStatus, StartedAt: timePtr(d.startedAt),
		})
	}
	return out, nil
}

// periodBounds is [from, before) of a date range; without one the current
// month in tz.
func periodBounds(r apiquery.TimeRange, tz string, now time.Time) (time.Time, time.Time) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	n := now.In(loc)
	from := time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, loc)
	before := from.AddDate(0, 1, 0)
	if r.From != nil {
		from = *r.From
	}
	if r.Before != nil {
		before = *r.Before
	}
	return from, before
}

// --- Vehicles --------------------------------------------------------------------

// PortalVehicleFilter narrows GET /v1/portal/fleet/vehicles.
type PortalVehicleFilter struct {
	Q                 string
	CarBrandUUIDs     []uuid.UUID
	HasActiveWarranty *bool
	Sort              []apiquery.SortField
	Limit, Offset     int32
}

// PortalVehicles is GET /v1/portal/fleet/vehicles.
func (s *Service) PortalVehicles(ctx context.Context, c PortalCaller, f PortalVehicleFilter) ([]PortalVehicleView, int64, error) {
	srt, err := apiquery.ResolveSort(f.Sort, PortalVehicleSort)
	if err != nil {
		return nil, 0, err
	}
	q := db.New(s.conn)
	p, err := s.resolvePortal(ctx, q, c)
	if err != nil {
		return nil, 0, err
	}
	qt, qPlate := searchArgs(f.Q)
	var warranty pgtype.Bool
	if f.HasActiveWarranty != nil {
		warranty = pgtype.Bool{Bool: *f.HasActiveWarranty, Valid: true}
	}
	rows, err := q.ListFleetPortalVehicles(ctx, db.ListFleetPortalVehiclesParams{
		FleetOrgID: p.fleet.ID, DealerIds: p.ids, Q: qt, QPlate: qPlate, CarBrandUuids: f.CarBrandUUIDs,
		HasActiveWarranty: warranty, SortKey: srt.Key, SortDesc: srt.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("fleet: portal vehicles: %w", err)
	}
	total, err := q.CountFleetPortalVehicles(ctx, db.CountFleetPortalVehiclesParams{
		FleetOrgID: p.fleet.ID, DealerIds: p.ids, Q: qt, QPlate: qPlate, CarBrandUuids: f.CarBrandUUIDs,
		HasActiveWarranty: warranty,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("fleet: portal vehicles: %w", err)
	}
	out := make([]PortalVehicleView, 0, len(rows))
	for _, r := range rows {
		out = append(out, portalVehicleView(r))
	}
	return out, total, nil
}

// PortalVehicle is GET /v1/portal/fleet/vehicles/{uuid}: a vehicle of the
// user's fleet (another fleet's vehicle is ErrNotFound) with its services
// and warranties from every visible dealer.
func (s *Service) PortalVehicle(ctx context.Context, c PortalCaller, id uuid.UUID) (PortalVehicleDetail, error) {
	q := db.New(s.conn)
	p, err := s.resolvePortal(ctx, q, c)
	if err != nil {
		return PortalVehicleDetail{}, err
	}
	rows, err := q.ListFleetPortalVehicles(ctx, db.ListFleetPortalVehiclesParams{
		FleetOrgID: p.fleet.ID, DealerIds: p.ids, VehicleUuid: pgtype.UUID{Bytes: id, Valid: true},
		SortKey: "plate", LimitCount: 1,
	})
	if err != nil {
		return PortalVehicleDetail{}, fmt.Errorf("fleet: portal vehicle: %w", err)
	}
	if len(rows) == 0 {
		return PortalVehicleDetail{}, ErrNotFound
	}
	vehicleID := pgtype.Int8{Int64: rows[0].ID, Valid: true}
	out := PortalVehicleDetail{PortalVehicleView: portalVehicleView(rows[0])}
	services, err := q.ListFleetPortalServices(ctx, db.ListFleetPortalServicesParams{
		FleetOrgID: p.fleet.ID, DealerIds: p.ids, VehicleID: vehicleID,
		SortKey: "created_at", SortDesc: true, LimitCount: maxPortalVehicleServices,
	})
	if err != nil {
		return PortalVehicleDetail{}, fmt.Errorf("fleet: portal vehicle services: %w", err)
	}
	out.Services = portalServiceViews(services)
	warranties, err := q.ListFleetPortalWarranties(ctx, db.ListFleetPortalWarrantiesParams{
		FleetOrgID: p.fleet.ID, DealerIds: p.ids, VehicleID: vehicleID,
		SortKey: "end_at", SortDesc: true, LimitCount: maxPortalVehicleServices,
	})
	if err != nil {
		return PortalVehicleDetail{}, fmt.Errorf("fleet: portal vehicle warranties: %w", err)
	}
	out.Warranties = s.portalWarrantyViews(warranties)
	return out, nil
}

// --- Services --------------------------------------------------------------------

// PortalServiceFilter narrows GET /v1/portal/fleet/services.
type PortalServiceFilter struct {
	Q             string
	Statuses      []string
	DealerUUIDs   []uuid.UUID
	VehicleUUID   *uuid.UUID
	Created       apiquery.TimeRange
	Sort          []apiquery.SortField
	Limit, Offset int32
}

// PortalServices is GET /v1/portal/fleet/services.
func (s *Service) PortalServices(ctx context.Context, c PortalCaller, f PortalServiceFilter) ([]PortalServiceView, int64, error) {
	srt, err := apiquery.ResolveSort(f.Sort, PortalServiceSort)
	if err != nil {
		return nil, 0, err
	}
	q := db.New(s.conn)
	p, err := s.resolvePortal(ctx, q, c)
	if err != nil {
		return nil, 0, err
	}
	vehicleID, ok, err := portalVehicleID(ctx, q, p, f.VehicleUUID)
	if err != nil || !ok {
		return []PortalServiceView{}, 0, err
	}
	qt, _ := searchArgs(f.Q)
	from, before := tsRange(f.Created)
	rows, err := q.ListFleetPortalServices(ctx, db.ListFleetPortalServicesParams{
		FleetOrgID: p.fleet.ID, DealerIds: p.ids, VehicleID: vehicleID, Statuses: f.Statuses,
		DealerUuids: f.DealerUUIDs, CreatedFrom: from, CreatedBefore: before, Q: qt,
		SortKey: srt.Key, SortDesc: srt.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("fleet: portal services: %w", err)
	}
	total, err := q.CountFleetPortalServices(ctx, db.CountFleetPortalServicesParams{
		FleetOrgID: p.fleet.ID, DealerIds: p.ids, VehicleID: vehicleID, Statuses: f.Statuses,
		DealerUuids: f.DealerUUIDs, CreatedFrom: from, CreatedBefore: before, Q: qt,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("fleet: portal services: %w", err)
	}
	return portalServiceViews(rows), total, nil
}

// --- Warranties ------------------------------------------------------------------

// PortalWarrantyFilter narrows GET /v1/portal/fleet/warranties.
type PortalWarrantyFilter struct {
	Q             string
	States        []string
	DealerUUIDs   []uuid.UUID
	VehicleUUID   *uuid.UUID
	Sort          []apiquery.SortField
	Limit, Offset int32
}

// PortalWarranties is GET /v1/portal/fleet/warranties.
func (s *Service) PortalWarranties(ctx context.Context, c PortalCaller, f PortalWarrantyFilter) ([]PortalWarrantyView, int64, error) {
	srt, err := apiquery.ResolveSort(f.Sort, PortalWarrantySort)
	if err != nil {
		return nil, 0, err
	}
	q := db.New(s.conn)
	p, err := s.resolvePortal(ctx, q, c)
	if err != nil {
		return nil, 0, err
	}
	vehicleID, ok, err := portalVehicleID(ctx, q, p, f.VehicleUUID)
	if err != nil || !ok {
		return []PortalWarrantyView{}, 0, err
	}
	qt, _ := searchArgs(f.Q)
	rows, err := q.ListFleetPortalWarranties(ctx, db.ListFleetPortalWarrantiesParams{
		FleetOrgID: p.fleet.ID, DealerIds: p.ids, VehicleID: vehicleID, DealerUuids: f.DealerUUIDs,
		Q: qt, States: f.States, SortKey: srt.Key, SortDesc: srt.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("fleet: portal warranties: %w", err)
	}
	total, err := q.CountFleetPortalWarranties(ctx, db.CountFleetPortalWarrantiesParams{
		FleetOrgID: p.fleet.ID, DealerIds: p.ids, VehicleID: vehicleID, DealerUuids: f.DealerUUIDs,
		Q: qt, States: f.States,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("fleet: portal warranties: %w", err)
	}
	return s.portalWarrantyViews(rows), total, nil
}

// portalVehicleID resolves the optional vehicle filter: a vehicle outside
// the fleet matches nothing (ok false).
func portalVehicleID(ctx context.Context, q *db.Queries, p portalScope, id *uuid.UUID) (pgtype.Int8, bool, error) {
	if id == nil {
		return pgtype.Int8{}, true, nil
	}
	v, err := q.GetVehicleByUUID(ctx, *id)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.Int8{}, false, nil
	}
	if err != nil {
		return pgtype.Int8{}, false, fmt.Errorf("fleet: portal vehicle: %w", err)
	}
	if !v.FleetOrgID.Valid || v.FleetOrgID.Int64 != p.fleet.ID {
		return pgtype.Int8{}, false, nil
	}
	return pgtype.Int8{Int64: v.ID, Valid: true}, true, nil
}

// --- Accounting ------------------------------------------------------------------

// PortalAccountLine is one row of the fleet's cari at a dealer: a service
// income (debit: the fleet owes) or a collection (credit).
type PortalAccountLine struct {
	UUID       uuid.UUID         `json:"uuid"`
	Date       time.Time         `json:"date"`
	Kind       string            `json:"kind"`
	Debit      string            `json:"debit"`
	Credit     string            `json:"credit"`
	Balance    string            `json:"balance"`
	IsReversal bool              `json:"is_reversal"`
	Service    *StatementService `json:"service"`
}

// PortalDealerAccount is the fleet's cari at one dealer over the period.
type PortalDealerAccount struct {
	Dealer             PartyRef            `json:"dealer"`
	LinkStatus         string              `json:"link_status"`
	Currency           string              `json:"currency"`
	OpeningBalance     string              `json:"opening_balance"`
	ServiceIncomeTotal string              `json:"service_income_total"`
	CollectionTotal    string              `json:"collection_total"`
	ClosingBalance     string              `json:"closing_balance"`
	Lines              []PortalAccountLine `json:"lines"`
}

// PortalAccounting is GET /v1/portal/fleet/accounting.
type PortalAccounting struct {
	PeriodFrom string                `json:"period_from"`
	PeriodTo   string                `json:"period_to"`
	Dealers    []PortalDealerAccount `json:"dealers"`
}

// PortalAccounting is GET /v1/portal/fleet/accounting: for every visible
// dealer the fleet cari in the dealer's ledger, restricted to the service
// income billed to the fleet and the collections (never the dealer's other
// records), with the balance of those rows. The period is inclusive days in
// each dealer's timezone (default: the current month).
func (s *Service) PortalAccounting(ctx context.Context, c PortalCaller, from, to string) (PortalAccounting, error) {
	q := db.New(s.conn)
	p, err := s.resolvePortal(ctx, q, c)
	if err != nil {
		return PortalAccounting{}, err
	}
	period, err := portalPeriod(from, to, p.fleet.Timezone, s.now())
	if err != nil {
		return PortalAccounting{}, err
	}
	out := PortalAccounting{
		PeriodFrom: period.From.Format(time.DateOnly), PeriodTo: period.To.Format(time.DateOnly),
		Dealers: make([]PortalDealerAccount, 0, len(p.dealers)),
	}
	for _, d := range p.dealers {
		acc, err := portalDealerAccount(ctx, q, d, period)
		if err != nil {
			return PortalAccounting{}, err
		}
		out.Dealers = append(out.Dealers, acc)
	}
	return out, nil
}

// portalPeriod parses date_from / date_to (both or neither; default the
// current month in tz).
func portalPeriod(from, to, tz string, now time.Time) (StatementPeriod, error) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == "" && to == "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			loc = time.UTC
		}
		n := now.In(loc)
		start := time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, time.UTC)
		return StatementPeriod{From: start, To: start.AddDate(0, 1, -1)}, nil
	}
	p, err := ParsePeriod(from, to)
	var ve *ValidationError
	if errors.As(err, &ve) {
		ve.Field = strings.Replace(ve.Field, "period_", "date_", 1)
		ve.Message = strings.ReplaceAll(ve.Message, "period_from", "date_from")
	}
	return p, err
}

func portalDealerAccount(ctx context.Context, q *db.Queries, d portalDealer, p StatementPeriod) (PortalDealerAccount, error) {
	dealer, err := q.GetOrganizationByID(ctx, d.id)
	if err != nil {
		return PortalDealerAccount{}, fmt.Errorf("fleet: portal dealer: %w", err)
	}
	out := PortalDealerAccount{
		Dealer: PartyRef{UUID: dealer.Uuid, Name: dealer.Name}, LinkStatus: d.linkStatus, Currency: dealer.Currency,
		Lines: []PortalAccountLine{},
	}
	opening, income, collection := new(big.Rat), new(big.Rat), new(big.Rat)
	if d.cariID.Valid {
		loc, err := time.LoadLocation(dealer.Timezone)
		if err != nil {
			loc = time.UTC
		}
		start := time.Date(p.From.Year(), p.From.Month(), p.From.Day(), 0, 0, 0, 0, loc)
		end := time.Date(p.To.Year(), p.To.Month(), p.To.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
		cari, err := q.GetCariAccount(ctx, db.GetCariAccountParams{ID: d.cariID.Int64, OrganizationID: dealer.ID})
		if err != nil {
			return PortalDealerAccount{}, fmt.Errorf("fleet: portal cari: %w", err)
		}
		out.Currency = cari.Currency
		ob, err := q.FleetPortalCariBalanceBefore(ctx, db.FleetPortalCariBalanceBeforeParams{
			CariID: cari.ID, OrganizationID: dealer.ID, Before: ts(start),
		})
		if err != nil {
			return PortalDealerAccount{}, fmt.Errorf("fleet: portal opening balance: %w", err)
		}
		opening = rat(ob)
		rows, err := q.ListFleetPortalCariEntries(ctx, db.ListFleetPortalCariEntriesParams{
			CariID: cari.ID, OrganizationID: dealer.ID, PeriodFrom: ts(start), PeriodTo: ts(end),
		})
		if err != nil {
			return PortalDealerAccount{}, fmt.Errorf("fleet: portal cari entries: %w", err)
		}
		running := new(big.Rat).Set(opening)
		for _, r := range rows {
			amt := rat(r.Amount)
			l := PortalAccountLine{UUID: r.Uuid, Date: r.CreatedAt.Time, IsReversal: r.IsReversal}
			if r.Direction == "collection" {
				l.Kind = LineCollection
				collection.Add(collection, amt)
				running.Sub(running, amt)
				l.Credit = fmtRat(amt)
			} else {
				l.Kind = LineServiceIncome
				income.Add(income, amt)
				running.Add(running, amt)
				l.Debit = fmtRat(amt)
				if r.ServiceUuid.Valid {
					sv := &StatementService{
						UUID: r.ServiceUuid.Bytes, ServiceNo: r.ServiceNo.String, Plate: textPtr(r.Plate),
						CompletedAt: timePtr(r.ServiceCompletedAt),
					}
					if r.VehicleUuid.Valid {
						v := uuid.UUID(r.VehicleUuid.Bytes)
						sv.VehicleUUID = &v
					}
					l.Service = sv
				}
			}
			l.Balance = fmtRat(running)
			out.Lines = append(out.Lines, l)
		}
	}
	closing := new(big.Rat).Add(opening, income)
	closing.Sub(closing, collection)
	out.OpeningBalance, out.ServiceIncomeTotal = fmtRat(opening), fmtRat(income)
	out.CollectionTotal, out.ClosingBalance = fmtRat(collection), fmtRat(closing)
	return out, nil
}

// --- Reports ---------------------------------------------------------------------

// ErrReportFilesUnavailable: no storage is wired for the downloads.
var ErrReportFilesUnavailable = errors.New("fleet: report storage unavailable")

// PortalReportFilter narrows GET /v1/portal/fleet/reports.
type PortalReportFilter struct {
	PeriodKinds   []string
	Sort          []apiquery.SortField
	Limit, Offset int32
}

// PortalReports is GET /v1/portal/fleet/reports: the ready reports.
func (s *Service) PortalReports(ctx context.Context, c PortalCaller, f PortalReportFilter) ([]PortalReportView, int64, error) {
	srt, err := apiquery.ResolveSort(f.Sort, PortalReportSort)
	if err != nil {
		return nil, 0, err
	}
	q := db.New(s.conn)
	p, err := s.resolvePortal(ctx, q, c)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.ListFleetPortalReports(ctx, db.ListFleetPortalReportsParams{
		FleetOrgID: p.fleet.ID, PeriodKinds: f.PeriodKinds, SortDesc: srt.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("fleet: portal reports: %w", err)
	}
	total, err := q.CountFleetPortalReports(ctx, db.CountFleetPortalReportsParams{FleetOrgID: p.fleet.ID, PeriodKinds: f.PeriodKinds})
	if err != nil {
		return nil, 0, fmt.Errorf("fleet: portal reports: %w", err)
	}
	out := make([]PortalReportView, 0, len(rows))
	for _, r := range rows {
		out = append(out, PortalReportView{
			UUID: r.Uuid, PeriodKind: r.PeriodKind, PeriodStart: r.PeriodStart.Time.Format(time.DateOnly),
			PeriodEnd: r.PeriodEnd.Time.Format(time.DateOnly), Locale: r.Locale, CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, total, nil
}

// PortalReportFile is GET /v1/portal/fleet/reports/{uuid}/file: the PDF of
// a ready report of the user's fleet (anything else ErrNotFound).
func (s *Service) PortalReportFile(ctx context.Context, c PortalCaller, id uuid.UUID) (io.ReadCloser, string, error) {
	q := db.New(s.conn)
	p, err := s.resolvePortal(ctx, q, c)
	if err != nil {
		return nil, "", err
	}
	r, err := q.GetFleetReportByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (r.FleetOrgID != p.fleet.ID || r.Status != model.ReportReady || !r.StorageKey.Valid)) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("fleet: portal report: %w", err)
	}
	if s.files == nil {
		return nil, "", ErrReportFilesUnavailable
	}
	rc, _, err := s.files.Download(ctx, r.StorageKey.String)
	if err != nil {
		return nil, "", fmt.Errorf("fleet: portal report file: %w", err)
	}
	name := fmt.Sprintf("fleet-report-%s-%s.pdf", r.PeriodKind, r.PeriodStart.Time.Format(time.DateOnly))
	return rc, name, nil
}

// --- Service PDF (existing portal endpoint) --------------------------------------

// PortalServiceHolder lets the existing portal service detail and PDF
// (/v1/portal/services/{uuid}) serve a fleet user: the service is on a
// vehicle of the user's fleet at a visible dealer and is not a draft. It
// returns the fleet's primary user, the holder of the fleet warranties.
func (s *Service) PortalServiceHolder(ctx context.Context, brandID, userID int64, svc db.Service) (int64, bool, error) {
	q := db.New(s.conn)
	p, err := s.resolvePortal(ctx, q, PortalCaller{UserID: userID, BrandID: brandID})
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrPortalClosed) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if svc.Status == "draft" {
		return 0, false, nil
	}
	if _, ok := p.dealer(svc.OrganizationID); !ok {
		return 0, false, nil
	}
	v, err := q.GetVehicleByID(ctx, svc.VehicleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("fleet: portal service vehicle: %w", err)
	}
	if !v.FleetOrgID.Valid || v.FleetOrgID.Int64 != p.fleet.ID {
		return 0, false, nil
	}
	return p.profile.PrimaryUserID.Int64, true, nil
}

// --- helpers ---------------------------------------------------------------------

func portalVehicleView(r db.ListFleetPortalVehiclesRow) PortalVehicleView {
	v := PortalVehicleView{
		UUID: r.Uuid, Plate: textPtr(r.Plate), PlateCountry: textPtr(r.PlateCountry), VIN: textPtr(r.Vin),
		ServiceCount: r.ServiceCount, LastServiceAt: timePtr(r.LastServiceAt),
		ActiveWarrantyCount: r.ActiveWarrantyCount, WarrantyUntil: timePtr(r.WarrantyUntil), CreatedAt: r.CreatedAt.Time,
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

func portalServiceViews(rows []db.ListFleetPortalServicesRow) []PortalServiceView {
	out := make([]PortalServiceView, 0, len(rows))
	for _, r := range rows {
		out = append(out, PortalServiceView{
			UUID: r.Uuid, ServiceNo: r.ServiceNo, Status: r.Status, Package: textPtr(r.Package),
			Dealer:      PartyRef{UUID: r.OrganizationUuid, Name: r.OrganizationName},
			VehicleUUID: r.VehicleUuid, Plate: textPtr(r.Plate),
			CarBrandName: r.CarBrandName, CarModelName: r.CarModelName,
			CompletedAt: timePtr(r.CompletedAt), CreatedAt: r.CreatedAt.Time,
		})
	}
	return out
}

func (s *Service) portalWarrantyViews(rows []db.ListFleetPortalWarrantiesRow) []PortalWarrantyView {
	now := s.now()
	out := make([]PortalWarrantyView, 0, len(rows))
	for _, r := range rows {
		days := 0
		if r.State == "active" {
			days = int(r.EndAt.Time.Sub(now).Hours() / 24)
		}
		out = append(out, PortalWarrantyView{
			UUID: r.Uuid, PublicCode: r.PublicCode, State: r.State, StartAt: r.StartAt.Time, EndAt: r.EndAt.Time,
			DaysLeft: days, ProductName: r.ProductName, ProductSKU: r.ProductSku,
			ServiceUUID: r.ServiceUuid, ServiceNo: r.ServiceNo,
			Dealer:      PartyRef{UUID: r.OrganizationUuid, Name: r.OrganizationName},
			VehicleUUID: r.VehicleUuid, Plate: textPtr(r.Plate),
		})
	}
	return out
}

// searchArgs is the q argument and its plate-normalized prefix.
func searchArgs(raw string) (pgtype.Text, pgtype.Text) {
	q := strings.TrimSpace(raw)
	if q == "" {
		return pgtype.Text{}, pgtype.Text{}
	}
	var plate pgtype.Text
	if p := geo.NormalizePlate(q); p != "" {
		plate = pgtype.Text{String: p, Valid: true}
	}
	return pgtype.Text{String: q, Valid: true}, plate
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func tsRange(r apiquery.TimeRange) (pgtype.Timestamptz, pgtype.Timestamptz) {
	var from, before pgtype.Timestamptz
	if r.From != nil {
		from = ts(*r.From)
	}
	if r.Before != nil {
		before = ts(*r.Before)
	}
	return from, before
}
