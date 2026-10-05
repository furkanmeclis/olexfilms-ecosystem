package usecase

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/documentstest"
	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const pdfTestVIN = "WVWZZZ3CZKE298001"

type pdfFixture struct {
	*dbFixture
	got   *documentstest.Gotenberg
	store *storage.Memory
}

func newPDFFixture(t *testing.T) *pdfFixture {
	f := newDBFixture(t)
	// The organization's local time is UTC+3 and its documents Turkish.
	if _, err := f.tx.Exec(f.ctx, `UPDATE organizations SET timezone = 'Europe/Istanbul', locale = 'tr' WHERE id = $1`, f.org.ID); err != nil {
		t.Fatal(err)
	}
	return &pdfFixture{dbFixture: f, got: documentstest.NewGotenberg(t, 0), store: storage.NewMemory()}
}

func (f *pdfFixture) pdfService(enq PDFEnqueuer) *PDFService {
	p := NewPDF(f.tx, f.q, f.store, pdfrender.New(f.got.URL), enq, pdfrender.FontsSystem, nil)
	p.SetClock(func() time.Time { return time.Date(2026, 10, 5, 9, 30, 0, 0, time.UTC) })
	return p
}

// upload stores the NexPTG mobile fixture (normalized on insert), with the
// VIN when vin is not empty.
func (f *pdfFixture) upload(vin string) db.MeasurementResult {
	f.t.Helper()
	var body map[string]any
	if err := json.Unmarshal(fixture(f.t, "nexptg_mobile.json"), &body); err != nil {
		f.t.Fatal(err)
	}
	body["client_measurement_id"] = "tec298-" + uuid.NewString()
	if vin != "" {
		body["vin"] = vin
	}
	raw, _ := json.Marshal(body)
	res, err := f.svc().Create(f.ctx, f.caller(f.org), Input{Body: raw})
	if err != nil {
		f.t.Fatal(err)
	}
	return f.result(res.UUID)
}

