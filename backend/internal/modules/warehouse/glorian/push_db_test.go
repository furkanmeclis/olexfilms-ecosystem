package glorian_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian/fake"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
)

// TEC-270: the barcode push against the fake hub and the migrated
// database (CI sets TEST_DATABASE_URL), in the pull fixture's rolled-back
// transaction.

const pushRemoteProduct = "10"

// pushUnits are serial units of one product in a center bin of org.
type pushUnits struct {
	product db.Product
	loc     db.WarehouseLocation
	units   []db.Unit
}

// pushUnits creates n printed serial units of a new product of org's
// brand; linked products are synced from the fixture connection.
func (f *pullFixture) pushUnits(t *testing.T, org db.Organization, n int, linked bool) pushUnits {
	t.Helper()
	f.seq++
	tag := fmt.Sprintf("T270-%s-%d-%d", f.suffix, org.ID, f.seq)
	cat, err := f.q.CreateProductCategory(f.ctx, db.CreateProductCategoryParams{
		OrganizationID: org.ID, BrandID: org.BrandID, Name: f.name("T270 " + tag),
		AvailableParts: []byte("[]"), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	var s pushUnits
	s.product, err = f.q.CreateProduct(f.ctx, db.CreateProductParams{
		OrganizationID: org.ID, BrandID: org.BrandID, CategoryID: cat.ID,
		Sku: tag, Name: f.name("T270 Film " + tag), Images: []byte("[]"), UnitType: "piece", Active: true,
	})
	if err != nil {
		t.Fatalf("product: %v", err)
	}
	if linked {
		if _, err := f.tx.Exec(f.ctx, `UPDATE products SET connection_id = $1, external_id = $2 WHERE id = $3`,
			f.conn.ID, pushRemoteProduct, s.product.ID); err != nil {
			t.Fatalf("link product: %v", err)
		}
	}
	s.loc, err = f.q.CreateWarehouseLocation(f.ctx, db.CreateWarehouseLocationParams{
		OrganizationID: org.ID, Code: tag, Name: "T270 bin", Active: true,
	})
	if err != nil {
		t.Fatalf("location: %v", err)
	}
	for i := 1; i <= n; i++ {
		u, err := f.q.CreateUnit(f.ctx, db.CreateUnitParams{
			OrganizationID: org.ID, BrandID: org.BrandID, ProductID: s.product.ID,
			Barcode: fmt.Sprintf("GT270-%s-%d-%d-%d", f.suffix, org.ID, f.seq, i), UnitKind: ledger.KindSerial,
			Source: "generated", Status: string(ledger.StatusPrinted),
		})
		if err != nil {
			t.Fatalf("unit %d: %v", i, err)
		}
		s.units = append(s.units, u)
	}
	return s
}

// post writes a movement through the ledger and returns it with the
// stock.* event the ledger wrote to the outbox.
func (f *pullFixture) post(t *testing.T, m ledger.Movement) (db.StockMovement, events.Event) {
	t.Helper()
	out := &captureOutbox{}
	m.Source, m.RefType, m.RefID = "test", "t270", time.Now().UnixNano()
	res, err := ledger.New(f.q, out).Post(f.ctx, f.tx, m)
	if err != nil {
		t.Fatalf("post %s: %v", m.Type, err)
	}
	if len(out.evs) != 1 {
		t.Fatalf("post %s: %d outbox events", m.Type, len(out.evs))
	}
	return res.Movement, out.evs[0]
}

// captureOutbox keeps the events the ledger writes.
type captureOutbox struct{ evs []events.Event }

func (c *captureOutbox) Enqueue(_ context.Context, _ pgx.Tx, ev events.Event) error {
	c.evs = append(c.evs, ev)
	return nil
}

// enter posts an entry of every unit into the bin.
func (f *pullFixture) enter(t *testing.T, s pushUnits, org db.Organization) []events.Event {
	t.Helper()
	var out []events.Event
	for _, u := range s.units {
		_, ev := f.post(t, ledger.Movement{
			Type: ledger.TypeEntry, UnitID: u.ID,
			To: &ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: s.loc.ID, OrgID: org.ID},
		})
		out = append(out, ev)
	}
	return out
}

func (f *pullFixture) pusher() *glorian.Pusher {
	factory := glorian.HTTPClientFactory(glorian.Options{HTTPClient: f.srv.Client(), RetryBaseDelay: time.Millisecond})
	return glorian.NewPusher(f.q, f.box, factory, nil)
}

type bulkBody struct {
	Items []glorian.BarcodeUpsert `json:"items"`
}

func pushRunCounts(t *testing.T, run db.IntegrationSyncRun) glorian.PushCounts {
	t.Helper()
	var c glorian.PushCounts
	if err := json.Unmarshal(run.Counts, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// recordingQueue is an Enqueuer that keeps the tasks and refuses a second
// task with the same id, like Asynq.
type recordingQueue struct {
	tasks []*asynq.Task
	ids   map[string]bool
}

func (r *recordingQueue) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	if r.ids == nil {
		r.ids = map[string]bool{}
	}
	for _, o := range opts {
		if o.Type() == asynq.TaskIDOpt {
			id := o.Value().(string)
			if r.ids[id] {
				return nil, asynq.ErrTaskIDConflict
			}
			r.ids[id] = true
		}
	}
	r.tasks = append(r.tasks, task)
	return &asynq.TaskInfo{}, nil
}

// dispatch feeds events through the push listener and returns the queue.
func (f *pullFixture) dispatch(t *testing.T, evs ...events.Event) *recordingQueue {
	t.Helper()
	q := &recordingQueue{}
	bus := events.NewBus(nil)
	// A fixed clock keeps every event in one debounce window.
	now := time.Now()
	glorian.NewListener(f.q, q, nil).WithClock(func() time.Time { return now }).Register(bus)
	for _, ev := range evs {
		if err := bus.Publish(f.ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	return q
}

// Three barcodes placed together go out in one bulk call with the right
// body; pushing again is safe (the hub upserts by barcode).
func TestPushThreeBarcodesOneBulkCall(t *testing.T) {
	f := newPullFixture(t, true)
	s := f.pushUnits(t, f.glorian, 3, true)
	evs := f.enter(t, s, f.glorian)

	q := f.dispatch(t, evs...)
	if len(q.tasks) != 1 || q.tasks[0].Type() != "glorian:push_barcodes" {
		t.Fatalf("listener enqueued %d tasks; want one push task", len(q.tasks))
	}

	p := f.pusher()
	if err := p.PushTask(f.ctx, f.conn.ID); err != nil {
		t.Fatalf("push: %v", err)
	}
	calls := f.srv.RequestsTo(http.MethodPost, "/stock-items/bulk")
	if len(calls) != 1 {
		t.Fatalf("bulk calls = %d; want 1", len(calls))
	}
	var body bulkBody
	if err := calls[0].JSON(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 3 {
		t.Fatalf("bulk items = %d; want 3", len(body.Items))
	}
	want := map[string]bool{}
	for _, u := range s.units {
		want[u.Barcode] = true
	}
	for _, it := range body.Items {
		if !want[it.Barcode] || it.ProductID != pushRemoteProduct ||
			it.Location != glorian.StockLocationCenter || it.Status != glorian.StockStatusAvailable || it.DealerID != nil {
			t.Fatalf("bulk item = %+v", it)
		}
		delete(want, it.Barcode)
		if row, ok := f.srv.StockItem(it.Barcode); !ok || row["status"] != glorian.StockStatusAvailable {
			t.Fatalf("hub stock item %s = %v", it.Barcode, row)
		}
	}
	runs := f.runs(t)
	run := latestRun(t, runs, glorian.KindPushBarcodes)
	if run.Status != glorian.RunSucceeded || !run.Watermark.Valid {
		t.Fatalf("push run = %+v", run)
	}
	if c := pushRunCounts(t, run); c.Barcodes != 3 || c.Calls != 1 || c.Created != 3 {
		t.Fatalf("push counts = %+v", c)
	}

	// Idempotent: a repeated push (retry, overlap) only updates.
	if err := p.PushTask(f.ctx, f.conn.ID); err != nil {
		t.Fatalf("second push: %v", err)
	}
	if c := pushRunCounts(t, latestRun(t, f.runs(t), glorian.KindPushBarcodes)); c.Created != 0 || c.Conflicts != 0 {
		t.Fatalf("second push counts = %+v; want no new rows, no conflicts", c)
	}
}

// An exit sends PATCH by barcode.
func TestPatchOnExit(t *testing.T) {
	f := newPullFixture(t, true)
	s := f.pushUnits(t, f.glorian, 1, true)
	f.enter(t, s, f.glorian)
	p := f.pusher()
	if err := p.PushTask(f.ctx, f.conn.ID); err != nil {
		t.Fatalf("push: %v", err)
	}
	mv, ev := f.post(t, ledger.Movement{Type: ledger.TypeExternalOutbound, UnitID: s.units[0].ID})
	q := f.dispatch(t, ev)
	if len(q.tasks) != 1 || q.tasks[0].Type() != "glorian:patch_stock_item" {
		t.Fatalf("listener enqueued %d tasks; want one patch task", len(q.tasks))
	}
	if err := p.PatchTask(f.ctx, mv.ID); err != nil {
		t.Fatalf("patch: %v", err)
	}
	barcode := s.units[0].Barcode
	calls := f.srv.RequestsTo(http.MethodPatch, "/stock-items/by-barcode/"+barcode)
	if len(calls) != 1 {
		t.Fatalf("patch calls = %d; want 1", len(calls))
	}
	var body glorian.StockItemPatch
	if err := calls[0].JSON(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != glorian.StockStatusExternalOutbound || body.Location != glorian.StockLocationCenter {
		t.Fatalf("patch body = %+v", body)
	}
	if row, _ := f.srv.StockItem(barcode); row["status"] != glorian.StockStatusExternalOutbound {
		t.Fatalf("hub stock item = %v", row)
	}
	run := latestRun(t, f.runs(t), glorian.KindPushBarcodes)
	if c := pushRunCounts(t, run); run.Status != glorian.RunSucceeded || c.Patched != 1 {
		t.Fatalf("patch run = %+v counts %+v", run, c)
	}
	// The PATCH run has no watermark and does not reset the bulk cursor.
	if run.Watermark.Valid {
		t.Fatalf("patch run watermark = %v", run.Watermark)
	}
}

// An Olex product produces no push: no task, no request.
func TestOlexProductProducesNoPush(t *testing.T) {
	f := newPullFixture(t, true)
	s := f.pushUnits(t, f.olex, 2, false)
	evs := f.enter(t, s, f.olex)
	mv, ev := f.post(t, ledger.Movement{Type: ledger.TypeExternalOutbound, UnitID: s.units[0].ID})
	q := f.dispatch(t, append(evs, ev)...)
	if len(q.tasks) != 0 {
		t.Fatalf("listener enqueued %d tasks for an Olex product", len(q.tasks))
	}
	p := f.pusher()
	if err := p.PushTask(f.ctx, f.conn.ID); err != nil {
		t.Fatalf("push: %v", err)
	}
	if err := p.PatchTask(f.ctx, mv.ID); err != nil {
		t.Fatalf("patch: %v", err)
	}
	if reqs := f.srv.Requests(); len(reqs) != 0 {
		t.Fatalf("hub requests = %d; want none", len(reqs))
	}
	for _, r := range f.runs(t) {
		if r.Kind == glorian.KindPushBarcodes {
			t.Fatalf("unexpected push run %+v", r)
		}
	}
}

// A passive connection makes no HTTP call and leaves a held run; the
// barcodes go out once it is active.
func TestPushHeldOnInactiveConnection(t *testing.T) {
	f := newPullFixture(t, false)
	s := f.pushUnits(t, f.glorian, 3, true)
	f.enter(t, s, f.glorian)
	p := f.pusher()
	if err := p.PushTask(f.ctx, f.conn.ID); err != nil {
		t.Fatalf("push: %v (a hold is not retried)", err)
	}
	err := p.PushConnection(f.ctx, f.conn)
	if reason, held := glorian.IsHeld(err); !held || reason != glorian.HeldInactiveConnection {
		t.Fatalf("push connection = %v; want held inactive_connection", err)
	}
	mv, _ := f.post(t, ledger.Movement{Type: ledger.TypeExternalOutbound, UnitID: s.units[0].ID})
	if err := p.PatchTask(f.ctx, mv.ID); err != nil {
		t.Fatalf("patch: %v", err)
	}
	if reqs := f.srv.Requests(); len(reqs) != 0 {
		t.Fatalf("hub requests = %d; want none on an inactive connection", len(reqs))
	}
	var held, patchHeld int
	for _, r := range f.runs(t) {
		if r.Kind != glorian.KindPushBarcodes {
			continue
		}
		if r.Status != glorian.RunFailed || !strings.HasPrefix(r.Error.String, "held: inactive_connection") {
			t.Fatalf("run = %+v; want a held failed run", r)
		}
		switch c := pushRunCounts(t, r); c.Held {
		case 3:
			held++
		case 1:
			patchHeld++
		default:
			t.Fatalf("held counts = %+v", c)
		}
	}
	if held != 2 || patchHeld != 1 {
		t.Fatalf("held runs = %d bulk, %d patch; want 2 and 1", held, patchHeld)
	}

	// Activated: the held barcodes go out (the cursor did not move).
	if _, err := f.tx.Exec(f.ctx, `UPDATE integration_connections SET active = true WHERE id = $1`, f.conn.ID); err != nil {
		t.Fatal(err)
	}
	conn, err := f.q.GetIntegrationConnectionByID(f.ctx, f.conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.PushConnection(f.ctx, conn); err != nil {
		t.Fatalf("push after activation: %v", err)
	}
	if calls := f.srv.RequestsTo(http.MethodPost, "/stock-items/bulk"); len(calls) != 1 {
		t.Fatalf("bulk calls after activation = %d; want 1", len(calls))
	}
}

// A hub 500 fails the task for an Asynq retry (not SkipRetry), the cursor
// stays, and the retry pushes the same barcodes.
func TestPushRetriedOnServerError(t *testing.T) {
	f := newPullFixture(t, true)
	s := f.pushUnits(t, f.glorian, 3, true)
	f.enter(t, s, f.glorian)
	p := f.pusher()

	f.srv.Inject(http.MethodPost, "/stock-items/bulk", fake.ServerError())
	err := p.PushTask(f.ctx, f.conn.ID)
	if err == nil || errors.Is(err, asynq.SkipRetry) || !errors.Is(err, glorian.ErrServer) {
		t.Fatalf("push on 500 = %v; want a retryable server error", err)
	}
	run := latestRun(t, f.runs(t), glorian.KindPushBarcodes)
	if run.Status != glorian.RunFailed || run.Watermark.Valid {
		t.Fatalf("failed run = %+v", run)
	}
	if err := p.PushTask(f.ctx, f.conn.ID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	calls := f.srv.RequestsTo(http.MethodPost, "/stock-items/bulk")
	if len(calls) != 2 {
		t.Fatalf("bulk calls = %d; want 2 (failed + retry)", len(calls))
	}
	var body bulkBody
	if err := calls[1].JSON(&body); err != nil || len(body.Items) != 3 {
		t.Fatalf("retry body = %+v, %v", body, err)
	}

	// PATCH: a 500 is retried, a 409 (hub refuses) is not.
	mv, _ := f.post(t, ledger.Movement{Type: ledger.TypeExternalOutbound, UnitID: s.units[0].ID})
	path := "/stock-items/by-barcode/" + s.units[0].Barcode
	f.srv.Inject(http.MethodPatch, path, fake.ServerError())
	if err := p.PatchTask(f.ctx, mv.ID); err == nil || errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("patch on 500 = %v; want retryable", err)
	}
	f.srv.Inject(http.MethodPatch, path, fake.ErrorEnvelope(http.StatusConflict, glorian.CodeConflict, "busy", nil))
	if err := p.PatchTask(f.ctx, mv.ID); !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("patch on 409 = %v; want SkipRetry", err)
	}
	if err := p.PatchTask(f.ctx, mv.ID); err != nil {
		t.Fatalf("patch retry: %v", err)
	}
}
