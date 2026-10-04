package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	servicesuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	StatusScheduled = "scheduled"
	StatusConfirmed = "confirmed"
	StatusArrived   = "arrived"
	StatusNoShow    = "no_show"
	StatusCancelled = "cancelled"

	CodeCapacityFull      = "APPOINTMENT_CAPACITY_FULL"
	CodeInvalidTransition = "APPOINTMENT_INVALID_TRANSITION"
	CodeIntakeStarted     = "APPOINTMENT_INTAKE_ALREADY_STARTED"

	maxNote   = 20000
	maxReason = 1000
)

var (
	ErrNotFound          = errors.New("appointments: not found")
	ErrForbidden         = errors.New("appointments: forbidden")
	ErrCapacityFull      = errors.New("appointments: capacity full")
	ErrInvalidTransition = errors.New("appointments: invalid transition")
	ErrIntakeStarted     = errors.New("appointments: intake already started")
)

type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type DraftServiceCreator interface {
	Create(ctx context.Context, c servicesuc.Caller, in servicesuc.CreateInput) (servicesuc.ServiceView, error)
}

type Caller struct {
	Principal authctx.Principal
	Org       orgctx.Scope
	Filter    scopefilter.Filter
}

type Service struct {
	pool     TxBeginner
	q        *db.Queries
	out      outbox.Enqueuer
	services DraftServiceCreator
	nowFunc  func() time.Time
}

func New(pool TxBeginner, q *db.Queries, out outbox.Enqueuer, services DraftServiceCreator) *Service {
	return &Service{pool: pool, q: q, out: out, services: services, nowFunc: time.Now}
}

func (s *Service) inTx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("appointments: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("appointments: commit: %w", err)
	}
	return nil
}

func (s *Service) SetClock(fn func() time.Time) {
	if fn != nil {
		s.nowFunc = fn
	}
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return "appointments: invalid " + e.Field + ": " + e.Message
}

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

type Appointment struct {
	UUID             uuid.UUID  `json:"uuid"`
	OrganizationID   int64      `json:"organization_id"`
	CustomerUserID   int64      `json:"customer_user_id"`
	VehicleID        *int64     `json:"vehicle_id,omitempty"`
	StartsAt         time.Time  `json:"starts_at"`
	EndsAt           time.Time  `json:"ends_at"`
	EstimatedMinutes int32      `json:"estimated_minutes"`
	Source           string     `json:"source"`
	Status           string     `json:"status"`
	CancelReason     *string    `json:"cancel_reason,omitempty"`
	LeadID           *int64     `json:"lead_id,omitempty"`
	ServiceID        *int64     `json:"service_id,omitempty"`
	Note             string     `json:"note"`
	CreatedByUserID  *int64     `json:"created_by_user_id,omitempty"`
	CreatedAt        *time.Time `json:"created_at,omitempty"`
	UpdatedAt        *time.Time `json:"updated_at,omitempty"`
}

type Settings struct {
	UUID                      uuid.UUID      `json:"uuid"`
	OrganizationID            int64          `json:"organization_id"`
	DailyVehicleCapacity      int32          `json:"daily_vehicle_capacity"`
	DefaultEstimatedMinutes   int32          `json:"default_estimated_minutes"`
	SlotIntervalMinutes       int32          `json:"slot_interval_minutes"`
	WorkingHours              map[string]any `json:"working_hours"`
	PortalAppointmentsEnabled bool           `json:"portal_appointments_enabled"`
}

type Closure struct {
	UUID           uuid.UUID `json:"uuid"`
	OrganizationID int64     `json:"organization_id"`
	ClosedOn       string    `json:"closed_on"`
	Reason         string    `json:"reason"`
}

type CreateInput struct {
	CustomerUserID   int64     `json:"customer_user_id"`
	VehicleID        *int64    `json:"vehicle_id"`
	StartsAt         time.Time `json:"starts_at"`
	EstimatedMinutes *int32    `json:"estimated_minutes"`
	Source           string    `json:"source"`
	Note             string    `json:"note"`
}

