package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/ubl"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

type fakeOutbox struct{ names []string }

func (f *fakeOutbox) Enqueue(_ context.Context, _ pgx.Tx, ev events.Event) error {
	f.names = append(f.names, ev.Name)
	return nil
}

type fakePDF struct {
	err   error
	calls int
}

func (f *fakePDF) Convert(_ context.Context, req pdfrender.Request) ([]byte, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if !strings.Contains(req.HTML, "<") {
		return nil, errors.New("no html")
	}
	return []byte("%PDF-1.7 fake"), nil
}

type fixture struct {
	ctx     context.Context
	tx      pgx.Tx
	q       *db.Queries
	svc     *Service
	store   *storage.Memory
	out     *fakeOutbox
	pdf     *fakePDF
	brandID int64
	center  db.Organization
	dist    db.Organization
	product db.Product
	user    db.User
	c       Caller
	year    int
	seq     int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	q := db.New(tx)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	dist, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: "t503-dist-" + suffix, Name: "T503 Ege Dist", Status: "active", Type: "distributor",
		Address: "Kordon No:5", City: "İzmir", District: "Konak",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		ParentID:       pgtype.Int8{Int64: center.ID, Valid: true}, BrandID: brand.ID,
		Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("distributor: %v", err)
	}
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "T503", Surname: "Accounting", Status: "active",
		Email: pgtype.Text{String: "t503-" + suffix + "@example.test", Valid: true},
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	category, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: center.ID, BrandID: brand.ID, Name: "t503-cat-" + suffix,
		AvailableParts: []byte(`[]`), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	product, err := q.CreateProduct(ctx, db.CreateProductParams{
		OrganizationID: center.ID, BrandID: brand.ID, CategoryID: category.ID,
		Sku: "t503-" + suffix, Name: "T503 Cam filmi", Images: []byte("[]"), UnitType: "piece", Active: true,
	})
	if err != nil {
		t.Fatalf("product: %v", err)
	}
	f := &fixture{
		ctx: ctx, tx: tx, q: q, store: storage.NewMemory(), out: &fakeOutbox{}, pdf: &fakePDF{},
		brandID: brand.ID, center: center, dist: dist, product: product, user: user,
		year: time.Now().In(trTime).Year(),
	}
	f.svc = New(tx, q, f.store, f.out, nil).WithPDF(f.pdf, nil)
	f.c = Caller{
		Principal: authctx.Principal{UserID: user.Uuid, UserInternal: user.ID},
		Org:       orgctx.Scope{InternalID: center.ID, UUID: center.Uuid, OrgType: "center", BrandID: brand.ID},
	}
	if _, err := f.svc.PutSettings(ctx, f.c, SettingsInput{
		VKN: "9000068418", TaxOffice: "Beşiktaş", LegalName: "Olexfilms Merkez A.Ş.",
		Address: "Papatya Cad. No:21", City: "İstanbul", District: "Beşiktaş",
		IBAN: "TR330006100519786457841326", EArchiveSeries: "EAR", EFaturaSeries: "EFN", PDFEnabled: true,
	}); err != nil {
		t.Fatalf("settings: %v", err)
	}
	return f
}

// withBuyerProfile gives the distributor a valid VKN profile.
func (f *fixture) withBuyerProfile(t *testing.T) {
	t.Helper()
	if _, err := f.svc.UpdateBuyerProfile(f.ctx, f.c, f.dist.Uuid, BuyerProfileInput{
		VKN: "1234567890", TaxOffice: "Konak", LegalName: "Ege Distribütör Ltd. Şti.",
	}); err != nil {
		t.Fatalf("buyer profile: %v", err)
	}
}

