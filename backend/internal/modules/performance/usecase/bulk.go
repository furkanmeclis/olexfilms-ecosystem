package usecase

// TEC-497 (F5-05h): bulk approval of dealer bonus accruals (bulk engine
// resource "performance.bonuses", POST /v1/performance/bonuses/bulk,
// performance.bonus.manage). The bulk handler stamps the active
// organization and the requesting user on the target query; every item
// runs ApproveBonus without an amount override (an adjusted amount needs a
// note, so it stays a single-row action). Approval books a staff payment,
// so the action is not reversible.

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// ResourceBonusBulk is the bulk engine resource of bonus accruals.
const ResourceBonusBulk = "performance.bonuses"

// BulkApproveBonus approves calculated accruals.
const BulkApproveBonus = "approve"

const (
	maxBonusBulkTargets     = 10000
	bulkQueryOrganizationID = "organization_uuid"
)

// BonusBulkAdapter implements bulkengine.BulkAdapter for bonus accruals.
type BonusBulkAdapter struct{ svc *Service }

// NewBonusBulkAdapter creates the bonus accrual bulk adapter.
func NewBonusBulkAdapter(svc *Service) *BonusBulkAdapter { return &BonusBulkAdapter{svc: svc} }

// Resource implements bulkengine.BulkAdapter.
func (a *BonusBulkAdapter) Resource() string { return ResourceBonusBulk }

// BulkActions implements bulkengine.BulkAdapter.
func (a *BonusBulkAdapter) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{{
		ID: BulkApproveBonus, LabelKey: "bulk.actions.performance_bonuses.approve", Permission: rbac.PermPerformanceBonusManage,
		ConfirmKey: "bulk.confirm.performance_bonuses.approve",
	}}
}

// caller rebuilds the dealer caller of a run from the stamped query.
func (a *BonusBulkAdapter) caller(ctx context.Context, query map[string]string) (Caller, error) {
	orgUUID, err := uuid.Parse(query[bulkQueryOrganizationID])
	if err != nil {
		return Caller{}, errors.New("organization scope is required")
	}
	org, err := a.svc.q.GetOrganizationByUUID(ctx, orgUUID)
	if err != nil {
		return Caller{}, errors.New("organization scope is required")
	}
	actor, _ := strconv.ParseInt(query[bulkengine.QueryActorUserID], 10, 64)
	if actor <= 0 {
		return Caller{}, errors.New("actor is required")
	}
	c := Caller{}
	c.Principal.UserInternal = actor
	c.Org.InternalID, c.Org.UUID, c.Org.BrandID, c.Org.OrgType, c.Org.Name = org.ID, org.Uuid, org.BrandID, org.Type, org.Name
	return c, nil
}

// ResolveTargets implements bulkengine.BulkAdapter: selected ids, or every
// calculated accrual matching the GET /v1/performance/bonuses filters
// (period, user_id) of target.query.
func (a *BonusBulkAdapter) ResolveTargets(ctx context.Context, action string, target bulkengine.BulkTarget) ([]string, error) {
	if action != BulkApproveBonus {
		return nil, errors.New("unknown action")
	}
	c, err := a.caller(ctx, target.Query)
	if err != nil {
		return nil, err
	}
	if err := a.svc.bonusDealer(ctx, c); err != nil {
		return nil, errors.New("bonuses are not available for this organization")
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
	values := map[string][]string{}
	for k, v := range target.Query {
		values[k] = []string{v}
	}
	f, err := ParseBonusFilter(values, a.svc.now())
	if err != nil {
		return nil, err
	}
	rows, err := a.svc.q.ListBonusAccruals(ctx, db.ListBonusAccrualsParams{
		OrganizationID: c.Org.InternalID, Period: textArg(f.Period), Statuses: []string{model.AccrualCalculated},
		UserID: int8Ptr(f.UserID), RowLimit: maxBonusBulkTargets,
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Uuid.String())
	}
	return out, nil
}

// ApplyItem implements bulkengine.BulkAdapter.
func (a *BonusBulkAdapter) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	run, ok := bulkengine.RunFrom(ctx)
	if !ok {
		return bulkengine.BulkItemResult{}, errors.New("bulk run context is required")
	}
	c, err := a.caller(ctx, run.Query)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	fail := func(msg string) (bulkengine.BulkItemResult, error) {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, EntityType: "bonus_accrual", OK: false, Error: msg}, nil
	}
	if action != BulkApproveBonus {
		return fail("unknown action")
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return fail("not found")
	}
	_, err = a.svc.ApproveBonus(ctx, c, id, BonusApprovalInput{})
	switch {
	case errors.Is(err, ErrNotFound):
		return fail("not found or not calculated")
	case errors.Is(err, ErrForbidden):
		return fail("forbidden")
	case err != nil:
		var ve *ValidationError
		if errors.As(err, &ve) {
			return fail(ve.Field + ": " + ve.Message)
		}
		return bulkengine.BulkItemResult{}, err
	}
	return bulkengine.BulkItemResult{EntityUUID: entityUUID, EntityType: "bonus_accrual", OK: true, Op: "update"}, nil
}

// RevertItem implements bulkengine.BulkAdapter: an approval books a staff
// payment and is never undone.
func (a *BonusBulkAdapter) RevertItem(context.Context, string, string, map[string]any) error {
	return errors.New("bonus approval cannot be undone")
}

var _ bulkengine.BulkAdapter = (*BonusBulkAdapter)(nil)
