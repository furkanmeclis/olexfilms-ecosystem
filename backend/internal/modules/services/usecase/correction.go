package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-230: consumption correction of a completed service, the conservative
// default until the product decides whether a completed service may be
// cancelled (that path does not exist; completed stays final).
//
// A unit consumed by mistake is taken back from the service into the
// service organization's stock with a ledger return movement and,
// optionally, the correct unit of the same product is consumed instead
// (same kind and amount: a whole serial unit or the item's fixed pieces).
// Limits:
//   - center only, services.cancel (brand scope) on the service;
//   - within CorrectionWindow after the service was completed;
//   - once per item, whole consumptions only (a partial cut is not
//     reversible: the ledger's return needs a used unit);
//   - the item has no warranty or only a void one; an active or expired
//     warranty is refused (void it first). The replacement gets no
//     warranty (no re-issue), corrected items are skipped by the warranty
//     listener and its repair scan;
//   - accounting is not touched.

// CorrectionWindow is how long after completion a consumption may be
// corrected.
const CorrectionWindow = 24 * time.Hour

// MaxCorrectionReasonLength bounds the correction reason.
const MaxCorrectionReasonLength = 1000

// Ledger reference of a correction: service:service_item_correction:<item
// id>:<return|consumption>:<barcode>.
const ledgerRefTypeCorrection = "service_item_correction"

// Correction errors.
var (
	// ErrCorrectionWindowClosed: the service was completed more than
	// CorrectionWindow ago.
	ErrCorrectionWindowClosed = errors.New("services: consumption correction window closed")
	// ErrItemWarrantyActive: the item has an active or expired warranty.
	ErrItemWarrantyActive = errors.New("services: item has a warranty that is not void")
	// ErrConsumptionNotReversible: the item's movement is not a whole
	// consumption (partial cut).
	ErrConsumptionNotReversible = errors.New("services: consumption is not reversible")
	// ErrItemAlreadyCorrected: the item already has a correction.
	ErrItemAlreadyCorrected = errors.New("services: item already corrected")
)

// CorrectionInput undoes the consumption of one item; ReplacementBarcode
// (optional) is the unit consumed instead.
type CorrectionInput struct {
	Reason             string
	ReplacementBarcode *string
}

// CorrectConsumption corrects the consumption of one item of a completed
// service (see the package comment above). Everything runs in one
// transaction with the service and the item locked.
func (s *Service) CorrectConsumption(ctx context.Context, c Caller, id, itemID uuid.UUID, in CorrectionInput) (ServiceView, error) {
	reason := strings.TrimSpace(in.Reason)
	if reason == "" || len([]rune(reason)) > MaxCorrectionReasonLength {
		return ServiceView{}, invalid("reason", fmt.Sprintf("is required (at most %d characters)", MaxCorrectionReasonLength))
	}
	replacement := ""
	if in.ReplacementBarcode != nil {
		replacement = strings.TrimSpace(*in.ReplacementBarcode)
		if len(replacement) > 64 {
			return ServiceView{}, invalid("replacement_barcode", "at most 64 characters")
		}
	}
	var result db.Service
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		svc, err := s.lockVisible(ctx, q, c, id)
		if err != nil {
			return err
		}
		if !c.isCenter() || !c.allows(rbac.PermServicesCancel, svc) {
			return ErrForbidden
		}
		if svc.Status != StatusCompleted || !svc.CompletedAt.Valid {
			return ErrNotEditable
		}
		if time.Since(svc.CompletedAt.Time) > CorrectionWindow {
			return ErrCorrectionWindowClosed
		}
		item, err := q.LockServiceItemByUUID(ctx, db.LockServiceItemByUUIDParams{Uuid: itemID, ServiceID: svc.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("services: lock item: %w", err)
		}
		if _, err := q.GetServiceItemCorrectionByItem(ctx, item.ID); err == nil {
			return ErrItemAlreadyCorrected
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("services: correction: %w", err)
		}
		if !item.StockMovementID.Valid {
			return ErrConsumptionNotReversible
		}
		consumed, err := q.GetStockMovement(ctx, item.StockMovementID.Int64)
		if err != nil {
			return fmt.Errorf("services: item movement: %w", err)
		}
		if consumed.Type != string(ledger.TypeConsumption) || consumed.UnitID != item.UnitID {
			return ErrConsumptionNotReversible
		}
		if w, err := q.GetWarrantyByServiceItem(ctx, item.ID); err == nil {
			if w.Status != "void" {
				return ErrItemWarrantyActive
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("services: warranty: %w", err)
		}

		l := ledger.New(s.q, s.out)
		returned, err := s.returnConsumption(ctx, q, tx, l, svc, item, consumed, c, reason)
		if err != nil {
			return err
		}
		params := db.CreateServiceItemCorrectionParams{
			ServiceItemID: item.ID, ReturnMovementID: returned.ID, Reason: reason,
			CreatedByUserID: c.actor(), ActorOrgID: c.actorOrg(),
		}
		extra := map[string]any{
			"change": "consumption_corrected", "item_uuid": item.Uuid.String(), "unit_id": item.UnitID,
			"return_movement_id": returned.ID,
		}
		if replacement != "" {
			unit, mv, err := s.consumeReplacement(ctx, q, tx, l, svc, item, replacement, c, reason)
			if err != nil {
				return err
			}
			params.ReplacementUnitID = pgtype.Int8{Int64: unit.ID, Valid: true}
			params.ReplacementMovementID = pgtype.Int8{Int64: mv.ID, Valid: true}
			extra["replacement_unit_id"] = unit.ID
			extra["replacement_movement_id"] = mv.ID
		}
		corr, err := q.CreateServiceItemCorrection(ctx, params)
		if err != nil {
			if isUniqueViolation(err, "uq_service_item_corrections_item") {
				return ErrItemAlreadyCorrected
			}
			return fmt.Errorf("services: create correction: %w", err)
		}
		extra["correction_uuid"] = corr.Uuid.String()
		result = svc
		return s.emit(ctx, tx, events.ServiceUpdated, svc, "", c, extra)
	})
	if err != nil {
		return ServiceView{}, err
	}
	return s.view(ctx, s.q, c, result)
}

