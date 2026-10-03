package adapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Tenant adapters (TEC-212). The bulk handler stamps the active
// organization as query organization_uuid (ExecuteTenant); the adapters
// resolve it to the organization row and bound every read and write by its
// brand (K1/K20) and, where the module requires it, to the brand center.

// QueryOrganizationUUID is the target query key the tenant bulk handler
// stamps with the active organization.
const QueryOrganizationUUID = "organization_uuid"

// ParamAssigneeUUID is the tasks.assign parameter.
const ParamAssigneeUUID = "assignee_uuid"

// ErrOrganizationScope marks a bulk run without a resolvable organization.
var ErrOrganizationScope = errors.New("organization scope is required")

// runOrganization loads the organization stamped on the bulk run.
func runOrganization(ctx context.Context, q *db.Queries) (db.Organization, error) {
	run, ok := bulkengine.RunFrom(ctx)
	if !ok {
		return db.Organization{}, ErrOrganizationScope
	}
	id, err := uuid.Parse(run.Query[QueryOrganizationUUID])
	if err != nil {
		return db.Organization{}, ErrOrganizationScope
	}
	org, err := q.GetOrganizationByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, ErrOrganizationScope
	}
	return org, err
}

func idsOnly(target bulkengine.BulkTarget) ([]string, error) {
	if target.Scope != "ids" {
		return nil, fmt.Errorf("target scope must be ids")
	}
	return uniqueNonEmpty(target.IDs)
}

// --- catalog.products --------------------------------------------------------

const ResourceCatalogProducts = "catalog.products"

// CatalogProductsAdapter activates / deactivates products of the active
// brand (center only, K4).
type CatalogProductsAdapter struct {
	q *db.Queries
}

func NewCatalogProducts(q *db.Queries) *CatalogProductsAdapter {
	return &CatalogProductsAdapter{q: q}
}

func (a *CatalogProductsAdapter) Resource() string { return ResourceCatalogProducts }

func (a *CatalogProductsAdapter) WithQueries(q *db.Queries) bulkengine.BulkAdapter {
	return &CatalogProductsAdapter{q: q}
}

func (a *CatalogProductsAdapter) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: "activate", LabelKey: "bulk.actions.catalog_products.activate",
			Permission: rbac.PermCatalogWrite, Reversible: true,
		},
		{
			ID: "deactivate", LabelKey: "bulk.actions.catalog_products.deactivate",
			Permission: rbac.PermCatalogWrite, Destructive: true, Reversible: true,
			ConfirmKey: "bulk.confirm.catalog_products.deactivate",
		},
	}
}

func (a *CatalogProductsAdapter) ResolveTargets(_ context.Context, _ string, target bulkengine.BulkTarget) ([]string, error) {
	return idsOnly(target)
}

func (a *CatalogProductsAdapter) center(ctx context.Context) (db.Organization, error) {
	org, err := runOrganization(ctx, a.q)
	if err != nil {
		return org, err
	}
	if org.Type != "center" {
		return org, fmt.Errorf("catalog bulk actions are center only")
	}
	return org, nil
}

func (a *CatalogProductsAdapter) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	var want bool
	switch action {
	case "activate":
		want = true
	case "deactivate":
		want = false
	default:
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
	}
	org, err := a.center(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "invalid uuid"}, nil
	}
	p, err := a.q.GetProductByUUID(ctx, db.GetProductByUUIDParams{Uuid: id, BrandID: org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not found"}, nil
	}
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	res := bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "product", OK: true, Op: "update",
		Previous: map[string]any{"active": p.Active},
		Applied:  map[string]any{"active": want},
	}
	if p.Active == want {
		return res, nil
	}
	if _, err := a.q.SetProductActiveByUUID(ctx, db.SetProductActiveByUUIDParams{Active: want, Uuid: id, BrandID: org.BrandID}); err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
	}
	return res, nil
}

func (a *CatalogProductsAdapter) CurrentState(ctx context.Context, _ string, entityUUID string) (map[string]any, error) {
	org, err := a.center(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return nil, err
	}
	p, err := a.q.GetProductByUUID(ctx, db.GetProductByUUIDParams{Uuid: id, BrandID: org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, bulkengine.ErrEntityGone
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"active": p.Active}, nil
}

func (a *CatalogProductsAdapter) RevertItem(ctx context.Context, _ string, entityUUID string, previous map[string]any) error {
	org, err := a.center(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	prev, ok := previous["active"].(bool)
	if !ok {
		return fmt.Errorf("missing product snapshot")
	}
	_, err = a.q.SetProductActiveByUUID(ctx, db.SetProductActiveByUUIDParams{Active: prev, Uuid: id, BrandID: org.BrandID})
	return err
}

// --- tasks -------------------------------------------------------------------

const ResourceTasks = "tasks"

// TasksAdapter assigns center tasks to a center member (TEC-214 rules:
// the active organization is the brand center and owns the task).
type TasksAdapter struct {
	q *db.Queries
}

func NewTasks(q *db.Queries) *TasksAdapter {
	return &TasksAdapter{q: q}
}

func (a *TasksAdapter) Resource() string { return ResourceTasks }

func (a *TasksAdapter) WithQueries(q *db.Queries) bulkengine.BulkAdapter {
	return &TasksAdapter{q: q}
}

func (a *TasksAdapter) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: "assign", LabelKey: "bulk.actions.tasks.assign",
			Permission: rbac.PermTasksWrite, Reversible: true,
			Params: []bulkengine.BulkActionParam{
				{Key: ParamAssigneeUUID, Kind: "uuid", Required: true, LabelKey: "bulk.params.assignee"},
			},
		},
	}
}

