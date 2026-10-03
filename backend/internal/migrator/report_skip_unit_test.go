package migrator

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
)

// F2-FIX-2 (TEC-264): every skip the olex steps report carries the legacy id
// under a prefix of its report table, so the report counts it as skipped.

// reportSpec returns the olex report table source.table.
func reportSpec(t *testing.T, src, table string) ReportTable {
	t.Helper()
	for _, tb := range OlexReportTables() {
		if tb.Source == src && tb.Table == table {
			return tb
		}
	}
	t.Fatalf("no report table %s.%s", src, table)
	return ReportTable{}
}

func TestOlexSkipKeysMatchReportPrefixes(t *testing.T) {
	const whID = "0000000c-0000-4000-8000-000000000001"
	for _, c := range []struct {
		src, table, key, id string
	}{
		{SourceHub, "service_images", "image_skipped_missing:21", "21"},
		{SourceHub, "service_images", "image_skipped_too_large:21", "21"},
		{SourceHub, "service_images", "image_skipped_unsupported:21", "21"},
		{SourceHub, "service_images", "image_skipped_no_storage:21", "21"},
		{SourceHub, "service_images", "image_skipped_service_unmapped:21", "21"},
		{SourceHub, "service_items", "item_skipped_service_unmapped:11", "11"},
		{SourceHub, "service_status_logs", "log_skipped_service_unmapped:31", "31"},
		{SourceHub, "warranties", "warranty_skipped_service_not_completed:5", "5"},
		{SourceHub, "orders", "order_skipped_status:hub:3", "3"},
		{SourceWH, "orders", "order_skipped_buyer_unmapped:wh:" + whID, whID},
		{SourceHub, "order_items", "lines_skipped_order_parties:hub:4", "4"},
		{SourceWH, "order_items", "lines_skipped_brand_unknown:wh:" + whID, whID},
		{SourceHub, "stock_items", "hub_skipped_state:6", "6"},
		{SourceHub, "stock_items", "hub_skipped_roll_meter:6", "6"},
		{SourceWH, "product_barcodes", "wh_skipped_status:" + whID, whID},
		{SourceWH, "product_barcodes", "wh_skipped_brand_unknown:" + whID, whID},
		{SourceWH, "product_barcodes", "wh_skipped_roll_meter:" + whID, whID},
		{SourceWH, "products", "wh_product_skipped_unmatched:" + whID, whID},
		{SourceWH, "products", "wh_product_skipped_brand_unknown:" + whID, whID},
		{SourceHub, "short_urls", "invalid_token:2", "2"},
	} {
		spec := reportSpec(t, c.src, c.table)
		got := skipReasons([]string{c.key}, spec.SkipPrefixes)
		if _, ok := got[c.id]; !ok || len(got) != 1 {
			t.Errorf("%s.%s: %q -> %v, want id %s", c.src, c.table, c.key, got, c.id)
		}
	}
	// Keys without a row id do not explain a row.
	for _, c := range []struct{ src, table, key string }{
		{SourceHub, "service_images", "image_missing:21"},
		{SourceHub, "service_items", "children_skipped_service_unmapped:2"},
		{SourceHub, "stock_items", "hub_state_unknown:dealer/lost"},
		{SourceWH, "product_barcodes", "wh_status_unknown:lost"},
		{SourceHub, "products", "wh_product_skipped_unmatched:7"},
	} {
		spec := reportSpec(t, c.src, c.table)
		if got := skipReasons([]string{c.key}, spec.SkipPrefixes); len(got) != 0 {
			t.Errorf("%s.%s: %q -> %v, want no match", c.src, c.table, c.key, got)
		}
	}
}

// Children of a skipped service are reported one by one.
func TestServiceChildrenOfSkippedServiceReportedByID(t *testing.T) {
	r := &serviceRun{c: counts{}}
	err := r.children(context.Background(), 2, nil,
		[]legacyServiceItem{{ID: 11, ServiceID: 2}, {ID: 12, ServiceID: 2}},
		[]legacyServiceImage{{ID: 21, ServiceID: 2}},
		[]legacyStatusLog{{ID: 31, ServiceID: 2}})
	if err != nil {
		t.Fatal(err)
	}
	want := counts{
		"item_skipped_service_unmapped": 2, "item_skipped_service_unmapped:11": 1, "item_skipped_service_unmapped:12": 1,
		"image_skipped_service_unmapped": 1, "image_skipped_service_unmapped:21": 1,
		"log_skipped_service_unmapped": 1, "log_skipped_service_unmapped:31": 1,
		"children_skipped_service_unmapped:2": 1,
	}
	if !reflect.DeepEqual(r.c, want) {
		t.Errorf("counts = %v, want %v", r.c, want)
	}
}

