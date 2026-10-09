package usecase

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/documentstest"
	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	docusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/usecase"
	psmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/photostandard/model"
	psusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/photostandard/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
)

// TEC-499 (F5-07b): photo standard rule on contracts and the intake photo
// grid of the executed PDF.

type psFeatures map[string]bool

func (f psFeatures) Enabled(_ context.Context, _ int64, key string) (bool, error) { return f[key], nil }

type intakeFixture struct {
	ps       *psusecase.Service
	feats    psFeatures
	angleKey string
	caller   psusecase.Caller
}

// newIntakeFixture creates a required angle for the brand and a photo
// standard service whose module resolver the test controls.
func (d *testDB) newIntakeFixture(t *testing.T, store storage.Driver, orgID int64) *intakeFixture {
	t.Helper()
	feats := psFeatures{features.ModulePhotoStandard: true}
	ps := psusecase.New(d.pool, d.q).WithFeatures(feats).WithStorage(store)
	key := "t499c-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	center := psusecase.Caller{Org: orgctx.Scope{InternalID: d.caller.OrganizationID, BrandID: d.caller.BrandID, OrgType: rbac.OrgTypeCenter}}
	if _, err := ps.CreateAngle(d.ctx, center, psusecase.AngleInput{
		Key: key, Name: []byte(`{"tr":"Sol yan TEC499","en":"Left side TEC499"}`), Required: true, SortOrder: -999, Active: true,
	}); err != nil {
		t.Fatalf("create angle: %v", err)
	}
	t.Cleanup(func() {
		c := context.Background()
		_, _ = d.pool.Exec(c, `DELETE FROM intake_photos WHERE angle_id IN (SELECT id FROM photo_angles WHERE key = $1)`, key)
		_, _ = d.pool.Exec(c, `DELETE FROM photo_angles WHERE key = $1`, key)
	})
	return &intakeFixture{ps: ps, feats: feats, angleKey: key, caller: psusecase.Caller{
		Principal: authctx.Principal{IsSuperAdmin: true},
		Org:       orgctx.Scope{InternalID: orgID, BrandID: d.caller.BrandID, OrgType: rbac.OrgTypeCenter},
		Filter:    scopefilter.Filter{Scope: rbac.ScopeAll},
	}}
}

// photographAll uploads a photo for every missing required angle of the
// service (other tests may leave required angles in the shared brand).
func (f *intakeFixture) photographAll(t *testing.T, ctx context.Context, store storage.Driver, svc db.Service, lat, lng string) {
	t.Helper()
	err := f.ps.RequireComplete(ctx, nil, psmodel.ServiceRef{ID: svc.ID, OrganizationID: svc.OrganizationID, BrandID: svc.BrandID})
	var inc *psmodel.IncompleteError
	if !errors.As(err, &inc) {
		t.Fatalf("expected missing angles, err = %v", err)
	}
	img := intakeJPEG(t)
	for _, key := range inc.Missing {
		slot, err := f.ps.Upload(ctx, f.caller, svc.Uuid, key, psusecase.UploadMeta{
			Mime: "image/jpeg", Size: int64(len(img)), SHA256: psusecase.Digest(img), Ext: "jpg",
			ExifLat: &lat, ExifLng: &lng,
		})
		if err != nil {
			t.Fatalf("upload %s: %v", key, err)
		}
		if err := store.Upload(ctx, storage.File{Body: bytes.NewReader(img), Size: int64(len(img)), ContentType: "image/jpeg"}, slot.ObjectKey); err != nil {
			t.Fatal(err)
		}
	}
}

