package usecase

// Moves inside and between the warehouses of one organization (TEC-205,
// F1-03e). Transfers between organizations are the transfers module
// (TEC-197/223); nothing here crosses organizations.
//
//   - Move (bin <-> bin): one step. Scanned unit barcodes go onto a scanned
//     (or picked) location of the same warehouse with one ledger placement
//     each. An unplaced unit (held by the organization itself, e.g. just
//     received from an order) may go onto any warehouse of the
//     organization. The idempotency reference is the unit's last movement
//     (warehouse_move:stock_movement:<last movement id>:placement:<barcode>),
//     so a retried request writes nothing twice.
//   - Warehouse transfer document: draft -> in_transit -> completed, or
//     cancelled (from draft or in_transit). Lines are serial units placed in
//     the source warehouse. Ship posts transfer_out (the unit goes in
//     transit, owned by the organization); complete posts the paired
//     transfer_in and a placement onto a location of the target warehouse;
//     cancel after shipping posts transfer_cancel_restore (back to the
//     source location). Keys: warehouse_transfer:warehouse_transfer_line:
//     <line id>:<type>:<barcode>. Every step is one transaction under the
//     document row lock; a repeated step finds the document in another
//     status and is refused.
//   - Unit lock: a unit on an open (draft/in_transit) warehouse transfer is
//     not picked by a move, another warehouse transfer, a stock transfer
//     request (TEC-197) or an order assignment; in transit the ledger
//     status refuses every other movement as well. Conversely a unit
//     reserved by an order or held by an open transfer request is not
//     added to a warehouse transfer.
//   - Order receipt placement (center -> distributor): orders ships and
//     receives the stock (order_out / received into the buyer
//     organization). PlaceOrder only shelves the received serial units of
//     the buyer onto one of its locations (warehouse_order:order_item_unit:
//     <assignment id>:placement:<barcode>); the order itself is untouched.
//
// Fixed barcode units move by quantity as out/in pairs and are not handled
// here yet: they are refused (moves, transfers) or skipped (order
// placement).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/labels"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Warehouse transfer and move errors.
var (
	ErrTransferNotFound     = errors.New("warehouse: transfer not found")
	ErrTransferLineNotFound = errors.New("warehouse: transfer line not found")
	// ErrTransferState: the step does not fit the transfer's status (a
	// second ship or complete lands here).
	ErrTransferState = errors.New("warehouse: transfer is not in the right status")
	// ErrTransferEmpty: ship without lines.
	ErrTransferEmpty = errors.New("warehouse: transfer has no lines")
	// ErrTransferUnplaced: complete while a line has no target location.
	ErrTransferUnplaced = errors.New("warehouse: every line needs a target location")
	// ErrUnitUnavailable: the unit is not on hand in the expected place of
	// the active organization.
	ErrUnitUnavailable = errors.New("warehouse: unit is not available here")
	// ErrUnitBusy: the unit is in transit, on another open warehouse
	// transfer, on an open stock transfer request or reserved by an order.
	ErrUnitBusy = errors.New("warehouse: unit is held by another operation")
	// ErrTransferLedger: the ledger refused a movement.
	ErrTransferLedger = errors.New("warehouse: ledger refused the movement")
	// ErrOrderNotFound: no order of the active brand bought by the active
	// organization.
	ErrOrderNotFound = errors.New("warehouse: order not found")
	// ErrOrderNotReceived: the order is not received yet.
	ErrOrderNotReceived = errors.New("warehouse: order is not received")
)

// Warehouse transfer statuses (warehouse_transfers.status).
const (
	TransferStatusDraft     = "draft"
	TransferStatusInTransit = "in_transit"
	TransferStatusCompleted = "completed"
	TransferStatusCancelled = "cancelled"
)

// Idempotency key parts.
const (
	transferSource   = "warehouse_transfer"
	transferRefType  = "warehouse_transfer_line"
	moveSource       = "warehouse_move"
	moveRefType      = "stock_movement"
	orderPlaceSource = "warehouse_order"
	orderPlaceRef    = "order_item_unit"
)

// Audit (activity_events).
const (
	AuditResourceTransfer  = "warehouse.transfer"
	AuditTransferCreated   = "warehouse.transfer.created"
	AuditTransferShipped   = "warehouse.transfer.shipped"
	AuditTransferCompleted = "warehouse.transfer.completed"
	AuditTransferCancelled = "warehouse.transfer.cancelled"
	AuditResourceMove      = "warehouse.move"
	AuditMoveDone          = "warehouse.move.placed"
	AuditOrderPlaced       = "warehouse.order.placed"
)

// MaxTransferLines caps the lines of one warehouse transfer.
const MaxTransferLines = 2000

// order statuses read here (orders.status).
const orderStatusReceived = "received"

// WarehouseTransfers implements moves, warehouse transfer documents and the
// order receipt placement.
type WarehouseTransfers struct {
	pool   TxBeginner
	q      *db.Queries
	ledger *ledger.Ledger
}

// NewWarehouseTransfers builds the use case; ledger events go to out.
func NewWarehouseTransfers(pool TxBeginner, q *db.Queries, out outbox.Enqueuer) *WarehouseTransfers {
	return &WarehouseTransfers{pool: pool, q: q, ledger: ledger.New(q, out)}
}

// --- views -----------------------------------------------------------------

// TransferLine is one unit of a warehouse transfer.
type TransferLine struct {
	UUID                  uuid.UUID      `json:"uuid"`
	UnitUUID              uuid.UUID      `json:"unit_uuid"`
	Barcode               string         `json:"barcode"`
	UnitStatus            string         `json:"unit_status"`
	Product               EntryProduct   `json:"product"`
	SourceLocation        *EntryLocation `json:"source_location"`
	TargetLocation        *EntryLocation `json:"target_location"`
	OutMovementUUID       *uuid.UUID     `json:"out_movement_uuid"`
	InMovementUUID        *uuid.UUID     `json:"in_movement_uuid"`
	PlacementMovementUUID *uuid.UUID     `json:"placement_movement_uuid"`
	RestoreMovementUUID   *uuid.UUID     `json:"restore_movement_uuid"`
}

