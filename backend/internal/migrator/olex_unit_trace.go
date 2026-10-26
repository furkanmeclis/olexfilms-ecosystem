package migrator

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
)

// UnitTraceStep completes the stock trace of the legacy units (TEC-1006).
//
// TEC-258 left the units used in legacy services without an owner (the
// service did not exist yet), so they were invisible in the warehouse layer
// (no unit_current_state). Once the services are migrated this step records
// one consumption movement per such unit to the service that used it — the
// latest completed one when a unit appears in several — and rebuilds the
// projections of the organizations involved. A unit that already has an
// owner is left alone (the ledger is the truth for it), so a rerun writes
// nothing.
//
// It then checks the warehouse layer against the legacy systems: every
// legacy barcode (hub stock items and non-Glorian warehouse barcodes) must
// be a unit, and every unit must have an owner (in stock, at a dealer, used
// in a service or in the trash). Missing barcodes and ownerless units are
// counted with their reason.
type UnitTraceStep struct {
	System   string
	WHSystem string
}

// Name implements Step.
func (UnitTraceStep) Name() string { return "unit_trace" }

// traceKeyPart names the movements this step writes (idempotency keys).
const traceKeyPart = "trace:service"

// unitTraceReason is the reason of the service consumption movements.
const unitTraceReason = "legacy service consumption"

// maxMissingReported caps the per-barcode report keys.
const maxMissingReported = 50

// Run implements Step.
func (s UnitTraceStep) Run(ctx context.Context, src Sources, dst *Target, _ *Mapper) (StepResult, error) {
	c := counts{}
	tree, err := resolveOlexTree(ctx, dst.Q, false, c)
	if err != nil {
		return StepResult{Counts: c}, err
	}

	rows, err := dst.Q.MigratorUnitsUsedWithoutOwner(ctx, tree.BrandID)
	if err != nil {
		return StepResult{Counts: c}, fmt.Errorf("used units: %w", err)
	}
	led := ledger.New(dst.Q, nil) // no outbox: imports trigger nothing
	orgs := map[int64]bool{}
	for _, r := range rows {
		at := time.Now()
		if r.CompletedAt.Valid {
			at = r.CompletedAt.Time
		}
		res, err := led.Import(ctx, dst.Tx, ledger.Recorded{
			UnitID: r.UnitID, Type: ledger.TypeConsumption, OrganizationID: r.OrganizationID,
			To:       &ledger.Owner{Type: ledger.OwnerService, ID: r.ServiceID, OrgID: r.OrganizationID},
			ToStatus: ledger.StatusUsed,
			Key:      fmt.Sprintf("%s%s:%s", ledger.ImportKeyPrefix, traceKeyPart, r.UnitUuid),
			Reason:   unitTraceReason,
			Metadata: map[string]any{
				"service_no": r.ServiceNo, "service_item_uuid": r.ServiceItemUuid.String(), "legacy_trace": true,
			},
			CreatedAt: at,
		})
		if err != nil {
			return StepResult{Counts: c}, fmt.Errorf("unit %d: %w", r.UnitID, err)
		}
		if res.Replayed {
			c.inc("service_consumption_replayed")
			continue
		}
		c.inc("service_consumption_created")
		orgs[r.OrganizationID] = true
	}
	orgIDs := make([]int64, 0, len(orgs))
	for id := range orgs {
		orgIDs = append(orgIDs, id)
	}
	slices.Sort(orgIDs)
	for _, id := range orgIDs {
		rep, err := rebuild.ApplyTx(ctx, dst.Tx, rebuild.Options{OrganizationID: id, Source: "migrator"})
		if err != nil {
			return StepResult{Counts: c}, fmt.Errorf("rebuild organization %d: %w", id, err)
		}
		c.inc("rebuild_organizations")
		c.add("rebuild_diffs", int64(rep.DiffCount))
		c.add("rebuild_anomalies", int64(len(rep.Anomalies)))
	}

	if err := s.check(ctx, src, dst.Q, tree.BrandID, c); err != nil {
		return StepResult{Counts: c}, err
	}
	return StepResult{Counts: c}, nil
}

// check counts the legacy barcodes against the units and the units without
// an owner (TEC-1006 consistency report).
func (s UnitTraceStep) check(ctx context.Context, src Sources, q *db.Queries, brandID int64, c counts) error {
	legacy := map[string]bool{}
	if hub, err := src.Get(SourceHub); err == nil {
		rows, err := hub.Query(ctx, `SELECT barcode FROM stock_items`)
		if err != nil {
			return fmt.Errorf("hub barcodes: %w", err)
		}
		for rows.Next() {
			var b string
			if err := rows.Scan(&b); err != nil {
				_ = rows.Close()
				return err
			}
			if b = strings.TrimSpace(b); b != "" {
				legacy[b] = true
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	if wh, err := src.Get(SourceWH); err == nil {
		rows, err := wh.Query(ctx, `SELECT pb.code, COALESCE(b.name, '') FROM product_barcodes pb
			JOIN products p ON p.id = pb.product_id LEFT JOIN brands b ON b.id = p.brand_id`)
		if err != nil {
			return fmt.Errorf("warehouse barcodes: %w", err)
		}
		for rows.Next() {
			var code, brand string
			if err := rows.Scan(&code, &brand); err != nil {
				_ = rows.Close()
				return err
			}
			if isGlorian(brand) {
				continue
			}
			if code = strings.TrimSpace(code); code != "" {
				legacy[code] = true
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	codes := make([]string, 0, len(legacy))
	for b := range legacy {
		codes = append(codes, b)
	}
	slices.Sort(codes)
	c.add("legacy_barcodes", int64(len(codes)))
	found, err := q.MigratorUnitBarcodesIn(ctx, db.MigratorUnitBarcodesInParams{BrandID: brandID, Barcodes: codes})
	if err != nil {
		return fmt.Errorf("unit barcodes: %w", err)
	}
	have := make(map[string]bool, len(found))
	for _, b := range found {
		have[b] = true
	}
	c.add("legacy_barcodes_in_units", int64(len(have)))
	reported := 0
	for _, b := range codes {
		if have[b] {
			continue
		}
		c.inc("legacy_barcodes_missing")
		if reported < maxMissingReported {
			c.inc("legacy_barcode_missing:" + b)
			reported++
		}
	}

	st, err := q.MigratorUnitOwnershipCounts(ctx, brandID)
	if err != nil {
		return fmt.Errorf("unit ownership: %w", err)
	}
	for _, r := range st {
		key := "units_owner:" + r.OwnerType
		if r.OwnerType == "" {
			key = "units_without_owner:" + r.Status
		}
		c.add(key, r.Units)
	}
	return nil
}

func isGlorian(brand string) bool {
	return strings.Contains(strings.ToLower(brand), "glorian")
}