type PatchInput = CreateInput

type StatusInput struct {
	Status       string  `json:"status"`
	CancelReason *string `json:"cancel_reason"`
}

type ListFilter struct {
	From, To time.Time
	Status   string
	Limit    int32
	Offset   int32
}

type OccupancyRow struct {
	OrganizationID   int64     `json:"organization_id"`
	OrganizationUUID uuid.UUID `json:"organization_uuid"`
	OrganizationName string    `json:"organization_name"`
	Capacity         int32     `json:"capacity"`
	Occupied         int64     `json:"occupied"`
	Remaining        int32     `json:"remaining"`
}

func (s *Service) GetSettings(ctx context.Context, c Caller) (Settings, error) {
	row, err := s.q.GetAppointmentSettings(ctx, c.Org.InternalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, ErrNotFound
	}
	if err != nil {
		return Settings{}, fmt.Errorf("appointments: settings: %w", err)
	}
	return settingsView(row), nil
}

func (s *Service) PutSettings(ctx context.Context, c Caller, in Settings) (Settings, error) {
	if !c.Principal.Can(rbac.PermAppointmentSettingsManage, rbac.ScopeManaged) {
		return Settings{}, ErrForbidden
	}
	if in.DailyVehicleCapacity <= 0 {
		return Settings{}, invalid("daily_vehicle_capacity", "must be greater than zero")
	}
	if in.DefaultEstimatedMinutes <= 0 {
		return Settings{}, invalid("default_estimated_minutes", "must be greater than zero")
	}
	if in.SlotIntervalMinutes <= 0 {
		return Settings{}, invalid("slot_interval_minutes", "must be greater than zero")
	}
	raw, err := json.Marshal(in.WorkingHours)
	if err != nil {
		return Settings{}, invalid("working_hours", "must be a JSON object")
	}
	row, err := s.q.UpsertAppointmentSettings(ctx, db.UpsertAppointmentSettingsParams{
		OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID,
		DailyVehicleCapacity: in.DailyVehicleCapacity, DefaultEstimatedMinutes: in.DefaultEstimatedMinutes,
		SlotIntervalMinutes: in.SlotIntervalMinutes, WorkingHours: raw,
		PortalAppointmentsEnabled: in.PortalAppointmentsEnabled,
	})
	if err != nil {
		return Settings{}, fmt.Errorf("appointments: upsert settings: %w", err)
	}
	return settingsView(row), nil
}

func (s *Service) ListClosures(ctx context.Context, c Caller, from, to time.Time) ([]Closure, error) {
	rows, err := s.q.ListAppointmentClosures(ctx, db.ListAppointmentClosuresParams{
		OrganizationID: c.Org.InternalID, FromDate: dateArg(from), ToDate: dateArg(to),
	})
	if err != nil {
		return nil, fmt.Errorf("appointments: closures: %w", err)
	}
	out := make([]Closure, 0, len(rows))
	for _, r := range rows {
		out = append(out, closureView(r))
	}
	return out, nil
}

func (s *Service) CreateClosure(ctx context.Context, c Caller, date time.Time, reason string) (Closure, error) {
	if !c.Principal.Can(rbac.PermAppointmentSettingsManage, rbac.ScopeManaged) {
		return Closure{}, ErrForbidden
	}
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) > maxReason {
		return Closure{}, invalid("reason", "must be at most 1000 characters")
	}
	row, err := s.q.CreateAppointmentClosure(ctx, db.CreateAppointmentClosureParams{
		OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, ClosedOn: dateArg(date), Reason: reason,
	})
	if err != nil {
		return Closure{}, fmt.Errorf("appointments: create closure: %w", err)
	}
	return closureView(row), nil
}

