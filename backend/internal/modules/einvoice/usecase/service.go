// Package usecase is the e-Invoice (UBL-TR) use case of the brand center
// (TEC-503, F5-08c).
//
// The center turns its billable sales (received orders to distributors and
// posted service catalog subscription periods of dealers, QUESTIONS S27)
// into invoices. A draft reserves the source (one active invoice per source,
// uq_einvoices_source_active) under a temporary TMP number; the preview
// renders it without a series number and with a PREVIEW watermark.
// Archiving builds the XML with the temporary number and validates it (XSD +
// Schematron); only a valid invoice takes the next series number from the
// counter, in the same transaction, and is rebuilt with it, so a failed
// validation never consumes a number (QUESTIONS S29). The XML is stored at
// einvoices/{year}/{number}.xml with its sha256; the PDF (XSLT HTML →
// Gotenberg) is rendered on the docs queue and a failure only lands in
// einvoices.error. Voiding is a mark: the row and its files stay and the
// source becomes billable again. No ledger row is written (the source's
// finance entry is referenced) and nothing is sent to GİB or an integrator
// (design §9).
package usecase

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/ubl"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Source types (einvoices.source_type).
const (
	SourceOrder        = "order"
	SourceSubscription = "service_subscription"
)

// SourceTypes are the billable source types.
var SourceTypes = []string{SourceOrder, SourceSubscription}

// Invoice statuses (einvoices.status).
const (
	StatusDraft    = "draft"
	StatusArchived = "archived"
	StatusFailed   = "failed"
	StatusVoided   = "voided"
)

// Statuses are the invoice statuses in lifecycle order.
var Statuses = []string{StatusDraft, StatusFailed, StatusArchived, StatusVoided}

// Profiles are the UBL profiles of the archive.
var Profiles = []string{ubl.ProfileEArchive, ubl.ProfileBasic, ubl.ProfileCommercial}

// DraftSeries is the series of the temporary draft numbers. It is reserved:
// the e-Arşiv / e-Fatura series cannot use it.
const DraftSeries = "TMP"

// MaxXSLTBytes is the upload limit of a custom stylesheet.
const MaxXSLTBytes = 1 << 20

// Activity actions.
const (
	ActionDraftCreated        = "einvoice.draft_created"
	ActionArchived            = "einvoice.archived"
	ActionArchiveFailed       = "einvoice.archive_failed"
	ActionVoided              = "einvoice.voided"
	ActionPDFRetried          = "einvoice.pdf_retried"
	ActionSettingsUpdated     = "einvoice.settings_updated"
	ActionXSLTUploaded        = "einvoice.xslt_uploaded"
	ActionXSLTReset           = "einvoice.xslt_reset"
	ActionBuyerProfileUpdated = "einvoice.buyer_profile_updated"
)

const activityResource = "einvoices"

var (
	// ErrNotFound: the invoice, source or organization is not the
	// caller's (404).
	ErrNotFound = errors.New("einvoice: not found")
	// ErrForbidden: only the brand center invoices (403).
	ErrForbidden = errors.New("einvoice: only the brand center can issue e-invoices")
	// ErrSettingsRequired: the seller profile and series are not set up
	// yet (422 EINVOICE_SETTINGS_REQUIRED, QUESTIONS S31).
	ErrSettingsRequired = errors.New("einvoice: e-invoice settings are not configured")
	// ErrSourceNotBillable: the source exists but cannot be invoiced (an
	// order not received yet, a period not posted, a buyer of another
	// level) (422 EINVOICE_SOURCE_NOT_BILLABLE).
	ErrSourceNotBillable = errors.New("einvoice: the source is not billable")
	// ErrAlreadyInvoiced: the source has an active invoice (409
	// EINVOICE_SOURCE_ALREADY_INVOICED).
	ErrAlreadyInvoiced = errors.New("einvoice: the source already has an active invoice")
	// ErrBuyerProfileIncomplete: the buyer's invoice profile misses fields
	// (422 EINVOICE_BUYER_PROFILE_INCOMPLETE; *ProfileError lists them).
	ErrBuyerProfileIncomplete = errors.New("einvoice: the buyer invoice profile is incomplete")
	// ErrInvalidStatus: the action does not fit the invoice status (409
	// EINVOICE_INVALID_STATUS).
	ErrInvalidStatus = errors.New("einvoice: the invoice status does not allow this action")
	// ErrNotArchived: there is no archived XML yet (409
	// EINVOICE_NOT_ARCHIVED).
	ErrNotArchived = errors.New("einvoice: the invoice is not archived")
	// ErrPDFNotReady: the PDF is pending or failed (409
	// EINVOICE_PDF_NOT_READY).
	ErrPDFNotReady = errors.New("einvoice: the invoice PDF is not ready")
	// ErrStorageUnavailable: no object storage is configured (503).
	ErrStorageUnavailable = errors.New("einvoice: storage unavailable")
)