// WarehouseTransfer is the transfer document.
type WarehouseTransfer struct {
	UUID          uuid.UUID      `json:"uuid"`
	TransferNo    string         `json:"transfer_no"`
	Status        string         `json:"status"`
	Note          *string        `json:"note"`
	FromWarehouse EntryWarehouse `json:"from_warehouse"`
	ToWarehouse   EntryWarehouse `json:"to_warehouse"`
	ToLocation    *EntryLocation `json:"to_location"`
	LineCount     int64          `json:"line_count"`
	Lines         []TransferLine `json:"lines,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	ShippedAt     *time.Time     `json:"shipped_at"`
	CompletedAt   *time.Time     `json:"completed_at"`
	CancelledAt   *time.Time     `json:"cancelled_at"`
}

// MovedUnit is one unit a move or an order placement handled.
type MovedUnit struct {
	UnitUUID     uuid.UUID      `json:"unit_uuid"`
	Barcode      string         `json:"barcode"`
	FromLocation *EntryLocation `json:"from_location"`
	MovementUUID *uuid.UUID     `json:"movement_uuid"`
	// Skipped names why an order unit was not placed (fixed, not_on_hand).
	Skipped string `json:"skipped,omitempty"`
}

// MoveResult is the outcome of a move or an order placement.
type MoveResult struct {
	Location EntryLocation `json:"location"`
	Units    []MovedUnit   `json:"units"`
}

// --- inputs ------------------------------------------------------------------

// MoveInput moves scanned units onto one location (LocationUUID or the
// scanned LocationCode, OFW:LOC:<full_code> or full_code).
type MoveInput struct {
	Barcodes     []string
	LocationUUID *uuid.UUID
	LocationCode string
}

// TransferInput opens a draft warehouse transfer.
type TransferInput struct {
	FromWarehouseUUID string
	ToWarehouseUUID   string
	ToLocationUUID    *uuid.UUID
	Note              *string
}

// CompleteInput optionally names the location for lines without a target.
type CompleteInput struct {
	LocationUUID *uuid.UUID
	LocationCode string
}

// --- move (bin <-> bin) ------------------------------------------------------

// Move places scanned serial units onto one location in one transaction.
func (s *WarehouseTransfers) Move(ctx context.Context, c EntryCaller, in MoveInput) (MoveResult, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return MoveResult{}, err
	}
	codes, err := normBarcodes(in.Barcodes, true)
	if err != nil {
		return MoveResult{}, err
	}
	var out MoveResult
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		loc, err := resolveLocation(ctx, q, org, in.LocationUUID, in.LocationCode)
		if err != nil {
			return err
		}
		out = MoveResult{Location: locView(loc), Units: make([]MovedUnit, 0, len(codes))}
		target := ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc.ID, OrgID: org}
		for _, code := range codes {
			u, err := s.serialUnit(ctx, q, c, code)
			if err != nil {
				return err
			}
			st, err := onHand(ctx, q, u, org)
			if err != nil {
				return err
			}
			if err := checkNoOpenTransfer(ctx, q, u); err != nil {
				return err
			}
			mu := MovedUnit{UnitUUID: u.Uuid, Barcode: u.Barcode}
			from := ledger.Owner{Type: ledger.OwnerType(st.OwnerType), ID: st.OwnerID, OrgID: org}
			if st.OwnerType == string(ledger.OwnerWarehouseLocation) {
				if st.OwnerID == loc.ID {
					return invalid("barcodes", "unit "+u.Barcode+" is already on this location")
				}
				cur, err := q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: st.OwnerID, OrganizationID: org})
				if err != nil {
					return err
				}
				if cur.WarehouseID != loc.WarehouseID {
					return invalid("location_uuid", "unit "+u.Barcode+" is in another warehouse; use a warehouse transfer")
				}
				v := locView(cur)
				mu.FromLocation = &v
			}
			if !st.LastMovementID.Valid {
				return fmt.Errorf("%w: %s", ErrUnitUnavailable, u.Barcode)
			}
			mv, err := s.post(ctx, tx, u, ledger.Movement{
				Type: ledger.TypePlacement, UnitID: u.ID, From: &from, To: &target,
				Source: moveSource, RefType: moveRefType, RefID: st.LastMovementID.Int64,
				ActorUserID: actorPtr(c), Reason: "warehouse move",
				Metadata: map[string]any{"location_uuid": loc.Uuid.String()},
			})
			if err != nil {
				return err
			}
			id := mv.Uuid
			mu.MovementUUID = &id
			out.Units = append(out.Units, mu)
		}
		return auditRow(ctx, q, c, AuditMoveDone, AuditResourceMove, loc.Uuid, map[string]any{
			"organization_id": org, "location_id": loc.ID, "units": len(out.Units),
		})
	})
	if err != nil {
		return MoveResult{}, err
	}
	return out, nil
}

// --- warehouse transfer documents ------------------------------------------

// CreateTransfer opens a draft between two active warehouses of the active
// organization.
func (s *WarehouseTransfers) CreateTransfer(ctx context.Context, c EntryCaller, in TransferInput) (WarehouseTransfer, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return WarehouseTransfer{}, err
	}
	note, err := normNote(in.Note)
	if err != nil {
		return WarehouseTransfer{}, err
	}
	from, err := s.activeWarehouse(ctx, org, "from_warehouse_uuid", in.FromWarehouseUUID)
	if err != nil {
		return WarehouseTransfer{}, err
	}
	to, err := s.activeWarehouse(ctx, org, "to_warehouse_uuid", in.ToWarehouseUUID)
	if err != nil {
		return WarehouseTransfer{}, err
	}
	if from.ID == to.ID {
		return WarehouseTransfer{}, invalid("to_warehouse_uuid", "must differ from the source warehouse; use a move inside one warehouse")
	}
	var toLoc pgtype.Int8
	if in.ToLocationUUID != nil {
		l, err := targetLocation(ctx, s.q, org, to.ID, "to_location_uuid", in.ToLocationUUID, "")
		if err != nil {
			return WarehouseTransfer{}, err
		}
		toLoc = pgtype.Int8{Int64: l.ID, Valid: true}
	}
	var t db.WarehouseTransfer
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		t, err = q.CreateWarehouseTransfer(ctx, db.CreateWarehouseTransferParams{
			OrganizationID: org, BrandID: c.Org.BrandID, FromWarehouseID: from.ID, ToWarehouseID: to.ID,
			ToLocationID: toLoc, Note: note, CreatedByUserID: c.actor(),
		})
		if err != nil {
			return err
		}
		return s.audit(ctx, q, c, AuditTransferCreated, t, map[string]any{"from_warehouse_id": from.ID, "to_warehouse_id": to.ID})
	})
	if err != nil {
		return WarehouseTransfer{}, err
	}
	return s.view(ctx, s.q, t, true)
}

// ListTransfers returns the transfers of the active organization (TEC-375:
// list contract, default newest first).
func (s *WarehouseTransfers) ListTransfers(ctx context.Context, c EntryCaller, f TransferListFilter) ([]WarehouseTransfer, int64, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return nil, 0, err
	}
	f.Sort = orDefault(f.Sort, TransferSort)
	cp := db.CountWarehouseTransfersParams{
		OrganizationID: org, Statuses: f.Statuses,
		FromWarehouseUuids: f.FromWarehouseUUIDs, ToWarehouseUuids: f.ToWarehouseUUIDs,
		CreatedFrom: listTS(f.CreatedFrom), CreatedBefore: listTS(f.CreatedBefore), Q: listQ(f.Q),
	}
	rows, err := s.q.ListWarehouseTransfers(ctx, db.ListWarehouseTransfersParams{
		OrganizationID: cp.OrganizationID, Statuses: cp.Statuses,
		FromWarehouseUuids: cp.FromWarehouseUuids, ToWarehouseUuids: cp.ToWarehouseUuids,
		CreatedFrom: cp.CreatedFrom, CreatedBefore: cp.CreatedBefore, Q: cp.Q,
		SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, PageLimit: f.Limit, PageOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountWarehouseTransfers(ctx, cp)
	if err != nil {
		return nil, 0, err
	}
	out := make([]WarehouseTransfer, 0, len(rows))
	for _, t := range rows {
		v, err := s.view(ctx, s.q, t, false)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, nil
}

// GetTransfer returns one transfer with its lines.
func (s *WarehouseTransfers) GetTransfer(ctx context.Context, c EntryCaller, id uuid.UUID) (WarehouseTransfer, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return WarehouseTransfer{}, err
	}
	t, err := s.q.GetWarehouseTransferByUUID(ctx, db.GetWarehouseTransferByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return WarehouseTransfer{}, notFound(err, ErrTransferNotFound)
	}
	return s.view(ctx, s.q, t, true)
}

// AddTransferLines adds scanned serial units placed in the source
// warehouse to a draft.
func (s *WarehouseTransfers) AddTransferLines(ctx context.Context, c EntryCaller, id uuid.UUID, barcodes []string) (WarehouseTransfer, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return WarehouseTransfer{}, err
	}
	codes, err := normBarcodes(barcodes, true)
	if err != nil {
		return WarehouseTransfer{}, err
	}
	var t db.WarehouseTransfer
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		if t, err = s.lock(ctx, q, org, id, TransferStatusDraft); err != nil {
			return err
		}
		have, err := q.CountWarehouseTransferLines(ctx, t.ID)
		if err != nil {
			return err
		}
		if have+int64(len(codes)) > MaxTransferLines {
			return invalid("barcodes", fmt.Sprintf("a transfer holds at most %d lines", MaxTransferLines))
		}
		for _, code := range codes {
			u, err := s.serialUnit(ctx, q, c, code)
			if err != nil {
				return err
			}
			if _, err := s.inSourceWarehouse(ctx, q, u, t); err != nil {
				return err
			}
			if err := checkFree(ctx, q, u); err != nil {
				return err
			}
			if _, err := q.InsertWarehouseTransferLine(ctx, db.InsertWarehouseTransferLineParams{TransferID: t.ID, UnitID: u.ID}); err != nil {
				var pg *pgconn.PgError
				if errors.As(err, &pg) && pg.Code == "23505" {
					if pg.ConstraintName == "uq_warehouse_transfer_lines_transfer_unit" {
						return invalid("barcodes", "barcode "+u.Barcode+" is already on this transfer")
					}
					return fmt.Errorf("%w: %s is on another open warehouse transfer", ErrUnitBusy, u.Barcode)
				}
				return err
			}
		}
		return nil
	})
	if err != nil {
		return WarehouseTransfer{}, err
	}
	return s.view(ctx, s.q, t, true)
}

// DeleteTransferLine removes a line from a draft.
func (s *WarehouseTransfers) DeleteTransferLine(ctx context.Context, c EntryCaller, id, lineID uuid.UUID) (WarehouseTransfer, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return WarehouseTransfer{}, err
	}
	var t db.WarehouseTransfer
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		if t, err = s.lock(ctx, q, org, id, TransferStatusDraft); err != nil {
			return err
		}
		n, err := q.DeleteWarehouseTransferLine(ctx, db.DeleteWarehouseTransferLineParams{Uuid: lineID, TransferID: t.ID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrTransferLineNotFound
		}
		return nil
	})
	if err != nil {
		return WarehouseTransfer{}, err
	}
	return s.view(ctx, s.q, t, true)
}

// PlaceTransferLines sets the target location (target warehouse) of lines
// of a draft or in-transit transfer: the given lines, the lines of the
// scanned barcodes, or every line.
func (s *WarehouseTransfers) PlaceTransferLines(ctx context.Context, c EntryCaller, id uuid.UUID, in PlaceInput) (WarehouseTransfer, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return WarehouseTransfer{}, err
	}
	code := strings.TrimSpace(in.LocationCode)
	if (in.LocationUUID == nil) == (code == "") {
		return WarehouseTransfer{}, invalid("location_uuid", "give exactly one of location_uuid or location_code")
	}
	if len(in.LineUUIDs) > 0 && len(in.Barcodes) > 0 {
		return WarehouseTransfer{}, invalid("line_uuids", "give line_uuids or barcodes, not both")
	}
	codes, err := normBarcodes(in.Barcodes, false)
	if err != nil {
		return WarehouseTransfer{}, err
	}
	var t db.WarehouseTransfer
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		if t, err = s.lock(ctx, q, org, id, TransferStatusDraft, TransferStatusInTransit); err != nil {
			return err
		}
		loc, err := targetLocation(ctx, q, org, t.ToWarehouseID, "location_uuid", in.LocationUUID, code)
		if err != nil {
			return err
		}
		lines, err := q.ListWarehouseTransferLines(ctx, t.ID)
		if err != nil {
			return err
		}
		ids, err := pickTransferLines(ctx, q, lines, in.LineUUIDs, codes)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return ErrTransferEmpty
		}
		_, err = q.SetWarehouseTransferLineTarget(ctx, db.SetWarehouseTransferLineTargetParams{
			LocationID: pgtype.Int8{Int64: loc.ID, Valid: true}, TransferID: t.ID, Ids: ids,
		})
		return err
	})
	if err != nil {
		return WarehouseTransfer{}, err
	}
	return s.view(ctx, s.q, t, true)
}

// Ship posts a transfer_out per line (draft -> in_transit).
func (s *WarehouseTransfers) Ship(ctx context.Context, c EntryCaller, id uuid.UUID) (WarehouseTransfer, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return WarehouseTransfer{}, err
	}
	var t db.WarehouseTransfer
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		if t, err = s.lock(ctx, q, org, id, TransferStatusDraft); err != nil {
			return err
		}
		lines, err := q.ListWarehouseTransferLines(ctx, t.ID)
		if err != nil {
			return err
		}
		if len(lines) == 0 {
			return ErrTransferEmpty
		}
		self := ledger.Owner{Type: ledger.OwnerOrganization, ID: org, OrgID: org}
		for _, l := range lines {
			u, err := q.LockUnit(ctx, l.UnitID)
			if err != nil {
				return err
			}
			st, err := s.inSourceWarehouse(ctx, q, u, t)
			if err != nil {
				return err
			}
			// An order may have reserved the unit after it was added.
			if err := checkNotReserved(ctx, q, u); err != nil {
				return err
			}
			from := ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: st.OwnerID, OrgID: org}
			mv, err := s.post(ctx, tx, u, ledger.Movement{
				Type: ledger.TypeTransferOut, UnitID: u.ID, From: &from, To: &self,
				Source: transferSource, RefType: transferRefType, RefID: l.ID,
				ActorUserID: actorPtr(c), Reason: "warehouse transfer", Metadata: transferMeta(t, l),
			})
			if err != nil {
				return err
			}
			if err := q.SetWarehouseTransferLineOut(ctx, db.SetWarehouseTransferLineOutParams{
				ID: l.ID, MovementID: pgtype.Int8{Int64: mv.ID, Valid: true},
				SourceLocationID: pgtype.Int8{Int64: st.OwnerID, Valid: true},
			}); err != nil {
				return err
			}
		}
		if t, err = q.ShipWarehouseTransfer(ctx, db.ShipWarehouseTransferParams{UserID: c.actor(), ID: t.ID}); err != nil {
			return notFound(err, ErrTransferState)
		}
		return s.audit(ctx, q, c, AuditTransferShipped, t, map[string]any{"lines": len(lines)})
	})
	if err != nil {
		return WarehouseTransfer{}, err
	}
	return s.view(ctx, s.q, t, true)
}

// Complete posts the paired transfer_in and a placement onto the target
// location of every line (in_transit -> completed). in names a location of
// the target warehouse for lines without one; the transfer's default
// location is used otherwise.
func (s *WarehouseTransfers) Complete(ctx context.Context, c EntryCaller, id uuid.UUID, in CompleteInput) (WarehouseTransfer, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return WarehouseTransfer{}, err
	}
	code := strings.TrimSpace(in.LocationCode)
	if in.LocationUUID != nil && code != "" {
		return WarehouseTransfer{}, invalid("location_uuid", "give at most one of location_uuid or location_code")
	}
	var t db.WarehouseTransfer
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		if t, err = s.lock(ctx, q, org, id, TransferStatusInTransit); err != nil {
			return err
		}
		fallback := t.ToLocationID
		if in.LocationUUID != nil || code != "" {
			loc, err := targetLocation(ctx, q, org, t.ToWarehouseID, "location_uuid", in.LocationUUID, code)
			if err != nil {
				return err
			}
			fallback = pgtype.Int8{Int64: loc.ID, Valid: true}
		}
		lines, err := q.ListWarehouseTransferLines(ctx, t.ID)
		if err != nil {
			return err
		}
		targets := make([]int64, len(lines))
		for i, l := range lines {
			switch {
			case l.TargetLocationID.Valid:
				targets[i] = l.TargetLocationID.Int64
			case fallback.Valid:
				targets[i] = fallback.Int64
			default:
				return ErrTransferUnplaced
			}
			// The target must still be an active location of the target warehouse.
			loc, err := q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: targets[i], OrganizationID: org})
			if err != nil {
				return err
			}
			if !loc.Active || !loc.WarehouseID.Valid || loc.WarehouseID.Int64 != t.ToWarehouseID {
				return invalid("location_uuid", "target location "+loc.Code+" is inactive or outside the target warehouse")
			}
		}
		self := ledger.Owner{Type: ledger.OwnerOrganization, ID: org, OrgID: org}
		for i, l := range lines {
			u, err := q.GetUnit(ctx, l.UnitID)
			if err != nil {
				return err
			}
			meta := transferMeta(t, l)
			inMv, err := s.post(ctx, tx, u, ledger.Movement{
				Type: ledger.TypeTransferIn, UnitID: u.ID, To: &self,
				Source: transferSource, RefType: transferRefType, RefID: l.ID,
				ActorUserID: actorPtr(c), Reason: "warehouse transfer received", Metadata: meta,
			})
			if err != nil {
				return err
			}
			loc := ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: targets[i], OrgID: org}
			pl, err := s.post(ctx, tx, u, ledger.Movement{
				Type: ledger.TypePlacement, UnitID: u.ID, From: &self, To: &loc,
				Source: transferSource, RefType: transferRefType, RefID: l.ID,
				ActorUserID: actorPtr(c), Reason: "warehouse transfer placement", Metadata: meta,
			})
			if err != nil {
				return err
			}
			if err := q.SetWarehouseTransferLineIn(ctx, db.SetWarehouseTransferLineInParams{
				ID: l.ID, InMovementID: pgtype.Int8{Int64: inMv.ID, Valid: true},
				PlacementMovementID: pgtype.Int8{Int64: pl.ID, Valid: true},
				TargetLocationID:    pgtype.Int8{Int64: targets[i], Valid: true},
			}); err != nil {
				return err
			}
		}
		if err := q.CloseWarehouseTransferLines(ctx, t.ID); err != nil {
			return err
		}
		if t, err = q.CompleteWarehouseTransfer(ctx, db.CompleteWarehouseTransferParams{UserID: c.actor(), ID: t.ID}); err != nil {
			return notFound(err, ErrTransferState)
		}
		return s.audit(ctx, q, c, AuditTransferCompleted, t, map[string]any{"lines": len(lines)})
	})
	if err != nil {
		return WarehouseTransfer{}, err
	}
	return s.view(ctx, s.q, t, true)
}

// Cancel closes a draft without stock, or restores every shipped unit of
// an in-transit transfer to its source location (transfer_cancel_restore).
func (s *WarehouseTransfers) Cancel(ctx context.Context, c EntryCaller, id uuid.UUID) (WarehouseTransfer, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return WarehouseTransfer{}, err
	}
	var t db.WarehouseTransfer
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		if t, err = s.lock(ctx, q, org, id, TransferStatusDraft, TransferStatusInTransit); err != nil {
			return err
		}
		restored := 0
		if t.Status == TransferStatusInTransit {
			lines, err := q.ListWarehouseTransferLines(ctx, t.ID)
			if err != nil {
				return err
			}
			for _, l := range lines {
				u, err := q.GetUnit(ctx, l.UnitID)
				if err != nil {
					return err
				}
				mv, err := s.post(ctx, tx, u, ledger.Movement{
					Type: ledger.TypeTransferCancelRestore, UnitID: u.ID,
					Source: transferSource, RefType: transferRefType, RefID: l.ID,
					ActorUserID: actorPtr(c), Reason: "warehouse transfer cancelled after shipping",
					Metadata: transferMeta(t, l),
				})
				if err != nil {
					return err
				}
				if err := q.SetWarehouseTransferLineRestore(ctx, db.SetWarehouseTransferLineRestoreParams{
					ID: l.ID, MovementID: pgtype.Int8{Int64: mv.ID, Valid: true},
				}); err != nil {
					return err
				}
				restored++
			}
		}
		if err := q.CloseWarehouseTransferLines(ctx, t.ID); err != nil {
			return err
		}
		if t, err = q.CancelWarehouseTransfer(ctx, db.CancelWarehouseTransferParams{UserID: c.actor(), ID: t.ID}); err != nil {
			return notFound(err, ErrTransferState)
		}
		return s.audit(ctx, q, c, AuditTransferCancelled, t, map[string]any{"restored": restored})
	})
	if err != nil {
		return WarehouseTransfer{}, err
	}
	return s.view(ctx, s.q, t, true)
}

// --- order receipt placement ------------------------------------------------

// PlaceOrder shelves the received serial units of an order bought by the
// active organization onto one of its locations (the given one, else the
// order's buyer location). Units no longer on hand unplaced in the
// organization (already shelved, moved on) and fixed barcodes are skipped.
// Barcodes limits the units; empty means every unit of the order.
func (s *WarehouseTransfers) PlaceOrder(ctx context.Context, c EntryCaller, orderID uuid.UUID, in MoveInput) (MoveResult, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return MoveResult{}, err
	}
	codes, err := normBarcodes(in.Barcodes, false)
	if err != nil {
		return MoveResult{}, err
	}
	code := strings.TrimSpace(in.LocationCode)
	if in.LocationUUID != nil && code != "" {
		return MoveResult{}, invalid("location_uuid", "give at most one of location_uuid or location_code")
	}
	o, err := s.q.GetOrderByUUID(ctx, db.GetOrderByUUIDParams{Uuid: orderID, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && o.BuyerOrgID != org) {
		return MoveResult{}, ErrOrderNotFound
	}
	if err != nil {
		return MoveResult{}, err
	}
	if o.Status != orderStatusReceived {
		return MoveResult{}, ErrOrderNotReceived
	}
	var out MoveResult
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		var loc db.WarehouseLocation
		switch {
		case in.LocationUUID != nil || code != "":
			if loc, err = resolveLocation(ctx, q, org, in.LocationUUID, code); err != nil {
				return err
			}
		case o.BuyerWarehouseLocationID.Valid:
			if loc, err = q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: o.BuyerWarehouseLocationID.Int64, OrganizationID: org}); err != nil {
				return notFound(err, ErrLocationNotFound)
			}
			if !loc.Active || !loc.WarehouseID.Valid {
				return invalid("location_uuid", "the order's location is inactive; pick a location")
			}
		default:
			return invalid("location_uuid", "is required (the order names no buyer location)")
		}
		units, err := q.ListOrderItemUnitsByOrder(ctx, o.ID)
		if err != nil {
			return err
		}
		want := map[string]bool{}
		for _, cd := range codes {
			want[strings.ToUpper(cd)] = true
		}
		out = MoveResult{Location: locView(loc), Units: []MovedUnit{}}
		target := ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc.ID, OrgID: org}
		self := ledger.Owner{Type: ledger.OwnerOrganization, ID: org, OrgID: org}
		seen := map[string]bool{}
		for _, a := range units {
			key := strings.ToUpper(a.Barcode)
			if len(want) > 0 && !want[key] {
				continue
			}
			seen[key] = true
			mu := MovedUnit{UnitUUID: a.UnitUuid, Barcode: a.Barcode}
			if a.UnitKind == ledger.KindFixed {
				mu.Skipped = "fixed"
				out.Units = append(out.Units, mu)
				continue
			}
			u, err := q.LockUnit(ctx, a.UnitID)
			if err != nil {
				return err
			}
			st, err := q.GetUnitCurrentState(ctx, u.ID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if err != nil || st.HolderOrgID != org || st.Status != string(ledger.StatusAvailable) ||
				st.OwnerType != string(ledger.OwnerOrganization) {
				mu.Skipped = "not_on_hand"
				out.Units = append(out.Units, mu)
				continue
			}
			mv, err := s.post(ctx, tx, u, ledger.Movement{
				Type: ledger.TypePlacement, UnitID: u.ID, From: &self, To: &target,
				Source: orderPlaceSource, RefType: orderPlaceRef, RefID: a.ID,
				ActorUserID: actorPtr(c), Reason: "order receipt placement",
				Metadata: map[string]any{"order_uuid": o.Uuid.String(), "order_no": o.OrderNo},
			})
			if err != nil {
				return err
			}
			id := mv.Uuid
			mu.MovementUUID = &id
			out.Units = append(out.Units, mu)
		}
		for _, cd := range codes {
			if !seen[strings.ToUpper(cd)] {
				return invalid("barcodes", "barcode "+cd+" is not on this order")
			}
		}
		return auditRow(ctx, q, c, AuditOrderPlaced, AuditResourceMove, o.Uuid, map[string]any{
			"organization_id": org, "order_id": o.ID, "location_id": loc.ID, "units": len(out.Units),
		})
	})
	if err != nil {
		return MoveResult{}, err
	}
	return out, nil
}

// --- helpers -----------------------------------------------------------------

func actorPtr(c EntryCaller) *int64 {
	if c.Principal.UserInternal <= 0 {
		return nil
	}
	v := c.Principal.UserInternal
	return &v
}

func transferMeta(t db.WarehouseTransfer, l db.WarehouseTransferLine) map[string]any {
	return map[string]any{"warehouse_transfer_uuid": t.Uuid.String(), "transfer_no": t.TransferNo, "line_uuid": l.Uuid.String()}
}

// post writes one movement; a ledger refusal becomes ErrTransferLedger
// (in-transit or otherwise moved units become ErrUnitBusy).
func (s *WarehouseTransfers) post(ctx context.Context, tx pgx.Tx, u db.Unit, m ledger.Movement) (db.StockMovement, error) {
	res, err := s.ledger.Post(ctx, tx, m)
	if err != nil {
		return db.StockMovement{}, fmt.Errorf("%w: %s %s: %w", ErrTransferLedger, m.Type, u.Barcode, err)
	}
	return res.Movement, nil
}

// serialUnit resolves a scanned barcode in the active brand and locks it.
func (s *WarehouseTransfers) serialUnit(ctx context.Context, q *db.Queries, c EntryCaller, code string) (db.Unit, error) {
	u, err := q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: c.Org.BrandID, Barcode: code})
	if errors.Is(err, pgx.ErrNoRows) {
		u, err = q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: c.Org.BrandID, Barcode: strings.ToUpper(code)})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return u, invalid("barcodes", "barcode "+code+" not found")
	}
	if err != nil {
		return u, err
	}
	if u.UnitKind == ledger.KindFixed {
		return u, invalid("barcodes", "barcode "+u.Barcode+" is a fixed barcode; fixed quantities are not moved by warehouse moves or transfers")
	}
	return q.LockUnit(ctx, u.ID)
}

// onHand returns the state of a serial unit on hand (available or placed)
// in org; in transit or elsewhere it is refused.
func onHand(ctx context.Context, q *db.Queries, u db.Unit, org int64) (db.UnitCurrentState, error) {
	st, err := q.GetUnitCurrentState(ctx, u.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, fmt.Errorf("%w: %s", ErrUnitUnavailable, u.Barcode)
	}
	if err != nil {
		return st, err
	}
	if st.HolderOrgID != org {
		return st, fmt.Errorf("%w: %s", ErrUnitUnavailable, u.Barcode)
	}
	if st.Status == string(ledger.StatusInTransit) {
		return st, fmt.Errorf("%w: %s is in transit", ErrUnitBusy, u.Barcode)
	}
	if st.Status != string(ledger.StatusAvailable) && st.Status != string(ledger.StatusPlaced) {
		return st, fmt.Errorf("%w: %s", ErrUnitUnavailable, u.Barcode)
	}
	return st, nil
}

// inSourceWarehouse checks that the unit is on hand on a location of the
// transfer's source warehouse.
func (s *WarehouseTransfers) inSourceWarehouse(ctx context.Context, q *db.Queries, u db.Unit, t db.WarehouseTransfer) (db.UnitCurrentState, error) {
	st, err := onHand(ctx, q, u, t.OrganizationID)
	if err != nil {
		return st, err
	}
	if st.OwnerType != string(ledger.OwnerWarehouseLocation) {
		return st, fmt.Errorf("%w: %s is not placed in the source warehouse", ErrUnitUnavailable, u.Barcode)
	}
	loc, err := q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: st.OwnerID, OrganizationID: t.OrganizationID})
	if err != nil {
		return st, err
	}
	if !loc.WarehouseID.Valid || loc.WarehouseID.Int64 != t.FromWarehouseID {
		return st, fmt.Errorf("%w: %s is not placed in the source warehouse", ErrUnitUnavailable, u.Barcode)
	}
	return st, nil
}

// checkNoOpenTransfer refuses a unit on an open warehouse transfer.
func checkNoOpenTransfer(ctx context.Context, q *db.Queries, u db.Unit) error {
	t, err := q.FindOpenWarehouseTransferForUnit(ctx, u.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: %s is on warehouse transfer %s", ErrUnitBusy, u.Barcode, t.TransferNo)
}

// checkNotReserved refuses a unit an order reserves or an open stock
// transfer request (TEC-197) holds.
func checkNotReserved(ctx context.Context, q *db.Queries, u db.Unit) error {
	active, err := q.LockActiveReservationsByUnit(ctx, u.ID)
	if err != nil {
		return err
	}
	if len(active) > 0 {
		return fmt.Errorf("%w: %s is reserved by an order", ErrUnitBusy, u.Barcode)
	}
	n, err := q.CountOpenTransferItemsByUnit(ctx, db.CountOpenTransferItemsByUnitParams{UnitID: u.ID, ExcludeRequestID: 0})
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%w: %s is on an open stock transfer request", ErrUnitBusy, u.Barcode)
	}
	return nil
}

// checkFree: no open warehouse transfer, order reservation or transfer
// request holds the unit.
func checkFree(ctx context.Context, q *db.Queries, u db.Unit) error {
	if err := checkNoOpenTransfer(ctx, q, u); err != nil {
		return err
	}
	return checkNotReserved(ctx, q, u)
}

func (s *WarehouseTransfers) activeWarehouse(ctx context.Context, org int64, field, raw string) (db.Warehouse, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return db.Warehouse{}, invalid(field, "must be a uuid")
	}
	w, err := s.q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: id, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return w, ErrWarehouseNotFound
	}
	if err != nil {
		return w, err
	}
	if !w.Active {
		return w, invalid(field, "warehouse is inactive")
	}
	return w, nil
}

// resolveLocation finds an active typed location of org by uuid or by a
// scanned QR / full_code (exactly one).
func resolveLocation(ctx context.Context, q *db.Queries, org int64, id *uuid.UUID, rawCode string) (db.WarehouseLocation, error) {
	code := strings.TrimSpace(rawCode)
	if (id == nil) == (code == "") {
		return db.WarehouseLocation{}, invalid("location_uuid", "give exactly one of location_uuid or location_code")
	}
	var (
		l   db.WarehouseLocation
		err error
	)
	field := "location_uuid"
	if id != nil {
		l, err = q.GetTypedLocationByUUID(ctx, db.GetTypedLocationByUUIDParams{Uuid: *id, OrganizationID: org})
	} else {
		field = "location_code"
		if utf8.RuneCountInString(code) > MaxScanLen {
			return l, invalid(field, "is too long")
		}
		full := strings.ToUpper(code)
		if rest, ok := strings.CutPrefix(full, labels.LocationPrefix); ok {
			full = strings.TrimSpace(rest)
		}
		l, err = q.GetWarehouseLocationByCode(ctx, db.GetWarehouseLocationByCodeParams{OrganizationID: org, Code: full})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrLocationNotFound
	}
	if err != nil {
		return l, err
	}
	if !l.Active || !l.WarehouseID.Valid {
		return l, invalid(field, "location is inactive")
	}
	return l, nil
}

// targetLocation resolves a location that must belong to warehouse.
func targetLocation(ctx context.Context, q *db.Queries, org, warehouse int64, field string, id *uuid.UUID, code string) (db.WarehouseLocation, error) {
	l, err := resolveLocation(ctx, q, org, id, code)
	if err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) && field != "location_uuid" {
			ve.Field = field
		}
		return l, err
	}
	if l.WarehouseID.Int64 != warehouse {
		return l, invalid(field, "location is not in the target warehouse")
	}
	return l, nil
}

// pickTransferLines selects line ids by uuid, by barcode, or all lines.
func pickTransferLines(ctx context.Context, q *db.Queries, lines []db.WarehouseTransferLine, uuids []uuid.UUID, codes []string) ([]int64, error) {
	if len(uuids) == 0 && len(codes) == 0 {
		ids := make([]int64, 0, len(lines))
		for _, l := range lines {
			ids = append(ids, l.ID)
		}
		return ids, nil
	}
	byUUID := make(map[uuid.UUID]int64, len(lines))
	for _, l := range lines {
		byUUID[l.Uuid] = l.ID
	}
	ids := make([]int64, 0, len(uuids)+len(codes))
	for _, u := range uuids {
		id, ok := byUUID[u]
		if !ok {
			return nil, ErrTransferLineNotFound
		}
		ids = append(ids, id)
	}
	if len(codes) > 0 {
		byBarcode := make(map[string]int64, len(lines))
		for _, l := range lines {
			u, err := q.GetUnit(ctx, l.UnitID)
			if err != nil {
				return nil, err
			}
			byBarcode[strings.ToUpper(u.Barcode)] = l.ID
		}
		for _, code := range codes {
			id, ok := byBarcode[strings.ToUpper(code)]
			if !ok {
				return nil, invalid("barcodes", "barcode "+code+" is not on this transfer")
			}
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (s *WarehouseTransfers) lock(ctx context.Context, q *db.Queries, org int64, id uuid.UUID, statuses ...string) (db.WarehouseTransfer, error) {
	t, err := q.LockWarehouseTransferByUUID(ctx, db.LockWarehouseTransferByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return t, notFound(err, ErrTransferNotFound)
	}
	if !slices.Contains(statuses, t.Status) {
		return t, ErrTransferState
	}
	return t, nil
}

func (s *WarehouseTransfers) inTx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx), tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *WarehouseTransfers) audit(ctx context.Context, q *db.Queries, c EntryCaller, action string, t db.WarehouseTransfer, extra map[string]any) error {
	m := map[string]any{"warehouse_transfer_id": t.ID, "organization_id": t.OrganizationID, "status": t.Status}
	for k, v := range extra {
		m[k] = v
	}
	return auditRow(ctx, q, c, action, AuditResourceTransfer, t.Uuid, m)
}

func auditRow(ctx context.Context, q *db.Queries, c EntryCaller, action, resource string, id uuid.UUID, m map[string]any) error {
	payload, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = q.InsertActivityEvent(ctx, db.InsertActivityEventParams{
		ActorUserID: c.actor(), Action: action, Resource: resource,
		ResourceUuid: pgtype.UUID{Bytes: id, Valid: true}, Payload: payload,
	})
	return err
}

func locView(l db.WarehouseLocation) EntryLocation {
	return EntryLocation{UUID: l.Uuid, Code: l.Code, FullCode: textPtr(l.FullCode)}
}

// view builds the document; full adds the lines.
func (s *WarehouseTransfers) view(ctx context.Context, q *db.Queries, t db.WarehouseTransfer, full bool) (WarehouseTransfer, error) {
	out := WarehouseTransfer{
		UUID: t.Uuid, TransferNo: t.TransferNo, Status: t.Status, Note: textPtr(t.Note),
		CreatedAt: ts(t.CreatedAt), ShippedAt: tsPtr(t.ShippedAt), CompletedAt: tsPtr(t.CompletedAt), CancelledAt: tsPtr(t.CancelledAt),
	}
	for _, side := range []struct {
		id  int64
		dst *EntryWarehouse
	}{{t.FromWarehouseID, &out.FromWarehouse}, {t.ToWarehouseID, &out.ToWarehouse}} {
		w, err := q.GetWarehouseByID(ctx, db.GetWarehouseByIDParams{ID: side.id, OrganizationID: t.OrganizationID})
		if err != nil {
			return out, err
		}
		*side.dst = EntryWarehouse{UUID: w.Uuid, Code: w.Code, Name: w.Name}
	}
	locations := map[int64]db.WarehouseLocation{}
	location := func(id pgtype.Int8) (*EntryLocation, error) {
		if !id.Valid {
			return nil, nil
		}
		l, ok := locations[id.Int64]
		if !ok {
			var err error
			if l, err = q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: id.Int64, OrganizationID: t.OrganizationID}); err != nil {
				return nil, err
			}
			locations[id.Int64] = l
		}
		v := locView(l)
		return &v, nil
	}
	var err error
	if out.ToLocation, err = location(t.ToLocationID); err != nil {
		return out, err
	}
	if !full {
		if out.LineCount, err = q.CountWarehouseTransferLines(ctx, t.ID); err != nil {
			return out, err
		}
		return out, nil
	}
	lines, err := q.ListWarehouseTransferLines(ctx, t.ID)
	if err != nil {
		return out, err
	}
	out.LineCount = int64(len(lines))
	out.Lines = make([]TransferLine, 0, len(lines))
	products := map[int64]db.Product{}
	for _, l := range lines {
		u, err := q.GetUnit(ctx, l.UnitID)
		if err != nil {
			return out, err
		}
		p, ok := products[u.ProductID]
		if !ok {
			if p, err = q.GetProductByIDAnyBrand(ctx, u.ProductID); err != nil {
				return out, err
			}
			products[u.ProductID] = p
		}
		line := TransferLine{
			UUID: l.Uuid, UnitUUID: u.Uuid, Barcode: u.Barcode, UnitStatus: u.Status,
			Product: EntryProduct{UUID: p.Uuid, SKU: p.Sku, Name: p.Name},
		}
		if line.SourceLocation, err = location(l.SourceLocationID); err != nil {
			return out, err
		}
		if line.TargetLocation, err = location(l.TargetLocationID); err != nil {
			return out, err
		}
		for _, mv := range []struct {
			id  pgtype.Int8
			dst **uuid.UUID
		}{
			{l.OutMovementID, &line.OutMovementUUID}, {l.InMovementID, &line.InMovementUUID},
			{l.PlacementMovementID, &line.PlacementMovementUUID}, {l.RestoreMovementID, &line.RestoreMovementUUID},
		} {
			if *mv.dst, err = movementUUID(ctx, q, mv.id); err != nil {
				return out, err
			}
		}
		out.Lines = append(out.Lines, line)
	}
	return out, nil
}
