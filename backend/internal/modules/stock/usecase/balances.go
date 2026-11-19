package usecase

import (
	"context"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	"github.com/jackc/pgx/v5/pgtype"
)

// BalanceScope is where a stock balance is read (TEC-1229): the whole
// organization, one of its warehouses or one location.
type BalanceScope struct {
	OrganizationID int64
	WarehouseID    int64
	LocationID     int64
}

func (b BalanceScope) warehouse() pgtype.Int8 {
	return pgtype.Int8{Int64: b.WarehouseID, Valid: b.WarehouseID > 0}
}

func (b BalanceScope) location() pgtype.Int8 {
	return pgtype.Int8{Int64: b.LocationID, Valid: b.LocationID > 0}
}

// Amount is a quantity and roll centimeters.
type Amount struct {
	Quantity    int64
	Centimeters int64
}

func cm(s string) int64 {
	v, err := ledger.ParseMeters(s)
	if err != nil {
		return 0
	}
	return v
}

// Balances reads reserved, incoming and quarantined stock of the products
// in a scope and combines them with the on-hand amounts.
func (s *Service) Balances(ctx context.Context, scope BalanceScope, onHand map[int64]Amount) (map[int64]model.StockBalance, error) {
	ids := make([]int64, 0, len(onHand))
	for id := range onHand {
		ids = append(ids, id)
	}
	reserved := map[int64]Amount{}
	incoming := map[int64]Amount{}
	if len(ids) > 0 {
		rs, err := s.q.ListStockReservedByProduct(ctx, db.ListStockReservedByProductParams{
			OrganizationID: scope.OrganizationID, WarehouseID: scope.warehouse(), LocationID: scope.location(), ProductIds: ids,
		})
		if err != nil {
			return nil, fmt.Errorf("stock: reserved: %w", err)
		}
		for _, r := range rs {
			reserved[r.ProductID] = Amount{Quantity: r.ReservedQuantity, Centimeters: cm(r.ReservedMeters)}
		}
		in, err := s.q.ListStockIncomingByProduct(ctx, db.ListStockIncomingByProductParams{
			OrganizationID: scope.OrganizationID, WarehouseID: scope.warehouse(), LocationID: scope.location(), ProductIds: ids,
		})
		if err != nil {
			return nil, fmt.Errorf("stock: incoming: %w", err)
		}
		for _, r := range in {
			incoming[r.ProductID] = Amount{Quantity: r.IncomingQuantity, Centimeters: cm(r.IncomingMeters)}
		}
	}
	quarantine := map[int64]Amount{}
	if len(ids) > 0 {
		qs, err := s.q.ListStockQuarantineByProduct(ctx, db.ListStockQuarantineByProductParams{
			OrganizationID: scope.OrganizationID, WarehouseID: scope.warehouse(), LocationID: scope.location(), ProductIds: ids,
		})
		if err != nil {
			return nil, fmt.Errorf("stock: quarantine: %w", err)
		}
		for _, r := range qs {
			quarantine[r.ProductID] = Amount{Quantity: r.QuarantineQuantity, Centimeters: cm(r.QuarantineMeters)}
		}
	}
	breakdown := map[int64]Breakdown{}
	if len(ids) > 0 {
		ms, err := s.q.ListStockMeasureBreakdownByProduct(ctx, db.ListStockMeasureBreakdownByProductParams{
			OrganizationID: scope.OrganizationID, WarehouseID: scope.warehouse(), LocationID: scope.location(), ProductIds: ids,
		})
		if err != nil {
			return nil, fmt.Errorf("stock: measures: %w", err)
		}
		for _, r := range ms {
			breakdown[r.ProductID] = Breakdown{
				Open:             Amount{Quantity: r.OpenCount, Centimeters: cm(r.OpenMeters)},
				Cut:              Amount{Quantity: r.CutCount, Centimeters: cm(r.CutMeters)},
				Remnant:          Amount{Quantity: r.RemnantCount, Centimeters: cm(r.RemnantMeters)},
				ReservedMetered:  r.ReservedMeteredCount,
				ReservedMetersCm: cm(r.ReservedMeters),
			}
		}
	}
	out := make(map[int64]model.StockBalance, len(onHand))
	for id, oh := range onHand {
		b := ComposeBalance(oh.Quantity, oh.Centimeters, reserved[id], incoming[id], quarantine[id])
		b.Measures = ComposeMeasures(oh.Quantity, reserved[id].Quantity, breakdown[id])
		out[id] = b
	}
	return out, nil
}

