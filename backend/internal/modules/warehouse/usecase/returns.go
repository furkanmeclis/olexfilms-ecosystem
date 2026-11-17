package usecase

// TEC-1226: return inspection. A serial unit received back on a return
// (transfers, kind return) waits in quarantine: held by the receiving
// organization, not counted, not pickable or usable. The warehouse decides:
//   - restock:         quarantine_release, then a placement into the chosen bin
//   - scrap:           a scrap adjustment document (pending approval) voids it
//                      when approved (the unit stays quarantined until then)
//   - supplier_return: external_outbound (stock.adjust)
// Every decision records the return reason and is one return_inspections
// row; its id is the ledger reference (return_inspection:return_inspection:<id>).
// No price or amount is part of any view.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Inspection decisions.
const (
	InspectRestock        = "restock"
	InspectScrap          = "scrap"
	InspectSupplierReturn = "supplier_return"
)

// InspectionDecisions lists the decisions.
var InspectionDecisions = []string{InspectRestock, InspectScrap, InspectSupplierReturn}

// ReturnReasons lists the return reasons.
var ReturnReasons = []string{"defective", "damaged", "wrong_product", "excess", "customer_return", "other"}

const (
	inspectionSource  = "return_inspection"
	inspectionRefType = "return_inspection"
)

// ErrNotQuarantined: the unit is not waiting for inspection here.
var ErrNotQuarantined = errors.New("warehouse: the unit is not waiting for inspection in this organization")

// QuarantineReturn is the return a quarantined unit came back on.
type QuarantineReturn struct {
	TransferUUID uuid.UUID  `json:"transfer_uuid"`
	TransferNo   string     `json:"transfer_no"`
	FromOrgUUID  uuid.UUID  `json:"from_org_uuid"`
	FromOrgName  string     `json:"from_org_name"`
	Note         *string    `json:"note"`
	ReceivedAt   *time.Time `json:"received_at"`
}

// QuarantineItem is a unit waiting for inspection (no price).
type QuarantineItem struct {
	Unit            AdjustmentUnit      `json:"unit"`
	Product         CountProduct        `json:"product"`
	RemainingMeters *string             `json:"remaining_meters"`
	Location        *AdjustmentLocation `json:"location"`
	QuarantinedAt   time.Time           `json:"quarantined_at"`
	Return          *QuarantineReturn   `json:"return"`
	// PendingScrap: a scrap document waits for approval.
	PendingScrap bool `json:"pending_scrap"`
}

// QuarantineProduct is the quarantined total of one product.
type QuarantineProduct struct {
	Product CountProduct `json:"product"`
	Units   int64        `json:"units"`
	Meters  string       `json:"meters"`
}

// InspectionInput is one decision.
type InspectionInput struct {
	Barcode      string
	UnitUUID     *string
	Decision     string
	ReturnReason string
	Note         *string
	LocationUUID *string
}

// InspectionResult is the outcome of a decision.
type InspectionResult struct {
	UUID       uuid.UUID        `json:"uuid"`
	Decision   string           `json:"decision"`
	Unit       AdjustmentUnit   `json:"unit"`
	Status     string           `json:"status"`
	Adjustment *StockAdjustment `json:"adjustment"`
}

// Quarantine lists the units waiting for inspection.
func (s *Adjustments) Quarantine(ctx context.Context, c ScanCaller, q string, limit, offset int32) ([]QuarantineItem, int64, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.q.ListQuarantinedUnits(ctx, db.ListQuarantinedUnitsParams{
		OrganizationID: org, Q: listQ(q), RowLimit: limit, RowOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("warehouse: quarantine: %w", err)
	}
	total, err := s.q.CountQuarantinedUnits(ctx, db.CountQuarantinedUnitsParams{OrganizationID: org, Q: listQ(q)})
	if err != nil {
		return nil, 0, fmt.Errorf("warehouse: quarantine count: %w", err)
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.UnitID)
	}
	info := map[int64]db.ListReturnInfoForUnitsRow{}
	if len(ids) > 0 {
		ret, err := s.q.ListReturnInfoForUnits(ctx, db.ListReturnInfoForUnitsParams{UnitIds: ids, OrganizationID: org})
		if err != nil {
			return nil, 0, fmt.Errorf("warehouse: quarantine returns: %w", err)
		}
		for _, r := range ret {
			info[r.UnitID] = r
		}
	}
	out := make([]QuarantineItem, 0, len(rows))
	for _, r := range rows {
		it := QuarantineItem{
			Unit:            AdjustmentUnit{UUID: r.UnitUuid, Barcode: r.Barcode, UnitKind: r.UnitKind, IsRoll: r.IsRoll},
			Product:         CountProduct{UUID: r.ProductUuid, Name: r.ProductName, SKU: r.ProductSku},
			RemainingMeters: metersPtr(r.RemainingMeters), QuarantinedAt: r.QuarantinedAt.Time, PendingScrap: r.PendingScrap,
		}
		if r.LocationUuid.Valid {
			it.Location = &AdjustmentLocation{UUID: r.LocationUuid.Bytes, FullCode: r.LocationFullCode.String}
		}
		if ri, ok := info[r.UnitID]; ok {
			it.Return = &QuarantineReturn{
				TransferUUID: ri.TransferUuid, TransferNo: ri.TransferNo, FromOrgUUID: ri.FromOrgUuid,
				FromOrgName: ri.FromOrgName, Note: textPtr(ri.ReturnNote), ReceivedAt: timePtr(ri.ReceivedAt),
			}
		}
		out = append(out, it)
	}
	return out, total, nil
}

