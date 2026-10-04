package usecase

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/documentstest"
	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	docusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/otp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testDB struct {
	t      *testing.T
	ctx    context.Context
	pool   *pgxpool.Pool
	q      *db.Queries
	svc    *Service
	caller Caller
	prefix string
}

func newTestDB(t *testing.T) *testDB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping contracts database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
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
	prefix := "TEC286 " + uuid.NewString()
	var oldDefaultID int64
	_ = pool.QueryRow(ctx, `SELECT id FROM contract_templates WHERE brand_id = $1 AND kind = 'vehicle_intake' AND is_default LIMIT 1`, brand.ID).Scan(&oldDefaultID)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE contract_templates SET is_default = false WHERE brand_id = $1 AND kind = 'vehicle_intake' AND name LIKE $2`, brand.ID, prefix+"%")
		if oldDefaultID > 0 {
			_, _ = pool.Exec(ctx, `UPDATE contract_templates SET is_default = true, is_active = true WHERE id = $1`, oldDefaultID)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM contract_templates WHERE name LIKE $1`, prefix+"%")
	})
	return &testDB{
		t: t, ctx: ctx, pool: pool, q: q, svc: New(repository.New(pool, q)),
		caller: Caller{UserID: 0, OrganizationID: center.ID, BrandID: brand.ID}, prefix: prefix,
	}
}

