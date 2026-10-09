package usecase

// TEC-504 (F5-08d): bulk "create draft" on the billable sources (bulk
// engine resource "einvoices.billable", POST /v1/einvoices/billable/bulk,
// einvoice.manage). The bulk handler stamps the active organization and the
// caller on the target; every item runs CreateDraft as that center, so the
// same rules apply as to a single draft (buyer profile, settings, an active
// invoice). A draft is released by voiding it, so the action is not
// reversible. The ids are source uuids (orders and subscription periods
// share no uuid); the source type is found by trying an order first.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/ubl"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ResourceBulk is the bulk engine resource of the billable sources.
const ResourceBulk = "einvoices.billable"

// BulkCreateDraft is the bulk action id.
const BulkCreateDraft = "create_draft"

const maxBulkQueryTargets = 10000

// BillableSortSpec is the list contract of GET /v1/einvoices/billable.
var BillableSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"billable_at": "billable_at", "source_no": "source_no", "payable": "payable", "buyer_name": "buyer_name",
	},
	Default: apiquery.SortField{Field: "billable_at", Desc: true},
}

// ParseBillableQuery maps the GET /v1/einvoices/billable query (also the
// query of a bulk run) to the list params; brand and center are set by
// ListBillable.
func ParseBillableQuery(vals url.Values) (db.ListEinvoiceBillableSourcesParams, error) {
	qp := apiquery.Parse(vals)
	sort, err := apiquery.ResolveSort(qp.Sort, BillableSortSpec)
	if err != nil {
		return db.ListEinvoiceBillableSourcesParams{}, err
	}
	types, err := apiquery.EnumList(vals, "source_type", SourceTypes...)
	if err != nil {
		return db.ListEinvoiceBillableSourcesParams{}, err
	}
	billable, err := apiquery.DateRange(vals, "billable_at")
	if err != nil {
		return db.ListEinvoiceBillableSourcesParams{}, err
	}
	payable, err := apiquery.NumRange(vals, "payable")
	if err != nil {
		return db.ListEinvoiceBillableSourcesParams{}, err
	}
	var buyers []uuid.UUID
	for _, v := range apiquery.CSVValues(vals, "buyer") {
		id, err := uuid.Parse(strings.TrimSpace(v))
		if err != nil {
			return db.ListEinvoiceBillableSourcesParams{}, invalid("buyer", "must be UUIDs")
		}
		buyers = append(buyers, id)
	}
	p := db.ListEinvoiceBillableSourcesParams{
		SourceTypes: types, BuyerOrgUuids: buyers, PayableMin: numericArg(payable.Min),
		PayableMax: numericArg(payable.Max), Q: text(qp.Q), SortKey: sort.Key, SortDesc: sort.Desc,
		RowLimit: qp.Limit, RowOffset: qp.Offset,
	}
	if billable.From != nil {
		p.BillableFrom = pgtype.Timestamptz{Time: *billable.From, Valid: true}
	}
	if billable.Before != nil {
		p.BillableBefore = pgtype.Timestamptz{Time: *billable.Before, Valid: true}
	}
	return p, nil
}

func numericArg(f *float64) pgtype.Numeric {
	var n pgtype.Numeric
	if f == nil {
		return n
	}
	_ = n.Scan(strconv.FormatFloat(*f, 'f', -1, 64))
	return n
}

// BulkAdapter implements bulkengine.BulkAdapter for the billable sources.
type BulkAdapter struct {
	svc *Service
	q   *db.Queries
}

// NewBulkAdapter creates the billable source bulk adapter.
func NewBulkAdapter(svc *Service, q *db.Queries) *BulkAdapter { return &BulkAdapter{svc: svc, q: q} }

// Resource implements bulkengine.BulkAdapter.
func (a *BulkAdapter) Resource() string { return ResourceBulk }

// BulkActions implements bulkengine.BulkAdapter.
func (a *BulkAdapter) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{{
		ID: BulkCreateDraft, LabelKey: "bulk.actions.einvoices.create_draft", Permission: rbac.PermEinvoiceManage,
		ConfirmKey: "bulk.confirm.einvoices.create_draft",
	}}
}