// QuarantineSummary sums the quarantined units per product (the stock
// views show them apart from the counted stock).
func (s *Adjustments) QuarantineSummary(ctx context.Context, c ScanCaller) ([]QuarantineProduct, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.SumQuarantineByProduct(ctx, org)
	if err != nil {
		return nil, fmt.Errorf("warehouse: quarantine summary: %w", err)
	}
	out := make([]QuarantineProduct, 0, len(rows))
	for _, r := range rows {
		m := "0.00"
		if v := metersPtr(r.Meters); v != nil {
			m = *v
		}
		out = append(out, QuarantineProduct{
			Product: CountProduct{UUID: r.ProductUuid, Name: r.ProductName, SKU: r.ProductSku}, Units: r.Units, Meters: m,
		})
	}
	return out, nil
}

// quarantinedUnit finds and locks the quarantined unit of the decision.
func quarantinedUnit(ctx context.Context, q *db.Queries, org int64, in InspectionInput) (db.Unit, db.UnitCurrentState, error) {
	var candidates []db.Unit
	switch {
	case in.UnitUUID != nil && strings.TrimSpace(*in.UnitUUID) != "":
		id, err := parseUUIDField("unit_uuid", *in.UnitUUID)
		if err != nil {
			return db.Unit{}, db.UnitCurrentState{}, err
		}
		u, err := q.GetUnitByUUID(ctx, id)
		if err != nil {
			return db.Unit{}, db.UnitCurrentState{}, notFound(err, ErrNotQuarantined)
		}
		candidates = []db.Unit{u}
	case strings.TrimSpace(in.Barcode) != "":
		units, err := q.ListUnitsByBarcode(ctx, strings.TrimSpace(in.Barcode))
		if err != nil {
			return db.Unit{}, db.UnitCurrentState{}, err
		}
		candidates = units
	default:
		return db.Unit{}, db.UnitCurrentState{}, invalid("barcode", "barcode or unit_uuid is required")
	}
	for _, u := range candidates {
		if u.UnitKind != ledger.KindSerial {
			continue
		}
		locked, err := q.LockUnit(ctx, u.ID)
		if err != nil {
			return db.Unit{}, db.UnitCurrentState{}, err
		}
		st, err := q.GetUnitCurrentState(ctx, u.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return db.Unit{}, db.UnitCurrentState{}, err
		}
		if st.HolderOrgID == org && st.Status == string(ledger.StatusQuarantine) {
			return locked, st, nil
		}
	}
	return db.Unit{}, db.UnitCurrentState{}, ErrNotQuarantined
}

