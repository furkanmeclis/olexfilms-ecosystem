package usecase

// Reclassification (TEC-157, F1-02e): a unit labelled with the wrong
// product moves to the right one while its barcode stays the same.
//
//   - Request: a user with stock.write in the organization that holds the
//     unit on hand names the unit (barcode or uuid), the target product and
//     a reason. The routing is checked at once (ledger.CheckReclassification
//     plus open services / order reservations) and a pending
//     stock_reclassifications row is saved; at most one pending request per
//     unit.
//   - Approval = application: a user with stock.reclassify (center, step-up
//     on the route) whose scope reaches the request, and who is not the
//     requester, approves it. In one transaction the request row is locked,
//     the routing is checked again under the unit lock, ledger.Reclassify
//     writes the reclassification movement and moves the projections, the
//     request becomes approved with its movement and an audit row is
//     written. Approving an approved request again writes nothing.
//   - Reject (stock.reclassify) and cancel (the requesting organization)
//     close a pending request without a movement.
//
// The ledger refuses a reclassification movement through Post, so a unit
// changes product only through an approved request.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Audit actions and resource of the reclassification flow.
const (
	AuditResourceReclassification = "stock.reclassification"
	AuditReclassRequested         = "stock.reclassification.requested"
	AuditReclassApproved          = "stock.reclassification.approved"
	AuditReclassRejected          = "stock.reclassification.rejected"
	AuditReclassCancelled         = "stock.reclassification.cancelled"
)

// maxReasonLen bounds the free-text reason and decision note.
const maxReasonLen = 2000

// Reclassification flow errors (the ledger.ErrReclassify* refusals pass
// through unchanged).
var (
	// ErrReclassNotPending: the request is already decided.
	ErrReclassNotPending = errors.New("stock: reclassification is not pending")
	// ErrReclassPendingExists: the unit already has a pending request.
	ErrReclassPendingExists = errors.New("stock: unit already has a pending reclassification")
	// ErrReclassSelfApproval: the requester cannot approve its own request.
	ErrReclassSelfApproval = errors.New("stock: requester cannot approve the reclassification")
	// ErrReclassUnitInUse: the unit is on an open service or reserved for
	// an order line of its current product.
	ErrReclassUnitInUse = errors.New("stock: unit is in use")
)

// ValidationError is a field-level input error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// TxBeginner opens a transaction (*pgxpool.Pool).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Reclassifications implements the reclassification use cases.
type Reclassifications struct {
	pool   TxBeginner
	q      *db.Queries
	ledger *ledger.Ledger
}

// NewReclassifications builds the use case; stock.* events go to out.
func NewReclassifications(pool TxBeginner, q *db.Queries, out outbox.Enqueuer) *Reclassifications {
	return &Reclassifications{pool: pool, q: q, ledger: ledger.New(q, out)}
}

// Caller is the request principal in its active organization; Filter is the
// resolved scope of the route permission.
type Caller struct {
	Principal authctx.Principal
	Org       orgctx.Scope
	Filter    scopefilter.Filter
	// IP and UserAgent go to the audit row.
	IP        *netip.Addr
	UserAgent string
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
	id := c.Principal.UserInternal
	return &id
}

// RequestInput is a new reclassification request. One of Barcode and
// UnitUUID names the unit.
type RequestInput struct {
	Barcode       string
	UnitUUID      string
	ToProductUUID string
	Reason        string
}

// ListFilter filters the request list.
type ListFilter struct {
	Status string
	Limit  int32
	Offset int32
}

func (s *Reclassifications) inTx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("stock: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("stock: commit: %w", err)
	}
	return nil
}