func readAll(t *testing.T, rc io.ReadCloser) string {
	t.Helper()
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TEC-298 acceptance: the first request renders the PDF (fake Gotenberg),
// stores it and sets pdf_key; the second one is served from pdf_key without
// another render.
func TestDBMeasurementPDFRenderedOnceFromPDFKey(t *testing.T) {
	f := newPDFFixture(t)
	row := f.upload(pdfTestVIN)
	p := f.pdfService(nil)

	first, err := p.RequestPDF(f.ctx, f.panel(f.org), row.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	if first.Pending || readAll(t, first.Body) != documentstest.PDF || first.Filename != "measurement-"+pdfTestVIN+".pdf" {
		t.Fatalf("first = %+v", first)
	}
	if n := f.got.Calls.Load(); n != 1 {
		t.Fatalf("gotenberg calls = %d, want 1", n)
	}
	stored := f.result(row.Uuid)
	wantKey := storage.MeasurementPDFObjectKey(f.org.Uuid, row.Uuid)
	if !stored.PdfKey.Valid || stored.PdfKey.String != wantKey {
		t.Fatalf("pdf_key = %+v, want %s", stored.PdfKey, wantKey)
	}
	if ok, err := f.store.Exists(f.ctx, wantKey); err != nil || !ok {
		t.Fatalf("stored object: %v %v", ok, err)
	}

	second, err := p.RequestPDF(f.ctx, f.panel(f.org), row.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	if second.Pending || readAll(t, second.Body) != documentstest.PDF {
		t.Fatalf("second = %+v", second)
	}
	if n := f.got.Calls.Load(); n != 1 {
		t.Fatalf("second request rendered again: gotenberg calls = %d", n)
	}
	// The worker task is idempotent too.
	if err := p.GeneratePDF(f.ctx, row.ID); err != nil || f.got.Calls.Load() != 1 {
		t.Fatalf("repeated task: %v calls=%d", err, f.got.Calls.Load())
	}
}

type fakeEnqueuer struct{ tasks []*asynq.Task }

func (e *fakeEnqueuer) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	for _, t := range e.tasks {
		if string(t.Payload()) == string(task.Payload()) {
			return nil, asynq.ErrTaskIDConflict
		}
	}
	e.tasks = append(e.tasks, task)
	return &asynq.TaskInfo{}, nil
}

// With a queue the first request enqueues measurement:pdf (worker-docs)
// and answers pending; after the task ran the PDF is streamed.
func TestDBMeasurementPDFQueuedOnWorkerDocs(t *testing.T) {
	f := newPDFFixture(t)
	row := f.upload(pdfTestVIN)
	enq := &fakeEnqueuer{}
	p := f.pdfService(enq)

	for range 2 {
		out, err := p.RequestPDF(f.ctx, f.panel(f.org), row.Uuid)
		if err != nil || !out.Pending || out.UUID != row.Uuid {
			t.Fatalf("pending = %+v, %v", out, err)
		}
	}
	if len(enq.tasks) != 1 || enq.tasks[0].Type() != queue.TaskMeasurementPDF {
		t.Fatalf("tasks = %d", len(enq.tasks))
	}
	payload, err := queue.ParseMeasurementPDFPayload(enq.tasks[0].Payload())
	if err != nil || payload.ResultID != row.ID {
		t.Fatalf("payload = %+v %v", payload, err)
	}
	if f.got.Calls.Load() != 0 {
		t.Fatal("the request rendered inline")
	}
	if err := p.GeneratePDF(f.ctx, payload.ResultID); err != nil {
		t.Fatal(err)
	}
	out, err := p.RequestPDF(f.ctx, f.panel(f.org), row.Uuid)
	if err != nil || out.Pending || readAll(t, out.Body) != documentstest.PDF {
		t.Fatalf("ready = %+v, %v", out, err)
	}
	if len(enq.tasks) != 1 || f.got.Calls.Load() != 1 {
		t.Fatalf("tasks %d calls %d", len(enq.tasks), f.got.Calls.Load())
	}
}

// A vin_pending measurement has no PDF (422); another organization's
// measurement is not found.
func TestDBMeasurementPDFRules(t *testing.T) {
	f := newPDFFixture(t)
	p := f.pdfService(nil)
	pending := f.upload("")
	if _, err := p.RequestPDF(f.ctx, f.panel(f.org), pending.Uuid); !errors.Is(err, ErrPDFVINPending) {
		t.Fatalf("vin_pending = %v", err)
	}
	row := f.upload(pdfTestVIN)
	other := f.newOrg("b")
	if _, err := p.RequestPDF(f.ctx, f.panel(other), row.Uuid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other org = %v", err)
	}
	if f.got.Calls.Load() != 0 {
		t.Fatal("a refused request rendered")
	}
}

// TEC-298 acceptance: the rendered HTML carries measured_at in the
// organization timezone (2023-12-15 08:39 UTC = 11:39 Europe/Istanbul), the
// VIN, the device serial, the generation stamp, the part map and the
// readings; the HTML sent to Gotenberg is the same document.
func TestDBMeasurementPDFHTML(t *testing.T) {
	f := newPDFFixture(t)
	row := f.upload(pdfTestVIN)
	p := f.pdfService(nil)
	doc, err := p.RenderHTML(f.ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"15.12.2023 11:39 (Europe/Istanbul)", // measured_at, org timezone
		"05.10.2026 12:30 (Europe/Istanbul)", // generation stamp
		pdfTestVIN,
		"18416 Professional",
		"Ölçüm Raporu", // the Turkish template
		"Parça haritası",
		`<img src="data:image/svg+xml;base64,`,
		"Kaput", // HOOD reading row
		`lang="tr"`,
	} {
		if !strings.Contains(doc.HTML, want) {
			t.Errorf("HTML misses %q", want)
		}
	}
	if strings.Contains(doc.HTML, "{{") {
		t.Error("HTML has an unfilled placeholder")
	}
	if doc.Vars["measured_at"] != "15.12.2023 11:39 (Europe/Istanbul)" || doc.Vars["vin"] != pdfTestVIN {
		t.Fatalf("vars = %v %v", doc.Vars["measured_at"], doc.Vars["vin"])
	}
	if _, err := p.RequestPDF(f.ctx, f.panel(f.org), row.Uuid); err != nil {
		t.Fatal(err)
	}
	sent := f.got.LastHTML()
	if !strings.Contains(sent, "15.12.2023 11:39 (Europe/Istanbul)") || !strings.Contains(sent, pdfTestVIN) {
		t.Fatal("Gotenberg did not get the measurement document")
	}
}

// The documents module loader serves the same values for the measurement
// kind and hides other organizations' measurements.
func TestDBMeasurementDocumentLoader(t *testing.T) {
	f := newPDFFixture(t)
	row := f.upload(pdfTestVIN)
	l := f.pdfService(nil).DocumentLoader()
	src, err := l.Load(f.ctx, docmodel.Viewer{OrganizationID: f.org.ID, BrandID: f.org.BrandID}, row.Uuid.String(), "tr")
	if err != nil {
		t.Fatal(err)
	}
	if src.OrganizationID != f.org.ID || src.Vars["vin"] != pdfTestVIN || src.Vars["measured_at"] != "15.12.2023 11:39 (Europe/Istanbul)" {
		t.Fatalf("source = %+v", src.Vars)
	}
	other := f.newOrg("c")
	if _, err := l.Load(f.ctx, docmodel.Viewer{OrganizationID: other.ID, BrandID: other.BrandID}, row.Uuid.String(), "tr"); !errors.Is(err, docmodel.ErrSourceNotFound) {
		t.Fatalf("other org = %v", err)
	}
}