// caller rebuilds the center of the run from the stamped target query.
func (a *BulkAdapter) caller(ctx context.Context, query map[string]string) (Caller, error) {
	orgUUID, err := uuid.Parse(query["organization_uuid"])
	if err != nil {
		return Caller{}, errors.New("organization scope is required")
	}
	org, err := a.q.GetOrganizationByUUID(ctx, orgUUID)
	if err != nil {
		return Caller{}, fmt.Errorf("organization scope: %w", err)
	}
	if org.Type != "center" {
		return Caller{}, ErrForbidden
	}
	actor, _ := strconv.ParseInt(query[bulkengine.QueryActorUserID], 10, 64)
	return Caller{
		Principal: authctx.Principal{UserInternal: actor},
		Org: orgctx.Scope{
			InternalID: org.ID, UUID: org.Uuid, Slug: org.Slug, Name: org.Name, Status: org.Status,
			OrgType: org.Type, BrandID: org.BrandID,
		},
	}, nil
}

// ResolveTargets implements bulkengine.BulkAdapter: the selected source
// uuids, or every billable source matching the list query.
func (a *BulkAdapter) ResolveTargets(ctx context.Context, _ string, target bulkengine.BulkTarget) ([]string, error) {
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
	vals := url.Values{}
	for k, v := range target.Query {
		if k == "organization_uuid" || strings.HasPrefix(k, "_") || k == "limit" || k == "offset" {
			continue
		}
		vals.Set(k, v)
	}
	p, err := ParseBillableQuery(vals)
	if err != nil {
		return nil, err
	}
	p.RowLimit, p.RowOffset = maxBulkQueryTargets, 0
	rows, _, err := a.svc.ListBillable(ctx, c, p)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.SourceUUID.String())
	}
	return out, nil
}

// bulkErrorCode is the item error of a failed draft: the API error code,
// so the bulk summary reads like the single draft endpoint. ok is false
// for an unexpected error, which aborts the run.
func bulkErrorCode(err error) (string, bool) {
	var ue *ubl.Error
	switch {
	case errors.Is(err, ErrBuyerProfileIncomplete):
		return "EINVOICE_BUYER_PROFILE_INCOMPLETE", true
	case errors.Is(err, ErrSettingsRequired):
		return "EINVOICE_SETTINGS_REQUIRED", true
	case errors.Is(err, ErrSourceNotBillable):
		return "EINVOICE_SOURCE_NOT_BILLABLE", true
	case errors.Is(err, ErrAlreadyInvoiced):
		return "EINVOICE_SOURCE_ALREADY_INVOICED", true
	case errors.Is(err, ErrNotFound):
		return "NOT_FOUND", true
	case errors.Is(err, ErrForbidden):
		return "FORBIDDEN", true
	case errors.As(err, &ue):
		return ue.Code, true
	}
	return "", false
}

// ApplyItem implements bulkengine.BulkAdapter.
func (a *BulkAdapter) ApplyItem(ctx context.Context, _ string, entityUUID string) (bulkengine.BulkItemResult, error) {
	run, ok := bulkengine.RunFrom(ctx)
	if !ok {
		return bulkengine.BulkItemResult{}, errors.New("bulk run context is required")
	}
	c, err := a.caller(ctx, run.Query)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	fail := func(msg string) (bulkengine.BulkItemResult, error) {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: msg}, nil
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return fail("NOT_FOUND")
	}
	v, err := a.svc.CreateDraft(ctx, c, DraftInput{SourceType: SourceOrder, SourceUUID: id})
	if errors.Is(err, ErrNotFound) {
		v, err = a.svc.CreateDraft(ctx, c, DraftInput{SourceType: SourceSubscription, SourceUUID: id})
	}
	if err != nil {
		if code, ok := bulkErrorCode(err); ok {
			return fail(code)
		}
		return bulkengine.BulkItemResult{}, err
	}
	return bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "einvoice_source", OK: true, Op: "create",
		Applied: map[string]any{"einvoice_uuid": v.UUID.String()},
	}, nil
}

// RevertItem implements bulkengine.BulkAdapter; drafts are released by
// voiding them, the action is not reversible.
func (a *BulkAdapter) RevertItem(context.Context, string, string, map[string]any) error {
	return errors.New("einvoice drafts are released by voiding them")
}
