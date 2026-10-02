package ledger

import (
	"encoding/json"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

// Replay rebuilds the projections from the recorded movements (TEC-156). It
// is pure: no database, no clock. The movement rows already carry the
// outcome Post planned (to_owner, to_status, quantity_delta, meters_delta,
// organization_id), so Replay re-applies them with the same projection
// rules as Post (counted statuses, owner holders, the per-owner deltas of
// applyProjections) instead of re-validating the transitions; a break in a
// unit's chain (from_status / from_owner not equal to the replayed state)
// is reported as an anomaly.
//
// Reclassification (TEC-157): the movement keeps owner and status and
// carries the new product (product_id) and the old one
// (metadata.from_product_id). Replay follows the product along the chain;
// the unit's stock counts under the product of its last movement, which
// must equal units.product_id (else an anomaly).
//
// Split (TEC-184): the source roll's movement is an ordinary meter change
// (owner kept, used at 0 m) and the new unit's first movement places it at
// the same owner with remaining = initial; trackSplits checks both shapes
// and SplitAnomalies that the meters the roll lost equal the new unit's.

// Projection is the expected state of every projection.
type Projection struct {
	Units     map[int64]*UnitProjection
	OrgStocks map[OrgStockKey]StockAmount
	BinStocks map[BinStockKey]BinStockAmount
	// splits: both sides of each split seen so far (TEC-184).
	splits map[int64]*splitPair
}

// OrgStockKey keys organization_product_stocks.
type OrgStockKey struct{ OrganizationID, ProductID int64 }

// BinStockKey keys bin_product_stocks.
type BinStockKey struct{ LocationID, ProductID int64 }

// StockAmount is an expected product stock row.
type StockAmount struct {
	BrandID     int64
	Quantity    int64
	Centimeters int64
}

// BinStockAmount is an expected bin stock row; OrganizationID is the
// location's organization.
type BinStockAmount struct {
	OrganizationID int64
	StockAmount
}

// UnitProjection is the expected projection of one unit.
type UnitProjection struct {
	Unit      db.Unit
	Movements int
	// ProductID is the product of the unit after its last movement
	// (units.product_id; reclassification changes it). Without movements
	// it is the unit's own product.
	ProductID int64
	// State is unit_current_state of a serial unit; nil before its first
	// movement (and always nil for fixed barcodes).
	State *UnitState
	// Holdings are the fixed_barcode_holdings rows of a fixed barcode, in
	// first-touch order.
	Holdings []FixedHolding
	// Status is the expected units.status; "" when no movement decides it
	// (a reserved or printed label keeps its own status).
	Status Status
	// RemainingCm is the expected units.remaining_meters of a roll.
	RemainingCm *int64
	// Anomalies are chain breaks found while replaying.
	Anomalies []string
}

// UnitState is an expected unit_current_state row.
type UnitState struct {
	Owner          Owner // OrgID = holder_org_id
	Status         Status
	LastMovementID int64
}

// FixedHolding is an expected fixed_barcode_holdings row.
type FixedHolding struct {
	Owner          Owner // OrgID = holder_org_id
	Quantity       int64
	LastMovementID int64
}

// NewProjection returns an empty projection.
func NewProjection() *Projection {
	return &Projection{
		Units:     map[int64]*UnitProjection{},
		OrgStocks: map[OrgStockKey]StockAmount{},
		BinStocks: map[BinStockKey]BinStockAmount{},
	}
}

// Replay applies the movements of unit u (chronological, all of the unit's
// movements) and adds the unit's product stock contribution.
func (p *Projection) Replay(u db.Unit, mvs []db.StockMovement) (*UnitProjection, error) {
	up := &UnitProjection{Unit: u, Movements: len(mvs)}
	for _, mv := range mvs {
		if mv.UnitID != u.ID {
			return nil, fmt.Errorf("ledger: replay: movement %d is not of unit %d", mv.ID, u.ID)
		}
	}
	up.ProductID = up.replayProduct(mvs)
	var d deltas
	var err error
	if u.UnitKind == KindFixed {
		err = up.replayFixed(mvs, &d)
	} else {
		err = up.replaySerial(mvs, &d)
	}
	if err != nil {
		return nil, err
	}
	p.trackSplits(u, up, mvs)
	p.Units[u.ID] = up
	for k, v := range d.bin {
		key := BinStockKey{LocationID: k[0], ProductID: up.ProductID}
		cur := p.BinStocks[key]
		cur.OrganizationID, cur.BrandID = k[1], u.BrandID
		cur.Quantity += int64(v.qty)
		cur.Centimeters += v.cm
		p.BinStocks[key] = cur
	}
	for org, v := range d.org {
		key := OrgStockKey{OrganizationID: org, ProductID: up.ProductID}
		cur := p.OrgStocks[key]
		cur.BrandID = u.BrandID
		cur.Quantity += int64(v.qty)
		cur.Centimeters += v.cm
		p.OrgStocks[key] = cur
	}
	return up, nil
}

func (up *UnitProjection) anomaly(format string, args ...any) {
	up.Anomalies = append(up.Anomalies, fmt.Sprintf(format, args...))
}

// replayProduct follows the unit's product through its movements: every
// movement is recorded on the product the unit has at that time, a
// reclassification moves it from metadata.from_product_id to product_id.
func (up *UnitProjection) replayProduct(mvs []db.StockMovement) int64 {
	if len(mvs) == 0 {
		return up.Unit.ProductID
	}
	var product int64
	for i := range mvs {
		mv := &mvs[i]
		from := mv.ProductID
		if MovementType(mv.Type) == TypeReclassification {
			var meta struct {
				FromProductID int64 `json:"from_product_id"`
			}
			if err := json.Unmarshal(mv.Metadata, &meta); err != nil || meta.FromProductID == 0 {
				up.anomaly("movement %d (%s): no from_product_id", mv.ID, mv.Type)
				from = product // unknown source: reported once
			} else {
				from = meta.FromProductID
			}
			if mv.QuantityDelta != 0 || mustCm(mv.MetersDelta) != 0 {
				up.anomaly("movement %d (%s): reclassification with a quantity or meters", mv.ID, mv.Type)
			}
			if mv.FromOwnerType != mv.ToOwnerType || mv.FromOwnerID != mv.ToOwnerID || mv.FromStatus != mv.ToStatus {
				up.anomaly("movement %d (%s): reclassification changes owner or status", mv.ID, mv.Type)
			}
		}
		if product != 0 && from != product {
			up.anomaly("movement %d (%s): product %d, replayed %d", mv.ID, mv.Type, from, product)
		}
		product = mv.ProductID
	}
	if product != up.Unit.ProductID {
		up.anomaly("units.product_id %d, ledger product %d", up.Unit.ProductID, product)
	}
	return product
}

func (up *UnitProjection) replaySerial(mvs []db.StockMovement, d *deltas) error {
	isRoll := up.Unit.InitialMeters.Valid
	var initial, remaining int64
	if isRoll {
		var err error
		if initial, err = numericToCm(up.Unit.InitialMeters); err != nil {
			return err
		}
		remaining = initial
	}
	var state *UnitState
	var last *db.StockMovement
	for i := range mvs {
		mv := &mvs[i]
		if state != nil {
			if Status(mv.FromStatus.String) != state.Status {
				up.anomaly("movement %d (%s): from_status %q, replayed %q", mv.ID, mv.Type, mv.FromStatus.String, state.Status)
			}
			if !state.Owner.same(OwnerType(mv.FromOwnerType.String), mv.FromOwnerID.Int64) {
				up.anomaly("movement %d (%s): from_owner %s %d, replayed %s %d", mv.ID, mv.Type,
					mv.FromOwnerType.String, mv.FromOwnerID.Int64, state.Owner.Type, state.Owner.ID)
			}
		} else if mv.FromOwnerType.Valid {
			up.anomaly("movement %d (%s): from_owner %s %d before any owner", mv.ID, mv.Type,
				mv.FromOwnerType.String, mv.FromOwnerID.Int64)
		}

		next := UnitState{LastMovementID: mv.ID}
		if state != nil {
			next.Owner, next.Status = state.Owner, state.Status
		}
		if mv.ToOwnerType.Valid {
			next.Owner = Owner{Type: OwnerType(mv.ToOwnerType.String), ID: mv.ToOwnerID.Int64}
			next.Owner.OrgID = replayHolder(mv, next.Owner, state, last)
		} else {
			up.anomaly("movement %d (%s): no to_owner", mv.ID, mv.Type)
		}
		if mv.ToStatus.Valid {
			next.Status = Status(mv.ToStatus.String)
		} else {
			up.anomaly("movement %d (%s): no to_status", mv.ID, mv.Type)
		}
		if state == nil && next.Owner.Type == "" {
			continue
		}
		cm, err := numericToCm(mv.MetersDelta)
		if err != nil {
			return fmt.Errorf("ledger: replay movement %d: %w", mv.ID, err)
		}
		if cm != 0 && !isRoll {
			up.anomaly("movement %d (%s): meters on a piece unit", mv.ID, mv.Type)
		}
		remaining += cm
		if isRoll && (remaining < 0 || remaining > initial) {
			up.anomaly("movement %d (%s): remaining %s m outside 0..%s m", mv.ID, mv.Type,
				FormatMeters(remaining), FormatMeters(initial))
		}
		state, last = &next, mv
	}
	if state == nil {
		return nil
	}
	up.State, up.Status = state, state.Status
	if isRoll {
		r := remaining
		up.RemainingCm = &r
	}
	// Post keeps the product stock equal to the counted units at their
	// owner: the final contribution of the unit.
	if counted(state.Status) {
		d.add(state.Owner, 1, remaining)
	}
	return nil
}

// replayHolder is the holding organization after a serial movement, as Post
// chose it: organization and trash owners are their own holder; the first
// movement is recorded on its holder (entry); a cancel restore returns to
// the holder of the paired out's source, which is that out's organization;
// every other owner stays with the current holder (placement, transfer_in,
// received, consumption, return, partial consumption, count adjustment).
func replayHolder(mv *db.StockMovement, to Owner, state *UnitState, last *db.StockMovement) int64 {
	switch {
	case to.Type == OwnerOrganization || to.Type == OwnerTrash:
		return to.ID
	case state == nil:
		return mv.OrganizationID
	case serialRules[MovementType(mv.Type)].target == targetRestore && last != nil:
		return last.OrganizationID
	default:
		return state.Owner.OrgID
	}
}

func (up *UnitProjection) replayFixed(mvs []db.StockMovement, d *deltas) error {
	type ownerKey struct {
		t  string
		id int64
	}
	index := map[ownerKey]int{}
	for i := range mvs {
		mv := &mvs[i]
		var t pgtype.Text
		var id pgtype.Int8
		switch {
		case mv.QuantityDelta > 0: // credit: to_owner
			t, id = mv.ToOwnerType, mv.ToOwnerID
		case mv.QuantityDelta < 0: // debit: from_owner
			t, id = mv.FromOwnerType, mv.FromOwnerID
		default:
			up.anomaly("movement %d (%s): fixed barcode movement without quantity", mv.ID, mv.Type)
			continue
		}
		if !t.Valid {
			up.anomaly("movement %d (%s): fixed barcode movement without its owner", mv.ID, mv.Type)
			continue
		}
		// The movement is recorded on the holder of the touched owner.
		// Post sets holder_org_id when it creates the row (Ensure) only.
		o := normalized(Owner{Type: OwnerType(t.String), ID: id.Int64, OrgID: mv.OrganizationID})
		key := ownerKey{t.String, id.Int64}
		pos, ok := index[key]
		if !ok {
			pos = len(up.Holdings)
			index[key] = pos
			up.Holdings = append(up.Holdings, FixedHolding{Owner: o})
		}
		h := &up.Holdings[pos]
		h.Quantity += int64(mv.QuantityDelta)
		h.LastMovementID = mv.ID
		if h.Quantity < 0 {
			up.anomaly("movement %d (%s): %s %d holds %d", mv.ID, mv.Type, o.Type, o.ID, h.Quantity)
		}
		// Every movement of a fixed barcode leaves units.status available
		// (planFixed.setStatus).
		up.Status = StatusAvailable
	}
	for _, h := range up.Holdings {
		d.add(h.Owner, int32(h.Quantity), 0)
	}
	return nil
}

// NumericToCentimeters converts a NUMERIC meter value to centimeters (NULL
// is 0).
func NumericToCentimeters(n pgtype.Numeric) (int64, error) { return numericToCm(n) }

// CentimetersToNumeric converts centimeters to a NUMERIC(…,2).
func CentimetersToNumeric(cm int64) pgtype.Numeric { return cmToNumeric(cm) }
