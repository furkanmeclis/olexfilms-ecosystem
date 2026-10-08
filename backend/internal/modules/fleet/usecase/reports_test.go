package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/mail"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// fakeRenderer is the fake PDF renderer (Gotenberg): it renders through
// the real fleet loader, so the variables of every call are kept.
type fakeRenderer struct {
	loader ReportLoader
	err    error
	calls  int
	vars   []map[string]string
}

func (r *fakeRenderer) RenderSource(ctx context.Context, kind, sourceID, locale string) ([]byte, error) {
	r.calls++
	if kind != docmodel.KindFleetReport {
		return nil, fmt.Errorf("kind %s", kind)
	}
	if r.err != nil {
		return nil, r.err
	}
	src, err := r.loader.Load(ctx, docmodel.Viewer{System: true}, sourceID, locale)
	if err != nil {
		return nil, err
	}
	r.vars = append(r.vars, src.Vars)
	return []byte("%PDF-1.7 fleet " + locale), nil
}

// fakeReportQueue records the enqueued report ids.
type fakeReportQueue struct{ ids []int64 }

func (q *fakeReportQueue) EnqueueFleetReport(_ context.Context, id int64) error {
	q.ids = append(q.ids, id)
	return nil
}

func (q *fakeReportQueue) count(id int64) int {
	n := 0
	for _, x := range q.ids {
		if x == id {
			n++
		}
	}
	return n
}

// fakeMail is the fake mailer.
type fakeMail struct {
	msgs []mail.Message
	err  error
}

func (m *fakeMail) Send(_ context.Context, msg mail.Message) error {
	if m.err != nil {
		return m.err
	}
	m.msgs = append(m.msgs, msg)
	return nil
}

type reportFixture struct {
	*apiFixture
	pf       testFleet
	vehicle  VehicleView
	s1, s2   db.Service
	mods     *fakeModules
	render   *fakeRenderer
	queue    *fakeReportQueue
	mail     *fakeMail
	store    *storage.Memory
	period   ReportPeriod
	reportAt time.Time
}

