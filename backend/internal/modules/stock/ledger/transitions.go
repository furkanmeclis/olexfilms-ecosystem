package ledger

import (
	"fmt"
	"slices"
)

// The transition tables are pure data plus pure planning functions so that
// they are table-tested without a database (transitions_test.go). Post adds
// the checks that need rows: locks, owner/organization lookups, paired
// reference movements and stock levels.

// targetRule says how the owner after a serial movement is chosen.
type targetRule int

const (
	targetGiven   targetRule = iota // Movement.To, type in rule.owners
	targetKeep                      // owner does not change
	targetRestore                   // back to the source owner of the paired out movement
	targetTrash                     // trash of the current holder
)

// holderRule constrains the organization holding the unit after the movement.
type holderRule int

const (
	holderAny    holderRule = iota // any organization allowed for the unit's brand
	holderSame                     // the current holder
	holderCenter                   // a center organization (entry, K14)
)

type serialRule struct {
	from       []Status    // allowed current statuses
	fromOwners []OwnerType // allowed current owner types (nil: any)
	to         Status      // status after; "" keeps it or restores the source status
	target     targetRule
	owners     []OwnerType // allowed target owner types (targetGiven)
	holder     holderRule
	pairedOut  MovementType // the previous movement must be this type, same reference
	rollOnly   bool
}

var (
	stocked     = []Status{StatusAvailable, StatusPlaced}
	stockOwners = []OwnerType{OwnerWarehouseLocation, OwnerOrganization}
)

// serialRules is the transition table for serial units (pieces and rolls).
var serialRules = map[MovementType]serialRule{
	// A printed (or still reserved) label enters stock at a center (K14).
	TypeEntry: {from: []Status{StatusReserved, StatusPrinted}, to: StatusAvailable,
		target: targetGiven, owners: stockOwners, holder: holderCenter},
	// Shelving inside the holder's own warehouse.
	TypePlacement: {from: stocked, to: StatusPlaced,
		target: targetGiven, owners: []OwnerType{OwnerWarehouseLocation}, holder: holderSame},
	// In transit the owner is the destination organization; stock
	// projections do not count in-transit units.
	TypeTransferOut: {from: stocked, to: StatusInTransit,
		target: targetGiven, owners: []OwnerType{OwnerOrganization}, holder: holderAny},
	TypeTransferIn: {from: []Status{StatusInTransit}, to: StatusAvailable,
		target: targetGiven, owners: stockOwners, holder: holderSame, pairedOut: TypeTransferOut},
	TypeTransferCancelRestore: {from: []Status{StatusInTransit},
		target: targetRestore, pairedOut: TypeTransferOut},
	TypeOrderOut: {from: stocked, to: StatusInTransit,
		target: targetGiven, owners: []OwnerType{OwnerOrganization}, holder: holderAny},
	TypeReceived: {from: []Status{StatusInTransit}, to: StatusAvailable,
		target: targetGiven, owners: stockOwners, holder: holderSame, pairedOut: TypeOrderOut},
	TypeOrderCancelRestore: {from: []Status{StatusInTransit},
		target: targetRestore, pairedOut: TypeOrderOut},
	// Used in a service of the holder; a roll is used up completely.
	TypeConsumption: {from: stocked, to: StatusUsed,
		target: targetGiven, owners: []OwnerType{OwnerService}, holder: holderSame},
	// Meters cut from a roll; the roll stays where it is.
	TypePartialConsumption: {from: stocked, target: targetKeep, rollOnly: true},
	// Back from a service into the holder's stock.
	TypeReturn: {from: []Status{StatusUsed}, fromOwners: []OwnerType{OwnerService}, to: StatusAvailable,
		target: targetGiven, owners: stockOwners, holder: holderSame},
	// Measured roll length differs from the ledger; a missing piece is a void.
	TypeCountAdjustment: {from: stocked, target: targetKeep, rollOnly: true},
	TypeVoid: {from: []Status{StatusReserved, StatusPrinted, StatusAvailable, StatusPlaced},
		to: StatusVoid, target: targetTrash},
	// Leaves the system (e.g. pushed to an external hub, K2).
	TypeExternalOutbound: {from: stocked, to: StatusUsed, target: targetTrash},
}