func (s *Service) DeleteClosure(ctx context.Context, c Caller, id uuid.UUID) error {
	if !c.Principal.Can(rbac.PermAppointmentSettingsManage, rbac.ScopeManaged) {
		return ErrForbidden
	}
	n, err := s.q.DeleteAppointmentClosureByUUID(ctx, db.DeleteAppointmentClosureByUUIDParams{Uuid: id, OrganizationID: c.Org.InternalID})
	if err != nil {
		return fmt.Errorf("appointments: delete closure: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) Availability(ctx context.Context, c Caller, from, to time.Time) ([]DayAvailability, error) {
	org, setting, loc, err := s.orgSettings(ctx, c.Org.InternalID)
	if err != nil {
		return nil, err
	}
	if !c.Filter.AllowsOrg(org.ID, org.BrandID) {
		return nil, ErrForbidden
	}
	startDay := dateOnly(from, loc)
	endDay := dateOnly(to, loc)
	if endDay.Before(startDay) {
		return nil, invalid("to", "must be after from")
	}
	closures, err := s.q.ListAppointmentClosures(ctx, db.ListAppointmentClosuresParams{
		OrganizationID: org.ID, FromDate: dateArg(startDay), ToDate: dateArg(endDay),
	})
	if err != nil {
		return nil, fmt.Errorf("appointments: closures: %w", err)
	}
	closed := map[string]bool{}
	for _, cl := range closures {
		closed[cl.ClosedOn.Time.Format(time.DateOnly)] = true
	}
	var out []DayAvailability
	for day := startDay; !day.After(endDay); day = day.AddDate(0, 0, 1) {
		dayStart, dayEnd := DayBounds(day, loc)
		occupied, err := s.q.CountActiveAppointmentsForOrganization(ctx, db.CountActiveAppointmentsForOrganizationParams{
			OrganizationID: org.ID, BrandID: org.BrandID,
			FromTime: tsArg(dayStart), ToTime: tsArg(dayEnd),
		})
		if err != nil {
			return nil, fmt.Errorf("appointments: day count: %w", err)
		}
		windows, err := ParseWorkingHours(setting.WorkingHours, day.Weekday())
		if err != nil {
			return nil, invalid("working_hours", err.Error())
		}
		av, err := BuildDayAvailability(DailyRule{
			Date: day, Location: loc, Capacity: setting.DailyVehicleCapacity,
			Interval: time.Duration(setting.SlotIntervalMinutes) * time.Minute,
			Duration: time.Duration(setting.DefaultEstimatedMinutes) * time.Minute,
			Windows:  windows, Closed: closed[day.Format(time.DateOnly)], Occupied: occupied, Now: s.nowFunc(),
		})
		if err != nil {
			return nil, invalid("working_hours", err.Error())
		}
		out = append(out, av)
	}
	return out, nil
}

func (s *Service) List(ctx context.Context, c Caller, f ListFilter) ([]Appointment, int64, error) {
	status, err := statusFilter(f.Status)
	if err != nil {
		return nil, 0, err
	}
	params := db.ListAppointmentsByOrganizationsParams{
		OrganizationIds: c.Filter.OrgIDsArg(), BrandID: c.Org.BrandID,
		FromTime: tsArg(f.From), ToTime: tsArg(f.To), Status: status,
		PageLimit: f.Limit, PageOffset: f.Offset,
	}
	rows, err := s.q.ListAppointmentsByOrganizations(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("appointments: list: %w", err)
	}
	total, err := s.q.CountAppointmentsByOrganizations(ctx, db.CountAppointmentsByOrganizationsParams{
		OrganizationIds: params.OrganizationIds, BrandID: params.BrandID, FromTime: params.FromTime, ToTime: params.ToTime, Status: params.Status,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("appointments: count: %w", err)
	}
	out := make([]Appointment, 0, len(rows))
	for _, r := range rows {
		out = append(out, appointmentView(r))
	}
	return out, total, nil
}

func (s *Service) Create(ctx context.Context, c Caller, in CreateInput) (Appointment, error) {
	if !c.Principal.Can(rbac.PermAppointmentsWrite, rbac.ScopeManaged) {
		return Appointment{}, ErrForbidden
	}
	row, err := s.save(ctx, c, db.Appointment{}, in, true)
	if err != nil {
		return Appointment{}, err
	}
	return appointmentView(row), nil
}

func (s *Service) Patch(ctx context.Context, c Caller, id uuid.UUID, in PatchInput) (Appointment, error) {
	cur, err := s.byUUID(ctx, c, id)
	if err != nil {
		return Appointment{}, err
	}
	if cur.Status == StatusCancelled || cur.Status == StatusNoShow {
		return Appointment{}, ErrInvalidTransition
	}
	row, err := s.save(ctx, c, cur, in, false)
	if err != nil {
		return Appointment{}, err
	}
	return appointmentView(row), nil
}

func (s *Service) SetStatus(ctx context.Context, c Caller, id uuid.UUID, in StatusInput) (Appointment, error) {
	cur, err := s.byUUID(ctx, c, id)
	if err != nil {
		return Appointment{}, err
	}
	next := strings.TrimSpace(in.Status)
	if !allowedTransition(cur.Status, next) {
		return Appointment{}, ErrInvalidTransition
	}
	reason := textOrNull(in.CancelReason)
	if next == StatusCancelled && !reason.Valid {
		return Appointment{}, invalid("cancel_reason", "is required when cancelling")
	}
	if next != StatusCancelled {
		reason = pgtype.Text{}
	}
	var row db.Appointment
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		updated, err := q.SetAppointmentStatus(ctx, db.SetAppointmentStatusParams{
			ID: cur.ID, OrganizationID: cur.OrganizationID, Status: next, CancelReason: reason,
		})
		if err != nil {
			return fmt.Errorf("appointments: status: %w", err)
		}
		row = updated
		if next == StatusCancelled {
			return s.emit(ctx, tx, events.AppointmentCancelled, row)
		}
		return nil
	})
	if err != nil {
		return Appointment{}, err
	}
	return appointmentView(row), nil
}

func (s *Service) StartIntake(ctx context.Context, c Caller, id uuid.UUID) (Appointment, error) {
	if s.services == nil {
		return Appointment{}, fmt.Errorf("appointments: services usecase is nil")
	}
	cur, err := s.byUUID(ctx, c, id)
	if err != nil {
		return Appointment{}, err
	}
	if cur.ServiceID.Valid {
		return Appointment{}, ErrIntakeStarted
	}
	if !cur.VehicleID.Valid {
		return Appointment{}, invalid("vehicle_id", "is required to start intake")
	}
	customer, err := s.q.GetUserByID(ctx, cur.CustomerUserID)
	if err != nil {
		return Appointment{}, fmt.Errorf("appointments: customer: %w", err)
	}
	vehicle, err := s.q.GetVehicleByID(ctx, cur.VehicleID.Int64)
	if err != nil {
		return Appointment{}, fmt.Errorf("appointments: vehicle: %w", err)
	}
	view, err := s.services.Create(ctx, servicesuc.Caller{Principal: c.Principal, Org: c.Org, Filter: c.Filter}, servicesuc.CreateInput{
		CustomerUUID: customer.Uuid.String(), VehicleUUID: vehicle.Uuid.String(), Notes: strPtr(cur.Note),
	})
	if err != nil {
		return Appointment{}, fmt.Errorf("appointments: start intake service: %w", err)
	}
	serviceRow, err := s.q.GetServiceByUUID(ctx, db.GetServiceByUUIDParams{Uuid: view.UUID, BrandID: c.Org.BrandID})
	if err != nil {
		return Appointment{}, fmt.Errorf("appointments: created service lookup: %w", err)
	}
	serviceID := pgtype.Int8{Int64: serviceRow.ID, Valid: true}
	row, err := s.q.UpdateAppointment(ctx, db.UpdateAppointmentParams{
		ID: cur.ID, OrganizationID: cur.OrganizationID,
		CustomerUserID: cur.CustomerUserID, VehicleID: cur.VehicleID, StartsAt: cur.StartsAt, EndsAt: cur.EndsAt,
		EstimatedMinutes: cur.EstimatedMinutes, Source: cur.Source, LeadID: cur.LeadID, ServiceID: serviceID, Note: cur.Note,
	})
	if err != nil {
		return Appointment{}, fmt.Errorf("appointments: link service: %w", err)
	}
	row, err = s.q.SetAppointmentStatus(ctx, db.SetAppointmentStatusParams{
		ID: row.ID, OrganizationID: row.OrganizationID, Status: StatusArrived,
	})
	if err != nil {
		return Appointment{}, fmt.Errorf("appointments: arrived: %w", err)
	}
	return appointmentView(row), nil
}

func (s *Service) Occupancy(ctx context.Context, c Caller, date time.Time) ([]OccupancyRow, error) {
	rows, err := s.q.ListOrganizationsInScope(ctx, db.ListOrganizationsInScopeParams{
		OrgIds: c.Filter.OrgIDsArg(), BrandID: c.Filter.BrandIDArg(), LimitCount: 1000,
	})
	if err != nil {
		return nil, fmt.Errorf("appointments: organizations: %w", err)
	}
	orgs := make([]db.Organization, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		if r.Organization.BrandID != c.Org.BrandID || !c.Filter.AllowsOrg(r.Organization.ID, r.Organization.BrandID) {
			continue
		}
		orgs = append(orgs, r.Organization)
		ids = append(ids, r.Organization.ID)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	settings, err := s.q.ListAppointmentSettingsByOrganizations(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("appointments: occupancy settings: %w", err)
	}
	capByOrg := map[int64]int32{}
	for _, st := range settings {
		capByOrg[st.OrganizationID] = st.DailyVehicleCapacity
	}
	loc := time.UTC
	dayStart, dayEnd := DayBounds(date, loc)
	counts, err := s.q.CountActiveAppointmentsByOrganization(ctx, db.CountActiveAppointmentsByOrganizationParams{
		OrganizationIds: ids, BrandID: c.Org.BrandID, FromTime: tsArg(dayStart), ToTime: tsArg(dayEnd),
	})
	if err != nil {
		return nil, fmt.Errorf("appointments: occupancy count: %w", err)
	}
	occByOrg := map[int64]int64{}
	for _, r := range counts {
		occByOrg[r.OrganizationID] = r.ActiveCount
	}
	out := make([]OccupancyRow, 0, len(orgs))
	for _, org := range orgs {
		capacity := capByOrg[org.ID]
		occupied := occByOrg[org.ID]
		remaining := int32(0)
		if int64(capacity) > occupied {
			remaining = int32(int64(capacity) - occupied)
		}
		out = append(out, OccupancyRow{OrganizationID: org.ID, OrganizationUUID: org.Uuid, OrganizationName: org.Name, Capacity: capacity, Occupied: occupied, Remaining: remaining})
	}
	return out, nil
}

func (s *Service) save(ctx context.Context, c Caller, cur db.Appointment, in CreateInput, create bool) (db.Appointment, error) {
	in.Source = strings.TrimSpace(in.Source)
	if in.Source == "" {
		in.Source = "panel"
	}
	if err := validateInput(in); err != nil {
		return db.Appointment{}, err
	}
	org, setting, loc, err := s.orgSettings(ctx, c.Org.InternalID)
	if err != nil {
		return db.Appointment{}, err
	}
	if org.BrandID != c.Org.BrandID || !c.Filter.AllowsOrg(org.ID, org.BrandID) {
		return db.Appointment{}, ErrForbidden
	}
	mins := setting.DefaultEstimatedMinutes
	if in.EstimatedMinutes != nil {
		mins = *in.EstimatedMinutes
	}
	starts := in.StartsAt.UTC()
	ends := starts.Add(time.Duration(mins) * time.Minute)
	dayStart, dayEnd := DayBounds(starts, loc)
	var exclude pgtype.Int8
	if !create {
		exclude = pgtype.Int8{Int64: cur.ID, Valid: true}
	}
	var row db.Appointment
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		count, err := q.CountActiveAppointmentsForOrganization(ctx, db.CountActiveAppointmentsForOrganizationParams{
			OrganizationID: org.ID, BrandID: org.BrandID, FromTime: tsArg(dayStart), ToTime: tsArg(dayEnd), ExcludeID: exclude,
		})
		if err != nil {
			return fmt.Errorf("appointments: capacity count: %w", err)
		}
		if count >= int64(setting.DailyVehicleCapacity) {
			return ErrCapacityFull
		}
		params := db.CreateAppointmentParams{
			OrganizationID: org.ID, BrandID: org.BrandID, CustomerUserID: in.CustomerUserID,
			VehicleID: int8Ptr(in.VehicleID), StartsAt: tsArg(starts), EndsAt: tsArg(ends),
			EstimatedMinutes: mins, Source: in.Source, Status: StatusScheduled,
			Note: strings.TrimSpace(in.Note), CreatedByUserID: c.actor(),
		}
		if create {
			created, err := q.CreateAppointment(ctx, params)
			if err != nil {
				return fmt.Errorf("appointments: create: %w", err)
			}
			row = created
			return s.emit(ctx, tx, events.AppointmentCreated, row)
		}
		updated, err := q.UpdateAppointment(ctx, db.UpdateAppointmentParams{
			ID: cur.ID, OrganizationID: cur.OrganizationID, CustomerUserID: params.CustomerUserID,
			VehicleID: params.VehicleID, StartsAt: params.StartsAt, EndsAt: params.EndsAt,
			EstimatedMinutes: params.EstimatedMinutes, Source: params.Source, LeadID: cur.LeadID,
			ServiceID: cur.ServiceID, Note: params.Note,
		})
		if err != nil {
			return fmt.Errorf("appointments: update: %w", err)
		}
		row = updated
		return s.emit(ctx, tx, events.AppointmentRescheduled, row)
	})
	if err != nil {
		return db.Appointment{}, err
	}
	return row, nil
}