// newReportFixture opens a fleet at d1 linked to d2, with a vehicle, a
// completed service of each dealer this month (d1's with parts, a product
// and a warranty), their income on the fleet cari, a billing address and
// the clock set to the next month's first day 08:00 (Istanbul): the due
// period is the current month.
func newReportFixture(t *testing.T) *reportFixture {
	t.Helper()
	f := &reportFixture{apiFixture: newAPIFixture(t)}
	f.mods = &fakeModules{off: map[int64]bool{}}
	f.svc.SetModules(f.mods)
	loc := location("Europe/Istanbul")
	f.period = periodContaining(model.ReportMonthly, localDate(time.Now(), loc))
	f.pf = f.openPortalFleet(t, f.d1, "r")
	f.linkSecond(t, f.d2, f.pf)
	f.vehicle = f.fleetVehicle(t, f.d1, f.pf, 7, true)
	f.s1 = f.partsService(t, f.d1, f.vehicle.UUID)
	f.s2 = f.completedService(t, f.d2, f.vehicle.UUID)
	f.income(t, f.d1, f.pf.org.ID, f.s1, "150.00")
	f.income(t, f.d2, f.pf.org.ID, f.s2, "275.00")
	if _, err := f.tx.Exec(f.ctx, `UPDATE fleet_profiles SET billing_email = $2 WHERE organization_id = $1`,
		f.pf.org.ID, "Billing-"+f.suffix+"@example.test"); err != nil {
		t.Fatal(err)
	}
	next := f.period.End.AddDate(0, 0, 1)
	f.reportAt = time.Date(next.Year(), next.Month(), next.Day(), 8, 0, 0, 0, loc)
	f.svc.now = func() time.Time { return f.reportAt }
	f.render = &fakeRenderer{loader: f.svc.ReportDocumentLoader()}
	f.queue = &fakeReportQueue{}
	f.mail = &fakeMail{}
	f.store = storage.NewMemory()
	f.svc.SetReports(ReportConfig{
		Renderer: f.render, Storage: f.store, Queue: f.queue, Mail: f.mail,
		PortalURL: "https://app.test/portal/fleet/reports", Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return f
}

// partsService is a completed service of org on the vehicle with one item
// (hood + roof applied) and a warranty ending in 30 days.
func (f *reportFixture) partsService(t *testing.T, org db.Organization, vehicleUUID uuid.UUID) db.Service {
	t.Helper()
	ctx := f.ctx
	v, err := f.q.GetVehicleByUUID(ctx, vehicleUUID)
	if err != nil {
		t.Fatal(err)
	}
	center, err := f.q.GetBrandCenter(ctx, f.brand.ID)
	if err != nil {
		t.Fatal(err)
	}
	seq := time.Now().UnixNano()
	cat, err := f.q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: center.ID, BrandID: f.brand.ID, Name: fmt.Sprintf("t476-cat-%d", seq),
		AvailableParts: []byte(`["body_kaput","body_tavan"]`), Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.q.CreateProduct(ctx, db.CreateProductParams{
		OrganizationID: center.ID, BrandID: f.brand.ID, CategoryID: cat.ID, Sku: fmt.Sprintf("t476-%d", seq),
		Name: "T476 PPF " + f.suffix, Images: []byte("[]"), UnitType: "piece", Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	unit, err := f.q.CreateUnit(ctx, db.CreateUnitParams{
		OrganizationID: center.ID, BrandID: f.brand.ID, ProductID: p.ID, Barcode: fmt.Sprintf("T476-%d", seq),
		UnitKind: "serial", Source: "generated", Status: "available",
	})
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := f.tx.QueryRow(ctx, `INSERT INTO services (service_no, organization_id, brand_id, customer_user_id,
		vehicle_id, car_brand_id, car_model_id, plate, plate_country, status)
		VALUES ('T476' || right(gen_random_uuid()::text, 8), $1, $2, $3, $4, $5, $6, $7, 'TR', 'pending')
		RETURNING id`, org.ID, org.BrandID, v.UserID, v.ID, f.carBrand.ID, f.carModel.ID, v.Plate.String).Scan(&id); err != nil {
		t.Fatalf("service: %v", err)
	}
	item, err := f.q.CreateServiceItem(ctx, db.CreateServiceItemParams{
		ServiceID: id, ProductID: p.ID, UnitID: unit.ID, Kind: "full", AppliedParts: []byte(`["body_kaput","body_tavan"]`),
	})
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	svc, err := f.q.CompleteService(ctx, db.CompleteServiceParams{ID: id})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	now := time.Now()
	if _, err := f.q.CreateWarrantyForServiceItem(ctx, db.CreateWarrantyForServiceItemParams{
		ServiceItemID: item.ID, HolderUserID: v.UserID,
		StartAt: pgtype.Timestamptz{Time: now, Valid: true}, EndAt: pgtype.Timestamptz{Time: f.period.End.AddDate(0, 0, 30), Valid: true},
	}); err != nil {
		t.Fatalf("warranty: %v", err)
	}
	return svc
}

// fleetReports lists the fleet's report rows.
func (f *reportFixture) fleetReports(t *testing.T) []db.FleetReport {
	t.Helper()
	rows, err := f.q.ListFleetReports(f.ctx, db.ListFleetReportsParams{FleetOrgID: f.pf.org.ID, LimitCount: 50})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func (f *reportFixture) setProfile(t *testing.T, set string, args ...any) {
	t.Helper()
	if _, err := f.tx.Exec(f.ctx, `UPDATE fleet_profiles SET `+set+` WHERE organization_id = $1`, append([]any{f.pf.org.ID}, args...)...); err != nil {
		t.Fatal(err)
	}
}

// Acceptance (TEC-476): two schedule ticks for the same period create one
// report (one generation task); before 07:00 on the first day nothing is
// due yet.
func TestFleetReportTwoTicksOneReport(t *testing.T) {
	f := newReportFixture(t)
	loc := location("Europe/Istanbul")
	next := f.period.End.AddDate(0, 0, 1)
	f.svc.now = func() time.Time { return time.Date(next.Year(), next.Month(), next.Day(), 6, 59, 0, 0, loc) }
	if err := f.svc.ScheduleReportsTask(f.ctx); err != nil {
		t.Fatal(err)
	}
	if rows := f.fleetReports(t); len(rows) != 0 {
		t.Fatalf("before 07:00: %d reports", len(rows))
	}

	f.svc.now = func() time.Time { return f.reportAt }
	for i := 0; i < 2; i++ {
		if err := f.svc.ScheduleReportsTask(f.ctx); err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
	}
	rows := f.fleetReports(t)
	if len(rows) != 1 {
		t.Fatalf("reports = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.PeriodKind != model.ReportMonthly || !r.PeriodStart.Time.Equal(f.period.Start) || !r.PeriodEnd.Time.Equal(f.period.End) ||
		r.Status != model.ReportPending || r.Locale != "tr" {
		t.Fatalf("report = %+v", r)
	}
	if n := f.queue.count(r.ID); n != 1 {
		t.Fatalf("enqueued %d times, want 1", n)
	}
	// A pending report whose task was lost (older than 15 minutes) is
	// enqueued again; the row stays single.
	if _, err := f.tx.Exec(f.ctx, `UPDATE fleet_reports SET created_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.ScheduleReportsTask(f.ctx); err != nil {
		t.Fatal(err)
	}
	if n := f.queue.count(r.ID); n != 2 || len(f.fleetReports(t)) != 1 {
		t.Fatalf("stale pending: enqueued %d", n)
	}
	// A later tick in the same month keeps the single report.
	f.svc.now = func() time.Time { return f.reportAt.Add(72 * time.Hour) }
	if err := f.svc.ScheduleReportsTask(f.ctx); err != nil {
		t.Fatal(err)
	}
	if rows := f.fleetReports(t); len(rows) != 1 {
		t.Fatalf("later tick: %d reports", len(rows))
	}

	// Quarterly: the last closed quarter once the quarter has begun.
	f.setProfile(t, "report_frequency = 'quarterly'")
	q := periodContaining(model.ReportQuarterly, f.period.Start)
	qNext := q.End.AddDate(0, 0, 1)
	f.svc.now = func() time.Time { return time.Date(qNext.Year(), qNext.Month(), qNext.Day(), 7, 30, 0, 0, loc) }
	for i := 0; i < 2; i++ {
		if err := f.svc.ScheduleReportsTask(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	quarterly := 0
	for _, r := range f.fleetReports(t) {
		if r.PeriodKind == model.ReportQuarterly {
			quarterly++
			if !r.PeriodStart.Time.Equal(q.Start) || !r.PeriodEnd.Time.Equal(q.End) {
				t.Fatalf("quarter = %+v", r)
			}
		}
	}
	if quarterly != 1 {
		t.Fatalf("quarterly reports = %d", quarterly)
	}
}

// Acceptance (TEC-476): a fleet with report_frequency off gets no report.
func TestFleetReportOffFleetGetsNone(t *testing.T) {
	f := newReportFixture(t)
	f.setProfile(t, "report_frequency = 'off'")
	if err := f.svc.ScheduleReportsTask(f.ctx); err != nil {
		t.Fatal(err)
	}
	if rows := f.fleetReports(t); len(rows) != 0 {
		t.Fatalf("off fleet: %d reports", len(rows))
	}
}

// Acceptance (TEC-476): a dealer with the fleet module off adds nothing to
// the report (services, parts, accounts); with no dealer left no report is
// produced.
func TestFleetReportExcludesModuleOffDealer(t *testing.T) {
	f := newReportFixture(t)
	if err := f.svc.ScheduleReportsTask(f.ctx); err != nil {
		t.Fatal(err)
	}
	r := f.fleetReports(t)[0]
	src, err := f.svc.ReportDocumentLoader().Load(f.ctx, docmodel.Viewer{System: true}, r.Uuid.String(), r.Locale)
	if err != nil {
		t.Fatal(err)
	}
	v := src.Vars
	if v["service_count"] != "2" || v["dealer_count"] != "2" || v["vehicle_count"] != "1" ||
		!strings.Contains(v["dealer_services_table"], f.d2.Name) || !strings.Contains(v["accounts_table"], "275.00 TRY") {
		t.Fatalf("both dealers on: %v", v)
	}
	if !strings.Contains(v["parts_table"], "Kaput") || !strings.Contains(v["parts_table"], "Tavan") ||
		!strings.Contains(v["products_table"], "T476 PPF "+f.suffix) || v["warranty_started_count"] != "1" ||
		v["warranty_active_count"] != "1" || !strings.Contains(v["upcoming_expirations_table"], *f.vehicle.Plate) ||
		!strings.Contains(v["accounts_table"], "150.00 TRY") {
		t.Fatalf("content: %v", v)
	}
	if src.OrganizationID != f.pf.org.ID {
		t.Fatalf("source org = %d", src.OrganizationID)
	}
	// Another organization's viewer does not load it.
	if _, err := f.svc.ReportDocumentLoader().Load(f.ctx, docmodel.Viewer{OrganizationID: f.d1.ID}, r.Uuid.String(), "tr"); !errors.Is(err, docmodel.ErrSourceNotFound) {
		t.Fatalf("dealer viewer: %v", err)
	}

	f.mods.off[f.d2.ID] = true
	if err := f.svc.GenerateReport(f.ctx, r.ID, false); err != nil {
		t.Fatal(err)
	}
	v = f.render.vars[len(f.render.vars)-1]
	if v["service_count"] != "1" || v["dealer_count"] != "1" || strings.Contains(v["dealer_services_table"], f.d2.Name) ||
		strings.Contains(v["accounts_table"], f.d2.Name) || strings.Contains(v["accounts_table"], "275.00") {
		t.Fatalf("dealer 2 off: %v", v)
	}

	// No dealer with the module: the next period gets no report, and a
	// pending one fails without a render.
	f.mods.off[f.d1.ID] = true
	f.svc.now = func() time.Time { return f.reportAt.AddDate(0, 1, 0) }
	if err := f.svc.ScheduleReportsTask(f.ctx); err != nil {
		t.Fatal(err)
	}
	if rows := f.fleetReports(t); len(rows) != 1 {
		t.Fatalf("all off: %d reports", len(rows))
	}
	pending, err := f.q.UpsertFleetReport(f.ctx, db.UpsertFleetReportParams{
		FleetOrgID: f.pf.org.ID, BrandID: f.brand.ID, PeriodKind: model.ReportQuarterly,
		PeriodStart: pgDate(f.period.Start), PeriodEnd: pgDate(f.period.End), Locale: "tr",
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := f.render.calls
	if err := f.svc.GenerateReport(f.ctx, pending.ID, false); err != nil {
		t.Fatal(err)
	}
	got, _ := f.q.GetFleetReportByID(f.ctx, pending.ID)
	if got.Status != model.ReportFailed || f.render.calls != calls {
		t.Fatalf("all off: status %s, renders %d", got.Status, f.render.calls-calls)
	}
}

// Acceptance (TEC-476): a PDF error leaves the report pending for the
// retries (the task returns the error) and marks it failed on the last of
// the three retries; nothing is stored or e-mailed.
func TestFleetReportPDFFailureRetriesThenFailed(t *testing.T) {
	f := newReportFixture(t)
	if err := f.svc.ScheduleReportsTask(f.ctx); err != nil {
		t.Fatal(err)
	}
	r := f.fleetReports(t)[0]
	f.render.err = errors.New("gotenberg: 503")
	for i := 0; i < 3; i++ {
		if err := f.svc.GenerateReport(f.ctx, r.ID, false); err == nil {
			t.Fatalf("attempt %d: want the PDF error for a retry", i)
		}
		if got, _ := f.q.GetFleetReportByID(f.ctx, r.ID); got.Status != model.ReportPending {
			t.Fatalf("attempt %d: status %s", i, got.Status)
		}
	}
	if err := f.svc.GenerateReport(f.ctx, r.ID, true); err == nil {
		t.Fatal("final attempt: want the error")
	}
	got, _ := f.q.GetFleetReportByID(f.ctx, r.ID)
	if got.Status != model.ReportFailed || !strings.Contains(got.Error.String, "gotenberg: 503") || got.StorageKey.Valid {
		t.Fatalf("after final attempt: %+v", got)
	}
	if f.render.calls != 4 || len(f.mail.msgs) != 0 {
		t.Fatalf("renders %d, mails %d", f.render.calls, len(f.mail.msgs))
	}
	// A failed report is not retried by a later task, nor rescheduled.
	if err := f.svc.GenerateReport(f.ctx, r.ID, false); err != nil || f.render.calls != 4 {
		t.Fatalf("failed report rerun: %v, renders %d", err, f.render.calls)
	}
	if err := f.svc.ScheduleReportsTask(f.ctx); err != nil || len(f.fleetReports(t)) != 1 {
		t.Fatalf("reschedule: %v", err)
	}
}

// Acceptance (TEC-476): the report is stored under
// fleet-reports/{fleet}/{period}.pdf, marked ready and e-mailed once to the
// fleet users and the billing address in the report language (de here, ar
// right to left), with the PDF attached.
func TestFleetReportMailInReportLanguage(t *testing.T) {
	f := newReportFixture(t)
	f.setProfile(t, "report_locale = 'de'")
	if err := f.svc.ScheduleReportsTask(f.ctx); err != nil {
		t.Fatal(err)
	}
	r := f.fleetReports(t)[0]
	if r.Locale != "de" {
		t.Fatalf("locale = %s", r.Locale)
	}
	if err := f.svc.GenerateReport(f.ctx, r.ID, false); err != nil {
		t.Fatal(err)
	}
	got, _ := f.q.GetFleetReportByID(f.ctx, r.ID)
	key := "fleet-reports/" + f.pf.uuid.String() + "/" + f.period.Start.Format("2006-01") + ".pdf"
	if got.Status != model.ReportReady || got.StorageKey.String != key || !got.EmailedAt.Valid {
		t.Fatalf("report = %+v", got)
	}
	if ok, _ := f.store.Exists(f.ctx, key); !ok {
		t.Fatalf("pdf not stored at %s", key)
	}
	if len(f.mail.msgs) != 1 {
		t.Fatalf("mails = %d", len(f.mail.msgs))
	}
	m := f.mail.msgs[0]
	month := reportTexts[i18n.LocaleDE].Months[f.period.Start.Month()-1]
	if !strings.Contains(m.Subject, "Flottenbericht") || !strings.Contains(m.Subject, month) ||
		!strings.Contains(m.Body, "im Anhang") || !strings.Contains(m.HTMLBody, `lang="de"`) {
		t.Fatalf("mail not in de: %q / %q", m.Subject, m.Body)
	}
	want := map[string]bool{f.pf.user.Email.String: true, "Billing-" + f.suffix + "@example.test": true}
	if len(m.To) != 2 || !want[m.To[0]] || !want[m.To[1]] {
		t.Fatalf("to = %v", m.To)
	}
	if len(m.Attachments) != 1 || m.Attachments[0].ContentType != "application/pdf" ||
		string(m.Attachments[0].Data) != "%PDF-1.7 fleet de" || m.Attachments[0].Filename != "fleet-report-"+f.period.Key()+".pdf" {
		t.Fatalf("attachments = %+v", m.Attachments)
	}
	// Idempotent: a second task sends nothing.
	if err := f.svc.GenerateReport(f.ctx, r.ID, false); err != nil || len(f.mail.msgs) != 1 || f.render.calls != 1 {
		t.Fatalf("rerun: %v, mails %d, renders %d", err, len(f.mail.msgs), f.render.calls)
	}

	// The portal downloads the ready report.
	f.svc.SetReportFiles(f.store)
	rc, name, err := f.svc.PortalReportFile(f.ctx, f.caller(f.pf.user), r.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	_ = rc.Close()
	if !strings.HasSuffix(name, ".pdf") {
		t.Fatalf("name = %s", name)
	}

	// ar: right to left.
	f.setProfile(t, "report_locale = 'ar'")
	ar, err := f.q.UpsertFleetReport(f.ctx, db.UpsertFleetReportParams{
		FleetOrgID: f.pf.org.ID, BrandID: f.brand.ID, PeriodKind: model.ReportQuarterly,
		PeriodStart: pgDate(f.period.Start), PeriodEnd: pgDate(f.period.End), Locale: "ar",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.GenerateReport(f.ctx, ar.ID, false); err != nil {
		t.Fatal(err)
	}
	m = f.mail.msgs[len(f.mail.msgs)-1]
	if !strings.Contains(m.Subject, "تقرير الأسطول") || !strings.Contains(m.HTMLBody, `dir="rtl"`) {
		t.Fatalf("ar mail: %q", m.Subject)
	}
}

// A mail error keeps the report ready for the retry, which sends it from
// storage without rendering again.
func TestFleetReportMailRetryReadsStoredPDF(t *testing.T) {
	f := newReportFixture(t)
	if err := f.svc.ScheduleReportsTask(f.ctx); err != nil {
		t.Fatal(err)
	}
	r := f.fleetReports(t)[0]
	f.mail.err = errors.New("smtp down")
	if err := f.svc.GenerateReport(f.ctx, r.ID, false); err == nil {
		t.Fatal("want the mail error")
	}
	if got, _ := f.q.GetFleetReportByID(f.ctx, r.ID); got.Status != model.ReportReady || got.EmailedAt.Valid {
		t.Fatalf("after mail error: %+v", got)
	}
	f.mail.err = nil
	if err := f.svc.GenerateReport(f.ctx, r.ID, false); err != nil {
		t.Fatal(err)
	}
	if f.render.calls != 1 || len(f.mail.msgs) != 1 || string(f.mail.msgs[0].Attachments[0].Data) != "%PDF-1.7 fleet tr" {
		t.Fatalf("renders %d, mails %d", f.render.calls, len(f.mail.msgs))
	}
}

// A PDF over 10 MB is not attached: the e-mail links the portal.
func TestFleetReportLargePDFLinksPortal(t *testing.T) {
	f := newReportFixture(t)
	r := db.FleetReport{Uuid: uuid.New(), PeriodKind: model.ReportMonthly, PeriodStart: pgDate(f.period.Start),
		PeriodEnd: pgDate(f.period.End), Locale: "en", BrandID: f.brand.ID}
	big := make([]byte, MaxReportAttachment+1)
	m, err := f.svc.reportMessage(f.ctx, f.pf.org, r, big)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Attachments) != 0 || !strings.Contains(m.Body, "https://app.test/portal/fleet/reports") {
		t.Fatalf("large pdf: %d attachments, body %q", len(m.Attachments), m.Body)
	}
	m, err = f.svc.reportMessage(f.ctx, f.pf.org, r, []byte("%PDF"))
	if err != nil || len(m.Attachments) != 1 || strings.Contains(m.Body, "portal") {
		t.Fatalf("small pdf: %+v, %v", m, err)
	}
}

// POST /v1/fleets/{uuid}/reports: a linked dealer requests a closed period;
// an open period is 400; a failed report runs again; a ready one is kept.
func TestRequestReport(t *testing.T) {
	f := newReportFixture(t)
	c := f.dealer(f.d1)
	open := f.reportAt.Format("2006-01")
	var ve *ValidationError
	if _, err := f.svc.RequestReport(f.ctx, c, f.pf.uuid, RequestReportInput{PeriodKind: "monthly", Period: open}); !errors.As(err, &ve) || ve.Field != "period" {
		t.Fatalf("open period: %v", err)
	}
	if _, err := f.svc.RequestReport(f.ctx, c, f.pf.uuid, RequestReportInput{PeriodKind: "weekly", Period: "2026-09"}); !errors.As(err, &ve) || ve.Field != "period_kind" {
		t.Fatalf("bad kind: %v", err)
	}
	if _, err := f.svc.RequestReport(f.ctx, c, f.pf.uuid, RequestReportInput{PeriodKind: "quarterly", Period: "2026-Q5"}); !errors.As(err, &ve) {
		t.Fatalf("bad quarter: %v", err)
	}
	if _, err := f.svc.RequestReport(f.ctx, c, uuid.New(), RequestReportInput{PeriodKind: "monthly", Period: f.period.Key()}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown fleet: %v", err)
	}
	v, err := f.svc.RequestReport(f.ctx, c, f.pf.uuid, RequestReportInput{PeriodKind: "monthly", Period: f.period.Key()})
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != model.ReportPending || v.PeriodStart != f.period.Start.Format(time.DateOnly) || len(f.queue.ids) != 1 {
		t.Fatalf("request: %+v, queued %v", v, f.queue.ids)
	}
	r, err := f.q.GetFleetReportByUUID(f.ctx, v.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.MarkFleetReportFailed(f.ctx, db.MarkFleetReportFailedParams{ID: r.ID, Error: pgtype.Text{String: "x", Valid: true}}); err != nil {
		t.Fatal(err)
	}
	again, err := f.svc.RequestReport(f.ctx, c, f.pf.uuid, RequestReportInput{PeriodKind: "monthly", Period: f.period.Key()})
	if err != nil || again.UUID != v.UUID || again.Status != model.ReportPending || len(f.queue.ids) != 2 {
		t.Fatalf("failed rerun: %+v, %v", again, err)
	}
	if err := f.svc.GenerateReport(f.ctx, r.ID, false); err != nil {
		t.Fatal(err)
	}
	ready, err := f.svc.RequestReport(f.ctx, c, f.pf.uuid, RequestReportInput{PeriodKind: "monthly", Period: f.period.Key()})
	if err != nil || ready.Status != model.ReportReady || len(f.queue.ids) != 2 {
		t.Fatalf("ready: %+v, %v", ready, err)
	}
}

// Periods: the previous month / quarter once the first day's 07:00 has
// passed in the fleet's timezone.
func TestDueReportPeriod(t *testing.T) {
	ist := location("Europe/Istanbul")
	cases := []struct {
		kind string
		now  time.Time
		ok   bool
		key  string
	}{
		{model.ReportMonthly, time.Date(2026, 10, 1, 6, 59, 0, 0, ist), false, ""},
		{model.ReportMonthly, time.Date(2026, 10, 1, 7, 0, 0, 0, ist), true, "2026-09"},
		{model.ReportMonthly, time.Date(2026, 1, 15, 12, 0, 0, 0, ist), true, "2025-12"},
		{model.ReportQuarterly, time.Date(2026, 10, 1, 7, 0, 0, 0, ist), true, "2026-Q3"},
		{model.ReportQuarterly, time.Date(2026, 11, 20, 7, 0, 0, 0, ist), true, "2026-Q3"},
		{model.ReportQuarterly, time.Date(2026, 1, 1, 6, 0, 0, 0, ist), false, ""},
		// 04:30 UTC is 07:30 in Istanbul: due there.
		{model.ReportMonthly, time.Date(2026, 10, 1, 4, 30, 0, 0, time.UTC), true, "2026-09"},
	}
	for _, c := range cases {
		p, ok := dueReportPeriod(c.kind, c.now, ist)
		if ok != c.ok || (ok && p.Key() != c.key) {
			t.Errorf("%s %v: %v %s, want %v %s", c.kind, c.now, ok, p.Key(), c.ok, c.key)
		}
	}
	q, err := ParseReportPeriod(model.ReportQuarterly, "2026-q4")
	if err != nil || q.Start != time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC) || q.End != time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("quarter = %+v, %v", q, err)
	}
	m, err := ParseReportPeriod(model.ReportMonthly, "2024-02")
	if err != nil || m.End.Day() != 29 {
		t.Fatalf("month = %+v, %v", m, err)
	}
}

// Acceptance (TEC-476): the report labels and e-mail texts exist in all 13
// languages with no empty key; ar is Arabic script.
func TestReportLabelsEveryLocale(t *testing.T) {
	if len(i18n.Supported) != 13 {
		t.Fatalf("locales = %d", len(i18n.Supported))
	}
	for _, l := range i18n.Supported {
		t.Run(string(l), func(t *testing.T) {
			lab, ok := reportTexts[l]
			if !ok {
				t.Fatal("missing")
			}
			v := reflect.ValueOf(lab)
			for i := 0; i < v.NumField(); i++ {
				fv := v.Field(i)
				if fv.Kind() == reflect.Array {
					for j := 0; j < fv.Len(); j++ {
						if strings.TrimSpace(fv.Index(j).String()) == "" {
							t.Errorf("%s[%d] empty", v.Type().Field(i).Name, j)
						}
					}
					continue
				}
				if strings.TrimSpace(fv.String()) == "" {
					t.Errorf("%s empty", v.Type().Field(i).Name)
				}
			}
			for _, k := range []string{"{fleet}", "{period}"} {
				if !strings.Contains(lab.MailSubject, k) && !strings.Contains(lab.MailBody, k) {
					t.Errorf("mail misses %s", k)
				}
			}
			if !strings.Contains(lab.MailLink, "{url}") || !strings.Contains(lab.Quarter, "{q}") || !strings.Contains(lab.Quarter, "{year}") {
				t.Error("placeholders missing")
			}
			if l != i18n.LocaleEN && lab.MailBody == reportTexts[i18n.LocaleEN].MailBody {
				t.Error("mail body not translated")
			}
		})
	}
	if !strings.ContainsFunc(reportTexts[i18n.LocaleAR].MailSubject, func(r rune) bool { return r >= 0x0600 && r <= 0x06FF }) {
		t.Fatal("ar labels are not Arabic")
	}
	if got, l := reportLabelsFor("zh-CN"); l != i18n.LocaleZhCN || got.Dealer != "经销商" {
		t.Fatalf("zh-CN resolves to %s", l)
	}
}
