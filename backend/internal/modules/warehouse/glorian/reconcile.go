package glorian

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
)

// TEC-272 (F2-02g): reconcile drift report (warehouse InventoryReconcile).
// Every remote stock item of a connection is compared with the local
// serial units of the connection's brand that belong to the sync (product
// synced from the connection, or unit already mirroring it). The report is
// read only: it never writes units, stock movements or projections; the
// only row it writes is its integration_sync_runs row (kind=reconcile)
// with the counts and the first rows of every category.
//
// Categories (matched by barcode, case-insensitive):
//   - only_remote: a hub stock item without a local unit.
//   - only_local: a synced local unit the hub does not know.
//   - status_drift: units.external_status (the pull mirror) differs from
//     the hub status.
//   - product_drift: the hub product id differs from the local product's
//     remote id (or the local product is not linked).
//   - owner_drift: the hub location (center/dealer) or dealer differs from
//     the local owner in the ledger projection: a center bin or the center
//     organization means location=center without a dealer, another
//     organization means location=dealer. Units in a service, in the trash
//     or without a state (printed labels) are not compared.

// KindReconcile is the sync run kind of the drift report.
const KindReconcile = "reconcile"

// Drift categories.
const (
	DriftOnlyRemote   = "only_remote"
	DriftOnlyLocal    = "only_local"
	DriftStatus       = "status_drift"
	DriftProduct      = "product_drift"
	DriftOwner        = "owner_drift"
	reconcilePageSize = 1000
)

// ReconcileDetailLimit is how many rows per category the sync run keeps.
const ReconcileDetailLimit = 100

// Exit codes of cmd/inventory-reconcile.
const (
	ReconcileExitClean = 0
	ReconcileExitError = 1
	ReconcileExitDrift = 3
)

// Local owner types of unit_current_state used by the owner comparison.
const (
	ownerWarehouseLocation = "warehouse_location"
	ownerOrganization      = "organization"
)

// ReconcileUnit is the local side of one barcode.
type ReconcileUnit struct {
	ID                int64
	UUID              uuid.UUID
	Barcode           string
	ExternalID        string
	ExternalStatus    string
	ProductExternalID string
	// OwnerType is empty for a unit without a ledger state.
	OwnerType   string
	OwnerID     int64
	HolderOrgID int64
}

// DriftRow is one entry of a drift category. Only the fields relevant to
// the category are set.
type DriftRow struct {
	Barcode  string `json:"barcode"`
	UnitID   string `json:"unit_id,omitempty"`
	RemoteID string `json:"remote_id,omitempty"`

	LocalExternalID     string `json:"local_external_id,omitempty"`
	LocalExternalStatus string `json:"local_external_status,omitempty"`
	RemoteStatus        string `json:"remote_status,omitempty"`

	LocalProductID  string `json:"local_product_id,omitempty"`
	RemoteProductID string `json:"remote_product_id,omitempty"`

	LocalOwnerType   string `json:"local_owner_type,omitempty"`
	LocalOwnerID     int64  `json:"local_owner_id,omitempty"`
	ExpectedLocation string `json:"expected_location,omitempty"`
	RemoteLocation   string `json:"remote_location,omitempty"`
	RemoteDealerID   string `json:"remote_dealer_id,omitempty"`
}

// ReconcileSummary are the per-category counts.
type ReconcileSummary struct {
	OnlyRemote   int `json:"only_remote"`
	OnlyLocal    int `json:"only_local"`
	StatusDrift  int `json:"status_drift"`
	ProductDrift int `json:"product_drift"`
	OwnerDrift   int `json:"owner_drift"`
}

// Total is the number of drift rows of every category.
func (s ReconcileSummary) Total() int {
	return s.OnlyRemote + s.OnlyLocal + s.StatusDrift + s.ProductDrift + s.OwnerDrift
}

// ReconcileDetails are the drift rows of every category.
type ReconcileDetails struct {
	OnlyRemote   []DriftRow `json:"only_remote"`
	OnlyLocal    []DriftRow `json:"only_local"`
	StatusDrift  []DriftRow `json:"status_drift"`
	ProductDrift []DriftRow `json:"product_drift"`
	OwnerDrift   []DriftRow `json:"owner_drift"`
}

