package adapters

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/orglist"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Platform organizations bulk actions (TEC-365). The platform bulk handler
// stamps the request brand as query orglist.QueryBrandID; every read and
// write is bound to it (K1/K20), so a job never touches another brand.

const ResourceOrganizations = "platform.organizations"

// ParamDays is the extend_access parameter (whole days, 1..MaxExtendDays).
const ParamDays = "days"

// MaxExtendDays caps one extend_access run (ten years).
const MaxExtendDays = 3650

// maxQueryTargets bounds a "select all matching" run.
const maxQueryTargets int32 = 10000

// orgStatusActions maps the status bulk actions to the status they set.
var orgStatusActions = map[string]string{
	"activate":      "active",
	"suspend":       "suspended",
	"set_read_only": "read_only",
}

// OrganizationsAdapter changes status / extends access of platform
// organizations; every action is reversible (undo restores both values).
type OrganizationsAdapter struct {
	q *db.Queries
}

func NewOrganizations(q *db.Queries) *OrganizationsAdapter {
	return &OrganizationsAdapter{q: q}
}

func (a *OrganizationsAdapter) Resource() string { return ResourceOrganizations }

// WithQueries binds the adapter to a transaction (bulkengine.TxBinder).
func (a *OrganizationsAdapter) WithQueries(q *db.Queries) bulkengine.BulkAdapter {
	return &OrganizationsAdapter{q: q}
}

func (a *OrganizationsAdapter) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: "activate", LabelKey: "bulk.actions.organizations.activate",
			Permission: rbac.PermPlatformOrganizationsWrite, Reversible: true,
		},
		{
			ID: "suspend", LabelKey: "bulk.actions.organizations.suspend",
			Permission: rbac.PermPlatformOrganizationsWrite, Destructive: true, Reversible: true,
			ConfirmKey: "bulk.confirm.organizations.suspend",
		},
		{
			ID: "set_read_only", LabelKey: "bulk.actions.organizations.set_read_only",
			Permission: rbac.PermPlatformOrganizationsWrite, Destructive: true, Reversible: true,
			ConfirmKey: "bulk.confirm.organizations.set_read_only",
		},
		{
			ID: "extend_access", LabelKey: "bulk.actions.organizations.extend_access",
			Permission: rbac.PermPlatformOrganizationsWrite, Reversible: true,
			ConfirmKey: "bulk.confirm.organizations.extend_access",
			Params: []bulkengine.BulkActionParam{
				{Key: ParamDays, Kind: "number", Required: true, LabelKey: "bulk.params.days"},
			},
		},
	}
}

// ErrBrandScope marks a platform bulk run without the stamped brand.
var ErrBrandScope = errors.New("brand scope is required")

func brandOf(query map[string]string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(query[orglist.QueryBrandID]), 10, 64)
	if err != nil || id <= 0 {
		return 0, ErrBrandScope
	}
	return id, nil
}

func runBrand(ctx context.Context) (int64, error) {
	run, ok := bulkengine.RunFrom(ctx)
	if !ok {
		return 0, ErrBrandScope
	}
	return brandOf(run.Query)
}

func parseDays(params map[string]string) (int, error) {
	raw := strings.TrimSpace(params[ParamDays])
	days, err := strconv.Atoi(raw)
	if err != nil || days < 1 || days > MaxExtendDays {
		return 0, fmt.Errorf("%s must be a whole number between 1 and %d", ParamDays, MaxExtendDays)
	}
	return days, nil
}

func (a *OrganizationsAdapter) ResolveTargets(ctx context.Context, action string, target bulkengine.BulkTarget) ([]string, error) {
	brandID, err := brandOf(target.Query)
	if err != nil {
		return nil, err
	}
	if action == "extend_access" {
		if _, err := parseDays(target.Params); err != nil {
			return nil, err
		}
	}
	switch target.Scope {
	case "ids":
		return uniqueNonEmpty(target.IDs)
	case "query":
	default:
		return nil, fmt.Errorf("invalid target scope")
	}
	f, err := orglist.Parse(orglist.QueryValues(target.Query))
	if err != nil {
		return nil, err
	}
	params, err := orglist.Params(ctx, a.q, brandID, f)
	if err != nil {
		return nil, err
	}
	params.LimitCount, params.OffsetCount = maxQueryTargets, 0
	rows, err := a.q.ListOrganizationsFiltered(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Organization.Uuid.String())
	}
	return out, nil
}

