package usecase

// Warehouse transfer discrepancies (TEC-1227).
//
// The target warehouse completes a transfer by scanning what arrived. Lines
// it did not get are reported missing: they get no transfer_in, stay in
// transit (owned by the organization) and keep their unit lock (is_open)
// under a loss investigation. Scanned barcodes that are not on the
// transfer are reported surplus (recorded only, no stock movement). Both
// are rows of the transfer's discrepancy record (tutanak). An open missing
// row is resolved later:
//
//   - found: the unit arrived after all; transfer_in and a placement onto a
//     location of the target warehouse are posted (target membership).
//   - returned_to_source: the unit never left; transfer_cancel_restore puts
//     it back to its source location (either side).
//
// A surplus row is closed by either side. Writing a lost unit off is the
// stock adjustment document's job (TEC-1226).

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Transfer discrepancy kinds, statuses and resolutions
// (warehouse_transfer_discrepancies, migration 000185).
const (
	TransferDiscrepancyMissing = "missing"
	TransferDiscrepancySurplus = "surplus"

	TransferDiscrepancyOpen     = "open"
	TransferDiscrepancyResolved = "resolved"

	TransferResolutionFound    = "found"
	TransferResolutionReturned = "returned_to_source"
	TransferResolutionClosed   = "closed"

	// MaxTransferDiscrepancyNote bounds a note.
	MaxTransferDiscrepancyNote = 500
	// MaxTransferSurplus bounds the surplus barcodes of one completion.
	MaxTransferSurplus = 500

	AuditTransferDiscrepancyResolved = "warehouse.transfer.discrepancy_resolved"
)

// Transfer discrepancy errors.
var (
	ErrTransferDiscrepancyNotFound = errors.New("warehouse: transfer discrepancy not found")
	ErrTransferDiscrepancyClosed   = errors.New("warehouse: transfer discrepancy is already resolved")
)

// TransferMissingInput names a line that did not arrive.
type TransferMissingInput struct {
	LineUUID uuid.UUID `json:"line_uuid"`
	Note     string    `json:"note"`
}

// TransferSurplusInput is a scanned barcode that is not on the transfer.
type TransferSurplusInput struct {
	Barcode string `json:"barcode"`
	Note    string `json:"note"`
}

// ResolveTransferDiscrepancyInput closes a record row. A found unit goes to
// the line's target, LocationUUID/LocationCode or the transfer's default.
type ResolveTransferDiscrepancyInput struct {
	Resolution   string
	LocationUUID *uuid.UUID
	LocationCode string
	Note         *string
}