// Limit returns a copy holding at most n rows per category.
func (d ReconcileDetails) Limit(n int) ReconcileDetails {
	cut := func(rows []DriftRow) []DriftRow {
		if rows == nil {
			return []DriftRow{}
		}
		if len(rows) > n {
			return rows[:n]
		}
		return rows
	}
	return ReconcileDetails{
		OnlyRemote: cut(d.OnlyRemote), OnlyLocal: cut(d.OnlyLocal), StatusDrift: cut(d.StatusDrift),
		ProductDrift: cut(d.ProductDrift), OwnerDrift: cut(d.OwnerDrift),
	}
}

// ReconcileCounts is the counts object of a reconcile sync run.
type ReconcileCounts struct {
	ReconcileSummary
	Remote  int `json:"remote"`
	Local   int `json:"local"`
	Pages   int `json:"pages"`
	Skipped int `json:"skipped"`
	// Details holds the first ReconcileDetailLimit rows of every category.
	Details ReconcileDetails `json:"details"`
}

// ReconcileReport is the drift report of one connection.
type ReconcileReport struct {
	ConnectionUUID uuid.UUID        `json:"connection"`
	RunUUID        uuid.UUID        `json:"run"`
	Remote         int              `json:"remote"`
	Local          int              `json:"local"`
	Skipped        int              `json:"skipped"`
	Summary        ReconcileSummary `json:"summary"`
	ReconcileDetails
}

// HasDrift reports whether any category has a row.
func (r ReconcileReport) HasDrift() bool { return r.Summary.Total() > 0 }

// ReconcileExitCode is the CLI exit status of a reconcile: an error wins,
// then any drift.
func ReconcileExitCode(reports []ReconcileReport, err error) int {
	if err != nil {
		return ReconcileExitError
	}
	for _, r := range reports {
		if r.HasDrift() {
			return ReconcileExitDrift
		}
	}
	return ReconcileExitClean
}

// Reconciler builds the drift report of glorian connections.
type Reconciler struct {
	outbound
}

// NewReconciler wires a reconciler; factory builds the client of a
// connection (HTTPClientFactory in production), log may be nil.
func NewReconciler(q db.Querier, box SecretBox, factory ClientFactory, log *slog.Logger) *Reconciler {
	return &Reconciler{outbound: newOutbound(q, box, factory, log)}
}

// Run reconciles every connection with key. No connection is
// ErrConnectionNotFound. Errors of one connection do not stop the others.
func (r *Reconciler) Run(ctx context.Context, key string) ([]ReconcileReport, error) {
	conns, err := r.q.ListIntegrationConnectionsByKey(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("glorian reconcile: list connections: %w", err)
	}
	if len(conns) == 0 {
		return nil, fmt.Errorf("%w: key %q", ErrConnectionNotFound, key)
	}
	var (
		reports []ReconcileReport
		errs    []error
	)
	for _, conn := range conns {
		rep, err := r.ReconcileConnection(ctx, conn)
		if err != nil {
			errs = append(errs, fmt.Errorf("connection %s: %w", conn.Uuid, err))
			continue
		}
		reports = append(reports, rep)
	}
	return reports, errors.Join(errs...)
}

// ReconcileConnection builds the report of one connection inside a
// reconcile sync run. An inactive connection makes no request and leaves a
// held run.
func (r *Reconciler) ReconcileConnection(ctx context.Context, conn db.IntegrationConnection) (ReconcileReport, error) {
	run, err := r.startRun(ctx, conn, KindReconcile)
	if err != nil {
		return ReconcileReport{}, err
	}
	return r.reconcile(ctx, conn, run)
}

