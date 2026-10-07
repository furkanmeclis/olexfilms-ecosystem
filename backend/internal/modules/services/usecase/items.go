package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Item kinds (chk_service_items_kind).
const (
	KindFull    = "full"
	KindPartial = "partial"
)

// Meters of a cut: NUMERIC(10,2), positive.
var metersRe = regexp.MustCompile(`^[0-9]{1,8}(\.[0-9]{1,2})?$`)

func parseRat(s string) (*big.Rat, bool) {
	return new(big.Rat).SetString(strings.TrimSpace(s))
}

// normalizeMeters validates a meter amount and returns it with two decimals.
func normalizeMeters(raw string) (string, *big.Rat, bool) {
	m := strings.TrimSpace(raw)
	if !metersRe.MatchString(m) {
		return "", nil, false
	}
	r, ok := parseRat(m)
	if !ok || r.Sign() <= 0 {
		return "", nil, false
	}
	return r.FloatString(2), r, true
}

func numeric(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("services: numeric %q: %w", s, err)
	}
	return n, nil
}

func numericRat(n pgtype.Numeric) *big.Rat {
	if !n.Valid || n.Int == nil {
		return nil
	}
	r := new(big.Rat).SetInt(n.Int)
	if n.Exp != 0 {
		e := n.Exp
		if e < 0 {
			e = -e
		}
		p := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(e)), nil))
		if n.Exp > 0 {
			r.Mul(r, p)
		} else {
			r.Quo(r, p)
		}
	}
	return r
}

func numericTextPtr(n pgtype.Numeric) *string {
	r := numericRat(n)
	if r == nil {
		return nil
	}
	s := r.FloatString(2)
	return &s
}

// ItemInput adds one stock unit to a service. The unit is found by its
// barcode in the service brand; ProductUUID, when given, must be the
// unit's product. full = the whole unit (Quantity pieces for a fixed
// barcode), partial = Meters cut from a roll.
type ItemInput struct {
	Barcode      string
	ProductUUID  *string
	Kind         string
	Quantity     *int64
	Meters       *string
	AppliedParts []string
	Notes        *string
}

// AddItem adds a stock unit to a draft / pending / processing service.
// The unit must be held by the service organization (serial: available or
// placed, fixed barcode: enough pieces on hand); a serial unit already in
// another open service (or this one) answers ErrUnitInUse. A roll may be
// cut for several services, but a whole roll excludes any other use.
func (s *Service) AddItem(ctx context.Context, c Caller, id uuid.UUID, in ItemInput) (ServiceView, error) {
	barcode := strings.TrimSpace(in.Barcode)
	if barcode == "" || len(barcode) > 64 {
		return ServiceView{}, invalid("barcode", "is required (at most 64 characters)")
	}
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = KindFull
	}
	if kind != KindFull && kind != KindPartial {
		return ServiceView{}, invalid("kind", "must be full or partial")
	}
	if err := checkLen("notes", in.Notes, MaxNoteLength); err != nil {
		return ServiceView{}, err
	}
	parts, err := normalizeParts(in.AppliedParts)
	if err != nil {
		return ServiceView{}, err
	}
	var updated db.Service
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		svc, err := s.lockWritable(ctx, q, c, id)
		if err != nil {
			return err
		}
		if !itemsEditable(svc.Status) {
			return ErrNotEditable
		}
		existing, err := q.ListServiceItems(ctx, svc.ID)
		if err != nil {
			return fmt.Errorf("services: items: %w", err)
		}
		if len(existing) >= MaxItems {
			return invalid("barcode", fmt.Sprintf("a service has at most %d items", MaxItems))
		}
		unit, err := q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: svc.BrandID, Barcode: barcode})
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid("barcode", "unit not found")
		}
		if err != nil {
			return fmt.Errorf("services: unit: %w", err)
		}
		// Serialises item writes on the unit (two services adding the same
		// serial unit at once).
		if unit, err = q.LockUnit(ctx, unit.ID); err != nil {
			return fmt.Errorf("services: lock unit: %w", err)
		}
		product, err := q.GetProduct(ctx, db.GetProductParams{ID: unit.ProductID, BrandID: svc.BrandID})
		if err != nil {
			return fmt.Errorf("services: product: %w", err)
		}
		if in.ProductUUID != nil && strings.TrimSpace(*in.ProductUUID) != "" {
			pid, err := parseUUID("product_uuid", *in.ProductUUID)
			if err != nil {
				return err
			}
			if pid != product.Uuid {
				return invalid("product_uuid", "the unit is not of this product")
			}
		}
		if err := checkParts(ctx, q, product, parts); err != nil {
			return err
		}
		params := db.CreateServiceItemParams{
			ServiceID: svc.ID, ProductID: product.ID, UnitID: unit.ID, Kind: kind,
			AppliedParts: mustJSON(parts), Notes: textOrNull(in.Notes),
		}
		roll := unit.UnitKind == ledger.KindSerial && product.UnitType == "roll_meter"
		switch {
		case unit.UnitKind == ledger.KindFixed:
			if kind != KindFull {
				return invalid("kind", "a fixed barcode is used in pieces (full)")
			}
			if in.Meters != nil || in.Quantity == nil || *in.Quantity <= 0 || *in.Quantity > MaxQuantity {
				return invalid("quantity", fmt.Sprintf("must be between 1 and %d", MaxQuantity))
			}
			params.Quantity = pgtype.Int4{Int32: int32(*in.Quantity), Valid: true}
			if err := requireFixedOnHand(ctx, q, unit.ID, svc.OrganizationID, *in.Quantity); err != nil {
				return err
			}
		case kind == KindPartial:
			if !roll {
				return invalid("kind", "only a roll can be cut (partial)")
			}
			if in.Quantity != nil || in.Meters == nil {
				return invalid("meters", "a cut needs meters")
			}
			m, r, ok := normalizeMeters(*in.Meters)
			if !ok {
				return invalid("meters", "must be a positive decimal with at most 2 fractional digits")
			}
			if rem := numericRat(unit.RemainingMeters); rem != nil && r.Cmp(rem) > 0 {
				return invalid("meters", "more than the remaining meters of the roll")
			}
			if params.Meters, err = numeric(m); err != nil {
				return err
			}
			if err := requireSerialHeld(ctx, q, unit.ID, svc.OrganizationID); err != nil {
				return err
			}
		default:
			if in.Quantity != nil || in.Meters != nil {
				return invalid("quantity", "a whole serial unit takes no quantity or meters")
			}
			if err := requireSerialHeld(ctx, q, unit.ID, svc.OrganizationID); err != nil {
				return err
			}
		}
		if unit.UnitKind == ledger.KindSerial {
			open, err := q.ListOpenServicesByUnit(ctx, db.ListOpenServicesByUnitParams{UnitID: unit.ID})
			if err != nil {
				return fmt.Errorf("services: open services: %w", err)
			}
			for _, o := range open {
				if !roll || kind == KindFull || o.Kind == KindFull {
					return ErrUnitInUse
				}
			}
		}
		item, err := q.CreateServiceItem(ctx, params)
		if err != nil {
			return fmt.Errorf("services: create item: %w", err)
		}
		if err := s.evaluateCertificatePolicy(ctx, q, tx, svc, c); err != nil {
			return err
		}
		updated = svc
		return s.emit(ctx, tx, events.ServiceUpdated, svc, "", c, map[string]any{
			"change": "item_added", "item_uuid": item.Uuid.String(), "unit_id": unit.ID,
		})
	})
	if err != nil {
		return ServiceView{}, err
	}
	return s.view(ctx, s.q, c, updated)
}

