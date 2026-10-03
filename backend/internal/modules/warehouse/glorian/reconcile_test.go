package glorian_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian/fake"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-272: the reconcile drift report against the fake hub, without a
// database. The querier stub implements only the reads of the report and
// the sync run rows; any other call (a write to units, the ledger or the
// projections) hits the nil embedded interface and panics.

const (
	recCenterOrg = 11
	recDealerOrg = 12
	recBrand     = 2
	recConnID    = 7
)

type reconcileQuerier struct {
	db.Querier // nil: every unlisted method panics
	conn       db.IntegrationConnection
	units      []db.ListGlorianReconcileUnitsRow
	// brandUnits are units of the brand outside the synced set, by barcode.
	brandUnits map[string]db.GetGlorianReconcileUnitByBarcodeRow
	started    []db.StartIntegrationSyncRunParams
	finished   []db.FinishIntegrationSyncRunParams
}

func (q *reconcileQuerier) ListIntegrationConnectionsByKey(_ context.Context, key string) ([]db.IntegrationConnection, error) {
	if key != q.conn.Key {
		return nil, nil
	}
	return []db.IntegrationConnection{q.conn}, nil
}

func (q *reconcileQuerier) StartIntegrationSyncRun(_ context.Context, arg db.StartIntegrationSyncRunParams) (db.IntegrationSyncRun, error) {
	q.started = append(q.started, arg)
	return db.IntegrationSyncRun{ID: int64(len(q.started)), Uuid: uuid.New(), Kind: arg.Kind, Status: glorian.RunRunning}, nil
}

func (q *reconcileQuerier) FinishIntegrationSyncRun(_ context.Context, arg db.FinishIntegrationSyncRunParams) (db.IntegrationSyncRun, error) {
	q.finished = append(q.finished, arg)
	return db.IntegrationSyncRun{ID: arg.ID, Status: arg.Status, Counts: arg.Counts}, nil
}

func (q *reconcileQuerier) ListGlorianReconcileUnits(_ context.Context, arg db.ListGlorianReconcileUnitsParams) ([]db.ListGlorianReconcileUnitsRow, error) {
	if arg.BrandID != q.conn.BrandID || arg.ConnectionID != q.conn.ID {
		return nil, errors.New("wrong scope")
	}
	var out []db.ListGlorianReconcileUnitsRow
	for _, u := range q.units {
		if u.ID > arg.AfterID && len(out) < int(arg.RowLimit) {
			out = append(out, u)
		}
	}
	return out, nil
}

func (q *reconcileQuerier) GetGlorianReconcileUnitByBarcode(_ context.Context, arg db.GetGlorianReconcileUnitByBarcodeParams) (db.GetGlorianReconcileUnitByBarcodeRow, error) {
	if u, ok := q.brandUnits[arg.Barcode]; ok && arg.BrandID == q.conn.BrandID {
		return u, nil
	}
	return db.GetGlorianReconcileUnitByBarcodeRow{}, pgx.ErrNoRows
}

