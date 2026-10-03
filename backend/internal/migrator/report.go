package migrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
)

// Validation report (TEC-264). It checks the legacy -> new direction: every
// row of a migrated legacy table is accounted for, either by a live target
// row (via migration_map) or by an expected difference. The cutover delta
// report (TEC-276, DeltaReport) is the opposite direction: rows born in this
// application that no legacy row maps to. Both read migration_map through
// the same DeltaQuerier, and the command runs this one in a READ ONLY
// transaction too.
//
// Every source row lands in exactly one class:
//
//   - migrated: mapped, and the target row exists;
//   - merged (expected): mapped onto a target another legacy row maps to as
//     well, a planned many-to-one (K26 same person, duplicate warranties,
//     hub/warehouse orders by external_reference, measurement rows kept in
//     the result's raw JSONB, ...);
//   - out_of_scope (expected): rows the profile does not migrate (Glorian,
//     K2; rows deleted in the warehouse);
//   - skipped (expected unless strict): rows a step skipped and reported
//     with their id in its migration_runs counts (e.g. a warranty of a
//     service that is not completed, a short URL to an external site);
//   - mismatch: not_migrated (no mapping, no reason), target_missing (the
//     mapped target row is gone), source_missing (a mapped legacy row no
//     longer exists), source_table_missing.
//
// Any mismatch makes Report.Err non-nil (the command exits non-zero).

// Report classes.
const (
	ClassMerged     = "merged"
	ClassOutOfScope = "out_of_scope"
	ClassSkipped    = "skipped"

	MismatchNotMigrated   = "not_migrated"
	MismatchTargetMissing = "target_missing"
	MismatchSourceMissing = "source_missing"
	MismatchTableMissing  = "source_table_missing"
)

// ErrReportMismatch is returned by Report.Err when a row is unaccounted for.
var ErrReportMismatch = errors.New("migrator: report has mismatches")

// ReportRule selects the ids of source rows that are out of the profile's
// scope. Query is a read-only SELECT of one id column on the source.
type ReportRule struct {
	Reason string
	Query  string
}

// ReportTable is one legacy table the report checks.
type ReportTable struct {
	Source      string // SourceHub / SourceWH
	Table       string // legacy table (migration_map.source_table)
	TargetTable string // migration_map.target_table
	// IDExpr selects the legacy id as the steps key it; empty means "id".
	IDExpr string
	// TargetExists is a SQL boolean over the migration_map row "mm"; empty
	// means a row of TargetTable with uuid = mm.target_uuid.
	TargetExists string
	// MergeReason labels many-to-one mappings. A shared target counts as a
	// merge when another row of this table, or of a MergeWith table, maps to
	// it; Folded tables are always merged into another table's target.
	MergeReason string
	MergeWith   []ReportTableRef
	Folded      bool
	// OutOfScope rules, then the skip reports: counter keys of Step that
	// start with one of SkipPrefixes and end in ":<id>".
	OutOfScope   []ReportRule
	Step         string
	SkipPrefixes []string
}

// ReportTableRef names another legacy table.
type ReportTableRef struct{ Source, Table string }

// whGlorian selects warehouse rows of Glorian products (K2), with the same
// brand rule as whBrand.
func whGlorian(from, productCol string) ReportRule {
	return ReportRule{Reason: "glorian", Query: `SELECT CAST(x.id AS CHAR(36)) FROM ` + from + ` x
JOIN products p ON p.id = x.` + productCol + `
JOIN brands b ON b.id = p.brand_id
WHERE LOWER(b.name) LIKE '%glorian%'`}
}

// whCast is the warehouse id expression (uuid columns, CHAR(36) on MariaDB).
const whCast = "CAST(id AS CHAR(36))"

