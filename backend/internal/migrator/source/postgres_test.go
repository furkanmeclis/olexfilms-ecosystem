package source

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/legacyfixture"
)

func fixtureSource(t *testing.T, name string) (*Postgres, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	legacyfixture.LoadAndHold(t, pool)
	src, err := NewPostgres(name, pool, FixtureSchemas[name])
	if err != nil {
		t.Fatal(err)
	}
	return src, pool
}

func TestPostgresFixtureRead(t *testing.T) {
	ctx := context.Background()
	src, _ := fixtureSource(t, "hub")

	rows, err := src.Query(ctx, "SELECT id, phone FROM customers WHERE id >= ? ORDER BY id", 1)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	cols, _ := rows.Columns()
	if len(cols) != 2 || cols[0] != "id" || cols[1] != "phone" {
		t.Errorf("columns = %v", cols)
	}
	var n int64
	for rows.Next() {
		var id int64
		var phone string
		if err := rows.Scan(&id, &phone); err != nil {
			t.Fatalf("scan: %v", err)
		}
		n++
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if want := legacyfixture.ExpectedRowCounts["legacy_hub.customers"]; n != want {
		t.Errorf("rows = %d, want %d", n, want)
	}

	for table, want := range map[string]bool{"customers": true, "warranties": true, "warehouses": false, "nope": false} {
		got, err := src.TableExists(ctx, table)
		if err != nil {
			t.Fatalf("TableExists(%s): %v", table, err)
		}
		if got != want {
			t.Errorf("TableExists(%s) = %v, want %v", table, got, want)
		}
	}
	if _, err := src.TableExists(ctx, "customers; drop"); !errors.Is(err, ErrBadTableName) {
		t.Errorf("TableExists(bad) = %v", err)
	}

	wh, err := NewPostgres("wh", src.pool, FixtureSchemas["wh"])
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := wh.TableExists(ctx, "warehouses"); err != nil || !ok {
		t.Errorf("wh TableExists(warehouses) = %v, %v", ok, err)
	}
}

// Every attempt to write to the source fails: the guard refuses it, and with
// the guard bypassed the READ ONLY transaction makes Postgres refuse it
// (SQLSTATE 25006). The fixture is left unchanged.
func TestPostgresSourceRefusesWrites(t *testing.T) {
	ctx := context.Background()
	src, pool := fixtureSource(t, "hub")

	writes := []string{
		"INSERT INTO customers (id, created_by, type, name, phone) VALUES (999, 1, 'individual', 'x', '1')",
		"UPDATE customers SET name = 'changed'",
		"DELETE FROM customers",
		"CREATE TABLE intruder (id int)",
		"DROP TABLE customers",
		"TRUNCATE customers",
	}
	for _, q := range writes {
		if _, err := src.Query(ctx, q); !errors.Is(err, ErrNotReadOnly) {
			t.Errorf("Query(%q) = %v, want ErrNotReadOnly", q, err)
		}

		rows, err := src.query(ctx, q)
		if err == nil {
			for rows.Next() {
			}
			err = rows.Close()
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "25006" {
			t.Errorf("unguarded %q = %v, want SQLSTATE 25006 read_only_sql_transaction", q, err)
		}
	}

	var n int64
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM legacy_hub.customers WHERE name <> 'changed'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if want := legacyfixture.ExpectedRowCounts["legacy_hub.customers"]; n != want {
		t.Errorf("customers after write attempts = %d, want %d", n, want)
	}
}
