package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskEinvoicePDF renders the PDF of an archived e-invoice (TEC-503,
// F5-08c): XSLT HTML → Gotenberg → einvoices/{year}/{number}.pdf. A failure
// is written to einvoices.error and never changes the invoice status.
const TaskEinvoicePDF = "einvoice:pdf"

// EinvoicePDFPayload names the invoice to render.
type EinvoicePDFPayload struct {
	InvoiceID int64 `json:"invoice_id"`
}

// EinvoicePDFFunc renders the PDF of one invoice.
type EinvoicePDFFunc func(ctx context.Context, invoiceID int64) error

// NewEinvoicePDFTask builds the e-invoice PDF task.
func NewEinvoicePDFTask(invoiceID int64) (*asynq.Task, error) {
	body, err := json.Marshal(EinvoicePDFPayload{InvoiceID: invoiceID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal einvoice pdf: %w", err)
	}
	return asynq.NewTask(TaskEinvoicePDF, body), nil
}

// ParseEinvoicePDFPayload decodes an e-invoice PDF payload.
func ParseEinvoicePDFPayload(data []byte) (EinvoicePDFPayload, error) {
	var payload EinvoicePDFPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return EinvoicePDFPayload{}, fmt.Errorf("queue: unmarshal einvoice pdf: %w", err)
	}
	return payload, nil
}

// EinvoicePDFOpts are the enqueue options: docs queue (worker-docs),
// retried on Gotenberg/storage failures. There is no task id: the retry
// endpoint must be able to queue again after an archived failure, and the
// handler is idempotent (the same key is overwritten).
func EinvoicePDFOpts() []asynq.Option {
	return []asynq.Option{
		asynq.Queue(QueueDocs),
		asynq.MaxRetry(3),
		asynq.Timeout(2 * time.Minute),
	}
}

// WithEinvoicePDF sets the einvoice:pdf processor; a repeated call replaces
// it.
func (w *Worker) WithEinvoicePDF(fn EinvoicePDFFunc) *Worker {
	w.einvoicePDF = fn
	return w
}

func (w *Worker) handleEinvoicePDF(ctx context.Context, task *asynq.Task) error {
	payload, err := ParseEinvoicePDFPayload(task.Payload())
	if err != nil {
		return err
	}
	if w.einvoicePDF == nil {
		w.log.Warn("einvoice_pdf_handler_missing", "invoice_id", payload.InvoiceID)
		return nil
	}
	return w.einvoicePDF(ctx, payload.InvoiceID)
}
