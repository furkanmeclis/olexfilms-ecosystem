package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	StatusActive          = "active"
	StatusCancelRequested = "cancel_requested"
	StatusApproved        = "approved"
	StatusCancelled       = "cancelled"
	StatusRejected        = "rejected"
)

var (
	ErrInvalidTarget         = errors.New("service subscriptions: invalid target")
	ErrInvalidStatus         = errors.New("service subscriptions: invalid status")
	ErrDuplicateCancel       = errors.New("service subscriptions: cancellation already pending")
	ErrModuleBlockedByParent = errors.New("service subscriptions: module blocked by parent")
	ErrRateNotFound          = errors.New("service subscriptions: exchange rate not found")
	ErrNotConfigured         = errors.New("service subscriptions: lifecycle dependencies are not configured")
)

// TxBeginner starts a transaction.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// RateResolver resolves a rate on a day.
type RateResolver interface {
	ResolveRate(ctx context.Context, on time.Time, base, quote string) (fxrates.Snapshot, error)
}

// FeatureService is the part of the feature flag service used by module
// bundle subscriptions.
type FeatureService interface {
	SetByService(ctx context.Context, q *db.Queries, actorID, orgID, serviceID int64, key string) error
	ClearByService(ctx context.Context, q *db.Queries, orgID int64, key string) error
	InvalidateOrg(ctx context.Context, orgID int64)
}

// Outbox stores domain events in the current transaction.
type Outbox interface {
	Enqueue(ctx context.Context, tx pgx.Tx, ev events.Event) error
}

// WithLifecycle wires the dependencies needed by subscription mutations.
func (s *Service) WithLifecycle(pool TxBeginner, out Outbox, rates RateResolver, feature FeatureService) *Service {
	s.pool = pool
	s.out = out
	s.rates = rates
	s.feature = feature
	return s
}

type Caller struct {
	Principal authctx.Principal
	Org       orgctx.Scope
	Filter    scopefilter.Filter
}

func (c Caller) actor() pgtype.Int8 {
	if c.Principal.UserInternal == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: c.Principal.UserInternal, Valid: true}
}

type SubscriptionInput struct {
	ItemUUID         uuid.UUID
	OrganizationUUID uuid.UUID
	StartsOn         time.Time
	EndsOn           time.Time
}

type CancelRequestInput struct {
	Reason string
}

type DecisionInput struct {
	Note *string
}

type SubscriptionView struct {
	UUID             uuid.UUID          `json:"uuid"`
	OrganizationUUID uuid.UUID          `json:"organization_uuid"`
	ItemUUID         uuid.UUID          `json:"item_uuid"`
	AssignedByOrgID  int64              `json:"assigned_by_org_id"`
	StartsOn         string             `json:"starts_on"`
	EndsOn           string             `json:"ends_on"`
	Recurrence       string             `json:"recurrence"`
	Price            string             `json:"price"`
	Currency         string             `json:"currency"`
	RateSnapshot     json.RawMessage    `json:"rate_snapshot"`
	CancellationFee  string             `json:"cancellation_fee"`
	Status           string             `json:"status"`
	Contract         *ContractSummary   `json:"contract,omitempty"`
	CreatedAt        pgtype.Timestamptz `json:"created_at"`
	// TEC-311: display names for the subscription screens.
	OrganizationName string `json:"organization_name"`
	ItemName         string `json:"item_name"`
	ItemCategory     string `json:"item_category"`

	contractID pgtype.Int8
}

type ContractSummary struct {
	UUID       uuid.UUID `json:"uuid"`
	ContractNo int64     `json:"contract_no"`
	Status     string    `json:"status"`
	PDFReady   bool      `json:"pdf_ready"`
}

type CancelRequestView struct {
	UUID             uuid.UUID          `json:"uuid"`
	SubscriptionUUID uuid.UUID          `json:"subscription_uuid"`
	Reason           string             `json:"reason"`
	Status           string             `json:"status"`
	CancellationFee  string             `json:"cancellation_fee"`
	Currency         string             `json:"currency"`
	DecisionNote     *string            `json:"decision_note,omitempty"`
	CreatedAt        pgtype.Timestamptz `json:"created_at"`
}