func intakeJPEG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 900, 600)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestContractRequiresIntakePhotos(t *testing.T) {
	d := newTestDB(t)
	store := storage.NewMemory()
	intake := d.newIntakeFixture(t, store, d.caller.OrganizationID)
	svc := New(repository.New(d.pool, d.q), WithStorage(store), WithIntakePhotos(intake.ps))
	tpl := d.create(t, model.KindVehicleIntake, true)
	if _, err := svc.PutLocale(d.ctx, d.caller, tpl.UUID, LocaleInput{Locale: "tr", HTML: `<p>{{service_no}}</p>`}); err != nil {
		t.Fatal(err)
	}
	row := d.createService(t, d.caller.OrganizationID)
	caller := d.caller
	caller.Filter = scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{d.caller.OrganizationID}, OrgID: d.caller.OrganizationID}

	// Module on + missing angle: 422 with the missing list.
	_, err := svc.CreateForService(d.ctx, caller, row.Uuid, CreateFromServiceInput{})
	var inc *psmodel.IncompleteError
	if !errors.As(err, &inc) || !containsKey(inc.Missing, intake.angleKey) {
		t.Fatalf("create with missing angle err = %v, want IncompleteError with %s", err, intake.angleKey)
	}

	// Module off: no rule.
	intake.feats[features.ModulePhotoStandard] = false
	contract, err := svc.CreateForService(d.ctx, caller, row.Uuid, CreateFromServiceInput{})
	if err != nil {
		t.Fatalf("module off create: %v", err)
	}
	intake.feats[features.ModulePhotoStandard] = true

	// Sending to signing re-checks (photos may be removed after creation).
	if _, err := svc.SignStaff(d.ctx, caller, contract.UUID, SignatureInput{PNGBase64: pngBase64()}); !errors.As(err, &inc) {
		t.Fatalf("staff sign with missing angle err = %v", err)
	}
	if _, err := svc.SignCustomer(d.ctx, caller, contract.UUID, SignatureInput{Code: "123456", PNGBase64: pngBase64()}); !errors.As(err, &inc) {
		t.Fatalf("customer sign with missing angle err = %v", err)
	}

	// All angles photographed: create passes.
	intake.photographAll(t, d.ctx, store, row, "41.1", "29.1")
	if _, err := svc.CreateForService(d.ctx, caller, row.Uuid, CreateFromServiceInput{}); err != nil {
		t.Fatalf("complete create: %v", err)
	}
}

// Executed contract PDF (fake Gotenberg, HTML snapshot): the
// {{intake_photos_html}} placeholder is filled with the angle names, the
// contract's own media stays separate, and no EXIF location is in the HTML.
func TestExecutedContractPDFEmbedsIntakePhotos(t *testing.T) {
	d := newTestDB(t)
	name := "tec499-contract " + uuid.NewString()
	docs := docusecase.New(d.pool, d.q, storage.NewMemory(), nil, nil, pdfrender.FontsEmbedded, slog.New(slog.NewTextHandler(io.Discard, nil)))
	view, err := docs.SaveDraft(d.ctx, 0, docusecase.SaveInput{
		Kind: docmodel.KindContract, BrandSlug: "olex", Language: "tr", Name: name,
		HTML: `<h1>{{contract_title}}</h1>{{contract_body_html}}{{signatures_html}}{{otp_proof_html}}<p>{{content_sha256}}</p>{{media_html}}<div id="intake">{{intake_photos_html}}</div>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := docs.Publish(d.ctx, view.UUID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.pool.Exec(context.Background(), `DELETE FROM document_templates WHERE name = $1`, name)
	})

	f := d.signingFixture(t, d.caller.OrganizationID)
	gotb := documentstest.NewGotenberg(t, 0)
	f.svc.pdf = pdfrender.New(gotb.URL)
	intake := d.newIntakeFixture(t, f.store, d.caller.OrganizationID)
	f.svc.intake = intake.ps
	inst, err := d.q.GetContractInstanceByUUID(d.ctx, f.contract.UUID)
	if err != nil {
		t.Fatal(err)
	}
	svcRow, err := d.q.GetServiceByUUID(d.ctx, db.GetServiceByUUIDParams{Uuid: mustServiceUUID(t, d, inst.SubjectID), BrandID: d.caller.BrandID})
	if err != nil {
		t.Fatal(err)
	}
	intake.photographAll(t, d.ctx, f.store, svcRow, "40.9876543", "29.1234567")

	executed := f.execute(t)
	row, err := d.q.GetContractInstanceByUUID(d.ctx, executed.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.GenerateExecutedPDF(d.ctx, row.ID); err != nil {
		t.Fatalf("generate pdf: %v", err)
	}
	html := gotb.LastHTML()
	for _, want := range []string{`<div id="intake"><section class="doc-intake-photos">`, "Araç kabul fotoğrafları", "Sol yan TEC499", "data:image/jpeg;base64,"} {
		if !strings.Contains(html, want) {
			t.Fatalf("PDF HTML missing %q", want)
		}
	}
	for _, leak := range []string{"40.9876543", "29.1234567", "40.98", "29.12", "exif"} {
		if strings.Contains(html, leak) {
			t.Fatalf("PDF HTML leaks %q", leak)
		}
	}
}

func mustServiceUUID(t *testing.T, d *testDB, id int64) uuid.UUID {
	t.Helper()
	var u uuid.UUID
	if err := d.pool.QueryRow(d.ctx, `SELECT uuid FROM services WHERE id = $1`, id).Scan(&u); err != nil {
		t.Fatal(err)
	}
	return u
}

func containsKey(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