func (a *TasksAdapter) ResolveTargets(_ context.Context, _ string, target bulkengine.BulkTarget) ([]string, error) {
	return idsOnly(target)
}

func (a *TasksAdapter) center(ctx context.Context) (db.Organization, error) {
	org, err := runOrganization(ctx, a.q)
	if err != nil {
		return org, err
	}
	if org.Type != "center" {
		return org, fmt.Errorf("task bulk actions are center only")
	}
	return org, nil
}

// task loads a task owned by the center of the run.
func (a *TasksAdapter) task(ctx context.Context, org db.Organization, entityUUID string) (db.Task, error) {
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return db.Task{}, bulkengine.ErrEntityGone
	}
	t, err := a.q.GetTaskByUUID(ctx, db.GetTaskByUUIDParams{Uuid: id, BrandID: org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && t.OrganizationID != org.ID) {
		return db.Task{}, bulkengine.ErrEntityGone
	}
	return t, err
}

func assigneeState(t db.Task, uuids map[int64]string) map[string]any {
	if !t.AssigneeUserID.Valid {
		return map[string]any{"assignee_uuid": nil}
	}
	return map[string]any{"assignee_uuid": uuids[t.AssigneeUserID.Int64]}
}

func (a *TasksAdapter) assigneeUUID(ctx context.Context, t db.Task) (map[int64]string, error) {
	out := map[int64]string{}
	if !t.AssigneeUserID.Valid {
		return out, nil
	}
	u, err := a.q.GetUserByID(ctx, t.AssigneeUserID.Int64)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		out[u.ID] = u.Uuid.String()
	}
	return out, nil
}

func (a *TasksAdapter) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	if action != "assign" {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
	}
	org, err := a.center(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	run, _ := bulkengine.RunFrom(ctx)
	assigneeID, err := uuid.Parse(run.Params[ParamAssigneeUUID])
	if err != nil {
		return bulkengine.BulkItemResult{}, fmt.Errorf("%s must be a user uuid", ParamAssigneeUUID)
	}
	member, err := a.q.GetCenterMemberByUUID(ctx, db.GetCenterMemberByUUIDParams{Uuid: assigneeID, OrganizationID: org.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return bulkengine.BulkItemResult{}, fmt.Errorf("%s must be a member of the center organization", ParamAssigneeUUID)
	}
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	t, err := a.task(ctx, org, entityUUID)
	if errors.Is(err, bulkengine.ErrEntityGone) {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not found"}, nil
	}
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	if t.Status == "done" || t.Status == "cancelled" {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "task is closed"}, nil
	}
	uuids, err := a.assigneeUUID(ctx, t)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	res := bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "task", OK: true, Op: "update",
		Previous: assigneeState(t, uuids),
		Applied:  map[string]any{"assignee_uuid": member.Uuid.String()},
	}
	if t.AssigneeUserID.Valid && t.AssigneeUserID.Int64 == member.ID {
		return res, nil
	}
	if _, err := a.q.SetTaskAssignee(ctx, db.SetTaskAssigneeParams{
		AssigneeUserID: pgtype.Int8{Int64: member.ID, Valid: true}, ID: t.ID,
	}); err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
	}
	return res, nil
}

func (a *TasksAdapter) CurrentState(ctx context.Context, _ string, entityUUID string) (map[string]any, error) {
	org, err := a.center(ctx)
	if err != nil {
		return nil, err
	}
	t, err := a.task(ctx, org, entityUUID)
	if err != nil {
		return nil, err
	}
	uuids, err := a.assigneeUUID(ctx, t)
	if err != nil {
		return nil, err
	}
	return assigneeState(t, uuids), nil
}

func (a *TasksAdapter) RevertItem(ctx context.Context, _ string, entityUUID string, previous map[string]any) error {
	org, err := a.center(ctx)
	if err != nil {
		return err
	}
	t, err := a.task(ctx, org, entityUUID)
	if err != nil {
		return err
	}
	raw, ok := previous["assignee_uuid"]
	if !ok {
		return fmt.Errorf("missing task snapshot")
	}
	assignee := pgtype.Int8{}
	if s, ok := raw.(string); ok && s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			return err
		}
		u, err := a.q.GetUserByUUID(ctx, id)
		if err != nil {
			return err
		}
		assignee = pgtype.Int8{Int64: u.ID, Valid: true}
	}
	_, err = a.q.SetTaskAssignee(ctx, db.SetTaskAssigneeParams{AssigneeUserID: assignee, ID: t.ID})
	return err
}
