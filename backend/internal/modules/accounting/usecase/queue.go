package usecase

// Accounting queue (TEC-1070; legacy olexfilms-warehouse "Muhasebe
// kuyruğu"): what the book's accounting still has to process. Orders it sold
// wait for an invoice mark (TEC-1030, orders.invoice_status; the mark itself
// stays POST /v1/orders/{uuid}/invoices), returns it accepted wait to be
// marked processed (credit note cut outside the app). The queue always works
// on the active organization's own book.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrReturnNotFound: no received return to the book with that uuid.
var ErrReturnNotFound = errors.New("accounting: return not found in the queue")

const queueNoteMax = 1000

// QueueCounts is GET /v1/accounting/queue/summary.
type QueueCounts struct {
	// OrdersToInvoice: received sales not fully invoiced (markable now).
	OrdersToInvoice int64 `json:"orders_to_invoice"`
	// OrdersInTransit: shipped or delivered sales not invoiced yet.
	OrdersInTransit int64 `json:"orders_in_transit"`
	// ReturnsPending: received returns not processed yet.
	ReturnsPending int64 `json:"returns_pending"`
}

// QueueSummary counts the queue of the active organization's book.
func (s *Service) QueueSummary(ctx context.Context, c Caller) (QueueCounts, error) {
	book, err := s.activeOrg(ctx, c)
	if err != nil {
		return QueueCounts{}, err
	}
	r, err := s.q.AccountingQueueCounts(ctx, book.ID)
	if err != nil {
		return QueueCounts{}, fmt.Errorf("accounting: queue counts: %w", err)
	}
	return QueueCounts{OrdersToInvoice: r.ToInvoice, OrdersInTransit: r.InTransit, ReturnsPending: r.ReturnsPending}, nil
}

// QueueReturnSort is the sort contract of the return queue.
var QueueReturnSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"received_at": "received_at", "transfer_no": "transfer_no", "processed_at": "processed_at"},
	Default: apiquery.SortField{Field: "received_at", Desc: true},
}

// QueueReturnFilter narrows the return queue.
type QueueReturnFilter struct {
	// Processed: nil every received return, true the processed, false the waiting ones.
	Processed     *bool
	Q             string
	Sort          apiquery.ResolvedSort
	Limit, Offset int32
}

// QueueReturn is one received return of the queue.
type QueueReturn struct {
	UUID        uuid.UUID    `json:"uuid"`
	TransferNo  string       `json:"transfer_no"`
	Sender      Counterparty `json:"sender"`
	ItemCount   int64        `json:"item_count"`
	Currency    string       `json:"currency"`
	Total       *string      `json:"total"`
	ReceivedAt  *time.Time   `json:"received_at"`
	CreatedAt   time.Time    `json:"created_at"`
	ProcessedAt *time.Time   `json:"processed_at"`
	ProcessedBy *string      `json:"processed_by"`
	Note        *string      `json:"note"`
	// Order (TEC-1085): the order the return refers to, or null.
	Order *QueueReturnOrder `json:"order"`
}

// QueueReturnOrder names the order of a queued return (TEC-1085).
type QueueReturnOrder struct {
	UUID    uuid.UUID `json:"uuid"`
	OrderNo string    `json:"order_no"`
}

// ParseQueueReturnFilter reads processed (true|false), q, sort, limit, offset.
func ParseQueueReturnFilter(values map[string][]string) (QueueReturnFilter, error) {
	q := apiquery.Parse(values)
	f := QueueReturnFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	switch v := strings.TrimSpace(first(values, "processed")); v {
	case "":
	case "true", "false":
		b := v == "true"
		f.Processed = &b
	default:
		return f, invalid("processed", "must be true or false")
	}
	sort, err := apiquery.ResolveSort(q.Sort, QueueReturnSort)
	if err != nil {
		return f, err
	}
	f.Sort = sort
	return f, nil
}

func first(values map[string][]string, key string) string {
	if v := values[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

func optBool(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

func optText(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

// ListQueueReturns lists the received returns to the book.
func (s *Service) ListQueueReturns(ctx context.Context, c Caller, f QueueReturnFilter) ([]QueueReturn, int64, error) {
	book, err := s.activeOrg(ctx, c)
	if err != nil {
		return nil, 0, err
	}
	processed, q := optBool(f.Processed), optText(f.Q)
	rows, err := s.q.ListAccountingQueueReturns(ctx, db.ListAccountingQueueReturnsParams{
		OrganizationID: book.ID, Processed: processed, Q: q,
		SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, PageLimit: f.Limit, PageOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("accounting: queue returns: %w", err)
	}
	total, err := s.q.CountAccountingQueueReturns(ctx, db.CountAccountingQueueReturnsParams{
		OrganizationID: book.ID, Processed: processed, Q: q,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("accounting: count queue returns: %w", err)
	}
	out := make([]QueueReturn, 0, len(rows))
	for _, r := range rows {
		sender := r.SenderUuid
		var order *QueueReturnOrder
		if r.OrderUuid.Valid {
			order = &QueueReturnOrder{UUID: uuid.UUID(r.OrderUuid.Bytes), OrderNo: r.OrderNo.String}
		}
		out = append(out, QueueReturn{
			Order: order,
			UUID:  r.Uuid, TransferNo: r.TransferNo,
			Sender:    Counterparty{Type: "organization", UUID: &sender, Name: r.SenderName, OrgType: r.SenderType},
			ItemCount: r.ItemCount, Currency: r.Currency, Total: strPtr(r.Total),
			ReceivedAt: tsPtr(r.ReceivedAt), CreatedAt: r.CreatedAt.Time,
			ProcessedAt: tsPtr(r.AccountingProcessedAt), ProcessedBy: strPtr(r.ProcessedByName),
			Note: textPtr(r.AccountingNote),
		})
	}
	return out, total, nil
}

// MarkReturnProcessed sets (processed) or clears the accounting mark of a
// received return to the active organization's book.
func (s *Service) MarkReturnProcessed(ctx context.Context, c Caller, id uuid.UUID, processed bool, note string) error {
	book, err := s.writeBook(ctx, c)
	if err != nil {
		return err
	}
	note = strings.TrimSpace(note)
	if len(note) > queueNoteMax {
		return invalid("note", fmt.Sprintf("must be at most %d characters", queueNoteMax))
	}
	var user pgtype.Int8
	if a := c.actor(); a != nil {
		user = pgtype.Int8{Int64: *a, Valid: true}
	}
	_, err = s.q.SetReturnAccountingProcessed(ctx, db.SetReturnAccountingProcessedParams{
		Processed: processed, UserID: user, Note: optText(note), Uuid: id, OrganizationID: book.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrReturnNotFound
	}
	if err != nil {
		return fmt.Errorf("accounting: mark return: %w", err)
	}
	return nil
}
