package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TEC-477 (F5-02f): panel reads of the plans and reports of a fleet.

var (
	// PlanSort is the sort contract of the panel plan list.
	PlanSort = apiquery.SortSpec{
		Columns: apiquery.SortColumns{"created_at": "created_at", "start_date": "start_date"},
		Default: apiquery.SortField{Field: "created_at", Desc: true},
	}
	// ReportSort is the sort contract of the panel report list.
	ReportSort = apiquery.SortSpec{
		Columns: apiquery.SortColumns{"period_start": "period_start"},
		Default: apiquery.SortField{Field: "period_start", Desc: true},
	}
)

// PlanFilter is GET /v1/fleets/{uuid}/service-plans.
type PlanFilter struct {
	Statuses []string
	Sort     []apiquery.SortField
	Limit    int32
	Offset   int32
}

// PlanSummary is one row of the panel plan list.
type PlanSummary struct {
	UUID              uuid.UUID `json:"uuid"`
	Status            string    `json:"status"`
	ServiceType       string    `json:"service_type"`
	Note              string    `json:"note"`
	StartDate         string    `json:"start_date"`
	DailyVehicleLimit int32     `json:"daily_vehicle_limit"`
	AppointmentCount  int64     `json:"appointment_count"`
	IntakeCount       int64     `json:"intake_count"`
	CreatedAt         time.Time `json:"created_at"`
}

