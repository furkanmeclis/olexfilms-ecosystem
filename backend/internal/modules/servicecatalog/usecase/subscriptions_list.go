package usecase

// TEC-311: list contract (docs/list-contract.md) of the subscription list,
// the center cancellation queue, and the assign dialog price preview.

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SubscriptionsSortSpec is the sort whitelist of GET /v1/service-subscriptions.
var SubscriptionsSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"created_at": "created_at", "starts_on": "starts_on", "ends_on": "ends_on",
		"status": "status", "price": "price", "organization_name": "organization_name", "item_name": "item_name",
	},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// CancelRequestsSortSpec is the sort whitelist of
// GET /v1/service-subscriptions/cancel-requests.
var CancelRequestsSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"created_at": "created_at", "status": "status", "cancellation_fee": "cancellation_fee",
		"organization_name": "organization_name", "item_name": "item_name",
	},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// Filter values of the lists.
var (
	SubscriptionStatuses  = []string{StatusScheduled, StatusActive, StatusCancelRequested, StatusCancelled, StatusExpired}
	CancelRequestStatuses = []string{"pending", StatusApproved, StatusRejected}
)

// SubscriptionListFilter is the parsed GET /v1/service-subscriptions query.
type SubscriptionListFilter struct {
	Q             string
	Statuses      []string
	Organizations []uuid.UUID
	Items         []uuid.UUID
	Ends          apiquery.TimeRange
	Sort          apiquery.ResolvedSort
	Limit         int32
	Offset        int32
}

// CancelRequestListFilter is the parsed cancellation queue query.
type CancelRequestListFilter struct {
	Q        string
	Statuses []string
	Created  apiquery.TimeRange
	Sort     apiquery.ResolvedSort
	Limit    int32
	Offset   int32
}

// ParseSubscriptionListFilter reads q, status / organization_uuid /
// item_uuid (CSV), ends_on_from/_to, sort, limit and offset.
func ParseSubscriptionListFilter(values url.Values) (SubscriptionListFilter, error) {
	q := apiquery.Parse(values)
	f := SubscriptionListFilter{Q: strings.TrimSpace(q.Q), Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.Sort, err = apiquery.ResolveSort(q.Sort, SubscriptionsSortSpec); err != nil {
		return f, err
	}
	if f.Statuses, err = apiquery.EnumList(values, "status", SubscriptionStatuses...); err != nil {
		return f, err
	}
	if f.Organizations, err = uuidList(values, "organization_uuid"); err != nil {
		return f, err
	}
	if f.Items, err = uuidList(values, "item_uuid"); err != nil {
		return f, err
	}
	if f.Ends, err = apiquery.DateRange(values, "ends_on"); err != nil {
		return f, err
	}
	return f, nil
}

// ParseCancelRequestListFilter reads q, status (CSV), created_from/_to,
// sort, limit and offset.
func ParseCancelRequestListFilter(values url.Values) (CancelRequestListFilter, error) {
	q := apiquery.Parse(values)
	f := CancelRequestListFilter{Q: strings.TrimSpace(q.Q), Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.Sort, err = apiquery.ResolveSort(q.Sort, CancelRequestsSortSpec); err != nil {
		return f, err
	}
	if f.Statuses, err = apiquery.EnumList(values, "status", CancelRequestStatuses...); err != nil {
		return f, err
	}
	if f.Created, err = apiquery.DateRange(values, "created"); err != nil {
		return f, err
	}
	return f, nil
}

func uuidList(values url.Values, key string) ([]uuid.UUID, error) {
	raw := apiquery.CSVValues(values, key)
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]uuid.UUID, 0, len(raw))
	for _, v := range raw {
		id, err := uuid.Parse(v)
		if err != nil {
			return nil, &apiquery.ValidationError{Details: []apiquery.Detail{{Field: key, Message: "must be UUIDs", Code: "invalid"}}}
		}
		out = append(out, id)
	}
	return out, nil
}