// OlexReportTables is the report's view of the olex profile: the legacy
// tables its steps map row by row. Tables the steps fold in without a
// mapping of their own (roles, order_item_stock, bin_product_stocks, ...)
// and synthetic keys (services.vehicle, services.warranty_number) are not
// listed.
func OlexReportTables() []ReportTable {
	hub := func(table string) ReportTableRef { return ReportTableRef{Source: SourceHub, Table: table} }
	wh := func(table string) ReportTableRef { return ReportTableRef{Source: SourceWH, Table: table} }
	return []ReportTable{
		{Source: SourceHub, Table: "dealers", TargetTable: "organizations", Step: "organizations"},
		{Source: SourceHub, Table: "users", TargetTable: "users", Step: "users",
			MergeReason: "same_person", MergeWith: []ReportTableRef{hub("customers")},
			SkipPrefixes: []string{"skipped_no_contact:"}},
		{Source: SourceHub, Table: "customers", TargetTable: "users", Step: "customers",
			MergeReason: "k26_same_person", MergeWith: []ReportTableRef{hub("users")}},
		{Source: SourceHub, Table: "car_brands", TargetTable: "car_brands", Step: "vehicle_catalog",
			SkipPrefixes: []string{"brand_skipped_"}},
		{Source: SourceHub, Table: "car_models", TargetTable: "car_models", Step: "vehicle_catalog",
			SkipPrefixes: []string{"model_skipped_"}},
		{Source: SourceHub, Table: "product_categories", TargetTable: "product_categories", Step: "catalog",
			SkipPrefixes: []string{"category_skipped_"}},
		{Source: SourceHub, Table: "products", TargetTable: "products", Step: "catalog",
			MergeReason: "sku_match", MergeWith: []ReportTableRef{wh("products")},
			SkipPrefixes: []string{"product_skipped_"}},
		{Source: SourceHub, Table: "stock_items", TargetTable: "units", Step: "units",
			MergeReason: "barcode_match", MergeWith: []ReportTableRef{wh("product_barcodes")},
			SkipPrefixes: []string{"hub_skipped_"}},
		{Source: SourceHub, Table: "stock_movements", TargetTable: "stock_movements", Step: "ledger"},
		{Source: SourceHub, Table: "orders", TargetTable: "orders", Step: "orders",
			MergeReason: "external_reference", MergeWith: []ReportTableRef{wh("orders")},
			SkipPrefixes: []string{"order_skipped_"}},
		{Source: SourceHub, Table: "order_items", TargetTable: "order_items", Step: "orders",
			MergeReason: "external_reference", MergeWith: []ReportTableRef{wh("order_items")},
			SkipPrefixes: []string{"lines_skipped_"}},
		{Source: SourceHub, Table: "services", TargetTable: "services", Step: "services",
			SkipPrefixes: []string{"service_skipped_"}},
		{Source: SourceHub, Table: "service_items", TargetTable: "service_items", Step: "services",
			SkipPrefixes: []string{"item_skipped_"}},
		{Source: SourceHub, Table: "service_images", TargetTable: "service_images", Step: "services",
			SkipPrefixes: []string{"image_skipped_"}},
		// Status logs carry no uuid; the migrated log keeps its legacy id in
		// metadata (TEC-259: the legacy log becomes a note on the service).
		{Source: SourceHub, Table: "service_status_logs", TargetTable: "service_status_logs", Step: "services",
			TargetExists: `EXISTS (SELECT 1 FROM service_status_logs t
				WHERE t.metadata->>'source' = 'legacy_hub.service_status_logs' AND t.metadata->>'legacy_id' = mm.source_id)`,
			SkipPrefixes: []string{"log_skipped_"}},
		{Source: SourceHub, Table: "warranties", TargetTable: "warranties", Step: "warranties",
			MergeReason: "duplicate_warranty", SkipPrefixes: []string{"warranty_skipped_"}},
		{Source: SourceHub, Table: "service_customer_transfers", TargetTable: "vehicle_transfers", Step: "warranties",
			SkipPrefixes: []string{"transfer_skipped_"}},
		{Source: SourceHub, Table: "nexptg_api_users", TargetTable: "measurement_devices", Step: "measurements",
			SkipPrefixes: []string{"device_skipped_"}},
		{Source: SourceHub, Table: "nexptg_reports", TargetTable: "measurement_results", Step: "measurements",
			SkipPrefixes: []string{"report_skipped_"}},
		{Source: SourceHub, Table: "nexptg_report_measurements", TargetTable: "measurement_results", Step: "measurements",
			MergeReason: "measurement_rows_in_raw", Folded: true},
		{Source: SourceHub, Table: "service_nexptg_report", TargetTable: "measurement_results", Step: "measurements",
			MergeReason: "service_link_in_raw", Folded: true},
		{Source: SourceHub, Table: "short_urls", TargetTable: "short_urls", Step: "short_urls",
			SkipPrefixes: []string{ShortTargetUnmapped + ":", ShortTargetInvalid + ":", "token_conflict:", "invalid_token:"}},
		{Source: SourceHub, Table: "sms_logs", TargetTable: "legacy_messages", Step: "legacy_messages"},
		{Source: SourceHub, Table: "notifications", TargetTable: "legacy_messages", Step: "legacy_messages"},

		{Source: SourceWH, Table: "warehouses", TargetTable: "warehouses", Step: "warehouses", IDExpr: whCast,
			SkipPrefixes: []string{"warehouse_skipped_"}},
		{Source: SourceWH, Table: "warehouse_sites", TargetTable: "rooms", Step: "warehouses", IDExpr: whCast,
			SkipPrefixes: []string{"room_skipped_"}},
		{Source: SourceWH, Table: "warehouse_locations", TargetTable: "warehouse_locations", Step: "warehouses",
			IDExpr: whCast, SkipPrefixes: []string{"location_skipped_"}},
		{Source: SourceWH, Table: "products", TargetTable: "products", Step: "catalog", IDExpr: whCast,
			MergeReason: "sku_match", MergeWith: []ReportTableRef{hub("products")},
			OutOfScope: []ReportRule{
				{Reason: "glorian", Query: `SELECT CAST(p.id AS CHAR(36)) FROM products p
JOIN brands b ON b.id = p.brand_id WHERE LOWER(b.name) LIKE '%glorian%'`},
			},
			SkipPrefixes: []string{"wh_product_skipped_"}},
		{Source: SourceWH, Table: "product_barcodes", TargetTable: "units", Step: "units", IDExpr: whCast,
			MergeReason: "barcode_match", MergeWith: []ReportTableRef{hub("stock_items")},
			OutOfScope: []ReportRule{
				whGlorian("product_barcodes", "product_id"),
				{Reason: "deleted", Query: `SELECT CAST(id AS CHAR(36)) FROM product_barcodes WHERE deleted_at IS NOT NULL`},
			},
			SkipPrefixes: []string{"wh_skipped_"}},
		{Source: SourceWH, Table: "stock_movements", TargetTable: "stock_movements", Step: "ledger", IDExpr: whCast,
			OutOfScope: []ReportRule{whGlorian("stock_movements", "product_id")}},
		{Source: SourceWH, Table: "orders", TargetTable: "orders", Step: "orders", IDExpr: whCast,
			MergeReason: "external_reference", MergeWith: []ReportTableRef{hub("orders")},
			OutOfScope: []ReportRule{
				{Reason: "deleted", Query: `SELECT CAST(id AS CHAR(36)) FROM orders WHERE deleted_at IS NOT NULL`},
			},
			SkipPrefixes: []string{"order_skipped_"}},
		{Source: SourceWH, Table: "order_items", TargetTable: "order_items", Step: "orders", IDExpr: whCast,
			MergeReason: "external_reference", MergeWith: []ReportTableRef{hub("order_items")},
			OutOfScope: []ReportRule{
				whGlorian("order_items", "product_id"),
				{Reason: "order_deleted", Query: `SELECT CAST(i.id AS CHAR(36)) FROM order_items i
JOIN orders o ON o.id = i.order_id WHERE o.deleted_at IS NOT NULL`},
			},
			SkipPrefixes: []string{"lines_skipped_"}},
	}
}

