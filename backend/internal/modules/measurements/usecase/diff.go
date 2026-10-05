package usecase

// TEC-297 (F3-02e): part based before/after micron difference table and
// the "measurement check required" flag of a service.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	toleranceSettingKey = "measurements.tolerance_um"
	defaultToleranceUM  = 30.0
	maxCheckNote        = 1000
)

// MeasurementDiff is GET /v1/services/{uuid}/measurements/diff.
type MeasurementDiff struct {
	ServiceUUID   uuid.UUID             `json:"service_uuid"`
	CheckRequired bool                  `json:"check_required"`
	CheckedAt     *time.Time            `json:"checked_at"`
	ToleranceUM   string                `json:"tolerance_um"`
	Parts         []MeasurementDiffPart `json:"parts"`
}

// MeasurementDiffPart is one place_id/part_type row of the diff table.
type MeasurementDiffPart struct {
	PlaceID        string           `json:"place_id"`
	PartType       string           `json:"part_type"`
	ServicePartKey string           `json:"service_part_key"`
	Before         MeasurementStats `json:"before"`
	After          MeasurementStats `json:"after"`
	DiffUM         *string          `json:"diff_um"`
	ExpectedUM     *string          `json:"expected_um"`
	Deviation      bool             `json:"deviation"`
	ExpectedStatus string           `json:"expected_status"`
}

// MeasurementStats summarizes all measurement points of one phase on a part.
type MeasurementStats struct {
	AverageUM *string `json:"average_um"`
	MinUM     *string `json:"min_um"`
	MaxUM     *string `json:"max_um"`
	Count     int32   `json:"count"`
}

// MarkCheckedInput is POST /v1/services/{uuid}/measurements/checked.
type MarkCheckedInput struct {
	Note string
}

// ServiceMeasurementDiff returns the computed diff rows without changing
// state; recalculation is done by link/item events.
func (l *Linker) ServiceMeasurementDiff(ctx context.Context, c LinkCaller, id uuid.UUID) (MeasurementDiff, error) {
	svc, err := l.visibleService(ctx, l.q, c, id)
	if err != nil {
		return MeasurementDiff{}, err
	}
	out, _, err := l.diffView(ctx, l.q, svc)
	return out, err
}

// MarkMeasurementsChecked stamps that a user reviewed the flagged table. The
// note is intentionally not stored yet; it is mandatory so the UI cannot
// clear the marker accidentally.
func (l *Linker) MarkMeasurementsChecked(ctx context.Context, c LinkCaller, id uuid.UUID, in MarkCheckedInput) error {
	note := strings.TrimSpace(in.Note)
	if note == "" {
		return invalid("note", "is required")
	}
	if len([]rune(note)) > maxCheckNote {
		return invalid("note", "must be at most 1000 characters")
	}
	return l.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		svc, err := l.lockService(ctx, q, c, id)
		if err != nil {
			return err
		}
		n, err := q.MarkServiceMeasurementChecked(ctx, db.MarkServiceMeasurementCheckedParams{
			ID: svc.ID, OrganizationID: svc.OrganizationID,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrServiceNotFound
		}
		return nil
	})
}

// RecalculateServiceDiff recomputes the service marker after service item
// edits or service events. It is idempotent and serialized by the service row
// lock.
func (l *Linker) RecalculateServiceDiff(ctx context.Context, serviceID int64) error {
	return l.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		row, err := q.GetServiceForMeasurementMatch(ctx, serviceID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return l.recalculateDiffLocked(ctx, q, tx, fromLockRow(row))
	})
}

func (l *Linker) recalculateDiffLocked(ctx context.Context, q *db.Queries, tx pgx.Tx, svc matchSvc) error {
	view, required, err := l.diffView(ctx, q, svc)
	if err != nil {
		return err
	}
	cur, err := q.GetService(ctx, db.GetServiceParams{ID: svc.ID, BrandID: svc.BrandID})
	if err != nil {
		return err
	}
	checkedAt := cur.MeasurementCheckedAt
	if required {
		checkedAt = pgtype.Timestamptz{}
	}
	if err := q.SetServiceMeasurementCheck(ctx, db.SetServiceMeasurementCheckParams{
		ID: svc.ID, OrganizationID: svc.OrganizationID, MeasurementCheckRequired: required,
		MeasurementCheckedAt: checkedAt,
	}); err != nil {
		return err
	}
	if !required || cur.MeasurementCheckRequired {
		return nil
	}
	return l.enqueueDiffRequired(ctx, q, tx, svc, cur.ServiceNo, view)
}