// ListSubscriptions returns one page of the subscriptions the caller's
// service_subscriptions.read scope reaches.
func (s *Service) ListSubscriptions(ctx context.Context, c Caller, f SubscriptionListFilter) ([]SubscriptionView, int64, error) {
	args := db.ListServiceSubscriptionsPageParams{
		BrandID: c.Org.BrandID, OrganizationIds: c.Filter.OrgIDsArg(), Statuses: f.Statuses,
		OrgUuids: f.Organizations, ItemUuids: f.Items, Q: textNarg(f.Q),
		EndsFrom: dateNarg(f.Ends.From), EndsBefore: dateNarg(f.Ends.Before),
		SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	rows, err := s.q.ListServiceSubscriptionsPage(ctx, args)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountServiceSubscriptionsPage(ctx, db.CountServiceSubscriptionsPageParams{
		BrandID: args.BrandID, OrganizationIds: args.OrganizationIds, Statuses: args.Statuses,
		OrgUuids: args.OrgUuids, ItemUuids: args.ItemUuids, Q: args.Q,
		EndsFrom: args.EndsFrom, EndsBefore: args.EndsBefore,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]SubscriptionView, 0, len(rows))
	for _, row := range rows {
		v, err := s.subscriptionRowView(ctx, row)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, nil
}

// CancelQueueView is one row of the center cancellation queue.
type CancelQueueView struct {
	CancelRequestView
	SubscriptionStatus string    `json:"subscription_status"`
	StartsOn           string    `json:"starts_on"`
	EndsOn             string    `json:"ends_on"`
	OrganizationUUID   uuid.UUID `json:"organization_uuid"`
	OrganizationName   string    `json:"organization_name"`
	ItemName           string    `json:"item_name"`
}

// ListCancelRequests returns one page of the brand's cancellation requests
// (center only, like the approve / reject decisions).
func (s *Service) ListCancelRequests(ctx context.Context, c Caller, f CancelRequestListFilter) ([]CancelQueueView, int64, error) {
	if c.Org.OrgType != rbac.OrgTypeCenter {
		return nil, 0, ErrForbidden
	}
	args := db.ListServiceSubscriptionCancelRequestsPageParams{
		BrandID: c.Org.BrandID, Statuses: f.Statuses, Q: textNarg(f.Q),
		CreatedFrom: tsNarg(f.Created.From), CreatedBefore: tsNarg(f.Created.Before),
		SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	rows, err := s.q.ListServiceSubscriptionCancelRequestsPage(ctx, args)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountServiceSubscriptionCancelRequestsPage(ctx, db.CountServiceSubscriptionCancelRequestsPageParams{
		BrandID: args.BrandID, Statuses: args.Statuses, Q: args.Q,
		CreatedFrom: args.CreatedFrom, CreatedBefore: args.CreatedBefore,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]CancelQueueView, 0, len(rows))
	for _, row := range rows {
		out = append(out, CancelQueueView{
			CancelRequestView:  s.cancelView(ctx, row.ServiceSubscriptionCancelRequest, row.SubscriptionUuid),
			SubscriptionStatus: row.SubscriptionStatus,
			StartsOn:           row.StartsOn.Time.Format(time.DateOnly),
			EndsOn:             row.EndsOn.Time.Format(time.DateOnly),
			OrganizationUUID:   row.OrgUuid,
			OrganizationName:   row.OrganizationName,
			ItemName:           row.ItemName,
		})
	}
	return out, total, nil
}

// PreviewPrice resolves the price an assignment of item to the target
// organization would freeze, with the same target rules as Assign.
func (s *Service) PreviewPrice(ctx context.Context, c Caller, itemID, orgID uuid.UUID) (Price, error) {
	if c.Org.OrgType == rbac.OrgTypeDealer {
		return Price{}, ErrForbidden
	}
	item, err := s.item(ctx, c.Org.BrandID, itemID)
	if err != nil {
		return Price{}, err
	}
	if !item.IsActive {
		return Price{}, ErrNotFound
	}
	target, err := s.targetOrg(ctx, c, orgID)
	if err != nil {
		return Price{}, err
	}
	p, err := s.ResolvePrice(ctx, item, target)
	if errors.Is(err, ErrInvalidBuyerOrg) || errors.Is(err, pgx.ErrNoRows) {
		return Price{}, ErrInvalidTarget
	}
	return p, err
}

func textNarg(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func dateNarg(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return dateArg(t.UTC())
}

func tsNarg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
