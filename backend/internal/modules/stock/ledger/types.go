// Package ledger is the single write path of the stock ledger (TEC-154).
//
// Every stock change is one call to Ledger.Post inside the caller's
// transaction: the unit row is locked (FOR UPDATE), the movement is checked
// against the transition table, appended to stock_movements (idempotent by
// key), the projections (unit_current_state, fixed_barcode_holdings,
// bin_product_stocks, organization_product_stocks) are updated, units.status
// is kept equal to unit_current_state.status and a stock.* event is written
// to the outbox. Nothing else writes these tables (rebuild/repair: TEC-156).
//
// Serial units (pieces and rolls) have exactly one owner: the movement's
// to_owner is the owner after the movement. Fixed barcode units hold a
// quantity per owner (TEC-94 decision 1): each movement touches exactly one
// owner, quantity_delta < 0 debits from_owner and quantity_delta > 0
// credits to_owner. Moving fixed stock is two movements (an *_out and an
// *_in / *_cancel_restore with the same reference).
package ledger

import (
	"errors"
)

// MovementType is stock_movements.type.
type MovementType string

const (
	TypeEntry                 MovementType = "entry"
	TypePlacement             MovementType = "placement"
	TypeTransferOut           MovementType = "transfer_out"
	TypeTransferIn            MovementType = "transfer_in"
	TypeTransferCancelRestore MovementType = "transfer_cancel_restore"
	TypeOrderOut              MovementType = "order_out"
	TypeReceived              MovementType = "received"
	TypeOrderCancelRestore    MovementType = "order_cancel_restore"
	TypeConsumption           MovementType = "consumption"
	TypePartialConsumption    MovementType = "partial_consumption"
	TypeReturn                MovementType = "return"
	TypeSale                  MovementType = "sale"
	TypeCountAdjustment       MovementType = "count_adjustment"
	TypeVoid                  MovementType = "void"
	TypeExternalOutbound      MovementType = "external_outbound"
)

// PostableTypes lists the movement types Post accepts. reclassification is
// written by the reclassification flow (TEC-157).
var PostableTypes = []MovementType{
	TypeEntry, TypePlacement, TypeTransferOut, TypeTransferIn, TypeTransferCancelRestore,
	TypeOrderOut, TypeReceived, TypeOrderCancelRestore, TypeConsumption,
	TypePartialConsumption, TypeReturn, TypeSale, TypeCountAdjustment, TypeVoid, TypeExternalOutbound,
}

// OwnerType is the polymorphic owner kind.
type OwnerType string

const (
	OwnerWarehouseLocation OwnerType = "warehouse_location"
	OwnerOrganization      OwnerType = "organization"
	OwnerService           OwnerType = "service"
	OwnerTrash             OwnerType = "trash"
)

// Status is the unit lifecycle status.
type Status string

const (
	StatusReserved  Status = "reserved"
	StatusPrinted   Status = "printed"
	StatusAvailable Status = "available"
	StatusPlaced    Status = "placed"
	StatusInTransit Status = "in_transit"
	StatusUsed      Status = "used"
	StatusVoid      Status = "void"
)

// counted reports whether a unit in status s is on hand (product stock
// projections count available and placed units only).
func counted(s Status) bool { return s == StatusAvailable || s == StatusPlaced }

// Unit kinds.
const (
	KindSerial = "serial"
	KindFixed  = "fixed"
)

// Owner names an owner. OrgID is the holding organization: required for
// warehouse_location (the location's organization) and service (the
// organization that performs the service); for organization and trash it
// is the owner itself and may be left zero.
type Owner struct {
	Type  OwnerType
	ID    int64
	OrgID int64
}

// holder returns the holding organization of the owner.
func (o Owner) holder() int64 {
	if o.Type == OwnerOrganization || o.Type == OwnerTrash {
		return o.ID
	}
	return o.OrgID
}

func (o Owner) same(t OwnerType, id int64) bool { return o.Type == t && o.ID == id }

// Movement is one ledger command. The movement row's organization_id is the
// holder before the movement (after it for an entry; for fixed barcodes the
// holder of the touched owner).
type Movement struct {
	Type   MovementType
	UnitID int64
	// From is the expected current owner. Serial units: optional, checked
	// when set. Fixed units: required for debits.
	From *Owner
	// To is the target owner. Serial: required where the owner changes
	// (entry, placement, transfer_out, transfer_in, order_out, received,
	// consumption, return); cancel restores go back to the source owner
	// and void/external_outbound go to the holder's trash. Fixed: required
	// for credits.
	To *Owner
	// Quantity is the fixed barcode quantity (> 0; count_adjustment is
	// signed). Ignored for serial units (always one).
	Quantity int32
	// Centimeters is the roll length in hundredths of a meter (NUMERIC(10,2)):
	// partial_consumption (> 0), count_adjustment (signed), return (>= 0,
	// meters coming back to the roll).
	Centimeters int64

	// Idempotency key parts: {source}:{ref_type}:{ref_id}:{type}:{barcode}.
	Source  string
	RefType string
	RefID   int64

	ActorUserID *int64
	Reason      string
	Metadata    map[string]any
}

// Errors returned by Post. Callers map them to HTTP codes (TEC-155).
var (
	ErrInvalidMovement      = errors.New("ledger: invalid movement")
	ErrUnitNotFound         = errors.New("ledger: unit not found")
	ErrTransitionNotAllowed = errors.New("ledger: transition not allowed")
	ErrOwnerMismatch        = errors.New("ledger: owner mismatch")
	ErrOwnerNotAllowed      = errors.New("ledger: owner not allowed")
	ErrInsufficientStock    = errors.New("ledger: insufficient stock")
	ErrInsufficientMeters   = errors.New("ledger: insufficient meters")
	ErrIdempotencyConflict  = errors.New("ledger: idempotency key reused for another movement")
	ErrConcurrentUpdate     = errors.New("ledger: concurrent update")
)