func (s *Service) byUUID(ctx context.Context, c Caller, id uuid.UUID) (db.Appointment, error) {
	row, err := s.q.GetAppointmentByUUID(ctx, db.GetAppointmentByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Appointment{}, ErrNotFound
	}
	if err != nil {
		return db.Appointment{}, fmt.Errorf("appointments: get: %w", err)
	}
	if !c.Filter.AllowsOrg(row.OrganizationID, row.BrandID) {
		return db.Appointment{}, ErrNotFound
	}
	return row, nil
}

func (s *Service) orgSettings(ctx context.Context, orgID int64) (db.Organization, db.AppointmentSetting, *time.Location, error) {
	org, err := s.q.GetOrganizationByID(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, db.AppointmentSetting{}, nil, ErrNotFound
	}
	if err != nil {
		return db.Organization{}, db.AppointmentSetting{}, nil, fmt.Errorf("appointments: organization: %w", err)
	}
	setting, err := s.q.GetAppointmentSettings(ctx, org.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, db.AppointmentSetting{}, nil, ErrNotFound
	}
	if err != nil {
		return db.Organization{}, db.AppointmentSetting{}, nil, fmt.Errorf("appointments: settings: %w", err)
	}
	loc, err := time.LoadLocation(org.Timezone)
	if err != nil {
		loc = time.UTC
	}
	return org, setting, loc, nil
}

