package migrator

import (
	"context"
	"testing"
	"time"
)

// LastRun picks the newest non-dry run row of the profile (optionally of a
// mode), never a step row, and reports the highest successful step
// watermark. DB test (CI).
func TestLastRun(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	profile := "preflight-" + time.Now().Format("150405.000000")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM migration_runs WHERE profile = $1", profile)
	})

	if _, ok, err := LastRun(ctx, pool, profile, ""); err != nil || ok {
		t.Fatalf("empty profile: ok=%v err=%v", ok, err)
	}

	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	insert := func(parent *int64, mode, step, status string, dry bool, started time.Time, wm *time.Time) int64 {
		t.Helper()
		var id int64
		var stepArg any
		if step != "" {
			stepArg = step
		}
		err := pool.QueryRow(ctx, `INSERT INTO migration_runs (parent_id, profile, mode, step, dry_run, started_at,
			finished_at, status, watermark) VALUES ($1, $2, $3, $4, $5, $6, $6, $7, $8) RETURNING id`,
			parent, profile, mode, stepArg, dry, started, status, wm).Scan(&id)
		if err != nil {
			t.Fatalf("insert run: %v", err)
		}
		return id
	}
	full := insert(nil, "full", "", StatusSucceeded, false, base, nil)
	delta := insert(nil, "delta", "", StatusSucceeded, false, base.Add(time.Hour), nil)
	wm1, wm2 := base.Add(30*time.Minute), base.Add(50*time.Minute)
	insert(&delta, "delta", "users", StatusSucceeded, false, base.Add(time.Hour), &wm1)
	insert(&delta, "delta", "orders", StatusSucceeded, false, base.Add(time.Hour), &wm2)
	insert(nil, "delta", "", StatusSucceeded, true, base.Add(2*time.Hour), nil) // dry run

	last, ok, err := LastRun(ctx, pool, profile, "")
	if err != nil || !ok || last.ID != delta || !last.Watermark.Equal(wm2) || last.FinishedAt.IsZero() {
		t.Fatalf("LastRun any = %+v ok=%v err=%v (want run %d)", last, ok, err, delta)
	}
	last, ok, err = LastRun(ctx, pool, profile, ModeFull)
	if err != nil || !ok || last.ID != full || !last.Watermark.IsZero() || last.Mode != "full" {
		t.Fatalf("LastRun full = %+v ok=%v err=%v (want run %d)", last, ok, err, full)
	}

	failed := insert(nil, "delta", "", StatusFailed, false, base.Add(3*time.Hour), nil)
	if last, _, _ = LastRun(ctx, pool, profile, ModeDelta); last.ID != failed || last.Status != StatusFailed {
		t.Fatalf("LastRun delta = %+v (want failed run %d)", last, failed)
	}
}