// ReportOptions are the parameters of BuildReport.
type ReportOptions struct {
	// Profile is the migration_runs profile whose step reports explain
	// skipped rows (default "olex").
	Profile string
	// Systems maps a source name to its migration_map source_system
	// (default: the source name itself; tests use their own systems).
	Systems map[string]string
	// Tables defaults to OlexReportTables.
	Tables []ReportTable
	// Strict reports skipped rows as mismatches.
	Strict bool
}

// Report is the validation report.
type Report struct {
	Profile     string        `json:"profile"`
	Strict      bool          `json:"strict"`
	GeneratedAt time.Time     `json:"generated_at"`
	Tables      []TableReport `json:"tables"`
	// Expected lists the expected differences, Mismatches the rest.
	Expected   []ReportDiff `json:"expected"`
	Mismatches []ReportDiff `json:"mismatches"`
}

// TableReport is the count line of one legacy table. SourceRows = Migrated +
// OutOfScope + Skipped + the table's not_migrated / target_missing rows;
// Merged is part of Migrated.
type TableReport struct {
	Source      string `json:"source"`
	Table       string `json:"table"`
	TargetTable string `json:"target_table"`
	SourceRows  int64  `json:"source_rows"`
	Migrated    int64  `json:"migrated"`
	// TargetRows is the distinct live target rows the table maps to.
	TargetRows int64 `json:"target_rows"`
	Merged     int64 `json:"merged"`
	OutOfScope int64 `json:"out_of_scope"`
	Skipped    int64 `json:"skipped"`
	Mismatches int64 `json:"mismatches"`
}