func validateInput(in CreateInput) error {
	if in.CustomerUserID <= 0 {
		return invalid("customer_user_id", "is required")
	}
	if in.StartsAt.IsZero() {
		return invalid("starts_at", "is required")
	}
	if in.EstimatedMinutes != nil && *in.EstimatedMinutes <= 0 {
		return invalid("estimated_minutes", "must be greater than zero")
	}
	if !validSource(strings.TrimSpace(in.Source)) {
		return invalid("source", "is invalid")
	}
	if len([]rune(strings.TrimSpace(in.Note))) > maxNote {
		return invalid("note", "must be at most 20000 characters")
	}
	return nil
}

func validSource(v string) bool {
	switch v {
	case "panel", "portal", "assistant", "lead":
		return true
	}
	return false
}

func statusFilter(v string) (pgtype.Text, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}, nil
	}
	if !validStatus(v) {
		return pgtype.Text{}, invalid("status", "is invalid")
	}
	return pgtype.Text{String: v, Valid: true}, nil
}

func validStatus(v string) bool {
	switch v {
	case StatusScheduled, StatusConfirmed, StatusArrived, StatusNoShow, StatusCancelled:
		return true
	}
	return false
}

func allowedTransition(from, to string) bool {
	switch from {
	case StatusScheduled:
		return to == StatusConfirmed || to == StatusCancelled || to == StatusNoShow
	case StatusConfirmed:
		return to == StatusArrived || to == StatusCancelled || to == StatusNoShow
	}
	return false
}

