package glorian

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-270 (F2-02e): barcode push. Serial units of products synced from a
// connection are pushed to the hub when they enter or are placed into a
// center bin (one bulk upsert per batch, location=center,
// status=available) and patched by barcode when they leave (exit, transfer
// or shipment). The client is the product's connection; a missing or
// inactive connection makes no request and leaves a held sync run.
//
// Bulk cursor: the push_barcodes run's watermark is the created_at of the
// newest movement it pushed; the next run starts PushOverlap before it, so
// a movement committed late is still seen. The hub upserts by barcode, so
// an overlapping or repeated push is harmless; a failed or held run keeps
// the cursor, and the barcodes go out once the connection works again.

// PushOverlap is subtracted from the last push watermark.
const PushOverlap = 90 * time.Second

// maxPushPages stops a run that would page forever.
const maxPushPages = 1000

// PushCounts are the per-run totals of a push_barcodes run.
type PushCounts struct {
	// Barcodes is how many distinct barcodes the run had to push.
	Barcodes  int `json:"barcodes"`
	Calls     int `json:"calls"`
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Conflicts int `json:"conflicts"`
	Patched   int `json:"patched"`
	Held      int `json:"held"`
}

// Pusher runs the barcode push and the outbound PATCH.
type Pusher struct {
	outbound
}

// NewPusher wires a pusher; factory builds the client of a connection
// (HTTPClientFactory in production), log may be nil.
func NewPusher(q db.Querier, box SecretBox, factory ClientFactory, log *slog.Logger) *Pusher {
	return &Pusher{outbound: newOutbound(q, box, factory, log)}
}

// PushTask is the glorian:push_barcodes handler. connectionID 0 (the cron
// run) pushes every active glorian connection; an inactive one is skipped
// there without a run. A failure is returned for an Asynq retry.
func (p *Pusher) PushTask(ctx context.Context, connectionID int64) error {
	if connectionID != 0 {
		conn, err := p.connection(ctx, connectionID)
		if errors.Is(err, ErrConnectionNotFound) {
			p.log.Warn("glorian_push_connection_missing", "connection_id", connectionID)
			return nil
		}
		if err != nil {
			return err
		}
		return TaskError(p.PushConnection(ctx, conn))
	}
	conns, err := p.q.ListIntegrationConnectionsByKey(ctx, ConnectionKey)
	if err != nil {
		return fmt.Errorf("glorian push: list connections: %w", err)
	}
	var errs []error
	for _, conn := range conns {
		if !conn.Active {
			continue
		}
		if err := p.PushConnection(ctx, conn); err != nil {
			errs = append(errs, fmt.Errorf("connection %s: %w", conn.Uuid, err))
		}
	}
	return TaskError(errors.Join(errs...))
}

// PushConnection bulk-upserts the barcodes entered or placed since the
// connection's last successful push. Nothing pending means no request and
// no run. On an inactive connection it records a held run (counts.held)
// and returns the HeldError.
func (p *Pusher) PushConnection(ctx context.Context, conn db.IntegrationConnection) error {
	since, err := p.pushSince(ctx, conn.ID)
	if err != nil {
		return err
	}
	cur := pushCursor{at: since}
	batch, err := p.nextBatch(ctx, conn, &cur)
	if err != nil {
		return err
	}
	if len(batch) == 0 {
		return nil
	}
	run, err := p.startRun(ctx, conn, KindPushBarcodes)
	if err != nil {
		return err
	}
	var counts PushCounts
	client, err := p.client(conn)
	if err != nil {
		if _, held := IsHeld(err); held {
			counts.Held = len(batch)
			p.log.Warn("glorian_push_held", "connection", conn.Uuid, "reason", HeldInactiveConnection, "barcodes", len(batch))
		}
		return run.finish(ctx, counts, time.Time{}, err)
	}
	var newest time.Time
	for page := 0; len(batch) > 0; page++ {
		if page >= maxPushPages {
			return run.finish(ctx, counts, time.Time{}, fmt.Errorf("glorian push: more than %d pages", maxPushPages))
		}
		counts.Barcodes += len(batch)
		counts.Calls++
		res, err := client.UpsertBarcodes(ctx, batch)
		if err != nil {
			return run.finish(ctx, counts, time.Time{}, fmt.Errorf("bulk upsert: %w", err))
		}
		counts.Created += len(res.Created)
		counts.Updated += len(res.Updated)
		counts.Conflicts += len(res.Conflicts)
		for _, c := range res.Conflicts {
			p.log.Warn("glorian_push_conflict", "connection", conn.Uuid, "barcode", c.Barcode, "reason", c.Reason)
		}
		newest = cur.at
		if batch, err = p.nextBatch(ctx, conn, &cur); err != nil {
			return run.finish(ctx, counts, time.Time{}, err)
		}
	}
	p.log.Info("glorian_push_done", "connection", conn.Uuid, "barcodes", counts.Barcodes, "calls", counts.Calls,
		"created", counts.Created, "updated", counts.Updated, "conflicts", counts.Conflicts)
	return run.finish(ctx, counts, newest, nil)
}