func (s *Service) correctionMovement(svc db.Service, item db.ServiceItem, c Caller, reason string) ledger.Movement {
	m := ledger.Movement{
		Source: ledgerSource, RefType: ledgerRefTypeCorrection, RefID: item.ID,
		Reason: "service " + svc.ServiceNo + " correction: " + reason,
		Metadata: map[string]any{
			"service_uuid": svc.Uuid.String(), "service_no": svc.ServiceNo, "item_uuid": item.Uuid.String(),
		},
	}
	if c.Principal.UserInternal != 0 {
		actor := c.Principal.UserInternal
		m.ActorUserID = &actor
	}
	return m
}

// returnConsumption posts the return of the item's consumption into the
// service organization's own stock (organization owner): a serial unit
// comes back with the meters the consumption took, fixed pieces are
// credited back.
func (s *Service) returnConsumption(ctx context.Context, q *db.Queries, tx pgx.Tx, l *ledger.Ledger, svc db.Service,
	item db.ServiceItem, consumed db.StockMovement, c Caller, reason string) (db.StockMovement, error) {
	unit, err := q.LockUnit(ctx, item.UnitID)
	if err != nil {
		return db.StockMovement{}, fmt.Errorf("services: lock unit: %w", err)
	}
	orgOwner := ledger.Owner{Type: ledger.OwnerOrganization, ID: svc.OrganizationID, OrgID: svc.OrganizationID}
	m := s.correctionMovement(svc, item, c, reason)
	m.Type, m.UnitID, m.To = ledger.TypeReturn, unit.ID, &orgOwner
	if unit.UnitKind == ledger.KindFixed {
		if !item.Quantity.Valid || item.Quantity.Int32 <= 0 {
			return db.StockMovement{}, fmt.Errorf("services: fixed item %d without quantity", item.ID)
		}
		m.Quantity = item.Quantity.Int32
		return post(ctx, l, tx, m, item)
	}
	serviceOwner := ledger.Owner{Type: ledger.OwnerService, ID: svc.ID, OrgID: svc.OrganizationID}
	m.From = &serviceOwner
	if unit.InitialMeters.Valid {
		cm, err := ledger.NumericToCentimeters(consumed.MetersDelta)
		if err != nil {
			return db.StockMovement{}, fmt.Errorf("services: consumed meters: %w", err)
		}
		m.Centimeters = -cm
	}
	return post(ctx, l, tx, m, item)
}

// consumeReplacement consumes the correct unit: same product, same kind
// (whole unit), the item's quantity for a fixed barcode; the unit must be
// held by the service organization and not be in an open service.
func (s *Service) consumeReplacement(ctx context.Context, q *db.Queries, tx pgx.Tx, l *ledger.Ledger, svc db.Service,
	item db.ServiceItem, barcode string, c Caller, reason string) (db.Unit, db.StockMovement, error) {
	unit, err := q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: svc.BrandID, Barcode: barcode})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Unit{}, db.StockMovement{}, invalid("replacement_barcode", "unit not found")
	}
	if err != nil {
		return db.Unit{}, db.StockMovement{}, fmt.Errorf("services: unit: %w", err)
	}
	if unit, err = q.LockUnit(ctx, unit.ID); err != nil {
		return db.Unit{}, db.StockMovement{}, fmt.Errorf("services: lock unit: %w", err)
	}
	if unit.ID == item.UnitID {
		return db.Unit{}, db.StockMovement{}, invalid("replacement_barcode", "is the unit being returned")
	}
	if unit.ProductID != item.ProductID {
		return db.Unit{}, db.StockMovement{}, invalid("replacement_barcode", "the unit is not of the item's product")
	}
	m := s.correctionMovement(svc, item, c, reason)
	m.Type, m.UnitID = ledger.TypeConsumption, unit.ID
	if unit.UnitKind == ledger.KindFixed {
		if !item.Quantity.Valid || item.Quantity.Int32 <= 0 {
			return db.Unit{}, db.StockMovement{}, invalid("replacement_barcode", "the item is not a fixed barcode")
		}
		from, err := fixedSource(ctx, q, unit.ID, svc.OrganizationID, item.Quantity.Int32)
		if err != nil {
			return db.Unit{}, db.StockMovement{}, err
		}
		m.From, m.Quantity = &from, item.Quantity.Int32
	} else {
		if item.Quantity.Valid {
			return db.Unit{}, db.StockMovement{}, invalid("replacement_barcode", "the item is a fixed barcode")
		}
		if err := requireSerialHeld(ctx, q, unit.ID, svc.OrganizationID); err != nil {
			return db.Unit{}, db.StockMovement{}, err
		}
		open, err := q.ListOpenServicesByUnit(ctx, db.ListOpenServicesByUnitParams{UnitID: unit.ID})
		if err != nil {
			return db.Unit{}, db.StockMovement{}, fmt.Errorf("services: open services: %w", err)
		}
		if len(open) > 0 {
			return db.Unit{}, db.StockMovement{}, ErrUnitInUse
		}
		serviceOwner := ledger.Owner{Type: ledger.OwnerService, ID: svc.ID, OrgID: svc.OrganizationID}
		m.To = &serviceOwner
	}
	mv, err := post(ctx, l, tx, m, item)
	if err != nil {
		return db.Unit{}, db.StockMovement{}, err
	}
	return unit, mv, nil
}
