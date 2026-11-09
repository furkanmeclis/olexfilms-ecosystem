package usecase

// Manual "invoiced" marks (TEC-1030). The seller's accounting records the
// external invoice (number, date) it cut for a shipped, delivered or
// received order (TEC-1070: like legacy, the invoice may be cut while the
// goods are on the road), per line:
// one order_invoices row per invoice, its lines in order_invoice_items.
// orders.invoice_status is the projection (pending, partial, invoiced).
// Only the seller side marks; the e-invoice module hides an order marked
// invoiced from its billable list. A mark is undone by deleting the
// invoice record (a typo in the number, the wrong lines).

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Invoice statuses of an order (orders.invoice_status).
const (
	InvoiceStatusPending  = "pending"
	InvoiceStatusPartial  = "partial"
	InvoiceStatusInvoiced = "invoiced"
)

// InvoiceStatuses lists the invoice statuses (list filter whitelist).
var InvoiceStatuses = []string{InvoiceStatusPending, InvoiceStatusPartial, InvoiceStatusInvoiced}

// QueryInvoiceStatus is the list filter of the invoice status (CSV).
const QueryInvoiceStatus = "invoice_status"

const (
	invoiceNoMax   = 64
	invoiceNoteMax = 1000
)

var (
	// ErrNotInvoiceable: only a shipped, delivered or received order is
	// marked invoiced.
	ErrNotInvoiceable = errors.New("orders: only a shipped, delivered or received order can be marked invoiced")
	// ErrOrderInvoiced: an order with an invoice record is not cancelled
	// (legacy: a return / cancel invoice is needed); delete the record first.
	ErrOrderInvoiced = errors.New("orders: an invoiced order cannot be cancelled")
	// ErrItemAlreadyInvoiced: a selected line is on an invoice already.
	ErrItemAlreadyInvoiced = errors.New("orders: the order line is already invoiced")
	// ErrNothingToInvoice: every line of the order is invoiced.
	ErrNothingToInvoice = errors.New("orders: every line is invoiced")
	// ErrInvoiceNotFound: no such invoice record on the order.
	ErrInvoiceNotFound = errors.New("orders: invoice record not found")
	// ErrInvoiceNoTaken: the order already has an invoice with that number.
	ErrInvoiceNoTaken = errors.New("orders: invoice number already recorded on this order")
)

// OrderInvoiceView is one manual invoice record of an order.
type OrderInvoiceView struct {
	UUID        uuid.UUID   `json:"uuid"`
	InvoiceNo   string      `json:"invoice_no"`
	InvoiceDate string      `json:"invoice_date"`
	Note        *string     `json:"note"`
	ItemUUIDs   []uuid.UUID `json:"item_uuids"`
	CreatedBy   *string     `json:"created_by"`
	CreatedAt   time.Time   `json:"created_at"`
}

// MarkInvoicedInput is the body of POST /v1/orders/{uuid}/invoices.
type MarkInvoicedInput struct {
	InvoiceNo   string
	InvoiceDate string
	Note        *string
	// ItemUUIDs are the lines on the invoice; empty means every line not
	// invoiced yet.
	ItemUUIDs []string
}

func (in MarkInvoicedInput) validate() (string, pgtype.Date, pgtype.Text, error) {
	no := strings.TrimSpace(in.InvoiceNo)
	if no == "" || len(no) > invoiceNoMax {
		return "", pgtype.Date{}, pgtype.Text{}, invalid("invoice_no", fmt.Sprintf("must be 1-%d characters", invoiceNoMax))
	}
	day, err := time.Parse(time.DateOnly, strings.TrimSpace(in.InvoiceDate))
	if err != nil {
		return "", pgtype.Date{}, pgtype.Text{}, invalid("invoice_date", "must be a date (YYYY-MM-DD)")
	}
	var note pgtype.Text
	if in.Note != nil {
		v := strings.TrimSpace(*in.Note)
		if len(v) > invoiceNoteMax {
			return "", pgtype.Date{}, pgtype.Text{}, invalid("note", fmt.Sprintf("must be at most %d characters", invoiceNoteMax))
		}
		if v != "" {
			note = pgtype.Text{String: v, Valid: true}
		}
	}
	return no, pgtype.Date{Time: day, Valid: true}, note, nil
}

