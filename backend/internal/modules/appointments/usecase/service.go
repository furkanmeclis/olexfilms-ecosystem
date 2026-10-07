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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
	CodeDayClosed         = "APPOINTMENT_DAY_CLOSED"
	CodeClosureExists     = "APPOINTMENT_CLOSURE_EXISTS"
	CodeCancelWindow      = "APPOINTMENT_CANCEL_WINDOW_CLOSED"

	// maxRangeDays bounds the availability window (one count query per day).
	maxRangeDays = 62

	maxNote   = 20000
	maxReason = 1000
)

var (
	ErrNotFound          = errors.New("appointments: not found")
	ErrForbidden         = errors.New("appointments: forbidden")
	ErrCapacityFull      = errors.New("appointments: capacity full")
	ErrInvalidTransition = errors.New("appointments: invalid transition")
	ErrIntakeStarted     = errors.New("appointments: intake already started")
	ErrDayClosed         = errors.New("appointments: day closed")
	ErrClosureExists     = errors.New("appointments: closure already exists")
	ErrCancelWindow      = errors.New("appointments: cancel window closed")
)

type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type DraftServiceCreator interface {
	Create(ctx context.Context, c servicesuc.Caller, in servicesuc.CreateInput) (servicesuc.ServiceView, error)
}

type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
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
	features FeatureChecker
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

func (s *Service) SetFeatureChecker(checker FeatureChecker) {
	s.features = checker
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
	// Panel references (TEC-326): the panel works with UUIDs and labels.
	CustomerUUID *uuid.UUID `json:"customer_uuid,omitempty"`
	CustomerName string     `json:"customer_name,omitempty"`
	VehicleUUID  *uuid.UUID `json:"vehicle_uuid,omitempty"`
	VehiclePlate *string    `json:"vehicle_plate,omitempty"`
	VehicleLabel *string    `json:"vehicle_label,omitempty"`
	ServiceUUID  *uuid.UUID `json:"service_uuid,omitempty"`
}

