package migrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// RunSummary is a run row of migration_runs (step IS NULL) as the cutover
// preflight (TEC-275) reads it.
type RunSummary struct {
	ID         int64
	Mode       string
	Status     string
	StartedAt  time.Time
	FinishedAt time.Time // zero while the run is unfinished
	Error      string
	// Watermark is the highest watermark of the run's successful step
	// rows; zero when no step recorded one.
	Watermark time.Time
}

// LastRun returns the newest non-dry run of profile, of mode when mode is
// not empty. ok is false when there is none. Step rows are never returned;
// a failed or still running run is (the caller judges its status).
func LastRun(ctx context.Context, q DeltaQuerier, profile string, mode Mode) (RunSummary, bool, error) {
	rows, err := q.Query(ctx, `SELECT r.id, r.mode, r.status, r.started_at, r.finished_at, COALESCE(r.error, ''),
		(SELECT MAX(s.watermark) FROM migration_runs s WHERE s.parent_id = r.id AND s.status = 'succeeded')
		FROM migration_runs r
		WHERE r.profile = $1 AND r.step IS NULL AND NOT r.dry_run AND ($2::text = '' OR r.mode = $2::text)
		ORDER BY r.started_at DESC, r.id DESC
		LIMIT 1`, profile, string(mode))
	if err != nil {
		return RunSummary{}, false, fmt.Errorf("migrator: last run: %w", err)
	}
	var (
		s                   RunSummary
		finished, watermark pgtype.Timestamptz
	)
	row, err := pgx.CollectExactlyOneRow(rows, func(r pgx.CollectableRow) (RunSummary, error) {
		err := r.Scan(&s.ID, &s.Mode, &s.Status, &s.StartedAt, &finished, &s.Error, &watermark)
		return s, err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return RunSummary{}, false, nil
	}
	if err != nil {
		return RunSummary{}, false, fmt.Errorf("migrator: last run: %w", err)
	}
	if finished.Valid {
		row.FinishedAt = finished.Time
	}
	if watermark.Valid {
		row.Watermark = watermark.Time
	}
	return row, true, nil
}
