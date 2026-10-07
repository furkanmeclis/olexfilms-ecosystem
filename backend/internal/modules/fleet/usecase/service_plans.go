package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	appointmentsuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/appointments/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	maxPlanVehicles = 300
	maxPlanDays     = 366
)

var (
	ErrServicePlanStale = &RuleError{
		Code: model.CodeServicePlanStale, Message: "fleet service plan capacity changed; preview again",
		status: http.StatusConflict,
	}
	ErrServicePlanCancelled = &RuleError{
		Code: model.CodeServicePlanCancelled, Message: "fleet service plan is cancelled",
		status: http.StatusConflict,
	}
)

type AppointmentStarter interface {
	StartIntake(ctx context.Context, c appointmentsuc.Caller, id uuid.UUID) (appointmentsuc.Appointment, error)
}

func (s *Service) SetAppointments(a AppointmentStarter) { s.appointments = a }

type ServicePlanInput struct {
	VehicleUUIDs     []uuid.UUID       `json:"vehicle_uuids"`
	ServiceType      string            `json:"service_type"`
	Note             string            `json:"note"`
	StartDate        string            `json:"start_date"`
	DailyMaxVehicles *int32            `json:"daily_max_vehicles"`
	PreferredTimes   []string          `json:"preferred_times"`
	Appointments     []PlanAppointment `json:"appointments"`
}

type PlanAppointment struct {
	VehicleUUID uuid.UUID `json:"vehicle_uuid"`
	StartsAt    time.Time `json:"starts_at"`
}

type PlanWarning struct {
	VehicleUUID uuid.UUID `json:"vehicle_uuid"`
	Date        string    `json:"date"`
	Code        string    `json:"code"`
	Message     string    `json:"message"`
}

type PlanPreview struct {
	FleetUUID    uuid.UUID         `json:"fleet_uuid"`
	DealerUUID   uuid.UUID         `json:"dealer_uuid"`
	ServiceType  string            `json:"service_type"`
	Note         string            `json:"note"`
	Appointments []PlanAppointment `json:"appointments"`
	Warnings     []PlanWarning     `json:"warnings"`
}

type ServicePlanView struct {
	UUID         uuid.UUID                    `json:"uuid"`
	FleetUUID    uuid.UUID                    `json:"fleet_uuid"`
	DealerUUID   uuid.UUID                    `json:"dealer_uuid"`
	Status       string                       `json:"status"`
	ServiceType  string                       `json:"service_type"`
	Note         string                       `json:"note"`
	Appointments []appointmentsuc.Appointment `json:"appointments"`
	Warnings     []PlanWarning                `json:"warnings,omitempty"`
	CreatedAt    time.Time                    `json:"created_at"`
}

type IntakeInput struct {
	AppointmentUUIDs []uuid.UUID `json:"appointment_uuids"`
}

type IntakeRow struct {
	AppointmentUUID uuid.UUID  `json:"appointment_uuid"`
	ServiceUUID     *uuid.UUID `json:"service_uuid,omitempty"`
	OK              bool       `json:"ok"`
	Code            string     `json:"code,omitempty"`
	Message         string     `json:"message,omitempty"`
}

type IntakeResult struct {
	PlanUUID uuid.UUID   `json:"plan_uuid"`
	Results  []IntakeRow `json:"results"`
}

func (s *Service) PreviewServicePlan(ctx context.Context, c Caller, fleetUUID uuid.UUID, in ServicePlanInput) (PlanPreview, error) {
	q := db.New(s.conn)
	a, err := s.resolve(ctx, q, c, fleetUUID)
	if err != nil {
		return PlanPreview{}, err
	}
	if a.own == nil {
		return PlanPreview{}, ErrNoDealerLink
	}
	spec, err := s.preparePlanSpec(ctx, q, c, a, in)
	if err != nil {
		return PlanPreview{}, err
	}
	items, warnings, err := s.schedulePlan(ctx, q, c, a, spec, nil, false)
	if err != nil {
		return PlanPreview{}, err
	}
	return PlanPreview{
		FleetUUID: a.fleet.Organization.Uuid, DealerUUID: spec.dealer.Uuid,
		ServiceType: spec.serviceType, Note: spec.note,
		Appointments: items, Warnings: warnings,
	}, nil
}