// Request saves a pending reclassification of a unit held on hand by the
// caller's organization.
func (s *Reclassifications) Request(ctx context.Context, c Caller, in RequestInput) (model.Reclassification, error) {
	reason := strings.TrimSpace(in.Reason)
	switch {
	case reason == "":
		return model.Reclassification{}, invalid("reason", "reason is required")
	case len(reason) > maxReasonLen:
		return model.Reclassification{}, invalid("reason", "reason is too long")
	}
	toUUID, err := uuid.Parse(strings.TrimSpace(in.ToProductUUID))
	if err != nil {
		return model.Reclassification{}, invalid("to_product_uuid", "to_product_uuid is invalid")
	}
	unitID, err := s.findUnit(ctx, c, in)
	if err != nil {
		return model.Reclassification{}, err
	}
	if err := checkWriteReach(ctx, s.q, c, unitID); err != nil {
		return model.Reclassification{}, err
	}

	var row db.StockReclassification
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		unit, err := q.LockUnit(ctx, unitID)
		if err != nil {
			return fmt.Errorf("stock: lock unit: %w", err)
		}
		from, err := q.GetProduct(ctx, db.GetProductParams{ID: unit.ProductID, BrandID: unit.BrandID})
		if err != nil {
			return fmt.Errorf("stock: unit product: %w", err)
		}
		to, err := q.GetProductByUUIDAnyBrand(ctx, toUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid("to_product_uuid", "product not found")
		}
		if err != nil {
			return fmt.Errorf("stock: target product: %w", err)
		}
		st, err := currentState(ctx, q, unit.ID)
		if err != nil {
			return err
		}
		if err := ledger.CheckReclassification(unit, st, from, to, c.Org.InternalID); err != nil {
			return err
		}
		if err := checkNotInUse(ctx, q, unit.ID); err != nil {
			return err
		}
		row, err = q.CreateStockReclassification(ctx, db.CreateStockReclassificationParams{
			OrganizationID: c.Org.InternalID, BrandID: unit.BrandID, UnitID: unit.ID,
			FromProductID: from.ID, ToProductID: to.ID, Reason: reason, RequestedByUserID: c.actor(),
		})
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrReclassPendingExists
		}
		if err != nil {
			return fmt.Errorf("stock: create reclassification: %w", err)
		}
		return audit(ctx, q, c, AuditReclassRequested, row, map[string]any{
			"barcode": unit.Barcode, "from_product_id": from.ID, "to_product_id": to.ID, "reason": reason,
		})
	})
	if err != nil {
		return model.Reclassification{}, err
	}
	return s.view(ctx, row)
}

// findUnit resolves the unit of a request: by uuid, or by barcode (the unit
// held by the caller's organization first, then the active brand's).
func (s *Reclassifications) findUnit(ctx context.Context, c Caller, in RequestInput) (int64, error) {
	if raw := strings.TrimSpace(in.UnitUUID); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return 0, invalid("unit_uuid", "unit_uuid is invalid")
		}
		u, err := s.q.GetUnitByUUID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		if err != nil {
			return 0, fmt.Errorf("stock: unit: %w", err)
		}
		return u.ID, nil
	}
	barcode := strings.TrimSpace(in.Barcode)
	if barcode == "" {
		return 0, invalid("barcode", "barcode or unit_uuid is required")
	}
	units, err := s.q.ListUnitsByBarcode(ctx, barcode)
	if err != nil {
		return 0, fmt.Errorf("stock: units: %w", err)
	}
	var pick int64
	for _, u := range units {
		st, err := currentState(ctx, s.q, u.ID)
		if err != nil {
			return 0, err
		}
		if st != nil && st.HolderOrgID == c.Org.InternalID {
			return u.ID, nil
		}
		if pick == 0 && u.BrandID == c.Org.BrandID {
			pick = u.ID
		}
	}
	if pick == 0 {
		return 0, ErrNotFound
	}
	return pick, nil
}

func currentState(ctx context.Context, q *db.Queries, unitID int64) (*db.UnitCurrentState, error) {
	st, err := q.GetUnitCurrentState(ctx, unitID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stock: unit state: %w", err)
	}
	return &st, nil
}

// checkNotInUse refuses a unit on an open service or with an active order
// reservation: both name the unit's current product.
func checkNotInUse(ctx context.Context, q *db.Queries, unitID int64) error {
	open, err := q.ListOpenServicesByUnit(ctx, db.ListOpenServicesByUnitParams{UnitID: unitID})
	if err != nil {
		return fmt.Errorf("stock: open services: %w", err)
	}
	if len(open) > 0 {
		return fmt.Errorf("%w: open service %s", ErrReclassUnitInUse, open[0].ServiceNo)
	}
	res, err := q.LockActiveReservationsByUnit(ctx, unitID)
	if err != nil {
		return fmt.Errorf("stock: reservations: %w", err)
	}
	if len(res) > 0 {
		return fmt.Errorf("%w: order reservation", ErrReclassUnitInUse)
	}
	return nil
}

// lockScoped locks a request inside the caller's scope (else not found).
func lockScoped(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID) (db.StockReclassification, error) {
	row, err := q.LockStockReclassificationByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !c.Filter.AllowsOrg(row.OrganizationID, row.BrandID)) {
		return db.StockReclassification{}, ErrNotFound
	}
	if err != nil {
		return db.StockReclassification{}, fmt.Errorf("stock: lock reclassification: %w", err)
	}
	return row, nil
}

