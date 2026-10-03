package source

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Fixture schemas created by legacyfixture (TEC-253), keyed by source name.
var FixtureSchemas = map[string]string{
	"hub": "legacy_hub",
	"wh":  "legacy_wh",
}

// Postgres is a LegacySource over a Postgres schema holding a copy of a legacy
// database: the CI fixture schemas legacy_hub / legacy_wh (F2-01b). Every
// query runs in BEGIN READ ONLY with search_path pinned to the schema, so the
// portable step SQL uses the same unqualified table names as on MariaDB.
type Postgres struct {
	name   string
	schema string
	pool   *pgxpool.Pool
	owned  bool
}

// NewPostgres wraps an existing pool; Close leaves the pool open.
func NewPostgres(name string, pool *pgxpool.Pool, schema string) (*Postgres, error) {
	if !validTableName(schema) {
		return nil, fmt.Errorf("source %s: invalid schema %q", name, schema)
	}
	return &Postgres{name: name, schema: schema, pool: pool}, nil
}

// OpenPostgres opens its own pool on dsn; Close closes it.
func OpenPostgres(ctx context.Context, name, dsn, schema string) (*Postgres, error) {
	if !validTableName(schema) {
		return nil, fmt.Errorf("source %s: invalid schema %q", name, schema)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("source %s: parse dsn: %w", name, err)
	}
	cfg.MaxConns = 4
	// Session default as a second line behind the per-query READ ONLY tx.
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("source %s: connect: %w", name, err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("source %s: ping: %w", name, err)
	}
	return &Postgres{name: name, schema: schema, pool: pool, owned: true}, nil
}

// Name implements LegacySource.
func (p *Postgres) Name() string { return p.name }

// Query implements LegacySource.
func (p *Postgres) Query(ctx context.Context, query string, args ...any) (Rows, error) {
	if err := CheckReadOnly(query); err != nil {
		return nil, err
	}
	return p.query(ctx, rewritePlaceholders(query), args...)
}

// query runs q in a read-only transaction without the guard. Only Query and
// the write-refusal test call it.
func (p *Postgres) query(ctx context.Context, q string, args ...any) (Rows, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("source %s: begin read only: %w", p.name, err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('search_path', $1, true)", p.schema); err != nil {
		_ = tx.Rollback(ctx)
		return nil, fmt.Errorf("source %s: search_path: %w", p.name, err)
	}
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, fmt.Errorf("source %s: query: %w", p.name, err)
	}
	return &pgRows{rows: rows, tx: tx, ctx: ctx}, nil
}

// TableExists implements LegacySource.
func (p *Postgres) TableExists(ctx context.Context, table string) (bool, error) {
	if !validTableName(table) {
		return false, ErrBadTableName
	}
	rows, err := p.Query(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = ?", table)
	if err != nil {
		return false, err
	}
	return countExists(rows)
}

// Close implements LegacySource.
func (p *Postgres) Close() error {
	if p.owned {
		p.pool.Close()
	}
	return nil
}

type pgRows struct {
	rows pgx.Rows
	tx   pgx.Tx
	ctx  context.Context
}

func (r *pgRows) Columns() ([]string, error) {
	fds := r.rows.FieldDescriptions()
	cols := make([]string, len(fds))
	for i, fd := range fds {
		cols[i] = fd.Name
	}
	return cols, nil
}

func (r *pgRows) Next() bool             { return r.rows.Next() }
func (r *pgRows) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r *pgRows) Err() error             { return r.rows.Err() }

func (r *pgRows) Close() error {
	r.rows.Close()
	err := r.rows.Err()
	if rbErr := r.tx.Rollback(context.WithoutCancel(r.ctx)); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) && err == nil {
		err = rbErr
	}
	return err
}
