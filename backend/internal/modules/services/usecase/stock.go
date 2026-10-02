package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Stock picker page size.
const (
	StockDefaultLimit = 20
	StockMaxLimit     = 100
)

// StockFilter narrows the stock picker (TEC-180): Barcode is an exact
// barcode (scanner lookup), ProductUUID a product, MinMeters the meters a
// roll must still have (only rolls match when it is set), Q a part of the
// product name, SKU or barcode (TEC-182 product search).
type StockFilter struct {
	Barcode     string
	Q           string
	ProductUUID string
	MinMeters   string
	Limit       int32
	Offset      int32
}

// StockUnitView is one unit the service organization can add as an item.
// QuantityOnHand is 1 for a serial unit and the pieces on hand for a fixed
// barcode; the meters are set for rolls.
type StockUnitView struct {
	UUID            uuid.UUID        `json:"uuid"`
	Barcode         string           `json:"barcode"`
	UnitKind        string           `json:"unit_kind"`
	Product         StockProductView `json:"product"`
	QuantityOnHand  int32            `json:"quantity_on_hand"`
	InitialMeters   *string          `json:"initial_meters"`
	RemainingMeters *string          `json:"remaining_meters"`
}

// StockProductView is the product of a picker unit with the parts its
// category allows (the applied_parts of a new item must come from them).
type StockProductView struct {
	ProductRef
	AvailableParts []string `json:"available_parts"`
}

// StockUnits lists the units the service organization holds and can add
// to the service: serial units available or placed in its own stock that
// are not already taken by an open service (a roll stays listed while it
// is only cut), and fixed barcodes with pieces on hand. The caller needs
// services.write on the service (the picker feeds AddItem).
func (s *Service) StockUnits(ctx context.Context, c Caller, id uuid.UUID, f StockFilter) ([]StockUnitView, error) {
	svc, err := s.q.GetServiceByUUID(ctx, db.GetServiceByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("services: get: %w", err)
	}
	if !visible(c, svc) {
		return nil, ErrNotFound
	}
	if !c.allows(rbac.PermServicesWrite, svc) {
		return nil, ErrForbidden
	}
	arg := db.ListServiceStockUnitsParams{
		OrganizationID: svc.OrganizationID, BrandID: svc.BrandID,
		RowLimit: f.Limit, RowOffset: f.Offset,
	}
	if arg.RowLimit <= 0 {
		arg.RowLimit = StockDefaultLimit
	}
	if arg.RowLimit > StockMaxLimit {
		arg.RowLimit = StockMaxLimit
	}
	if arg.RowOffset < 0 {
		arg.RowOffset = 0
	}
	if b := strings.TrimSpace(f.Barcode); b != "" {
		if len(b) > 64 {
			return nil, invalid("barcode", "must be at most 64 characters")
		}
		arg.Barcode = pgtype.Text{String: b, Valid: true}
	}
	if term := strings.TrimSpace(f.Q); term != "" {
		if utf8.RuneCountInString(term) > 100 {
			return nil, invalid("q", "must be at most 100 characters")
		}
		arg.Q = pgtype.Text{String: term, Valid: true}
	}
	if p := strings.TrimSpace(f.ProductUUID); p != "" {
		pid, err := parseUUID("product_uuid", p)
		if err != nil {
			return nil, err
		}
		product, err := s.q.GetProductIDByUUID(ctx, pid)
		if errors.Is(err, pgx.ErrNoRows) {
			return []StockUnitView{}, nil
		}
		if err != nil {
			return nil, fmt.Errorf("services: product: %w", err)
		}
		arg.ProductID = pgtype.Int8{Int64: product, Valid: true}
	}
	if m := strings.TrimSpace(f.MinMeters); m != "" {
		norm, _, ok := normalizeMeters(m)
		if !ok {
			return nil, invalid("min_meters", "must be a positive decimal with at most 2 fractional digits")
		}
		if arg.MinMeters, err = numeric(norm); err != nil {
			return nil, err
		}
	}
	rows, err := s.q.ListServiceStockUnits(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("services: stock units: %w", err)
	}
	out := make([]StockUnitView, 0, len(rows))
	for _, r := range rows {
		parts := []string{}
		if len(r.ProductAvailableParts) > 0 {
			if err := json.Unmarshal(r.ProductAvailableParts, &parts); err != nil || parts == nil {
				parts = []string{}
			}
		}
		out = append(out, StockUnitView{
			UUID: r.Uuid, Barcode: r.Barcode, UnitKind: r.UnitKind,
			Product: StockProductView{
				ProductRef:     ProductRef{UUID: r.ProductUuid, SKU: r.ProductSku, Name: r.ProductName, UnitType: r.ProductUnitType},
				AvailableParts: parts,
			},
			QuantityOnHand:  r.QuantityOnHand,
			InitialMeters:   numericTextPtr(r.InitialMeters),
			RemainingMeters: numericTextPtr(r.RemainingMeters),
		})
	}
	return out, nil
}
