package adapters

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	tasks "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/tasks/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
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
		// TEC-369: move products to another category of the brand.
		{
			ID: ActionSetCategory, LabelKey: "bulk.actions.catalog_products.set_category",
			Permission: rbac.PermCatalogWrite, Reversible: true,
			Params: []bulkengine.BulkActionParam{
				{Key: ParamCategoryUUID, Kind: "uuid", Required: true, LabelKey: "bulk.params.category"},
			},
		},
	}
}

// ActionSetCategory / ParamCategoryUUID: catalog.products set_category.
const (
	ActionSetCategory = "set_category"
	ParamCategoryUUID = "category_uuid"
)

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
	if action == ActionSetCategory {
		return a.setCategory(ctx, entityUUID)
	}
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
	if slices.Contains(p.LockedFields, "active") {
		// TEC-268: the integration sync owns the active flag.
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "locked"}, nil
	}
	if _, err := a.q.SetProductActiveByUUID(ctx, db.SetProductActiveByUUIDParams{Active: want, Uuid: id, BrandID: org.BrandID}); err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
	}
	return res, nil
}

// categoryUUIDOf returns the public id of a product's category.
func (a *CatalogProductsAdapter) categoryUUIDOf(ctx context.Context, p db.Product) (string, error) {
	c, err := a.q.GetProductCategory(ctx, db.GetProductCategoryParams{ID: p.CategoryID, BrandID: p.BrandID})
	if err != nil {
		return "", err
	}
	return c.Uuid.String(), nil
}

// setCategory moves one product to the run's category_uuid (TEC-369). A
// category outside the brand fails the whole run; a product whose category
// the integration sync owns is skipped (TEC-268).
func (a *CatalogProductsAdapter) setCategory(ctx context.Context, entityUUID string) (bulkengine.BulkItemResult, error) {
	org, err := a.center(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	run, _ := bulkengine.RunFrom(ctx)
	catID, err := uuid.Parse(run.Params[ParamCategoryUUID])
	if err != nil {
		return bulkengine.BulkItemResult{}, fmt.Errorf("%s must be a category uuid", ParamCategoryUUID)
	}
	cat, err := a.q.GetProductCategoryByUUID(ctx, db.GetProductCategoryByUUIDParams{Uuid: catID, BrandID: org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return bulkengine.BulkItemResult{}, fmt.Errorf("%s must be a category of the brand", ParamCategoryUUID)
	}
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
	prev, err := a.categoryUUIDOf(ctx, p)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	res := bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "product", OK: true, Op: "update",
		Previous: map[string]any{ParamCategoryUUID: prev},
		Applied:  map[string]any{ParamCategoryUUID: cat.Uuid.String()},
	}
	if p.CategoryID == cat.ID {
		return res, nil
	}
	if slices.Contains(p.LockedFields, ParamCategoryUUID) {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "locked"}, nil
	}
	if _, err := a.q.SetProductCategoryByUUID(ctx, db.SetProductCategoryByUUIDParams{
		CategoryID: cat.ID, Uuid: id, BrandID: org.BrandID,
	}); err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
	}
	return res, nil
}

func (a *CatalogProductsAdapter) CurrentState(ctx context.Context, action string, entityUUID string) (map[string]any, error) {
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
	if action == ActionSetCategory {
		cat, err := a.categoryUUIDOf(ctx, p)
		if err != nil {
			return nil, err
		}
		return map[string]any{ParamCategoryUUID: cat}, nil
	}
	return map[string]any{"active": p.Active}, nil
}