// Approve approves and applies a pending request (the route requires
// stock.reclassify and a step-up). Replayed is true when the request was
// already approved: nothing is written.
func (s *Reclassifications) Approve(ctx context.Context, c Caller, id uuid.UUID, note string) (model.Reclassification, bool, error) {
	note = strings.TrimSpace(note)
	if len(note) > maxReasonLen {
		return model.Reclassification{}, false, invalid("note", "note is too long")
	}
	var row db.StockReclassification
	replayed := false
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		var err error
		if row, err = lockScoped(ctx, q, c, id); err != nil {
			return err
		}
		switch row.Status {
		case model.ReclassApproved:
			replayed = true
			return nil
		case model.ReclassPending:
		default:
			return ErrReclassNotPending
		}
		if row.RequestedByUserID.Valid && row.RequestedByUserID.Int64 == c.Principal.UserInternal {
			return ErrReclassSelfApproval
		}
		if err := checkNotInUse(ctx, q, row.UnitID); err != nil {
			return err
		}
		res, err := s.ledger.Reclassify(ctx, tx, ledger.Reclassification{
			RequestID: row.ID, UnitID: row.UnitID, FromProductID: row.FromProductID,
			ToProductID: row.ToProductID, HolderOrgID: row.OrganizationID,
			ActorUserID: c.actorPtr(), Reason: row.Reason,
			Metadata: map[string]any{"reclassification_uuid": row.Uuid.String()},
		})
		if err != nil {
			return err
		}
		mvID := res.Movement.ID
		row, err = q.DecideStockReclassification(ctx, db.DecideStockReclassificationParams{
			ID: row.ID, Status: model.ReclassApproved, DecidedByUserID: c.actor(),
			DecisionNote: pgText(note), MovementID: pgtype.Int8{Int64: mvID, Valid: true},
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrReclassNotPending
		}
		if err != nil {
			return fmt.Errorf("stock: approve reclassification: %w", err)
		}
		return audit(ctx, q, c, AuditReclassApproved, row, map[string]any{
			"movement_id": mvID, "from_product_id": row.FromProductID, "to_product_id": row.ToProductID,
			"replayed_movement": res.Replayed,
		})
	})
	if err != nil {
		return model.Reclassification{}, false, err
	}
	out, err := s.view(ctx, row)
	return out, replayed, err
}

// Reject closes a pending request without a movement (stock.reclassify).
func (s *Reclassifications) Reject(ctx context.Context, c Caller, id uuid.UUID, note string) (model.Reclassification, error) {
	return s.close(ctx, c, id, note, model.ReclassRejected, AuditReclassRejected, false)
}

// Cancel withdraws a pending request of the caller's organization.
func (s *Reclassifications) Cancel(ctx context.Context, c Caller, id uuid.UUID, note string) (model.Reclassification, error) {
	return s.close(ctx, c, id, note, model.ReclassCancelled, AuditReclassCancelled, true)
}

func (s *Reclassifications) close(ctx context.Context, c Caller, id uuid.UUID, note, status, action string, ownOrg bool) (model.Reclassification, error) {
	note = strings.TrimSpace(note)
	if len(note) > maxReasonLen {
		return model.Reclassification{}, invalid("note", "note is too long")
	}
	var row db.StockReclassification
	err := s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		var err error
		if row, err = lockScoped(ctx, q, c, id); err != nil {
			return err
		}
		if ownOrg && row.OrganizationID != c.Org.InternalID {
			return ErrNotFound
		}
		switch row.Status {
		case status:
			return nil // repeated decision: no-op
		case model.ReclassPending:
		default:
			return ErrReclassNotPending
		}
		row, err = q.DecideStockReclassification(ctx, db.DecideStockReclassificationParams{
			ID: row.ID, Status: status, DecidedByUserID: c.actor(), DecisionNote: pgText(note),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrReclassNotPending
		}
		if err != nil {
			return fmt.Errorf("stock: decide reclassification: %w", err)
		}
		return audit(ctx, q, c, action, row, map[string]any{"note": note})
	})
	if err != nil {
		return model.Reclassification{}, err
	}
	return s.view(ctx, row)
}

// Get returns a request inside the caller's stock.read scope.
func (s *Reclassifications) Get(ctx context.Context, c Caller, id uuid.UUID) (model.Reclassification, error) {
	row, err := s.q.GetStockReclassificationByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !c.Filter.AllowsOrg(row.OrganizationID, row.BrandID)) {
		return model.Reclassification{}, ErrNotFound
	}
	if err != nil {
		return model.Reclassification{}, fmt.Errorf("stock: reclassification: %w", err)
	}
	return s.view(ctx, row)
}

