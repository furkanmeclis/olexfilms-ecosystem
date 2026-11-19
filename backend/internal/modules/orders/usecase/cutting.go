package usecase

// Cutting a meter line (TEC-1241, kutu/metraj 4/7; kararlar TEC-1236 K5,
// K6, K7, K11, K12).
//
// A meter line of a piece product (sale_measure meter, line_credits set) is
// picked by cutting one piece of the line's length:
//
//   - from an open roll of the product (a box opened earlier): the piece is
//     split off with its own barcode (ledger.Split); the whole remainder of
//     the roll goes without a cut;
//   - from a closed box: the box is opened (ledger.ConvertMeasure,
//     reason box_opened, the product's box length) and cut in the same
//     transaction (K5). The client confirms the opening (open_box); the
//     preview names the box, the cut and what stays as an open roll.
//
// A reserved box, a box on another order or on an open warehouse transfer
// is refused before anything is opened (checkSerialStock under the unit
// row lock). The minimum cut (products.min_cut_meters) applies to every cut
// except taking the whole remainder.
//
// K7 (TEC-1250, 19.11): a piece that completes the line is not cut when
// what would stay is shorter than the minimum cut: the whole roll (or box)
// goes to the line and the extra meters go free (excess). The buyer pays
// the line's meters; invoices, the account and returns use the line's
// meters, the ledger moves the roll's real meters. Only a partial piece the
// warehouse typed shorter can still leave a remnant (the preview warns).
//
// One piece covers the line by default. A shorter piece is a partial piece
// ("parçalı gönder", K11): the client confirms it (partial) with a note.
// The line's credits are spread over its pieces: a partial piece carries
// its meter share rounded (or the credits the warehouse typed), the piece
// completing the line the rest; the pieces never carry more than the line
// (order_item_units.credits; unit_credit_default reads it). The plotter
// mark of the line shares the credits (K12).

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Cutting refusals; the handler maps them to 409 / 422.
var (
	// ErrBoxOpenConfirm: a closed box is opened only with open_box.
	ErrBoxOpenConfirm = errors.New("orders: opening the closed box needs confirmation")
	// ErrPartialPieceConfirm: a piece shorter than the rest of the line
	// needs partial and a note (K11).
	ErrPartialPieceConfirm = errors.New("orders: a partial piece needs confirmation")
	// ErrCutBelowMinCut: the cut is shorter than the product's minimum cut
	// and is not the whole remainder.
	ErrCutBelowMinCut = errors.New("orders: the cut is shorter than the minimum cut")
	// ErrNotCutLine: the line is not a meter line of a piece product.
	ErrNotCutLine = errors.New("orders: the line is not cut from a box")
)

// Validation codes of cutting.
const (
	CodeBoxLengthUnknown = "BOX_LENGTH_UNKNOWN"
	CodeNoteRequired     = "NOTE_REQUIRED"
	CodeCreditsExceed    = "CREDITS_EXCEED_LINE"
	CodeNotCuttable      = "UNIT_NOT_CUTTABLE"
)

// MaxPieceNoteLen bounds the note of a partial piece.
const MaxPieceNoteLen = 500

// ActionBoxOpened is the order timeline step of a box opened for a line.
const ActionBoxOpened = "order.box_opened"

// BoxOpenedReason is the ledger reason (and metadata reason) of a box
// opened while picking.
const BoxOpenedReason = "box_opened"

// isCutLine: a meter line of a piece product (line credits set, TEC-1238).
func isCutLine(it db.OrderItem) bool { return it.Meters.Valid && it.LineCredits.Valid }