// ReportDiff is one expected difference or mismatch.
type ReportDiff struct {
	Source string `json:"source"`
	Table  string `json:"table"`
	ID     string `json:"id"`
	// Class is merged / out_of_scope / skipped or a mismatch kind.
	Class  string     `json:"class"`
	Reason string     `json:"reason,omitempty"`
	Target *uuid.UUID `json:"target_uuid,omitempty"`
}

// Err is ErrReportMismatch when the report has a mismatch.
func (r *Report) Err() error {
	if len(r.Mismatches) > 0 {
		return fmt.Errorf("%w: %d", ErrReportMismatch, len(r.Mismatches))
	}
	return nil
}

// Table returns the count line of source.table.
func (r *Report) Table(source, table string) (TableReport, bool) {
	for _, t := range r.Tables {
		if t.Source == source && t.Table == table {
			return t, true
		}
	}
	return TableReport{}, false
}

type mapRow struct {
	target uuid.UUID
	live   bool
	shares []string // "system.table" of the other rows mapped to the target
}

// BuildReport compares every table of opts.Tables between the legacy
// sources and migration_map / the target tables q reads.
func BuildReport(ctx context.Context, srcs Sources, q DeltaQuerier, opts ReportOptions) (*Report, error) {
	if opts.Profile == "" {
		opts.Profile = "olex"
	}
	if opts.Tables == nil {
		opts.Tables = OlexReportTables()
	}
	system := func(source string) string {
		if s, ok := opts.Systems[source]; ok && s != "" {
			return s
		}
		return source
	}
	skips, err := loadSkipKeys(ctx, q, opts.Profile)
	if err != nil {
		return nil, err
	}
	rep := &Report{Profile: opts.Profile, Strict: opts.Strict, GeneratedAt: time.Now().UTC(),
		Expected: []ReportDiff{}, Mismatches: []ReportDiff{}}
	for _, spec := range opts.Tables {
		tr, err := reportTable(ctx, srcs, q, spec, system, skips[spec.Step], opts.Strict, rep)
		if err != nil {
			return nil, fmt.Errorf("report %s.%s: %w", spec.Source, spec.Table, err)
		}
		rep.Tables = append(rep.Tables, tr)
	}
	return rep, nil
}