func (c Caller) actor() pgtype.Int8 {
	return pgtype.Int8{Int64: c.Principal.UserInternal, Valid: c.Principal.UserInternal != 0}
}

func (s *Service) emit(ctx context.Context, tx pgx.Tx, name string, a db.Appointment) error {
	if s.out == nil {
		return nil
	}
	id, uid := a.ID, a.Uuid
	ev := events.New(name).WithTenant(a.OrganizationID).WithEntity("appointment", &id, &uid).WithPayload(map[string]any{
		"appointment_uuid": a.Uuid.String(), "organization_id": a.OrganizationID, "brand_id": a.BrandID,
		"status": a.Status, "starts_at": a.StartsAt.Time.UTC().Format(time.RFC3339),
		"customer_user_id": a.CustomerUserID,
	})
	if a.CreatedByUserID.Valid {
		ev = ev.WithActor(a.CreatedByUserID.Int64)
	}
	if err := s.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("appointments: outbox: %w", err)
	}
	return nil
}

func settingsView(r db.AppointmentSetting) Settings {
	m := map[string]any{}
	_ = json.Unmarshal(r.WorkingHours, &m)
	return Settings{UUID: r.Uuid, OrganizationID: r.OrganizationID, DailyVehicleCapacity: r.DailyVehicleCapacity,
		DefaultEstimatedMinutes: r.DefaultEstimatedMinutes, SlotIntervalMinutes: r.SlotIntervalMinutes,
		WorkingHours: m, PortalAppointmentsEnabled: r.PortalAppointmentsEnabled}
}

