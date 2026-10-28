package usecase

import (
	"context"
	"fmt"
	"math/big"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1035: a statement row booked from an order (source "order") carries
// the order: its number, lines (product, quantity/meters, unit price, line
// total) and the unit barcodes assigned to each line, i.e. what went to the
// buyer with the order. The same detail shows on the seller's statement of
// the buyer cari and on the buyer's own statement; only orders the book is a
// party of are read.

// statementOrderSource is the finance_entries.source_type of order rows
// (orders usecase AccountingSourceType).
const statementOrderSource = "order"

// StatementOrder is the order behind a statement row.
type StatementOrder struct {
	UUID     uuid.UUID            `json:"uuid"`
	OrderNo  string               `json:"order_no"`
	Status   string               `json:"status"`
	Currency string               `json:"currency"`
	Items    []StatementOrderItem `json:"items"`
}

// StatementOrderItem is one line of the order with its assigned units.
type StatementOrderItem struct {
	ProductUUID uuid.UUID            `json:"product_uuid"`
	ProductSKU  string               `json:"product_sku"`
	ProductName string               `json:"product_name"`
	UnitType    string               `json:"unit_type"`
	Quantity    *int32               `json:"quantity"`
	Meters      *string              `json:"meters"`
	UnitPrice   string               `json:"unit_price"`
	LineTotal   string               `json:"line_total"`
	Units       []StatementOrderUnit `json:"units"`
}

// StatementOrderUnit is a unit (barcode) assigned to an order line.
type StatementOrderUnit struct {
	UUID     uuid.UUID `json:"uuid"`
	Barcode  string    `json:"barcode"`
	Quantity *int32    `json:"quantity"`
	Meters   *string   `json:"meters"`
}

// Barcodes lists the barcodes of the item's units.
func (it StatementOrderItem) Barcodes() []string {
	out := make([]string, 0, len(it.Units))
	for _, u := range it.Units {
		out = append(out, u.Barcode)
	}
	return out
}

// attachStatementOrders loads the orders of the order rows of lines (two
// queries for the whole statement) and sets Line.Order.
func (s *Service) attachStatementOrders(ctx context.Context, bookID int64, lines []StatementLine) error {
	seen := map[uuid.UUID]bool{}
	ids := make([]uuid.UUID, 0)
	for _, l := range lines {
		if l.SourceType == nil || *l.SourceType != statementOrderSource || l.SourceUUID == nil {
			continue
		}
		if !seen[*l.SourceUUID] {
			seen[*l.SourceUUID] = true
			ids = append(ids, *l.SourceUUID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	items, err := s.q.ListStatementOrderItems(ctx, db.ListStatementOrderItemsParams{OrderUuids: ids, OrganizationID: bookID})
	if err != nil {
		return fmt.Errorf("accounting: statement orders: %w", err)
	}
	itemIDs := make([]int64, 0, len(items))
	for _, it := range items {
		itemIDs = append(itemIDs, it.ItemID)
	}
	units := map[int64][]StatementOrderUnit{}
	if len(itemIDs) > 0 {
		rows, err := s.q.ListStatementOrderItemUnits(ctx, itemIDs)
		if err != nil {
			return fmt.Errorf("accounting: statement order units: %w", err)
		}
		for _, r := range rows {
			units[r.OrderItemID] = append(units[r.OrderItemID], StatementOrderUnit{
				UUID: r.UnitUuid, Barcode: r.Barcode, Quantity: int4Ptr(r.Quantity), Meters: numericPtr(r.Meters, 2),
			})
		}
	}
	orders := map[uuid.UUID]*StatementOrder{}
	for _, it := range items {
		o, ok := orders[it.OrderUuid]
		if !ok {
			o = &StatementOrder{
				UUID: it.OrderUuid, OrderNo: it.OrderNo, Status: it.OrderStatus, Currency: it.Currency,
				Items: []StatementOrderItem{},
			}
			orders[it.OrderUuid] = o
		}
		u := units[it.ItemID]
		if u == nil {
			u = []StatementOrderUnit{}
		}
		o.Items = append(o.Items, StatementOrderItem{
			ProductUUID: it.ProductUuid, ProductSKU: it.ProductSku, ProductName: it.ProductName, UnitType: it.UnitType,
			Quantity: int4Ptr(it.Quantity), Meters: numericPtr(it.Meters, 2),
			UnitPrice: numericString(it.UnitPrice, 4), LineTotal: numericString(it.LineTotal, 2),
			Units: u,
		})
	}
	for i := range lines {
		l := &lines[i]
		if l.SourceType == nil || *l.SourceType != statementOrderSource || l.SourceUUID == nil {
			continue
		}
		if o, ok := orders[*l.SourceUUID]; ok {
			l.Order = o
		}
	}
	return nil
}

func int4Ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	n := v.Int32
	return &n
}

// numericString renders a stored NUMERIC with scale decimals ("" when NULL).
func numericString(n pgtype.Numeric, scale int) string {
	if !n.Valid || n.Int == nil {
		return ""
	}
	r := new(big.Rat).SetInt(n.Int)
	if n.Exp > 0 {
		r.Mul(r, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n.Exp)), nil)))
	} else if n.Exp < 0 {
		r.Quo(r, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-n.Exp)), nil)))
	}
	return r.FloatString(scale)
}

func numericPtr(n pgtype.Numeric, scale int) *string {
	if !n.Valid {
		return nil
	}
	v := numericString(n, scale)
	return &v
}