// ListServicePlans lists the caller organization's plans of a fleet.
func (s *Service) ListServicePlans(ctx context.Context, c Caller, fleetUUID uuid.UUID, f PlanFilter) ([]PlanSummary, int64, error) {
	srt, err := apiquery.ResolveSort(f.Sort, PlanSort)
	if err != nil {
		return nil, 0, err
	}
	q := db.New(s.conn)
	a, err := s.resolve(ctx, q, c, fleetUUID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.ListFleetServicePlansOfOrg(ctx, db.ListFleetServicePlansOfOrgParams{
		OrganizationID: c.OrgID, FleetOrgID: a.orgID(), Statuses: f.Statuses,
		SortField: srt.Key, SortDesc: srt.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("fleet plan: list: %w", err)
	}
	total, err := q.CountFleetServicePlansOfOrg(ctx, db.CountFleetServicePlansOfOrgParams{
		OrganizationID: c.OrgID, FleetOrgID: a.orgID(), Statuses: f.Statuses,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("fleet plan: count: %w", err)
	}
	out := make([]PlanSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, PlanSummary{
			UUID: r.Uuid, Status: r.Status, ServiceType: r.ServiceType, Note: r.Note,
			StartDate: r.StartDate.Time.Format(time.DateOnly), DailyVehicleLimit: r.DailyVehicleLimit,
			AppointmentCount: r.AppointmentCount, IntakeCount: r.IntakeCount, CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, total, nil
}

// GetServicePlan is the plan detail: its appointments with the vehicle
// plate and label and the draft service opened by the intake.
func (s *Service) GetServicePlan(ctx context.Context, c Caller, fleetUUID, planUUID uuid.UUID) (ServicePlanView, error) {
	q := db.New(s.conn)
	a, err := s.resolve(ctx, q, c, fleetUUID)
	if err != nil {
		return ServicePlanView{}, err
	}
	plan, err := q.GetFleetServicePlanByUUID(ctx, db.GetFleetServicePlanByUUIDParams{Uuid: planUUID, OrganizationID: c.OrgID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && plan.FleetOrgID != a.orgID()) {
		return ServicePlanView{}, ErrNotFound
	}
	if err != nil {
		return ServicePlanView{}, fmt.Errorf("fleet plan: get: %w", err)
	}
	view, err := s.planView(ctx, q, c, plan, nil)
	if err != nil {
		return ServicePlanView{}, err
	}
	return view, s.labelPlanAppointments(ctx, q, plan, &view)
}

// labelPlanAppointments fills the panel references of the appointments.
func (s *Service) labelPlanAppointments(ctx context.Context, q *db.Queries, plan db.FleetServicePlan, view *ServicePlanView) error {
	refs, err := q.ListFleetPlanAppointmentRefs(ctx, db.ListFleetPlanAppointmentRefsParams{
		PlanID: plan.ID, OrganizationID: plan.OrganizationID,
	})
	if err != nil {
		return fmt.Errorf("fleet plan: appointment refs: %w", err)
	}
	byUUID := make(map[uuid.UUID]db.ListFleetPlanAppointmentRefsRow, len(refs))
	for _, r := range refs {
		byUUID[r.AppointmentUuid] = r
	}
	for i := range view.Appointments {
		ap := &view.Appointments[i]
		r, ok := byUUID[ap.UUID]
		if !ok {
			continue
		}
		if r.VehicleUuid.Valid {
			id := uuid.UUID(r.VehicleUuid.Bytes)
			ap.VehicleUUID = &id
			ap.VehiclePlate = textPtr(r.Plate)
			if label := joinLabel(r.CarBrandName.String, r.CarModelName.String); label != "" {
				ap.VehicleLabel = &label
			}
		}
		if r.ServiceUuid.Valid {
			id := uuid.UUID(r.ServiceUuid.Bytes)
			ap.ServiceUUID = &id
		}
	}
	return nil
}

func joinLabel(brand, model string) string {
	switch {
	case brand == "":
		return model
	case model == "":
		return brand
	}
	return brand + " " + model
}

// ReportFilter is GET /v1/fleets/{uuid}/reports.
type ReportFilter struct {
	Statuses    []string
	PeriodKinds []string
	Sort        []apiquery.SortField
	Limit       int32
	Offset      int32
}

// ListReports lists the periodic reports of a fleet the caller reaches.
func (s *Service) ListReports(ctx context.Context, c Caller, fleetUUID uuid.UUID, f ReportFilter) ([]ReportView, int64, error) {
	srt, err := apiquery.ResolveSort(f.Sort, ReportSort)
	if err != nil {
		return nil, 0, err
	}
	q := db.New(s.conn)
	a, err := s.resolve(ctx, q, c, fleetUUID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.ListFleetReportsSorted(ctx, db.ListFleetReportsSortedParams{
		FleetOrgID: a.orgID(), Statuses: f.Statuses, PeriodKinds: f.PeriodKinds,
		SortDesc: srt.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("fleet: list reports: %w", err)
	}
	total, err := q.CountFleetReportsFiltered(ctx, db.CountFleetReportsFilteredParams{
		FleetOrgID: a.orgID(), Statuses: f.Statuses, PeriodKinds: f.PeriodKinds,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("fleet: count reports: %w", err)
	}
	out := make([]ReportView, 0, len(rows))
	for _, r := range rows {
		out = append(out, reportView(r))
	}
	return out, total, nil
}

// ReportFile is the PDF of a ready report of a fleet the caller reaches.
func (s *Service) ReportFile(ctx context.Context, c Caller, fleetUUID, reportUUID uuid.UUID) (io.ReadCloser, string, error) {
	q := db.New(s.conn)
	a, err := s.resolve(ctx, q, c, fleetUUID)
	if err != nil {
		return nil, "", err
	}
	r, err := q.GetFleetReportByUUID(ctx, reportUUID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (r.FleetOrgID != a.orgID() || r.Status != model.ReportReady || !r.StorageKey.Valid)) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("fleet: report: %w", err)
	}
	if s.files == nil {
		return nil, "", ErrReportFilesUnavailable
	}
	rc, _, err := s.files.Download(ctx, r.StorageKey.String)
	if err != nil {
		return nil, "", fmt.Errorf("fleet: report file: %w", err)
	}
	return rc, fmt.Sprintf("fleet-report-%s-%s.pdf", r.PeriodKind, r.PeriodStart.Time.Format(time.DateOnly)), nil
}

// Lookup is GET /v1/fleets/lookup (TEC-477): the panel's "new fleet"
// dialog asks for the VKN/TCKN first. A fleet of the brand with that
// number is returned with the caller's open link (the dialog then offers a
// link request); none is ErrNotFound (the dialog shows the opening form).
// A wrong checksum is FLEET_INVALID_TAX_NUMBER. Same reach as Open.
func (s *Service) Lookup(ctx context.Context, c Caller, taxNumber string) (FleetExistsError, error) {
	taxNumber = strings.TrimSpace(taxNumber)
	if taxNumber == "" {
		return FleetExistsError{}, &ValidationError{Field: "tax_number", Message: "required"}
	}
	if !model.ValidTaxNumber(taxNumber) {
		return FleetExistsError{}, ErrInvalidTaxNumber
	}
	opener, err := db.New(s.conn).GetOrganizationByID(ctx, c.OrgID)
	if err != nil {
		return FleetExistsError{}, fmt.Errorf("fleet: opener: %w", err)
	}
	if opener.Type != "dealer" && opener.Type != "distributor" {
		return FleetExistsError{}, ErrForbidden
	}
	if _, err := repository.New(s.conn).FindByTaxNumber(ctx, opener.BrandID, taxNumber); errors.Is(err, pgx.ErrNoRows) {
		return FleetExistsError{}, ErrNotFound
	} else if err != nil {
		return FleetExistsError{}, fmt.Errorf("fleet: lookup: %w", err)
	}
	var found *FleetExistsError
	if !errors.As(s.existsError(ctx, opener, taxNumber), &found) {
		return FleetExistsError{}, fmt.Errorf("fleet: lookup: no match")
	}
	return *found, nil
}
