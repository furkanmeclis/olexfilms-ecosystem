package usecase

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/documentstest"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
)

// Concurrent deliveries of contract.executed (outbox lease expiry, two
// workers) must render and upload the PDF exactly once.
func TestExecutedContractPDFConcurrentDeliveryRendersOnce(t *testing.T) {
	d := newTestDB(t)
	f := d.signingFixture(t, d.caller.OrganizationID)
	gotb := documentstest.NewGotenberg(t, 300*time.Millisecond)
	f.svc.pdf = pdfrender.New(gotb.URL)
	executed := f.execute(t)
	row, err := d.q.GetContractInstanceByUUID(d.ctx, executed.UUID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(d.ctx, 15*time.Second)
	defer cancel()

	const workers = 4
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- f.svc.GenerateExecutedPDF(ctx, row.ID)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("generate pdf: %v", err)
		}
	}
	if calls := gotb.Calls.Load(); calls != 1 {
		t.Fatalf("gotenberg calls = %d, want 1 for %d concurrent deliveries", calls, workers)
	}
	row, err = d.q.GetContractInstanceByUUID(d.ctx, executed.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if !row.PdfKey.Valid || row.PdfKey.String == "" {
		t.Fatal("pdf_key was not set")
	}
}

// The seeded system `contract` template (000030) predates the OTP evidence
// variables; the executed PDF must still carry the signatures, the OTP proof
// block, the content hash and the embedded media.
func TestExecutedContractPDFDefaultTemplateCarriesEvidence(t *testing.T) {
	d := newTestDB(t)
	f := d.signingFixture(t, d.caller.OrganizationID)
	gotb := documentstest.NewGotenberg(t, 0)
	f.svc.pdf = pdfrender.New(gotb.URL)
	png := testPNG()
	if _, err := f.svc.AddMedia(d.ctx, f.caller, f.contract.UUID, MediaInput{
		Body: bytes.NewReader(png), Size: int64(len(png)), Filename: "front.png", Title: "Front bumper", ContentType: "image/png",
	}); err != nil {
		t.Fatalf("add media: %v", err)
	}
	executed := f.execute(t)
	row, err := d.q.GetContractInstanceByUUID(d.ctx, executed.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.GenerateExecutedPDF(d.ctx, row.ID); err != nil {
		t.Fatalf("generate pdf: %v", err)
	}
	html := gotb.LastHTML()
	for _, want := range []string{"KVKK", row.ContentSha256.String, "Front bumper"} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered HTML missing %q:\n%s", want, html)
		}
	}
	// Two signer signatures + one media photo.
	if count := strings.Count(html, "data:image/png;base64"); count < 3 {
		t.Fatalf("embedded images = %d, want >= 3 (2 signatures + media)", count)
	}
}

// TEC-526: the executed PDF's chrome (title, OTP proof table, signer roles,
// media heading, hash caption) is printed in the contract's locale; a tr
// contract carries no English fixed text.
func TestExecutedContractPDFChromeFollowsContractLocale(t *testing.T) {
	d := newTestDB(t)
	f := d.signingFixture(t, d.caller.OrganizationID)
	d.publishContractDocumentTemplate(t)
	gotb := documentstest.NewGotenberg(t, 0)
	f.svc.pdf = pdfrender.New(gotb.URL)
	png := testPNG()
	if _, err := f.svc.AddMedia(d.ctx, f.caller, f.contract.UUID, MediaInput{
		Body: bytes.NewReader(png), Size: int64(len(png)), Filename: "front.png", Title: "Ön tampon", ContentType: "image/png",
	}); err != nil {
		t.Fatalf("add media: %v", err)
	}
	executed := f.execute(t)
	row, err := d.q.GetContractInstanceByUUID(d.ctx, executed.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.GenerateExecutedPDF(d.ctx, row.ID); err != nil {
		t.Fatalf("generate pdf: %v", err)
	}
	html := gotb.LastHTML()
	for _, want := range []string{
		`<html lang="tr" dir="ltr">`, "<title>Sözleşme " + strconv.FormatInt(row.ContractNo, 10) + "</title>",
		"İmzacı", "Maskeli telefon", "Doğrulama zamanı", "KVKK bildirim sürümü", "İçerik SHA-256",
		"Müşteri", "Personel", "Görseller ve ekler",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("tr rendered HTML missing %q:\n%s", want, html)
		}
	}
	for _, stale := range []string{
		"Contract ", "Signer", "Masked phone", "Verified at", "KVKK notice version", "Content SHA-256",
		">customer<", ">staff<", "<h2>Media</h2>", "doc-hash\">SHA-256:",
	} {
		if strings.Contains(html, stale) {
			t.Fatalf("tr rendered HTML still carries %q:\n%s", stale, html)
		}
	}
	// The meta-table captions the system template prints through
	// {{label_*}} (migration 000133) come from the tr catalog.
	assertContractLabelVars(t, d, f, row, map[string]string{
		"label_contract_no": "Sözleşme no", "label_date": "Tarih", "label_party": "Taraf", "label_signed_at": "İmza zamanı",
		"contract_title": "Sözleşme " + strconv.FormatInt(row.ContractNo, 10),
	})
}