// loadSkipKeys returns, per step, the counter keys of the profile's
// successful non-dry runs.
func loadSkipKeys(ctx context.Context, q DeltaQuerier, profile string) (map[string][]string, error) {
	rows, err := q.Query(ctx, `SELECT DISTINCT r.step, k.key
		FROM migration_runs r CROSS JOIN LATERAL jsonb_object_keys(r.counts) AS k(key)
		WHERE r.profile = $1 AND r.step IS NOT NULL AND r.status = 'succeeded' AND NOT r.dry_run
		  AND jsonb_typeof(r.counts) = 'object' AND position(':' in k.key) > 0`, profile)
	if err != nil {
		return nil, fmt.Errorf("report: step counts: %w", err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var step, key string
		if err := rows.Scan(&step, &key); err != nil {
			return nil, err
		}
		out[step] = append(out[step], key)
	}
	return out, rows.Err()
}

// skipReasons maps a legacy id to the reason a step reported it with.
func skipReasons(keys, prefixes []string) map[string]string {
	out := map[string]string{}
	for _, k := range keys {
		i := strings.LastIndexByte(k, ':')
		if i <= 0 {
			continue
		}
		for _, p := range prefixes {
			if strings.HasPrefix(k, p) {
				if _, seen := out[k[i+1:]]; !seen {
					out[k[i+1:]] = k[:i]
				}
				break
			}
		}
	}
	return out
}

func reportTable(ctx context.Context, srcs Sources, q DeltaQuerier, spec ReportTable, system func(string) string,
	stepKeys []string, strict bool, rep *Report) (TableReport, error) {
	tr := TableReport{Source: spec.Source, Table: spec.Table, TargetTable: spec.TargetTable}
	diff := func(id, class, reason string, target *uuid.UUID) ReportDiff {
		return ReportDiff{Source: spec.Source, Table: spec.Table, ID: id, Class: class, Reason: reason, Target: target}
	}
	src, err := srcs.Get(spec.Source)
	if err != nil {
		return tr, err
	}
	ok, err := src.TableExists(ctx, spec.Table)
	if err != nil {
		return tr, err
	}
	if !ok {
		tr.Mismatches++
		rep.Mismatches = append(rep.Mismatches, diff("", MismatchTableMissing, "", nil))
		return tr, nil
	}

	idExpr := spec.IDExpr
	if idExpr == "" {
		idExpr = "id"
	}
	ids, err := sourceIDs(ctx, src, "SELECT "+idExpr+" FROM "+spec.Table)
	if err != nil {
		return tr, err
	}
	outOfScope := map[string]string{}
	for _, rule := range spec.OutOfScope {
		ruleIDs, err := sourceIDs(ctx, src, rule.Query)
		if err != nil {
			return tr, fmt.Errorf("rule %s: %w", rule.Reason, err)
		}
		for _, id := range ruleIDs {
			if _, seen := outOfScope[id]; !seen {
				outOfScope[id] = rule.Reason
			}
		}
	}
	skipped := skipReasons(stepKeys, spec.SkipPrefixes)

	sys := system(spec.Source)
	mapped, err := mapRows(ctx, q, spec, sys)
	if err != nil {
		return tr, err
	}
	mergeWith := map[string]bool{sys + "." + spec.Table: true}
	for _, ref := range spec.MergeWith {
		mergeWith[system(ref.Source)+"."+ref.Table] = true
	}
	merged := func(m mapRow) bool {
		if spec.Folded {
			return true
		}
		for _, s := range m.shares {
			if mergeWith[s] {
				return true
			}
		}
		return false
	}

	targets := map[uuid.UUID]bool{}
	inSource := make(map[string]bool, len(ids))
	tr.SourceRows = int64(len(ids))
	for _, id := range ids {
		inSource[id] = true
		if m, ok := mapped[id]; ok {
			target := m.target
			switch {
			case !m.live:
				tr.Mismatches++
				rep.Mismatches = append(rep.Mismatches, diff(id, MismatchTargetMissing, spec.TargetTable, &target))
			case merged(m):
				tr.Migrated++
				tr.Merged++
				targets[target] = true
				rep.Expected = append(rep.Expected, diff(id, ClassMerged, spec.MergeReason, &target))
			default:
				tr.Migrated++
				targets[target] = true
			}
			continue
		}
		if reason, ok := outOfScope[id]; ok {
			tr.OutOfScope++
			rep.Expected = append(rep.Expected, diff(id, ClassOutOfScope, reason, nil))
			continue
		}
		if reason, ok := skipped[id]; ok {
			tr.Skipped++
			d := diff(id, ClassSkipped, reason, nil)
			if strict {
				tr.Mismatches++
				rep.Mismatches = append(rep.Mismatches, d)
			} else {
				rep.Expected = append(rep.Expected, d)
			}
			continue
		}
		tr.Mismatches++
		rep.Mismatches = append(rep.Mismatches, diff(id, MismatchNotMigrated, "", nil))
	}
	gone := make([]string, 0)
	for id := range mapped {
		if !inSource[id] {
			gone = append(gone, id)
		}
	}
	sortIDs(gone)
	for _, id := range gone {
		target := mapped[id].target
		tr.Mismatches++
		rep.Mismatches = append(rep.Mismatches, diff(id, MismatchSourceMissing, "", &target))
	}
	tr.TargetRows = int64(len(targets))
	return tr, nil
}

// mapRows reads the migration_map rows of one legacy table with the
// liveness of their target and the other rows sharing it.
func mapRows(ctx context.Context, q DeltaQuerier, spec ReportTable, system string) (map[string]mapRow, error) {
	exists := spec.TargetExists
	if exists == "" {
		// TargetTable is a constant of the table list, never input.
		exists = "EXISTS (SELECT 1 FROM " + spec.TargetTable + " t WHERE t.uuid = mm.target_uuid)"
	}
	rows, err := q.Query(ctx, `SELECT mm.source_id, mm.target_uuid, `+exists+`,
		COALESCE((SELECT array_agg(DISTINCT o.source_system || '.' || o.source_table)
		          FROM migration_map o
		          WHERE o.target_table = mm.target_table AND o.target_uuid = mm.target_uuid AND o.id <> mm.id),
		         '{}'::text[])
		FROM migration_map mm
		WHERE mm.source_system = $1 AND mm.source_table = $2 AND mm.target_table = $3`,
		system, spec.Table, spec.TargetTable)
	if err != nil {
		return nil, fmt.Errorf("migration_map: %w", err)
	}
	defer rows.Close()
	out := map[string]mapRow{}
	for rows.Next() {
		var id string
		var m mapRow
		if err := rows.Scan(&id, &m.target, &m.live, &m.shares); err != nil {
			return nil, fmt.Errorf("migration_map scan: %w", err)
		}
		out[id] = m
	}
	return out, rows.Err()
}

// sourceIDs runs a one-column id query on a legacy source and returns the
// ids as the steps key them (decimal integers, trimmed text), sorted.
func sourceIDs(ctx context.Context, src source.LegacySource, query string) ([]string, error) {
	rows, err := src.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var v any
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		id, err := legacyID(v)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sortIDs(ids)
	return ids, nil
}

// sortIDs sorts numerically when every id is an integer, else as text.
func sortIDs(ids []string) {
	sort.Slice(ids, func(i, j int) bool {
		a, errA := strconv.ParseInt(ids[i], 10, 64)
		b, errB := strconv.ParseInt(ids[j], 10, 64)
		if errA == nil && errB == nil {
			return a < b
		}
		return ids[i] < ids[j]
	})
}

// legacyID formats a scanned id column like the steps key it.
func legacyID(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "", errors.New("null id")
	case int64:
		return strconv.FormatInt(x, 10), nil
	case int32:
		return strconv.FormatInt(int64(x), 10), nil
	case int:
		return strconv.Itoa(x), nil
	case uint64:
		return strconv.FormatUint(x, 10), nil
	case string:
		return strings.TrimSpace(x), nil
	case []byte:
		return strings.TrimSpace(string(x)), nil
	case [16]byte:
		return uuid.UUID(x).String(), nil
	case fmt.Stringer:
		return strings.TrimSpace(x.String()), nil
	}
	return "", fmt.Errorf("unsupported id type %T", v)
}

