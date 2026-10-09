package usecase

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/photostandard/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeFeatures map[string]bool

func (f fakeFeatures) Enabled(_ context.Context, _ int64, key string) (bool, error) {
	return f[key], nil
}

type policyFixture struct {
	ctx          context.Context
	pool         *pgxpool.Pool
	q            *db.Queries
	svc          *Service
	feats        fakeFeatures
	out          *outbox.Memory
	store        *storage.Memory
	center       db.Organization
	dealer       db.Organization
	svcRow       db.Service
	dealerCaller Caller
	angleKey     string
}

func newPolicyFixture(t *testing.T) *policyFixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping photo standard database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatal(err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatal(err)
	}
	f := &policyFixture{ctx: ctx, pool: pool, q: q, feats: fakeFeatures{features.ModulePhotoStandard: true},
		out: outbox.NewMemory(), store: storage.NewMemory(), center: center}
	f.svc = New(pool, q).WithFeatures(f.feats).WithOutbox(f.out).WithStorage(f.store)
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	f.dealer, err = q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: "tec499-" + suffix, Name: "TEC499 Dealer", Status: "active", PlanCode: pgtype.Text{String: "test", Valid: true},
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}, Type: "dealer",
		ParentID: pgtype.Int8{Int64: center.ID, Valid: true}, BrandID: brand.ID, Currency: "TRY", Locale: "tr",
		Timezone: "Europe/Istanbul", Settings: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	f.svcRow = f.createService(t, suffix)
	f.angleKey = "t499-" + suffix
	centerCaller := Caller{Org: orgctx.Scope{InternalID: center.ID, BrandID: brand.ID, OrgType: rbac.OrgTypeCenter}}
	if _, err := f.svc.CreateAngle(ctx, centerCaller, AngleInput{
		Key: f.angleKey, Name: []byte(`{"tr":"Ön kaput TEC499","en":"Front hood TEC499"}`), Required: true, SortOrder: -1000, Active: true,
	}); err != nil {
		t.Fatalf("create angle: %v", err)
	}
	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM intake_photos WHERE service_id = $1`, f.svcRow.ID)
		_, _ = pool.Exec(c, `DELETE FROM photo_angle_overrides WHERE organization_id = $1`, f.dealer.ID)
		_, _ = pool.Exec(c, `DELETE FROM photo_angles WHERE key = $1`, f.angleKey)
	})
	f.dealerCaller = Caller{
		Principal: authctx.Principal{UserInternal: f.svcRow.CreatedByUserID.Int64, Roles: []string{rbac.RoleDealerOwner}},
		Org:       orgctx.Scope{InternalID: f.dealer.ID, UUID: f.dealer.Uuid, BrandID: brand.ID, OrgType: rbac.OrgTypeDealer},
		Filter:    scopefilter.Filter{Scope: rbac.ScopeManaged, OrgID: f.dealer.ID, OrgIDs: []int64{f.dealer.ID}},
	}
	return f
}

func (f *policyFixture) createService(t *testing.T, suffix string) db.Service {
	t.Helper()
	user, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		Email: pgtype.Text{String: "tec499." + suffix + "@example.test", Valid: true}, PasswordHash: "x",
		Name: "TEC499", Surname: "Customer", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	cb, err := f.q.CreateCarBrand(f.ctx, db.CreateCarBrandParams{
		ExternalID: pgtype.Text{String: "tec499-" + suffix, Valid: true}, Name: "TEC499 Brand " + suffix, ShowName: true, Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	cm, err := f.q.CreateCarModel(f.ctx, db.CreateCarModelParams{CarBrandID: cb.ID, Name: "TEC499 Model " + suffix, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	veh, err := f.q.CreateVehicle(f.ctx, db.CreateVehicleParams{
		UserID: user.ID, OrganizationID: pgtype.Int8{Int64: f.dealer.ID, Valid: true}, BrandID: f.dealer.BrandID,
		CarBrandID: pgtype.Int8{Int64: cb.ID, Valid: true}, CarModelID: pgtype.Int8{Int64: cm.ID, Valid: true},
		Plate: pgtype.Text{String: "34 TEC 499", Valid: true}, PlateNormalized: pgtype.Text{String: "34TEC499", Valid: true},
		PlateCountry: pgtype.Text{String: "TR", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	row, err := f.q.CreateService(f.ctx, db.CreateServiceParams{
		ServiceNo: "DS" + strings.ToUpper(suffix[:8]), OrganizationID: f.dealer.ID, BrandID: f.dealer.BrandID,
		CustomerUserID: user.ID, VehicleID: veh.ID, CarBrandID: cb.ID, CarModelID: cm.ID,
		Plate: pgtype.Text{String: "34 TEC 499", Valid: true}, PlateCountry: pgtype.Text{String: "TR", Valid: true},
		Status: "draft", CreatedByUserID: pgtype.Int8{Int64: user.ID, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func (f *policyFixture) ref() model.ServiceRef {
	return model.ServiceRef{ID: f.svcRow.ID, OrganizationID: f.dealer.ID, BrandID: f.dealer.BrandID}
}

func (f *policyFixture) missing(t *testing.T) []string {
	t.Helper()
	err := f.svc.RequireComplete(f.ctx, nil, f.ref())
	if err == nil {
		return nil
	}
	var inc *model.IncompleteError
	if !errors.As(err, &inc) {
		t.Fatalf("RequireComplete err = %v", err)
	}
	return inc.Missing
}

// upload stores a JPEG for the angle the way the handler does (DB row via
// Upload, bytes in the object store).
func (f *policyFixture) upload(t *testing.T, key string, body []byte, lat, lng, device *string) {
	t.Helper()
	slot, err := f.svc.Upload(f.ctx, f.dealerCaller, f.svcRow.Uuid, key, UploadMeta{
		Mime: "image/jpeg", Size: int64(len(body)), SHA256: Digest(body), Ext: "jpg",
		ExifLat: lat, ExifLng: lng, ExifDevice: device,
	})
	if err != nil {
		t.Fatalf("upload %s: %v", key, err)
	}
	if err := f.store.Upload(f.ctx, storage.File{Body: bytes.NewReader(body), Size: int64(len(body)), ContentType: "image/jpeg"}, slot.ObjectKey); err != nil {
		t.Fatal(err)
	}
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func completedEvents(out *outbox.Memory) int {
	n := 0
	for _, row := range out.All() {
		if row.EventName == events.ServiceIntakePhotosCompleted {
			n++
		}
	}
	return n
}

func TestRequireCompleteRule(t *testing.T) {
	f := newPolicyFixture(t)

	// Module on + missing required angle: the error lists it.
	if got := f.missing(t); !contains(got, f.angleKey) {
		t.Fatalf("missing = %v, want %s", got, f.angleKey)
	}

	// Module off: no rule at all.
	f.feats[features.ModulePhotoStandard] = false
	if err := f.svc.RequireComplete(f.ctx, nil, f.ref()); err != nil {
		t.Fatalf("module off err = %v", err)
	}
	f.feats[features.ModulePhotoStandard] = true

	// Override required=false on the dealer for every missing angle: passes
	// with the photos still missing.
	missing := f.missing(t)
	items := make([]OverrideInput, 0, len(missing))
	for _, k := range missing {
		items = append(items, OverrideInput{AngleKey: k, Required: false})
	}
	centerCaller := Caller{Org: orgctx.Scope{InternalID: f.center.ID, BrandID: f.center.BrandID, OrgType: rbac.OrgTypeCenter},
		Filter: scopefilter.Filter{Scope: rbac.ScopeAll}}
	if _, err := f.svc.PutOverrides(f.ctx, centerCaller, f.dealer.Uuid, items); err != nil {
		t.Fatalf("overrides: %v", err)
	}
	if err := f.svc.RequireComplete(f.ctx, nil, f.ref()); err != nil {
		t.Fatalf("optional override err = %v", err)
	}
	if _, err := f.svc.PutOverrides(f.ctx, centerCaller, f.dealer.Uuid, nil); err != nil {
		t.Fatalf("clear overrides: %v", err)
	}

	// Every required angle photographed: passes, and the completion event
	// is written exactly once (the last missing angle), not on replacement.
	missing = f.missing(t)
	if len(missing) == 0 {
		t.Fatal("expected missing angles after clearing overrides")
	}
	img := testJPEG(t)
	for i, k := range missing {
		f.upload(t, k, img, nil, nil, nil)
		if want := 0; i < len(missing)-1 && completedEvents(f.out) != want {
			t.Fatalf("event before completion after %d/%d uploads", i+1, len(missing))
		}
	}
	if err := f.svc.RequireComplete(f.ctx, nil, f.ref()); err != nil {
		t.Fatalf("complete err = %v", err)
	}
	if n := completedEvents(f.out); n != 1 {
		t.Fatalf("completed events = %d, want 1", n)
	}
	f.upload(t, f.angleKey, img, nil, nil, nil)
	if n := completedEvents(f.out); n != 1 {
		t.Fatalf("completed events after replacement = %d, want 1", n)
	}
}

// The PDF grid carries the angle name and capture time, never the EXIF
// location or device; the embedded image is re-encoded (no EXIF block).
func TestIntakePhotosHTMLHasNoLocation(t *testing.T) {
	f := newPolicyFixture(t)
	raw, err := os.ReadFile("../handler/testdata/exif.jpg")
	if err != nil {
		t.Fatal(err)
	}
	lat, lng, device := "41.0082376", "28.9783589", "TEC499Phone X"
	f.upload(t, f.angleKey, raw, &lat, &lng, &device)

	out, err := f.svc.IntakePhotosHTML(f.ctx, nil, f.ref(), "tr")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Araç kabul fotoğrafları", "Ön kaput TEC499", "data:image/jpeg;base64,", `class="doc-photos"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("html missing %q:\n%.400s", want, out)
		}
	}
	for _, leak := range []string{lat, lng, "41.008", "28.978", device} {
		if strings.Contains(out, leak) {
			t.Fatalf("html leaks %q", leak)
		}
	}
	m := regexp.MustCompile(`data:image/jpeg;base64,([A-Za-z0-9+/=]+)`).FindStringSubmatch(out)
	if m == nil {
		t.Fatal("no embedded image")
	}
	data, err := base64.StdEncoding.DecodeString(m[1])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("Exif\x00")) || bytes.Contains(data, []byte("GPS")) {
		t.Fatal("embedded image still carries EXIF")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width > PDFPhotoMaxPx || cfg.Height > PDFPhotoMaxPx {
		t.Fatalf("embedded image %dx%d err=%v", cfg.Width, cfg.Height, err)
	}

	// English label in an English contract.
	en, err := f.svc.IntakePhotosHTML(f.ctx, nil, f.ref(), "en")
	if err != nil || !strings.Contains(en, "Front hood TEC499") {
		t.Fatalf("en html err=%v", err)
	}
}