// Invoiceable reports whether an order in status may get an invoice mark:
// shipped, delivered or received (TEC-1070).
func Invoiceable(status string) bool {
	return status == StatusShipped || status == StatusDelivered || status == StatusReceived
}

// invoicedGuard drops the cancel moves of an order with an invoice mark
// (ErrOrderInvoiced).
func invoicedGuard(o db.Order, moves []string) []string {
	if o.InvoiceStatus == InvoiceStatusPending {
		return moves
	}
	out := moves[:0:0]
	for _, m := range moves {
		if m != StatusCancelling && m != StatusCancelled {
			out = append(out, m)
		}
	}
	return out
}

// MarkInvoiced records an invoice on the lines of a shipped, delivered or
// received order the caller sells; the order's invoice status follows.
func (s *Service) MarkInvoiced(ctx context.Context, c Caller, orderUUID uuid.UUID, in MarkInvoicedInput) (OrderView, error) {
	no, day, note, err := in.validate()
	if err != nil {
		return OrderView{}, err
	}
	wanted := make([]uuid.UUID, 0, len(in.ItemUUIDs))
	for i, raw := range in.ItemUUIDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			return OrderView{}, invalid(fmt.Sprintf("item_uuids[%d]", i), "must be a UUID")
		}
		wanted = append(wanted, id)
	}
	var out db.Order
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		o, err := s.lockVisible(ctx, q, c, orderUUID)
		if err != nil {
			return err
		}
		if partyOf(c, o) != PartySeller {
			return ErrForbidden
		}
		if !Invoiceable(o.Status) {
			return ErrNotInvoiceable
		}
		items, err := q.ListOrderItems(ctx, o.ID)
		if err != nil {
			return fmt.Errorf("orders: lines: %w", err)
		}
		invoiced, err := q.ListOrderInvoiceItemsByOrder(ctx, o.ID)
		if err != nil {
			return fmt.Errorf("orders: invoiced lines: %w", err)
		}
		done := make(map[int64]bool, len(invoiced))
		for _, r := range invoiced {
			done[r.OrderItemID] = true
		}
		byUUID := make(map[uuid.UUID]db.OrderItem, len(items))
		for _, it := range items {
			byUUID[it.Uuid] = it
		}
		var targets []int64
		if len(wanted) == 0 {
			for _, it := range items {
				if !done[it.ID] {
					targets = append(targets, it.ID)
				}
			}
			if len(targets) == 0 {
				return ErrNothingToInvoice
			}
		} else {
			seen := map[int64]bool{}
			for _, id := range wanted {
				it, ok := byUUID[id]
				if !ok {
					return ErrItemNotFound
				}
				if done[it.ID] {
					return ErrItemAlreadyInvoiced
				}
				if !seen[it.ID] {
					seen[it.ID] = true
					targets = append(targets, it.ID)
				}
			}
		}
		inv, err := q.CreateOrderInvoice(ctx, db.CreateOrderInvoiceParams{
			OrderID: o.ID, OrganizationID: o.OrganizationID, BrandID: o.BrandID,
			InvoiceNo: no, InvoiceDate: day, Note: note, CreatedBy: c.actor(),
		})
		if isUniqueViolation(err) {
			return ErrInvoiceNoTaken
		}
		if err != nil {
			return fmt.Errorf("orders: invoice: %w", err)
		}
		for _, id := range targets {
			if err := q.CreateOrderInvoiceItem(ctx, db.CreateOrderInvoiceItemParams{OrderInvoiceID: inv.ID, OrderItemID: id}); err != nil {
				if isUniqueViolation(err) {
					return ErrItemAlreadyInvoiced
				}
				return fmt.Errorf("orders: invoice line: %w", err)
			}
		}
		out, err = s.projectInvoiceStatus(ctx, q, o)
		return err
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.view(ctx, s.q, c, out)
}