// order inserts a received center → distributor order of qty × price.
func (f *fixture) order(t *testing.T, qty int, price string) uuid.UUID {
	t.Helper()
	f.seq++
	var id int64
	var uid uuid.UUID
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO orders (
		order_no, organization_id, brand_id, seller_org_id, buyer_org_id, status, currency, subtotal, total, received_at
	) VALUES ($1, $2, $3, $2, $4, 'received', 'TRY', $5::numeric * $6::int, $5::numeric * $6::int, NOW()) RETURNING id, uuid`,
		fmt.Sprintf("T503-%d-%d", time.Now().UnixNano(), f.seq), f.center.ID, f.brandID, f.dist.ID, price, qty,
	).Scan(&id, &uid); err != nil {
		t.Fatalf("order: %v", err)
	}
	if _, err := f.tx.Exec(f.ctx, `INSERT INTO order_items (
		order_id, organization_id, brand_id, product_id, quantity, unit_price, price_source, line_total
	) VALUES ($1, $2, $3, $4, $5::int, $6::numeric, 'list', $6::numeric * $5::int)`,
		id, f.center.ID, f.brandID, f.product.ID, qty, price); err != nil {
		t.Fatalf("order item: %v", err)
	}
	return uid
}

func (f *fixture) draft(t *testing.T, src uuid.UUID) InvoiceView {
	t.Helper()
	v, err := f.svc.CreateDraft(f.ctx, f.c, DraftInput{SourceType: SourceOrder, SourceUUID: src})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	return v
}

func (f *fixture) counter(t *testing.T, series string) int64 {
	t.Helper()
	ctr, err := f.q.GetEinvoiceCounter(f.ctx, db.GetEinvoiceCounterParams{
		OrganizationID: f.center.ID, Series: series, Year: int32(f.year),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0
	}
	if err != nil {
		t.Fatalf("counter: %v", err)
	}
	return ctr.LastNo
}

func (f *fixture) download(t *testing.T, key string) []byte {
	t.Helper()
	rc, _, err := f.store.Download(f.ctx, key)
	if err != nil {
		t.Fatalf("download %s: %v", key, err)
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (f *fixture) row(t *testing.T, id uuid.UUID) db.Einvoice {
	t.Helper()
	r, err := f.q.GetEinvoiceByUUID(f.ctx, db.GetEinvoiceByUUIDParams{Uuid: id, BrandID: f.brandID})
	if err != nil {
		t.Fatalf("row: %v", err)
	}
	return r
}

// Acceptance: a buyer without a VKN/TCKN cannot get a draft (422 with the
// missing fields); nothing is written.
func TestCreateDraftBuyerWithoutTaxIDIs422(t *testing.T) {
	f := newFixture(t)
	src := f.order(t, 2, "100.00")
	_, err := f.svc.CreateDraft(f.ctx, f.c, DraftInput{SourceType: SourceOrder, SourceUUID: src})
	var pe *ProfileError
	if !errors.As(err, &pe) || !errors.Is(err, ErrBuyerProfileIncomplete) {
		t.Fatalf("draft without VKN = %v, want ProfileError", err)
	}
	if len(pe.Fields) != 1 || pe.Fields[0] != "invoice_vkn" || pe.OrganizationUUID != f.dist.Uuid {
		t.Fatalf("missing = %+v", pe)
	}
	if _, err := f.q.GetActiveEinvoiceBySource(f.ctx, db.GetActiveEinvoiceBySourceParams{
		OrganizationID: f.center.ID, SourceType: SourceOrder, SourceUuid: src,
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a draft was written: %v", err)
	}
	// A VKN without its tax office is still incomplete.
	if _, err := f.svc.UpdateBuyerProfile(f.ctx, f.c, f.dist.Uuid, BuyerProfileInput{VKN: "1234567890"}); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.CreateDraft(f.ctx, f.c, DraftInput{SourceType: SourceOrder, SourceUUID: src})
	if !errors.As(err, &pe) || len(pe.Fields) != 1 || pe.Fields[0] != "invoice_tax_office" {
		t.Fatalf("draft without tax office = %v", err)
	}
	f.withBuyerProfile(t)
	v := f.draft(t, src)
	if v.Status != StatusDraft || v.Number != nil || v.Payable != "240.00" || v.TaxTotal != "40.00" {
		t.Fatalf("draft = %+v", v)
	}
}

// Acceptance: an XML that fails validation marks the invoice failed with the
// messages and consumes no series number; the next valid archive takes the
// next number.
func TestArchiveValidationFailureConsumesNoNumber(t *testing.T) {
	f := newFixture(t)
	f.withBuyerProfile(t)
	d := f.draft(t, f.order(t, 1, "500.00"))
	before := f.counter(t, "EAR")
	for _, status := range []string{ubl.StatusInvalid, ubl.StatusRuleWarnings} {
		f.svc.SetValidator(func(context.Context, []byte, string) (ubl.Validation, error) {
			return ubl.Validation{Status: status, Messages: []string{"rule BR-TR-01 violated"}}, nil
		})
		v, err := f.svc.Archive(f.ctx, f.c, d.UUID)
		if err != nil {
			t.Fatalf("archive (%s): %v", status, err)
		}
		if v.Status != StatusFailed || v.ValidationStatus != status || v.Number != nil ||
			len(v.ValidationMessages) != 1 || v.Error == nil || v.HasXML {
			t.Fatalf("failed archive (%s) = %+v", status, v)
		}
		if got := f.counter(t, "EAR"); got != before {
			t.Fatalf("counter after failed archive = %d, want %d", got, before)
		}
	}
	f.svc.SetValidator(ubl.Validate)
	v, err := f.svc.Archive(f.ctx, f.c, d.UUID)
	if err != nil {
		t.Fatalf("archive after fix: %v", err)
	}
	want := fmt.Sprintf("EAR%d%09d", f.year, before+1)
	if v.Status != StatusArchived || v.Number == nil || *v.Number != want || v.Error != nil {
		t.Fatalf("archive = %+v, want number %s", v, want)
	}
}

// Acceptance: a successful archive stores the XML in S3 with a matching
// sha256 and numbers invoices sequentially; the event and the ledger
// reference are there and no ledger row is written.
func TestArchiveStoresXMLAndNumbersSequentially(t *testing.T) {
	f := newFixture(t)
	f.withBuyerProfile(t)
	a := f.draft(t, f.order(t, 2, "100.00"))
	b := f.draft(t, f.order(t, 3, "50.00"))
	before := f.counter(t, "EAR")
	var entriesBefore int
	if err := f.tx.QueryRow(f.ctx, `SELECT COUNT(*) FROM finance_entries WHERE organization_id = $1`, f.center.ID).Scan(&entriesBefore); err != nil {
		t.Fatal(err)
	}
	var numbers []string
	for i, d := range []InvoiceView{a, b} {
		v, err := f.svc.Archive(f.ctx, f.c, d.UUID)
		if err != nil {
			t.Fatalf("archive %d: %v", i, err)
		}
		if v.Status != StatusArchived || v.Number == nil || !v.HasXML || v.XMLSHA256 == nil || v.Profile != ubl.ProfileEArchive {
			t.Fatalf("archive %d = %+v", i, v)
		}
		numbers = append(numbers, *v.Number)
		row := f.row(t, d.UUID)
		wantKey := fmt.Sprintf("einvoices/%d/%s.xml", f.year, *v.Number)
		if row.XmlStorageKey.String != wantKey {
			t.Fatalf("xml key = %q, want %q", row.XmlStorageKey.String, wantKey)
		}
		xml := f.download(t, wantKey)
		sum := sha256.Sum256(xml)
		if hex.EncodeToString(sum[:]) != *v.XMLSHA256 || row.XmlSha256.String != *v.XMLSHA256 {
			t.Fatalf("sha256 mismatch: %s vs %s", hex.EncodeToString(sum[:]), *v.XMLSHA256)
		}
		if !bytes.Contains(xml, []byte(*v.Number)) || !bytes.Contains(xml, []byte(d.UUID.String())) {
			t.Fatal("xml lacks the number or the ETTN")
		}
		if val, err := ubl.Validate(f.ctx, xml, v.Profile); err != nil || !val.Archivable() {
			t.Fatalf("stored xml validation = %+v %v", val, err)
		}
	}
	for i, n := range numbers {
		if want := fmt.Sprintf("EAR%d%09d", f.year, before+int64(i)+1); n != want {
			t.Fatalf("number %d = %s, want %s", i, n, want)
		}
	}
	archived := 0
	for _, n := range f.out.names {
		if n == events.EinvoiceArchived {
			archived++
		}
	}
	if archived != 2 {
		t.Fatalf("events = %v", f.out.names)
	}
	var entriesAfter int
	if err := f.tx.QueryRow(f.ctx, `SELECT COUNT(*) FROM finance_entries WHERE organization_id = $1`, f.center.ID).Scan(&entriesAfter); err != nil {
		t.Fatal(err)
	}
	if entriesAfter != entriesBefore {
		t.Fatalf("finance entries %d → %d: the invoice must not write the ledger", entriesBefore, entriesAfter)
	}
	// Archived and with a PDF (pdf_enabled, inline render).
	pdf, err := f.svc.PDF(f.ctx, f.c, a.UUID)
	if err != nil {
		t.Fatalf("pdf: %v", err)
	}
	_ = pdf.Body.Close()
	if pdf.Filename != numbers[0]+".pdf" {
		t.Fatalf("pdf name = %s", pdf.Filename)
	}
	html, err := f.svc.HTML(f.ctx, f.c, a.UUID)
	if err != nil || !bytes.Contains(html, []byte(numbers[0])) {
		t.Fatalf("html: %v", err)
	}
	// An archived invoice cannot be archived twice.
	if _, err := f.svc.Archive(f.ctx, f.c, a.UUID); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("second archive = %v", err)
	}
}

// Acceptance: a second invoice for the same source is 409 (draft and
// archived alike).
func TestSecondInvoiceForSameSourceIs409(t *testing.T) {
	f := newFixture(t)
	f.withBuyerProfile(t)
	src := f.order(t, 1, "10.00")
	d := f.draft(t, src)
	if _, err := f.svc.CreateDraft(f.ctx, f.c, DraftInput{SourceType: SourceOrder, SourceUUID: src}); !errors.Is(err, ErrAlreadyInvoiced) {
		t.Fatalf("second draft = %v", err)
	}
	if _, err := f.svc.Archive(f.ctx, f.c, d.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateDraft(f.ctx, f.c, DraftInput{SourceType: SourceOrder, SourceUUID: src}); !errors.Is(err, ErrAlreadyInvoiced) {
		t.Fatalf("draft of an archived source = %v", err)
	}
}

// Acceptance: voiding keeps the old record (number, XML) and makes the
// source billable again.
func TestVoidMakesSourceBillableAgain(t *testing.T) {
	f := newFixture(t)
	f.withBuyerProfile(t)
	src := f.order(t, 4, "25.00")
	billable := func() bool {
		items, _, err := f.svc.ListBillable(f.ctx, f.c, db.ListEinvoiceBillableSourcesParams{
			SourceTypes: []string{SourceOrder}, SortKey: "billable_at", SortDesc: true, RowLimit: 100,
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range items {
			if it.SourceUUID == src {
				return true
			}
		}
		return false
	}
	if !billable() {
		t.Fatal("received order is not billable")
	}
	d := f.draft(t, src)
	if billable() {
		t.Fatal("drafted order is still billable")
	}
	archived, err := f.svc.Archive(f.ctx, f.c, d.UUID)
	if err != nil {
		t.Fatal(err)
	}
	var ve *ValidationError
	if _, err := f.svc.Void(f.ctx, f.c, d.UUID, VoidInput{Reason: "  "}); !errors.As(err, &ve) {
		t.Fatalf("void without reason = %v", err)
	}
	v, err := f.svc.Void(f.ctx, f.c, d.UUID, VoidInput{Reason: "Yanlış alıcı"})
	if err != nil {
		t.Fatalf("void: %v", err)
	}
	if v.Status != StatusVoided || v.VoidReason == nil || v.VoidedAt == nil || *v.Number != *archived.Number || !v.HasXML {
		t.Fatalf("voided = %+v", v)
	}
	if !billable() {
		t.Fatal("voided source is not billable again")
	}
	if _, err := f.svc.Void(f.ctx, f.c, d.UUID, VoidInput{Reason: "again"}); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("second void = %v", err)
	}
	if _, err := f.svc.Archive(f.ctx, f.c, d.UUID); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("archive voided = %v", err)
	}
	again := f.draft(t, src)
	if again.UUID == d.UUID {
		t.Fatal("re-invoice reused the voided row")
	}
	old := f.row(t, d.UUID)
	if old.Status != StatusVoided || !old.XmlStorageKey.Valid {
		t.Fatalf("old row = %+v", old)
	}
	if _, err := f.svc.XML(f.ctx, f.c, d.UUID); err != nil {
		t.Fatalf("voided xml download: %v", err)
	}
	if !contains(f.out.names, events.EinvoiceVoided) {
		t.Fatalf("events = %v", f.out.names)
	}
}

// Acceptance: a PDF failure leaves the invoice archived and lands in error;
// the retry clears it.
func TestPDFFailureKeepsInvoiceArchived(t *testing.T) {
	f := newFixture(t)
	f.withBuyerProfile(t)
	d := f.draft(t, f.order(t, 1, "99.90"))
	f.pdf.err = errors.New("gotenberg: 503")
	v, err := f.svc.Archive(f.ctx, f.c, d.UUID)
	if err != nil {
		t.Fatalf("archive with failing pdf: %v", err)
	}
	if v.Status != StatusArchived || v.HasPDF || v.Error == nil || !strings.Contains(*v.Error, "gotenberg") || v.Number == nil {
		t.Fatalf("archive = %+v", v)
	}
	if _, err := f.svc.PDF(f.ctx, f.c, d.UUID); !errors.Is(err, ErrPDFNotReady) {
		t.Fatalf("pdf download = %v", err)
	}
	f.pdf.err = nil
	v, err = f.svc.RetryPDF(f.ctx, f.c, d.UUID)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if v.Status != StatusArchived || !v.HasPDF || v.Error != nil {
		t.Fatalf("retry = %+v", v)
	}
	row := f.row(t, d.UUID)
	if want := fmt.Sprintf("einvoices/%d/%s.pdf", f.year, *v.Number); row.PdfStorageKey.String != want {
		t.Fatalf("pdf key = %s, want %s", row.PdfStorageKey.String, want)
	}
}

// Acceptance (use case level): only the brand center invoices.
func TestNonCenterCallerForbidden(t *testing.T) {
	f := newFixture(t)
	dist := f.c
	dist.Org = orgctx.Scope{InternalID: f.dist.ID, UUID: f.dist.Uuid, OrgType: "distributor", BrandID: f.brandID}
	if _, err := f.svc.CreateDraft(f.ctx, dist, DraftInput{SourceType: SourceOrder, SourceUUID: uuid.New()}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("distributor draft = %v", err)
	}
	if _, _, err := f.svc.List(f.ctx, dist, db.ListEinvoicesParams{RowLimit: 10}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("distributor list = %v", err)
	}
	if _, err := f.svc.GetSettings(f.ctx, dist); !errors.Is(err, ErrForbidden) {
		t.Fatalf("distributor settings = %v", err)
	}
}

func TestPreviewIsUnnumberedWithWatermark(t *testing.T) {
	f := newFixture(t)
	f.withBuyerProfile(t)
	d := f.draft(t, f.order(t, 1, "100.00"))
	row := f.row(t, d.UUID)
	html, err := f.svc.Preview(f.ctx, f.c, d.UUID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !bytes.Contains(html, []byte(`data-watermark="PREVIEW"`)) || bytes.Contains(html, []byte(row.Number)) {
		t.Fatal("preview lacks the watermark or shows the temporary number")
	}
	if !bytes.Contains(html, []byte("Ege Distrib")) {
		t.Fatal("preview lacks the buyer")
	}
	if _, err := f.svc.Archive(f.ctx, f.c, d.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Preview(f.ctx, f.c, d.UUID); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("preview archived = %v", err)
	}
}

func TestSettingsAndStylesheet(t *testing.T) {
	f := newFixture(t)
	var ve *ValidationError
	_, err := f.svc.PutSettings(f.ctx, f.c, SettingsInput{
		VKN: "9000068419", TaxOffice: "x", LegalName: "x", Address: "x", City: "x", District: "x",
		EArchiveSeries: "TMP", EFaturaSeries: "EFN",
	})
	if !errors.As(err, &ve) || !hasIssue(ve, "vkn") || !hasIssue(ve, "earchive_series") {
		t.Fatalf("invalid settings = %v", err)
	}
	if _, err := f.svc.UploadXSLT(f.ctx, f.c, []byte(`<html xmlns="http://www.w3.org/1999/xhtml"/>`)); !errors.Is(err, ubl.ErrInvalidStylesheet) {
		t.Fatalf("non-stylesheet root = %v", err)
	}
	broken := []byte(`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform"><xsl:template match="/"><xsl:value-of select="unknown-fn(1)"/></xsl:template></xsl:stylesheet>`)
	if _, err := f.svc.UploadXSLT(f.ctx, f.c, broken); !errors.Is(err, ubl.ErrInvalidStylesheet) {
		t.Fatalf("broken stylesheet = %v", err)
	}
	if _, err := f.svc.UploadXSLT(f.ctx, f.c, bytes.Repeat([]byte("a"), MaxXSLTBytes+1)); !errors.As(err, &ve) {
		t.Fatalf("oversized stylesheet = %v", err)
	}
	custom := []byte(`<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"><xsl:template match="/"><html><body><h1>OLEX <xsl:value-of select="//cbc:ID[1]"/></h1></body></html></xsl:template></xsl:stylesheet>`)
	st, err := f.svc.UploadXSLT(f.ctx, f.c, custom)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if !st.CustomXSLT || st.XSLTSHA1 == nil {
		t.Fatalf("settings after upload = %+v", st)
	}
	// Archived HTML uses the custom stylesheet.
	f.withBuyerProfile(t)
	d := f.draft(t, f.order(t, 1, "10.00"))
	v, err := f.svc.Archive(f.ctx, f.c, d.UUID)
	if err != nil {
		t.Fatal(err)
	}
	html, err := f.svc.HTML(f.ctx, f.c, d.UUID)
	if err != nil || !bytes.Contains(html, []byte("OLEX "+*v.Number)) {
		t.Fatalf("custom html = %s %v", html, err)
	}
	st, err = f.svc.ResetXSLT(f.ctx, f.c)
	if err != nil || st.CustomXSLT {
		t.Fatalf("reset = %+v %v", st, err)
	}
	// Settings updates keep the stylesheet columns.
	if _, err := f.svc.UploadXSLT(f.ctx, f.c, custom); err != nil {
		t.Fatal(err)
	}
	st, err = f.svc.PutSettings(f.ctx, f.c, SettingsInput{
		VKN: "9000068418", TaxOffice: "Beşiktaş", LegalName: "Olexfilms Merkez A.Ş.", Address: "Papatya Cad. No:21",
		City: "İstanbul", District: "Beşiktaş", EArchiveSeries: "OEA", EFaturaSeries: "OEF",
	})
	if err != nil || !st.CustomXSLT || st.EArchiveSeries != "OEA" || st.PDFEnabled {
		t.Fatalf("settings update = %+v %v", st, err)
	}
}

