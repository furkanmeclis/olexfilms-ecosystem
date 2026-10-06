package usecase

// TEC-371 (DT-BE-4): lead bulk actions (bulk engine resource "leads",
// POST /v1/leads/bulk, leads.write). The bulk handler stamps the active
// organization and the caller's leads.write scope on the target
// (ExecuteTenantScoped); every item is re-checked against that scope
// (JobCaller), so a run never reaches a lead the list would not show.
//
//   - assign: params.assignee_user_id (user id, a member of the lead's or
//     of the active organization); a timeline "assigned" event like
//     POST /v1/leads/{uuid}/assign. Undo restores the previous assignee.
//   - set_status: params.status (+ lost_reason for lost); the pipeline
//     rules of POST /v1/leads/{uuid}/status apply per item (a disallowed
//     transition fails that item). Undo restores status, lost reason and
//     won reference.
//
// There is no lead delete (no single delete endpoint either), so there is
// no bulk delete.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ResourceBulk is the bulk engine resource of leads.
const ResourceBulk = "leads"

// Bulk action ids and params.
const (
	BulkAssign           = "assign"
	BulkSetStatus        = "set_status"
	ParamAssigneeUserID  = "assignee_user_id"
	ParamStatus          = "status"
	ParamLostReason      = "lost_reason"
	maxBulkQueryTargets  = 10000
	queryOrganizationKey = "organization_uuid"
)

// BulkAdapter implements bulkengine.BulkAdapter for leads.
type BulkAdapter struct{ q *db.Queries }

// NewBulkAdapter creates the lead bulk adapter.
func NewBulkAdapter(q *db.Queries) *BulkAdapter { return &BulkAdapter{q: q} }

// Resource implements bulkengine.BulkAdapter.
func (a *BulkAdapter) Resource() string { return ResourceBulk }

// WithQueries binds the adapter to the run transaction.
func (a *BulkAdapter) WithQueries(q *db.Queries) bulkengine.BulkAdapter { return &BulkAdapter{q: q} }

// BulkActions implements bulkengine.BulkAdapter.
func (a *BulkAdapter) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: BulkAssign, LabelKey: "bulk.actions.leads.assign", Permission: rbac.PermLeadsWrite, Reversible: true,
			Params: []bulkengine.BulkActionParam{
				{Key: ParamAssigneeUserID, Kind: "number", Required: true, LabelKey: "bulk.params.assignee"},
			},
		},
		{
			ID: BulkSetStatus, LabelKey: "bulk.actions.leads.set_status", Permission: rbac.PermLeadsWrite, Reversible: true,
			ConfirmKey: "bulk.confirm.leads.set_status",
			Params: []bulkengine.BulkActionParam{
				{Key: ParamStatus, Kind: "enum", Required: true, LabelKey: "bulk.params.lead_status", Options: LeadStatuses},
				{Key: ParamLostReason, Kind: "text", LabelKey: "bulk.params.lost_reason"},
			},
		},
	}
}

func bulkParamsError(action string, params map[string]string) error {
	switch action {
	case BulkAssign:
		if _, err := assigneeParam(params); err != nil {
			return err
		}
	case BulkSetStatus:
		to := strings.TrimSpace(params[ParamStatus])
		if !validStatus(to) {
			return fmt.Errorf("%s must be one of %s", ParamStatus, strings.Join(LeadStatuses, ", "))
		}
		if to == StatusLost && strings.TrimSpace(params[ParamLostReason]) == "" {
			return fmt.Errorf("%s is required when status is lost", ParamLostReason)
		}
	}
	return nil
}

func assigneeParam(params map[string]string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(params[ParamAssigneeUserID]), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%s must be a user id", ParamAssigneeUserID)
	}
	return id, nil
}

// caller rebuilds the caller of a run from the stamped target query.
func (a *BulkAdapter) caller(ctx context.Context, query map[string]string) (Caller, error) {
	orgUUID, err := uuid.Parse(query[queryOrganizationKey])
	if err != nil {
		return Caller{}, errors.New("organization scope is required")
	}
	org, err := a.q.GetOrganizationByUUID(ctx, orgUUID)
	if err != nil {
		return Caller{}, fmt.Errorf("organization scope: %w", err)
	}
	raw, ok := query[bulkengine.QueryScope]
	if !ok {
		return Caller{}, errors.New("permission scope is required")
	}
	actor, _ := strconv.ParseInt(query[bulkengine.QueryActorUserID], 10, 64)
	return JobCaller(ctx, a.q, org.ID, raw, actor)
}