func (l *Linker) enqueueDiffRequired(ctx context.Context, q *db.Queries, tx pgx.Tx, svc matchSvc, serviceNo string, view MeasurementDiff) error {
	if l.out == nil {
		return nil
	}
	ids, err := q.ListOrganizationOwnerUserIDs(ctx, svc.OrganizationID)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	id, uid := svc.ID, svc.UUID
	ev := events.New(events.MeasurementDiffCheckRequired).WithTenant(svc.OrganizationID).
		WithEntity("service", &id, &uid).WithPayload(map[string]any{
		"service_uuid":    svc.UUID.String(),
		"organization_id": svc.OrganizationID,
		"brand_id":        svc.BrandID,
		"service_no":      serviceNo,
		"notify_user_ids": ids,
		"deviation_count": deviationCount(view.Parts),
	})
	if err := l.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("measurements: diff outbox: %w", err)
	}
	return nil
}

func (l *Linker) diffView(ctx context.Context, q *db.Queries, svc matchSvc) (MeasurementDiff, bool, error) {
	tol := l.toleranceUM(ctx, q)
	rows, err := q.ListServiceMeasurementDiffParts(ctx, db.ListServiceMeasurementDiffPartsParams{
		ServiceID: svc.ID, OrganizationID: svc.OrganizationID,
	})
	if err != nil {
		return MeasurementDiff{}, false, err
	}
	cur, err := q.GetService(ctx, db.GetServiceParams{ID: svc.ID, BrandID: svc.BrandID})
	if err != nil {
		return MeasurementDiff{}, false, err
	}
	out := MeasurementDiff{
		ServiceUUID:   svc.UUID,
		CheckRequired: cur.MeasurementCheckRequired,
		CheckedAt:     timeOut(cur.MeasurementCheckedAt),
		ToleranceUM:   formatFloat(tol),
		Parts:         make([]MeasurementDiffPart, 0, len(rows)),
	}
	required := false
	for _, row := range rows {
		diff := numericStringPtr(row.DiffUm)
		expected := numericStringPtr(row.ExpectedUm)
		part := MeasurementDiffPart{
			PlaceID: row.PlaceID, PartType: row.PartType, ServicePartKey: row.ServicePartKey,
			Before: MeasurementStats{
				AverageUM: numericStringPtr(row.BeforeAvgUm), MinUM: numericStringPtr(row.BeforeMinUm),
				MaxUM: numericStringPtr(row.BeforeMaxUm), Count: row.BeforeCount,
			},
			After: MeasurementStats{
				AverageUM: numericStringPtr(row.AfterAvgUm), MinUM: numericStringPtr(row.AfterMinUm),
				MaxUM: numericStringPtr(row.AfterMaxUm), Count: row.AfterCount,
			},
			DiffUM: diff, ExpectedUM: expected, ExpectedStatus: "available",
		}
		if expected == nil {
			part.ExpectedStatus = "missing"
		} else if diff == nil {
			part.ExpectedStatus = "incomplete"
		} else if d, okD := numericFloat(row.DiffUm); okD {
			if e, okE := numericFloat(row.ExpectedUm); okE && math.Abs(d-e) > tol {
				part.Deviation = true
				required = true
			}
		}
		out.Parts = append(out.Parts, part)
	}
	return out, required, nil
}

func (l *Linker) toleranceUM(ctx context.Context, q *db.Queries) float64 {
	row, err := q.GetSystemSetting(ctx, toleranceSettingKey)
	if err != nil {
		return defaultToleranceUM
	}
	var n float64
	if err := json.Unmarshal(row.Value, &n); err == nil && n > 0 {
		return n
	}
	var obj struct {
		Value float64 `json:"value"`
	}
	if err := json.Unmarshal(row.Value, &obj); err == nil && obj.Value > 0 {
		return obj.Value
	}
	return defaultToleranceUM
}

func deviationCount(parts []MeasurementDiffPart) int {
	n := 0
	for _, p := range parts {
		if p.Deviation {
			n++
		}
	}
	return n
}

func numericRatValue(n pgtype.Numeric) (*big.Rat, bool) {
	if !n.Valid || n.Int == nil {
		return nil, false
	}
	r := new(big.Rat).SetInt(n.Int)
	if n.Exp != 0 {
		e := n.Exp
		if e < 0 {
			e = -e
		}
		p := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(e)), nil))
		if n.Exp > 0 {
			r.Mul(r, p)
		} else {
			r.Quo(r, p)
		}
	}
	return r, true
}

func numericStringPtr(n pgtype.Numeric) *string {
	r, ok := numericRatValue(n)
	if !ok {
		return nil
	}
	s := r.FloatString(2)
	return &s
}

func numericFloat(n pgtype.Numeric) (float64, bool) {
	r, ok := numericRatValue(n)
	if !ok {
		return 0, false
	}
	f, _ := r.Float64()
	return f, true
}

func formatFloat(v float64) string {
	return new(big.Rat).SetFloat64(v).FloatString(2)
}