// Breakdown is the metered part of a product's on-hand stock (TEC-1243):
// open rolls, cut pieces and remnants, and the reservations on metered
// units (count and meters, remnants excluded).
type Breakdown struct {
	Open, Cut, Remnant Amount
	ReservedMetered    int64
	ReservedMetersCm   int64
}

// ComposeMeasures builds the K14 split: closed boxes are the on-hand
// quantity minus the metered units; available boxes subtract the
// reservations that are not on metered units; available meters are the
// open roll and cut piece meters minus their reserved meters.
func ComposeMeasures(onHandQty, reservedQty int64, b Breakdown) model.StockMeasures {
	metered := b.Open.Quantity + b.Cut.Quantity + b.Remnant.Quantity
	boxes := max(onHandQty-metered, 0)
	boxReserved := max(reservedQty-b.ReservedMetered, 0)
	return model.StockMeasures{
		Boxes:           boxes,
		AvailableBoxes:  max(boxes-boxReserved, 0),
		OpenRolls:       b.Open.Quantity,
		OpenMeters:      ledger.FormatMeters(b.Open.Centimeters),
		CutPieces:       b.Cut.Quantity,
		CutMeters:       ledger.FormatMeters(b.Cut.Centimeters),
		Remnants:        b.Remnant.Quantity,
		RemnantMeters:   ledger.FormatMeters(b.Remnant.Centimeters),
		AvailableMeters: ledger.FormatMeters(max(b.Open.Centimeters+b.Cut.Centimeters-b.ReservedMetersCm, 0)),
	}
}

// ComposeBalance builds the balance of one product: available = on hand -
// reserved, never below zero. Quarantined units (TEC-1226) are not on hand,
// so they are shown apart and not subtracted again.
func ComposeBalance(onHandQty, onHandCm int64, reserved, incoming, quarantine Amount) model.StockBalance {
	availQty := max(onHandQty-reserved.Quantity, 0)
	availCm := max(onHandCm-reserved.Centimeters, 0)
	return model.StockBalance{
		OnHandQuantity: onHandQty, OnHandMeters: ledger.FormatMeters(onHandCm),
		ReservedQuantity: reserved.Quantity, ReservedMeters: ledger.FormatMeters(reserved.Centimeters),
		AvailableQuantity: availQty, AvailableMeters: ledger.FormatMeters(availCm),
		IncomingQuantity: incoming.Quantity, IncomingMeters: ledger.FormatMeters(incoming.Centimeters),
		QuarantineQuantity: quarantine.Quantity, QuarantineMeters: ledger.FormatMeters(quarantine.Centimeters),
		Measures: ComposeMeasures(onHandQty, reserved.Quantity, Breakdown{}),
	}
}

// withBalances fills ProductStock.Balance of a product page.
func (s *Service) withBalances(ctx context.Context, scope BalanceScope, items []model.ProductStock, productIDs []int64) error {
	onHand := make(map[int64]Amount, len(items))
	for i, it := range items {
		onHand[productIDs[i]] = Amount{Quantity: int64(it.Quantity), Centimeters: cm(it.Meters)}
	}
	bal, err := s.Balances(ctx, scope, onHand)
	if err != nil {
		return err
	}
	for i := range items {
		b := bal[productIDs[i]]
		items[i].Balance = &b
	}
	return nil
}