// serialCurrent is the locked state of a serial unit.
type serialCurrent struct {
	status    Status
	hasState  bool
	owner     Owner // OrgID = holder
	issuerOrg int64 // units.organization_id
	isRoll    bool
	initial   int64 // centimeters
	remaining int64 // centimeters
}

// prevMovement is the last movement of a unit (state.last_movement_id).
type prevMovement struct {
	typ        MovementType
	refType    string
	refID      int64
	fromOwner  *Owner
	fromStatus Status
}

type serialPlan struct {
	toStatus      Status
	toOwner       Owner
	ownerChanged  bool
	checkTarget   bool // the target owner/organization must be validated
	holder        holderRule
	metersDelta   int64
	quantityDelta int32
}

func planSerial(m Movement, cur serialCurrent, prev *prevMovement) (serialPlan, error) {
	rule, ok := serialRules[m.Type]
	if !ok {
		return serialPlan{}, fmt.Errorf("%w: %s for a serial unit", ErrTransitionNotAllowed, m.Type)
	}
	if rule.rollOnly && !cur.isRoll {
		return serialPlan{}, fmt.Errorf("%w: %s needs a roll", ErrTransitionNotAllowed, m.Type)
	}
	if !slices.Contains(rule.from, cur.status) {
		return serialPlan{}, fmt.Errorf("%w: %s from status %s", ErrTransitionNotAllowed, m.Type, cur.status)
	}
	if m.Type == TypeEntry && cur.hasState {
		return serialPlan{}, fmt.Errorf("%w: unit already has an owner", ErrTransitionNotAllowed)
	}
	if m.From != nil && (!cur.hasState || !m.From.same(cur.owner.Type, cur.owner.ID)) {
		return serialPlan{}, fmt.Errorf("%w: unit is not held by %s %d", ErrOwnerMismatch, m.From.Type, m.From.ID)
	}
	if rule.fromOwners != nil && !slices.Contains(rule.fromOwners, cur.owner.Type) {
		return serialPlan{}, fmt.Errorf("%w: %s from owner %s", ErrTransitionNotAllowed, m.Type, cur.owner.Type)
	}
	if rule.pairedOut != "" {
		if prev == nil || prev.typ != rule.pairedOut || prev.refType != m.RefType || prev.refID != m.RefID {
			return serialPlan{}, fmt.Errorf("%w: %s needs the %s of reference %s:%d",
				ErrTransitionNotAllowed, m.Type, rule.pairedOut, m.RefType, m.RefID)
		}
	}

	plan := serialPlan{toStatus: rule.to, holder: rule.holder}
	switch rule.target {
	case targetGiven:
		if m.To == nil {
			return serialPlan{}, fmt.Errorf("%w: %s needs a target owner", ErrInvalidMovement, m.Type)
		}
		if !slices.Contains(rule.owners, m.To.Type) {
			return serialPlan{}, fmt.Errorf("%w: %s to %s", ErrOwnerNotAllowed, m.Type, m.To.Type)
		}
		if err := validOwner(*m.To); err != nil {
			return serialPlan{}, err
		}
		to := normalized(*m.To)
		if rule.holder == holderSame && to.OrgID != cur.owner.OrgID {
			return serialPlan{}, fmt.Errorf("%w: %s must stay with organization %d",
				ErrOwnerNotAllowed, m.Type, cur.owner.OrgID)
		}
		if cur.hasState && to.same(cur.owner.Type, cur.owner.ID) && rule.to == cur.status {
			return serialPlan{}, fmt.Errorf("%w: unit is already there", ErrInvalidMovement)
		}
		plan.toOwner, plan.checkTarget = to, true
	case targetKeep:
		if m.To != nil && !m.To.same(cur.owner.Type, cur.owner.ID) {
			return serialPlan{}, fmt.Errorf("%w: %s does not move the unit", ErrInvalidMovement, m.Type)
		}
		plan.toOwner = cur.owner
	case targetRestore:
		if prev.fromOwner == nil {
			return serialPlan{}, fmt.Errorf("%w: paired movement has no source", ErrTransitionNotAllowed)
		}
		if m.To != nil && !m.To.same(prev.fromOwner.Type, prev.fromOwner.ID) {
			return serialPlan{}, fmt.Errorf("%w: restore goes back to %s %d",
				ErrOwnerMismatch, prev.fromOwner.Type, prev.fromOwner.ID)
		}
		plan.toOwner = *prev.fromOwner
		plan.toStatus = prev.fromStatus
	case targetTrash:
		holder := cur.issuerOrg
		if cur.hasState {
			holder = cur.owner.OrgID
		}
		trash := Owner{Type: OwnerTrash, ID: holder, OrgID: holder}
		if m.To != nil && !m.To.same(trash.Type, trash.ID) {
			return serialPlan{}, fmt.Errorf("%w: %s goes to the holder's trash", ErrOwnerNotAllowed, m.Type)
		}
		plan.toOwner = trash
	}
	if plan.toStatus == "" {
		plan.toStatus = cur.status
	}
	plan.ownerChanged = !cur.hasState || !plan.toOwner.same(cur.owner.Type, cur.owner.ID)

	delta, err := serialMeters(m, cur)
	if err != nil {
		return serialPlan{}, err
	}
	plan.metersDelta = delta
	plan.quantityDelta = b2i(counted(plan.toStatus)) - b2i(cur.hasState && counted(cur.status))
	return plan, nil
}

