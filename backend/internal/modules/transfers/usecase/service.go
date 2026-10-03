// Package usecase holds the stock transfer request rules behind
// /v1/stock-transfers (TEC-197, F1-04f2, K13): a dealer (or distributor)
// hands units over to a sibling under the same parent. The giver requests
// with the units' barcodes, the receiver or the common parent approves or
// rejects, the giver ships (transfer_out), the receiver receives
// (transfer_in); a cancel after shipping restores the units
// (transfer_cancel_restore). Every stock change goes through ledger.Post
// with an idempotency key per request item; every transition writes a
// transfers.* outbox event in the same transaction.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Errors returned by the service; the handler maps them to HTTP codes.
var (
	ErrNotFound  = errors.New("transfers: request not found")
	ErrForbidden = errors.New("transfers: not allowed for this organization")
	// ErrNotSibling: the target is not a sibling of the active organization
	// (another parent, brand or organization type) or the active
	// organization cannot transfer at all (the center, K13).
	ErrNotSibling = errors.New("transfers: target is not a sibling organization")
	// ErrNotParent: a return names another organization than the active
	// organization's direct parent, or the active organization has no
	// parent to return to (TEC-223).
	ErrNotParent = errors.New("transfers: a return goes to the direct parent only")
	// ErrInvalidTransition: the request's status does not allow the move.
	ErrInvalidTransition = errors.New("transfers: invalid status transition")
	// ErrStockUnavailable: a unit left the giver's stock or the ledger
	// refused a movement.
	ErrStockUnavailable = errors.New("transfers: stock movement refused")
)

// ValidationError is a field-level input error.
type ValidationError struct {
	Field   string
	Code    string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// Validation codes of a request line.
const (
	CodeUnitNotFound      = "TRANSFER_UNIT_NOT_FOUND"
	CodeUnitNotAvailable  = "TRANSFER_UNIT_NOT_AVAILABLE"
	CodeUnitReserved      = "TRANSFER_UNIT_RESERVED"
	CodeInsufficientStock = "TRANSFER_INSUFFICIENT_STOCK"
)

// Organization types (organizations.type).
const (
	OrgDistributor = "distributor"
	OrgDealer      = "dealer"
)

// MaxItems bounds the units of one request; MaxQuantity a fixed barcode line.
const (
	MaxItems    = 200
	MaxQuantity = 1_000_000
)

// Ledger reference of a transfer: one movement per request item.
const (
	LedgerSource  = "transfer"
	LedgerRefType = "transfer_item"
)

// TxBeginner starts a transaction (*pgxpool.Pool).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Caller is the request principal in its active organization.
type Caller struct {
	Principal authctx.Principal
	Org       orgctx.Scope
}

func (c Caller) actor() pgtype.Int8 {
	if c.Principal.UserInternal == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: c.Principal.UserInternal, Valid: true}
}

func (c Caller) actorPtr() *int64 {
	if c.Principal.UserInternal == 0 {
		return nil
	}
	v := c.Principal.UserInternal
	return &v
}

// can reports whether the caller holds slug for its own organization.
func (c Caller) can(slug string) bool { return c.Principal.Can(slug, rbac.ScopeManaged) }

// Service implements the transfer request use cases.
type Service struct {
	pool   TxBeginner
	q      *db.Queries
	out    outbox.Enqueuer
	ledger *ledger.Ledger
	// accounting books received transfers (TEC-200, K13); nil: none.
	accounting AccountingPoster
	now        func() time.Time
}

// New creates the service.
func New(pool TxBeginner, q *db.Queries, out outbox.Enqueuer) *Service {
	return &Service{pool: pool, q: q, out: out, ledger: ledger.New(q, out), now: time.Now}
}

func (s *Service) inTx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("transfers: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("transfers: commit: %w", err)
	}
	return nil
}

