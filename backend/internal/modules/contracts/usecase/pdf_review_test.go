package usecase

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

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
