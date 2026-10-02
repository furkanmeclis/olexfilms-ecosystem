// Package rebuild regenerates the stock projections from the append-only
// movements, reports the drift against the stored projections and repairs
// it (TEC-156).
//
// Check is a dry run: it reads one snapshot (REPEATABLE READ, read only) and
// writes nothing. Repair with Apply locks the ledger the way Post does (unit
// rows in id order, then SHARE on stock_movements so no movement is posted
// meanwhile), rescans under the lock and sets every drifted projection row
// to the expected value in one transaction, with an activity_events audit
// row. stock_movements is never written.
package rebuild

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
)

// batchSize is the number of units loaded per query.
const batchSize = 500

// lockTimeout bounds the wait for the ledger locks of a repair.
const lockTimeout = "30s"

// AuditAction is the activity_events action of an applied repair.
const AuditAction = "stock.projection.repair"

// auditDiffLimit caps the diffs copied into the audit payload.
const auditDiffLimit = 200

// ErrBusy is returned when a unit could not be locked for the repair.
var ErrBusy = errors.New("rebuild: stock ledger is busy, retry")

// Beginner starts transactions (*pgxpool.Pool).
type Beginner interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}

// Service runs scans and repairs.
type Service struct {
	pool Beginner
	q    *db.Queries
}

// New returns a Service.
func New(pool Beginner, q *db.Queries) *Service {
	return &Service{pool: pool, q: q}
}

// Options select the scope and the mode.
type Options struct {
	// OrganizationID narrows the scan to the units that touched the
	// organization and to its product stock rows; 0 scans everything.
	OrganizationID int64
	// Apply writes the expected projections; false is a dry run.
	Apply bool
	// ActorUserID and Source go to the audit row.
	ActorUserID *int64
	Source      string
}

