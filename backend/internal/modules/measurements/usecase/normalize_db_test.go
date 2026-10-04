package usecase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dbFixture runs every test inside one rolled back transaction.
type dbFixture struct {
	t    *testing.T
	ctx  context.Context
	tx   pgx.Tx
	q    *db.Queries
	org  db.Organization
	user db.User
}

func newDBFixture(t *testing.T) *dbFixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	f := &dbFixture{t: t, ctx: ctx, tx: tx, q: db.New(tx)}
	f.org = f.newOrg("a")
	f.user, err = f.q.CreateUser(ctx, db.CreateUserParams{
		Email:        pgtype.Text{String: fmt.Sprintf("tec294-%d@example.test", time.Now().UnixNano()), Valid: true},
		PasswordHash: "x", Name: "tec294", Surname: "User", Status: "active",
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	return f
}

func (f *dbFixture) newOrg(slug string) db.Organization {
	f.t.Helper()
	var brand, center int64
	if err := f.tx.QueryRow(f.ctx, `SELECT b.id, o.id FROM brands b JOIN organizations o ON o.brand_id = b.id AND o.type = 'center'
		WHERE b.slug = 'olex' ORDER BY o.id LIMIT 1`).Scan(&brand, &center); err != nil {
		f.t.Fatalf("brand: %v", err)
	}
	slug = fmt.Sprintf("tec294-%s-%d", slug, time.Now().UnixNano())
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: slug, Name: slug, Status: "active", Type: "dealer",
		ParentID: pgtype.Int8{Int64: center, Valid: true}, BrandID: brand,
		Currency: "TRY", Locale: "tr", Timezone: "UTC", Settings: []byte(`{}`),
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	if err != nil {
		f.t.Fatalf("org: %v", err)
	}
	return o
}

func (f *dbFixture) svc() *Service { return New(f.q).WithNormalizer(f.tx, nil) }

func (f *dbFixture) caller(o db.Organization) Caller {
	return Caller{UserID: f.user.ID, OrganizationID: o.ID, BrandID: o.BrandID}
}

func (f *dbFixture) panel(o db.Organization) PanelCaller {
	return PanelCaller{
		Org:    orgctx.Scope{InternalID: o.ID, UUID: o.Uuid, BrandID: o.BrandID},
		Filter: scopefilter.Filter{Permission: rbac.PermMeasurementsLink, Scope: rbac.ScopeManaged, OrgID: o.ID, OrgIDs: []int64{o.ID}},
	}
}

func (f *dbFixture) result(id uuid.UUID) db.MeasurementResult {
	f.t.Helper()
	row, err := f.q.GetMeasurementResultByUUID(f.ctx, db.GetMeasurementResultByUUIDParams{Uuid: id, OrganizationID: f.org.ID})
	if err != nil {
		f.t.Fatalf("result: %v", err)
	}
	return row
}

func (f *dbFixture) counts(resultID int64) (values, tires int) {
	f.t.Helper()
	if err := f.tx.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM measurement_values WHERE result_id = $1),
		(SELECT count(*) FROM measurement_tires WHERE result_id = $1)`, resultID).Scan(&values, &tires); err != nil {
		f.t.Fatal(err)
	}
	return values, tires
}

func (f *dbFixture) devices(o db.Organization) []db.MeasurementDevice {
	f.t.Helper()
	rows, err := f.q.ListMeasurementDevices(f.ctx, o.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	return rows
}

// TEC-294: a mobile upload is normalized in its insert transaction: the
// fixture's 15 readings and 4 tires, measured_at and body type; the unknown
// serial registers a device (model from raw) and a second upload reuses it.
func TestDBMobileUploadNormalizes(t *testing.T) {
	f := newDBFixture(t)
	s := f.svc()
	res, err := s.Create(f.ctx, f.caller(f.org), Input{Body: fixture(t, "nexptg_mobile.json")})
	if err != nil {
		t.Fatal(err)
	}
	row := f.result(res.UUID)
	if res.Status != StatusVINPending || !row.ParsedAt.Valid || !row.MeasuredAt.Time.Equal(fixtureMeasuredAt) ||
		row.BodyType.String != "SEDAN" || !row.DeviceID.Valid {
		t.Fatalf("row = %+v", row)
	}
	if v, tires := f.counts(row.ID); v != 15 || tires != 4 {
		t.Fatalf("values %d tires %d", v, tires)
	}
	devs := f.devices(f.org)
	if len(devs) != 1 || devs[0].ID != row.DeviceID.Int64 || devs[0].Serial != "18416 Professional" ||
		devs[0].Model.String != "Professional" || !devs[0].IsActive {
		t.Fatalf("devices = %+v", devs)
	}

	// A second upload of the same device: same registry row.
	in := Input{IdempotencyKey: "tec294-second"}
	in.Body = []byte(`{"client_measurement_id":"tec294-second","device":{"serial":"18416 Professional"},` +
		`"raw":{"date":1702629999,"data":[{"placeId":"top","data":[{"type":"HOOD","values":[{"value":"101","position":1}]}]}]}}`)
	res2, err := s.Create(f.ctx, f.caller(f.org), in)
	if err != nil {
		t.Fatal(err)
	}
	row2 := f.result(res2.UUID)
	if row2.DeviceID != row.DeviceID || len(f.devices(f.org)) != 1 {
		t.Fatalf("second device = %+v, devices %d", row2.DeviceID, len(f.devices(f.org)))
	}

	// A repeat of the first upload's key writes nothing and keeps one set.
	again, err := s.Create(f.ctx, f.caller(f.org), Input{Body: fixture(t, "nexptg_mobile.json")})
	if err != nil || !again.Replayed || again.UUID != res.UUID {
		t.Fatalf("replay = %+v, %v", again, err)
	}
	if v, tires := f.counts(row.ID); v != 15 || tires != 4 {
		t.Fatalf("after replay values %d tires %d", v, tires)
	}
}

// TEC-294: a broken raw is still accepted; parsed_at stays NULL and
// nothing is written besides the raw row.
func TestDBBrokenRawAccepted(t *testing.T) {
	f := newDBFixture(t)
	res, err := f.svc().Create(f.ctx, f.caller(f.org), Input{
		Body: []byte(`{"device":{"serial":"NX-BROKEN"},"raw":{"device":"NexPTG","values":[98,102]}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	row := f.result(res.UUID)
	if row.ParsedAt.Valid || row.MeasuredAt.Valid || row.DeviceID.Valid {
		t.Fatalf("broken row = %+v", row)
	}
	if v, tires := f.counts(row.ID); v != 0 || tires != 0 || len(f.devices(f.org)) != 0 {
		t.Fatalf("broken wrote values %d tires %d devices %d", v, tires, len(f.devices(f.org)))
	}
}

// TEC-294: the backfill normalizes unparsed rows (the migrator's
// legacy_import ones too); a second run writes nothing.
func TestDBReparseIsIdempotent(t *testing.T) {
	f := newDBFixture(t)
	insert := func(raw []byte) int64 {
		t.Helper()
		id, err := f.q.MigratorInsertMeasurementResult(f.ctx, db.MigratorInsertMeasurementResultParams{
			Uuid: uuid.New(), OrganizationID: f.org.ID, BrandID: f.org.BrandID, Status: StatusVINPending, Raw: raw,
			DeviceSerial: pgtype.Text{String: "18416 Professional", Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	legacy := insert(fixture(t, "nexptg_legacy.json"))
	broken := insert([]byte(`{"legacy":{"system":"hub"},"report":{"id":1},"measurements":[]}`))

	first, err := Reparse(f.ctx, f.tx, f.q, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := f.counts(legacy); v != 15 || first.Parsed < 1 || first.Values < 15 {
		t.Fatalf("first run = %+v, legacy values %d", first, v)
	}
	var parsed, measured pgtype.Timestamptz
	var device pgtype.Int8
	if err := f.tx.QueryRow(f.ctx, `SELECT parsed_at, measured_at, device_id FROM measurement_results WHERE id = $1`, legacy).
		Scan(&parsed, &measured, &device); err != nil {
		t.Fatal(err)
	}
	if !parsed.Valid || !measured.Time.Equal(fixtureMeasuredAt) || !device.Valid {
		t.Fatalf("legacy row parsed %v measured %v device %v", parsed, measured, device)
	}
	if err := f.tx.QueryRow(f.ctx, `SELECT parsed_at FROM measurement_results WHERE id = $1`, broken).Scan(&parsed); err != nil || parsed.Valid {
		t.Fatalf("broken legacy row parsed = %v, %v", parsed, err)
	}

	var before int
	countRows := func() int {
		var n int
		if err := f.tx.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM measurement_values) + (SELECT count(*) FROM measurement_tires)
			+ (SELECT count(*) FROM measurement_devices)`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before = countRows()
	second, err := Reparse(f.ctx, f.tx, f.q, 50, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.Parsed != 0 || second.Values != 0 || second.Tires != 0 || second.DevicesCreated != 0 || countRows() != before {
		t.Fatalf("second run = %+v, rows %d -> %d", second, before, countRows())
	}
}

// TEC-294: VIN completion vin_pending -> accepted; an invalid VIN is a
// validation error; another VIN later is refused; another organization's
// measurement is not found.
func TestDBCompleteVIN(t *testing.T) {
	f := newDBFixture(t)
	s := f.svc()
	res, err := s.Create(f.ctx, f.caller(f.org), Input{Body: fixture(t, "nexptg_mobile.json")})
	if err != nil || res.Status != StatusVINPending {
		t.Fatalf("upload = %+v, %v", res, err)
	}
	if err := EnsureLinkable(res.Status); !errors.Is(err, ErrVINPending) {
		t.Fatalf("pending linkable = %v", err)
	}
	for _, bad := range []string{"", "WVWZZZ3CZKE01234", "WVWZZZ3CZKE01234I", "WVWZZZ3CZKE01234Q", "WVWZZZ3CZKE0123-5"} {
		var ve *ValidationError
		if _, err := s.CompleteVIN(f.ctx, f.panel(f.org), res.UUID, bad); !errors.As(err, &ve) || ve.Field != "vin" {
			t.Fatalf("%q = %v", bad, err)
		}
	}
	if row := f.result(res.UUID); row.Status != StatusVINPending || row.Vin.Valid {
		t.Fatalf("after invalid = %+v", row)
	}

	other := f.newOrg("b")
	if _, err := s.CompleteVIN(f.ctx, f.panel(other), res.UUID, "WVWZZZ3CZKE012345"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign = %v", err)
	}

	out, err := s.CompleteVIN(f.ctx, f.panel(f.org), res.UUID, " wvwzzz3czke012345 ")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != StatusAccepted || out.VIN == nil || *out.VIN != "WVWZZZ3CZKE012345" || len(out.Values) != 15 {
		t.Fatalf("completed = %+v", out.MeasurementSummary)
	}
	if err := EnsureLinkable(out.Status); err != nil {
		t.Fatalf("accepted linkable = %v", err)
	}
	if _, err := s.CompleteVIN(f.ctx, f.panel(f.org), res.UUID, "WVWZZZ3CZKE012345"); err != nil {
		t.Fatalf("same vin again = %v", err)
	}
	if _, err := s.CompleteVIN(f.ctx, f.panel(f.org), res.UUID, "WVWZZZ3CZKE099999"); !errors.Is(err, ErrVINAlreadySet) {
		t.Fatalf("other vin = %v", err)
	}
}

// CompleteMeasurementResultVIN completes the Store of the in-memory fake
// (usecase_test.go); its panel lookup never finds a row.
func (f *fakeStore) CompleteMeasurementResultVIN(context.Context, db.CompleteMeasurementResultVINParams) (int64, error) {
	return 0, nil
}