// serialMeters returns the change of remaining meters (centimeters).
func serialMeters(m Movement, cur serialCurrent) (int64, error) {
	cm := m.Centimeters
	if !cur.isRoll {
		if cm != 0 {
			return 0, fmt.Errorf("%w: meters on a piece unit", ErrInvalidMovement)
		}
		return 0, nil
	}
	switch m.Type {
	case TypePartialConsumption:
		switch {
		case cm <= 0:
			return 0, fmt.Errorf("%w: partial consumption needs meters > 0", ErrInvalidMovement)
		case cm > cur.remaining:
			return 0, fmt.Errorf("%w: %s m left, %s m requested",
				ErrInsufficientMeters, FormatMeters(cur.remaining), FormatMeters(cm))
		case cm == cur.remaining:
			return 0, fmt.Errorf("%w: the rest of the roll is a consumption", ErrInvalidMovement)
		}
		return -cm, nil
	case TypeCountAdjustment:
		switch {
		case cm == 0:
			return 0, fmt.Errorf("%w: count adjustment needs a meter difference", ErrInvalidMovement)
		case cur.remaining+cm < 0:
			return 0, fmt.Errorf("%w: %s m left, adjustment %s m",
				ErrInsufficientMeters, FormatMeters(cur.remaining), FormatMeters(cm))
		case cur.remaining+cm > cur.initial:
			return 0, fmt.Errorf("%w: roll cannot exceed its initial %s m", ErrInvalidMovement, FormatMeters(cur.initial))
		}
		return cm, nil
	case TypeReturn:
		if cm <= 0 || cur.remaining+cm > cur.initial {
			return 0, fmt.Errorf("%w: a returned roll needs 0 < meters <= %s",
				ErrInvalidMovement, FormatMeters(cur.initial-cur.remaining))
		}
		return cm, nil
	case TypeConsumption:
		if cm != 0 {
			return 0, fmt.Errorf("%w: consumption uses the whole roll", ErrInvalidMovement)
		}
		return -cur.remaining, nil
	default:
		if cm != 0 {
			return 0, fmt.Errorf("%w: %s does not change meters", ErrInvalidMovement, m.Type)
		}
		return 0, nil
	}
}

// fixedRule: sign +1 credits Movement.To, -1 debits Movement.From, 0 is
// signed by Quantity (count_adjustment).
type fixedRule struct {
	sign       int
	unitStatus []Status
	holder     holderRule
	pairedOut  MovementType
	restore    bool
}