func (s *Service) Assign(ctx context.Context, c Caller, in SubscriptionInput) (SubscriptionView, error) {
	if s.pool == nil || s.queries == nil || s.rates == nil {
		return SubscriptionView{}, ErrNotConfigured
	}
	if c.Org.OrgType == rbac.OrgTypeDealer {
		return SubscriptionView{}, ErrForbidden
	}
	if !in.EndsOn.After(in.StartsOn) {
		return SubscriptionView{}, invalid("ends_on", "must be after starts_on")
	}
	item, err := s.item(ctx, c.Org.BrandID, in.ItemUUID)
	if err != nil {
		return SubscriptionView{}, err
	}
	if !item.IsActive {
		return SubscriptionView{}, ErrNotFound
	}
	target, err := s.targetOrg(ctx, c, in.OrganizationUUID)
	if err != nil {
		return SubscriptionView{}, err
	}
	price, err := s.ResolvePrice(ctx, item, target)
	if err != nil {
		return SubscriptionView{}, err
	}
	priceNum, err := parsePrice(price.Amount)
	if err != nil {
		return SubscriptionView{}, err
	}
	snap, err := s.rateSnapshot(ctx, in.StartsOn, price.Currency, target.Currency)
	if err != nil {
		return SubscriptionView{}, err
	}
	var sub db.ServiceSubscription
	if err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		sub, err = q.CreateServiceSubscription(ctx, db.CreateServiceSubscriptionParams{
			OrganizationID: target.ID, BrandID: c.Org.BrandID, SellerOrgID: item.OrganizationID,
			ItemID: item.ID, AssignedByOrgID: c.Org.InternalID, AssignedByUserID: c.actor(),
			StartsOn: dateArg(in.StartsOn), EndsOn: dateArg(in.EndsOn), Recurrence: item.Recurrence,
			Price: priceNum, Currency: price.Currency, RateSnapshot: snap,
			CancellationFee: item.CancellationFee,
		})
		if err != nil {
			return mapDBError(err)
		}
		if item.ContractTemplateID.Valid {
			sub, err = s.createSubscriptionContract(ctx, q, c, sub, item, target)
			if err != nil {
				return err
			}
		}
		if err := s.openModules(ctx, q, sub, item, c.Principal.UserInternal); err != nil {
			return err
		}
		return s.emit(ctx, tx, events.ServiceSubscriptionAssigned, sub, item, c.Principal.UserInternal, nil)
	}); err != nil {
		return SubscriptionView{}, err
	}
	s.invalidateModules(ctx, sub, item)
	return s.subscriptionView(ctx, sub)
}

func (s *Service) GetSubscription(ctx context.Context, c Caller, id uuid.UUID) (SubscriptionView, error) {
	sub, err := s.q.GetServiceSubscriptionByUUID(ctx, db.GetServiceSubscriptionByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return SubscriptionView{}, ErrNotFound
	}
	if err != nil {
		return SubscriptionView{}, err
	}
	if !c.Filter.AllowsOrg(sub.OrganizationID, sub.BrandID) {
		return SubscriptionView{}, ErrNotFound
	}
	return s.subscriptionView(ctx, sub)
}

func (s *Service) RequestCancel(ctx context.Context, c Caller, id uuid.UUID, in CancelRequestInput) (CancelRequestView, error) {
	reason, err := requiredText("reason", &in.Reason, 5000)
	if err != nil {
		return CancelRequestView{}, err
	}
	sub, err := s.q.GetServiceSubscriptionByUUID(ctx, db.GetServiceSubscriptionByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return CancelRequestView{}, ErrNotFound
	}
	if err != nil {
		return CancelRequestView{}, err
	}
	if sub.OrganizationID != c.Org.InternalID {
		return CancelRequestView{}, ErrNotFound
	}
	var req db.ServiceSubscriptionCancelRequest
	if err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		sub, err = q.SetServiceSubscriptionCancelRequested(ctx, db.SetServiceSubscriptionCancelRequestedParams{ID: sub.ID, BrandID: sub.BrandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidStatus
		}
		if err != nil {
			return err
		}
		req, err = q.CreateServiceSubscriptionCancelRequest(ctx, db.CreateServiceSubscriptionCancelRequestParams{
			SubscriptionID: sub.ID, OrganizationID: sub.OrganizationID, BrandID: sub.BrandID,
			RequestedByUserID: c.actor(), RequestedByOrgID: c.Org.InternalID, Reason: reason,
			CancellationFee: sub.CancellationFee, Currency: sub.Currency,
		})
		if err != nil {
			return mapDBError(err)
		}
		item, err := q.GetServiceCatalogItem(ctx, db.GetServiceCatalogItemParams{ID: sub.ItemID, BrandID: sub.BrandID})
		if err != nil {
			return err
		}
		return s.emit(ctx, tx, events.ServiceSubscriptionCancelRequested, sub, item, c.Principal.UserInternal, map[string]any{"reason": reason})
	}); err != nil {
		return CancelRequestView{}, err
	}
	return s.cancelView(ctx, req, sub.Uuid), nil
}