// PortalAppointment is one row of the portal "my appointments" list
// (TEC-327): the appointment with its dealer and vehicle labels.
type PortalAppointment struct {
	Appointment
	DealerUUID   uuid.UUID  `json:"dealer_uuid"`
	DealerName   string     `json:"dealer_name"`
	VehicleUUID  *uuid.UUID `json:"vehicle_uuid"`
	VehiclePlate *string    `json:"vehicle_plate"`
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
	CustomerUserID int64  `json:"customer_user_id"`
	VehicleID      *int64 `json:"vehicle_id"`
	// CustomerUUID / VehicleUUID (TEC-326) may replace the internal ids; a
	// UUID wins over the id when both are sent.
	CustomerUUID     *uuid.UUID `json:"customer_uuid"`
	VehicleUUID      *uuid.UUID `json:"vehicle_uuid"`
	StartsAt         time.Time  `json:"starts_at"`
	EstimatedMinutes *int32     `json:"estimated_minutes"`
	Source           string     `json:"source"`
	Note             string     `json:"note"`
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

type PortalCaller struct {
	UserID  int64
	BrandID int64
}

type PortalCreateInput struct {
	DealerUUID  uuid.UUID `json:"dealer_uuid"`
	VehicleUUID uuid.UUID `json:"vehicle_uuid"`
	StartsAt    time.Time `json:"starts_at"`
	Note        string    `json:"note"`
}

type PortalListFilter struct {
	Period string
	Limit  int32
	Offset int32
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
	if in.WorkingHours == nil {
		in.WorkingHours = map[string]any{}
	}
	raw, err := json.Marshal(in.WorkingHours)
	if err != nil {
		return Settings{}, invalid("working_hours", "must be a JSON object")
	}
	if err := ValidateWorkingHours(raw); err != nil {
		return Settings{}, invalid("working_hours", err.Error())
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
	if isUniqueViolation(err) {
		return Closure{}, ErrClosureExists
	}
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
	return s.availabilityForOrg(ctx, org, setting, loc, from, to)
}

func (s *Service) PortalAvailability(ctx context.Context, c PortalCaller, dealerID uuid.UUID, from, to time.Time) ([]DayAvailability, error) {
	dealer, err := s.portalDealer(ctx, c, dealerID)
	if err != nil {
		return nil, err
	}
	org, setting, loc, err := s.orgSettings(ctx, dealer.ID)
	if err != nil {
		return nil, err
	}
	return s.availabilityForOrg(ctx, org, setting, loc, from, to)
}

func (s *Service) PortalCreate(ctx context.Context, c PortalCaller, in PortalCreateInput) (Appointment, error) {
	dealer, err := s.portalDealer(ctx, c, in.DealerUUID)
	if err != nil {
		return Appointment{}, err
	}
	v, err := s.q.GetPortalVehicle(ctx, db.GetPortalVehicleParams{
		Uuid: in.VehicleUUID, UserID: c.UserID, BrandID: c.BrandID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Appointment{}, ErrNotFound
	}
	if err != nil {
		return Appointment{}, fmt.Errorf("appointments: portal vehicle: %w", err)
	}
	row, err := s.save(ctx, portalSaveCaller(c), dealer.ID, db.Appointment{}, CreateInput{
		CustomerUserID: c.UserID, VehicleID: &v.ID, StartsAt: in.StartsAt, Source: "portal", Note: in.Note,
	}, true)
	if err != nil {
		return Appointment{}, err
	}
	return appointmentView(row), nil
}

func (s *Service) PortalList(ctx context.Context, c PortalCaller, f PortalListFilter) ([]PortalAppointment, int64, error) {
	upcoming, past, err := portalPeriod(f.Period)
	if err != nil {
		return nil, 0, err
	}
	now := tsArg(s.nowFunc().UTC())
	params := db.ListPortalAppointmentsParams{
		CustomerUserID: c.UserID, BrandID: c.BrandID, Upcoming: upcoming, Past: past,
		Now: now, PageLimit: f.Limit, PageOffset: f.Offset,
	}
	rows, err := s.q.ListPortalAppointments(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("appointments: portal list: %w", err)
	}
	total, err := s.q.CountPortalAppointments(ctx, db.CountPortalAppointmentsParams{
		CustomerUserID: c.UserID, BrandID: c.BrandID, Upcoming: upcoming, Past: past, Now: now,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("appointments: portal count: %w", err)
	}
	out := make([]PortalAppointment, 0, len(rows))
	for _, r := range rows {
		item := PortalAppointment{
			Appointment: appointmentView(r.Appointment),
			DealerUUID:  r.DealerUuid, DealerName: r.DealerName,
			VehiclePlate: textPtr(r.VehiclePlate),
		}
		if r.VehicleUuid.Valid {
			v := uuid.UUID(r.VehicleUuid.Bytes)
			item.VehicleUUID = &v
		}
		out = append(out, item)
	}
	return out, total, nil
}

func (s *Service) PortalCancel(ctx context.Context, c PortalCaller, id uuid.UUID) (Appointment, error) {
	cur, err := s.q.GetPortalAppointmentByUUID(ctx, db.GetPortalAppointmentByUUIDParams{
		Uuid: id, CustomerUserID: c.UserID, BrandID: c.BrandID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Appointment{}, ErrNotFound
	}
	if err != nil {
		return Appointment{}, fmt.Errorf("appointments: portal get: %w", err)
	}
	if cur.StartsAt.Time.UTC().Sub(s.nowFunc().UTC()) < 2*time.Hour {
		return Appointment{}, ErrCancelWindow
	}
	if !allowedTransition(cur.Status, StatusCancelled) {
		return Appointment{}, ErrInvalidTransition
	}
	reason := pgtype.Text{String: "portal cancellation", Valid: true}
	var row db.Appointment
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		locked, err := q.LockAppointmentByID(ctx, db.LockAppointmentByIDParams{
			ID: cur.ID, OrganizationID: cur.OrganizationID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("appointments: portal lock: %w", err)
		}
		if locked.CustomerUserID != c.UserID || locked.BrandID != c.BrandID {
			return ErrNotFound
		}
		if locked.StartsAt.Time.UTC().Sub(s.nowFunc().UTC()) < 2*time.Hour {
			return ErrCancelWindow
		}
		if !allowedTransition(locked.Status, StatusCancelled) {
			return ErrInvalidTransition
		}
		updated, err := q.SetAppointmentStatus(ctx, db.SetAppointmentStatusParams{
			ID: locked.ID, OrganizationID: locked.OrganizationID, Status: StatusCancelled, CancelReason: reason,
		})
		if err != nil {
			return fmt.Errorf("appointments: portal cancel: %w", err)
		}
		row = updated
		return s.emit(ctx, tx, events.AppointmentCancelled, row)
	})
	if err != nil {
		return Appointment{}, err
	}
	return appointmentView(row), nil
}

func (s *Service) availabilityForOrg(
	ctx context.Context,
	org db.Organization,
	setting db.AppointmentSetting,
	loc *time.Location,
	from time.Time,
	to time.Time,
) ([]DayAvailability, error) {
	startDay := calendarDay(from, loc)
	endDay := calendarDay(to, loc)
	if endDay.Before(startDay) {
		return nil, invalid("to", "must be after from")
	}
	if endDay.After(startDay.AddDate(0, 0, maxRangeDays)) {
		return nil, invalid("to", fmt.Sprintf("range must be at most %d days", maxRangeDays))
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
		av.Timezone = loc.String()
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
	out, err := s.withRefs(ctx, rows...)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (s *Service) Create(ctx context.Context, c Caller, in CreateInput) (Appointment, error) {
	if !c.Principal.Can(rbac.PermAppointmentsWrite, rbac.ScopeManaged) {
		return Appointment{}, ErrForbidden
	}
	if err := s.resolveParties(ctx, &in); err != nil {
		return Appointment{}, err
	}
	row, err := s.save(ctx, c, c.Org.InternalID, db.Appointment{}, in, true)
	if err != nil {
		return Appointment{}, err
	}
	return s.oneWithRefs(ctx, row)
}

func (s *Service) Patch(ctx context.Context, c Caller, id uuid.UUID, in PatchInput) (Appointment, error) {
	cur, err := s.writableByUUID(ctx, c, id)
	if err != nil {
		return Appointment{}, err
	}
	if !reschedulable(cur.Status) {
		return Appointment{}, ErrInvalidTransition
	}
	if err := s.resolveParties(ctx, &in); err != nil {
		return Appointment{}, err
	}
	row, err := s.save(ctx, c, cur.OrganizationID, cur, in, false)
	if err != nil {
		return Appointment{}, err
	}
	return s.oneWithRefs(ctx, row)
}

func (s *Service) SetStatus(ctx context.Context, c Caller, id uuid.UUID, in StatusInput) (Appointment, error) {
	cur, err := s.writableByUUID(ctx, c, id)
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
		locked, err := q.LockAppointmentByID(ctx, db.LockAppointmentByIDParams{ID: cur.ID, OrganizationID: cur.OrganizationID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("appointments: lock: %w", err)
		}
		if !allowedTransition(locked.Status, next) {
			return ErrInvalidTransition
		}
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
	return s.oneWithRefs(ctx, row)
}

// StartIntake opens the draft service of an appointment through the services
// usecase (F1-05) and links it. The appointment row stays locked while the
// service is created, so a concurrent second call waits and then gets
// ErrIntakeStarted instead of opening a second draft.
func (s *Service) StartIntake(ctx context.Context, c Caller, id uuid.UUID) (Appointment, error) {
	if s.services == nil {
		return Appointment{}, fmt.Errorf("appointments: services usecase is nil")
	}
	cur, err := s.writableByUUID(ctx, c, id)
	if err != nil {
		return Appointment{}, err
	}
	var row db.Appointment
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		locked, err := q.LockAppointmentByID(ctx, db.LockAppointmentByIDParams{ID: cur.ID, OrganizationID: cur.OrganizationID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("appointments: lock: %w", err)
		}
		if locked.ServiceID.Valid {
			return ErrIntakeStarted
		}
		if locked.Status != StatusScheduled && locked.Status != StatusConfirmed && locked.Status != StatusArrived {
			return ErrInvalidTransition
		}
		if !locked.VehicleID.Valid {
			return invalid("vehicle_id", "is required to start intake")
		}
		customer, err := q.GetUserByID(ctx, locked.CustomerUserID)
		if err != nil {
			return fmt.Errorf("appointments: customer: %w", err)
		}
		vehicle, err := q.GetVehicleByID(ctx, locked.VehicleID.Int64)
		if err != nil {
			return fmt.Errorf("appointments: vehicle: %w", err)
		}
		view, err := s.services.Create(ctx, servicesuc.Caller{Principal: c.Principal, Org: c.Org, Filter: c.Filter}, servicesuc.CreateInput{
			CustomerUUID: customer.Uuid.String(), VehicleUUID: vehicle.Uuid.String(), Notes: strPtr(locked.Note),
		})
		if err != nil {
			return fmt.Errorf("appointments: start intake service: %w", err)
		}
		serviceRow, err := q.GetServiceByUUID(ctx, db.GetServiceByUUIDParams{Uuid: view.UUID, BrandID: c.Org.BrandID})
		if err != nil {
			return fmt.Errorf("appointments: created service lookup: %w", err)
		}
		linked, err := q.UpdateAppointment(ctx, db.UpdateAppointmentParams{
			ID: locked.ID, OrganizationID: locked.OrganizationID,
			CustomerUserID: locked.CustomerUserID, VehicleID: locked.VehicleID, StartsAt: locked.StartsAt, EndsAt: locked.EndsAt,
			EstimatedMinutes: locked.EstimatedMinutes, Source: locked.Source, LeadID: locked.LeadID,
			ServiceID: pgtype.Int8{Int64: serviceRow.ID, Valid: true}, Note: locked.Note,
		})
		if err != nil {
			return fmt.Errorf("appointments: link service: %w", err)
		}
		row, err = q.SetAppointmentStatus(ctx, db.SetAppointmentStatusParams{
			ID: linked.ID, OrganizationID: linked.OrganizationID, Status: StatusArrived,
		})
		if err != nil {
			return fmt.Errorf("appointments: arrived: %w", err)
		}
		return nil
	})
	if err != nil {
		return Appointment{}, err
	}
	return s.oneWithRefs(ctx, row)
}

// Occupancy reports the active booking count against the daily capacity of
// every organization in the read scope (center: the brand network,
// distributor: its subtree, dealer: itself). date is a calendar day; each
// organization's day is bounded in its own time zone.
func (s *Service) Occupancy(ctx context.Context, c Caller, date time.Time) ([]OccupancyRow, error) {
	rows, err := s.q.ListOrganizationsInScope(ctx, db.ListOrganizationsInScopeParams{
		OrgIds: c.Filter.OrgIDsArg(), BrandID: pgtype.Int8{Int64: c.Org.BrandID, Valid: true}, LimitCount: 1000,
	})
	if err != nil {
		return nil, fmt.Errorf("appointments: organizations: %w", err)
	}
	orgs := make([]db.Organization, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	byZone := map[string][]int64{}
	for _, r := range rows {
		if r.Organization.BrandID != c.Org.BrandID || !c.Filter.AllowsOrg(r.Organization.ID, r.Organization.BrandID) {
			continue
		}
		orgs = append(orgs, r.Organization)
		ids = append(ids, r.Organization.ID)
		byZone[r.Organization.Timezone] = append(byZone[r.Organization.Timezone], r.Organization.ID)
	}
	if len(ids) == 0 {
		return []OccupancyRow{}, nil
	}
	settings, err := s.q.ListAppointmentSettingsByOrganizations(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("appointments: occupancy settings: %w", err)
	}
	capByOrg := map[int64]int32{}
	for _, st := range settings {
		capByOrg[st.OrganizationID] = st.DailyVehicleCapacity
	}
	occByOrg := map[int64]int64{}
	for zone, zoneIDs := range byZone {
		dayStart, dayEnd := DayBounds(calendarDay(date, loadLocation(zone)), loadLocation(zone))
		counts, err := s.q.CountActiveAppointmentsByOrganization(ctx, db.CountActiveAppointmentsByOrganizationParams{
			OrganizationIds: zoneIDs, BrandID: c.Org.BrandID, FromTime: tsArg(dayStart), ToTime: tsArg(dayEnd),
		})
		if err != nil {
			return nil, fmt.Errorf("appointments: occupancy count: %w", err)
		}
		for _, r := range counts {
			occByOrg[r.OrganizationID] = r.ActiveCount
		}
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

func (s *Service) save(ctx context.Context, c Caller, orgID int64, cur db.Appointment, in CreateInput, create bool) (db.Appointment, error) {
	in.Source = strings.TrimSpace(in.Source)
	if in.Source == "" {
		in.Source = "panel"
	}
	if err := validateInput(in); err != nil {
		return db.Appointment{}, err
	}
	org, setting, loc, err := s.orgSettings(ctx, orgID)
	if err != nil {
		return db.Appointment{}, err
	}
	if org.BrandID != c.Org.BrandID || !c.Filter.AllowsOrg(org.ID, org.BrandID) {
		return db.Appointment{}, ErrForbidden
	}
	if err := s.checkParties(ctx, org, in); err != nil {
		return db.Appointment{}, err
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
		// The settings row lock serializes bookings of the organization, so
		// the count below cannot race a concurrent insert (capacity re-read
		// under the lock).
		locked, err := q.LockAppointmentSettings(ctx, org.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("appointments: lock settings: %w", err)
		}
		closed, err := q.AppointmentClosureExists(ctx, db.AppointmentClosureExistsParams{
			OrganizationID: org.ID, ClosedOn: dateArg(dateOnly(starts, loc)),
		})
		if err != nil {
			return fmt.Errorf("appointments: closure check: %w", err)
		}
		if closed {
			return ErrDayClosed
		}
		count, err := q.CountActiveAppointmentsForOrganization(ctx, db.CountActiveAppointmentsForOrganizationParams{
			OrganizationID: org.ID, BrandID: org.BrandID, FromTime: tsArg(dayStart), ToTime: tsArg(dayEnd), ExcludeID: exclude,
		})
		if err != nil {
			return fmt.Errorf("appointments: capacity count: %w", err)
		}
		if count >= int64(locked.DailyVehicleCapacity) {
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

// writableByUUID loads an appointment for a write: the read scope must reach
// it and the appointments.write grant must cover its organization (managed
// writes only the active organization; all reaches the brand).
func (s *Service) writableByUUID(ctx context.Context, c Caller, id uuid.UUID) (db.Appointment, error) {
	row, err := s.byUUID(ctx, c, id)
	if err != nil {
		return db.Appointment{}, err
	}
	scope, ok := c.Principal.ScopeFor(rbac.PermAppointmentsWrite)
	if !ok || (scope != rbac.ScopeAll && row.OrganizationID != c.Org.InternalID) {
		return db.Appointment{}, ErrForbidden
	}
	return row, nil
}

// checkParties keeps the booking inside the brand boundary (K20): the
// customer must be a customer of the organization's brand and the vehicle
// must be the customer's vehicle of the same brand.
func (s *Service) checkParties(ctx context.Context, org db.Organization, in CreateInput) error {
	inBrand, err := s.q.CustomerInScope(ctx, db.CustomerInScopeParams{
		UserID: in.CustomerUserID, BrandID: pgtype.Int8{Int64: org.BrandID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("appointments: customer scope: %w", err)
	}
	if !inBrand {
		return invalid("customer_user_id", "customer not found")
	}
	if in.VehicleID == nil {
		return nil
	}
	v, err := s.q.GetVehicleByID(ctx, *in.VehicleID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (v.UserID != in.CustomerUserID || v.BrandID != org.BrandID)) {
		return invalid("vehicle_id", "vehicle not found for this customer")
	}
	if err != nil {
		return fmt.Errorf("appointments: vehicle: %w", err)
	}
	return nil
}

func (s *Service) portalDealer(ctx context.Context, c PortalCaller, id uuid.UUID) (db.GetPortalAppointmentDealerRow, error) {
	if c.UserID <= 0 || c.BrandID <= 0 {
		return db.GetPortalAppointmentDealerRow{}, ErrNotFound
	}
	dealer, err := s.q.GetPortalAppointmentDealer(ctx, db.GetPortalAppointmentDealerParams{Uuid: id, BrandID: c.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.GetPortalAppointmentDealerRow{}, ErrNotFound
	}
	if err != nil {
		return db.GetPortalAppointmentDealerRow{}, fmt.Errorf("appointments: portal dealer: %w", err)
	}
	if s.features != nil {
		on, err := s.features.Enabled(ctx, dealer.ID, features.ModuleAppointments)
		if err != nil {
			return db.GetPortalAppointmentDealerRow{}, fmt.Errorf("appointments: portal feature: %w", err)
		}
		if !on {
			return db.GetPortalAppointmentDealerRow{}, ErrNotFound
		}
	}
	return dealer, nil
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
	return org, setting, loadLocation(org.Timezone), nil
}

func portalSaveCaller(c PortalCaller) Caller {
	return Caller{
		Principal: authctx.Principal{UserInternal: c.UserID, PermissionScopes: map[string]rbac.Scope{
			rbac.PermAppointmentsRead:  rbac.ScopeManaged,
			rbac.PermAppointmentsWrite: rbac.ScopeManaged,
		}},
		Org: orgctx.Scope{BrandID: c.BrandID},
		Filter: scopefilter.Filter{
			Permission: rbac.PermAppointmentsRead,
			Scope:      rbac.ScopeAll,
			UserID:     c.UserID,
			BrandID:    c.BrandID,
		},
	}
}

func portalPeriod(v string) (upcoming, past bool, err error) {
	switch strings.TrimSpace(v) {
	case "", "upcoming":
		return true, false, nil
	case "past":
		return false, true, nil
	case "all":
		return false, false, nil
	default:
		return false, false, invalid("period", "must be upcoming, past or all")
	}
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

// reschedulable: only bookings that have not happened yet can move.
func reschedulable(status string) bool {
	return status == StatusScheduled || status == StatusConfirmed
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
	var organizationName, plate string
	_ = tx.QueryRow(ctx, `SELECT name FROM organizations WHERE id = $1`, a.OrganizationID).Scan(&organizationName)
	if a.VehicleID.Valid {
		_ = tx.QueryRow(ctx, `SELECT COALESCE(plate, '') FROM vehicles WHERE id = $1`, a.VehicleID.Int64).Scan(&plate)
	}
	ev := events.New(name).WithTenant(a.OrganizationID).WithEntity("appointment", &id, &uid).WithPayload(map[string]any{
		"appointment_id": a.ID, "appointment_uuid": a.Uuid.String(), "organization_id": a.OrganizationID, "brand_id": a.BrandID,
		"status": a.Status, "starts_at": a.StartsAt.Time.UTC().Format(time.RFC3339),
		"customer_user_id": a.CustomerUserID, "organization_name": organizationName, "plate": plate,
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

// resolveParties maps customer_uuid / vehicle_uuid of a panel request to the
// internal ids; checkParties still enforces the brand boundary.
func (s *Service) resolveParties(ctx context.Context, in *CreateInput) error {
	if in.CustomerUUID != nil {
		u, err := s.q.GetUserByUUID(ctx, *in.CustomerUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid("customer_uuid", "customer not found")
		}
		if err != nil {
			return fmt.Errorf("appointments: customer uuid: %w", err)
		}
		in.CustomerUserID = u.ID
	}
	if in.VehicleUUID != nil {
		v, err := s.q.GetVehicleByUUID(ctx, *in.VehicleUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid("vehicle_uuid", "vehicle not found for this customer")
		}
		if err != nil {
			return fmt.Errorf("appointments: vehicle uuid: %w", err)
		}
		in.VehicleID = &v.ID
	}
	return nil
}

// withRefs builds the panel views with the customer, vehicle and service
// references (TEC-326) in one query.
func (s *Service) withRefs(ctx context.Context, rows ...db.Appointment) ([]Appointment, error) {
	out := make([]Appointment, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	refs, err := s.q.ListAppointmentRefs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("appointments: refs: %w", err)
	}
	byID := make(map[int64]db.ListAppointmentRefsRow, len(refs))
	for _, ref := range refs {
		byID[ref.ID] = ref
	}
	for _, r := range rows {
		view := appointmentView(r)
		if ref, ok := byID[r.ID]; ok {
			customer := ref.CustomerUuid
			view.CustomerUUID = &customer
			view.CustomerName = strings.TrimSpace(ref.CustomerName + " " + ref.CustomerSurname)
			view.VehicleUUID = uuidPtr(ref.VehicleUuid)
			view.VehiclePlate = textPtr(ref.VehiclePlate)
			if label := strings.TrimSpace(ref.CarBrand.String + " " + ref.CarModel.String); label != "" {
				view.VehicleLabel = &label
			}
			view.ServiceUUID = uuidPtr(ref.ServiceUuid)
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *Service) oneWithRefs(ctx context.Context, row db.Appointment) (Appointment, error) {
	out, err := s.withRefs(ctx, row)
	if err != nil {
		return Appointment{}, err
	}
	return out[0], nil
}

func uuidPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := uuid.UUID(v.Bytes)
	return &id
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

func loadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil || name == "" {
		return time.UTC
	}
	return loc
}

// calendarDay is the calendar date written in t (its own year/month/day),
// as midnight in loc. A YYYY-MM-DD query parameter parses as UTC midnight;
// converting it with t.In(loc) would shift the day west of UTC.
func calendarDay(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