// DeleteInvoice removes an invoice record (and its line marks) of an order
// the caller sells.
func (s *Service) DeleteInvoice(ctx context.Context, c Caller, orderUUID, invoiceUUID uuid.UUID) (OrderView, error) {
	var out db.Order
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		o, err := s.lockVisible(ctx, q, c, orderUUID)
		if err != nil {
			return err
		}
		if partyOf(c, o) != PartySeller {
			return ErrForbidden
		}
		inv, err := q.GetOrderInvoiceByUUID(ctx, db.GetOrderInvoiceByUUIDParams{Uuid: invoiceUUID, OrderID: o.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvoiceNotFound
		}
		if err != nil {
			return fmt.Errorf("orders: invoice: %w", err)
		}
		if _, err := q.DeleteOrderInvoice(ctx, inv.ID); err != nil {
			return fmt.Errorf("orders: delete invoice: %w", err)
		}
		out, err = s.projectInvoiceStatus(ctx, q, o)
		return err
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.view(ctx, s.q, c, out)
}

// projectInvoiceStatus recomputes orders.invoice_status from the line marks.
func (s *Service) projectInvoiceStatus(ctx context.Context, q *db.Queries, o db.Order) (db.Order, error) {
	total, err := q.CountOrderItems(ctx, o.ID)
	if err != nil {
		return db.Order{}, fmt.Errorf("orders: count lines: %w", err)
	}
	marked, err := q.CountInvoicedOrderItems(ctx, o.ID)
	if err != nil {
		return db.Order{}, fmt.Errorf("orders: count invoiced lines: %w", err)
	}
	status := InvoiceStatusPending
	switch {
	case total > 0 && marked >= total:
		status = InvoiceStatusInvoiced
	case marked > 0:
		status = InvoiceStatusPartial
	}
	out, err := q.SetOrderInvoiceStatus(ctx, db.SetOrderInvoiceStatusParams{InvoiceStatus: status, ID: o.ID})
	if err != nil {
		return db.Order{}, fmt.Errorf("orders: invoice status: %w", err)
	}
	return out, nil
}

// invoiceViews loads the invoice records of an order and the invoice of
// every marked line (by order_items.id).
func invoiceViews(ctx context.Context, q *db.Queries, orderID int64) ([]OrderInvoiceView, map[int64]db.ListOrderInvoiceItemsByOrderRow, error) {
	rows, err := q.ListOrderInvoices(ctx, orderID)
	if err != nil {
		return nil, nil, fmt.Errorf("orders: invoices: %w", err)
	}
	lines, err := q.ListOrderInvoiceItemsByOrder(ctx, orderID)
	if err != nil {
		return nil, nil, fmt.Errorf("orders: invoiced lines: %w", err)
	}
	itemUUID := map[int64]uuid.UUID{}
	if len(lines) > 0 {
		items, err := q.ListOrderItems(ctx, orderID)
		if err != nil {
			return nil, nil, fmt.Errorf("orders: lines: %w", err)
		}
		for _, it := range items {
			itemUUID[it.ID] = it.Uuid
		}
	}
	byInvoice := map[int64][]uuid.UUID{}
	byItem := make(map[int64]db.ListOrderInvoiceItemsByOrderRow, len(lines))
	for _, l := range lines {
		byItem[l.OrderItemID] = l
		byInvoice[l.OrderInvoiceID] = append(byInvoice[l.OrderInvoiceID], itemUUID[l.OrderItemID])
	}
	out := make([]OrderInvoiceView, 0, len(rows))
	for _, r := range rows {
		v := OrderInvoiceView{
			UUID: r.Uuid, InvoiceNo: r.InvoiceNo, Note: textPtr(r.Note), CreatedAt: r.CreatedAt.Time,
			ItemUUIDs: byInvoice[r.ID],
		}
		if v.ItemUUIDs == nil {
			v.ItemUUIDs = []uuid.UUID{}
		}
		if r.InvoiceDate.Valid {
			v.InvoiceDate = r.InvoiceDate.Time.Format(time.DateOnly)
		}
		if r.CreatedByName.Valid {
			name := strings.TrimSpace(r.CreatedByName.String + " " + r.CreatedBySurname.String)
			v.CreatedBy = &name
		}
		out = append(out, v)
	}
	return out, byItem, nil
}
