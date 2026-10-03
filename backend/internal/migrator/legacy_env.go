package migrator

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
)

// LegacyDSNEnv maps a source name to its DSN variable (TEC-252). Only
// placeholders live in .env.example; the real DSNs are set on the server by
// hand.
var LegacyDSNEnv = map[string]string{
	SourceHub: "LEGACY_HUB_DSN",
	SourceWH:  "LEGACY_WH_DSN",
}

// OpenLegacyFromEnv opens a legacy source from its DSN variable: a
// go-sql-driver DSN opens MariaDB, a postgres:// DSN the legacy fixture
// schema. Used by cmd/migrator and cmd/cutover-preflight.
func OpenLegacyFromEnv(ctx context.Context, name string) (source.LegacySource, error) {
	key, ok := LegacyDSNEnv[name]
	if !ok {
		return nil, fmt.Errorf("no DSN variable for source %q", name)
	}
	dsn := strings.TrimSpace(os.Getenv(key))
	if dsn == "" {
		return nil, fmt.Errorf("%s is not set", key)
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return source.OpenPostgres(ctx, name, dsn, source.FixtureSchemas[name])
	}
	return source.OpenMariaDB(ctx, name, dsn)
}

// OpenSources opens every source of p with open. The returned close
// function closes the sources opened so far, also after an error.
func OpenSources(ctx context.Context, p Profile, open func(context.Context, string) (source.LegacySource, error)) (Sources, func(), error) {
	srcs := Sources{}
	closeAll := func() {
		for _, s := range srcs {
			_ = s.Close()
		}
	}
	for _, name := range p.Sources {
		s, err := open(ctx, name)
		if err != nil {
			return srcs, closeAll, fmt.Errorf("open source %s: %w", name, err)
		}
		srcs[name] = s
	}
	return srcs, closeAll, nil
}

// BuildReportReadOnly opens the profile's legacy sources and builds the
// validation report in a READ ONLY, REPEATABLE READ transaction of pool.
func BuildReportReadOnly(ctx context.Context, pool *pgxpool.Pool, p Profile,
	open func(context.Context, string) (source.LegacySource, error), opts ReportOptions) (*Report, error) {
	srcs, closeSources, err := OpenSources(ctx, p, open)
	defer closeSources()
	if err != nil {
		return nil, err
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, fmt.Errorf("begin read only: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if opts.Profile == "" {
		opts.Profile = p.Name
	}
	return BuildReport(ctx, srcs, tx, opts)
}