func (s *Service) CreateServicePlan(ctx context.Context, c Caller, fleetUUID uuid.UUID, in ServicePlanInput, idempotencyKey string) (ServicePlanView, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > 128 {
		return ServicePlanView{}, &ValidationError{Field: "Idempotency-Key", Message: "must be at most 128 characters"}
	}
	var out ServicePlanView
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		if idempotencyKey != "" {
			existing, err := q.GetFleetServicePlanByIdempotency(ctx, db.GetFleetServicePlanByIdempotencyParams{
				OrganizationID: c.OrgID, IdempotencyKey: idempotencyKey,
			})
			if err == nil {
				view, err := s.planView(ctx, q, c, existing, nil)
				out = view
				return err
			}
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("fleet plan: idempotency: %w", err)
			}
		}
		a, err := s.resolve(ctx, q, c, fleetUUID)
		if err != nil {
			return err
		}
		if a.own == nil {
			return ErrNoDealerLink
		}
		spec, err := s.preparePlanSpec(ctx, q, c, a, in)
		if err != nil {
			return err
		}
		if _, err := q.LockAppointmentSettings(ctx, spec.dealer.ID); err != nil {
			return fmt.Errorf("fleet plan: lock appointment settings: %w", err)
		}
		items := in.Appointments
		var warnings []PlanWarning
		if len(items) == 0 {
			items, warnings, err = s.schedulePlan(ctx, q, c, a, spec, nil, true)
		} else {
			warnings, err = s.validateEditedPlan(ctx, q, a, spec, items, nil)
		}
		if err != nil {
			return err
		}
		rawTimes, _ := json.Marshal(spec.preferredTimes)
		title := strings.TrimSpace(a.fleet.Organization.Name + " - " + spec.serviceType)
		plan, err := q.CreateFleetServicePlan(ctx, db.CreateFleetServicePlanParams{
			OrganizationID: spec.dealer.ID, BrandID: spec.dealer.BrandID,
			FleetOrgID: a.fleet.Organization.ID, FleetLinkID: a.own.ID, Title: title,
			ServiceType: spec.serviceType, Note: spec.note, StartDate: dateArg(spec.startDate),
			DailyVehicleLimit: spec.dailyLimit, PreferredTimes: rawTimes,
			IdempotencyKey: textNarg(idempotencyKey), CreatedByUserID: int8Arg(c.UserID),
		})
		if err != nil {
			return fmt.Errorf("fleet plan: create plan: %w", err)
		}
		for _, item := range items {
			v := spec.vehicles[item.VehicleUUID]
			mins := spec.settings.DefaultEstimatedMinutes
			row, err := q.CreateAppointment(ctx, db.CreateAppointmentParams{
				OrganizationID: spec.dealer.ID, BrandID: spec.dealer.BrandID,
				CustomerUserID: v.UserID, VehicleID: pgtype.Int8{Int64: v.ID, Valid: true},
				StartsAt: tsArg(item.StartsAt.UTC()), EndsAt: tsArg(item.StartsAt.UTC().Add(time.Duration(mins) * time.Minute)),
				EstimatedMinutes: mins, Source: "fleet_plan", Status: appointmentsuc.StatusScheduled,
				Note: spec.note, CreatedByUserID: int8Arg(c.UserID), PlanID: pgtype.Int8{Int64: plan.ID, Valid: true},
			})
			if err != nil {
				return fmt.Errorf("fleet plan: create appointment: %w", err)
			}
			if err := s.emitAppointment(ctx, tx, events.AppointmentCreated, row); err != nil {
				return err
			}
		}
		notify, err := q.ListActiveFleetUserIDs(ctx, a.fleet.Organization.ID)
		if err != nil {
			return fmt.Errorf("fleet plan: users: %w", err)
		}
		if err := s.emit(ctx, tx, events.FleetServicePlanCreated, spec.dealer.ID, c.UserID, a.fleet.Organization, map[string]any{
			"plan_uuid": plan.Uuid.String(), "appointment_count": len(items), "notify_user_ids": notify,
		}); err != nil {
			return err
		}
		view, err := s.planView(ctx, q, c, plan, warnings)
		out = view
		return err
	})
	return out, err
}