// CutPreviewView describes what confirming the scanned unit does on a cut
// line; nothing is written.
type CutPreviewView struct {
	ItemUUID uuid.UUID `json:"item_uuid"`
	UnitUUID uuid.UUID `json:"unit_uuid"`
	Barcode  string    `json:"barcode"`
	// ClosedBox: the unit is a closed box; it is opened (open_box).
	ClosedBox bool `json:"closed_box"`
	// SourceMeters is the box length or the roll's remaining meters;
	// CutMeters the piece; RemainingMeters what stays in stock.
	SourceMeters    string `json:"source_meters"`
	CutMeters       string `json:"cut_meters"`
	RemainingMeters string `json:"remaining_meters"`
	// Whole: the whole roll (or box) goes to the line; no new barcode.
	Whole bool `json:"whole"`
	// Remnant: the remainder is shorter than the minimum cut (only a
	// partial piece leaves one, K7).
	Remnant bool `json:"remnant"`
	// ExcessMeters (TEC-1250, K7): the meters over the line that go with
	// the whole roll free of charge; "0.00" when the piece fits the line.
	ExcessMeters string  `json:"excess_meters"`
	MinCutMeters *string `json:"min_cut_meters"`
	// MissingMeters is what the line still needs; Partial is true when the
	// piece is shorter (partial and a note are required).
	MissingMeters string `json:"missing_meters"`
	Partial       bool   `json:"partial"`
	// Credits the piece carries; LineCredits the line's credits.
	Credits      int32   `json:"credits"`
	LineCredits  int32   `json:"line_credits"`
	PlotterCut   bool    `json:"plotter_cut"`
	LocationCode *string `json:"location_code"`
}

// cutPlan is the checked outcome of a cut, shared by the preview and the
// assignment.
type cutPlan struct {
	product              db.Product
	closedBox, whole     bool
	sourceCm, cutCm      int64
	missingCm, excessCm  int64
	partial, remnant     bool
	credits              int32
	explicitPieceCredits bool
	note                 string
	locationCode         *string
}

func ratCm(r *big.Rat) int64 {
	cm := new(big.Rat).Mul(r, big.NewRat(100, 1))
	n := new(big.Int).Quo(cm.Num(), cm.Denom())
	return n.Int64()
}

func cmString(cm int64) string { return big.NewRat(cm, 100).FloatString(2) }

// planCut checks a cut of unit u for the cut line it. The caller holds the
// line and unit row locks.
func (s *Service) planCut(ctx context.Context, q *db.Queries, o db.Order, it db.OrderItem, u db.Unit, in AssignInput) (cutPlan, error) {
	var p cutPlan
	if u.UnitKind != ledger.KindSerial {
		return p, &ValidationError{Field: "barcode", Code: CodeNotCuttable, Message: "a meter line is cut from a roll or a closed box"}
	}
	if in.Quantity != nil {
		return p, invalid("quantity", "a meter line is assigned in meters")
	}
	prod, err := q.GetProduct(ctx, db.GetProductParams{ID: it.ProductID, BrandID: o.BrandID})
	if err != nil {
		return p, fmt.Errorf("orders: product: %w", err)
	}
	p.product = prod
	required, err := q.OrganizationRequiresPlacement(ctx, o.SellerOrgID)
	if err != nil {
		return p, err
	}
	if required && u.Status != string(ledger.StatusPlaced) {
		return p, ErrLocationPlacementRequired
	}
	// Reserved, on another order or on an open transfer: refused before a
	// box is opened.
	if err := checkSerialStock(ctx, q, u, o.SellerOrgID); err != nil {
		return p, err
	}
	switch {
	case u.RemainingMeters.Valid:
		p.sourceCm, err = ledger.NumericToCentimeters(u.RemainingMeters)
		if err != nil || p.sourceCm <= 0 {
			return p, ErrUnitNotAvailable
		}
	case u.InitialMeters.Valid:
		return p, ErrUnitNotAvailable
	default:
		if !prod.DefaultRollMeters.Valid {
			return p, &ValidationError{Field: "barcode", Code: CodeBoxLengthUnknown, Message: "the product has no box length"}
		}
		p.closedBox = true
		p.sourceCm, err = ledger.NumericToCentimeters(prod.DefaultRollMeters)
		if err != nil || p.sourceCm <= 0 {
			return p, &ValidationError{Field: "barcode", Code: CodeBoxLengthUnknown, Message: "the product has no box length"}
		}
	}
	done, err := reservedOnLine(ctx, q, it)
	if err != nil {
		return p, err
	}
	p.missingCm = ratCm(new(big.Rat).Sub(lineAmount(it), done))
	if p.missingCm <= 0 {
		return p, ErrOverAssigned
	}
	p.cutCm = min(p.missingCm, p.sourceCm)
	if in.Meters != nil {
		m, ok := normalizeMeters(*in.Meters)
		if !ok {
			return p, invalid("meters", "meters must be a positive number with at most two decimals")
		}
		want, _ := parseRat(m)
		p.cutCm = ratCm(want)
		if p.cutCm > p.sourceCm {
			return p, fmt.Errorf("%w: %s m left", ErrRollMetersExceed, cmString(p.sourceCm))
		}
		// The whole roll may be typed when the rest after the line would
		// be shorter than the minimum cut (K7).
		if p.cutCm > p.missingCm && (p.cutCm != p.sourceCm || !ledger.IsRemnant(cmToNumeric(p.sourceCm-p.missingCm), prod.MinCutMeters)) {
			return p, ErrOverAssigned
		}
	}
	// K7 (TEC-1250): the piece completing the line takes the whole roll
	// when the rest would be shorter than the minimum cut; the extra meters
	// go free.
	if p.cutCm >= p.missingCm && p.sourceCm > p.missingCm &&
		ledger.IsRemnant(cmToNumeric(p.sourceCm-p.missingCm), prod.MinCutMeters) {
		p.cutCm = p.sourceCm
		p.excessCm = p.sourceCm - p.missingCm
	}
	p.whole = p.cutCm == p.sourceCm
	if err := ledger.CheckMinCut(p.cutCm, p.sourceCm, prod.MinCutMeters); err != nil {
		return p, ErrCutBelowMinCut
	}
	rest := p.sourceCm - p.cutCm
	if rest > 0 {
		p.remnant = ledger.IsRemnant(cmToNumeric(rest), prod.MinCutMeters)
	}
	p.partial = p.cutCm < p.missingCm
	p.note = strings.TrimSpace(in.Note)
	if len(p.note) > MaxPieceNoteLen {
		return p, invalid("note", fmt.Sprintf("at most %d characters", MaxPieceNoteLen))
	}
	if err := s.planCredits(ctx, q, it, &p, in.Credits); err != nil {
		return p, err
	}
	if st, err := q.GetUnitCurrentState(ctx, u.ID); err == nil && st.OwnerType == string(ledger.OwnerWarehouseLocation) {
		if loc, err := q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: st.OwnerID, OrganizationID: o.SellerOrgID}); err == nil {
			code := loc.FullCode.String
			if code == "" {
				code = loc.Code
			}
			p.locationCode = &code
		}
	}
	return p, nil
}