func (d *testDB) create(t *testing.T, kind string, def bool) model.Template {
	t.Helper()
	item, err := d.svc.Create(d.ctx, d.caller, Input{
		Name: d.prefix + " " + kind + " " + time.Now().Format("150405.000000"),
		Kind: kind, IsDefault: &def,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return item
}

func TestSetDefaultClearsPreviousDefault(t *testing.T) {
	d := newTestDB(t)
	first := d.create(t, model.KindVehicleIntake, true)
	second := d.create(t, model.KindVehicleIntake, false)

	if _, err := d.svc.SetDefault(d.ctx, d.caller, second.UUID); err != nil {
		t.Fatalf("set default: %v", err)
	}
	gotFirst, err := d.svc.Get(d.ctx, d.caller, first.UUID)
	if err != nil {
		t.Fatal(err)
	}
	gotSecond, err := d.svc.Get(d.ctx, d.caller, second.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if gotFirst.IsDefault || !gotSecond.IsDefault {
		t.Fatalf("defaults = first %v, second %v", gotFirst.IsDefault, gotSecond.IsDefault)
	}
}

func TestPrepareHTMLValidationAndSanitize(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		wantErr bool
	}{
		{name: "known variable", html: `<p>{{ customer_name }}</p>`},
		{name: "unknown variable", html: `<p>{{ nope }}</p>`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PrepareHTML(tt.html)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "nope") {
					t.Fatalf("err = %v, want unknown variable", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("PrepareHTML: %v", err)
			}
			if got == "" {
				t.Fatal("empty sanitized html")
			}
		})
	}

	out, err := PrepareHTML(`<p>ok</p><script>alert(1)</script>`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(out), "script") {
		t.Fatalf("script survived sanitize: %s", out)
	}
}

func TestRenderFallsBackToTurkishAndEscapesValues(t *testing.T) {
	d := newTestDB(t)
	tpl := d.create(t, model.KindVehicleIntake, false)
	if _, err := d.svc.PutLocale(d.ctx, d.caller, tpl.UUID, LocaleInput{
		Locale: "tr", HTML: `<p>Merhaba {{customer_name}}</p>`,
	}); err != nil {
		t.Fatalf("put tr: %v", err)
	}
	rendered, err := d.svc.Render(d.ctx, d.caller, model.RenderInput{
		TemplateUUID: tpl.UUID, Locale: "fr", Values: map[string]string{"customer_name": `<Ada & Co>`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rendered.Locale != "tr" {
		t.Fatalf("locale = %s, want tr", rendered.Locale)
	}
	if !strings.Contains(rendered.HTML, "&lt;Ada &amp; Co&gt;") {
		t.Fatalf("value was not escaped: %s", rendered.HTML)
	}
}

func TestSigningFlowExecutesOnce(t *testing.T) {
	d := newTestDB(t)
	f := d.signingFixture(t, d.caller.OrganizationID)

	if _, err := f.svc.SignCustomer(d.ctx, f.caller, f.contract.UUID, SignatureInput{
		Code: "000000", PNGBase64: pngBase64(),
	}); !errors.Is(err, ErrInvalidOTP) {
		t.Fatalf("wrong OTP err = %v, want ErrInvalidOTP", err)
	}
	got, err := f.svc.SignCustomer(d.ctx, f.caller, f.contract.UUID, SignatureInput{
		Code: "123456", PNGBase64: pngBase64(), IP: "203.0.113.10", UserAgent: "go-test",
	})
	if err != nil {
		t.Fatalf("customer sign: %v", err)
	}
	if got.Signers[0].SignedAt == nil && got.Signers[1].SignedAt == nil {
		t.Fatal("customer signer was not marked signed")
	}
	got, err = f.svc.SignStaff(d.ctx, f.caller, f.contract.UUID, SignatureInput{PNGBase64: pngBase64()})
	if err != nil {
		t.Fatalf("staff sign: %v", err)
	}
	if got.Status != model.StatusExecuted || got.ExecutedAt == nil {
		t.Fatalf("status = %s executed_at=%v, want executed", got.Status, got.ExecutedAt)
	}
	if rows := f.out.All(); len(rows) != 1 || rows[0].EventName != "contract.executed" {
		t.Fatalf("outbox rows = %+v, want one contract.executed", rows)
	}
	if _, err := f.svc.SignStaff(d.ctx, f.caller, f.contract.UUID, SignatureInput{PNGBase64: pngBase64()}); !errors.Is(err, ErrAlreadySigned) {
		t.Fatalf("second staff sign err = %v, want ErrAlreadySigned", err)
	}
	if rows := f.out.All(); len(rows) != 1 {
		t.Fatalf("outbox rows after second sign = %d, want 1", len(rows))
	}
}

func TestCustomerSignWindowExpires(t *testing.T) {
	d := newTestDB(t)
	f := d.signingFixture(t, d.caller.OrganizationID)
	f.otp.createdAt = f.now.Add(-31 * time.Minute)
	if _, err := f.svc.SignCustomer(d.ctx, f.caller, f.contract.UUID, SignatureInput{
		Code: "123456", PNGBase64: pngBase64(),
	}); !errors.Is(err, ErrWindowExpired) {
		t.Fatalf("err = %v, want ErrWindowExpired", err)
	}
}

func TestExecutedContractPDFGeneratedOnce(t *testing.T) {
	d := newTestDB(t)
	f := d.signingFixture(t, d.caller.OrganizationID)
	d.publishContractDocumentTemplate(t)
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
	row, err = d.q.GetContractInstanceByUUID(d.ctx, executed.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if !row.PdfKey.Valid || row.PdfKey.String == "" {
		t.Fatal("pdf_key was not set")
	}
	if ok, err := f.store.Exists(d.ctx, row.PdfKey.String); err != nil || !ok {
		t.Fatalf("stored pdf exists = %v, %v; want true", ok, err)
	}
	if calls := gotb.Calls.Load(); calls != 1 {
		t.Fatalf("gotenberg calls = %d, want 1", calls)
	}
	if err := f.svc.GenerateExecutedPDF(d.ctx, row.ID); err != nil {
		t.Fatalf("second generate pdf: %v", err)
	}
	if calls := gotb.Calls.Load(); calls != 1 {
		t.Fatalf("gotenberg calls after second event = %d, want 1", calls)
	}
}

func TestExecutedContractPDFHTMLContainsEvidence(t *testing.T) {
	d := newTestDB(t)
	f := d.signingFixture(t, d.caller.OrganizationID)
	d.publishContractDocumentTemplate(t)
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
	html := gotb.LastHTML()
	for _, want := range []string{"data:image/png;base64", "KVKK notice version", row.ContentSha256.String} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered HTML missing %q:\n%s", want, html)
		}
	}
	if count := strings.Count(html, "data:image/png;base64"); count < 2 {
		t.Fatalf("signature images = %d, want at least 2", count)
	}
}

func TestInvalidPNGAndOtherOrgService(t *testing.T) {
	d := newTestDB(t)
	f := d.signingFixture(t, d.caller.OrganizationID)
	if _, err := f.svc.SignStaff(d.ctx, f.caller, f.contract.UUID, SignatureInput{
		PNGBase64: base64.StdEncoding.EncodeToString([]byte("not a png")),
	}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("non-png err = %v, want ErrInvalidRequest", err)
	}
	otherOrg := d.createOrg(t)
	other := d.createService(t, otherOrg.ID)
	if _, err := f.svc.CreateForService(d.ctx, f.caller, other.Uuid, CreateFromServiceInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other org service err = %v, want ErrNotFound", err)
	}
}

type signingFixture struct {
	svc      *Service
	caller   Caller
	contract model.Contract
	otp      *fakeOTP
	out      *outbox.Memory
	store    *storage.Memory
	now      time.Time
}

func (d *testDB) signingFixture(t *testing.T, orgID int64) signingFixture {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	fake := &fakeOTP{q: d.q, now: now, createdAt: now}
	out := outbox.NewMemory()
	store := storage.NewMemory()
	svc := New(repository.New(d.pool, d.q), WithOTP(fake), WithStorage(store), WithOutbox(out), WithClock(func() time.Time { return now }))
	tpl := d.create(t, model.KindVehicleIntake, true)
	if _, err := svc.PutLocale(d.ctx, d.caller, tpl.UUID, LocaleInput{
		Locale: "tr", HTML: `<p>{{service_no}} {{customer_name}} {{staff_name}}</p>`,
	}); err != nil {
		t.Fatalf("put locale: %v", err)
	}
	svcRow := d.createService(t, orgID)
	caller := d.caller
	caller.Filter = scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{orgID}, OrgID: orgID, UserID: d.caller.UserID}
	contract, err := svc.CreateForService(d.ctx, caller, svcRow.Uuid, CreateFromServiceInput{})
	if err != nil {
		t.Fatalf("create contract: %v", err)
	}
	return signingFixture{svc: svc, caller: caller, contract: contract, otp: fake, out: out, store: store, now: now}
}

func (f signingFixture) execute(t *testing.T) model.Contract {
	t.Helper()
	if _, err := f.svc.SignCustomer(context.Background(), f.caller, f.contract.UUID, SignatureInput{
		Code: "123456", PNGBase64: pngBase64(), IP: "203.0.113.10", UserAgent: "go-test",
	}); err != nil {
		t.Fatalf("customer sign: %v", err)
	}
	got, err := f.svc.SignStaff(context.Background(), f.caller, f.contract.UUID, SignatureInput{PNGBase64: pngBase64()})
	if err != nil {
		t.Fatalf("staff sign: %v", err)
	}
	return got
}

func (d *testDB) publishContractDocumentTemplate(t *testing.T) {
	t.Helper()
	svc := docusecase.New(d.pool, d.q, storage.NewMemory(), nil, nil, pdfrender.FontsEmbedded, slog.New(slog.NewTextHandler(io.Discard, nil)))
	name := "tec288-contract " + uuid.NewString()
	view, err := svc.SaveDraft(d.ctx, 0, docusecase.SaveInput{
		Kind: docmodel.KindContract, BrandSlug: "olex", Language: "tr", Name: name,
		HTML: `<h1>{{contract_title}}</h1>{{contract_body_html}}{{signatures_html}}<h2>OTP</h2>{{otp_proof_html}}<p>{{content_sha256}}</p>{{media_html}}`,
	})
	if err != nil {
		t.Fatalf("save document template: %v", err)
	}
	if _, err := svc.Publish(d.ctx, view.UUID); err != nil {
		t.Fatalf("publish document template: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.pool.Exec(context.Background(), `DELETE FROM document_templates WHERE name = $1`, name)
	})
}

func (d *testDB) createService(t *testing.T, orgID int64) db.Service {
	t.Helper()
	customer := d.createUser(t, "customer")
	staff := d.createUser(t, "staff")
	carBrand, err := d.q.CreateCarBrand(d.ctx, db.CreateCarBrandParams{
		ExternalID: pgText("tec287-" + uuid.NewString()), Name: "TEC287 Brand " + uuid.NewString(), ShowName: true, Active: true,
	})
	if err != nil {
		t.Fatalf("car brand: %v", err)
	}
	carModel, err := d.q.CreateCarModel(d.ctx, db.CreateCarModelParams{
		CarBrandID: carBrand.ID, Name: "TEC287 Model " + uuid.NewString(), Active: true,
	})
	if err != nil {
		t.Fatalf("car model: %v", err)
	}
	vehicle, err := d.q.CreateVehicle(d.ctx, db.CreateVehicleParams{
		UserID: customer.ID, OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, BrandID: d.caller.BrandID,
		CarBrandID: pgtype.Int8{Int64: carBrand.ID, Valid: true}, CarModelID: pgtype.Int8{Int64: carModel.ID, Valid: true},
		ModelYear: pgtype.Int2{Int16: 2024, Valid: true}, Plate: pgText("34 TST 287"), PlateNormalized: pgText("34TST287"),
		PlateCountry: pgText("TR"), Vin: pgText("WVWZZZ1JZ3W386752"),
	})
	if err != nil {
		t.Fatalf("vehicle: %v", err)
	}
	row, err := d.q.CreateService(d.ctx, db.CreateServiceParams{
		ServiceNo:      "DS" + strings.ToUpper(strings.ReplaceAll(uuid.NewString()[:8], "-", "")),
		OrganizationID: orgID, BrandID: d.caller.BrandID, CustomerUserID: customer.ID, VehicleID: vehicle.ID,
		CarBrandID: carBrand.ID, CarModelID: carModel.ID, ModelYear: pgtype.Int2{Int16: 2024, Valid: true},
		Plate: pgText("34 TST 287"), PlateCountry: pgText("TR"), Vin: pgText("WVWZZZ1JZ3W386752"),
		Package: pgText("PPF"), HasMeasurement: false, Status: "draft", CreatedByUserID: pgtype.Int8{Int64: staff.ID, Valid: true},
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	return row
}

func (d *testDB) createUser(t *testing.T, kind string) db.User {
	t.Helper()
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	phoneSuffix := fmt.Sprintf("%09d", time.Now().UnixNano()%1_000_000_000)
	row, err := d.q.CreateUser(d.ctx, db.CreateUserParams{
		Email: pgText(kind + "." + id + "@example.test"), PasswordHash: "x",
		Name: strings.ToUpper(kind[:1]) + kind[1:], Surname: "TEC287", Status: "active",
		PhoneE164: pgText("+90555" + phoneSuffix), PhoneVerifiedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	return row
}

func (d *testDB) createOrg(t *testing.T) db.Organization {
	t.Helper()
	row, err := d.q.CreateOrganization(d.ctx, db.CreateOrganizationParams{
		Slug: "tec287-" + uuid.NewString(), Name: "TEC287 Other", Status: "active", PlanCode: pgText("test"),
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		Type:           "dealer", ParentID: pgtype.Int8{Int64: d.caller.OrganizationID, Valid: true},
		BrandID: d.caller.BrandID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("org: %v", err)
	}
	return row
}

type fakeOTP struct {
	q         *db.Queries
	now       time.Time
	createdAt time.Time
}

func (f *fakeOTP) Request(context.Context, otp.RequestInput) (otp.RequestResult, error) {
	return otp.RequestResult{Channel: "fake", ExpiresAt: f.now.Add(5 * time.Minute), ResendAt: f.now.Add(time.Minute)}, nil
}

func (f *fakeOTP) Verify(ctx context.Context, in otp.VerifyInput) (otp.Verified, error) {
	if strings.TrimSpace(in.Code) != "123456" {
		return otp.Verified{}, otp.ErrInvalidCode
	}
	row, err := f.q.CreatePhoneOTP(ctx, db.CreatePhoneOTPParams{
		Uuid: uuid.New(), PhoneE164: pgText(in.Phone), CodeHash: "fake", Type: otp.PurposeContractSign,
		ExpiresAt: pgtype.Timestamptz{Time: f.createdAt.Add(5 * time.Minute), Valid: true}, MaxAttempts: 5,
		KvkkLocale: pgText("tr"), KvkkVersion: pgtype.Int4{Int32: 1, Valid: true}, MessageSha256: pgText(strings.Repeat("a", 64)),
		CreatedAt: pgtype.Timestamptz{Time: f.createdAt, Valid: true},
	})
	if err != nil {
		return otp.Verified{}, err
	}
	if err := f.q.ConsumeOTPAt(ctx, db.ConsumeOTPAtParams{ID: row.ID, Now: pgtype.Timestamptz{Time: f.now, Valid: true}}); err != nil {
		return otp.Verified{}, err
	}
	return otp.Verified{ID: row.Uuid, Phone: in.Phone}, nil
}

func pngBase64() string { return base64.StdEncoding.EncodeToString(testPNG()) }

func testPNG() []byte {
	raw, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	return raw
}