func txt(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func i8(v int64) pgtype.Int8 {
	if v == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
}

// recUnit is a synced local unit: external status, product remote id and
// owner (owner type, holder org).
func recUnit(id int64, barcode, extStatus, product, ownerType string, holder int64) db.ListGlorianReconcileUnitsRow {
	return db.ListGlorianReconcileUnitsRow{
		ID: id, Uuid: uuid.New(), Barcode: barcode, ExternalID: txt("r-" + barcode),
		ExternalStatus: txt(extStatus), ProductExternalID: txt(product),
		OwnerType: txt(ownerType), OwnerID: i8(100 + id), HolderOrgID: i8(holder),
	}
}

func recItem(id, barcode, product string, dealer any, status, location string) fake.Row {
	return fake.Row{
		"id": id, "barcode": barcode, "product_id": product, "dealer_id": dealer,
		"status": status, "location": location, "updated_at": "2026-09-01T12:00:00+00:00",
	}
}

type reconcileEnv struct {
	q   *reconcileQuerier
	srv *fake.Server
	r   *glorian.Reconciler
}

func newReconcileEnv(t *testing.T, active bool) *reconcileEnv {
	t.Helper()
	box, err := crypto.NewSecretBox("test-encryption-key-32-bytes!!!!")
	if err != nil {
		t.Fatal(err)
	}
	enc, err := box.Encrypt(fake.APIKey)
	if err != nil {
		t.Fatal(err)
	}
	srv := fake.New(t)
	q := &reconcileQuerier{
		conn: db.IntegrationConnection{
			ID: recConnID, Uuid: uuid.New(), OrganizationID: recCenterOrg, BrandID: recBrand,
			Key: glorian.ConnectionKey, BaseUrl: srv.URL, ApiKeyEnc: enc, Active: active, ApiVersion: "1",
		},
		brandUnits: map[string]db.GetGlorianReconcileUnitByBarcodeRow{},
	}
	factory := glorian.HTTPClientFactory(glorian.Options{HTTPClient: srv.Client(), RetryBaseDelay: time.Millisecond})
	return &reconcileEnv{q: q, srv: srv, r: glorian.NewReconciler(q, box, factory, nil)}
}

// inSync sets one local unit per remote item, all matching.
func (e *reconcileEnv) inSync() {
	e.q.units = []db.ListGlorianReconcileUnitsRow{
		recUnit(1, "GL-0001", glorian.StockStatusAvailable, "10", "warehouse_location", recCenterOrg),
		recUnit(2, "GL-0002", glorian.StockStatusExternalOutbound, "10", "organization", recDealerOrg),
		// A printed label (no ledger state) is not owner-compared.
		recUnit(3, "GL-0003", glorian.StockStatusReserved, "11", "", 0),
		// An exited unit (trash) keeps location center on the hub.
		recUnit(4, "GL-0004", glorian.StockStatusExternalOutbound, "11", "trash", recCenterOrg),
	}
	e.srv.SetFixture(fake.FixtureStockItems, []fake.Row{
		recItem("r-GL-0001", "GL-0001", "10", nil, glorian.StockStatusAvailable, glorian.StockLocationCenter),
		// Lower case on the hub still matches.
		recItem("r-GL-0002", "gl-0002", "10", "5", glorian.StockStatusExternalOutbound, glorian.StockLocationDealer),
		recItem("r-GL-0003", "GL-0003", "11", nil, glorian.StockStatusReserved, glorian.StockLocationCenter),
		recItem("r-GL-0004", "GL-0004", "11", nil, glorian.StockStatusExternalOutbound, glorian.StockLocationCenter),
	})
}

func (e *reconcileEnv) runCounts(t *testing.T) glorian.ReconcileCounts {
	t.Helper()
	if len(e.q.started) != 1 || e.q.started[0].Kind != glorian.KindReconcile {
		t.Fatalf("started runs = %+v, want one reconcile run", e.q.started)
	}
	if len(e.q.finished) != 1 {
		t.Fatalf("finished runs = %d, want 1", len(e.q.finished))
	}
	var c glorian.ReconcileCounts
	if err := json.Unmarshal(e.q.finished[0].Counts, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestReconcileInSyncHasNoDrift(t *testing.T) {
	e := newReconcileEnv(t, true)
	e.inSync()
	reports, err := e.r.Run(context.Background(), glorian.ConnectionKey)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("reports = %d", len(reports))
	}
	rep := reports[0]
	if rep.HasDrift() || rep.Summary.Total() != 0 || rep.Remote != 4 || rep.Local != 4 {
		t.Fatalf("report = %+v; want 4/4 without drift", rep)
	}
	if code := glorian.ReconcileExitCode(reports, err); code != glorian.ReconcileExitClean {
		t.Fatalf("exit code = %d, want 0", code)
	}
	c := e.runCounts(t)
	if e.q.finished[0].Status != glorian.RunSucceeded || c.Total() != 0 || c.Remote != 4 || c.Local != 4 || c.Pages != 1 {
		t.Fatalf("run = %s %+v", e.q.finished[0].Status, c)
	}
	if e.q.finished[0].Watermark.Valid {
		t.Fatal("reconcile run must not set a watermark")
	}
	// Read only on the hub as well: GETs only.
	for _, r := range e.srv.Requests() {
		if r.Method != "GET" {
			t.Fatalf("reconcile sent %s %s", r.Method, r.Path)
		}
	}
}

func TestReconcileCountsEveryDriftKind(t *testing.T) {
	e := newReconcileEnv(t, true)
	e.q.units = []db.ListGlorianReconcileUnitsRow{
		// In sync.
		recUnit(1, "GL-0001", glorian.StockStatusAvailable, "10", "warehouse_location", recCenterOrg),
		// status_drift: the mirror says available, the hub used.
		recUnit(2, "GL-0002", glorian.StockStatusAvailable, "10", "warehouse_location", recCenterOrg),
		// product_drift: local product 10, hub product 11.
		recUnit(3, "GL-0003", glorian.StockStatusAvailable, "10", "warehouse_location", recCenterOrg),
		// owner_drift: in a center bin locally, at a dealer on the hub.
		recUnit(4, "GL-0004", glorian.StockStatusAvailable, "10", "warehouse_location", recCenterOrg),
		// only_local: the hub does not know it.
		recUnit(5, "GL-0005", "", "10", "warehouse_location", recCenterOrg),
	}
	// A brand unit outside the synced set (product not linked) is paired
	// by barcode: product_drift, not only_remote.
	unlinked := recUnit(6, "GL-0007", glorian.StockStatusAvailable, "", "warehouse_location", recCenterOrg)
	e.q.brandUnits["GL-0007"] = db.GetGlorianReconcileUnitByBarcodeRow(unlinked)
	e.srv.SetFixture(fake.FixtureStockItems, []fake.Row{
		recItem("r-GL-0001", "GL-0001", "10", nil, glorian.StockStatusAvailable, glorian.StockLocationCenter),
		recItem("r-GL-0002", "GL-0002", "10", nil, glorian.StockStatusUsed, glorian.StockLocationCenter),
		recItem("r-GL-0003", "GL-0003", "11", nil, glorian.StockStatusAvailable, glorian.StockLocationCenter),
		recItem("r-GL-0004", "GL-0004", "10", "5", glorian.StockStatusAvailable, glorian.StockLocationDealer),
		// only_remote: no local unit anywhere in the brand.
		recItem("r-GL-0006", "GL-0006", "10", nil, glorian.StockStatusAvailable, glorian.StockLocationCenter),
		recItem("r-GL-0007", "GL-0007", "10", nil, glorian.StockStatusAvailable, glorian.StockLocationCenter),
	})
	reports, err := e.r.Run(context.Background(), glorian.ConnectionKey)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	rep := reports[0]
	want := glorian.ReconcileSummary{OnlyRemote: 1, OnlyLocal: 1, StatusDrift: 1, ProductDrift: 2, OwnerDrift: 1}
	if rep.Summary != want {
		t.Fatalf("summary = %+v, want %+v", rep.Summary, want)
	}
	for name, c := range map[string]struct {
		rows    []glorian.DriftRow
		barcode string
	}{
		glorian.DriftOnlyRemote: {rep.OnlyRemote, "GL-0006"},
		glorian.DriftOnlyLocal:  {rep.OnlyLocal, "GL-0005"},
		glorian.DriftStatus:     {rep.StatusDrift, "GL-0002"},
		glorian.DriftOwner:      {rep.OwnerDrift, "GL-0004"},
	} {
		if len(c.rows) != 1 || c.rows[0].Barcode != c.barcode {
			t.Fatalf("%s = %+v, want %s", name, c.rows, c.barcode)
		}
	}
	if r := rep.StatusDrift[0]; r.LocalExternalStatus != glorian.StockStatusAvailable || r.RemoteStatus != glorian.StockStatusUsed {
		t.Fatalf("status drift row = %+v", r)
	}
	if r := rep.OwnerDrift[0]; r.ExpectedLocation != glorian.StockLocationCenter || r.RemoteLocation != glorian.StockLocationDealer || r.RemoteDealerID != "5" {
		t.Fatalf("owner drift row = %+v", r)
	}
	if rep.ProductDrift[0].Barcode != "GL-0003" || rep.ProductDrift[1].Barcode != "GL-0007" || rep.ProductDrift[1].LocalProductID != "" {
		t.Fatalf("product drift = %+v", rep.ProductDrift)
	}
	if code := glorian.ReconcileExitCode(reports, err); code == glorian.ReconcileExitClean {
		t.Fatal("drift must give a non-zero exit code")
	}
	c := e.runCounts(t)
	if c.ReconcileSummary != want || len(c.Details.ProductDrift) != 2 || len(c.Details.OnlyRemote) != 1 {
		t.Fatalf("run counts = %+v", c)
	}
	if e.q.finished[0].Status != glorian.RunSucceeded {
		t.Fatalf("run status = %s; drift is a report, not a failure", e.q.finished[0].Status)
	}
}

func TestReconcileInactiveConnectionIsHeld(t *testing.T) {
	e := newReconcileEnv(t, false)
	e.inSync()
	reports, err := e.r.Run(context.Background(), glorian.ConnectionKey)
	if !errors.Is(err, glorian.ErrInactiveConnection) {
		t.Fatalf("err = %v, want inactive connection", err)
	}
	if len(reports) != 0 || len(e.srv.Requests()) != 0 {
		t.Fatalf("reports = %d, requests = %d; want none", len(reports), len(e.srv.Requests()))
	}
	if code := glorian.ReconcileExitCode(reports, err); code != glorian.ReconcileExitError {
		t.Fatalf("exit code = %d, want error", code)
	}
	if len(e.q.finished) != 1 || e.q.finished[0].Status != glorian.RunFailed || e.q.finished[0].Error.String != "held: inactive_connection" {
		t.Fatalf("finished = %+v", e.q.finished)
	}
}

func TestReconcileRemoteFailureFailsRun(t *testing.T) {
	e := newReconcileEnv(t, true)
	e.inSync()
	for range 5 {
		e.srv.Inject("GET", "/stock-items", fake.ServerError())
	}
	reports, err := e.r.Run(context.Background(), glorian.ConnectionKey)
	if err == nil || len(reports) != 0 {
		t.Fatalf("reports = %d, err = %v; want a failure", len(reports), err)
	}
	if glorian.ReconcileExitCode(reports, err) != glorian.ReconcileExitError {
		t.Fatal("remote failure must exit with the error code")
	}
	if len(e.q.finished) != 1 || e.q.finished[0].Status != glorian.RunFailed {
		t.Fatalf("finished = %+v", e.q.finished)
	}
}

func TestReconcileUnknownKey(t *testing.T) {
	e := newReconcileEnv(t, true)
	if _, err := e.r.Run(context.Background(), "nope"); !errors.Is(err, glorian.ErrConnectionNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestDiffStockSkipsBlankAndKeepsLastDuplicate(t *testing.T) {
	remote := []glorian.StockItem{
		{ID: "1", Barcode: " ", Status: "available"},
		{ID: "2", Barcode: "A1", Status: "available", ProductID: "10", Location: "center"},
		{ID: "3", Barcode: "a1", Status: "used", ProductID: "10", Location: "center"},
	}
	local := []glorian.ReconcileUnit{{ID: 1, Barcode: "A1", ExternalStatus: "used", ProductExternalID: "10"}}
	rep := glorian.DiffStock(remote, local, recCenterOrg)
	if rep.Skipped != 1 || rep.Remote != 1 || rep.HasDrift() {
		t.Fatalf("report = %+v", rep)
	}
}