// pushSince is the bulk cursor: the last successful watermark minus the
// overlap, zero for the first push.
func (p *Pusher) pushSince(ctx context.Context, connectionID int64) (time.Time, error) {
	wm, err := p.q.LastIntegrationSyncRunWatermark(ctx, db.LastIntegrationSyncRunWatermarkParams{
		ConnectionID: connectionID, Kind: KindPushBarcodes,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return time.Time{}, nil
	case err != nil:
		return time.Time{}, fmt.Errorf("glorian push: last watermark: %w", err)
	case !wm.Valid:
		return time.Time{}, nil
	}
	return wm.Time.Add(-PushOverlap), nil
}

type pushCursor struct {
	at time.Time
	id int64
}

// nextBatch reads up to MaxBulkItems distinct barcodes after cur and moves
// cur past the movements it read.
func (p *Pusher) nextBatch(ctx context.Context, conn db.IntegrationConnection, cur *pushCursor) ([]BarcodeUpsert, error) {
	var out []BarcodeUpsert
	seen := map[string]bool{}
	for len(out) < MaxBulkItems {
		rows, err := p.q.ListGlorianPushUnits(ctx, db.ListGlorianPushUnitsParams{
			BrandID: conn.BrandID, ConnectionID: conn.ID,
			AfterCreatedAt: pgtype.Timestamptz{Time: cur.at, Valid: true}, AfterID: cur.id,
			RowLimit: int32(MaxBulkItems - len(out)),
		})
		if err != nil {
			return nil, fmt.Errorf("glorian push: list units: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			cur.at, cur.id = r.CreatedAt.Time, r.MovementID
			barcode, product := strings.TrimSpace(r.Barcode), strings.TrimSpace(r.ProductExternalID)
			if barcode == "" || product == "" || seen[barcode] {
				continue
			}
			seen[barcode] = true
			out = append(out, BarcodeUpsert{
				Barcode: barcode, ProductID: product,
				Location: StockLocationCenter, Status: StockStatusAvailable,
			})
		}
	}
	return out, nil
}

// Patch of a movement type: the hub state of a unit after it.
var patchByMovement = map[string]StockItemPatch{
	// Exit: the unit left the warehouse (to trash, status used).
	"external_outbound": {Status: StockStatusExternalOutbound},
	// Transfer and shipment to another organization.
	"transfer_out": {Status: StockStatusExternalOutbound, Location: StockLocationDealer},
	"order_out":    {Status: StockStatusExternalOutbound, Location: StockLocationDealer},
	// A cancelled transfer or order puts the unit back into the center.
	"transfer_cancel_restore": {Status: StockStatusAvailable, Location: StockLocationCenter},
	"order_cancel_restore":    {Status: StockStatusAvailable, Location: StockLocationCenter},
}

// PatchMovementTypes are the stock movement types that send a PATCH.
func PatchMovementTypes() []string {
	out := make([]string, 0, len(patchByMovement))
	for k := range patchByMovement {
		out = append(out, k)
	}
	return out
}

// PatchFor returns the PATCH body of a movement type (false: no PATCH).
func PatchFor(movementType string) (StockItemPatch, bool) {
	patch, ok := patchByMovement[movementType]
	return patch, ok
}

// PatchTask is the glorian:patch_stock_item handler.
func (p *Pusher) PatchTask(ctx context.Context, movementID int64) error {
	return TaskError(p.PatchMovement(ctx, movementID))
}

// PatchMovement sends the PATCH of one movement of a synced serial unit. A
// movement of an unsynced product (e.g. Olex) sends nothing. An inactive
// connection records a held run and returns the HeldError; a hub that does
// not know the barcode yet (404, bulk push pending) is a transient error.
func (p *Pusher) PatchMovement(ctx context.Context, movementID int64) error {
	mv, err := p.q.GetGlorianPushMovement(ctx, movementID)
	if errors.Is(err, pgx.ErrNoRows) {
		p.log.Warn("glorian_patch_movement_missing", "movement_id", movementID)
		return nil
	}
	if err != nil {
		return fmt.Errorf("glorian patch: movement %d: %w", movementID, err)
	}
	patch, ok := PatchFor(mv.Type)
	if !ok || mv.UnitKind != "serial" || !mv.ConnectionID.Valid {
		return nil
	}
	conn, err := p.connection(ctx, mv.ConnectionID.Int64)
	if errors.Is(err, ErrConnectionNotFound) {
		p.log.Warn("glorian_patch_held", "movement_id", movementID, "barcode", mv.Barcode, "reason", "connection_missing")
		return nil
	}
	if err != nil {
		return err
	}
	run, err := p.startRun(ctx, conn, KindPushBarcodes)
	if err != nil {
		return err
	}
	var counts PushCounts
	client, err := p.client(conn)
	if err != nil {
		if _, held := IsHeld(err); held {
			counts.Held = 1
			p.log.Warn("glorian_patch_held", "connection", conn.Uuid, "movement_id", movementID, "barcode", mv.Barcode, "reason", HeldInactiveConnection)
		}
		return run.finish(ctx, counts, time.Time{}, err)
	}
	counts.Calls = 1
	if _, err := client.PatchStockItemByBarcode(ctx, mv.Barcode, patch); err != nil {
		return run.finish(ctx, counts, time.Time{}, fmt.Errorf("patch %s: %w", mv.Barcode, err))
	}
	counts.Patched = 1
	return run.finish(ctx, counts, time.Time{}, nil)
}