// ResolveTargets implements bulkengine.BulkAdapter: selected ids, or every
// lead matching the GET /v1/leads filters of target.query in scope.
func (a *BulkAdapter) ResolveTargets(ctx context.Context, action string, target bulkengine.BulkTarget) ([]string, error) {
	if err := bulkParamsError(action, target.Params); err != nil {
		return nil, err
	}
	c, err := a.caller(ctx, target.Query)
	if err != nil {
		return nil, err
	}
	switch target.Scope {
	case "ids":
		out := make([]string, 0, len(target.IDs))
		for _, id := range target.IDs {
			if id = strings.TrimSpace(id); id != "" && !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
		if len(out) == 0 {
			return nil, errors.New("no target ids")
		}
		return out, nil
	case "query":
	default:
		return nil, errors.New("invalid target scope")
	}
	f, err := ParseListFilter(ListValues(target.Query))
	if err != nil {
		return nil, err
	}
	f.Limit = maxBulkQueryTargets
	svc := New(nil, a.q, nil)
	p, err := svc.params(c, f)
	if err != nil {
		return nil, err
	}
	rows, err := a.q.ListLeadsInScope(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Uuid.String())
	}
	return out, nil
}

// lead loads a lead of the run inside the stamped scope.
func (a *BulkAdapter) lead(ctx context.Context, c Caller, entityUUID string) (db.Lead, error) {
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return db.Lead{}, bulkengine.ErrEntityGone
	}
	row, err := a.q.GetLeadByUUID(ctx, db.GetLeadByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !c.Filter.AllowsOrg(row.OrganizationID, row.BrandID)) {
		return db.Lead{}, bulkengine.ErrEntityGone
	}
	return row, err
}

func (a *BulkAdapter) runCaller(ctx context.Context) (Caller, bulkengine.Run, error) {
	run, ok := bulkengine.RunFrom(ctx)
	if !ok {
		return Caller{}, run, errors.New("bulk run context is required")
	}
	c, err := a.caller(ctx, run.Query)
	return c, run, err
}

func leadState(action string, l db.Lead) map[string]any {
	if action == BulkAssign {
		var v any
		if l.AssigneeUserID.Valid {
			v = l.AssigneeUserID.Int64
		}
		return map[string]any{"assignee_user_id": v}
	}
	return map[string]any{"status": l.Status}
}

// ApplyItem implements bulkengine.BulkAdapter.
func (a *BulkAdapter) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	c, run, err := a.runCaller(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	if err := bulkParamsError(action, run.Params); err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	fail := func(msg string) (bulkengine.BulkItemResult, error) {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: msg}, nil
	}
	cur, err := a.lead(ctx, c, entityUUID)
	if errors.Is(err, bulkengine.ErrEntityGone) {
		return fail("not found")
	}
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	res := bulkengine.BulkItemResult{EntityUUID: entityUUID, EntityType: "lead", OK: true, Op: "update"}
	switch action {
	case BulkAssign:
		assignee, _ := assigneeParam(run.Params)
		res.Previous = leadState(action, cur)
		res.Applied = map[string]any{"assignee_user_id": assignee}
		if cur.AssigneeUserID.Valid && cur.AssigneeUserID.Int64 == assignee {
			return res, nil
		}
		member, err := a.isMember(ctx, assignee, cur.OrganizationID, c.Org.InternalID)
		if err != nil {
			return bulkengine.BulkItemResult{}, err
		}
		if !member {
			return fail("assignee is not a member of the organization")
		}
		row, err := a.q.AssignLead(ctx, db.AssignLeadParams{
			ID: cur.ID, OrganizationID: cur.OrganizationID, AssigneeUserID: pgtype.Int8{Int64: assignee, Valid: true},
		})
		if err != nil {
			return bulkengine.BulkItemResult{}, fmt.Errorf("leads: bulk assign: %w", err)
		}
		if err := addEvent(ctx, a.q, row, EventAssigned, map[string]any{"assignee_user_id": assignee, "bulk": true}, c.actor()); err != nil {
			return bulkengine.BulkItemResult{}, err
		}
		return res, nil
	case BulkSetStatus:
		to := strings.TrimSpace(run.Params[ParamStatus])
		res.Previous = map[string]any{
			"status": cur.Status, "lost_reason": textPtr(cur.LostReason),
			"won_ref_type": textPtr(cur.WonRefType), "won_ref_id": intPtr(cur.WonRefID),
		}
		res.Applied = map[string]any{"status": to}
		if cur.Status == to {
			return res, nil
		}
		if !allowedTransition(cur.Status, to) {
			return fail("status transition is not allowed")
		}
		var lost pgtype.Text
		if to == StatusLost {
			lost = pgtype.Text{String: strings.TrimSpace(run.Params[ParamLostReason]), Valid: true}
		}
		if _, err := a.q.SetLeadStatus(ctx, db.SetLeadStatusParams{
			ID: cur.ID, OrganizationID: cur.OrganizationID, Status: to, LostReason: lost,
		}); err != nil {
			return bulkengine.BulkItemResult{}, fmt.Errorf("leads: bulk status: %w", err)
		}
		return res, nil
	default:
		return fail("unknown action")
	}
}