// ProfileError lists the missing buyer invoice profile fields.
type ProfileError struct {
	OrganizationUUID uuid.UUID
	Fields           []string
}

func (e *ProfileError) Error() string {
	return ErrBuyerProfileIncomplete.Error() + ": " + strings.Join(e.Fields, ", ")
}

func (e *ProfileError) Unwrap() error { return ErrBuyerProfileIncomplete }

// FieldIssue is one invalid input field (400 VALIDATION_ERROR).
type FieldIssue struct {
	Field   string
	Message string
}

// ValidationError is an input error (400 VALIDATION_ERROR).
type ValidationError struct {
	Issues []FieldIssue
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Issues))
	for _, i := range e.Issues {
		parts = append(parts, i.Field+": "+i.Message)
	}
	return "einvoice: invalid input: " + strings.Join(parts, "; ")
}

func invalid(field, msg string) error {
	return &ValidationError{Issues: []FieldIssue{{Field: field, Message: msg}}}
}

// TxBeginner opens transactions (*pgxpool.Pool, or a pgx.Tx in tests).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// PDFRenderer converts HTML to PDF (*pdfrender.Client).
type PDFRenderer interface {
	Convert(ctx context.Context, req pdfrender.Request) ([]byte, error)
}

// Enqueuer schedules the docs queue task (*queue.Client).
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// IntSettings reads integer system settings (*sysconfig.Service).
type IntSettings interface {
	Int(ctx context.Context, key string) int64
}

// Validator validates invoice XML (ubl.Validate; tests replace it).
type Validator func(ctx context.Context, xml []byte, profile string) (ubl.Validation, error)

// Caller is the authenticated center member.
type Caller struct {
	Principal authctx.Principal
	Org       orgctx.Scope
	Meta      activity.Meta
}

func (c Caller) center() error {
	if c.Org.OrgType != "center" {
		return ErrForbidden
	}
	return nil
}

func (c Caller) actor() *int64 {
	if c.Principal.UserInternal == 0 {
		return nil
	}
	id := c.Principal.UserInternal
	return &id
}

func (c Caller) actorArg() pgtype.Int8 {
	if c.Principal.UserInternal == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: c.Principal.UserInternal, Valid: true}
}

// Service is the e-invoice use case.
type Service struct {
	tx       TxBeginner
	q        *db.Queries
	store    storage.Driver
	out      outbox.Enqueuer
	pdf      PDFRenderer
	queue    Enqueuer
	sys      IntSettings
	validate Validator
	now      func() time.Time
	log      *slog.Logger
}

// New builds the use case. out may be nil (no events).
func New(tx TxBeginner, q *db.Queries, store storage.Driver, out outbox.Enqueuer, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{tx: tx, q: q, store: store, out: out, validate: ubl.Validate, now: time.Now, log: log}
}

// WithPDF sets the PDF renderer and the docs queue. With a nil queue the
// PDF is rendered right after archiving (tests, in-process setups).
func (s *Service) WithPDF(pdf PDFRenderer, queue Enqueuer) *Service {
	s.pdf, s.queue = pdf, queue
	return s
}

// WithSettings sets the system settings (einvoice.default_vat_rate).
func (s *Service) WithSettings(sys IntSettings) *Service {
	s.sys = sys
	return s
}

// SetValidator replaces the XML validator (tests).
func (s *Service) SetValidator(v Validator) { s.validate = v }

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) emit(ctx context.Context, tx pgx.Tx, name string, row db.Einvoice, actor *int64, payload map[string]any) error {
	if s.out == nil {
		return nil
	}
	id, uid := row.ID, row.Uuid
	ev := events.New(name).WithTenant(row.OrganizationID).WithEntity("einvoice", &id, &uid).WithPayload(payload)
	if actor != nil {
		ev = ev.WithActor(*actor)
	}
	return s.out.Enqueue(ctx, tx, ev)
}

func (s *Service) settings(ctx context.Context, q *db.Queries, c Caller) (db.EinvoiceSetting, error) {
	st, err := q.GetEinvoiceSettingsByOrg(ctx, db.GetEinvoiceSettingsByOrgParams{
		OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.EinvoiceSetting{}, ErrSettingsRequired
	}
	return st, err
}

// invoice loads an invoice of the caller's center.
func (s *Service) invoice(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID, lock bool) (db.Einvoice, error) {
	var (
		row db.Einvoice
		err error
	)
	if lock {
		row, err = q.GetEinvoiceForUpdate(ctx, db.GetEinvoiceForUpdateParams{Uuid: id, BrandID: c.Org.BrandID})
	} else {
		row, err = q.GetEinvoiceByUUID(ctx, db.GetEinvoiceByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	}
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.OrganizationID != c.Org.InternalID) {
		return db.Einvoice{}, ErrNotFound
	}
	return row, err
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

func text(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

// trTime is Türkiye time (no DST since 2016); invoice dates and number years
// follow it.
var trTime = time.FixedZone("TRT", 3*60*60)