// Diff is one projection value that differs from the replayed ledger.
type Diff struct {
	Table    string `json:"table"`
	Key      string `json:"key"`
	UnitID   int64  `json:"unit_id,omitempty"`
	Barcode  string `json:"barcode,omitempty"`
	Field    string `json:"field"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

// Report is the result of a scan (and of a repair when Applied).
type Report struct {
	OrganizationID    int64     `json:"organization_id,omitempty"`
	Applied           bool      `json:"applied"`
	UnitsScanned      int       `json:"units_scanned"`
	MovementsReplayed int       `json:"movements_replayed"`
	DiffCount         int       `json:"diff_count"`
	Diffs             []Diff    `json:"diffs"`
	Anomalies         []string  `json:"anomalies"`
	StartedAt         time.Time `json:"started_at"`
	FinishedAt        time.Time `json:"finished_at"`
}

// Check scans without writing.
func (s *Service) Check(ctx context.Context, orgID int64) (Report, error) {
	return s.Run(ctx, Options{OrganizationID: orgID})
}

// Run scans and, with opts.Apply, repairs.
func (s *Service) Run(ctx context.Context, opts Options) (Report, error) {
	if opts.OrganizationID < 0 {
		return Report{}, fmt.Errorf("rebuild: organization id %d", opts.OrganizationID)
	}
	rep := Report{OrganizationID: opts.OrganizationID, StartedAt: time.Now().UTC()}
	txOpts := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	if opts.Apply {
		txOpts = pgx.TxOptions{IsoLevel: pgx.ReadCommitted}
	}
	tx, err := s.pool.BeginTx(ctx, txOpts)
	if err != nil {
		return Report{}, fmt.Errorf("rebuild: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	if opts.Apply {
		if err := lockLedger(ctx, tx, q, opts.OrganizationID); err != nil {
			return Report{}, err
		}
	}
	sc, err := scan(ctx, q, opts.OrganizationID)
	if err != nil {
		return Report{}, err
	}
	sc.fill(&rep)
	if !opts.Apply {
		rep.FinishedAt = time.Now().UTC()
		return rep, tx.Commit(ctx)
	}
	if len(sc.diffs) > 0 {
		if err := sc.apply(ctx, q); err != nil {
			return Report{}, err
		}
		if err := audit(ctx, q, opts, rep); err != nil {
			return Report{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Report{}, fmt.Errorf("rebuild: commit: %w", err)
	}
	rep.Applied = len(sc.diffs) > 0
	rep.FinishedAt = time.Now().UTC()
	return rep, nil
}

// lockLedger takes Post's first lock (the unit rows, id order) on every unit
// in scope, then SHARE on stock_movements: in-flight posts finish, new ones
// wait, and Post writes the projections only after its movement insert, so
// nothing changes them until commit. Units that entered the scope while
// waiting are locked without waiting (a waiting post may hold them).
func lockLedger(ctx context.Context, tx pgx.Tx, q *db.Queries, orgID int64) error {
	if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '"+lockTimeout+"'"); err != nil {
		return fmt.Errorf("rebuild: lock timeout: %w", err)
	}
	ids, err := unitIDs(ctx, q, orgID)
	if err != nil {
		return err
	}
	for b := range slices.Chunk(ids, batchSize) {
		if _, err := q.LockUnitsByIDs(ctx, b); err != nil {
			return fmt.Errorf("rebuild: lock units: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, "LOCK TABLE stock_movements IN SHARE MODE"); err != nil {
		return fmt.Errorf("rebuild: lock movements: %w", err)
	}
	after, err := unitIDs(ctx, q, orgID)
	if err != nil {
		return err
	}
	var extra []int64
	for _, id := range after {
		if _, found := slices.BinarySearch(ids, id); !found {
			extra = append(extra, id)
		}
	}
	if len(extra) > 0 {
		if _, err := tx.Exec(ctx, "SELECT id FROM units WHERE id = ANY($1) ORDER BY id FOR UPDATE NOWAIT", extra); err != nil {
			return fmt.Errorf("%w: %v", ErrBusy, err)
		}
	}
	return nil
}

func unitIDs(ctx context.Context, q *db.Queries, orgID int64) ([]int64, error) {
	var ids []int64
	var err error
	if orgID > 0 {
		ids, err = q.ListRebuildUnitIDsByOrganization(ctx, orgID)
	} else {
		ids, err = q.ListRebuildUnitIDs(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("rebuild: unit ids: %w", err)
	}
	return ids, nil
}

func audit(ctx context.Context, q *db.Queries, opts Options, rep Report) error {
	byTable := map[string]int{}
	for _, d := range rep.Diffs {
		byTable[d.Table]++
	}
	diffs := rep.Diffs
	if len(diffs) > auditDiffLimit {
		diffs = diffs[:auditDiffLimit]
	}
	payload := map[string]any{
		"organization_id":    opts.OrganizationID,
		"source":             opts.Source,
		"units_scanned":      rep.UnitsScanned,
		"movements_replayed": rep.MovementsReplayed,
		"diff_count":         rep.DiffCount,
		"diffs_by_table":     byTable,
		"diffs":              diffs,
		"anomalies":          len(rep.Anomalies),
	}
	body, err := jsonMarshal(payload)
	if err != nil {
		return err
	}
	arg := db.InsertActivityEventParams{Action: AuditAction, Resource: "stock_projection", Payload: body}
	if opts.ActorUserID != nil {
		arg.ActorUserID = i8(*opts.ActorUserID)
	}
	if _, err := q.InsertActivityEvent(ctx, arg); err != nil {
		return fmt.Errorf("rebuild: audit: %w", err)
	}
	return nil
}

// ScanTask is the worker's inventory:rebuild processor: a dry run that logs
// the drift (warn, with the first differences) and never repairs.
func (s *Service) ScanTask(log *slog.Logger) func(ctx context.Context, orgID int64) error {
	if log == nil {
		log = slog.Default()
	}
	return func(ctx context.Context, orgID int64) error {
		rep, err := s.Check(ctx, orgID)
		if err != nil {
			return err
		}
		attrs := []any{
			"organization_id", orgID, "units", rep.UnitsScanned, "movements", rep.MovementsReplayed,
			"diffs", rep.DiffCount, "anomalies", len(rep.Anomalies),
		}
		if rep.DiffCount == 0 && len(rep.Anomalies) == 0 {
			log.Info("inventory_rebuild_clean", attrs...)
			return nil
		}
		sample := rep.Diffs[:min(len(rep.Diffs), 20)]
		log.Warn("inventory_rebuild_drift", append(attrs, "sample", sample)...)
		return nil
	}
}