// planCredits spreads the line's credits: a single piece carries the line
// (NULL on the assignment), a partial piece its rounded meter share (or the
// typed credits), the completing piece the rest.
func (s *Service) planCredits(ctx context.Context, q *db.Queries, it db.OrderItem, p *cutPlan, typed *int64) error {
	line := it.LineCredits.Int32
	rows, err := q.ListOrderItemUnitsByItem(ctx, it.ID)
	if err != nil {
		return fmt.Errorf("orders: line pieces: %w", err)
	}
	used := int32(0)
	for _, r := range rows {
		if !r.ReservationID.Valid {
			continue
		}
		if r.Credits.Valid {
			used += r.Credits.Int32
		} else {
			used += line
		}
	}
	avail := max(line-used, 0)
	single := len(rows) == 0 && !p.partial
	switch {
	case typed != nil:
		if *typed < 0 || *typed > int64(avail) {
			return &ValidationError{Field: "credits", Code: CodeCreditsExceed,
				Message: fmt.Sprintf("the pieces of the line carry at most %d credits; %d left", line, avail)}
		}
		p.credits, p.explicitPieceCredits = int32(*typed), true
	case single:
		p.credits = line
	case !p.partial:
		p.credits, p.explicitPieceCredits = avail, true
	default:
		lineCm := ratCm(numericRat(it.Meters))
		share := int32(0)
		if lineCm > 0 {
			// Rounded half up: line × cut / line meters.
			num := int64(line)*p.cutCm*2 + lineCm
			share = int32(num / (2 * lineCm))
		}
		p.credits, p.explicitPieceCredits = min(share, avail), true
	}
	return nil
}

func cmToNumeric(cm int64) pgtype.Numeric {
	n, _ := numeric(cmString(cm))
	return n
}