func (s *Service) ApproveCancel(ctx context.Context, c Caller, id uuid.UUID, in DecisionInput) (CancelRequestView, error) {
	return s.decide(ctx, c, id, StatusApproved, events.ServiceSubscriptionCancelled, in)
}

func (s *Service) RejectCancel(ctx context.Context, c Caller, id uuid.UUID, in DecisionInput) (CancelRequestView, error) {
	return s.decide(ctx, c, id, StatusRejected, events.ServiceSubscriptionCancelRejected, in)
}

func (s *Service) decide(ctx context.Context, c Caller, id uuid.UUID, status, eventName string, in DecisionInput) (CancelRequestView, error) {
	if c.Org.OrgType != rbac.OrgTypeCenter {
		return CancelRequestView{}, ErrForbidden
	}
	req, err := s.q.GetServiceSubscriptionCancelRequestByUUID(ctx, db.GetServiceSubscriptionCancelRequestByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return CancelRequestView{}, ErrNotFound
	}
	if err != nil {
		return CancelRequestView{}, err
	}
	var (
		sub    db.ServiceSubscription
		closed bool
	)
	if err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		req, err = q.DecideServiceSubscriptionCancelRequest(ctx, db.DecideServiceSubscriptionCancelRequestParams{
			Status: status, DecidedByUserID: c.actor(), DecisionNote: nullableTextArg(in.Note),
			ID: req.ID, BrandID: c.Org.BrandID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidStatus
		}
		if err != nil {
			return err
		}
		sub, err = q.LockServiceSubscription(ctx, db.LockServiceSubscriptionParams{ID: req.SubscriptionID, BrandID: c.Org.BrandID})
		if err != nil {
			return err
		}
		item, err := q.GetServiceCatalogItem(ctx, db.GetServiceCatalogItemParams{ID: sub.ItemID, BrandID: sub.BrandID})
		if err != nil {
			return err
		}
		switch {
		case status == StatusApproved:
			// The subscription may have expired while the request was
			// pending; final states cannot change (409).
			if sub.Status != StatusActive && sub.Status != StatusCancelRequested {
				return ErrInvalidStatus
			}
			sub, err = q.SetServiceSubscriptionStatus(ctx, db.SetServiceSubscriptionStatusParams{
				Status: StatusCancelled, ID: sub.ID, BrandID: sub.BrandID,
			})
			if err != nil {
				return err
			}
			if err := s.closeModules(ctx, q, sub, item); err != nil {
				return err
			}
			closed = item.Category == CategoryModuleBundle
		case sub.Status == StatusCancelRequested:
			// A rejected request returns the subscription to active so it
			// keeps running and can be cancelled again later.
			sub, err = q.SetServiceSubscriptionStatus(ctx, db.SetServiceSubscriptionStatusParams{
				Status: StatusActive, ID: sub.ID, BrandID: sub.BrandID,
			})
			if err != nil {
				return err
			}
		}
		return s.emit(ctx, tx, eventName, sub, item, c.Principal.UserInternal, map[string]any{"decision_note": valueOrEmpty(in.Note)})
	}); err != nil {
		return CancelRequestView{}, err
	}
	if closed && s.feature != nil {
		s.feature.InvalidateOrg(ctx, sub.OrganizationID)
	}
	return s.cancelView(ctx, req, sub.Uuid), nil
}

func (s *Service) inTx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("service subscriptions: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.queries.WithTx(tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("service subscriptions: commit: %w", err)
	}
	return nil
}

func (s *Service) targetOrg(ctx context.Context, c Caller, id uuid.UUID) (db.Organization, error) {
	org, err := s.q.GetOrganizationByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || org.BrandID != c.Org.BrandID {
		return db.Organization{}, ErrNotFound
	}
	if err != nil {
		return db.Organization{}, err
	}
	switch c.Org.OrgType {
	case rbac.OrgTypeCenter:
		if org.Type != rbac.OrgTypeDistributor && org.Type != rbac.OrgTypeDealer {
			return db.Organization{}, ErrInvalidTarget
		}
		return org, nil
	case rbac.OrgTypeDistributor:
		if org.Type != rbac.OrgTypeDealer || !org.ParentID.Valid || org.ParentID.Int64 != c.Org.InternalID {
			return db.Organization{}, ErrNotFound
		}
		return org, nil
	default:
		return db.Organization{}, ErrForbidden
	}
}

