package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

// JobView is the API view of a job.
type JobView struct {
	UUID        uuid.UUID       `json:"uuid"`
	Action      string          `json:"action"`
	Reversible  bool            `json:"reversible"`
	Params      json.RawMessage `json:"params"`
	Selection   json.RawMessage `json:"selection"`
	Status      string          `json:"status"`
	Total       int32           `json:"total"`
	Eligible    int32           `json:"eligible"`
	Excluded    int32           `json:"excluded"`
	Succeeded   int32           `json:"succeeded"`
	Failed      int32           `json:"failed"`
	Reverted    int32           `json:"reverted"`
	Error       *string         `json:"error"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
	StartedAt   *time.Time      `json:"started_at"`
	FinishedAt  *time.Time      `json:"finished_at"`
	UndoneAt    *time.Time      `json:"undone_at"`
	Breakdown   []ReasonCount   `json:"breakdown,omitempty"`
	SampleOK    []ItemView      `json:"sample_eligible,omitempty"`
	SampleNotOK []ItemView      `json:"sample_excluded,omitempty"`
}

// ReasonCount counts items per status and reason.
type ReasonCount struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
	Count  int32  `json:"count"`
}

// ItemView is one unit of a job.
type ItemView struct {
	UnitUUID   uuid.UUID `json:"unit_uuid"`
	Barcode    string    `json:"barcode"`
	Product    Product   `json:"product"`
	UnitStatus string    `json:"unit_status"`
	Status     string    `json:"status"`
	Reason     *string   `json:"reason"`
	// Meters is the roll length a conversion gives the unit; UnitMeters
	// the unit's current length.
	Meters     *string   `json:"meters"`
	UnitMeters *string   `json:"unit_meters"`
	Lot        *string   `json:"lot"`
	Note       *string   `json:"note"`
	Error      *string   `json:"error"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Product names a product.
type Product struct {
	UUID uuid.UUID `json:"uuid"`
	SKU  string    `json:"sku"`
	Name string    `json:"name"`
}

// Get returns a job with its breakdown.
func (s *Service) Get(ctx context.Context, c Caller, id uuid.UUID) (JobView, error) {
	job, err := s.q.GetStockBulkJob(ctx, db.GetStockBulkJobParams{Uuid: id, OrganizationID: c.OrgID})
	if errors.Is(err, pgx.ErrNoRows) {
		return JobView{}, ErrNotFound
	}
	if err != nil {
		return JobView{}, fmt.Errorf("stockbulk: job: %w", err)
	}
	return s.view(ctx, job, job.Status == StatusPreview)
}

func (s *Service) view(ctx context.Context, j db.StockBulkJob, samples bool) (JobView, error) {
	v := JobView{
		UUID: j.Uuid, Action: j.Action, Reversible: Reversible(j.Action), Params: j.Params, Selection: j.Selection,
		Status: j.Status, Total: j.Total, Eligible: j.Eligible, Excluded: j.Excluded, Succeeded: j.Succeeded,
		Failed: j.Failed, Reverted: j.Reverted, CreatedAt: j.CreatedAt.Time,
	}
	if j.Error.Valid {
		v.Error = &j.Error.String
	}
	for _, t := range []struct {
		dst **time.Time
		src time.Time
		ok  bool
	}{{&v.StartedAt, j.StartedAt.Time, j.StartedAt.Valid}, {&v.FinishedAt, j.FinishedAt.Time, j.FinishedAt.Valid},
		{&v.UndoneAt, j.UndoneAt.Time, j.UndoneAt.Valid}} {
		if t.ok {
			tt := t.src
			*t.dst = &tt
		}
	}
	if u, err := s.q.GetUserByID(ctx, j.CreatedBy); err == nil {
		v.CreatedBy = u.Name + " " + u.Surname
	}
	counts, err := s.q.CountStockBulkJobItemsByReason(ctx, j.ID)
	if err != nil {
		return v, fmt.Errorf("stockbulk: breakdown: %w", err)
	}
	for _, c := range counts {
		v.Breakdown = append(v.Breakdown, ReasonCount{Status: c.Status, Reason: c.Reason, Count: c.N})
	}
	if samples {
		if v.SampleOK, _, err = s.items(ctx, j.ID, []string{ItemEligible}, sampleSize, 0); err != nil {
			return v, err
		}
		if v.SampleNotOK, _, err = s.items(ctx, j.ID, []string{ItemExcluded}, sampleSize, 0); err != nil {
			return v, err
		}
	}
	return v, nil
}

// ItemStatuses are the item status filter values.
var ItemStatuses = []string{ItemEligible, ItemExcluded, ItemApplied, ItemFailed, ItemReverted, ItemRevertSkipped}

// Items lists the units of a job.
func (s *Service) Items(ctx context.Context, c Caller, id uuid.UUID, statuses []string, limit, offset int32) ([]ItemView, int64, error) {
	job, err := s.q.GetStockBulkJob(ctx, db.GetStockBulkJobParams{Uuid: id, OrganizationID: c.OrgID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("stockbulk: job: %w", err)
	}
	return s.items(ctx, job.ID, statuses, limit, offset)
}

func (s *Service) items(ctx context.Context, jobID int64, statuses []string, limit, offset int32) ([]ItemView, int64, error) {
	total, err := s.q.CountStockBulkJobItems(ctx, db.CountStockBulkJobItemsParams{JobID: jobID, Statuses: statuses})
	if err != nil {
		return nil, 0, fmt.Errorf("stockbulk: count items: %w", err)
	}
	rows, err := s.q.ListStockBulkJobItems(ctx, db.ListStockBulkJobItemsParams{
		JobID: jobID, Statuses: statuses, RowLimit: limit, RowOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stockbulk: items: %w", err)
	}
	out := make([]ItemView, 0, len(rows))
	for _, r := range rows {
		it := ItemView{
			UnitUUID: r.UnitUuid, Barcode: r.Barcode, UnitStatus: r.UnitStatus, Status: r.Status,
			Product:   Product{UUID: r.ProductUuid, SKU: r.ProductSku, Name: r.ProductName},
			UpdatedAt: r.UpdatedAt.Time,
		}
		if r.Reason.Valid {
			it.Reason = &r.Reason.String
		}
		if r.Error.Valid {
			it.Error = &r.Error.String
		}
		if r.Lot.Valid {
			it.Lot = &r.Lot.String
		}
		if r.Note.Valid {
			it.Note = &r.Note.String
		}
		if cm, err := ledger.NumericToCentimeters(r.Meters); err == nil && r.Meters.Valid {
			m := ledger.FormatMeters(cm)
			it.Meters = &m
		}
		if cm, err := ledger.NumericToCentimeters(r.InitialMeters); err == nil && r.InitialMeters.Valid {
			m := ledger.FormatMeters(cm)
			it.UnitMeters = &m
		}
		out = append(out, it)
	}
	return out, total, nil
}

// JobSort is the sort contract of the job list.
var JobSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"created_at": "created_at", "eligible": "eligible"},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// JobStatuses are the job status filter values.
var JobStatuses = []string{StatusPreview, StatusQueued, StatusRunning, StatusCompleted, StatusFailed, StatusUndoing,
	StatusUndone, StatusDiscarded}

// ListFilter is the job list query.
type ListFilter struct {
	Statuses, Actions []string
	Sort              apiquery.ResolvedSort
	Limit, Offset     int32
}

// ParseListFilter reads the list contract parameters.
func ParseListFilter(v url.Values) (ListFilter, error) {
	q := apiquery.Parse(v)
	out := ListFilter{Limit: q.Limit, Offset: q.Offset}
	var err error
	if out.Sort, err = apiquery.ResolveSort(q.Sort, JobSort); err != nil {
		return out, err
	}
	if out.Statuses, err = apiquery.EnumList(v, "status", JobStatuses...); err != nil {
		return out, err
	}
	if out.Actions, err = apiquery.EnumList(v, "action", Actions...); err != nil {
		return out, err
	}
	return out, nil
}

// List lists the jobs of the organization (newest first by default).
func (s *Service) List(ctx context.Context, c Caller, f ListFilter) ([]JobView, int64, error) {
	total, err := s.q.CountStockBulkJobs(ctx, db.CountStockBulkJobsParams{
		OrganizationID: c.OrgID, Statuses: f.Statuses, Actions: f.Actions,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stockbulk: count jobs: %w", err)
	}
	rows, err := s.q.ListStockBulkJobs(ctx, db.ListStockBulkJobsParams{
		OrganizationID: c.OrgID, Statuses: f.Statuses, Actions: f.Actions,
		SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, RowLimit: f.Limit, RowOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stockbulk: jobs: %w", err)
	}
	out := make([]JobView, 0, len(rows))
	for _, r := range rows {
		v := JobView{
			UUID: r.Uuid, Action: r.Action, Reversible: Reversible(r.Action), Params: r.Params, Selection: r.Selection,
			Status: r.Status, Total: r.Total, Eligible: r.Eligible, Excluded: r.Excluded, Succeeded: r.Succeeded,
			Failed: r.Failed, Reverted: r.Reverted, CreatedAt: r.CreatedAt.Time,
			CreatedBy: r.CreatedByName + " " + r.CreatedBySurname,
		}
		if r.Error.Valid {
			v.Error = &r.Error.String
		}
		if r.FinishedAt.Valid {
			t := r.FinishedAt.Time
			v.FinishedAt = &t
		}
		if r.UndoneAt.Valid {
			t := r.UndoneAt.Time
			v.UndoneAt = &t
		}
		out = append(out, v)
	}
	return out, total, nil
}