// CutPreview answers what assigning the scanned unit to a cut line does
// (seller, orders.ship, preparing). Nothing is written.
func (s *Service) CutPreview(ctx context.Context, c Caller, orderUUID, itemUUID uuid.UUID, in AssignInput) (CutPreviewView, error) {
	var out CutPreviewView
	err := s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		o, it, err := s.lockAssignable(ctx, q, c, orderUUID, itemUUID)
		if err != nil {
			return err
		}
		if !isCutLine(it) {
			return ErrNotCutLine
		}
		u, err := findUnit(ctx, q, o.BrandID, in)
		if err != nil {
			return err
		}
		if u, err = q.LockUnit(ctx, u.ID); err != nil {
			return fmt.Errorf("orders: lock unit: %w", err)
		}
		if u.ProductID != it.ProductID {
			return invalid("barcode", "the unit is not of the line's product")
		}
		if _, err := q.GetOrderItemUnit(ctx, db.GetOrderItemUnitParams{OrderItemID: it.ID, UnitID: u.ID}); err == nil {
			return ErrUnitAlreadyAssigned
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("orders: assignment: %w", err)
		}
		p, err := s.planCut(ctx, q, o, it, u, in)
		if err != nil {
			return err
		}
		out = CutPreviewView{
			ItemUUID: it.Uuid, UnitUUID: u.Uuid, Barcode: u.Barcode, ClosedBox: p.closedBox,
			SourceMeters: cmString(p.sourceCm), CutMeters: cmString(p.cutCm),
			RemainingMeters: cmString(p.sourceCm - p.cutCm), Whole: p.whole, Remnant: p.remnant,
			ExcessMeters: cmString(p.excessCm),
			MinCutMeters: numericTextPtr(p.product.MinCutMeters, 2), MissingMeters: cmString(p.missingCm),
			Partial: p.partial, Credits: p.credits, LineCredits: it.LineCredits.Int32,
			PlotterCut: it.PlotterCut, LocationCode: p.locationCode,
		}
		return nil
	})
	return out, err
}