func closureView(r db.AppointmentClosure) Closure {
	return Closure{UUID: r.Uuid, OrganizationID: r.OrganizationID, ClosedOn: r.ClosedOn.Time.Format(time.DateOnly), Reason: r.Reason}
}

func appointmentView(r db.Appointment) Appointment {
	return Appointment{UUID: r.Uuid, OrganizationID: r.OrganizationID, CustomerUserID: r.CustomerUserID,
		VehicleID: intPtr(r.VehicleID), StartsAt: r.StartsAt.Time, EndsAt: r.EndsAt.Time,
		EstimatedMinutes: r.EstimatedMinutes, Source: r.Source, Status: r.Status, CancelReason: textPtr(r.CancelReason),
		LeadID: intPtr(r.LeadID), ServiceID: intPtr(r.ServiceID), Note: r.Note, CreatedByUserID: intPtr(r.CreatedByUserID),
		CreatedAt: timePtr(r.CreatedAt), UpdatedAt: timePtr(r.UpdatedAt)}
}

func textOrNull(p *string) pgtype.Text {
	if p == nil {
		return pgtype.Text{}
	}
	v := strings.TrimSpace(*p)
	return pgtype.Text{String: v, Valid: v != ""}
}

func strPtr(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return &v
}

func int8Ptr(p *int64) pgtype.Int8 {
	if p == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *p, Valid: true}
}

func intPtr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func textPtr(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func timePtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	return &v.Time
}

func dateArg(t time.Time) pgtype.Date {
	return pgtype.Date{Time: t, Valid: true}
}

func tsArg(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}