// StartRun opens the running reconcile sync run of conn without doing the
// work; ReconcileRun (the glorian:reconcile task) finishes it. The admin
// endpoint (TEC-273) answers with this row right away.
func (r *Reconciler) StartRun(ctx context.Context, conn db.IntegrationConnection) (db.IntegrationSyncRun, error) {
	run, err := r.startRun(ctx, conn, KindReconcile)
	if err != nil {
		return db.IntegrationSyncRun{}, err
	}
	return run.row, nil
}

// ErrRunNotRunnable: the sync run is not a running reconcile run (already
// finished, e.g. a redelivered task, or another kind). Nothing is done.
var ErrRunNotRunnable = errors.New("glorian reconcile: sync run is not a running reconcile run")

// ReconcileRun does the work of a run opened by StartRun.
func (r *Reconciler) ReconcileRun(ctx context.Context, runID int64) (ReconcileReport, error) {
	row, err := r.q.GetIntegrationSyncRunByID(ctx, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReconcileReport{}, fmt.Errorf("%w: run %d not found", ErrRunNotRunnable, runID)
	}
	if err != nil {
		return ReconcileReport{}, fmt.Errorf("glorian reconcile: run %d: %w", runID, err)
	}
	if row.Kind != KindReconcile || row.Status != RunRunning {
		return ReconcileReport{}, fmt.Errorf("%w: run %s is %s/%s", ErrRunNotRunnable, row.Uuid, row.Kind, row.Status)
	}
	run := &syncRun{q: r.q, row: row}
	conn, err := r.connection(ctx, row.ConnectionID)
	if err != nil {
		return ReconcileReport{}, run.finish(context.WithoutCancel(ctx), ReconcileCounts{}, time.Time{}, err)
	}
	return r.reconcile(ctx, conn, run)
}

// ReconcileTask is the glorian:reconcile handler. The outcome is recorded
// on the run; a finished or missing run is skipped, nothing is retried
// (a retry would find the run finished).
func (r *Reconciler) ReconcileTask(ctx context.Context, runID int64) error {
	_, err := r.ReconcileRun(ctx, runID)
	if errors.Is(err, ErrRunNotRunnable) {
		r.log.Warn("glorian_reconcile_run_skipped", "run_id", runID, "error", err)
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: %w", asynq.SkipRetry, err)
	}
	return nil
}

// reconcile builds the report of conn inside run and finishes the run.
func (r *Reconciler) reconcile(ctx context.Context, conn db.IntegrationConnection, run *syncRun) (ReconcileReport, error) {
	// The run row is recorded even when the context was cancelled mid-run.
	finishCtx := context.WithoutCancel(ctx)
	var counts ReconcileCounts
	client, err := r.client(conn)
	if err != nil {
		return ReconcileReport{}, run.finish(finishCtx, counts, time.Time{}, err)
	}
	remote, pages, err := r.remoteItems(ctx, client)
	counts.Pages = pages
	if err != nil {
		return ReconcileReport{}, run.finish(finishCtx, counts, time.Time{}, fmt.Errorf("remote stock items: %w", err))
	}
	local, err := r.localUnits(ctx, conn)
	if err != nil {
		return ReconcileReport{}, run.finish(finishCtx, counts, time.Time{}, err)
	}
	// Pair remote barcodes with brand units outside the synced set.
	known := make(map[string]bool, len(local))
	for _, u := range local {
		known[barcodeKey(u.Barcode)] = true
	}
	for _, it := range remote {
		key := barcodeKey(it.Barcode)
		if key == "" || known[key] {
			continue
		}
		u, ok, err := r.unitByBarcode(ctx, conn.BrandID, it.Barcode)
		if err != nil {
			return ReconcileReport{}, run.finish(finishCtx, counts, time.Time{}, err)
		}
		if ok {
			known[key] = true
			local = append(local, u)
		}
	}

	rep := DiffStock(remote, local, conn.OrganizationID)
	rep.ConnectionUUID, rep.RunUUID = conn.Uuid, run.row.Uuid
	counts.ReconcileSummary = rep.Summary
	counts.Remote, counts.Local, counts.Skipped = rep.Remote, rep.Local, rep.Skipped
	counts.Details = rep.Limit(ReconcileDetailLimit)
	r.log.Info("glorian_reconcile_done", "connection", conn.Uuid, "remote", rep.Remote, "local", rep.Local,
		DriftOnlyRemote, rep.Summary.OnlyRemote, DriftOnlyLocal, rep.Summary.OnlyLocal,
		DriftStatus, rep.Summary.StatusDrift, DriftProduct, rep.Summary.ProductDrift, DriftOwner, rep.Summary.OwnerDrift)
	if err := run.finish(finishCtx, counts, time.Time{}, nil); err != nil {
		return ReconcileReport{}, err
	}
	return rep, nil
}