// RemoveItem deletes one item of a draft / pending / processing service.
func (s *Service) RemoveItem(ctx context.Context, c Caller, id, itemID uuid.UUID) (ServiceView, error) {
	var updated db.Service
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		svc, err := s.lockWritable(ctx, q, c, id)
		if err != nil {
			return err
		}
		if !itemsEditable(svc.Status) {
			return ErrNotEditable
		}
		item, err := q.GetServiceItemByUUID(ctx, db.GetServiceItemByUUIDParams{Uuid: itemID, ServiceID: svc.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("services: item: %w", err)
		}
		if _, err := q.DeleteServiceItem(ctx, db.DeleteServiceItemParams{ID: item.ID, ServiceID: svc.ID}); err != nil {
			return fmt.Errorf("services: delete item: %w", err)
		}
		updated = svc
		return s.emit(ctx, tx, events.ServiceUpdated, svc, "", c, map[string]any{
			"change": "item_removed", "item_uuid": item.Uuid.String(), "unit_id": item.UnitID,
		})
	})
	if err != nil {
		return ServiceView{}, err
	}
	return s.view(ctx, s.q, c, updated)
}

// requireSerialHeld: the serial unit is held by org and available or
// placed (not in transit, used or void).
func requireSerialHeld(ctx context.Context, q *db.Queries, unitID, orgID int64) error {
	st, err := q.GetUnitCurrentState(ctx, unitID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUnitNotAvailable
	}
	if err != nil {
		return fmt.Errorf("services: unit state: %w", err)
	}
	if st.HolderOrgID != orgID ||
		(st.Status != string(ledger.StatusAvailable) && st.Status != string(ledger.StatusPlaced)) ||
		(st.OwnerType != string(ledger.OwnerOrganization) && st.OwnerType != string(ledger.OwnerWarehouseLocation)) {
		return ErrUnitNotAvailable
	}
	return nil
}

// requireFixedOnHand: org holds at least qty pieces of the fixed barcode.
func requireFixedOnHand(ctx context.Context, q *db.Queries, unitID, orgID, qty int64) error {
	rows, err := q.ListFixedBarcodeHoldingsByUnit(ctx, unitID)
	if err != nil {
		return fmt.Errorf("services: holdings: %w", err)
	}
	var onHand int64
	for _, h := range rows {
		if h.HolderOrgID == orgID &&
			(h.OwnerType == string(ledger.OwnerOrganization) || h.OwnerType == string(ledger.OwnerWarehouseLocation)) {
			onHand += int64(h.QuantityOnHand)
		}
	}
	if onHand < qty {
		return ErrUnitNotAvailable
	}
	return nil
}

func normalizeParts(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for i, p := range in {
		p = strings.TrimSpace(p)
		if p == "" || len(p) > 64 {
			return nil, invalid(fmt.Sprintf("applied_parts[%d]", i), "must be a part key")
		}
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out, nil
}

// checkParts: the parts must be in the product category's available_parts
// (also enforced by the service_items trigger).
func checkParts(ctx context.Context, q *db.Queries, p db.Product, parts []string) error {
	if len(parts) == 0 {
		return nil
	}
	cat, err := q.GetProductCategory(ctx, db.GetProductCategoryParams{ID: p.CategoryID, BrandID: p.BrandID})
	if err != nil {
		return fmt.Errorf("services: category: %w", err)
	}
	var allowed []string
	if len(cat.AvailableParts) > 0 {
		if err := json.Unmarshal(cat.AvailableParts, &allowed); err != nil {
			allowed = nil
		}
	}
	for i, part := range parts {
		if !slices.Contains(allowed, part) {
			return invalid(fmt.Sprintf("applied_parts[%d]", i), "part is not available for this product")
		}
	}
	return nil
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("[]")
	}
	return b
}