func (s *Service) CancelServicePlan(ctx context.Context, c Caller, fleetUUID, planUUID uuid.UUID, reason string) (ServicePlanView, error) {
	var out ServicePlanView
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		a, err := s.resolve(ctx, q, c, fleetUUID)
		if err != nil {
			return err
		}
		if a.own == nil {
			return ErrNoDealerLink
		}
		plan, err := q.GetFleetServicePlanByUUID(ctx, db.GetFleetServicePlanByUUIDParams{Uuid: planUUID, OrganizationID: c.OrgID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && plan.FleetOrgID != a.orgID()) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("fleet plan: get: %w", err)
		}
		if plan.Status == "cancelled" {
			return ErrServicePlanCancelled
		}
		reason = strings.TrimSpace(reason)
		if reason == "" {
			reason = "fleet plan cancelled"
		}
		plan, err = q.CancelFleetServicePlan(ctx, db.CancelFleetServicePlanParams{
			ID: plan.ID, OrganizationID: c.OrgID, CancelReason: textNarg(reason), CancelledByUserID: int8Arg(c.UserID),
		})
		if err != nil {
			return fmt.Errorf("fleet plan: cancel: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE appointments SET status = 'cancelled', cancel_reason = $3
			WHERE plan_id = $1 AND organization_id = $2 AND deleted_at IS NULL
			  AND status IN ('scheduled', 'confirmed')`, plan.ID, c.OrgID, reason); err != nil {
			return fmt.Errorf("fleet plan: cancel appointments: %w", err)
		}
		view, err := s.planView(ctx, q, c, plan, nil)
		out = view
		return err
	})
	return out, err
}

func (s *Service) StartServicePlanIntake(ctx context.Context, c Caller, fleetUUID, planUUID uuid.UUID, in IntakeInput) (IntakeResult, error) {
	if s.appointments == nil {
		return IntakeResult{}, fmt.Errorf("fleet plan: appointments usecase is nil")
	}
	q := db.New(s.conn)
	a, err := s.resolve(ctx, q, c, fleetUUID)
	if err != nil {
		return IntakeResult{}, err
	}
	if a.own == nil {
		return IntakeResult{}, ErrNoDealerLink
	}
	plan, err := q.GetFleetServicePlanByUUID(ctx, db.GetFleetServicePlanByUUIDParams{Uuid: planUUID, OrganizationID: c.OrgID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && plan.FleetOrgID != a.orgID()) {
		return IntakeResult{}, ErrNotFound
	}
	if err != nil {
		return IntakeResult{}, fmt.Errorf("fleet plan: get: %w", err)
	}
	rows, err := q.ListAppointmentsByPlan(ctx, db.ListAppointmentsByPlanParams{PlanID: plan.ID, OrganizationID: c.OrgID})
	if err != nil {
		return IntakeResult{}, fmt.Errorf("fleet plan: appointments: %w", err)
	}
	selected := map[uuid.UUID]bool{}
	for _, id := range in.AppointmentUUIDs {
		selected[id] = true
	}
	var out IntakeResult
	out.PlanUUID = plan.Uuid
	ac := appointmentsuc.Caller{Principal: c.Principal, Org: c.Org, Filter: c.Filter}
	for _, row := range rows {
		if len(selected) > 0 && !selected[row.Uuid] {
			continue
		}
		res := IntakeRow{AppointmentUUID: row.Uuid}
		got, err := s.appointments.StartIntake(ctx, ac, row.Uuid)
		if err != nil {
			res.Code, res.Message = intakeErr(err)
		} else {
			res.OK = true
			res.ServiceUUID = got.ServiceUUID
		}
		out.Results = append(out.Results, res)
	}
	return out, nil
}

type planSpec struct {
	dealer         db.Organization
	settings       db.AppointmentSetting
	location       *time.Location
	vehicles       map[uuid.UUID]db.ListFleetPlanVehiclesRow
	vehicleOrder   []uuid.UUID
	serviceType    string
	note           string
	startDate      time.Time
	dailyLimit     int32
	preferredTimes []string
}

func (s *Service) preparePlanSpec(ctx context.Context, q *db.Queries, c Caller, a access, in ServicePlanInput) (planSpec, error) {
	if len(in.VehicleUUIDs) == 0 {
		return planSpec{}, &ValidationError{Field: "vehicle_uuids", Message: "is required"}
	}
	if len(in.VehicleUUIDs) > maxPlanVehicles {
		return planSpec{}, &ValidationError{Field: "vehicle_uuids", Message: "too many vehicles"}
	}
	serviceType := strings.TrimSpace(in.ServiceType)
	if serviceType == "" {
		return planSpec{}, &ValidationError{Field: "service_type", Message: "is required"}
	}
	if len(serviceType) > 120 {
		return planSpec{}, &ValidationError{Field: "service_type", Message: "must be at most 120 characters"}
	}
	note := strings.TrimSpace(in.Note)
	if len([]rune(note)) > 20000 {
		return planSpec{}, &ValidationError{Field: "note", Message: "must be at most 20000 characters"}
	}
	start, err := time.Parse(time.DateOnly, strings.TrimSpace(in.StartDate))
	if err != nil {
		return planSpec{}, &ValidationError{Field: "start_date", Message: "must be YYYY-MM-DD"}
	}
	dealer, err := q.GetOrganizationByID(ctx, c.OrgID)
	if err != nil {
		return planSpec{}, fmt.Errorf("fleet plan: dealer: %w", err)
	}
	settings, err := q.GetAppointmentSettings(ctx, dealer.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return planSpec{}, ErrNotFound
	}
	if err != nil {
		return planSpec{}, fmt.Errorf("fleet plan: settings: %w", err)
	}
	limit := settings.DailyVehicleCapacity
	if in.DailyMaxVehicles != nil {
		if *in.DailyMaxVehicles <= 0 {
			return planSpec{}, &ValidationError{Field: "daily_max_vehicles", Message: "must be greater than zero"}
		}
		limit = *in.DailyMaxVehicles
	}
	preferred := make([]string, 0, len(in.PreferredTimes))
	for _, raw := range in.PreferredTimes {
		v := strings.TrimSpace(raw)
		if _, _, err := parseClock(v); err != nil {
			return planSpec{}, &ValidationError{Field: "preferred_times", Message: "must contain HH:MM values"}
		}
		preferred = append(preferred, v)
	}
	rows, err := q.ListFleetPlanVehicles(ctx, db.ListFleetPlanVehiclesParams{FleetOrgID: a.orgID(), VehicleUuids: in.VehicleUUIDs})
	if err != nil {
		return planSpec{}, fmt.Errorf("fleet plan: vehicles: %w", err)
	}
	if len(rows) != len(in.VehicleUUIDs) {
		return planSpec{}, ErrNotFound
	}
	vehicles := map[uuid.UUID]db.ListFleetPlanVehiclesRow{}
	order := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		vehicles[r.Uuid] = r
		order = append(order, r.Uuid)
	}
	return planSpec{
		dealer: dealer, settings: settings, location: loadLocation(dealer.Timezone),
		vehicles: vehicles, vehicleOrder: order, serviceType: serviceType, note: note,
		startDate: start, dailyLimit: limit, preferredTimes: preferred,
	}, nil
}

func (s *Service) schedulePlan(ctx context.Context, q *db.Queries, c Caller, a access, spec planSpec, excludePlan *int64, strict bool) ([]PlanAppointment, []PlanWarning, error) {
	queue := append([]uuid.UUID(nil), spec.vehicleOrder...)
	var out []PlanAppointment
	var warnings []PlanWarning
	for day, guard := spec.startDate, 0; len(queue) > 0; day, guard = day.AddDate(0, 0, 1), guard+1 {
		if guard > maxPlanDays {
			return nil, nil, ErrServicePlanStale
		}
		slots, remaining, err := s.daySlots(ctx, q, spec, day, excludePlan, strict)
		if err != nil {
			return nil, nil, err
		}
		if remaining <= 0 || len(slots) == 0 {
			continue
		}
		n := minInt(len(queue), minInt(int(remaining), len(slots)))
		for i := 0; i < n; i++ {
			vehicleID := queue[0]
			queue = queue[1:]
			at := slots[i].Start
			out = append(out, PlanAppointment{VehicleUUID: vehicleID, StartsAt: at})
			w, err := s.conflictWarning(ctx, q, spec, vehicleID, at, excludePlan)
			if err != nil {
				return nil, nil, err
			}
			if w != nil {
				warnings = append(warnings, *w)
			}
		}
	}
	return out, warnings, nil
}

func (s *Service) validateEditedPlan(ctx context.Context, q *db.Queries, a access, spec planSpec, items []PlanAppointment, excludePlan *int64) ([]PlanWarning, error) {
	seen := map[uuid.UUID]bool{}
	byDay := map[string]int{}
	var warnings []PlanWarning
	for _, item := range items {
		if _, ok := spec.vehicles[item.VehicleUUID]; !ok {
			return nil, &ValidationError{Field: "appointments.vehicle_uuid", Message: "vehicle is not in the fleet"}
		}
		if item.StartsAt.IsZero() {
			return nil, &ValidationError{Field: "appointments.starts_at", Message: "is required"}
		}
		seen[item.VehicleUUID] = true
		day := dateOnly(item.StartsAt, spec.location)
		byDay[day.Format(time.DateOnly)]++
		w, err := s.conflictWarning(ctx, q, spec, item.VehicleUUID, item.StartsAt, excludePlan)
		if err != nil {
			return nil, err
		}
		if w != nil {
			warnings = append(warnings, *w)
		}
	}
	if len(seen) != len(spec.vehicleOrder) {
		return nil, &ValidationError{Field: "appointments", Message: "must include every requested vehicle exactly once"}
	}
	for raw, planned := range byDay {
		day, _ := time.Parse(time.DateOnly, raw)
		_, remaining, err := s.daySlots(ctx, q, spec, day, excludePlan, true)
		if err != nil {
			return nil, err
		}
		if planned > int(remaining) {
			return nil, ErrServicePlanStale
		}
	}
	return warnings, nil
}

func (s *Service) daySlots(ctx context.Context, q *db.Queries, spec planSpec, day time.Time, excludePlan *int64, strict bool) ([]appointmentsuc.Slot, int32, error) {
	closed, err := q.AppointmentClosureExists(ctx, db.AppointmentClosureExistsParams{
		OrganizationID: spec.dealer.ID, ClosedOn: dateArg(day),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("fleet plan: closure: %w", err)
	}
	if closed {
		return nil, 0, nil
	}
	dayStart, dayEnd := appointmentsuc.DayBounds(day, spec.location)
	count, err := q.CountActiveAppointmentsForOrganization(ctx, db.CountActiveAppointmentsForOrganizationParams{
		OrganizationID: spec.dealer.ID, BrandID: spec.dealer.BrandID,
		FromTime: tsArg(dayStart), ToTime: tsArg(dayEnd),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("fleet plan: count: %w", err)
	}
	capacity := spec.settings.DailyVehicleCapacity
	if spec.dailyLimit < capacity {
		capacity = spec.dailyLimit
	}
	remaining := int64(capacity) - count
	if remaining <= 0 {
		if strict {
			return nil, 0, ErrServicePlanStale
		}
		return nil, 0, nil
	}
	windows, err := appointmentsuc.ParseWorkingHours(spec.settings.WorkingHours, day.In(spec.location).Weekday())
	if err != nil {
		return nil, 0, &ValidationError{Field: "working_hours", Message: err.Error()}
	}
	av, err := appointmentsuc.BuildDayAvailability(appointmentsuc.DailyRule{
		Date: day, Location: spec.location, Capacity: capacity, Occupied: count,
		Interval: time.Duration(spec.settings.SlotIntervalMinutes) * time.Minute,
		Duration: time.Duration(spec.settings.DefaultEstimatedMinutes) * time.Minute,
		Windows:  windows, Now: s.now(),
	})
	if err != nil {
		return nil, 0, &ValidationError{Field: "working_hours", Message: err.Error()}
	}
	slots := orderPreferredSlots(av.Slots, spec.preferredTimes, spec.location)
	if len(slots) == 0 && strict {
		return nil, 0, ErrServicePlanStale
	}
	return slots, int32(remaining), nil
}

func (s *Service) conflictWarning(ctx context.Context, q *db.Queries, spec planSpec, vehicleUUID uuid.UUID, at time.Time, excludePlan *int64) (*PlanWarning, error) {
	v := spec.vehicles[vehicleUUID]
	dayStart, dayEnd := appointmentsuc.DayBounds(at, spec.location)
	var ex pgtype.Int8
	if excludePlan != nil {
		ex = pgtype.Int8{Int64: *excludePlan, Valid: true}
	}
	has, err := q.VehicleHasActiveAppointmentOnDay(ctx, db.VehicleHasActiveAppointmentOnDayParams{
		VehicleID: v.ID, BrandID: spec.dealer.BrandID, FromTime: tsArg(dayStart), ToTime: tsArg(dayEnd), ExcludePlanID: ex,
	})
	if err != nil {
		return nil, fmt.Errorf("fleet plan: conflict: %w", err)
	}
	if !has {
		return nil, nil
	}
	return &PlanWarning{
		VehicleUUID: vehicleUUID, Date: at.In(spec.location).Format(time.DateOnly),
		Code: "FLEET_PLAN_VEHICLE_DAY_CONFLICT", Message: "vehicle has another active appointment on this day",
	}, nil
}

func (s *Service) planView(ctx context.Context, q *db.Queries, c Caller, plan db.FleetServicePlan, warnings []PlanWarning) (ServicePlanView, error) {
	rows, err := q.ListAppointmentsByPlan(ctx, db.ListAppointmentsByPlanParams{PlanID: plan.ID, OrganizationID: plan.OrganizationID})
	if err != nil {
		return ServicePlanView{}, fmt.Errorf("fleet plan: appointments: %w", err)
	}
	appts := make([]appointmentsuc.Appointment, 0, len(rows))
	for _, row := range rows {
		appts = append(appts, appointmentsuc.Appointment{
			UUID: row.Uuid, OrganizationID: row.OrganizationID, CustomerUserID: row.CustomerUserID,
			VehicleID: intPtr(row.VehicleID), StartsAt: row.StartsAt.Time, EndsAt: row.EndsAt.Time,
			EstimatedMinutes: row.EstimatedMinutes, Source: row.Source, Status: row.Status,
			PlanID: intPtr(row.PlanID), ServiceID: intPtr(row.ServiceID), Note: row.Note,
		})
	}
	fleetUUID := c.Org.UUID
	if f, err := q.GetOrganizationByID(ctx, plan.FleetOrgID); err == nil {
		fleetUUID = f.Uuid
	}
	return ServicePlanView{
		UUID: plan.Uuid, FleetUUID: fleetUUID, DealerUUID: c.Org.UUID, Status: plan.Status,
		ServiceType: plan.ServiceType, Note: plan.Note, Appointments: appts,
		Warnings: warnings, CreatedAt: plan.CreatedAt.Time,
	}, nil
}

func (s *Service) emitAppointment(ctx context.Context, tx pgx.Tx, name string, a db.Appointment) error {
	if s.out == nil {
		return nil
	}
	id, uid := a.ID, a.Uuid
	ev := events.New(name).WithTenant(a.OrganizationID).WithEntity("appointment", &id, &uid).WithPayload(map[string]any{
		"appointment_id": a.ID, "appointment_uuid": a.Uuid.String(), "organization_id": a.OrganizationID,
		"brand_id": a.BrandID, "status": a.Status, "starts_at": a.StartsAt.Time.UTC().Format(time.RFC3339),
		"customer_user_id": a.CustomerUserID,
	})
	if a.CreatedByUserID.Valid {
		ev = ev.WithActor(a.CreatedByUserID.Int64)
	}
	if err := s.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("fleet plan: outbox appointment: %w", err)
	}
	return nil
}

func orderPreferredSlots(slots []appointmentsuc.Slot, preferred []string, loc *time.Location) []appointmentsuc.Slot {
	if len(preferred) == 0 || len(slots) == 0 {
		return slots
	}
	rank := map[string]int{}
	for i, v := range preferred {
		rank[v] = i
	}
	out := append([]appointmentsuc.Slot(nil), slots...)
	sort.SliceStable(out, func(i, j int) bool {
		ki, iok := rank[out[i].Start.In(loc).Format("15:04")]
		kj, jok := rank[out[j].Start.In(loc).Format("15:04")]
		if iok != jok {
			return iok
		}
		if iok && ki != kj {
			return ki < kj
		}
		return out[i].Start.Before(out[j].Start)
	})
	return out
}

func intakeErr(err error) (string, string) {
	switch {
	case errors.Is(err, appointmentsuc.ErrIntakeStarted):
		return appointmentsuc.CodeIntakeStarted, "appointment intake is already started"
	case errors.Is(err, appointmentsuc.ErrInvalidTransition):
		return appointmentsuc.CodeInvalidTransition, "appointment status transition is not allowed"
	case errors.Is(err, appointmentsuc.ErrNotFound):
		return "NOT_FOUND", "appointment not found"
	default:
		return "ERROR", err.Error()
	}
}

func dateArg(t time.Time) pgtype.Date {
	return pgtype.Date{Time: t, Valid: true}
}

func tsArg(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func int8Arg(v int64) pgtype.Int8 {
	return pgtype.Int8{Int64: v, Valid: v != 0}
}

func textNarg(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	return pgtype.Text{String: v, Valid: v != ""}
}

func intPtr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func loadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(strings.TrimSpace(name))
	if err != nil || loc == nil {
		return time.UTC
	}
	return loc
}

func dateOnly(t time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func parseClock(v string) (int, int, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(v))
	if err != nil {
		return 0, 0, err
	}
	return t.Hour(), t.Minute(), nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