func assertContractLabelVars(t *testing.T, d *testDB, f signingFixture, row db.ContractInstance, want map[string]string) {
	t.Helper()
	org, err := d.q.GetOrganizationByID(d.ctx, row.OrganizationID)
	if err != nil {
		t.Fatal(err)
	}
	vars, err := f.svc.contractDocumentVars(d.ctx, d.q, row, org)
	if err != nil {
		t.Fatalf("contract document vars: %v", err)
	}
	for k, v := range want {
		if got := vars[k]; got != v {
			t.Fatalf("vars[%s] = %q, want %q", k, got, v)
		}
	}
}

// TEC-526: an ar contract renders right-to-left with Arabic chrome even when
// the document template itself falls back to tr/en.
func TestExecutedContractPDFArabicIsRTL(t *testing.T) {
	d := newTestDB(t)
	f := d.signingFixture(t, d.caller.OrganizationID)
	d.publishContractDocumentTemplate(t)
	gotb := documentstest.NewGotenberg(t, 0)
	f.svc.pdf = pdfrender.New(gotb.URL)
	if _, err := f.svc.PutLocale(d.ctx, d.caller, f.template.UUID, LocaleInput{
		Locale: "ar", HTML: `<p>{{customer_name}} {{service_no}}</p>`,
	}); err != nil {
		t.Fatalf("put ar locale: %v", err)
	}
	svcRow := d.createService(t, d.caller.OrganizationID)
	contract, err := f.svc.CreateForService(d.ctx, f.caller, svcRow.Uuid, CreateFromServiceInput{Locale: "ar"})
	if err != nil {
		t.Fatalf("create ar contract: %v", err)
	}
	if contract.Locale != "ar" {
		t.Fatalf("contract locale = %q, want ar", contract.Locale)
	}
	if _, err := f.svc.SignCustomer(d.ctx, f.caller, contract.UUID, SignatureInput{
		Code: "123456", PNGBase64: pngBase64(), IP: "203.0.113.10", UserAgent: "go-test",
	}); err != nil {
		t.Fatalf("customer sign: %v", err)
	}
	if _, err := f.svc.SignStaff(d.ctx, f.caller, contract.UUID, SignatureInput{PNGBase64: pngBase64()}); err != nil {
		t.Fatalf("staff sign: %v", err)
	}
	row, err := d.q.GetContractInstanceByUUID(d.ctx, contract.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.GenerateExecutedPDF(d.ctx, row.ID); err != nil {
		t.Fatalf("generate pdf: %v", err)
	}
	html := gotb.LastHTML()
	for _, want := range []string{
		`<html lang="ar" dir="rtl">`, "الموقّع", "العميل", "الموظف", "العقد " + strconv.FormatInt(row.ContractNo, 10),
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("ar rendered HTML missing %q:\n%s", want, html)
		}
	}
	for _, stale := range []string{"Signer", "Masked phone", "Contract ", "<th>Party</th>", "<th>Date</th>", ">customer<", ">staff<"} {
		if strings.Contains(html, stale) {
			t.Fatalf("ar rendered HTML still carries %q:\n%s", stale, html)
		}
	}
	assertContractLabelVars(t, d, f, row, map[string]string{
		"label_contract_no": "رقم العقد", "label_date": "التاريخ", "label_party": "الطرف", "label_signed_at": "تاريخ التوقيع",
	})
}

func TestMaskPhoneHidesSubscriberDigits(t *testing.T) {
	got := maskPhone("+905551234567")
	if strings.Contains(got, "555123") || strings.Contains(got, "1234") {
		t.Fatalf("maskPhone leaked digits: %q", got)
	}
	if !strings.HasSuffix(got, "67") || len(got) != len("+905551234567") {
		t.Fatalf("maskPhone = %q, want same length ending in 67", got)
	}
}

// The panel PDF download follows the same scope rules as GET /v1/contracts:
// a brand-wide caller (no org set) reads it, an own-scope user who did not
// create the service does not.
func TestDownloadPDFFollowsContractScope(t *testing.T) {
	d := newTestDB(t)
	f := d.signingFixture(t, d.caller.OrganizationID)
	gotb := documentstest.NewGotenberg(t, 0)
	f.svc.pdf = pdfrender.New(gotb.URL)
	executed := f.execute(t)
	row, err := d.q.GetContractInstanceByUUID(d.ctx, executed.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.GenerateExecutedPDF(d.ctx, row.ID); err != nil {
		t.Fatalf("generate pdf: %v", err)
	}

	brand := f.caller
	brand.Filter = scopefilter.Filter{Scope: rbac.ScopeBrand, BrandID: f.caller.BrandID, UserID: f.caller.UserID}
	rc, _, err := f.svc.DownloadPDF(d.ctx, brand, executed.UUID)
	if err != nil {
		t.Fatalf("brand-scope download: %v", err)
	}
	_ = rc.Close()

	other := d.createUser(t, "staff")
	own := f.caller
	own.UserID = other.ID
	own.Filter = scopefilter.Filter{Scope: rbac.ScopeOwn, OrgIDs: []int64{f.caller.OrganizationID}, OrgID: f.caller.OrganizationID, UserID: other.ID}
	if rc, _, err := f.svc.DownloadPDF(d.ctx, own, executed.UUID); !errors.Is(err, ErrNotFound) {
		if rc != nil {
			_ = rc.Close()
		}
		t.Fatalf("own-scope foreign download err = %v, want ErrNotFound", err)
	}
}