// TransferDiscrepancy is one row of a transfer's record. No prices.
type TransferDiscrepancy struct {
	UUID           uuid.UUID  `json:"uuid"`
	Kind           string     `json:"kind"`
	Status         string     `json:"status"`
	Barcode        string     `json:"barcode"`
	LineUUID       *uuid.UUID `json:"line_uuid"`
	UnitUUID       *uuid.UUID `json:"unit_uuid"`
	ProductName    *string    `json:"product_name"`
	ProductSKU     *string    `json:"product_sku"`
	Note           *string    `json:"note"`
	Resolution     *string    `json:"resolution"`
	ResolutionNote *string    `json:"resolution_note"`
	CreatedBy      *string    `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	ResolvedBy     *string    `json:"resolved_by"`
	ResolvedAt     *time.Time `json:"resolved_at"`
}

func noteText(field, note string) (pgtype.Text, error) {
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > MaxTransferDiscrepancyNote {
		return pgtype.Text{}, invalid(field, fmt.Sprintf("at most %d characters", MaxTransferDiscrepancyNote))
	}
	return pgtype.Text{String: note, Valid: note != ""}, nil
}

// missingLines matches the reported lines to the transfer; at least one
// line must arrive (otherwise the transfer is cancelled or left in transit).
func missingLines(lines []db.WarehouseTransferLine, in []TransferMissingInput) (map[int64]string, error) {
	out := map[int64]string{}
	if len(in) == 0 {
		return out, nil
	}
	byUUID := make(map[uuid.UUID]int64, len(lines))
	for _, l := range lines {
		byUUID[l.Uuid] = l.ID
	}
	for i, m := range in {
		field := fmt.Sprintf("missing[%d]", i)
		id, ok := byUUID[m.LineUUID]
		if !ok {
			return nil, invalid(field+".line_uuid", "the line is not on this transfer")
		}
		if _, dup := out[id]; dup {
			return nil, invalid(field+".line_uuid", "the line is reported twice")
		}
		if _, err := noteText(field+".note", m.Note); err != nil {
			return nil, err
		}
		out[id] = strings.TrimSpace(m.Note)
	}
	if len(out) >= len(lines) {
		return nil, invalid("missing", "at least one line must arrive; cancel the transfer or leave it in transit")
	}
	return out, nil
}

// receiveLine posts the transfer_in (and, for a serial unit, the placement)
// of one line onto target and stores the movements on the line.
func (s *WarehouseTransfers) receiveLine(ctx context.Context, q *db.Queries, tx pgx.Tx, c EntryCaller, org int64,
	t db.WarehouseTransfer, l db.WarehouseTransferLine, target int64) error {
	u, err := q.GetUnit(ctx, l.UnitID)
	if err != nil {
		return err
	}
	meta := transferMeta(t, l)
	loc := ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: target, OrgID: org}
	if u.UnitKind == ledger.KindFixed {
		mv, err := s.post(ctx, tx, u, ledger.Movement{Type: ledger.TypeTransferIn, UnitID: u.ID, To: &loc,
			Quantity: l.Quantity, Source: transferSource, RefType: transferRefType, RefID: l.ID,
			ActorUserID: actorPtr(c), Reason: "fixed barcode warehouse transfer received", Metadata: meta})
		if err != nil {
			return err
		}
		return q.SetWarehouseTransferLineIn(ctx, db.SetWarehouseTransferLineInParams{ID: l.ID,
			InMovementID: pgtype.Int8{Int64: mv.ID, Valid: true}, TargetLocationID: pgtype.Int8{Int64: target, Valid: true}})
	}
	self := ledger.Owner{Type: ledger.OwnerOrganization, ID: org, OrgID: org}
	inMv, err := s.post(ctx, tx, u, ledger.Movement{
		Type: ledger.TypeTransferIn, UnitID: u.ID, To: &self,
		Source: transferSource, RefType: transferRefType, RefID: l.ID,
		ActorUserID: actorPtr(c), Reason: "warehouse transfer received", Metadata: meta,
	})
	if err != nil {
		return err
	}
	pl, err := s.post(ctx, tx, u, ledger.Movement{
		Type: ledger.TypePlacement, UnitID: u.ID, From: &self, To: &loc,
		Source: transferSource, RefType: transferRefType, RefID: l.ID,
		ActorUserID: actorPtr(c), Reason: "warehouse transfer placement", Metadata: meta,
	})
	if err != nil {
		return err
	}
	return q.SetWarehouseTransferLineIn(ctx, db.SetWarehouseTransferLineInParams{
		ID: l.ID, InMovementID: pgtype.Int8{Int64: inMv.ID, Valid: true},
		PlacementMovementID: pgtype.Int8{Int64: pl.ID, Valid: true},
		TargetLocationID:    pgtype.Int8{Int64: target, Valid: true},
	})
}

// restoreLine posts the transfer_cancel_restore of one line (back to its
// source location) and stores it on the line.
func (s *WarehouseTransfers) restoreLine(ctx context.Context, q *db.Queries, tx pgx.Tx, c EntryCaller, org int64,
	t db.WarehouseTransfer, l db.WarehouseTransferLine, reason string) error {
	u, err := q.GetUnit(ctx, l.UnitID)
	if err != nil {
		return err
	}
	m := ledger.Movement{
		Type: ledger.TypeTransferCancelRestore, UnitID: u.ID,
		Source: transferSource, RefType: transferRefType, RefID: l.ID,
		ActorUserID: actorPtr(c), Reason: reason, Metadata: transferMeta(t, l),
	}
	if u.UnitKind == ledger.KindFixed {
		if !l.SourceLocationID.Valid {
			return ErrUnitUnavailable
		}
		from := ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: l.SourceLocationID.Int64, OrgID: org}
		m.To, m.Quantity = &from, l.Quantity
	}
	mv, err := s.post(ctx, tx, u, m)
	if err != nil {
		return err
	}
	return q.SetWarehouseTransferLineRestore(ctx, db.SetWarehouseTransferLineRestoreParams{
		ID: l.ID, MovementID: pgtype.Int8{Int64: mv.ID, Valid: true},
	})
}

// recordTransferDiscrepancies writes the record rows of a completion and
// returns the surplus count.
func (s *WarehouseTransfers) recordTransferDiscrepancies(ctx context.Context, q *db.Queries, c EntryCaller,
	t db.WarehouseTransfer, lines []db.WarehouseTransferLine, missing map[int64]string, surplus []TransferSurplusInput) (int, error) {
	if len(surplus) > MaxTransferSurplus {
		return 0, invalid("surplus", fmt.Sprintf("at most %d barcodes", MaxTransferSurplus))
	}
	onTransfer := map[string]bool{}
	for _, l := range lines {
		u, err := q.GetUnit(ctx, l.UnitID)
		if err != nil {
			return 0, err
		}
		onTransfer[u.Barcode] = true
		note, gone := missing[l.ID]
		if !gone {
			continue
		}
		if _, err := q.CreateWarehouseTransferDiscrepancy(ctx, db.CreateWarehouseTransferDiscrepancyParams{
			TransferID: t.ID, TransferLineID: pgtype.Int8{Int64: l.ID, Valid: true},
			UnitID: pgtype.Int8{Int64: u.ID, Valid: true}, Barcode: u.Barcode, Kind: TransferDiscrepancyMissing,
			Note: pgtype.Text{String: note, Valid: note != ""}, CreatedByUserID: c.actor(),
		}); err != nil {
			return 0, err
		}
	}
	seen := map[string]bool{}
	for i, sp := range surplus {
		field := fmt.Sprintf("surplus[%d]", i)
		code := strings.TrimSpace(sp.Barcode)
		if code == "" {
			return 0, invalid(field+".barcode", "is required")
		}
		if onTransfer[code] {
			return 0, invalid(field+".barcode", "the barcode is on this transfer")
		}
		if seen[code] {
			continue
		}
		seen[code] = true
		note, err := noteText(field+".note", sp.Note)
		if err != nil {
			return 0, err
		}
		var unitID pgtype.Int8
		if u, err := q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: t.BrandID, Barcode: code}); err == nil {
			unitID = pgtype.Int8{Int64: u.ID, Valid: true}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return 0, err
		}
		if _, err := q.CreateWarehouseTransferDiscrepancy(ctx, db.CreateWarehouseTransferDiscrepancyParams{
			TransferID: t.ID, UnitID: unitID, Barcode: code, Kind: TransferDiscrepancySurplus,
			Note: note, CreatedByUserID: c.actor(),
		}); err != nil {
			return 0, err
		}
	}
	return len(seen), nil
}

// ResolveTransferDiscrepancy closes one row of a completed transfer's
// record: a missing line found (target side) or returned to its source,
// or a surplus row closed.
func (s *WarehouseTransfers) ResolveTransferDiscrepancy(ctx context.Context, c EntryCaller, id, discID uuid.UUID,
	in ResolveTransferDiscrepancyInput) (WarehouseTransfer, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return WarehouseTransfer{}, err
	}
	res := strings.TrimSpace(in.Resolution)
	code := strings.TrimSpace(in.LocationCode)
	if in.LocationUUID != nil && code != "" {
		return WarehouseTransfer{}, invalid("location_uuid", "give at most one of location_uuid or location_code")
	}
	var note pgtype.Text
	if in.Note != nil {
		if note, err = noteText("note", *in.Note); err != nil {
			return WarehouseTransfer{}, err
		}
	}
	var t db.WarehouseTransfer
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		side := sideEither
		if res == TransferResolutionFound {
			side = sideTarget
		}
		if t, err = s.lockFor(ctx, q, c, side, org, id, TransferStatusCompleted); err != nil {
			return err
		}
		d, err := q.LockWarehouseTransferDiscrepancy(ctx, db.LockWarehouseTransferDiscrepancyParams{Uuid: discID, TransferID: t.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTransferDiscrepancyNotFound
		}
		if err != nil {
			return err
		}
		if d.Status != TransferDiscrepancyOpen {
			return ErrTransferDiscrepancyClosed
		}
		switch {
		case d.Kind == TransferDiscrepancySurplus && res == TransferResolutionClosed:
		case d.Kind == TransferDiscrepancyMissing && (res == TransferResolutionFound || res == TransferResolutionReturned):
			l, err := q.GetWarehouseTransferLine(ctx, d.TransferLineID.Int64)
			if err != nil {
				return err
			}
			if res == TransferResolutionFound {
				target := l.TargetLocationID
				if in.LocationUUID != nil || code != "" {
					loc, err := targetLocation(ctx, q, org, t.ToWarehouseID, "location_uuid", in.LocationUUID, code)
					if err != nil {
						return err
					}
					target = pgtype.Int8{Int64: loc.ID, Valid: true}
				}
				if !target.Valid {
					target = t.ToLocationID
				}
				if !target.Valid {
					return ErrTransferUnplaced
				}
				loc, err := q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: target.Int64, OrganizationID: org})
				if err != nil {
					return err
				}
				if !loc.Active || !loc.WarehouseID.Valid || loc.WarehouseID.Int64 != t.ToWarehouseID {
					return invalid("location_uuid", "target location "+loc.Code+" is inactive or outside the target warehouse")
				}
				if err := s.receiveLine(ctx, q, tx, c, org, t, l, target.Int64); err != nil {
					return err
				}
			} else if err := s.restoreLine(ctx, q, tx, c, org, t, l, "missing warehouse transfer unit returned to source"); err != nil {
				return err
			}
			if err := q.CloseWarehouseTransferLine(ctx, l.ID); err != nil {
				return err
			}
		default:
			if d.Kind == TransferDiscrepancySurplus {
				return invalid("resolution", "a surplus row is closed")
			}
			return invalid("resolution", "must be found or returned_to_source")
		}
		if _, err := q.ResolveWarehouseTransferDiscrepancy(ctx, db.ResolveWarehouseTransferDiscrepancyParams{
			ID: d.ID, Resolution: pgtype.Text{String: res, Valid: true}, ResolutionNote: note, ResolvedByUserID: c.actor(),
		}); err != nil {
			return err
		}
		return s.audit(ctx, q, c, AuditTransferDiscrepancyResolved, t, map[string]any{
			"discrepancy_uuid": d.Uuid.String(), "kind": d.Kind, "barcode": d.Barcode, "resolution": res,
		})
	})
	if err != nil {
		return WarehouseTransfer{}, err
	}
	return s.view(ctx, s.q, t, true)
}

func personName(name, surname pgtype.Text) *string {
	n := strings.TrimSpace(strings.TrimSpace(name.String) + " " + strings.TrimSpace(surname.String))
	if n == "" {
		return nil
	}
	return &n
}

// transferDiscrepancies reads the record of a transfer and its open count.
func transferDiscrepancies(ctx context.Context, q *db.Queries, t db.WarehouseTransfer) ([]TransferDiscrepancy, int64, error) {
	rows, err := q.ListWarehouseTransferDiscrepancies(ctx, t.ID)
	if err != nil {
		return nil, 0, err
	}
	lineUUIDs := map[int64]uuid.UUID{}
	out := make([]TransferDiscrepancy, 0, len(rows))
	var open int64
	for _, r := range rows {
		v := TransferDiscrepancy{
			UUID: r.Uuid, Kind: r.Kind, Status: r.Status, Barcode: r.Barcode,
			Note: textPtr(r.Note), Resolution: textPtr(r.Resolution), ResolutionNote: textPtr(r.ResolutionNote),
			CreatedBy: personName(r.CreatedByName, r.CreatedBySurname), CreatedAt: ts(r.CreatedAt),
			ResolvedBy: personName(r.ResolvedByName, r.ResolvedBySurname), ResolvedAt: tsPtr(r.ResolvedAt),
			ProductName: textPtr(r.ProductName), ProductSKU: textPtr(r.ProductSku),
		}
		if r.UnitUuid.Valid {
			u := uuid.UUID(r.UnitUuid.Bytes)
			v.UnitUUID = &u
		}
		if r.TransferLineID.Valid {
			lu, ok := lineUUIDs[r.TransferLineID.Int64]
			if !ok {
				l, err := q.GetWarehouseTransferLine(ctx, r.TransferLineID.Int64)
				if err != nil {
					return nil, 0, err
				}
				lu = l.Uuid
				lineUUIDs[l.ID] = lu
			}
			v.LineUUID = &lu
		}
		if r.Status == TransferDiscrepancyOpen {
			open++
		}
		out = append(out, v)
	}
	return out, open, nil
}
