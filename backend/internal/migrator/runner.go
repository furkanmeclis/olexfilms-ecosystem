package migrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// Mode is a run mode.
type Mode string

const (
	// ModeFull reads every legacy row.
	ModeFull Mode = "full"
	// ModeDelta reads rows changed since the step's last watermark.
	ModeDelta Mode = "delta"
)

// ParseMode validates a --mode value.
func ParseMode(s string) (Mode, error) {
	switch Mode(s) {
	case ModeFull, ModeDelta:
		return Mode(s), nil
	}
	return "", fmt.Errorf("migrator: unknown mode %q (full|delta)", s)
}

// Sources are the open legacy sources of a run, keyed by source name.
type Sources map[string]source.LegacySource

// Get returns the named source or an error naming the missing one.
func (s Sources) Get(name string) (source.LegacySource, error) {
	src, ok := s[name]
	if !ok || src == nil {
		return nil, fmt.Errorf("migrator: source %q is not open", name)
	}
	return src, nil
}

// Target is the write side a step gets: its own transaction on the new
// database plus the run parameters.
type Target struct {
	Tx pgx.Tx
	Q  *db.Queries
	// Mode and Since: in delta mode Since is the step's last successful
	// watermark (zero when it never ran, which means a full read).
	Mode   Mode
	Since  time.Time
	DryRun bool
	// Storage receives copied legacy media (nil: media is not copied).
	Storage storage.Driver
	// LegacyFiles is the legacy hub storage directory (nil: not mounted).
	LegacyFiles fs.FS
}

// StepResult is what a step reports back.
type StepResult struct {
	// Counts are free-form per-step counters (read, created, updated, ...).
	Counts map[string]int64
	// Watermark is the highest source timestamp seen; zero keeps none.
	Watermark time.Time
}

// Step imports one slice of legacy data.
type Step interface {
	Name() string
	Run(ctx context.Context, src Sources, dst *Target, mapper *Mapper) (StepResult, error)
}

// Options are the parameters of one run.
type Options struct {
	Profile string
	Mode    Mode
	// Steps limits the run to these step names (profile order kept).
	Steps  []string
	DryRun bool
	// Overlap is subtracted from the watermark in delta mode; zero means
	// DefaultDeltaOverlap, a negative value none.
	Overlap time.Duration
}

// DefaultDeltaOverlap is the safety overlap a delta run reads before a
// step's last watermark (TEC-264): rows the legacy side wrote while the
// previous run was reading, or stamped with a clock slightly behind, are read
// again. The steps are idempotent (migration_map + checksum), so re-reading
// an unchanged row writes nothing.
const DefaultDeltaOverlap = 10 * time.Minute

// DeltaSince is the lower bound a delta step reads from: the last watermark
// minus the overlap (zero: DefaultDeltaOverlap, negative: none). A zero
// watermark (the step never ran) stays zero, which means a full read.
func DeltaSince(watermark time.Time, overlap time.Duration) time.Time {
	if watermark.IsZero() {
		return time.Time{}
	}
	switch {
	case overlap == 0:
		overlap = DefaultDeltaOverlap
	case overlap < 0:
		overlap = 0
	}
	return watermark.Add(-overlap)
}

// OpenFunc opens a legacy source by name.
type OpenFunc func(ctx context.Context, name string) (source.LegacySource, error)

// DB is the new database a run writes to: a *pgxpool.Pool, or a
// transaction (each step then runs in a savepoint; tests roll the whole run
// back, which the append-only stock ledger needs, TEC-258).
type DB interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

var _ DB = (*pgxpool.Pool)(nil)

// Runner executes profile runs against the new database.
type Runner struct {
	Pool DB
	// Open opens a legacy source; called only when a run has steps.
	Open OpenFunc
	// Profiles defaults to Profiles().
	Profiles map[string]Profile
	Log      *slog.Logger
	// Storage and LegacyFiles are handed to the steps that copy legacy
	// media (TEC-256); either may be nil.
	Storage     storage.Driver
	LegacyFiles fs.FS
}

// RunReport summarizes a finished run.
type RunReport struct {
	RunID  int64
	Status string
	Steps  []StepReport
}

// StepReport is one executed step.
type StepReport struct {
	Name      string
	RunID     int64
	Status    string
	Counts    map[string]int64
	Watermark time.Time
	Error     string
}