// isMember reports whether user is a member of one of the organizations.
func (a *BulkAdapter) isMember(ctx context.Context, user int64, orgs ...int64) (bool, error) {
	for _, org := range orgs {
		_, err := a.q.GetOrganizationMember(ctx, db.GetOrganizationMemberParams{OrganizationID: org, UserID: user})
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return false, err
		}
	}
	return false, nil
}

// CurrentState implements bulkengine.StateReader (undo conflict check).
func (a *BulkAdapter) CurrentState(ctx context.Context, action, entityUUID string) (map[string]any, error) {
	c, _, err := a.runCaller(ctx)
	if err != nil {
		return nil, err
	}
	cur, err := a.lead(ctx, c, entityUUID)
	if err != nil {
		return nil, err
	}
	return leadState(action, cur), nil
}

// RevertItem implements bulkengine.BulkAdapter.
func (a *BulkAdapter) RevertItem(ctx context.Context, action, entityUUID string, previous map[string]any) error {
	c, _, err := a.runCaller(ctx)
	if err != nil {
		return err
	}
	cur, err := a.lead(ctx, c, entityUUID)
	if err != nil {
		return err
	}
	switch action {
	case BulkAssign:
		assignee := pgtype.Int8{}
		if id, ok := numberOf(previous["assignee_user_id"]); ok {
			assignee = pgtype.Int8{Int64: id, Valid: true}
		}
		row, err := a.q.AssignLead(ctx, db.AssignLeadParams{ID: cur.ID, OrganizationID: cur.OrganizationID, AssigneeUserID: assignee})
		if err != nil {
			return err
		}
		var v *int64
		if assignee.Valid {
			v = &assignee.Int64
		}
		return addEvent(ctx, a.q, row, EventAssigned, map[string]any{"assignee_user_id": v, "bulk_undo": true}, c.actor())
	case BulkSetStatus:
		status, _ := previous["status"].(string)
		if !validStatus(status) {
			return errors.New("missing lead status snapshot")
		}
		p := db.SetLeadStatusParams{ID: cur.ID, OrganizationID: cur.OrganizationID, Status: status}
		if s, ok := previous["lost_reason"].(string); ok {
			p.LostReason = pgtype.Text{String: s, Valid: true}
		}
		if s, ok := previous["won_ref_type"].(string); ok {
			p.WonRefType = pgtype.Text{String: s, Valid: true}
		}
		if id, ok := numberOf(previous["won_ref_id"]); ok {
			p.WonRefID = pgtype.Int8{Int64: id, Valid: true}
		}
		_, err := a.q.SetLeadStatus(ctx, p)
		return err
	default:
		return errors.New("unsupported action")
	}
}

// numberOf reads an id from a decoded JSON snapshot (float64) or a live
// value (int64 / *int64).
func numberOf(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case *int64:
		if t != nil {
			return *t, true
		}
	}
	return 0, false
}

var (
	_ bulkengine.BulkAdapter = (*BulkAdapter)(nil)
	_ bulkengine.TxBinder    = (*BulkAdapter)(nil)
	_ bulkengine.StateReader = (*BulkAdapter)(nil)
)