// orgState is the undo state of one organization: status and access end
// (RFC3339 UTC, nil when open ended).
func orgState(o db.Organization) map[string]any {
	var ends any
	if o.AccessEndsAt.Valid {
		ends = o.AccessEndsAt.Time.UTC().Format(time.RFC3339Nano)
	}
	return map[string]any{"status": o.Status, "access_ends_at": ends}
}

// load returns the organization when it belongs to brandID.
func (a *OrganizationsAdapter) load(ctx context.Context, brandID int64, entityUUID string) (db.Organization, bool, error) {
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return db.Organization{}, false, nil
	}
	o, err := a.q.GetOrganizationByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, false, nil
	}
	if err != nil {
		return db.Organization{}, false, err
	}
	if o.BrandID != brandID {
		return db.Organization{}, false, nil
	}
	return o, true, nil
}

// CurrentState reports the live status / access end for the undo conflict check.
func (a *OrganizationsAdapter) CurrentState(ctx context.Context, _ string, entityUUID string) (map[string]any, error) {
	brandID, err := runBrand(ctx)
	if err != nil {
		return nil, err
	}
	o, ok, err := a.load(ctx, brandID, entityUUID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, bulkengine.ErrEntityGone
	}
	return orgState(o), nil
}

func (a *OrganizationsAdapter) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	fail := func(msg string) (bulkengine.BulkItemResult, error) {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: msg}, nil
	}
	brandID, err := runBrand(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	o, ok, err := a.load(ctx, brandID, entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	if !ok {
		return fail("not found")
	}
	if o.Type == "center" {
		return fail("the brand center cannot be changed in bulk")
	}
	next := db.SetOrganizationBulkStateParams{
		Uuid: o.Uuid, BrandID: brandID, Status: o.Status, AccessEndsAt: o.AccessEndsAt,
	}
	if status, isStatus := orgStatusActions[action]; isStatus {
		next.Status = status
	} else if action == "extend_access" {
		run, _ := bulkengine.RunFrom(ctx)
		days, err := parseDays(run.Params)
		if err != nil {
			return fail(err.Error())
		}
		base := time.Now().UTC()
		if o.AccessEndsAt.Valid && o.AccessEndsAt.Time.After(base) {
			base = o.AccessEndsAt.Time.UTC()
		}
		next.AccessEndsAt = pgtype.Timestamptz{Time: base.AddDate(0, 0, days).Truncate(time.Microsecond), Valid: true}
	} else {
		return fail("unknown action")
	}
	previous := orgState(o)
	updated, err := a.write(ctx, next)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	return bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "organization", OK: true, Op: "update",
		Previous: previous, Applied: orgState(updated),
	}, nil
}

func (a *OrganizationsAdapter) RevertItem(ctx context.Context, _ string, entityUUID string, previous map[string]any) error {
	brandID, err := runBrand(ctx)
	if err != nil {
		return err
	}
	o, ok, err := a.load(ctx, brandID, entityUUID)
	if err != nil || !ok {
		return err
	}
	prev := db.SetOrganizationBulkStateParams{Uuid: o.Uuid, BrandID: brandID, Status: o.Status}
	if s, _ := previous["status"].(string); s != "" {
		prev.Status = s
	}
	if raw, _ := previous["access_ends_at"].(string); raw != "" {
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return err
		}
		prev.AccessEndsAt = pgtype.Timestamptz{Time: t, Valid: true}
	}
	_, err = a.write(ctx, prev)
	return err
}

// write updates the organization and emits organization.updated on the
// same queries (transaction), as the organizations module does.
func (a *OrganizationsAdapter) write(ctx context.Context, p db.SetOrganizationBulkStateParams) (db.Organization, error) {
	o, err := a.q.SetOrganizationBulkState(ctx, p)
	if err != nil {
		return db.Organization{}, err
	}
	id, uid := o.ID, o.Uuid
	payload := map[string]any{
		"organization_uuid": o.Uuid.String(),
		"organization_id":   o.ID,
		"brand_id":          o.BrandID,
		"type":              o.Type,
		"slug":              o.Slug,
	}
	if o.ParentID.Valid {
		payload["parent_id"] = o.ParentID.Int64
	}
	ev := events.New(events.OrganizationUpdated).WithTenant(o.ID).WithEntity("organization", &id, &uid).WithPayload(payload)
	if err := outbox.EnqueueQueries(ctx, a.q, ev); err != nil {
		return db.Organization{}, err
	}
	return o, nil
}