// List pages the requests inside the caller's stock.read scope, newest
// first.
func (s *Reclassifications) List(ctx context.Context, c Caller, f ListFilter) ([]model.Reclassification, int64, error) {
	var status pgtype.Text
	switch f.Status {
	case "":
	case model.ReclassPending, model.ReclassApproved, model.ReclassRejected, model.ReclassCancelled:
		status = pgText(f.Status)
	default:
		return nil, 0, invalid("status", "status must be pending, approved, rejected or cancelled")
	}
	brand, orgs := c.Filter.BrandIDArg(), c.Filter.OrgIDsArg()
	rows, err := s.q.ListStockReclassificationsScoped(ctx, db.ListStockReclassificationsScopedParams{
		BrandID: brand, OrgIds: orgs, Status: status, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stock: reclassifications: %w", err)
	}
	total, err := s.q.CountStockReclassificationsScoped(ctx, db.CountStockReclassificationsScopedParams{
		BrandID: brand, OrgIds: orgs, Status: status,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stock: count reclassifications: %w", err)
	}
	out := make([]model.Reclassification, 0, len(rows))
	for _, r := range rows {
		v, err := s.view(ctx, r)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, nil
}

// audit writes the activity row in the request's transaction.
func audit(ctx context.Context, q *db.Queries, c Caller, action string, row db.StockReclassification, extra map[string]any) error {
	payload := map[string]any{
		"reclassification_id": row.ID, "unit_id": row.UnitID, "organization_id": row.OrganizationID,
		"brand_id": row.BrandID, "status": row.Status,
	}
	for k, v := range extra {
		payload[k] = v
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("stock: audit payload: %w", err)
	}
	if _, err := q.InsertActivityEvent(ctx, db.InsertActivityEventParams{
		ActorUserID: c.actor(), Action: action, Resource: AuditResourceReclassification,
		ResourceUuid: pgtype.UUID{Bytes: row.Uuid, Valid: true}, Payload: body,
		IpAddress: c.IP, UserAgent: pgText(c.UserAgent),
	}); err != nil {
		return fmt.Errorf("stock: audit: %w", err)
	}
	return nil
}

// view renders a request with its unit, organization, products and users.
func (s *Reclassifications) view(ctx context.Context, row db.StockReclassification) (model.Reclassification, error) {
	unit, err := s.q.GetUnit(ctx, row.UnitID)
	if err != nil {
		return model.Reclassification{}, fmt.Errorf("stock: unit: %w", err)
	}
	org, err := s.q.GetOrganizationByID(ctx, row.OrganizationID)
	if err != nil {
		return model.Reclassification{}, fmt.Errorf("stock: organization: %w", err)
	}
	product := func(id int64) (model.ProductRef, error) {
		p, err := s.q.GetProduct(ctx, db.GetProductParams{ID: id, BrandID: row.BrandID})
		if err != nil {
			return model.ProductRef{}, fmt.Errorf("stock: product: %w", err)
		}
		return model.ProductRef{UUID: p.Uuid, SKU: p.Sku, Name: p.Name, UnitType: p.UnitType, UsesFixedBarcode: p.UsesFixedBarcode}, nil
	}
	out := model.Reclassification{
		UUID: row.Uuid, Status: row.Status, Reason: row.Reason,
		Unit:         model.UnitRef{UUID: unit.Uuid, Barcode: unit.Barcode},
		Organization: model.OrgRef{UUID: org.Uuid, Name: org.Name, Type: org.Type},
		DecisionNote: textPtr(row.DecisionNote), CreatedAt: row.CreatedAt.Time,
	}
	if out.FromProduct, err = product(row.FromProductID); err != nil {
		return model.Reclassification{}, err
	}
	if out.ToProduct, err = product(row.ToProductID); err != nil {
		return model.Reclassification{}, err
	}
	if row.DecidedAt.Valid {
		t := row.DecidedAt.Time.UTC().Truncate(time.Microsecond)
		out.DecidedAt = &t
	}
	if row.MovementID.Valid {
		mv, err := s.q.GetStockMovement(ctx, row.MovementID.Int64)
		if err != nil {
			return model.Reclassification{}, fmt.Errorf("stock: movement: %w", err)
		}
		out.MovementUUID = &mv.Uuid
	}
	var ids []int64
	for _, u := range []pgtype.Int8{row.RequestedByUserID, row.DecidedByUserID} {
		if u.Valid {
			ids = append(ids, u.Int64)
		}
	}
	if len(ids) > 0 {
		users, err := s.q.ListUsersByIDs(ctx, ids)
		if err != nil {
			return model.Reclassification{}, fmt.Errorf("stock: users: %w", err)
		}
		for _, u := range users {
			ref := &model.UserRef{UUID: u.Uuid, Name: strings.TrimSpace(u.Name + " " + u.Surname)}
			if row.RequestedByUserID.Valid && row.RequestedByUserID.Int64 == u.ID {
				out.RequestedBy = ref
			}
			if row.DecidedByUserID.Valid && row.DecidedByUserID.Int64 == u.ID {
				out.DecidedBy = ref
			}
		}
	}
	return out, nil
}

func pgText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