func (a *CatalogProductsAdapter) RevertItem(ctx context.Context, action string, entityUUID string, previous map[string]any) error {
	org, err := a.center(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	if action == ActionSetCategory {
		raw, _ := previous[ParamCategoryUUID].(string)
		catID, err := uuid.Parse(raw)
		if err != nil {
			return fmt.Errorf("missing product category snapshot")
		}
		cat, err := a.q.GetProductCategoryByUUID(ctx, db.GetProductCategoryByUUIDParams{Uuid: catID, BrandID: org.BrandID})
		if err != nil {
			return err
		}
		_, err = a.q.SetProductCategoryByUUID(ctx, db.SetProductCategoryByUUIDParams{CategoryID: cat.ID, Uuid: id, BrandID: org.BrandID})
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

// Task bulk actions and params (TEC-379 adds set_status / set_priority and
// the "select all matching" target).
const (
	ActionTasksAssign      = "assign"
	ActionTasksSetStatus   = "set_status"
	ActionTasksSetPriority = "set_priority"
	ParamTaskStatus        = "status"
	ParamTaskPriority      = "priority"
	maxTaskQueryTargets    = 10000
)

// TasksAdapter changes center tasks (TEC-214 rules: the active organization
// is the brand center and owns the task): assign, set_status, set_priority.
// Targets are selected ids or every task matching the GET /v1/tasks filters
// of target.query (mine is not supported there).
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
			ID: ActionTasksAssign, LabelKey: "bulk.actions.tasks.assign",
			Permission: rbac.PermTasksWrite, Reversible: true,
			Params: []bulkengine.BulkActionParam{
				{Key: ParamAssigneeUUID, Kind: "uuid", Required: true, LabelKey: "bulk.params.assignee"},
			},
		},
		{
			ID: ActionTasksSetStatus, LabelKey: "bulk.actions.tasks.set_status",
			Permission: rbac.PermTasksWrite, Reversible: true, ConfirmKey: "bulk.confirm.tasks.set_status",
			Params: []bulkengine.BulkActionParam{
				{Key: ParamTaskStatus, Kind: "enum", Required: true, LabelKey: "bulk.params.task_status", Options: tasks.Statuses},
			},
		},
		{
			ID: ActionTasksSetPriority, LabelKey: "bulk.actions.tasks.set_priority",
			Permission: rbac.PermTasksWrite, Reversible: true,
			Params: []bulkengine.BulkActionParam{
				{Key: ParamTaskPriority, Kind: "enum", Required: true, LabelKey: "bulk.params.task_priority", Options: tasks.Priorities},
			},
		},
	}
}

// taskParamsError validates the params of an action.
func taskParamsError(action string, params map[string]string) error {
	switch action {
	case ActionTasksAssign:
		if _, err := uuid.Parse(params[ParamAssigneeUUID]); err != nil {
			return fmt.Errorf("%s must be a user uuid", ParamAssigneeUUID)
		}
	case ActionTasksSetStatus:
		if !slices.Contains(tasks.Statuses, params[ParamTaskStatus]) {
			return fmt.Errorf("%s must be one of open, in_progress, done, cancelled", ParamTaskStatus)
		}
	case ActionTasksSetPriority:
		if !slices.Contains(tasks.Priorities, params[ParamTaskPriority]) {
			return fmt.Errorf("%s must be one of low, normal, high, urgent", ParamTaskPriority)
		}
	}
	return nil
}

func (a *TasksAdapter) ResolveTargets(ctx context.Context, action string, target bulkengine.BulkTarget) ([]string, error) {
	if err := taskParamsError(action, target.Params); err != nil {
		return nil, err
	}
	if target.Scope != "query" {
		return idsOnly(target)
	}
	id, err := uuid.Parse(target.Query[QueryOrganizationUUID])
	if err != nil {
		return nil, ErrOrganizationScope
	}
	org, err := a.q.GetOrganizationByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrOrganizationScope
	}
	if err != nil {
		return nil, err
	}
	f, err := tasks.ParseListFilter(queryValues(target.Query), nil)
	if err != nil {
		return nil, err
	}
	c := tasks.Caller{Org: orgctx.Scope{InternalID: org.ID, UUID: org.Uuid, OrgType: org.Type, BrandID: org.BrandID}}
	ids, err := tasks.New(nil, a.q, nil).MatchingUUIDs(ctx, c, f, maxTaskQueryTargets)
	if errors.Is(err, tasks.ErrCenterOnly) {
		return nil, fmt.Errorf("task bulk actions are center only")
	}
	return ids, err
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

func taskClosed(status string) bool {
	return status == tasks.StatusDone || status == tasks.StatusCancelled
}

// taskStatusState is the undo snapshot of set_status: the status and, for
// a closed task, when and by whom it was closed.
func taskStatusState(t db.Task) map[string]any {
	out := map[string]any{"status": t.Status, "closed_at": nil, "closed_by_user_id": nil}
	if t.ClosedAt.Valid {
		out["closed_at"] = t.ClosedAt.Time.UTC().Format(time.RFC3339Nano)
	}
	if t.ClosedByUserID.Valid {
		out["closed_by_user_id"] = t.ClosedByUserID.Int64
	}
	return out
}

