package legacyfixture

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	createRe = regexp.MustCompile(`^CREATE TABLE (legacy_(?:hub|wh)\.[a-z_]+) \($`)
	insertRe = regexp.MustCompile(`^INSERT INTO (legacy_(?:hub|wh)\.[a-z_]+) \(`)
	// Any schema-qualified reference outside the legacy schemas.
	foreignRe = regexp.MustCompile(`(?i)\b(public|pg_temp)\.`)
)

// scanFixture statically parses the SQL files: CREATE TABLE names and the
// number of VALUES tuples (one per line) inserted into each table.
func scanFixture(t *testing.T) (tables []string, rows map[string]int64) {
	t.Helper()
	dir, err := TestdataDir()
	if err != nil {
		t.Fatal(err)
	}
	rows = map[string]int64{}
	for _, name := range files {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		current := ""
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(strings.TrimSpace(line), "--") {
				continue
			}
			if foreignRe.MatchString(line) {
				t.Errorf("%s references a non-legacy schema: %q", name, line)
			}
			if m := createRe.FindStringSubmatch(line); m != nil {
				tables = append(tables, m[1])
				continue
			}
			if m := insertRe.FindStringSubmatch(line); m != nil {
				current = m[1]
				continue
			}
			if current != "" && strings.HasPrefix(line, "    (") {
				rows[current]++
				if strings.HasSuffix(line, ");") {
					current = ""
				}
			}
		}
		if err := sc.Err(); err != nil {
			t.Fatalf("scan %s: %v", name, err)
		}
		_ = f.Close()
	}
	return tables, rows
}

// TestFixtureFilesMatchExpectedCounts runs without a database: the SQL files
// declare exactly the tables in ExpectedRowCounts and seed the expected
// number of rows into each.
func TestFixtureFilesMatchExpectedCounts(t *testing.T) {
	tables, rows := scanFixture(t)
	want := make([]string, 0, len(ExpectedRowCounts))
	for k := range ExpectedRowCounts {
		want = append(want, k)
	}
	slices.Sort(want)
	got := slices.Clone(tables)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("fixture tables mismatch\n got: %v\nwant: %v", got, want)
	}
	for table, n := range ExpectedRowCounts {
		if rows[table] != n {
			t.Errorf("%s: fixture seeds %d rows, expected %d", table, rows[table], n)
		}
	}
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func publicTables(ctx context.Context, t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT c.relname FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'v', 'm')
		ORDER BY c.relname`)
	if err != nil {
		t.Fatalf("list public tables: %v", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("collect public tables: %v", err)
	}
	return names
}

// snapshot returns row count and a content digest per fixture table.
func snapshot(ctx context.Context, t *testing.T, pool *pgxpool.Pool) (map[string]int64, map[string]string) {
	t.Helper()
	counts := map[string]int64{}
	digests := map[string]string{}
	for table := range ExpectedRowCounts {
		var n int64
		var digest string
		err := pool.QueryRow(ctx,
			"SELECT count(*), md5(coalesce(string_agg(x::text, '|' ORDER BY x::text), '')) FROM "+table+" x",
		).Scan(&n, &digest)
		if err != nil {
			t.Fatalf("snapshot %s: %v", table, err)
		}
		counts[table] = n
		digests[table] = digest
	}
	return counts, digests
}

func legacyTables(ctx context.Context, t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT table_schema || '.' || table_name FROM information_schema.tables
		WHERE table_schema IN ($1, $2) ORDER BY 1`, HubSchema, WHSchema)
	if err != nil {
		t.Fatalf("list legacy tables: %v", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("collect legacy tables: %v", err)
	}
	return names
}

// TestLoadLegacyFixture is the TEC-253 acceptance test against CI PG18.
func TestLoadLegacyFixture(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	publicBefore := publicTables(ctx, t, pool)

	// First load in an explicit transaction so the per-transaction statistics
	// can prove that nothing in public was inserted, updated or deleted.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := Load(ctx, tx); err != nil {
		t.Fatalf("first load: %v", err)
	}
	var publicWrites int64
	if err := tx.QueryRow(ctx, `
		SELECT coalesce(sum(n_tup_ins + n_tup_upd + n_tup_del), 0)::bigint
		FROM pg_stat_xact_user_tables WHERE schemaname = 'public'`).Scan(&publicWrites); err != nil {
		t.Fatalf("xact stats: %v", err)
	}
	if publicWrites != 0 {
		t.Errorf("fixture load wrote %d rows into public, want 0", publicWrites)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	want := make([]string, 0, len(ExpectedRowCounts))
	for k := range ExpectedRowCounts {
		want = append(want, k)
	}
	slices.Sort(want)
	if got := legacyTables(ctx, t, pool); !slices.Equal(got, want) {
		t.Fatalf("legacy tables mismatch\n got: %v\nwant: %v", got, want)
	}

	counts1, digests1 := snapshot(ctx, t, pool)
	for table, n := range ExpectedRowCounts {
		if counts1[table] != n {
			t.Errorf("%s: %d rows, want %d", table, counts1[table], n)
		}
	}

	// Second load: same tables, same rows, same content.
	LoadLegacyFixture(t, pool)
	counts2, digests2 := snapshot(ctx, t, pool)
	for table := range ExpectedRowCounts {
		if counts2[table] != counts1[table] {
			t.Errorf("%s: second load %d rows, first %d", table, counts2[table], counts1[table])
		}
		if digests2[table] != digests1[table] {
			t.Errorf("%s: content changed on second load", table)
		}
	}

	if publicAfter := publicTables(ctx, t, pool); !slices.Equal(publicAfter, publicBefore) {
		t.Errorf("public relations changed\nbefore: %v\n after: %v", publicBefore, publicAfter)
	}

	// Scenario spot checks the migrator tests rely on.
	checks := []struct {
		name string
		sql  string
		want int64
	}{
		{"barcode collides hub/wh", `SELECT count(*) FROM legacy_hub.stock_items s JOIN legacy_wh.product_barcodes b ON b.code = s.barcode`, 1},
		{"order external_reference matches", `SELECT count(*) FROM legacy_hub.orders o JOIN legacy_wh.orders w ON w.external_reference = o.external_reference`, 1},
		{"product sku matches", `SELECT count(*) FROM legacy_hub.products p JOIN legacy_wh.products w ON w.sku = p.sku`, 1},
		{"VIN-less measurement report", `SELECT count(*) FROM legacy_hub.nexptg_reports WHERE vin IS NULL`, 1},
		{"duplicate warranty pair", `SELECT count(*) FROM (SELECT 1 FROM legacy_hub.warranties GROUP BY service_id, stock_item_id HAVING count(*) > 1) d`, 1},
		{"customer served at two dealers", `SELECT count(*) FROM (SELECT customer_id FROM legacy_hub.services GROUP BY customer_id HAVING count(DISTINCT dealer_id) > 1) d`, 1},
		{"phoneless customer", `SELECT count(*) FROM legacy_hub.customers WHERE phone = ''`, 1},
	}
	for _, c := range checks {
		var got int64
		if err := pool.QueryRow(ctx, c.sql).Scan(&got); err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}