// assignCutLocked assigns a piece cut from u to the cut line it: the box is
// opened when needed, the piece split off, reserved and assigned with its
// credits. The caller holds the line and unit row locks.
func (s *Service) assignCutLocked(ctx context.Context, q *db.Queries, tx pgx.Tx, c Caller, o db.Order, it db.OrderItem, u db.Unit, in AssignInput) (*SplitView, error) {
	splitKey, err := assignSplitKey(it, in.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	// A retried cut: the split already happened under the client's key.
	if in.IdempotencyKey != "" && u.RemainingMeters.Valid {
		prev, err := ledger.FindSplit(ctx, q, o.SellerOrgID, splitKey)
		if err != nil {
			return nil, err
		}
		if prev != nil {
			if prev.Split.SourceUnitID != u.ID {
				return nil, invalid("idempotency_key", "the key was used for another roll")
			}
			split := splitView(*prev)
			if _, err := q.GetOrderItemUnit(ctx, db.GetOrderItemUnitParams{OrderItemID: it.ID, UnitID: prev.New.ID}); err == nil {
				return split, nil
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("orders: assignment: %w", err)
			}
			return nil, ErrUnitNotAvailable
		}
	}
	p, err := s.planCut(ctx, q, o, it, u, in)
	if err != nil {
		return nil, err
	}
	if p.closedBox && !in.OpenBox {
		return nil, ErrBoxOpenConfirm
	}
	if p.partial {
		if !in.Partial {
			return nil, ErrPartialPieceConfirm
		}
		if p.note == "" {
			return nil, &ValidationError{Field: "note", Code: CodeNoteRequired, Message: "a partial piece needs a note"}
		}
	}
	meta := map[string]any{"order_uuid": o.Uuid.String(), "order_no": o.OrderNo}
	if p.closedBox {
		boxMeta := map[string]any{"order_uuid": o.Uuid.String(), "order_no": o.OrderNo, "reason": BoxOpenedReason}
		if _, err := s.ledger.ConvertMeasure(ctx, tx, ledger.MeasureConversion{
			UnitID: u.ID, Centimeters: p.sourceCm, HolderOrgID: o.SellerOrgID,
			Source: LedgerSource, RefType: SplitRefType, RefID: it.ID, ActorUserID: actorOf(c),
			Reason: BoxOpenedReason, Metadata: boxMeta,
		}); err != nil {
			return nil, convertErr(err)
		}
		if u, err = q.LockUnit(ctx, u.ID); err != nil {
			return nil, fmt.Errorf("orders: lock unit: %w", err)
		}
		m := cmString(p.sourceCm)
		if err := s.recordActivity(ctx, q, c, o, ActionBoxOpened, activityPayload{
			ItemUUID: it.Uuid.String(), UnitUUID: u.Uuid.String(), Barcode: u.Barcode, Meters: &m,
		}); err != nil {
			return nil, err
		}
	}
	var split *SplitView
	if !p.whole {
		res, err := s.ledger.Split(ctx, tx, ledger.SplitCommand{
			SourceUnitID: u.ID, Centimeters: p.cutCm, HolderOrgID: o.SellerOrgID,
			IdempotencyKey: splitKey, RefType: SplitRefType, RefID: it.ID, ActorUserID: actorOf(c),
			Metadata: meta,
		})
		if err != nil {
			return nil, splitErr(u.Barcode, err)
		}
		split = splitView(res)
		split.SourceRemnant = p.remnant
		split.BoxOpened = p.closedBox
		if u, err = q.LockUnit(ctx, res.New.ID); err != nil {
			return nil, fmt.Errorf("orders: lock unit: %w", err)
		}
	}
	meters := u.RemainingMeters
	var credits pgtype.Int4
	if p.explicitPieceCredits {
		credits = pgtype.Int4{Int32: p.credits, Valid: true}
	}
	var note pgtype.Text
	if p.note != "" {
		note = pgtype.Text{String: p.note, Valid: true}
	}
	res, err := q.CreateStockReservation(ctx, db.CreateStockReservationParams{
		Meters: meters, CreatedByUserID: c.actor(), UnitID: u.ID, OrderItemID: it.ID,
	})
	if isUniqueViolation(err) {
		return nil, ErrUnitReserved
	}
	if err != nil {
		return nil, fmt.Errorf("orders: reserve: %w", err)
	}
	if _, err := q.CreateOrderItemUnit(ctx, db.CreateOrderItemUnitParams{
		UnitID: u.ID, Meters: meters, OrderItemID: it.ID,
		ReservationID: pgtype.Int8{Int64: res.ID, Valid: true}, AssignedByUserID: c.actor(),
		Credits: credits, PieceNote: note,
	}); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrUnitAlreadyAssigned
		}
		return nil, fmt.Errorf("orders: assign: %w", err)
	}
	return split, nil
}

// convertErr maps a box opening refusal of the order flow.
func convertErr(err error) error {
	switch {
	case errors.Is(err, ledger.ErrConvertNotOnHand), errors.Is(err, ledger.ErrConvertAlreadyRoll),
		errors.Is(err, ledger.ErrConvertFixedBarcode):
		return ErrUnitNotAvailable
	}
	return fmt.Errorf("orders: open box: %w", err)
}

// DetachedPieceView is a piece cut for the order that is no longer on its
// line (the order was cancelled or the piece unassigned): it stays in
// stock as a separate unit.
type DetachedPieceView struct {
	UnitUUID        uuid.UUID `json:"unit_uuid"`
	Barcode         string    `json:"barcode"`
	CutMeters       string    `json:"cut_meters"`
	RemainingMeters *string   `json:"remaining_meters"`
}

func (s *Service) detachedPieces(ctx context.Context, q *db.Queries, o db.Order) ([]DetachedPieceView, error) {
	rows, err := q.ListOrderDetachedCutPieces(ctx, o.ID)
	if err != nil {
		return nil, fmt.Errorf("orders: detached pieces: %w", err)
	}
	out := make([]DetachedPieceView, 0, len(rows))
	for _, r := range rows {
		out = append(out, DetachedPieceView{
			UnitUUID: r.Uuid, Barcode: r.Barcode, CutMeters: ratString(numericRat(r.CutMeters)),
			RemainingMeters: numericTextPtr(r.RemainingMeters, 2),
		})
	}
	return out, nil
}
