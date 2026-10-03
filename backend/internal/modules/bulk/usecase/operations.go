package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-212: every bulk run (sync or async) writes one bulk_operations row in
// the transaction that applied the items. The row keeps, per record, the
// values before (previous) and after (applied) the action; Undo restores
// previous for the records whose live values still equal applied and skips
// the others (changed since, or gone). Rules: same organization only, the
// action's permission, once, within the undo window.

var (
	// ErrUndoUnavailable: already undone, nothing to undo, or being undone.
	ErrUndoUnavailable = errors.New("undo unavailable")
	// ErrUndoExpired: the undo window has passed.
	ErrUndoExpired = errors.New("undo window expired")
)

// Undo statuses (bulk_operations.undo_status).
const (
	UndoNone      = "none"
	UndoAvailable = "available"
	UndoUndone    = "undone"
	UndoPartial   = "partial"
)

// Skip reasons reported by undo.
const (
	SkipConflict = "conflict"
	SkipGone     = "gone"
)

// Change is one record of a bulk operation.
type Change struct {
	EntityType string         `json:"entity_type"`
	EntityUUID string         `json:"entity_uuid"`
	Op         string         `json:"op"`
	Previous   map[string]any `json:"previous,omitempty"`
	Applied    map[string]any `json:"applied,omitempty"`
}

// UndoSkipped is a record undo left untouched.
type UndoSkipped struct {
	EntityUUID string `json:"entity_uuid"`
	Reason     string `json:"reason"`
}

// UndoFailed is a record whose revert returned an error.
type UndoFailed struct {
	EntityUUID string `json:"entity_uuid"`
	Error      string `json:"error"`
}

// UndoResult summarizes an undo.
type UndoResult struct {
	Restored int           `json:"restored"`
	Skipped  []UndoSkipped `json:"skipped"`
	Failed   []UndoFailed  `json:"failed"`
}