func textOrNull(p *string) pgtype.Text {
	if p == nil {
		return pgtype.Text{}
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

// --- Parties -------------------------------------------------------------------

func partyOf(c Caller, r db.StockTransferRequest) Party {
	// The parent is checked before the receiver: on a return (TEC-223) the
	// receiver is the parent; on a sibling transfer they always differ.
	switch c.Org.InternalID {
	case r.FromOrgID:
		return PartySender
	case r.ApproverOrgID:
		return PartyParent
	case r.ToOrgID:
		return PartyReceiver
	}
	return ""
}

func visible(c Caller, r db.StockTransferRequest) bool {
	return r.BrandID == c.Org.BrandID && partyOf(c, r) != ""
}

// sibling returns the target and the common parent when target is a
// sibling of from: same brand, same type (dealer or distributor), same
// parent (K13: no transfer across parents or brands).
func sibling(from, target db.Organization) (int64, bool) {
	if from.Type != OrgDealer && from.Type != OrgDistributor {
		return 0, false
	}
	if !from.ParentID.Valid || !target.ParentID.Valid || from.ID == target.ID {
		return 0, false
	}
	if target.BrandID != from.BrandID || target.Type != from.Type || target.ParentID.Int64 != from.ParentID.Int64 {
		return 0, false
	}
	return from.ParentID.Int64, true
}

// returnParent returns the organization a return of from goes to: its
// direct parent in the same brand (TEC-223); ok is false when from cannot
// return (the center, or no parent).
func returnParent(ctx context.Context, q *db.Queries, from db.Organization) (db.Organization, bool, error) {
	if (from.Type != OrgDealer && from.Type != OrgDistributor) || !from.ParentID.Valid {
		return db.Organization{}, false, nil
	}
	parent, err := q.GetOrganizationByID(ctx, from.ParentID.Int64)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, false, nil
	}
	if err != nil {
		return db.Organization{}, false, fmt.Errorf("transfers: parent: %w", err)
	}
	if parent.BrandID != from.BrandID {
		return db.Organization{}, false, nil
	}
	return parent, true, nil
}

// Targets lists the organizations the active organization may transfer to:
// its siblings, or for kind return its direct parent (TEC-223).
func (s *Service) Targets(ctx context.Context, c Caller, kind string) ([]OrgRef, error) {
	if c.Org.InternalID == 0 || !c.can(rbac.PermTransfersRequest) {
		return nil, ErrForbidden
	}
	kind = strings.TrimSpace(kind)
	if kind != "" && !IsKind(kind) {
		return nil, invalid("kind", "must be sibling or return")
	}
	from, err := s.q.GetOrganizationByID(ctx, c.Org.InternalID)
	if err != nil {
		return nil, fmt.Errorf("transfers: organization: %w", err)
	}
	out := []OrgRef{}
	if kind == KindReturn {
		parent, ok, err := returnParent(ctx, s.q, from)
		if err != nil || !ok {
			return out, err
		}
		return append(out, OrgRef{UUID: parent.Uuid, Name: parent.Name, Type: parent.Type}), nil
	}
	if (from.Type != OrgDealer && from.Type != OrgDistributor) || !from.ParentID.Valid {
		return out, nil
	}
	rows, err := s.q.ListTransferSiblings(ctx, db.ListTransferSiblingsParams{
		BrandID: from.BrandID, ParentID: from.ParentID, Type: from.Type, OrgID: from.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("transfers: siblings: %w", err)
	}
	for _, o := range rows {
		out = append(out, OrgRef{UUID: o.Uuid, Name: o.Name, Type: o.Type})
	}
	return out, nil
}

// --- Create ---------------------------------------------------------------------

// ItemInput names a unit by barcode; Quantity is required for fixed
// barcodes (pieces and rolls move whole).
type ItemInput struct {
	Barcode  string
	Quantity *int64
}

// CreateInput is a new request of the active organization (the giver).
// Kind is sibling (default) or return; a return goes to the direct parent
// and ToOrgUUID, when set, must name it.
type CreateInput struct {
	Kind      string
	ToOrgUUID string
	Note      *string
	Items     []ItemInput
}

// Create opens a transfer request from the active organization to a sibling.
func (s *Service) Create(ctx context.Context, c Caller, in CreateInput) (TransferView, error) {
	if c.Org.InternalID == 0 || !c.can(rbac.PermTransfersRequest) {
		return TransferView{}, ErrForbidden
	}
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = KindSibling
	}
	if !IsKind(kind) {
		return TransferView{}, invalid("kind", "must be sibling or return")
	}
	var toID uuid.UUID
	if raw := strings.TrimSpace(in.ToOrgUUID); raw != "" || kind == KindSibling {
		var err error
		if toID, err = uuid.Parse(raw); err != nil {
			return TransferView{}, invalid("to_org_uuid", "must be a UUID")
		}
	}
	if len(in.Items) == 0 {
		return TransferView{}, invalid("items", "at least one unit is required")
	}
	if len(in.Items) > MaxItems {
		return TransferView{}, invalid("items", fmt.Sprintf("at most %d units", MaxItems))
	}
	var created db.StockTransferRequest
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		from, err := q.GetOrganizationByID(ctx, c.Org.InternalID)
		if err != nil {
			return fmt.Errorf("transfers: organization: %w", err)
		}
		target, parent, err := s.resolveTarget(ctx, q, kind, from, toID)
		if err != nil {
			return err
		}
		brand, err := q.GetBrandByID(ctx, from.BrandID)
		if err != nil {
			return fmt.Errorf("transfers: brand: %w", err)
		}
		r, err := q.InsertTransferRequest(ctx, db.InsertTransferRequestParams{
			FromOrgID: from.ID, BrandID: from.BrandID, ToOrgID: target.ID, ApproverOrgID: parent,
			Currency: strings.TrimSpace(brand.Currency), Reason: textOrNull(in.Note), RequestedByUserID: c.actor(),
			Kind: kind,
		})
		if err != nil {
			return fmt.Errorf("transfers: create: %w", err)
		}
		seen := map[int64]bool{}
		for i, it := range in.Items {
			field := fmt.Sprintf("items[%d]", i)
			u, qty, meters, err := s.checkLine(ctx, q, r, field, it)
			if err != nil {
				return err
			}
			if seen[u.ID] {
				return invalid(field+".barcode", "each unit may appear once")
			}
			seen[u.ID] = true
			if _, err := q.InsertTransferRequestItem(ctx, db.InsertTransferRequestItemParams{
				RequestID: r.ID, OrganizationID: r.OrganizationID, BrandID: r.BrandID,
				UnitID: u.ID, ProductID: u.ProductID, Quantity: qty, Meters: meters,
			}); err != nil {
				return fmt.Errorf("transfers: create item: %w", err)
			}
		}
		created = r
		return s.emit(ctx, tx, events.TransfersRequested, r, "", c)
	})
	if err != nil {
		return TransferView{}, err
	}
	return s.view(ctx, s.q, c, created)
}

