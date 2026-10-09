package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/ubl"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
)

// errPDFRendererMissing is written to einvoices.error when no Gotenberg
// client is configured.
var errPDFRendererMissing = errors.New("pdf renderer is not configured")

// File is a downloadable invoice file.
type File struct {
	Body        io.ReadCloser
	Size        int64
	Filename    string
	ContentType string
}

func (s *Service) archivedRow(ctx context.Context, c Caller, id uuid.UUID) (db.Einvoice, error) {
	if err := c.center(); err != nil {
		return db.Einvoice{}, err
	}
	row, err := s.invoice(ctx, s.q, c, id, false)
	if err != nil {
		return db.Einvoice{}, err
	}
	if !row.XmlStorageKey.Valid || (row.Status != StatusArchived && row.Status != StatusVoided) {
		return db.Einvoice{}, ErrNotArchived
	}
	if s.store == nil {
		return db.Einvoice{}, ErrStorageUnavailable
	}
	return row, nil
}

// XML streams the archived UBL-TR XML.
func (s *Service) XML(ctx context.Context, c Caller, id uuid.UUID) (File, error) {
	row, err := s.archivedRow(ctx, c, id)
	if err != nil {
		return File{}, err
	}
	rc, size, err := s.store.Download(ctx, row.XmlStorageKey.String)
	if err != nil {
		return File{}, err
	}
	return File{Body: rc, Size: size, Filename: row.Number + ".xml", ContentType: "application/xml"}, nil
}

// PDF streams the rendered PDF (ErrPDFNotReady while pending or failed).
func (s *Service) PDF(ctx context.Context, c Caller, id uuid.UUID) (File, error) {
	row, err := s.archivedRow(ctx, c, id)
	if err != nil {
		return File{}, err
	}
	if !row.PdfStorageKey.Valid {
		return File{}, ErrPDFNotReady
	}
	rc, size, err := s.store.Download(ctx, row.PdfStorageKey.String)
	if err != nil {
		return File{}, err
	}
	return File{Body: rc, Size: size, Filename: row.Number + ".pdf", ContentType: "application/pdf"}, nil
}

// HTML renders the archived XML with the center's stylesheet.
func (s *Service) HTML(ctx context.Context, c Caller, id uuid.UUID) ([]byte, error) {
	row, err := s.archivedRow(ctx, c, id)
	if err != nil {
		return nil, err
	}
	return s.renderArchived(ctx, row)
}

func (s *Service) renderArchived(ctx context.Context, row db.Einvoice) ([]byte, error) {
	rc, _, err := s.store.Download(ctx, row.XmlStorageKey.String)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	xml, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	var st *db.EinvoiceSetting
	if got, err := s.q.GetEinvoiceSettingsByOrg(ctx, db.GetEinvoiceSettingsByOrgParams{
		OrganizationID: row.OrganizationID, BrandID: row.BrandID,
	}); err == nil {
		st = &got
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	html, err := ubl.RenderHTML(ctx, xml, s.stylesheet(ctx, st))
	if err != nil {
		return nil, err
	}
	if row.Status == StatusVoided {
		html = Watermark(html, "VOID")
	}
	return html, nil
}

// RetryPDF renders the PDF of an archived invoice again (after a failure or
// to refresh it with a new stylesheet).
func (s *Service) RetryPDF(ctx context.Context, c Caller, id uuid.UUID) (InvoiceView, error) {
	row, err := s.archivedRow(ctx, c, id)
	if err != nil {
		return InvoiceView{}, err
	}
	if err := activity.Write(ctx, s.q, c.actor(), ActionPDFRetried, activityResource, &row.Uuid, map[string]any{
		"number": row.Number, "previous_error": row.Error.String,
	}, c.Meta); err != nil {
		return InvoiceView{}, err
	}
	s.schedulePDF(ctx, row.ID)
	if s.queue == nil {
		if row, err = s.invoice(ctx, s.q, c, id, false); err != nil {
			return InvoiceView{}, err
		}
	}
	return s.view(ctx, s.q, row)
}

// schedulePDF queues the PDF render on the docs queue, or renders it now
// without a queue. Failures are recorded on the invoice, never returned.
func (s *Service) schedulePDF(ctx context.Context, invoiceID int64) {
	if s.queue == nil {
		if err := s.GeneratePDF(ctx, invoiceID); err != nil {
			s.log.Warn("einvoice_pdf_failed", "einvoice_id", invoiceID, "error", err)
		}
		return
	}
	task, err := queue.NewEinvoicePDFTask(invoiceID)
	if err == nil {
		_, err = s.queue.Enqueue(task, queue.EinvoicePDFOpts()...)
	}
	if err != nil {
		s.log.Warn("einvoice_pdf_enqueue_failed", "einvoice_id", invoiceID, "error", err)
		s.recordPDFError(ctx, invoiceID, fmt.Errorf("enqueue: %w", err))
	}
}

// GeneratePDF is the docs queue handler of einvoice:pdf: the archived XML →
// XSLT HTML → Gotenberg → einvoices/{year}/{number}.pdf. A failure is
// written to einvoices.error (the status never changes) and returned so the
// queue retries; a later success clears it.
func (s *Service) GeneratePDF(ctx context.Context, invoiceID int64) error {
	row, err := s.q.GetEinvoiceByID(ctx, invoiceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !row.XmlStorageKey.Valid || (row.Status != StatusArchived && row.Status != StatusVoided) {
		return nil
	}
	if err := s.renderPDF(ctx, row); err != nil {
		s.recordPDFError(ctx, row.ID, err)
		return err
	}
	return nil
}

func (s *Service) renderPDF(ctx context.Context, row db.Einvoice) error {
	if s.store == nil {
		return ErrStorageUnavailable
	}
	if s.pdf == nil {
		return errPDFRendererMissing
	}
	html, err := s.renderArchived(ctx, row)
	if err != nil {
		return err
	}
	data, err := s.pdf.Convert(ctx, pdfrender.Request{HTML: string(html)})
	if err != nil {
		return err
	}
	key := PDFObjectKey(row.IssueDate.Time.Year(), row.Number)
	if err := s.store.Upload(ctx, storage.File{
		Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: "application/pdf",
		Filename: row.Number + ".pdf", Metadata: map[string]string{"ettn": row.Uuid.String()},
	}, key); err != nil {
		return err
	}
	_, err = s.q.SetEinvoicePDF(ctx, db.SetEinvoicePDFParams{ID: row.ID, PdfStorageKey: pgtype.Text{String: key, Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

func (s *Service) recordPDFError(ctx context.Context, invoiceID int64, cause error) {
	msg := "pdf: " + strings.TrimSpace(cause.Error())
	if len(msg) > 1000 {
		msg = msg[:1000]
	}
	if err := s.q.SetEinvoicePDFError(ctx, db.SetEinvoicePDFErrorParams{
		ID: invoiceID, Error: pgtype.Text{String: msg, Valid: true},
	}); err != nil {
		s.log.Warn("einvoice_pdf_error_record_failed", "einvoice_id", invoiceID, "error", err)
	}
}