// OperationView is the API shape of a bulk operation.
type OperationView struct {
	UUID       uuid.UUID              `json:"uuid"`
	Resource   string                 `json:"resource"`
	Action     string                 `json:"action"`
	JobUUID    *uuid.UUID             `json:"job_uuid,omitempty"`
	Summary    bulkengine.BulkSummary `json:"summary"`
	UndoStatus string                 `json:"undo_status"`
	UndoUntil  *time.Time             `json:"undo_until,omitempty"`
	UndoneAt   *time.Time             `json:"undone_at,omitempty"`
	UndoResult *UndoResult            `json:"undo_result,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

// UndoInput identifies the caller of Undo.
type UndoInput struct {
	UUID    uuid.UUID
	ActorID int64
	// OrganizationID is the active organization for tenant routes; nil on
	// platform routes (only platform operations are visible then).
	OrganizationID *int64
	HasPermission  func(slug string) bool
}

// WithPool enables transactional runs (items + log row in one transaction).
func (s *Service) WithPool(pool *pgxpool.Pool) *Service {
	s.pool = pool
	return s
}

// WithUndoWindow sets how many hours an operation stays undoable
// (sysconfig bulk_undo_window_hours); nil keeps the catalog default.
func (s *Service) WithUndoWindow(fn func(ctx context.Context) int) *Service {
	s.undoWindow = fn
	return s
}

func (s *Service) undoWindowHours(ctx context.Context) int {
	if s.undoWindow != nil {
		if h := s.undoWindow(ctx); h > 0 {
			return h
		}
	}
	return sysconfig.DefaultBulkUndoWindowHours
}

// run applies targets and logs the operation in one transaction.
func (s *Service) run(
	ctx context.Context,
	actorID, jobID int64,
	adapter bulkengine.BulkAdapter,
	def bulkengine.BulkActionDef,
	resource, action string,
	target bulkengine.BulkTarget,
	targets []string,
) (db.BulkOperation, bulkengine.BulkSummary, error) {
	ctx = bulkengine.WithRun(ctx, bulkengine.Run{Params: target.Params, Query: target.Query})
	q := s.q
	var tx pgx.Tx
	if s.pool != nil {
		var err error
		tx, err = s.pool.Begin(ctx)
		if err != nil {
			return db.BulkOperation{}, bulkengine.BulkSummary{}, err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		q = s.q.WithTx(tx)
	}
	bound := bulkengine.Bind(adapter, q)
	summary, changes, err := s.applyTargets(ctx, bound, action, targets)
	if err != nil {
		return db.BulkOperation{}, summary, err
	}
	org, brand, err := s.scopeOf(ctx, q, target)
	if err != nil {
		return db.BulkOperation{}, summary, err
	}
	status := UndoNone
	var until pgtype.Timestamptz
	if def.Reversible && len(changes) > 0 {
		status = UndoAvailable
		until = pgtype.Timestamptz{
			Time: time.Now().UTC().Add(time.Duration(s.undoWindowHours(ctx)) * time.Hour), Valid: true,
		}
	}
	targetJSON, _ := json.Marshal(target)
	changesJSON, _ := json.Marshal(changes)
	var job pgtype.Int8
	if jobID > 0 {
		job = pgtype.Int8{Int64: jobID, Valid: true}
	}
	op, err := q.InsertBulkOperation(ctx, db.InsertBulkOperationParams{
		OrganizationID: org, BrandID: brand, JobID: job,
		Resource: resource, Action: action, Permission: def.Permission, ActorUserID: actorID,
		TargetJson: targetJSON, Changes: changesJSON,
		Total: int32(summary.Total), Succeeded: int32(summary.Succeeded), Failed: int32(summary.Failed), //nolint:gosec // bounded by SyncMax / target size
		UndoStatus: status, UndoUntil: until,
	})
	if err != nil {
		return db.BulkOperation{}, summary, err
	}
	if tx != nil {
		if err := tx.Commit(ctx); err != nil {
			return db.BulkOperation{}, summary, err
		}
	}
	return op, summary, nil
}

// scopeOf resolves the organization stamped by the tenant handler; a
// platform run (no organization_uuid) logs with both scope columns NULL.
func (s *Service) scopeOf(ctx context.Context, q *db.Queries, target bulkengine.BulkTarget) (pgtype.Int8, pgtype.Int8, error) {
	raw := target.Query[queryOrganizationUUID]
	if raw == "" {
		return pgtype.Int8{}, pgtype.Int8{}, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return pgtype.Int8{}, pgtype.Int8{}, fmt.Errorf("%w: organization_uuid is invalid", ErrInvalidRequest)
	}
	org, err := q.GetOrganizationByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.Int8{}, pgtype.Int8{}, fmt.Errorf("%w: organization not found", ErrInvalidRequest)
	}
	if err != nil {
		return pgtype.Int8{}, pgtype.Int8{}, err
	}
	return pgtype.Int8{Int64: org.ID, Valid: true}, pgtype.Int8{Int64: org.BrandID, Valid: true}, nil
}

const queryOrganizationUUID = "organization_uuid"

func (s *Service) applyTargets(
	ctx context.Context,
	adapter bulkengine.BulkAdapter,
	action string,
	targets []string,
) (bulkengine.BulkSummary, []Change, error) {
	summary := bulkengine.BulkSummary{Total: len(targets)}
	changes := make([]Change, 0, len(targets))
	for _, entityUUID := range targets {
		res, err := adapter.ApplyItem(ctx, action, entityUUID)
		if err != nil {
			return summary, nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
		}
		if !res.OK {
			summary.Failed++
			continue
		}
		summary.Succeeded++
		changes = append(changes, Change{
			EntityType: res.EntityType, EntityUUID: res.EntityUUID, Op: res.Op,
			Previous: res.Previous, Applied: res.Applied,
		})
	}
	return summary, changes, nil
}

// Undo reverts an operation (POST .../bulk-operations/{uuid}/undo).
func (s *Service) Undo(ctx context.Context, in UndoInput) (OperationView, error) {
	var org pgtype.Int8
	if in.OrganizationID != nil {
		org = pgtype.Int8{Int64: *in.OrganizationID, Valid: true}
	}
	row, err := s.q.GetBulkOperationByUUID(ctx, db.GetBulkOperationByUUIDParams{Uuid: in.UUID, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return OperationView{}, ErrNotFound
	}
	if err != nil {
		return OperationView{}, err
	}
	if in.HasPermission == nil || !in.HasPermission(row.Permission) {
		return OperationView{}, ErrForbidden
	}
	return s.undoOperation(ctx, row, in.ActorID)
}

func (s *Service) undoOperation(ctx context.Context, row db.BulkOperation, actorID int64) (OperationView, error) {
	if row.UndoStatus != UndoAvailable {
		return OperationView{}, ErrUndoUnavailable
	}
	if row.UndoUntil.Valid && time.Now().After(row.UndoUntil.Time) {
		return OperationView{}, ErrUndoExpired
	}
	_, adapter, err := s.registry.ActionDef(row.Resource, row.Action)
	if err != nil {
		return OperationView{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	var target bulkengine.BulkTarget
	_ = json.Unmarshal(row.TargetJson, &target)
	ctx = bulkengine.WithRun(ctx, bulkengine.Run{Params: target.Params, Query: target.Query})
	var changes []Change
	if err := json.Unmarshal(row.Changes, &changes); err != nil {
		return OperationView{}, err
	}

	q := s.q
	var tx pgx.Tx
	if s.pool != nil {
		tx, err = s.pool.Begin(ctx)
		if err != nil {
			return OperationView{}, err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		q = s.q.WithTx(tx)
	}
	locked, err := q.LockBulkOperationForUndo(ctx, row.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return OperationView{}, ErrUndoUnavailable
	}
	if err != nil {
		return OperationView{}, err
	}
	bound := bulkengine.Bind(adapter, q)
	reader, _ := bound.(bulkengine.StateReader)
	result := UndoResult{Skipped: []UndoSkipped{}, Failed: []UndoFailed{}}
	for i := len(changes) - 1; i >= 0; i-- {
		ch := changes[i]
		if reader != nil && ch.Applied != nil {
			live, err := reader.CurrentState(ctx, row.Action, ch.EntityUUID)
			switch {
			case errors.Is(err, bulkengine.ErrEntityGone):
				result.Skipped = append(result.Skipped, UndoSkipped{EntityUUID: ch.EntityUUID, Reason: SkipGone})
				continue
			case err != nil:
				result.Failed = append(result.Failed, UndoFailed{EntityUUID: ch.EntityUUID, Error: err.Error()})
				continue
			case !bulkengine.SameState(ch.Applied, live):
				result.Skipped = append(result.Skipped, UndoSkipped{EntityUUID: ch.EntityUUID, Reason: SkipConflict})
				continue
			}
		}
		if err := bound.RevertItem(ctx, row.Action, ch.EntityUUID, ch.Previous); err != nil {
			result.Failed = append(result.Failed, UndoFailed{EntityUUID: ch.EntityUUID, Error: err.Error()})
			continue
		}
		result.Restored++
	}
	status := UndoUndone
	if len(result.Skipped) > 0 || len(result.Failed) > 0 {
		status = UndoPartial
	}
	resultJSON, _ := json.Marshal(result)
	updated, err := q.MarkBulkOperationUndone(ctx, db.MarkBulkOperationUndoneParams{
		UndoStatus: status, UndoneByUserID: pgtype.Int8{Int64: actorID, Valid: true},
		UndoResult: resultJSON, ID: locked.ID,
	})
	if err != nil {
		return OperationView{}, err
	}
	if tx != nil {
		if err := tx.Commit(ctx); err != nil {
			return OperationView{}, err
		}
	}
	if s.activity != nil {
		uid := actorID
		s.activity.Record(ctx, &uid, "bulk.undone", row.Resource, &row.Uuid, map[string]any{
			"action": row.Action, "restored": result.Restored,
			"skipped": len(result.Skipped), "failed": len(result.Failed),
		}, nil)
	}
	return s.operationView(ctx, updated), nil
}

// ListOperations pages the operations of an organization.
func (s *Service) ListOperations(ctx context.Context, orgID int64, limit, offset int32) ([]OperationView, int64, error) {
	rows, err := s.q.ListBulkOperationsForOrganization(ctx, db.ListBulkOperationsForOrganizationParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountBulkOperationsForOrganization(ctx, pgtype.Int8{Int64: orgID, Valid: true})
	if err != nil {
		return nil, 0, err
	}
	out := make([]OperationView, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.operationView(ctx, r))
	}
	return out, total, nil
}

func (s *Service) operationView(ctx context.Context, row db.BulkOperation) OperationView {
	v := OperationView{
		UUID: row.Uuid, Resource: row.Resource, Action: row.Action,
		Summary:    bulkengine.BulkSummary{Total: int(row.Total), Succeeded: int(row.Succeeded), Failed: int(row.Failed)},
		UndoStatus: row.UndoStatus, CreatedAt: row.CreatedAt.Time,
	}
	if row.UndoUntil.Valid {
		t := row.UndoUntil.Time
		v.UndoUntil = &t
	}
	if row.UndoneAt.Valid {
		t := row.UndoneAt.Time
		v.UndoneAt = &t
	}
	if len(row.UndoResult) > 0 {
		var r UndoResult
		if json.Unmarshal(row.UndoResult, &r) == nil {
			v.UndoResult = &r
		}
	}
	if row.JobID.Valid {
		if job, err := s.q.GetBulkJobByID(ctx, row.JobID.Int64); err == nil {
			id := job.Uuid
			v.JobUUID = &id
		}
	}
	return v
}