// A skipped warranty group reports every row of the group by id; the total
// counts groups.
func TestWarrantySkipGroupReportsEveryRow(t *testing.T) {
	r := &warrantyRun{c: counts{}}
	g := warrantyGroup{Kept: legacyWarranty{ID: 5}, All: []legacyWarranty{{ID: 5}, {ID: 6}}}
	r.skipGroup("warranty_skipped_service_not_completed", g)
	want := counts{"warranty_skipped_service_not_completed": 1,
		"warranty_skipped_service_not_completed:5": 1, "warranty_skipped_service_not_completed:6": 1}
	if !reflect.DeepEqual(r.c, want) {
		t.Errorf("counts = %v, want %v", r.c, want)
	}
}

// A skipped order reports both legacy sides and, unless the lines were
// reported on their own, every line.
func TestSkipOrderReportsSidesAndLines(t *testing.T) {
	l := &legacyOrders{
		hubItems: map[int64][]hubOrderItem{3: {{ID: 30, OrderID: 3}, {ID: 31, OrderID: 3}}},
		whItems:  map[string][]whOrderItem{"w1": {{ID: "wi1", OrderID: "w1"}}},
	}
	cand := &orderCandidate{Hub: &hubOrder{ID: 3}, WH: &whOrder{ID: "w1"}}
	c := counts{}
	skipOrder(c, l, cand, "parties", true)
	want := counts{"order_skipped_parties": 1, "order_skipped_parties:hub:3": 1, "order_skipped_parties:wh:w1": 1,
		"lines_skipped_order_parties:hub:30": 1, "lines_skipped_order_parties:hub:31": 1,
		"lines_skipped_order_parties:wh:wi1": 1}
	if !reflect.DeepEqual(c, want) {
		t.Errorf("counts = %v, want %v", c, want)
	}
	c = counts{}
	skipOrder(c, l, cand, "no_lines", false)
	want = counts{"order_skipped_no_lines": 1, "order_skipped_no_lines:hub:3": 1, "order_skipped_no_lines:wh:w1": 1}
	if !reflect.DeepEqual(c, want) {
		t.Errorf("no_lines counts = %v, want %v", c, want)
	}
}