func TestBuyerProfileValidation(t *testing.T) {
	f := newFixture(t)
	var ve *ValidationError
	if _, err := f.svc.UpdateBuyerProfile(f.ctx, f.c, f.dist.Uuid, BuyerProfileInput{VKN: "1234567891"}); !errors.As(err, &ve) {
		t.Fatalf("bad VKN = %v", err)
	}
	if _, err := f.svc.UpdateBuyerProfile(f.ctx, f.c, f.dist.Uuid, BuyerProfileInput{VKN: "1234567890", TCKN: "10000000146"}); !errors.As(err, &ve) {
		t.Fatalf("VKN and TCKN = %v", err)
	}
	if _, err := f.svc.UpdateBuyerProfile(f.ctx, f.c, f.center.Uuid, BuyerProfileInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("center profile = %v", err)
	}
	v, err := f.svc.UpdateBuyerProfile(f.ctx, f.c, f.dist.Uuid, BuyerProfileInput{
		VKN: "1234567890", TaxOffice: "Konak", EInvoiceRegistered: true, EInvoiceAlias: "urn:mail:defaultpk@ege.example",
	})
	if err != nil || len(v.Missing) != 0 || !v.EInvoiceRegistered {
		t.Fatalf("profile = %+v %v", v, err)
	}
	// A registered buyer gets TICARIFATURA from the e-Fatura series.
	d := f.draft(t, f.order(t, 1, "10.00"))
	if d.Profile != ubl.ProfileCommercial {
		t.Fatalf("profile = %s", d.Profile)
	}
	a, err := f.svc.Archive(f.ctx, f.c, d.UUID)
	if err != nil || a.Status != StatusArchived || !strings.HasPrefix(*a.Number, "EFN") {
		t.Fatalf("e-fatura archive = %+v %v", a, err)
	}
}

func hasIssue(ve *ValidationError, field string) bool {
	for _, i := range ve.Issues {
		if i.Field == field {
			return true
		}
	}
	return false
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
