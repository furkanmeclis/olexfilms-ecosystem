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
}

// New creates the service.
func New(pool TxBeginner, q *db.Queries, out outbox.Enqueuer) *Service {
	return &Service{pool: pool, q: q, out: out, ledger: ledger.New(q, out)}
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
	switch c.Org.InternalID {
	case r.FromOrgID:
		return PartySender
	case r.ToOrgID:
		return PartyReceiver
	case r.ApproverOrgID:
		return PartyParent
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

// Targets lists the siblings the active organization may transfer to.
func (s *Service) Targets(ctx context.Context, c Caller) ([]OrgRef, error) {
	if c.Org.InternalID == 0 || !c.can(rbac.PermTransfersRequest) {
		return nil, ErrForbidden
	}
	from, err := s.q.GetOrganizationByID(ctx, c.Org.InternalID)
	if err != nil {
		return nil, fmt.Errorf("transfers: organization: %w", err)
	}
	out := []OrgRef{}
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
type CreateInput struct {
	ToOrgUUID string
	Note      *string
	Items     []ItemInput
}

// Create opens a transfer request from the active organization to a sibling.
func (s *Service) Create(ctx context.Context, c Caller, in CreateInput) (TransferView, error) {
	if c.Org.InternalID == 0 || !c.can(rbac.PermTransfersRequest) {
		return TransferView{}, ErrForbidden
	}
	toID, err := uuid.Parse(strings.TrimSpace(in.ToOrgUUID))
	if err != nil {
		return TransferView{}, invalid("to_org_uuid", "must be a UUID")
	}
	if len(in.Items) == 0 {
		return TransferView{}, invalid("items", "at least one unit is required")
	}
	if len(in.Items) > MaxItems {
		return TransferView{}, invalid("items", fmt.Sprintf("at most %d units", MaxItems))
	}
	var created db.StockTransferRequest
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		from, err := q.GetOrganizationByID(ctx, c.Org.InternalID)
		if err != nil {
			return fmt.Errorf("transfers: organization: %w", err)
		}
		target, err := q.GetOrganizationByUUID(ctx, toID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotSibling
		}
		if err != nil {
			return fmt.Errorf("transfers: target: %w", err)
		}
		parent, ok := sibling(from, target)
		if !ok {
			return ErrNotSibling
		}
		brand, err := q.GetBrandByID(ctx, from.BrandID)
		if err != nil {
			return fmt.Errorf("transfers: brand: %w", err)
		}
		r, err := q.InsertTransferRequest(ctx, db.InsertTransferRequestParams{
			FromOrgID: from.ID, BrandID: from.BrandID, ToOrgID: target.ID, ApproverOrgID: parent,
			Currency: strings.TrimSpace(brand.Currency), Reason: textOrNull(in.Note), RequestedByUserID: c.actor(),
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
		gs, ok := lookupTransition(r.Status, to)
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
		case StatusCancelled:
			if from == StatusShipped {
				// The goods are back: one restore per shipped unit.
				err = s.restore(ctx, q, tx, c, r)
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
	}
	if from != "" && from != r.Status {
		payload["from_status"] = from
	}
	ev := events.New(name).WithTenant(r.OrganizationID).WithEntity("stock_transfer_request", &id, &uid).WithPayload(payload)
	if c.Principal.UserInternal != 0 {
		ev = ev.WithActor(c.Principal.UserInternal)
	}
	if err := s.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("transfers: outbox: %w", err)
	}
	return nil
}