// Inspect records one decision on a quarantined unit.
func (s *Adjustments) Inspect(ctx context.Context, c ScanCaller, in InspectionInput) (InspectionResult, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return InspectionResult{}, err
	}
	if !slices.Contains(InspectionDecisions, in.Decision) {
		return InspectionResult{}, invalid("decision", "must be one of "+strings.Join(InspectionDecisions, ", "))
	}
	if !slices.Contains(ReturnReasons, in.ReturnReason) {
		return InspectionResult{}, invalid("return_reason", "must be one of "+strings.Join(ReturnReasons, ", "))
	}
	note, err := optText("note", in.Note, maxAdjustmentNote)
	if err != nil {
		return InspectionResult{}, err
	}
	if in.Decision == InspectSupplierReturn && !s.canAdjust(ctx, c, org) {
		return InspectionResult{}, ErrAdjustmentForbidden
	}
	if in.Decision != InspectRestock && in.LocationUUID != nil && strings.TrimSpace(*in.LocationUUID) != "" {
		return InspectionResult{}, invalid("location_uuid", "only a restocked unit takes a bin")
	}
	var res InspectionResult
	var adjustment *db.StockAdjustment
	err = s.inTx(ctx, func(tx pgx.Tx, q *db.Queries) error {
		u, st, err := quarantinedUnit(ctx, q, org, in)
		if err != nil {
			return err
		}
		busy, err := q.CountOpenAdjustmentLinesByUnit(ctx, db.CountOpenAdjustmentLinesByUnitParams{UnitID: u.ID})
		if err != nil {
			return err
		}
		if busy > 0 {
			return fmt.Errorf("%w: a scrap document waits for approval", ErrAdjustmentUnitBusy)
		}
		if st.OwnerType == string(ledger.OwnerWarehouseLocation) {
			if l, err := q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: st.OwnerID, OrganizationID: org}); err == nil && l.WarehouseID.Valid {
				if err := requireMember(ctx, q, c.Principal, org, l.WarehouseID.Int64); err != nil {
					return err
				}
			}
		}
		loc, err := lineLocation(ctx, q, c, org, in.LocationUUID)
		if err != nil {
			return err
		}
		var item pgtype.Int8
		if ri, err := q.GetLastReturnItemForUnit(ctx, db.GetLastReturnItemForUnitParams{UnitID: u.ID, OrganizationID: org}); err == nil {
			item = i8(ri.ID)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		arg := db.InsertReturnInspectionParams{
			OrganizationID: org, BrandID: u.BrandID, UnitID: u.ID, TransferItemID: item, Decision: in.Decision,
			ReturnReason: in.ReturnReason, Note: note, UserID: userArg(c),
		}
		if loc != nil {
			arg.LocationID = i8(loc.ID)
		}
		if in.Decision == InspectScrap {
			a, err := s.scrapDocument(ctx, q, c, org, u, in, note)
			if err != nil {
				return err
			}
			adjustment, arg.AdjustmentID = &a, i8(a.ID)
		}
		ins, err := q.InsertReturnInspection(ctx, arg)
		if err != nil {
			return fmt.Errorf("warehouse: inspection: %w", err)
		}
		status := string(ledger.StatusQuarantine)
		if in.Decision != InspectScrap {
			m := ledger.Movement{
				UnitID: u.ID, Source: inspectionSource, RefType: inspectionRefType, RefID: ins.ID,
				ActorUserID: actorPtr(EntryCaller(c)), Reason: "return inspection: " + in.ReturnReason,
				Metadata: map[string]any{"inspection_uuid": ins.Uuid.String(), "decision": in.Decision, "return_reason": in.ReturnReason},
			}
			if in.Decision == InspectRestock {
				m.Type = ledger.TypeQuarantineRelease
			} else {
				m.Type = ledger.TypeExternalOutbound
			}
			r, err := s.ledger.Post(ctx, tx, m)
			if err != nil {
				if isLedgerConflict(err) {
					return fmt.Errorf("%w: %v", ErrAdjustmentStale, err)
				}
				return err
			}
			if err := q.SetReturnInspectionMovement(ctx, db.SetReturnInspectionMovementParams{ID: ins.ID, MovementID: i8(r.Movement.ID)}); err != nil {
				return err
			}
			status = r.Movement.ToStatus.String
			if loc != nil {
				owner := ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc.ID, OrgID: org}
				m.Type, m.To = ledger.TypePlacement, &owner
				r, err := s.ledger.Post(ctx, tx, m)
				if err != nil {
					if isLedgerConflict(err) {
						return fmt.Errorf("%w: %v", ErrAdjustmentStale, err)
					}
					return err
				}
				status = r.Movement.ToStatus.String
			}
		}
		res = InspectionResult{
			UUID: ins.Uuid, Decision: in.Decision, Status: status,
			Unit: AdjustmentUnit{UUID: u.Uuid, Barcode: u.Barcode, UnitKind: u.UnitKind, IsRoll: u.InitialMeters.Valid},
		}
		return nil
	})
	if err != nil {
		return InspectionResult{}, err
	}
	if adjustment != nil {
		v, err := s.detail(ctx, s.q, c, *adjustment)
		if err != nil {
			return InspectionResult{}, err
		}
		res.Adjustment = &v
	}
	return res, nil
}

// scrapDocument opens the scrap document of a quarantined unit and submits
// it (the threshold decides whether a second person approves).
func (s *Adjustments) scrapDocument(ctx context.Context, q *db.Queries, c ScanCaller, org int64, u db.Unit, in InspectionInput, note pgtype.Text) (db.StockAdjustment, error) {
	a, err := q.CreateStockAdjustment(ctx, db.CreateStockAdjustmentParams{
		OrganizationID: org, BrandID: c.Org.BrandID, Type: AdjTypeScrap,
		ReasonCode: pgtype.Text{String: "return_" + in.ReturnReason, Valid: true}, Note: note,
		Source: AdjSourceReturn, UserID: userArg(c),
	})
	if err != nil {
		return db.StockAdjustment{}, fmt.Errorf("warehouse: scrap document: %w", err)
	}
	line := db.InsertStockAdjustmentLineParams{
		AdjustmentID: a.ID, OrganizationID: org, UnitID: u.ID, ProductID: u.ProductID, Kind: AdjLineWhole,
	}
	if u.InitialMeters.Valid {
		line.RollMeters = u.RemainingMeters
	}
	if _, err := q.InsertStockAdjustmentLine(ctx, line); err != nil {
		return db.StockAdjustment{}, fmt.Errorf("warehouse: scrap line: %w", err)
	}
	return s.submit(ctx, q, c, a)
}
