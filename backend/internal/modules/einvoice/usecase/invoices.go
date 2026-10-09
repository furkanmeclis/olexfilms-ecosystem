package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/ubl"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// OrgRef is a short organization reference.
type OrgRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// FinanceEntryRef is the center's ledger row of the invoiced source.
type FinanceEntryRef struct {
	UUID       uuid.UUID `json:"uuid"`
	Direction  string    `json:"direction"`
	Amount     string    `json:"amount"`
	Currency   string    `json:"currency"`
	SourceType string    `json:"source_type"`
}

// InvoiceView is an invoice of the API.
type InvoiceView struct {
	// UUID is the ETTN.
	UUID uuid.UUID `json:"uuid"`
	// Number is the GİB number; nil while the invoice has only a
	// temporary draft number.
	Number             *string          `json:"number"`
	Profile            string           `json:"profile"`
	InvoiceType        string           `json:"invoice_type"`
	Status             string           `json:"status"`
	ValidationStatus   string           `json:"validation_status"`
	ValidationMessages []string         `json:"validation_messages"`
	SourceType         string           `json:"source_type"`
	SourceUUID         uuid.UUID        `json:"source_uuid"`
	BuyerOrganization  *OrgRef          `json:"buyer_organization"`
	Buyer              json.RawMessage  `json:"buyer"`
	Seller             json.RawMessage  `json:"seller"`
	Lines              json.RawMessage  `json:"lines"`
	TaxBreakdown       json.RawMessage  `json:"tax_breakdown"`
	Currency           string           `json:"currency"`
	LineExtension      string           `json:"line_extension"`
	TaxExclusive       string           `json:"tax_exclusive"`
	TaxTotal           string           `json:"tax_total"`
	Payable            string           `json:"payable"`
	IssueDate          string           `json:"issue_date"`
	XMLSHA256          *string          `json:"xml_sha256"`
	HasXML             bool             `json:"has_xml"`
	HasPDF             bool             `json:"has_pdf"`
	Error              *string          `json:"error"`
	VoidedAt           *time.Time       `json:"voided_at"`
	VoidReason         *string          `json:"void_reason"`
	FinanceEntry       *FinanceEntryRef `json:"finance_entry,omitempty"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

func rawJSON(b []byte, fallback string) json.RawMessage {
	if len(bytes.TrimSpace(b)) == 0 {
		return json.RawMessage(fallback)
	}
	return json.RawMessage(b)
}

func viewOf(e db.Einvoice, buyer *OrgRef) InvoiceView {
	v := InvoiceView{
		UUID: e.Uuid, Profile: e.Profile, InvoiceType: e.InvoiceType, Status: e.Status,
		ValidationStatus: e.ValidationStatus, ValidationMessages: []string{},
		SourceType: e.SourceType, SourceUUID: e.SourceUuid, BuyerOrganization: buyer,
		Buyer: rawJSON(e.Buyer, "{}"), Seller: rawJSON(e.Seller, "{}"), Lines: rawJSON(e.Lines, "[]"),
		TaxBreakdown: rawJSON(e.TaxBreakdown, "[]"), Currency: e.Currency,
		LineExtension: money(e.LineExtension), TaxExclusive: money(e.TaxExclusive),
		TaxTotal: money(e.TaxTotal), Payable: money(e.Payable),
		XMLSHA256: textPtr(e.XmlSha256), HasXML: e.XmlStorageKey.Valid, HasPDF: e.PdfStorageKey.Valid,
		Error: textPtr(e.Error), VoidReason: textPtr(e.VoidReason),
		CreatedAt: e.CreatedAt.Time, UpdatedAt: e.UpdatedAt.Time,
	}
	if !strings.HasPrefix(e.Number, DraftSeries) {
		n := e.Number
		v.Number = &n
	}
	if e.IssueDate.Valid {
		v.IssueDate = e.IssueDate.Time.Format(time.DateOnly)
	}
	if e.VoidedAt.Valid {
		t := e.VoidedAt.Time
		v.VoidedAt = &t
	}
	_ = json.Unmarshal(e.ValidationMessages, &v.ValidationMessages)
	if v.ValidationMessages == nil {
		v.ValidationMessages = []string{}
	}
	return v
}

// money renders a NUMERIC(18,2) with two decimals.
func money(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil || n.NaN {
		return "0.00"
	}
	return decimal.NewFromBigInt(n.Int, n.Exp).StringFixed(2)
}

func (s *Service) view(ctx context.Context, q *db.Queries, e db.Einvoice) (InvoiceView, error) {
	var ref *OrgRef
	if e.BuyerOrgID.Valid {
		o, err := q.GetOrganizationByID(ctx, e.BuyerOrgID.Int64)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return InvoiceView{}, err
		}
		if err == nil {
			ref = &OrgRef{UUID: o.Uuid, Name: o.Name}
		}
	}
	return viewOf(e, ref), nil
}

// DraftInput names the source of a new invoice.
type DraftInput struct {
	SourceType string
	SourceUUID uuid.UUID
}

// CreateDraft opens a draft invoice for a billable source: the buyer must
// have a complete invoice profile (422 with the missing fields), the source
// no active invoice (409). The draft carries a temporary TMP number.
func (s *Service) CreateDraft(ctx context.Context, c Caller, in DraftInput) (InvoiceView, error) {
	if err := c.center(); err != nil {
		return InvoiceView{}, err
	}
	tx, err := s.tx.Begin(ctx)
	if err != nil {
		return InvoiceView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)
	st, err := s.settings(ctx, qtx, c)
	if err != nil {
		return InvoiceView{}, err
	}
	src, err := s.loadSource(ctx, qtx, c, in.SourceType, in.SourceUUID)
	if err != nil {
		return InvoiceView{}, err
	}
	if _, err := qtx.GetActiveEinvoiceBySource(ctx, db.GetActiveEinvoiceBySourceParams{
		OrganizationID: c.Org.InternalID, SourceType: src.Type, SourceUuid: src.UUID,
	}); err == nil {
		return InvoiceView{}, ErrAlreadyInvoiced
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return InvoiceView{}, err
	}
	now := s.now()
	doc, profile, err := s.document(ctx, src, st, now)
	if err != nil {
		return InvoiceView{}, err
	}
	tmp, err := qtx.IncrementEinvoiceCounter(ctx, db.IncrementEinvoiceCounterParams{
		OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, Series: DraftSeries, Year: int32(now.In(trTime).Year()),
	})
	if err != nil {
		return InvoiceView{}, err
	}
	ettn := uuid.New()
	inv, _, err := compose(doc, profile, tmp.Number, ettn, nil)
	if err != nil {
		return InvoiceView{}, err
	}
	sn, err := takeSnapshot(doc, inv)
	if err != nil {
		return InvoiceView{}, err
	}
	rate := src.RateSnapshot
	if len(bytes.TrimSpace(rate)) == 0 || string(rate) == "null" {
		rate = []byte("{}")
	}
	row, err := qtx.CreateEinvoice(ctx, db.CreateEinvoiceParams{
		Uuid: ettn, OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, Number: tmp.Number,
		Profile: profile, InvoiceType: "SATIS", SourceType: src.Type, SourceUuid: src.UUID,
		BuyerOrgID: pgtype.Int8{Int64: src.Buyer.ID, Valid: true}, Buyer: sn.Buyer, Seller: sn.Seller,
		Lines: sn.Lines, Currency: strings.ToUpper(doc.Currency), RateSnapshot: rate,
		LineExtension: sn.LineExtension, TaxExclusive: sn.TaxExclusive, TaxTotal: sn.TaxTotal, Payable: sn.Payable,
		TaxBreakdown: sn.TaxBreakdown, ValidationStatus: ubl.StatusValid, ValidationMessages: []byte("[]"),
		Status: StatusDraft, IssueDate: date(now), CreatedBy: c.actorArg(),
	})
	if isUniqueViolation(err) {
		return InvoiceView{}, ErrAlreadyInvoiced
	}
	if err != nil {
		return InvoiceView{}, err
	}
	if err := activity.Write(ctx, qtx, c.actor(), ActionDraftCreated, activityResource, &row.Uuid, map[string]any{
		"source_type": src.Type, "source_uuid": src.UUID.String(), "source_no": src.No,
	}, c.Meta); err != nil {
		return InvoiceView{}, err
	}
	v, err := s.view(ctx, qtx, row)
	if err != nil {
		return InvoiceView{}, err
	}
	return v, tx.Commit(ctx)
}

// Get returns an invoice with the center's ledger row of its source.
func (s *Service) Get(ctx context.Context, c Caller, id uuid.UUID) (InvoiceView, error) {
	if err := c.center(); err != nil {
		return InvoiceView{}, err
	}
	row, err := s.invoice(ctx, s.q, c, id, false)
	if err != nil {
		return InvoiceView{}, err
	}
	v, err := s.view(ctx, s.q, row)
	if err != nil {
		return InvoiceView{}, err
	}
	fe, err := s.q.GetEinvoiceFinanceEntry(ctx, db.GetEinvoiceFinanceEntryParams{
		OrganizationID: row.OrganizationID, SourceUuid: pgtype.UUID{Bytes: row.SourceUuid, Valid: true},
	})
	switch {
	case err == nil:
		v.FinanceEntry = &FinanceEntryRef{
			UUID: fe.Uuid, Direction: fe.Direction, Amount: money(fe.Amount), Currency: fe.Currency,
			SourceType: fe.SourceType.String,
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return InvoiceView{}, err
	}
	return v, nil
}

// List returns the center's invoices (list contract; the caller fills the
// filters, the center scope is forced).
func (s *Service) List(ctx context.Context, c Caller, p db.ListEinvoicesParams) ([]InvoiceView, int64, error) {
	if err := c.center(); err != nil {
		return nil, 0, err
	}
	p.BrandID, p.OrgIds = c.Org.BrandID, []int64{c.Org.InternalID}
	rows, err := s.q.ListEinvoices(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]InvoiceView, 0, len(rows))
	for _, r := range rows {
		var ref *OrgRef
		if r.BuyerOrgUuid.Valid {
			ref = &OrgRef{UUID: r.BuyerOrgUuid.Bytes, Name: r.BuyerOrgName.String}
		}
		out = append(out, viewOf(db.Einvoice{
			ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, BrandID: r.BrandID, Number: r.Number,
			Profile: r.Profile, InvoiceType: r.InvoiceType, SourceType: r.SourceType, SourceUuid: r.SourceUuid,
			BuyerOrgID: r.BuyerOrgID, Buyer: r.Buyer, Seller: r.Seller, Lines: r.Lines, Currency: r.Currency,
			RateSnapshot: r.RateSnapshot, LineExtension: r.LineExtension, TaxExclusive: r.TaxExclusive,
			TaxTotal: r.TaxTotal, Payable: r.Payable, TaxBreakdown: r.TaxBreakdown, XmlStorageKey: r.XmlStorageKey,
			XmlSha256: r.XmlSha256, PdfStorageKey: r.PdfStorageKey, ValidationStatus: r.ValidationStatus,
			ValidationMessages: r.ValidationMessages, Status: r.Status, Error: r.Error, VoidedAt: r.VoidedAt,
			VoidedBy: r.VoidedBy, VoidReason: r.VoidReason, IssueDate: r.IssueDate, CreatedBy: r.CreatedBy,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}, ref))
	}
	total := int64(0)
	if len(rows) > 0 {
		total = rows[0].TotalCount
	} else if p.RowOffset > 0 {
		total, err = s.q.CountEinvoices(ctx, db.CountEinvoicesParams{
			BrandID: p.BrandID, OrgIds: p.OrgIds, Statuses: p.Statuses, Profiles: p.Profiles,
			BuyerOrgIds: p.BuyerOrgIds, IssueDateFrom: p.IssueDateFrom, IssueDateTo: p.IssueDateTo,
			PayableMin: p.PayableMin, PayableMax: p.PayableMax, Q: p.Q,
		})
		if err != nil {
			return nil, 0, err
		}
	}
	return out, total, nil
}

// BillableView is a billable center sale.
type BillableView struct {
	SourceType        string    `json:"source_type"`
	SourceUUID        uuid.UUID `json:"source_uuid"`
	SourceNo          string    `json:"source_no"`
	BuyerOrganization OrgRef    `json:"buyer_organization"`
	Currency          string    `json:"currency"`
	LineExtension     string    `json:"line_extension"`
	TaxTotal          string    `json:"tax_total"`
	Payable           string    `json:"payable"`
	BillableAt        *string   `json:"billable_at"`
	PeriodStart       *string   `json:"period_start"`
	PeriodEnd         *string   `json:"period_end"`
}

// ListBillable returns the center sales without an active invoice (list
// contract; the center scope is forced).
func (s *Service) ListBillable(ctx context.Context, c Caller, p db.ListEinvoiceBillableSourcesParams) ([]BillableView, int64, error) {
	if err := c.center(); err != nil {
		return nil, 0, err
	}
	p.BrandID, p.CenterOrgID = c.Org.BrandID, c.Org.InternalID
	rows, err := s.q.ListEinvoiceBillableSources(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	total := int64(0)
	if len(rows) > 0 {
		total = rows[0].TotalCount
	} else if p.RowOffset > 0 {
		probe := p
		probe.RowOffset, probe.RowLimit = 0, 1
		first, err := s.q.ListEinvoiceBillableSources(ctx, probe)
		if err != nil {
			return nil, 0, err
		}
		if len(first) > 0 {
			total = first[0].TotalCount
		}
	}
	out := make([]BillableView, 0, len(rows))
	for _, r := range rows {
		v := BillableView{
			SourceType: r.SourceType, SourceUUID: r.SourceUuid, SourceNo: r.SourceNo,
			BuyerOrganization: OrgRef{UUID: r.BuyerOrgUuid, Name: r.BuyerName}, Currency: r.Currency,
			LineExtension: money(r.LineExtension), TaxTotal: money(r.TaxTotal), Payable: money(r.Payable),
		}
		if r.BillableAt.Valid {
			t := r.BillableAt.Time.UTC().Format(time.RFC3339)
			v.BillableAt = &t
		}
		if r.PeriodStart.Valid {
			t := r.PeriodStart.Time.Format(time.DateOnly)
			v.PeriodStart = &t
		}
		if r.PeriodEnd.Valid {
			t := r.PeriodEnd.Time.Format(time.DateOnly)
			v.PeriodEnd = &t
		}
		out = append(out, v)
	}
	return out, total, nil
}

// Preview renders a draft (or a failed attempt) to HTML without a number
// (the temporary one is stripped) and with a PREVIEW watermark; nothing is
// stored.
func (s *Service) Preview(ctx context.Context, c Caller, id uuid.UUID) ([]byte, error) {
	if err := c.center(); err != nil {
		return nil, err
	}
	row, err := s.invoice(ctx, s.q, c, id, false)
	if err != nil {
		return nil, err
	}
	if row.Status != StatusDraft && row.Status != StatusFailed {
		return nil, ErrInvalidStatus
	}
	st, err := s.settings(ctx, s.q, c)
	if err != nil {
		return nil, err
	}
	src, err := s.loadSource(ctx, s.q, c, row.SourceType, row.SourceUuid)
	if err != nil {
		return nil, err
	}
	doc, profile, err := s.document(ctx, src, st, s.now())
	if err != nil {
		return nil, err
	}
	xslt := s.stylesheet(ctx, &st)
	_, xml, err := compose(doc, profile, row.Number, row.Uuid, xslt)
	if err != nil {
		return nil, err
	}
	html, err := ubl.RenderHTML(ctx, xml, xslt)
	if err != nil {
		return nil, err
	}
	// The temporary number is not an invoice number: the preview shows none.
	html = bytes.ReplaceAll(html, []byte(row.Number), nil)
	return Watermark(html, "PREVIEW"), nil
}

// Watermark overlays a large diagonal label on every page of the HTML.
func Watermark(html []byte, label string) []byte {
	mark := fmt.Sprintf(`<div data-watermark="%[1]s" style="position:fixed;inset:0;display:flex;align-items:center;justify-content:center;pointer-events:none;z-index:9999"><span style="font:700 120px sans-serif;color:rgba(220,38,38,.18);transform:rotate(-30deg)">%[1]s</span></div>`, label)
	lower := bytes.ToLower(html)
	if i := bytes.Index(lower, []byte("<body")); i >= 0 {
		if j := bytes.IndexByte(html[i:], '>'); j >= 0 {
			at := i + j + 1
			out := make([]byte, 0, len(html)+len(mark))
			out = append(out, html[:at]...)
			out = append(out, mark...)
			return append(out, html[at:]...)
		}
	}
	return append([]byte(mark), html...)
}

// Archive numbers, validates and stores a draft. The XML is validated with
// the temporary number first; an XSD error (invalid) or a Schematron
// violation (rule_warnings) marks the invoice failed with the messages and
// consumes no number. A valid invoice takes the next number of its series
// in the same transaction, is rebuilt with it and stored at
// einvoices/{year}/{number}.xml with its sha256. The PDF follows on the
// docs queue when pdf_enabled.
func (s *Service) Archive(ctx context.Context, c Caller, id uuid.UUID) (InvoiceView, error) {
	if err := c.center(); err != nil {
		return InvoiceView{}, err
	}
	if s.store == nil {
		return InvoiceView{}, ErrStorageUnavailable
	}
	v, pdf, err := s.archive(ctx, c, id)
	if err != nil {
		return InvoiceView{}, err
	}
	if pdf != 0 {
		s.schedulePDF(ctx, pdf)
		if s.queue == nil {
			// Rendered inline: reload the PDF key / error.
			row, err := s.invoice(ctx, s.q, c, id, false)
			if err == nil {
				return s.view(ctx, s.q, row)
			}
		}
	}
	return v, nil
}

func (s *Service) archive(ctx context.Context, c Caller, id uuid.UUID) (InvoiceView, int64, error) {
	tx, err := s.tx.Begin(ctx)
	if err != nil {
		return InvoiceView{}, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)
	row, err := s.invoice(ctx, qtx, c, id, true)
	if err != nil {
		return InvoiceView{}, 0, err
	}
	if row.Status != StatusDraft && row.Status != StatusFailed {
		return InvoiceView{}, 0, ErrInvalidStatus
	}
	st, err := s.settings(ctx, qtx, c)
	if err != nil {
		return InvoiceView{}, 0, err
	}
	src, err := s.loadSource(ctx, qtx, c, row.SourceType, row.SourceUuid)
	if err != nil {
		return InvoiceView{}, 0, err
	}
	issueAt := s.now()
	doc, profile, err := s.document(ctx, src, st, issueAt)
	if err != nil {
		return InvoiceView{}, 0, err
	}
	xslt := s.stylesheet(ctx, &st)

	// 1. Validate with the temporary number: nothing is consumed yet.
	inv, xml, err := compose(doc, profile, row.Number, row.Uuid, xslt)
	if err != nil {
		return InvoiceView{}, 0, err
	}
	val, err := s.validate(ctx, xml, profile)
	if err != nil {
		return InvoiceView{}, 0, err
	}
	if !val.Archivable() {
		failed, err := s.fail(ctx, qtx, c, row, doc, inv, profile, val)
		if err != nil {
			return InvoiceView{}, 0, err
		}
		return failed, 0, tx.Commit(ctx)
	}

	// 2. Take the series number in this transaction and rebuild.
	series := st.EfaturaSeries
	if profile == ubl.ProfileEArchive {
		series = st.EarchiveSeries
	}
	year := issueAt.In(trTime).Year()
	ctr, err := qtx.IncrementEinvoiceCounter(ctx, db.IncrementEinvoiceCounterParams{
		OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, Series: series, Year: int32(year),
	})
	if err != nil {
		return InvoiceView{}, 0, err
	}
	inv, xml, err = compose(doc, profile, ctr.Number, row.Uuid, xslt)
	if err != nil {
		return InvoiceView{}, 0, err
	}
	if val, err = s.validate(ctx, xml, profile); err != nil {
		return InvoiceView{}, 0, err
	}
	if !val.Archivable() {
		// The numbered XML failed: roll the number back, keep the failure.
		_ = tx.Rollback(ctx)
		return s.failAfterRollback(ctx, c, id, doc, inv, profile, val)
	}
	sn, err := takeSnapshot(doc, inv)
	if err != nil {
		return InvoiceView{}, 0, err
	}
	sum := sha256.Sum256(xml)
	digest := hex.EncodeToString(sum[:])
	key := XMLObjectKey(year, ctr.Number)
	if err := s.store.Upload(ctx, storage.File{
		Body: bytes.NewReader(xml), Size: int64(len(xml)), ContentType: "application/xml",
		Filename: ctr.Number + ".xml",
		Metadata: map[string]string{"sha256": digest, "ettn": row.Uuid.String()},
	}, key); err != nil {
		return InvoiceView{}, 0, err
	}
	archived, err := qtx.ArchiveEinvoice(ctx, db.ArchiveEinvoiceParams{
		ID: row.ID, Number: ctr.Number, Profile: profile, Buyer: sn.Buyer, Seller: sn.Seller, Lines: sn.Lines,
		LineExtension: sn.LineExtension, TaxExclusive: sn.TaxExclusive, TaxTotal: sn.TaxTotal, Payable: sn.Payable,
		TaxBreakdown: sn.TaxBreakdown, IssueDate: date(issueAt),
		XmlStorageKey: pgtype.Text{String: key, Valid: true}, XmlSha256: pgtype.Text{String: digest, Valid: true},
	})
	if err != nil {
		return InvoiceView{}, 0, err
	}
	payload := map[string]any{
		"einvoice_uuid": archived.Uuid.String(), "number": archived.Number, "profile": archived.Profile,
		"source_type": archived.SourceType, "source_uuid": archived.SourceUuid.String(),
		"buyer_org_id": archived.BuyerOrgID.Int64, "payable": money(archived.Payable),
		"currency": archived.Currency, "issue_date": archived.IssueDate.Time.Format(time.DateOnly),
		"xml_sha256": digest,
	}
	if err := s.emit(ctx, tx, events.EinvoiceArchived, archived, c.actor(), payload); err != nil {
		return InvoiceView{}, 0, err
	}
	if err := activity.Write(ctx, qtx, c.actor(), ActionArchived, activityResource, &archived.Uuid, payload, c.Meta); err != nil {
		return InvoiceView{}, 0, err
	}
	v, err := s.view(ctx, qtx, archived)
	if err != nil {
		return InvoiceView{}, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return InvoiceView{}, 0, err
	}
	pdf := int64(0)
	if st.PdfEnabled {
		pdf = archived.ID
	}
	return v, pdf, nil
}

// fail records a validation failure on the draft (status failed).
func (s *Service) fail(ctx context.Context, q *db.Queries, c Caller, row db.Einvoice, doc ubl.Document, inv *invoice.Invoice, profile string, val ubl.Validation) (InvoiceView, error) {
	sn := snapshot{
		Buyer: row.Buyer, Seller: row.Seller, Lines: row.Lines, TaxBreakdown: row.TaxBreakdown,
		LineExtension: row.LineExtension, TaxExclusive: row.TaxExclusive, TaxTotal: row.TaxTotal, Payable: row.Payable,
	}
	if inv != nil {
		if fresh, err := takeSnapshot(doc, inv); err == nil {
			sn = fresh
		}
	}
	msgs := val.Messages
	if len(msgs) == 0 {
		msgs = []string{val.Status}
	}
	body, err := json.Marshal(msgs)
	if err != nil {
		return InvoiceView{}, err
	}
	summary := msgs[0]
	if len(summary) > 1000 {
		summary = summary[:1000]
	}
	failed, err := q.FailEinvoice(ctx, db.FailEinvoiceParams{
		ID: row.ID, Profile: profile, Buyer: sn.Buyer, Seller: sn.Seller, Lines: sn.Lines,
		LineExtension: sn.LineExtension, TaxExclusive: sn.TaxExclusive, TaxTotal: sn.TaxTotal, Payable: sn.Payable,
		TaxBreakdown: sn.TaxBreakdown, ValidationStatus: val.Status, ValidationMessages: body,
		Error: pgtype.Text{String: "validation " + val.Status + ": " + summary, Valid: true},
	})
	if err != nil {
		return InvoiceView{}, err
	}
	if err := activity.Write(ctx, q, c.actor(), ActionArchiveFailed, activityResource, &failed.Uuid, map[string]any{
		"validation_status": val.Status, "message_count": len(msgs),
	}, c.Meta); err != nil {
		return InvoiceView{}, err
	}
	return s.view(ctx, q, failed)
}

func (s *Service) failAfterRollback(ctx context.Context, c Caller, id uuid.UUID, doc ubl.Document, inv *invoice.Invoice, profile string, val ubl.Validation) (InvoiceView, int64, error) {
	tx, err := s.tx.Begin(ctx)
	if err != nil {
		return InvoiceView{}, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)
	row, err := s.invoice(ctx, qtx, c, id, true)
	if err != nil {
		return InvoiceView{}, 0, err
	}
	v, err := s.fail(ctx, qtx, c, row, doc, inv, profile, val)
	if err != nil {
		return InvoiceView{}, 0, err
	}
	return v, 0, tx.Commit(ctx)
}

// VoidInput is the void request.
type VoidInput struct {
	Reason string
}

// Void marks an invoice voided (a mark only: nothing is sent, the row and
// its files stay). The source becomes billable again.
func (s *Service) Void(ctx context.Context, c Caller, id uuid.UUID, in VoidInput) (InvoiceView, error) {
	if err := c.center(); err != nil {
		return InvoiceView{}, err
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return InvoiceView{}, invalid("reason", "is required")
	}
	if len([]rune(reason)) > 1000 {
		return InvoiceView{}, invalid("reason", "must be at most 1000 characters")
	}
	tx, err := s.tx.Begin(ctx)
	if err != nil {
		return InvoiceView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)
	row, err := s.invoice(ctx, qtx, c, id, true)
	if err != nil {
		return InvoiceView{}, err
	}
	if row.Status == StatusVoided {
		return InvoiceView{}, ErrInvalidStatus
	}
	voided, err := qtx.VoidEinvoice(ctx, db.VoidEinvoiceParams{
		ID: row.ID, BrandID: row.BrandID, VoidedBy: c.actorArg(), VoidReason: pgtype.Text{String: reason, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return InvoiceView{}, ErrInvalidStatus
	}
	if err != nil {
		return InvoiceView{}, err
	}
	payload := map[string]any{
		"einvoice_uuid": voided.Uuid.String(), "number": voided.Number, "previous_status": row.Status,
		"source_type": voided.SourceType, "source_uuid": voided.SourceUuid.String(), "reason": reason,
	}
	if err := s.emit(ctx, tx, events.EinvoiceVoided, voided, c.actor(), payload); err != nil {
		return InvoiceView{}, err
	}
	if err := activity.Write(ctx, qtx, c.actor(), ActionVoided, activityResource, &voided.Uuid, payload, c.Meta); err != nil {
		return InvoiceView{}, err
	}
	v, err := s.view(ctx, qtx, voided)
	if err != nil {
		return InvoiceView{}, err
	}
	return v, tx.Commit(ctx)
}

// XMLObjectKey is einvoices/{year}/{number}.xml.
func XMLObjectKey(year int, number string) string {
	return fmt.Sprintf("einvoices/%d/%s.xml", year, number)
}

// PDFObjectKey is einvoices/{year}/{number}.pdf.
func PDFObjectKey(year int, number string) string {
	return fmt.Sprintf("einvoices/%d/%s.pdf", year, number)
}