// Run statuses (migration_runs.status).
const (
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

// Run executes opts. Every run, including one with no steps, leaves a
// migration_runs row; each executed step adds a child row. The first failing
// step stops the run.
func (r *Runner) Run(ctx context.Context, opts Options) (RunReport, error) {
	profiles := r.Profiles
	if profiles == nil {
		profiles = Profiles()
	}
	if opts.Mode == "" {
		opts.Mode = ModeFull
	}
	if _, err := ParseMode(string(opts.Mode)); err != nil {
		return RunReport{}, err
	}
	profile, err := Lookup(profiles, opts.Profile)
	if err != nil {
		return RunReport{}, err
	}
	steps, err := SelectSteps(profile, opts.Steps)
	if err != nil {
		return RunReport{}, err
	}
	log := r.Log
	if log == nil {
		log = slog.Default()
	}

	q := db.New(r.Pool)
	run, err := q.CreateMigrationRun(ctx, db.CreateMigrationRunParams{
		Profile: profile.Name, Mode: string(opts.Mode), DryRun: opts.DryRun,
	})
	if err != nil {
		return RunReport{}, fmt.Errorf("migrator: create run: %w", err)
	}
	rep := RunReport{RunID: run.ID}
	log.Info("migrator: run started", "run_id", run.ID, "profile", profile.Name, "mode", opts.Mode,
		"dry_run", opts.DryRun, "steps", len(steps))

	srcs, closeSources, err := r.openSources(ctx, profile, len(steps) > 0)
	defer closeSources()
	if err == nil {
		for _, step := range steps {
			sr, stepErr := r.runStep(ctx, q, run, profile.Name, opts, step, srcs)
			rep.Steps = append(rep.Steps, sr)
			if stepErr != nil {
				err = stepErr
				break
			}
		}
	}

	total := map[string]int64{}
	for _, s := range rep.Steps {
		for k, v := range s.Counts {
			total[s.Name+"."+k] += v
		}
	}
	rep.Status = StatusSucceeded
	if err != nil {
		rep.Status = StatusFailed
	}
	if _, finErr := finish(context.WithoutCancel(ctx), q, run.ID, rep.Status, total, time.Time{}, err); finErr != nil {
		return rep, errors.Join(err, finErr)
	}
	log.Info("migrator: run finished", "run_id", run.ID, "status", rep.Status)
	return rep, err
}

func (r *Runner) openSources(ctx context.Context, p Profile, needed bool) (Sources, func(), error) {
	if !needed {
		return Sources{}, func() {}, nil
	}
	if r.Open == nil {
		return Sources{}, func() {}, errors.New("migrator: no source opener configured")
	}
	srcs, closeAll, err := OpenSources(ctx, p, r.Open)
	if err != nil {
		err = fmt.Errorf("migrator: %w", err)
	}
	return srcs, closeAll, err
}

func (r *Runner) runStep(ctx context.Context, q *db.Queries, run db.MigrationRun, profile string, opts Options, step Step, srcs Sources) (StepReport, error) {
	name := step.Name()
	sr := StepReport{Name: name}
	row, err := q.CreateMigrationRun(ctx, db.CreateMigrationRunParams{
		ParentID: pgtype.Int8{Int64: run.ID, Valid: true},
		Profile:  profile,
		Mode:     string(opts.Mode),
		Step:     pgtype.Text{String: name, Valid: true},
		DryRun:   opts.DryRun,
	})
	if err != nil {
		return sr, fmt.Errorf("migrator: create step run %s: %w", name, err)
	}
	sr.RunID = row.ID

	var prev, since time.Time
	if opts.Mode == ModeDelta {
		wm, err := q.LastMigrationWatermark(ctx, db.LastMigrationWatermarkParams{
			Profile: profile, Step: pgtype.Text{String: name, Valid: true},
		})
		switch {
		case err == nil && wm.Valid:
			prev = wm.Time
			since = DeltaSince(prev, opts.Overlap)
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return r.failStep(ctx, q, sr, fmt.Errorf("migrator: read watermark %s: %w", name, err))
		}
	}

	res, err := r.execStep(ctx, step, srcs, opts, since)
	if err != nil {
		sr.Counts = res.Counts
		return r.failStep(ctx, q, sr, fmt.Errorf("migrator: step %s: %w", name, err))
	}
	// The watermark never moves back: a delta run that saw nothing newer
	// (only the overlap, or no row at all) keeps the previous one.
	if res.Watermark.Before(prev) {
		res.Watermark = prev
	}
	sr.Status, sr.Counts, sr.Watermark = StatusSucceeded, res.Counts, res.Watermark
	if _, err := finish(ctx, q, row.ID, StatusSucceeded, res.Counts, res.Watermark, nil); err != nil {
		return sr, err
	}
	return sr, nil
}

// execStep runs step in its own transaction; a dry run always rolls back.
func (r *Runner) execStep(ctx context.Context, step Step, srcs Sources, opts Options, since time.Time) (StepResult, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return StepResult{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	dst := &Target{Tx: tx, Q: db.New(tx), Mode: opts.Mode, Since: since, DryRun: opts.DryRun,
		Storage: r.Storage, LegacyFiles: r.LegacyFiles}
	res, err := step.Run(ctx, srcs, dst, NewMapper(dst.Q))
	if err != nil {
		return res, err
	}
	if opts.DryRun {
		return res, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return res, fmt.Errorf("commit: %w", err)
	}
	return res, nil
}

func (r *Runner) failStep(ctx context.Context, q *db.Queries, sr StepReport, err error) (StepReport, error) {
	sr.Status, sr.Error = StatusFailed, err.Error()
	if _, finErr := finish(context.WithoutCancel(ctx), q, sr.RunID, StatusFailed, sr.Counts, time.Time{}, err); finErr != nil {
		return sr, errors.Join(err, finErr)
	}
	return sr, err
}

func finish(ctx context.Context, q *db.Queries, id int64, status string, counts map[string]int64, watermark time.Time, runErr error) (db.MigrationRun, error) {
	if counts == nil {
		counts = map[string]int64{}
	}
	raw, err := json.Marshal(counts)
	if err != nil {
		return db.MigrationRun{}, fmt.Errorf("migrator: encode counts: %w", err)
	}
	var errText pgtype.Text
	if runErr != nil {
		errText = pgtype.Text{String: runErr.Error(), Valid: true}
	}
	row, err := q.FinishMigrationRun(ctx, db.FinishMigrationRunParams{
		ID:        id,
		Status:    status,
		Counts:    raw,
		Watermark: pgtype.Timestamptz{Time: watermark, Valid: !watermark.IsZero()},
		Error:     errText,
	})
	if err != nil {
		return row, fmt.Errorf("migrator: finish run %d: %w", id, err)
	}
	return row, nil
}