// resolveTarget returns the receiver and the approver of a new request: a
// sibling and the common parent (K13), or for a return the direct parent
// twice (TEC-223). toID is uuid.Nil for a return without an explicit target.
func (s *Service) resolveTarget(ctx context.Context, q *db.Queries, kind string, from db.Organization,
	toID uuid.UUID) (db.Organization, int64, error) {
	if kind == KindReturn {
		parent, ok, err := returnParent(ctx, q, from)
		if err != nil {
			return db.Organization{}, 0, err
		}
		if !ok || (toID != uuid.Nil && toID != parent.Uuid) {
			return db.Organization{}, 0, ErrNotParent
		}
		return parent, parent.ID, nil
	}
	target, err := q.GetOrganizationByUUID(ctx, toID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, 0, ErrNotSibling
	}
	if err != nil {
		return db.Organization{}, 0, fmt.Errorf("transfers: target: %w", err)
	}
	parent, ok := sibling(from, target)
	if !ok {
		return db.Organization{}, 0, ErrNotSibling
	}
	return target, parent, nil
}

// --- Transitions -------------------------------------------------------------------

// TransitionInput moves a request to Status; Reason is the decision note
// (approve/reject) or the cancel reason.
type TransitionInput struct {
	Status string
	Reason *string
}

func (s *Service) lockVisible(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID) (db.StockTransferRequest, error) {
	r, err := q.LockTransferRequestByUUID(ctx, db.LockTransferRequestByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.StockTransferRequest{}, ErrNotFound
	}
	if err != nil {
		return db.StockTransferRequest{}, fmt.Errorf("transfers: lock: %w", err)
	}
	if !visible(c, r) {
		return db.StockTransferRequest{}, ErrNotFound
	}
	return r, nil
}