// BuildReport over in-memory sources: a skipped service with an item, an
// image and a log, and an image of a migrated service whose file is
// missing, are skipped (exit 0); with strict they are mismatches.
func TestBuildReportCountsServiceSkips(t *testing.T) {
	ctx := context.Background()
	// The service step's report keys, as a run writes them.
	r := &serviceRun{c: counts{}}
	r.report("service_skipped_dealer_unmapped", 2)
	if err := r.children(ctx, 2, nil, []legacyServiceItem{{ID: 11, ServiceID: 2}},
		[]legacyServiceImage{{ID: 21, ServiceID: 2}}, []legacyStatusLog{{ID: 31, ServiceID: 2}}); err != nil {
		t.Fatal(err)
	}
	r.report("image_skipped_missing", 22) // image of the migrated service 1
	var keys []string
	for k := range r.c {
		keys = append(keys, k)
	}

	src := fakeSource{rows: map[string][]any{
		"services":            {int64(1), int64(2)},
		"service_items":       {int64(10), int64(11)},
		"service_images":      {int64(20), int64(21), int64(22)},
		"service_status_logs": {int64(30), int64(31)},
	}}
	maps := map[string][]string{
		"services": {"1"}, "service_items": {"10"}, "service_images": {"20"}, "service_status_logs": {"30"},
	}
	q := &fakeReportQuerier{steps: map[string][]string{"services": keys}, maps: maps}
	var tables []ReportTable
	for _, name := range []string{"services", "service_items", "service_images", "service_status_logs"} {
		tables = append(tables, reportSpec(t, SourceHub, name))
	}

	rep, err := BuildReport(ctx, Sources{SourceHub: src}, q, ReportOptions{Tables: tables})
	if err != nil {
		t.Fatal(err)
	}
	if err := rep.Err(); err != nil {
		t.Fatalf("report: %v: %+v", err, rep.Mismatches)
	}
	for table, skipped := range map[string]int64{
		"services": 1, "service_items": 1, "service_images": 2, "service_status_logs": 1,
	} {
		tr, _ := rep.Table(SourceHub, table)
		if tr.Skipped != skipped || tr.Mismatches != 0 || tr.Migrated+tr.Skipped != tr.SourceRows {
			t.Errorf("%s = %+v, want skipped %d", table, tr, skipped)
		}
	}
	var got []string
	for _, d := range rep.Expected {
		got = append(got, d.Table+":"+d.ID+":"+d.Reason)
	}
	sort.Strings(got)
	wantExp := []string{
		"service_images:21:image_skipped_service_unmapped", "service_images:22:image_skipped_missing",
		"service_items:11:item_skipped_service_unmapped", "service_status_logs:31:log_skipped_service_unmapped",
		"services:2:service_skipped_dealer_unmapped",
	}
	if !reflect.DeepEqual(got, wantExp) {
		t.Errorf("expected = %v, want %v", got, wantExp)
	}

	strict, err := BuildReport(ctx, Sources{SourceHub: src}, q, ReportOptions{Tables: tables, Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := strict.Err(); !errors.Is(err, ErrReportMismatch) || len(strict.Mismatches) != 5 {
		t.Fatalf("strict report = %v, %+v; want 5 mismatches", err, strict.Mismatches)
	}
	for _, d := range strict.Mismatches {
		if d.Class != ClassSkipped {
			t.Errorf("strict mismatch %+v, want class skipped", d)
		}
	}
}

// fakeSource is a legacy source of id columns.
type fakeSource struct{ rows map[string][]any }

func (fakeSource) Name() string { return SourceHub }
func (s fakeSource) Query(_ context.Context, query string, _ ...any) (source.Rows, error) {
	table := strings.TrimSpace(query[strings.LastIndex(query, " FROM ")+len(" FROM "):])
	vals, ok := s.rows[table]
	if !ok {
		return nil, fmt.Errorf("fake source: unknown query %q", query)
	}
	out := make([][]any, len(vals))
	for i, v := range vals {
		out[i] = []any{v}
	}
	return fakeSourceRows{&fakeRows{rows: out}}, nil
}
func (s fakeSource) TableExists(_ context.Context, table string) (bool, error) {
	_, ok := s.rows[table]
	return ok, nil
}
func (fakeSource) Close() error { return nil }

// fakeReportQuerier answers the report's two queries: the step keys and the
// migration_map rows of a table (every mapped target is live, unshared).
type fakeReportQuerier struct {
	steps map[string][]string
	maps  map[string][]string // table -> mapped source ids
}

func (q *fakeReportQuerier) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	var out [][]any
	switch {
	case strings.Contains(sql, "jsonb_object_keys"):
		for step, keys := range q.steps {
			for _, k := range keys {
				if strings.Contains(k, ":") {
					out = append(out, []any{step, k})
				}
			}
		}
	case strings.Contains(sql, "FROM migration_map mm"):
		for _, id := range q.maps[args[1].(string)] {
			out = append(out, []any{id, uuid.New(), true, []string{}})
		}
	default:
		return nil, fmt.Errorf("fake querier: unknown query %q", sql)
	}
	return &fakeRows{rows: out}, nil
}

// fakeRows is a result set of in-memory rows (pgx.Rows and source.Rows).
type fakeRows struct {
	rows [][]any
	i    int
}

func (r *fakeRows) Next() bool { r.i++; return r.i <= len(r.rows) }
func (r *fakeRows) Scan(dest ...any) error {
	row := r.rows[r.i-1]
	if len(dest) != len(row) {
		return fmt.Errorf("fake rows: scan %d into %d", len(row), len(dest))
	}
	for i, d := range dest {
		reflect.ValueOf(d).Elem().Set(reflect.ValueOf(row[i]))
	}
	return nil
}
func (r *fakeRows) Err() error                                   { return nil }
func (r *fakeRows) Columns() ([]string, error)                   { return nil, nil }
func (r *fakeRows) Close()                                       {}
func (r *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeRows) Values() ([]any, error)                       { return r.rows[r.i-1], nil }
func (r *fakeRows) RawValues() [][]byte                          { return nil }
func (r *fakeRows) Conn() *pgx.Conn                              { return nil }

// fakeSourceRows adapts fakeRows to source.Rows (Close returns an error).
type fakeSourceRows struct{ *fakeRows }

func (r fakeSourceRows) Close() error { return nil }