func (a *TasksAdapter) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	switch action {
	case ActionTasksAssign, ActionTasksSetStatus, ActionTasksSetPriority:
	default:
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
	}
	org, err := a.center(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	run, _ := bulkengine.RunFrom(ctx)
	if err := taskParamsError(action, run.Params); err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	switch action {
	case ActionTasksSetStatus:
		return a.applyStatus(ctx, org, entityUUID, run.Params[ParamTaskStatus])
	case ActionTasksSetPriority:
		return a.applyPriority(ctx, org, entityUUID, run.Params[ParamTaskPriority])
	}
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
	if taskClosed(t.Status) {
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

// applyStatus moves a task to status like PATCH /v1/tasks/{uuid}: closing
// stamps closed_at (closed_by stays empty, the bulk operation records the
// actor), moving between done and cancelled keeps the closing, reopening
// clears it.
func (a *TasksAdapter) applyStatus(ctx context.Context, org db.Organization, entityUUID, status string) (bulkengine.BulkItemResult, error) {
	t, err := a.task(ctx, org, entityUUID)
	if errors.Is(err, bulkengine.ErrEntityGone) {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not found"}, nil
	}
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	res := bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "task", OK: true, Op: "update",
		Previous: taskStatusState(t), Applied: map[string]any{"status": status},
	}
	if t.Status == status {
		return res, nil
	}
	arg := db.SetTaskStatusParams{ID: t.ID, Status: status}
	if taskClosed(status) && taskClosed(t.Status) {
		arg.ClosedAt, arg.ClosedByUserID = t.ClosedAt, t.ClosedByUserID
	}
	if _, err := a.q.SetTaskStatus(ctx, arg); err != nil {
		return bulkengine.BulkItemResult{}, fmt.Errorf("tasks: bulk status: %w", err)
	}
	return res, nil
}

func (a *TasksAdapter) applyPriority(ctx context.Context, org db.Organization, entityUUID, priority string) (bulkengine.BulkItemResult, error) {
	t, err := a.task(ctx, org, entityUUID)
	if errors.Is(err, bulkengine.ErrEntityGone) {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not found"}, nil
	}
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	res := bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "task", OK: true, Op: "update",
		Previous: map[string]any{"priority": t.Priority}, Applied: map[string]any{"priority": priority},
	}
	if t.Priority == priority {
		return res, nil
	}
	if _, err := a.q.SetTaskPriority(ctx, db.SetTaskPriorityParams{ID: t.ID, Priority: priority}); err != nil {
		return bulkengine.BulkItemResult{}, fmt.Errorf("tasks: bulk priority: %w", err)
	}
	return res, nil
}

func (a *TasksAdapter) CurrentState(ctx context.Context, action string, entityUUID string) (map[string]any, error) {
	org, err := a.center(ctx)
	if err != nil {
		return nil, err
	}
	t, err := a.task(ctx, org, entityUUID)
	if err != nil {
		return nil, err
	}
	switch action {
	case ActionTasksSetStatus:
		return map[string]any{"status": t.Status}, nil
	case ActionTasksSetPriority:
		return map[string]any{"priority": t.Priority}, nil
	}
	uuids, err := a.assigneeUUID(ctx, t)
	if err != nil {
		return nil, err
	}
	return assigneeState(t, uuids), nil
}

func (a *TasksAdapter) RevertItem(ctx context.Context, action string, entityUUID string, previous map[string]any) error {
	org, err := a.center(ctx)
	if err != nil {
		return err
	}
	t, err := a.task(ctx, org, entityUUID)
	if err != nil {
		return err
	}
	switch action {
	case ActionTasksSetStatus:
		return a.revertStatus(ctx, t, previous)
	case ActionTasksSetPriority:
		p, _ := previous["priority"].(string)
		if !slices.Contains(tasks.Priorities, p) {
			return fmt.Errorf("missing task priority snapshot")
		}
		_, err := a.q.SetTaskPriority(ctx, db.SetTaskPriorityParams{ID: t.ID, Priority: p})
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

// revertStatus restores the status and the closing of the snapshot.
func (a *TasksAdapter) revertStatus(ctx context.Context, t db.Task, previous map[string]any) error {
	status, _ := previous["status"].(string)
	if !slices.Contains(tasks.Statuses, status) {
		return fmt.Errorf("missing task status snapshot")
	}
	arg := db.SetTaskStatusParams{ID: t.ID, Status: status}
	if s, ok := previous["closed_at"].(string); ok && s != "" {
		at, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return fmt.Errorf("task status snapshot: %w", err)
		}
		arg.ClosedAt = pgtype.Timestamptz{Time: at, Valid: true}
	}
	switch v := previous["closed_by_user_id"].(type) {
	case float64:
		arg.ClosedByUserID = pgtype.Int8{Int64: int64(v), Valid: true}
	case int64:
		arg.ClosedByUserID = pgtype.Int8{Int64: v, Valid: true}
	}
	_, err := a.q.SetTaskStatus(ctx, arg)
	return err
}