// Transition moves a request along the state machine. A repeated request
// for the current status is a no-op (a second ship writes no movement).
func (s *Service) Transition(ctx context.Context, c Caller, id uuid.UUID, in TransitionInput) (TransferView, error) {
	to := strings.TrimSpace(in.Status)
	if !IsStatus(to) {
		return TransferView{}, invalid("status", "unknown transfer status")
	}
	var result db.StockTransferRequest
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		r, err := s.lockVisible(ctx, q, c, id)
		if err != nil {
			return err
		}
		result = r
		if r.Status == to {
			return nil
		}
		gs, ok := lookupKindTransition(r.Kind, r.Status, to)
		if !ok {
			return ErrInvalidTransition
		}
		if !allowed(gs, partyOf(c, r), c.can) {
			return ErrForbidden
		}
		from := r.Status
		reason := textOrNull(in.Reason)
		switch to {
		case StatusApproved:
			var total pgtype.Numeric
			if total, err = s.freezePrices(ctx, q, r); err == nil {
				r, err = q.DecideTransferRequest(ctx, db.DecideTransferRequestParams{
					ID: r.ID, Status: to, Total: total, ActorUserID: c.actor(), Note: reason,
				})
			}
		case StatusRejected:
			// A rejection never touches the stock.
			r, err = q.DecideTransferRequest(ctx, db.DecideTransferRequestParams{
				ID: r.ID, Status: to, ActorUserID: c.actor(), Note: reason,
			})
		case StatusShipped:
			if err = s.ship(ctx, q, tx, c, r); err == nil {
				r, err = q.ShipTransferRequest(ctx, db.ShipTransferRequestParams{ID: r.ID, ActorUserID: c.actor()})
			}
		case StatusReceived:
			if err = s.receive(ctx, q, tx, c, r); err == nil {
				r, err = q.ReceiveTransferRequest(ctx, db.ReceiveTransferRequestParams{ID: r.ID, ActorUserID: c.actor()})
			}
			if err == nil {
				// K13: A alacak / B borç (a return: the reversal of the
				// parent's sale), same transaction as transfer_in.
				err = s.book(ctx, q, tx, c, r)
			}
		case StatusCancelled:
			if from == StatusShipped {
				// The goods are back: one restore per shipped unit, and
				// any accounting row of the request is reversed.
				if err = s.restore(ctx, q, tx, c, r); err == nil {
					_, err = s.VoidAccountingTx(ctx, tx, r, "transfer cancelled after shipping", c.actorPtr())
				}
			}
			if err == nil {
				r, err = q.CancelTransferRequest(ctx, db.CancelTransferRequestParams{
					ID: r.ID, ActorUserID: c.actor(), Reason: reason,
				})
			}
		default:
			return ErrInvalidTransition
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidTransition
		}
		if err != nil {
			return err
		}
		result = r
		return s.emit(ctx, tx, eventFor(to), r, from, c)
	})
	if err != nil {
		return TransferView{}, err
	}
	return s.view(ctx, s.q, c, result)
}