// remoteItems pages through every stock item of the hub.
func (r *Reconciler) remoteItems(ctx context.Context, client InventoryClient) ([]StockItem, int, error) {
	var (
		pc  PullCounts
		out []StockItem
	)
	err := pages(ctx, time.Time{}, &pc, client.ListStockItems, func(it StockItem) error {
		out = append(out, it)
		return nil
	})
	return out, pc.Pages, err
}

// localUnits reads the synced serial units of the connection's brand.
func (r *Reconciler) localUnits(ctx context.Context, conn db.IntegrationConnection) ([]ReconcileUnit, error) {
	var (
		out   []ReconcileUnit
		after int64
	)
	for {
		rows, err := r.q.ListGlorianReconcileUnits(ctx, db.ListGlorianReconcileUnitsParams{
			BrandID: conn.BrandID, ConnectionID: conn.ID, AfterID: after, RowLimit: reconcilePageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("glorian reconcile: local units: %w", err)
		}
		for _, row := range rows {
			out = append(out, reconcileUnit(db.GetGlorianReconcileUnitByBarcodeRow(row)))
			after = row.ID
		}
		if len(rows) < reconcilePageSize {
			return out, nil
		}
	}
}

// unitByBarcode finds a serial unit of the brand by a remote barcode
// (stored upper case locally).
func (r *Reconciler) unitByBarcode(ctx context.Context, brandID int64, barcode string) (ReconcileUnit, bool, error) {
	barcode = strings.TrimSpace(barcode)
	codes := []string{barcode}
	if up := strings.ToUpper(barcode); up != barcode {
		codes = append(codes, up)
	}
	for _, code := range codes {
		if code == "" || len(code) > maxUnitBarcode {
			continue
		}
		row, err := r.q.GetGlorianReconcileUnitByBarcode(ctx, db.GetGlorianReconcileUnitByBarcodeParams{BrandID: brandID, Barcode: code})
		if err == nil {
			return reconcileUnit(row), true, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return ReconcileUnit{}, false, fmt.Errorf("glorian reconcile: unit %s: %w", barcode, err)
		}
	}
	return ReconcileUnit{}, false, nil
}

func reconcileUnit(row db.GetGlorianReconcileUnitByBarcodeRow) ReconcileUnit {
	return ReconcileUnit{
		ID: row.ID, UUID: row.Uuid, Barcode: row.Barcode,
		ExternalID: row.ExternalID.String, ExternalStatus: row.ExternalStatus.String,
		ProductExternalID: row.ProductExternalID.String,
		OwnerType:         row.OwnerType.String, OwnerID: row.OwnerID.Int64, HolderOrgID: row.HolderOrgID.Int64,
	}
}

// barcodeKey is the comparison key of a barcode.
func barcodeKey(barcode string) string { return strings.ToUpper(strings.TrimSpace(barcode)) }

// DiffStock compares the remote stock items with the local units. Remote
// items without a barcode are skipped; a barcode repeated on the hub keeps
// its last row. Every category is sorted by barcode. centerOrgID is the connection's (center) organization.
func DiffStock(remote []StockItem, local []ReconcileUnit, centerOrgID int64) ReconcileReport {
	rep := ReconcileReport{ReconcileDetails: ReconcileDetails{
		OnlyRemote: []DriftRow{}, OnlyLocal: []DriftRow{}, StatusDrift: []DriftRow{},
		ProductDrift: []DriftRow{}, OwnerDrift: []DriftRow{},
	}}
	var order []string
	byKey := make(map[string]StockItem, len(remote))
	for _, it := range remote {
		key := barcodeKey(it.Barcode)
		if key == "" {
			rep.Skipped++
			continue
		}
		if _, seen := byKey[key]; !seen {
			order = append(order, key)
		}
		byKey[key] = it
	}
	localByKey := make(map[string]ReconcileUnit, len(local))
	var localOrder []string
	for _, u := range local {
		key := barcodeKey(u.Barcode)
		if _, seen := localByKey[key]; !seen {
			localOrder = append(localOrder, key)
		}
		localByKey[key] = u
	}
	rep.Remote, rep.Local = len(order), len(localOrder)

	for _, key := range order {
		it := byKey[key]
		u, ok := localByKey[key]
		if !ok {
			rep.OnlyRemote = append(rep.OnlyRemote, DriftRow{
				Barcode: strings.TrimSpace(it.Barcode), RemoteID: it.ID, RemoteStatus: it.Status,
				RemoteProductID: it.ProductID, RemoteLocation: it.Location, RemoteDealerID: deref(it.DealerID),
			})
			continue
		}
		delete(localByKey, key)
		base := DriftRow{Barcode: u.Barcode, UnitID: u.UUID.String(), RemoteID: it.ID}

		if remoteStatus := strings.TrimSpace(it.Status); u.ExternalStatus != remoteStatus {
			row := base
			row.LocalExternalID, row.LocalExternalStatus, row.RemoteStatus = u.ExternalID, u.ExternalStatus, remoteStatus
			rep.StatusDrift = append(rep.StatusDrift, row)
		}
		if remoteProduct := strings.TrimSpace(it.ProductID); remoteProduct != "" && u.ProductExternalID != remoteProduct {
			row := base
			row.LocalProductID, row.RemoteProductID = u.ProductExternalID, remoteProduct
			rep.ProductDrift = append(rep.ProductDrift, row)
		}
		if want := expectedLocation(u, centerOrgID); want != "" {
			loc, dealer := strings.TrimSpace(it.Location), strings.TrimSpace(deref(it.DealerID))
			if loc != want || (want == StockLocationCenter && dealer != "") {
				row := base
				row.LocalOwnerType, row.LocalOwnerID, row.ExpectedLocation = u.OwnerType, u.OwnerID, want
				row.RemoteLocation, row.RemoteDealerID = loc, dealer
				rep.OwnerDrift = append(rep.OwnerDrift, row)
			}
		}
	}
	for _, key := range localOrder {
		u, ok := localByKey[key]
		if !ok {
			continue
		}
		rep.OnlyLocal = append(rep.OnlyLocal, DriftRow{
			Barcode: u.Barcode, UnitID: u.UUID.String(), LocalExternalID: u.ExternalID,
			LocalExternalStatus: u.ExternalStatus, LocalProductID: u.ProductExternalID,
			LocalOwnerType: u.OwnerType, LocalOwnerID: u.OwnerID,
		})
	}
	for _, rows := range [][]DriftRow{rep.OnlyRemote, rep.OnlyLocal, rep.StatusDrift, rep.ProductDrift, rep.OwnerDrift} {
		slices.SortStableFunc(rows, func(a, b DriftRow) int { return strings.Compare(a.Barcode, b.Barcode) })
	}
	rep.Summary = ReconcileSummary{
		OnlyRemote: len(rep.OnlyRemote), OnlyLocal: len(rep.OnlyLocal), StatusDrift: len(rep.StatusDrift),
		ProductDrift: len(rep.ProductDrift), OwnerDrift: len(rep.OwnerDrift),
	}
	return rep
}

// expectedLocation is the hub location the local owner implies, or ""
// when the owner is not compared (service, trash, no state).
func expectedLocation(u ReconcileUnit, centerOrgID int64) string {
	switch u.OwnerType {
	case ownerWarehouseLocation, ownerOrganization:
		if u.HolderOrgID == centerOrgID {
			return StockLocationCenter
		}
		return StockLocationDealer
	}
	return ""
}