// WriteReportJSON writes the report as JSON.
func WriteReportJSON(w io.Writer, rep *Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// WriteReportText writes the human readable report: the count table, the
// expected differences grouped by class and reason, every mismatch.
func WriteReportText(w io.Writer, rep *Report) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "Migration report, profile %s, %s\n\n", rep.Profile, rep.GeneratedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintln(tw, "SOURCE TABLE\tTARGET\tSOURCE\tMIGRATED\tTARGETS\tMERGED\tOUT_OF_SCOPE\tSKIPPED\tMISMATCH")
	for _, t := range rep.Tables {
		_, _ = fmt.Fprintf(tw, "%s.%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n", t.Source, t.Table, t.TargetTable,
			t.SourceRows, t.Migrated, t.TargetRows, t.Merged, t.OutOfScope, t.Skipped, t.Mismatches)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	type group struct {
		key string
		ids []string
	}
	var groups []*group
	byKey := map[string]*group{}
	for _, d := range rep.Expected {
		k := d.Class + "\t" + d.Reason + "\t" + d.Source + "." + d.Table
		g, ok := byKey[k]
		if !ok {
			g = &group{key: k}
			byKey[k] = g
			groups = append(groups, g)
		}
		g.ids = append(g.ids, d.ID)
	}
	_, _ = fmt.Fprintf(w, "\nExpected differences (%d):\n", len(rep.Expected))
	tw = tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	for _, g := range groups {
		ids := g.ids
		more := ""
		if len(ids) > 10 {
			more = fmt.Sprintf(" ... +%d", len(ids)-10)
			ids = ids[:10]
		}
		_, _ = fmt.Fprintf(tw, "  %s\t%d\t%s%s\n", g.key, len(g.ids), strings.Join(ids, ","), more)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(w, "\nMismatches (%d):\n", len(rep.Mismatches))
	tw = tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	for _, d := range rep.Mismatches {
		target := ""
		if d.Target != nil {
			target = d.Target.String()
		}
		_, _ = fmt.Fprintf(tw, "  %s\t%s.%s\t%s\t%s\t%s\n", d.Class, d.Source, d.Table, d.ID, d.Reason, target)
	}
	return tw.Flush()
}