var fixedRules = map[MovementType]fixedRule{
	TypeEntry:                 {sign: 1, unitStatus: []Status{StatusReserved, StatusPrinted, StatusAvailable}, holder: holderCenter},
	TypeTransferOut:           {sign: -1},
	TypeTransferIn:            {sign: 1, pairedOut: TypeTransferOut},
	TypeTransferCancelRestore: {sign: 1, pairedOut: TypeTransferOut, restore: true},
	TypeOrderOut:              {sign: -1},
	TypeReceived:              {sign: 1, pairedOut: TypeOrderOut},
	TypeOrderCancelRestore:    {sign: 1, pairedOut: TypeOrderOut, restore: true},
	TypeConsumption:           {sign: -1},
	TypeReturn:                {sign: 1},
	TypeCountAdjustment:       {sign: 0},
	TypeVoid:                  {sign: -1},
	TypeExternalOutbound:      {sign: -1},
	// placement / partial_consumption: a fixed barcode moves between owners
	// as an out + in pair (decision 1) and has no meters.
}

type fixedPlan struct {
	owner     Owner // the one owner touched
	delta     int32 // signed quantity change at owner
	credit    bool
	holder    holderRule
	pairedOut MovementType
	restore   bool
	setStatus Status // units.status after ("" unchanged)
}

func planFixed(m Movement, unitStatus Status) (fixedPlan, error) {
	rule, ok := fixedRules[m.Type]
	if !ok {
		return fixedPlan{}, fmt.Errorf("%w: %s for a fixed barcode", ErrTransitionNotAllowed, m.Type)
	}
	allowed := rule.unitStatus
	if allowed == nil {
		allowed = []Status{StatusAvailable}
	}
	if !slices.Contains(allowed, unitStatus) {
		return fixedPlan{}, fmt.Errorf("%w: %s from status %s", ErrTransitionNotAllowed, m.Type, unitStatus)
	}
	if m.Centimeters != 0 {
		return fixedPlan{}, fmt.Errorf("%w: meters on a fixed barcode", ErrInvalidMovement)
	}
	q := m.Quantity
	if rule.sign != 0 && q <= 0 {
		return fixedPlan{}, fmt.Errorf("%w: %s needs quantity > 0", ErrInvalidMovement, m.Type)
	}
	if q == 0 {
		return fixedPlan{}, fmt.Errorf("%w: count adjustment needs a quantity difference", ErrInvalidMovement)
	}
	credit := rule.sign > 0 || (rule.sign == 0 && q > 0)
	plan := fixedPlan{credit: credit, holder: rule.holder, pairedOut: rule.pairedOut, restore: rule.restore}
	var o *Owner
	if credit {
		o, plan.delta = m.To, abs32(q)
	} else {
		o, plan.delta = m.From, -abs32(q)
	}
	if o == nil {
		side := "source (From)"
		if credit {
			side = "target (To)"
		}
		return fixedPlan{}, fmt.Errorf("%w: %s needs a %s owner", ErrInvalidMovement, m.Type, side)
	}
	if !slices.Contains(stockOwners, o.Type) {
		return fixedPlan{}, fmt.Errorf("%w: fixed stock is held by %s", ErrOwnerNotAllowed, o.Type)
	}
	if err := validOwner(*o); err != nil {
		return fixedPlan{}, err
	}
	plan.owner = normalized(*o)
	if unitStatus != StatusAvailable {
		plan.setStatus = StatusAvailable
	}
	return plan, nil
}

func validOwner(o Owner) error {
	if o.ID <= 0 {
		return fmt.Errorf("%w: owner id", ErrInvalidMovement)
	}
	switch o.Type {
	case OwnerWarehouseLocation, OwnerService:
		if o.OrgID <= 0 {
			return fmt.Errorf("%w: %s owner needs its organization", ErrInvalidMovement, o.Type)
		}
	case OwnerOrganization, OwnerTrash:
		if o.OrgID != 0 && o.OrgID != o.ID {
			return fmt.Errorf("%w: %s owner is its own holder", ErrInvalidMovement, o.Type)
		}
	default:
		return fmt.Errorf("%w: owner type %q", ErrInvalidMovement, o.Type)
	}
	return nil
}

// normalized fills OrgID for organization and trash owners.
func normalized(o Owner) Owner {
	o.OrgID = o.holder()
	return o
}

func b2i(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}