func (s *Service) emit(ctx context.Context, tx pgx.Tx, name string, r db.StockTransferRequest, from string, c Caller) error {
	id, uid := r.ID, r.Uuid
	payload := map[string]any{
		"transfer_uuid":   r.Uuid.String(),
		"transfer_no":     r.TransferNo,
		"status":          r.Status,
		"from_org_id":     r.FromOrgID,
		"to_org_id":       r.ToOrgID,
		"approver_org_id": r.ApproverOrgID,
		"brand_id":        r.BrandID,
		"kind":            r.Kind,
	}
	if from != "" && from != r.Status {
		payload["from_status"] = from
	}
	// TEC-200: the notification bus reads the recipients and the names
	// from the payload (resolved here, in the transition's transaction).
	q := s.q.WithTx(tx)
	for key, id := range map[string]int64{"from_org_name": r.FromOrgID, "to_org_name": r.ToOrgID} {
		o, err := q.GetOrganizationByID(ctx, id)
		if err != nil {
			return fmt.Errorf("transfers: organization %d: %w", id, err)
		}
		payload[key] = o.Name
	}
	ids, err := s.notifyUserIDs(ctx, q, name, r, c)
	if err != nil {
		return err
	}
	payload["notify_user_ids"] = ids
	ev := events.New(name).WithTenant(r.OrganizationID).WithEntity("stock_transfer_request", &id, &uid).WithPayload(payload)
	if c.Principal.UserInternal != 0 {
		ev = ev.WithActor(c.Principal.UserInternal)
	}
	if err := s.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("transfers: outbox: %w", err)
	}
	return nil
}

// notifyParties lists the sides told about a transfer event (TEC-200):
// a new request goes to the deciders (receiver, common parent), a decision
// to the giver and the receiver, a shipment to the receiver, a receipt to
// the giver, a cancel to every side. On a return (TEC-223) the receiver is
// the parent: the giver's steps go to the parent, the parent's steps go to
// the giver, a cancel to both.
func notifyParties(kind, event string) []Party {
	if kind == KindReturn {
		switch event {
		case events.TransfersRequested, events.TransfersShipped:
			return []Party{PartyParent}
		case events.TransfersApproved, events.TransfersRejected, events.TransfersReceived:
			return []Party{PartySender}
		case events.TransfersCancelled:
			return []Party{PartySender, PartyParent}
		}
		return nil
	}
	switch event {
	case events.TransfersRequested:
		return []Party{PartyReceiver, PartyParent}
	case events.TransfersApproved, events.TransfersRejected:
		return []Party{PartySender, PartyReceiver}
	case events.TransfersShipped:
		return []Party{PartyReceiver}
	case events.TransfersReceived:
		return []Party{PartySender}
	case events.TransfersCancelled:
		return []Party{PartySender, PartyReceiver, PartyParent}
	}
	return nil
}

// notifyUserIDs resolves the recipients of a transfer event: members of each
// notified side holding that side's transfer permission (transfers.request
// for the giver and the receiver, transfers.approve for the parent), minus
// the actor.
func (s *Service) notifyUserIDs(ctx context.Context, q *db.Queries, event string, r db.StockTransferRequest, c Caller) ([]int64, error) {
	out := []int64{}
	seen := map[int64]bool{c.Principal.UserInternal: true}
	for _, p := range notifyParties(r.Kind, event) {
		org, perm := r.FromOrgID, rbac.PermTransfersRequest
		switch p {
		case PartyReceiver:
			org = r.ToOrgID
		case PartyParent:
			org, perm = r.ApproverOrgID, rbac.PermTransfersApprove
		}
		ids, err := q.ListTransferNotifyUserIDs(ctx, db.ListTransferNotifyUserIDsParams{OrganizationID: org, PermissionSlug: perm})
		if err != nil {
			return nil, fmt.Errorf("transfers: recipients: %w", err)
		}
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out, nil
}
