package source

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
)

// sessionReadOnly is run on every new MariaDB connection.
const sessionReadOnly = "SET SESSION TRANSACTION READ ONLY"

// MariaDB is a LegacySource over the legacy hub / warehouse MariaDB
// (go-sql-driver/mysql). Each connection is put in read-only session mode and
// each query runs in its own START TRANSACTION READ ONLY.
type MariaDB struct {
	name string
	db   *sql.DB
}

// OpenMariaDB opens a read-only MariaDB source. dsn is a go-sql-driver DSN
// (user:pass@tcp(host:3306)/dbname). Multi-statements and client-side
// interpolation are forced off; parseTime is forced on.
func OpenMariaDB(ctx context.Context, name, dsn string) (*MariaDB, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("source %s: parse dsn: %w", name, err)
	}
	cfg.MultiStatements = false
	cfg.InterpolateParams = false
	cfg.ParseTime = true
	if cfg.Loc == nil {
		cfg.Loc = time.UTC
	}
	base, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, fmt.Errorf("source %s: connector: %w", name, err)
	}
	db := sql.OpenDB(readOnlyConnector{base})
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("source %s: ping: %w", name, err)
	}
	return &MariaDB{name: name, db: db}, nil
}

// Name implements LegacySource.
func (m *MariaDB) Name() string { return m.name }

// Query implements LegacySource.
func (m *MariaDB) Query(ctx context.Context, query string, args ...any) (Rows, error) {
	if err := CheckReadOnly(query); err != nil {
		return nil, err
	}
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("source %s: begin read only: %w", m.name, err)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("source %s: query: %w", m.name, err)
	}
	return &sqlRows{Rows: rows, tx: tx}, nil
}

// TableExists implements LegacySource.
func (m *MariaDB) TableExists(ctx context.Context, table string) (bool, error) {
	if !validTableName(table) {
		return false, ErrBadTableName
	}
	rows, err := m.Query(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", table)
	if err != nil {
		return false, err
	}
	return countExists(rows)
}

// Close implements LegacySource.
func (m *MariaDB) Close() error { return m.db.Close() }

// readOnlyConnector puts every new connection in read-only session mode, so a
// statement outside the per-query transaction is still refused by MariaDB.
type readOnlyConnector struct{ driver.Connector }

func (c readOnlyConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	ex, ok := conn.(driver.ExecerContext)
	if !ok {
		_ = conn.Close()
		return nil, errors.New("source: driver cannot set read-only session")
	}
	if _, err := ex.ExecContext(ctx, sessionReadOnly, nil); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("source: %s: %w", sessionReadOnly, err)
	}
	return conn, nil
}

type sqlRows struct {
	*sql.Rows
	tx *sql.Tx
}

func (r *sqlRows) Close() error {
	err := r.Rows.Close()
	if rbErr := r.tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) && err == nil {
		err = rbErr
	}
	return err
}

func countExists(rows Rows) (bool, error) {
	defer func() { _ = rows.Close() }()
	var n int64
	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			return false, err
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return n > 0, nil
}