func (s *Service) rateSnapshot(ctx context.Context, on time.Time, base, quote string) ([]byte, error) {
	snap, err := s.rates.ResolveRate(ctx, on, base, quote)
	if errors.Is(err, fxrates.ErrRateNotFound) {
		return nil, ErrRateNotFound
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(snap)
}

// openModules writes the bundle's module flags in the subscription
// transaction; a module closed upstream aborts the whole assignment.
func (s *Service) openModules(ctx context.Context, q *db.Queries, sub db.ServiceSubscription, item db.ServiceCatalogItem, actorID int64) error {
	if item.Category != CategoryModuleBundle || s.feature == nil {
		return nil
	}
	modules, err := q.ListServiceCatalogModules(ctx, item.ID)
	if err != nil {
		return err
	}
	for _, key := range modules {
		if err := s.feature.SetByService(ctx, q, actorID, sub.OrganizationID, sub.ID, key); errors.Is(err, features.ErrUpstreamDisabled) {
			return ErrModuleBlockedByParent
		} else if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) closeModules(ctx context.Context, q *db.Queries, sub db.ServiceSubscription, item db.ServiceCatalogItem) error {
	if item.Category != CategoryModuleBundle || s.feature == nil {
		return nil
	}
	modules, err := q.ListServiceCatalogModules(ctx, item.ID)
	if err != nil {
		return err
	}
	for _, key := range modules {
		n, err := q.CountActiveServiceModuleSubscriptions(ctx, db.CountActiveServiceModuleSubscriptionsParams{
			OrganizationID: sub.OrganizationID, BrandID: sub.BrandID, ModuleKey: key,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			if err := s.feature.ClearByService(ctx, q, sub.OrganizationID, key); err != nil {
				return err
			}
		}
	}
	return nil
}

// invalidateModules drops the feature cache of the receiving organization
// after a committed module bundle change.
func (s *Service) invalidateModules(ctx context.Context, sub db.ServiceSubscription, item db.ServiceCatalogItem) {
	if s.feature == nil || item.Category != CategoryModuleBundle {
		return
	}
	s.feature.InvalidateOrg(ctx, sub.OrganizationID)
}

func (s *Service) emit(ctx context.Context, tx pgx.Tx, name string, sub db.ServiceSubscription, item db.ServiceCatalogItem, actorID int64, extra map[string]any) error {
	org, _ := s.q.GetOrganizationByID(ctx, sub.OrganizationID)
	payload := map[string]any{
		"subscription_uuid": sub.Uuid.String(),
		"organization_id":   sub.OrganizationID,
		"organization_name": org.Name,
		"brand_id":          sub.BrandID,
		"item_uuid":         item.Uuid.String(),
		"item_name":         item.Name,
		"status":            sub.Status,
		"ends_on":           sub.EndsOn.Time.Format(time.DateOnly),
		"price":             numText(sub.Price),
		"currency":          sub.Currency,
		"cancellation_fee":  numText(sub.CancellationFee),
	}
	if ids, err := s.notificationUsers(ctx, name, sub); err == nil && len(ids) > 0 {
		payload["notify_user_ids"] = ids
	}
	for k, v := range extra {
		payload[k] = v
	}
	ev := events.New(name).WithTenant(sub.OrganizationID).WithEntity("service_subscription", &sub.ID, &sub.Uuid).WithPayload(payload)
	if actorID != 0 {
		ev = ev.WithActor(actorID)
	}
	if s.out == nil {
		return nil
	}
	return s.out.Enqueue(ctx, tx, ev)
}

func (s *Service) notificationUsers(ctx context.Context, name string, sub db.ServiceSubscription) ([]int64, error) {
	switch name {
	case events.ServiceSubscriptionCancelRequested:
		return s.q.ListUserIDsByRoleSlug(ctx, rbac.RoleSuperAdmin)
	case events.ServiceSubscriptionAssigned, events.ServiceSubscriptionCancelled, events.ServiceSubscriptionCancelRejected:
		return s.q.ListOrganizationOwnerUserIDs(ctx, sub.OrganizationID)
	default:
		return nil, nil
	}
}

func (s *Service) subscriptionView(ctx context.Context, sub db.ServiceSubscription) (SubscriptionView, error) {
	item, err := s.q.GetServiceCatalogItem(ctx, db.GetServiceCatalogItemParams{ID: sub.ItemID, BrandID: sub.BrandID})
	if err != nil {
		return SubscriptionView{}, err
	}
	org, err := s.q.GetOrganizationByID(ctx, sub.OrganizationID)
	if err != nil {
		return SubscriptionView{}, err
	}
	view := SubscriptionView{
		UUID: sub.Uuid, OrganizationUUID: org.Uuid, ItemUUID: item.Uuid, AssignedByOrgID: sub.AssignedByOrgID,
		StartsOn: sub.StartsOn.Time.Format(time.DateOnly), EndsOn: sub.EndsOn.Time.Format(time.DateOnly),
		Recurrence: sub.Recurrence, Price: numText(sub.Price), Currency: sub.Currency,
		RateSnapshot: json.RawMessage(sub.RateSnapshot), CancellationFee: numText(sub.CancellationFee),
		Status: sub.Status, CreatedAt: sub.CreatedAt,
		OrganizationName: org.Name, ItemName: item.Name, ItemCategory: item.Category,
		contractID: sub.ContractID,
	}
	return s.withContract(ctx, view)
}

// subscriptionRowView builds a list row view from the joined page row.
func (s *Service) subscriptionRowView(ctx context.Context, row db.ListServiceSubscriptionsPageRow) (SubscriptionView, error) {
	sub := row.ServiceSubscription
	view := SubscriptionView{
		UUID: sub.Uuid, OrganizationUUID: row.OrgUuid, ItemUUID: row.ItemUuid, AssignedByOrgID: sub.AssignedByOrgID,
		StartsOn: sub.StartsOn.Time.Format(time.DateOnly), EndsOn: sub.EndsOn.Time.Format(time.DateOnly),
		Recurrence: sub.Recurrence, Price: numText(sub.Price), Currency: sub.Currency,
		RateSnapshot: json.RawMessage(sub.RateSnapshot), CancellationFee: numText(sub.CancellationFee),
		Status: sub.Status, CreatedAt: sub.CreatedAt,
		OrganizationName: row.OrganizationName, ItemName: row.ItemName, ItemCategory: row.ItemCategory,
		contractID: sub.ContractID,
	}
	return s.withContract(ctx, view)
}

func (s *Service) withContract(ctx context.Context, view SubscriptionView) (SubscriptionView, error) {
	if view.contractID.Valid {
		inst, err := s.q.GetContractInstanceByID(ctx, view.contractID.Int64)
		if err != nil {
			return SubscriptionView{}, err
		}
		view.Contract = &ContractSummary{
			UUID: inst.Uuid, ContractNo: inst.ContractNo, Status: inst.Status, PDFReady: inst.PdfKey.Valid,
		}
	}
	return view, nil
}

func (s *Service) createSubscriptionContract(
	ctx context.Context,
	q *db.Queries,
	c Caller,
	sub db.ServiceSubscription,
	item db.ServiceCatalogItem,
	target db.Organization,
) (db.ServiceSubscription, error) {
	tpl, err := q.GetContractTemplateByID(ctx, item.ContractTemplateID.Int64)
	if errors.Is(err, pgx.ErrNoRows) || tpl.BrandID != sub.BrandID || tpl.Kind != "service_sale" || !tpl.IsActive {
		return db.ServiceSubscription{}, invalid("contract_template_id", "must reference an active service_sale template")
	}
	if err != nil {
		return db.ServiceSubscription{}, err
	}
	loc, err := subscriptionContractLocale(ctx, q, tpl.ID, target.Locale)
	if err != nil {
		return db.ServiceSubscription{}, err
	}
	rendered := pdfrender.Fill(pdfrender.SanitizeHTML(loc.Html), subscriptionContractValues(sub, item, target), nil)
	sum := sha256Hex([]byte(rendered))
	no, err := q.NextContractNo(ctx, sub.OrganizationID)
	if err != nil {
		return db.ServiceSubscription{}, err
	}
	inst, err := q.CreateContractInstance(ctx, db.CreateContractInstanceParams{
		OrganizationID: sub.OrganizationID, BrandID: sub.BrandID, ContractNo: no,
		SubjectType: "service_subscription", SubjectID: sub.ID, TemplateID: tpl.ID, Kind: tpl.Kind,
		Locale: loc.Locale, TemplateVersion: loc.Version, OtpRequired: tpl.OtpRequired,
		SignatureRequired: tpl.SignatureRequired, Status: "pending",
		RenderedHtml: pgText(rendered), ContentSha256: pgText(sum), CreatedByUserID: c.actor(),
	})
	if err != nil {
		return db.ServiceSubscription{}, err
	}
	owner, err := q.GetPrimaryOrganizationOwnerForServiceContract(ctx, sub.OrganizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ServiceSubscription{}, invalid("organization_uuid", "owner user is required")
	}
	if err != nil {
		return db.ServiceSubscription{}, err
	}
	if _, err := q.UpsertContractSigner(ctx, db.UpsertContractSignerParams{
		InstanceID: inst.ID, OrganizationID: inst.OrganizationID, BrandID: inst.BrandID,
		Role: "customer", UserID: pgtype.Int8{Int64: owner.ID, Valid: true},
		Name: userName(owner, "Customer"), PhoneE164: owner.PhoneE164,
	}); err != nil {
		return db.ServiceSubscription{}, err
	}
	staffName := "Staff"
	staffID := c.actor()
	if staffID.Valid {
		staff, err := q.GetUserByID(ctx, staffID.Int64)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return db.ServiceSubscription{}, err
		}
		if err == nil {
			staffName = userName(staff, "Staff")
		}
	}
	if _, err := q.UpsertContractSigner(ctx, db.UpsertContractSignerParams{
		InstanceID: inst.ID, OrganizationID: inst.OrganizationID, BrandID: inst.BrandID,
		Role: "staff", UserID: staffID, Name: staffName,
	}); err != nil {
		return db.ServiceSubscription{}, err
	}
	sub, err = q.SetServiceSubscriptionContract(ctx, db.SetServiceSubscriptionContractParams{
		ID: sub.ID, BrandID: sub.BrandID, ContractID: pgtype.Int8{Int64: inst.ID, Valid: true},
	})
	if err != nil {
		return db.ServiceSubscription{}, err
	}
	return sub, nil
}

func subscriptionContractLocale(ctx context.Context, q *db.Queries, templateID int64, requested string) (db.ContractTemplateLocale, error) {
	candidates := []string{strings.TrimSpace(requested), "tr", "en"}
	seen := map[string]bool{}
	for _, loc := range candidates {
		if loc == "" || seen[loc] {
			continue
		}
		seen[loc] = true
		row, err := q.GetContractTemplateLocale(ctx, db.GetContractTemplateLocaleParams{TemplateID: templateID, Locale: loc})
		if err == nil {
			return row, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return db.ContractTemplateLocale{}, err
		}
	}
	return db.ContractTemplateLocale{}, ErrNotFound
}

func subscriptionContractValues(sub db.ServiceSubscription, item db.ServiceCatalogItem, org db.Organization) map[string]string {
	return map[string]string{
		"org_name":      org.Name,
		"org_phone":     org.Phone,
		"org_email":     org.Email,
		"org_address":   org.Address,
		"service_name":  item.Name,
		"start_date":    sub.StartsOn.Time.Format(time.DateOnly),
		"end_date":      sub.EndsOn.Time.Format(time.DateOnly),
		"price":         numText(sub.Price) + " " + sub.Currency,
		"plate":         "",
		"vin":           "",
		"vehicle_label": "",
	}
}

func userName(u db.User, fallback string) string {
	full := strings.TrimSpace(strings.TrimSpace(u.Name) + " " + strings.TrimSpace(u.Surname))
	if full == "" {
		return fallback
	}
	return full
}

func (s *Service) cancelView(_ context.Context, req db.ServiceSubscriptionCancelRequest, subUUID uuid.UUID) CancelRequestView {
	var note *string
	if req.DecisionNote.Valid {
		note = &req.DecisionNote.String
	}
	return CancelRequestView{
		UUID: req.Uuid, SubscriptionUUID: subUUID, Reason: req.Reason, Status: req.Status,
		CancellationFee: numText(req.CancellationFee), Currency: req.Currency, DecisionNote: note,
		CreatedAt: req.CreatedAt,
	}
}

func dateArg(t time.Time) pgtype.Date {
	y, m, d := t.Date()
	return pgtype.Date{Time: time.Date(y, m, d, 0, 0, 0, 0, time.UTC), Valid: true}
}

func nullableTextArg(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