// EXIF location and device reach only the dealer owner, the center and
// super admins.
func TestIntakeEXIFVisibility(t *testing.T) {
	f := newPolicyFixture(t)
	lat, lng, device := "41.0000000", "29.0000000", "TEC499Phone"
	f.upload(t, f.angleKey, testJPEG(t), &lat, &lng, &device)

	photoFor := func(c Caller) *IntakePhotoView {
		view, err := f.svc.Intake(f.ctx, c, f.svcRow.Uuid)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range view.Angles {
			if a.Angle.Key == f.angleKey {
				return a.Photo
			}
		}
		t.Fatal("angle not in intake view")
		return nil
	}
	if p := photoFor(f.dealerCaller); p == nil || p.ExifLat == nil || p.ExifDevice == nil {
		t.Fatalf("dealer owner should see EXIF: %+v", p)
	}
	staff := f.dealerCaller
	staff.Principal.Roles = []string{rbac.RoleDealerStaff}
	if p := photoFor(staff); p == nil || p.ExifLat != nil || p.ExifLng != nil || p.ExifDevice != nil {
		t.Fatalf("dealer staff must not see EXIF location: %+v", p)
	}
	center := Caller{Org: orgctx.Scope{InternalID: f.center.ID, BrandID: f.center.BrandID, OrgType: rbac.OrgTypeCenter},
		Filter: scopefilter.Filter{Scope: rbac.ScopeBrand, BrandID: f.center.BrandID}}
	if p := photoFor(center); p == nil || p.ExifLat == nil {
		t.Fatalf("center should see EXIF: %+v", p)
	}
}

func TestAngleNameFallback(t *testing.T) {
	raw := []byte(`{"tr":"Ön","en":"Front","de":"Vorne"}`)
	cases := map[string]string{"de": "Vorne", "de-AT": "Vorne", "fr": "Ön", "": "Ön"}
	for loc, want := range cases {
		if got := AngleName(raw, loc, "front"); got != want {
			t.Fatalf("AngleName(%q) = %q, want %q", loc, got, want)
		}
	}
	if got := AngleName([]byte(`{}`), "tr", "front"); got != "front" {
		t.Fatalf("empty names = %q", got)
	}
}

func testJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1200, 800))
	for x := 0; x < 1200; x += 10 {
		img.Set(x, x%800, color.RGBA{R: 200, A: 255})
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
