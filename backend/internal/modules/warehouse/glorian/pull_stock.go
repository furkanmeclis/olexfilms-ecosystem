package glorian

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-269 (F2-02d): stock item pull. The hub's stock_items changed since
// the last successful pull_stock run are mirrored onto the local unit of
// the same barcode in the connection's brand: units.external_status gets
// the remote status and the unit is linked to the remote row
// (connection_id, external_id).
//
// The mirror is informational only. The pull never posts a ledger movement
// and never touches units.status or the stock projections: the physical
// stock stays owned by the local ledger (K2), and a drift between the two
// is the reconcile job's input. Remote items without a local unit are
// counted as unmatched in the sync run for the same reason.

// maxExternalStatus is the units.external_status column width.
const maxExternalStatus = 32

// maxUnitBarcode is the units.barcode column width.
const maxUnitBarcode = 64

func (r *connectionPull) stockItems(ctx context.Context, since time.Time, counts *PullCounts) (time.Time, error) {
	var newest time.Time
	err := pages(ctx, since, counts, r.client.ListStockItems, func(it StockItem) error {
		newest = later(newest, it.UpdatedAt)
		return r.mirrorStockItem(ctx, it, counts)
	})
	return newest, err
}

func (r *connectionPull) mirrorStockItem(ctx context.Context, it StockItem, counts *PullCounts) error {
	remoteID := strings.TrimSpace(it.ID)
	barcode := strings.TrimSpace(it.Barcode)
	status := strings.TrimSpace(it.Status)
	if remoteID == "" || utf8.RuneCountInString(remoteID) > maxExternalID ||
		barcode == "" || utf8.RuneCountInString(barcode) > maxUnitBarcode ||
		status == "" || utf8.RuneCountInString(status) > maxExternalStatus {
		r.p.log.Warn("glorian_pull_stock_skipped", "connection", r.conn.Uuid, "remote_id", remoteID, "reason", "invalid")
		counts.Skipped++
		return nil
	}
	unit, ok, err := r.unitByBarcode(ctx, barcode)
	if err != nil {
		return err
	}
	if !ok {
		counts.Unmatched++
		return nil
	}
	if unit.ConnectionID.Valid && unit.ConnectionID.Int64 != r.conn.ID {
		r.p.log.Warn("glorian_pull_stock_skipped", "connection", r.conn.Uuid, "remote_id", remoteID, "reason", "other_connection")
		counts.Skipped++
		return nil
	}
	want := db.UpdateUnitExternalParams{
		ID:             unit.ID,
		ConnectionID:   pgtype.Int8{Int64: r.conn.ID, Valid: true},
		ExternalID:     pgtype.Text{String: remoteID, Valid: true},
		ExternalStatus: pgtype.Text{String: status, Valid: true},
	}
	if unit.ConnectionID == want.ConnectionID && unit.ExternalID == want.ExternalID && unit.ExternalStatus == want.ExternalStatus {
		counts.Unchanged++
		return nil
	}
	if _, err := r.p.q.UpdateUnitExternal(ctx, want); err != nil {
		if isUniqueViolation(err) {
			// Another local unit already mirrors this remote row.
			r.p.log.Warn("glorian_pull_stock_skipped", "connection", r.conn.Uuid, "remote_id", remoteID, "reason", "external_id_taken")
			counts.Skipped++
			return nil
		}
		return fmt.Errorf("mirror stock item %s: %w", remoteID, err)
	}
	counts.Updated++
	return nil
}

// unitByBarcode finds the connection brand's unit of a barcode; barcodes
// are stored upper case, so a lower case remote barcode is tried again.
func (r *connectionPull) unitByBarcode(ctx context.Context, barcode string) (db.Unit, bool, error) {
	codes := []string{barcode}
	if up := strings.ToUpper(barcode); up != barcode {
		codes = append(codes, up)
	}
	for _, code := range codes {
		u, err := r.p.q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: r.conn.BrandID, Barcode: code})
		if err == nil {
			return u, true, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return db.Unit{}, false, fmt.Errorf("unit %s: %w", barcode, err)
		}
	}
	return db.Unit{}, false, nil
}
