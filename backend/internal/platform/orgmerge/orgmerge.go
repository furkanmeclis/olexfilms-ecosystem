// Package orgmerge merges one organization into another (TEC-1003): every
// row that belongs to the source organization is moved to the target, the
// source members become target members with the matching center role, and
// the source is soft deleted with merged_into_id pointing at the target.
//
// A merge runs inside the caller's transaction. Each changed row is recorded
// in org_merge_changes, so Undo puts everything back; a dry run is a merge
// whose transaction the caller rolls back (the report is the same).
//
// Columns are discovered from the catalog, not listed by hand: every bigint
// column of a base table that references organizations or is named
// organization_id / *_org_id / *_organization_id. Composite foreign keys
// carry the organization column of parent and child rows, so the merge
// defers them (migration 000152 made them deferrable) and moves both sides
// before commit. Append-only ledgers accept the UPDATE while the
// transaction-local setting olex.org_merge is on (migration 000152); the
// merge turns it on for its own statements only and lists the rewritten
// ledgers in the report.
package orgmerge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// Errors.
var (
	ErrNotFound       = errors.New("orgmerge: organization not found")
	ErrInvalid        = errors.New("orgmerge: invalid merge")
	ErrAlreadyUndone  = errors.New("orgmerge: run is already undone")
	ErrBalanceDrift   = errors.New("orgmerge: balances differ before and after the merge")
	ErrNotDeferrable  = errors.New("orgmerge: a composite foreign key is not deferrable")
	errUniqueConflict = errors.New("unique conflict")
)

// Column is a table column ("orders.buyer_org_id").
type Column struct {
	Table  string `json:"table"`
	Column string `json:"column"`
}

func (c Column) String() string { return c.Table + "." + c.Column }

// ParseColumn reads "table.column".
func ParseColumn(s string) (Column, error) {
	t, c, ok := strings.Cut(strings.TrimSpace(s), ".")
	if !ok || t == "" || c == "" {
		return Column{}, fmt.Errorf("%w: column %q must be table.column", ErrInvalid, s)
	}
	return Column{Table: t, Column: c}, nil
}

// handled columns are moved by dedicated steps (or never).
var handled = map[string]bool{
	"organizations.merged_into_id":         true,
	"organization_members.organization_id": true,
	"territories.organization_id":          true,
	"org_merge_runs.source_org_id":         true,
	"org_merge_runs.target_org_id":         true,
}

// dealerOnly tables hold rows that only a dealer or distributor can own
// (their checks refuse a center): merged into a center they stay with the
// merged organization, as its history (TEC-1003).
var dealerOnly = map[string]bool{
	"dealer_showcases":            true,
	"dealer_showcase_photos":      true,
	"dealer_showcase_services":    true,
	"distributor_dealer_prices":   true,
	"distributor_price_overrides": true,
	"service_price_overrides":     true,
	"price_discipline_snapshots":  true,
	"performance_targets":         true,
}

// colSpec is a column to move. TypeCol is set for a polymorphic owner
// column: the id is an organization only when TypeCol is one of
// orgOwnerTypes.
type colSpec struct {
	Col     string
	TypeCol string
}

// orgOwnerTypes are the owner types whose id is an organization.
const orgOwnerTypes = "('organization', 'trash')"

// polymorphic lists the (owner type, owner id) pairs of the stock ledger
// and its projections.
var polymorphic = map[string][]colSpec{
	"unit_current_state":     {{Col: "owner_id", TypeCol: "owner_type"}},
	"fixed_barcode_holdings": {{Col: "owner_id", TypeCol: "owner_type"}},
	"stock_movements":        {{Col: "from_owner_id", TypeCol: "from_owner_type"}, {Col: "to_owner_id", TypeCol: "to_owner_type"}},
	"stock_import_batches":   {{Col: "target_owner_id", TypeCol: "target_owner_type"}},
	"stock_import_rows":      {{Col: "target_owner_id", TypeCol: "target_owner_type"}},
}

// Options describe a merge.
type Options struct {
	SourceID int64
	TargetID int64
	// Actor is recorded on the run ("cli:furkan", "migrator").
	Actor string
	// Keep lists columns that stay on the source (history rows such as
	// orders.buyer_org_id of the orders the source bought).
	Keep []Column
	// ServicePoint copies the source name and logo to the target's
	// service point fields when they are empty and turns the service point
	// on (the AFM Florya case).
	ServicePoint bool
}

// Org is an organization in the report.
type Org struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	BrandID  int64  `json:"brand_id"`
	ParentID *int64 `json:"parent_id,omitempty"`
}

// TableReport counts one column.
type TableReport struct {
	Column
	Moved int64 `json:"moved"`
	// Kept rows stayed on the source: they collide with a unique key the
	// target already holds (one appointment setting per organization, ...).
	Kept int64 `json:"kept,omitempty"`
	// LedgerRewrite: append-only triggers were bypassed for this column.
	LedgerRewrite bool `json:"ledger_rewrite,omitempty"`
}

// MemberReport is one source member.
type MemberReport struct {
	UserID      int64    `json:"user_id"`
	SourceRoles []string `json:"source_roles"`
	// TargetRoles are granted on a new target membership; empty when the
	// user was already a target member (the membership is kept as is).
	TargetRoles     []string `json:"target_roles,omitempty"`
	AlreadyAtTarget bool     `json:"already_at_target"`
}

// Balance is the sum of a balance view per organization and currency.
type Balance struct {
	View     string `json:"view"`
	OrgID    int64  `json:"organization_id"`
	Currency string `json:"currency"`
	Balance  string `json:"balance"`
}

// Report is the result of a merge (or dry run).
type Report struct {
	RunID          int64          `json:"run_id"`
	RunUUID        string         `json:"run_uuid"`
	Source         Org            `json:"source"`
	Target         Org            `json:"target"`
	Tables         []TableReport  `json:"tables"`
	Members        []MemberReport `json:"members"`
	Territories    int64          `json:"territories_removed"`
	BalancesBefore []Balance      `json:"balances_before"`
	BalancesAfter  []Balance      `json:"balances_after"`
	// KeptTables stayed with the source because the target cannot own
	// them (dealerOnly, merges into a center).
	KeptTables []string `json:"kept_tables,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
}

// Moved is the number of moved rows.
func (r Report) Moved() int64 {
	var n int64
	for _, t := range r.Tables {
		n += t.Moved
	}
	return n
}

// previous holds the organization fields a run changes, for Undo.
type previous struct {
	SourceDeletedAt *time.Time `json:"source_deleted_at"`
	TargetSPEnabled bool       `json:"target_service_point_enabled"`
	TargetSPName    *string    `json:"target_service_point_name"`
	TargetSPLogo    *string    `json:"target_service_point_logo_object_key"`
}

// Merge moves the source organization into the target inside tx.
func Merge(ctx context.Context, tx pgx.Tx, opts Options) (Report, error) {
	var rep Report
	if opts.SourceID == opts.TargetID || opts.SourceID <= 0 || opts.TargetID <= 0 {
		return rep, fmt.Errorf("%w: source and target must be two organizations", ErrInvalid)
	}
	if strings.TrimSpace(opts.Actor) == "" {
		return rep, fmt.Errorf("%w: actor is required", ErrInvalid)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('org_merge'))`); err != nil {
		return rep, fmt.Errorf("orgmerge: lock: %w", err)
	}
	src, srcDeleted, err := loadOrg(ctx, tx, opts.SourceID)
	if err != nil {
		return rep, err
	}
	tgt, tgtDeleted, err := loadOrg(ctx, tx, opts.TargetID)
	if err != nil {
		return rep, err
	}
	rep.Source, rep.Target = src, tgt
	switch {
	case srcDeleted != nil:
		return rep, fmt.Errorf("%w: source %d is deleted or already merged", ErrInvalid, src.ID)
	case tgtDeleted != nil:
		return rep, fmt.Errorf("%w: target %d is deleted", ErrInvalid, tgt.ID)
	case src.BrandID != tgt.BrandID:
		return rep, fmt.Errorf("%w: source and target belong to different brands", ErrInvalid)
	case src.Type == rbac.OrgTypeCenter:
		return rep, fmt.Errorf("%w: a center cannot be merged", ErrInvalid)
	case tgt.Type != rbac.OrgTypeCenter && tgt.Type != src.Type:
		return rep, fmt.Errorf("%w: a %s can only be merged into a center or another %s", ErrInvalid, src.Type, src.Type)
	}
	if under, err := isDescendant(ctx, tx, tgt.ID, src.ID); err != nil {
		return rep, err
	} else if under {
		return rep, fmt.Errorf("%w: the target is inside the source's tree", ErrInvalid)
	}
	if err := checkDeferrable(ctx, tx); err != nil {
		return rep, err
	}
	cols, err := orgColumns(ctx, tx)
	if err != nil {
		return rep, err
	}
	ledgers, err := ledgerTables(ctx, tx, cols)
	if err != nil {
		return rep, err
	}
	if err := ledgerBypass(ctx, tx, true); err != nil {
		return rep, err
	}
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL DEFERRED`); err != nil {
		return rep, fmt.Errorf("orgmerge: defer constraints: %w", err)
	}

	ids := []int64{src.ID, tgt.ID}
	totalsBefore, err := totals(ctx, tx, ids)
	if err != nil {
		return rep, err
	}
	if rep.BalancesBefore, err = balances(ctx, tx, ids); err != nil {
		return rep, err
	}

	prev := previous{SourceDeletedAt: srcDeleted}
	if err := tx.QueryRow(ctx, `SELECT service_point_enabled, service_point_name, service_point_logo_object_key
		FROM organizations WHERE id = $1`, tgt.ID).Scan(&prev.TargetSPEnabled, &prev.TargetSPName, &prev.TargetSPLogo); err != nil {
		return rep, fmt.Errorf("orgmerge: target service point: %w", err)
	}
	prevJSON, _ := json.Marshal(prev)
	var runUUID string
	if err := tx.QueryRow(ctx, `INSERT INTO org_merge_runs (source_org_id, target_org_id, actor, previous)
		VALUES ($1, $2, $3, $4) RETURNING id, uuid::text`, src.ID, tgt.ID, opts.Actor, prevJSON).Scan(&rep.RunID, &runUUID); err != nil {
		return rep, fmt.Errorf("orgmerge: run: %w", err)
	}
	rep.RunUUID = runUUID

	// 1. Target memberships for source members (before the member-keyed
	//    children move).
	if rep.Members, err = addTargetMembers(ctx, tx, rep.RunID, src, tgt); err != nil {
		return rep, err
	}

	// 2. Every organization column.
	keep := map[string]bool{}
	for _, k := range opts.Keep {
		keep[k.String()] = true
	}
	// Columns are moved per table in one statement (a row check may tie two
	// of them: orders.organization_id = seller_org_id). The children's
	// parent goes first: row checks of other tables (the order parties
	// trigger, K6) read the tree as it is when they run.
	var tables []string
	byTable := map[string][]colSpec{}
	add := func(table string, spec colSpec) {
		if _, ok := byTable[table]; !ok {
			tables = append(tables, table)
		}
		byTable[table] = append(byTable[table], spec)
	}
	for _, col := range cols {
		if handled[col.String()] || keep[col.String()] {
			continue
		}
		if tgt.Type == rbac.OrgTypeCenter && dealerOnly[col.Table] {
			var n int64
			if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s = $1`, ident(col.Table), ident(col.Column)), src.ID).Scan(&n); err != nil {
				return rep, fmt.Errorf("orgmerge: count %s: %w", col, err)
			}
			if n > 0 {
				rep.KeptTables = append(rep.KeptTables, col.String())
			}
			continue
		}
		add(col.Table, colSpec{Col: col.Column})
	}
	polyTables := make([]string, 0, len(polymorphic))
	for t := range polymorphic {
		polyTables = append(polyTables, t)
	}
	sort.Strings(polyTables)
	for _, t := range polyTables {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT to_regclass(format('public.%I', $1::text)) IS NOT NULL`, t).Scan(&exists); err != nil {
			return rep, fmt.Errorf("orgmerge: table %s: %w", t, err)
		}
		if !exists {
			continue
		}
		for _, spec := range polymorphic[t] {
			if keep[t+"."+spec.Col] {
				continue
			}
			add(t, spec)
		}
	}
	sort.SliceStable(tables, func(i, j int) bool { return tables[i] == "organizations" && tables[j] != "organizations" })
	for _, table := range tables {
		trs, err := moveTable(ctx, tx, rep.RunID, table, byTable[table], src.ID, tgt.ID)
		if err != nil {
			return rep, err
		}
		for _, tr := range trs {
			tr.LedgerRewrite = tr.Moved > 0 && ledgers[table]
			if tr.Moved > 0 || tr.Kept > 0 {
				rep.Tables = append(rep.Tables, tr)
			}
			if tr.Kept > 0 {
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: %d row(s) kept on the source (unique conflict with the target)", tr.Column, tr.Kept))
			}
		}
	}

	// 3. Territories of the source are released: an area without a
	//    distributor belongs to the brand center (K5, TEC-1003).
	if rep.Territories, err = deleteRows(ctx, tx, rep.RunID, "territories", "organization_id = $1", src.ID); err != nil {
		return rep, err
	}

	// 4. Source memberships (their roles first, so undo restores both).
	if _, err := deleteRows(ctx, tx, rep.RunID, "organization_member_roles",
		"member_id IN (SELECT id FROM organization_members WHERE organization_id = $1)", src.ID); err != nil {
		return rep, err
	}
	if _, err := deleteRows(ctx, tx, rep.RunID, "organization_members", "organization_id = $1", src.ID); err != nil {
		return rep, err
	}

	// 5. The source is soft deleted and points at the target.
	if _, err := tx.Exec(ctx, `UPDATE organizations SET merged_into_id = $2, deleted_at = NOW() WHERE id = $1`, src.ID, tgt.ID); err != nil {
		return rep, fmt.Errorf("orgmerge: mark source: %w", err)
	}
	if opts.ServicePoint {
		if _, err := tx.Exec(ctx, `UPDATE organizations t SET
			service_point_enabled = TRUE,
			service_point_name = COALESCE(t.service_point_name, s.name),
			service_point_logo_object_key = COALESCE(t.service_point_logo_object_key, NULLIF(s.logo_object_key, ''))
			FROM organizations s WHERE t.id = $1 AND s.id = $2`, tgt.ID, src.ID); err != nil {
			return rep, fmt.Errorf("orgmerge: service point: %w", err)
		}
	}

	// 6. Checks.
	if rep.BalancesAfter, err = balances(ctx, tx, ids); err != nil {
		return rep, err
	}
	totalsAfter, err := totals(ctx, tx, ids)
	if err != nil {
		return rep, err
	}
	if !equalMaps(totalsBefore, totalsAfter) {
		return rep, ErrBalanceDrift
	}
	var selfCari int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM cari_accounts WHERE organization_id = $1 AND counterparty_org_id = $1`, tgt.ID).Scan(&selfCari); err != nil {
		return rep, fmt.Errorf("orgmerge: cari check: %w", err)
	}
	if selfCari > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%d cari account(s) of the target now name the target itself as counterparty", selfCari))
	}
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		return rep, fmt.Errorf("orgmerge: foreign keys after the move: %w", err)
	}
	if err := ledgerBypass(ctx, tx, false); err != nil {
		return rep, err
	}
	repJSON, _ := json.Marshal(rep)
	if _, err := tx.Exec(ctx, `UPDATE org_merge_runs SET report = $2 WHERE id = $1`, rep.RunID, repJSON); err != nil {
		return rep, fmt.Errorf("orgmerge: save report: %w", err)
	}
	return rep, nil
}

// UndoReport is the result of Undo.
type UndoReport struct {
	RunID    int64 `json:"run_id"`
	SourceID int64 `json:"source_id"`
	TargetID int64 `json:"target_id"`
	Restored int64 `json:"restored"`
	Reverted int64 `json:"reverted"`
	Removed  int64 `json:"removed"`
}

// Undo reverts an applied run inside tx: deleted rows come back, moved
// columns return to the source, rows the run inserted are removed and the
// source organization is restored. Rows created at the target after the
// merge stay at the target.
func Undo(ctx context.Context, tx pgx.Tx, runUUID string) (UndoReport, error) {
	var out UndoReport
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('org_merge'))`); err != nil {
		return out, fmt.Errorf("orgmerge: lock: %w", err)
	}
	var status string
	var prevJSON []byte
	err := tx.QueryRow(ctx, `SELECT id, source_org_id, target_org_id, status, previous FROM org_merge_runs
		WHERE uuid = $1 FOR UPDATE`, runUUID).Scan(&out.RunID, &out.SourceID, &out.TargetID, &status, &prevJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, fmt.Errorf("%w: run %s", ErrNotFound, runUUID)
	}
	if err != nil {
		return out, fmt.Errorf("orgmerge: run: %w", err)
	}
	if status != "applied" {
		return out, ErrAlreadyUndone
	}
	var prev previous
	if err := json.Unmarshal(prevJSON, &prev); err != nil {
		return out, fmt.Errorf("orgmerge: previous: %w", err)
	}
	if err := checkDeferrable(ctx, tx); err != nil {
		return out, err
	}
	if err := ledgerBypass(ctx, tx, true); err != nil {
		return out, err
	}
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL DEFERRED`); err != nil {
		return out, fmt.Errorf("orgmerge: defer constraints: %w", err)
	}

	// Source organization first (its membership rows come back below).
	if _, err := tx.Exec(ctx, `UPDATE organizations SET merged_into_id = NULL, deleted_at = $2 WHERE id = $1`,
		out.SourceID, prev.SourceDeletedAt); err != nil {
		return out, fmt.Errorf("orgmerge: restore source: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE organizations SET service_point_enabled = $2, service_point_name = $3,
		service_point_logo_object_key = $4 WHERE id = $1`, out.TargetID, prev.TargetSPEnabled, prev.TargetSPName, prev.TargetSPLogo); err != nil {
		return out, fmt.Errorf("orgmerge: restore target: %w", err)
	}

	// Deleted rows, in the order opposite to their deletion.
	delTables, err := changeTables(ctx, tx, out.RunID, "delete")
	if err != nil {
		return out, err
	}
	for _, t := range delTables {
		var cols []string
		if err := tx.QueryRow(ctx, `SELECT array_agg(quote_ident(a.attname::text) ORDER BY a.attnum)
			FROM pg_attribute a WHERE a.attrelid = format('public.%I', $1::text)::regclass
			  AND a.attnum > 0 AND NOT a.attisdropped AND a.attgenerated = ''`, t).Scan(&cols); err != nil {
			return out, fmt.Errorf("orgmerge: columns of %s: %w", t, err)
		}
		sel := make([]string, len(cols))
		for i, c := range cols {
			sel[i] = "r." + c
		}
		tag, err := tx.Exec(ctx, fmt.Sprintf(`INSERT INTO %[1]s (%[2]s) OVERRIDING SYSTEM VALUE
			SELECT %[3]s FROM org_merge_changes c, jsonb_populate_record(NULL::%[1]s, c.row_data) r
			WHERE c.run_id = $1 AND c.kind = 'delete' AND c.table_name = $2 ORDER BY c.id`,
			ident(t), strings.Join(cols, ", "), strings.Join(sel, ", ")), out.RunID, t)
		if err != nil {
			return out, fmt.Errorf("orgmerge: restore %s: %w", t, err)
		}
		out.Restored += tag.RowsAffected()
	}

	// Moved columns, per table in one statement (row checks may tie two of
	// them).
	rows, err := tx.Query(ctx, `SELECT table_name, array_agg(DISTINCT column_name ORDER BY column_name)
		FROM org_merge_changes WHERE run_id = $1 AND kind = 'update' GROUP BY table_name ORDER BY table_name`, out.RunID)
	if err != nil {
		return out, fmt.Errorf("orgmerge: changes: %w", err)
	}
	type moved struct {
		table string
		cols  []string
	}
	var all []moved
	for rows.Next() {
		var m moved
		if err := rows.Scan(&m.table, &m.cols); err != nil {
			rows.Close()
			return out, err
		}
		all = append(all, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	// The tree first, as in Merge (the order parties check reads it).
	sort.SliceStable(all, func(i, j int) bool { return all[i].table == "organizations" && all[j].table != "organizations" })
	for _, m := range all {
		pk, err := primaryKey(ctx, tx, m.table)
		if err != nil {
			return out, err
		}
		match := make([]string, len(pk))
		for i, k := range pk {
			match[i] = fmt.Sprintf("t.%s::text = c.pk->>%s", ident(k), literal(k))
		}
		sets := make([]string, len(m.cols))
		for i, col := range m.cols {
			sets[i] = fmt.Sprintf("%[1]s = CASE WHEN %[2]s = ANY (c.cols) AND t.%[1]s = $3 THEN $2 ELSE t.%[1]s END",
				ident(col), literal(col))
		}
		tag, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s t SET %s
			FROM (SELECT pk, array_agg(column_name::text) AS cols FROM org_merge_changes
			      WHERE run_id = $1 AND kind = 'update' AND table_name = %s GROUP BY pk) c
			WHERE %s`, ident(m.table), strings.Join(sets, ", "), literal(m.table), strings.Join(match, " AND ")),
			out.RunID, out.SourceID, out.TargetID)
		if err != nil {
			return out, fmt.Errorf("orgmerge: revert %s: %w", m.table, err)
		}
		out.Reverted += tag.RowsAffected()
	}

	// Rows added at the target after the merge under a parent the undo moved
	// back (a migrator rerun adds measurement values to a merged result)
	// follow their parent through the composite keys.
	followed, err := followParents(ctx, tx, out.SourceID, out.TargetID)
	if err != nil {
		return out, err
	}
	out.Reverted += followed

	// Inserted rows (target memberships; their roles cascade).
	tag, err := tx.Exec(ctx, `DELETE FROM organization_members m USING org_merge_changes c
		WHERE c.run_id = $1 AND c.kind = 'insert' AND c.table_name = 'organization_members'
		  AND m.id::text = c.pk->>'id'`, out.RunID)
	if err != nil {
		return out, fmt.Errorf("orgmerge: remove target members: %w", err)
	}
	out.Removed = tag.RowsAffected()

	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		return out, fmt.Errorf("orgmerge: foreign keys after undo: %w", err)
	}
	if err := ledgerBypass(ctx, tx, false); err != nil {
		return out, err
	}
	if _, err := tx.Exec(ctx, `UPDATE org_merge_runs SET status = 'undone', undone_at = NOW() WHERE id = $1`, out.RunID); err != nil {
		return out, fmt.Errorf("orgmerge: mark undone: %w", err)
	}
	return out, nil
}

// --- helpers -------------------------------------------------------------

// followParents moves child rows from target back to source when their
// composite foreign key points at a parent row that is at the source,
// repeating until nothing changes (children of children).
func followParents(ctx context.Context, tx pgx.Tx, source, target int64) (int64, error) {
	cols, err := orgColumns(ctx, tx)
	if err != nil {
		return 0, err
	}
	isOrg := map[string]bool{}
	for _, c := range cols {
		isOrg[c.String()] = true
	}
	rows, err := tx.Query(ctx, `SELECT c.conrelid::regclass::text, c.confrelid::regclass::text,
			ARRAY(SELECT a.attname::text FROM unnest(c.conkey) WITH ORDINALITY k(n, i)
			      JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.n ORDER BY k.i),
			ARRAY(SELECT a.attname::text FROM unnest(c.confkey) WITH ORDINALITY k(n, i)
			      JOIN pg_attribute a ON a.attrelid = c.confrelid AND a.attnum = k.n ORDER BY k.i)
		FROM pg_constraint c JOIN pg_namespace n ON n.oid = c.connamespace
		WHERE c.contype = 'f' AND n.nspname = 'public' AND array_length(c.conkey, 1) > 1`)
	if err != nil {
		return 0, fmt.Errorf("orgmerge: composite keys: %w", err)
	}
	type fk struct {
		child, parent      string
		childCols, parCols []string
	}
	var fks []fk
	for rows.Next() {
		var f fk
		if err := rows.Scan(&f.child, &f.parent, &f.childCols, &f.parCols); err != nil {
			rows.Close()
			return 0, err
		}
		fks = append(fks, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	var stmts []string
	for _, f := range fks {
		child, parent := strings.Trim(f.child, `"`), strings.Trim(f.parent, `"`)
		org := -1
		for i, c := range f.childCols {
			if isOrg[child+"."+c] && isOrg[parent+"."+f.parCols[i]] {
				org = i
				break
			}
		}
		if org < 0 {
			continue
		}
		join := []string{}
		for i := range f.childCols {
			if i != org {
				join = append(join, fmt.Sprintf("c.%s = p.%s", ident(f.childCols[i]), ident(f.parCols[i])))
			}
		}
		stmts = append(stmts, fmt.Sprintf(`UPDATE %s c SET %s = $1 FROM %s p
			WHERE c.%s = $2 AND p.%s = $1 AND %s`,
			ident(child), ident(f.childCols[org]), ident(parent), ident(f.childCols[org]), ident(f.parCols[org]),
			strings.Join(join, " AND ")))
	}
	var total int64
	for range 10 {
		var round int64
		for _, st := range stmts {
			tag, err := tx.Exec(ctx, st, source, target)
			if err != nil {
				return total, fmt.Errorf("orgmerge: follow parents: %w", err)
			}
			round += tag.RowsAffected()
		}
		total += round
		if round == 0 {
			break
		}
	}
	return total, nil
}

func loadOrg(ctx context.Context, tx pgx.Tx, id int64) (Org, *time.Time, error) {
	var o Org
	var deleted *time.Time
	err := tx.QueryRow(ctx, `SELECT id, name, type, brand_id, parent_id, deleted_at FROM organizations
		WHERE id = $1 FOR UPDATE`, id).Scan(&o.ID, &o.Name, &o.Type, &o.BrandID, &o.ParentID, &deleted)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, nil, fmt.Errorf("%w: %d", ErrNotFound, id)
	}
	if err != nil {
		return o, nil, fmt.Errorf("orgmerge: organization %d: %w", id, err)
	}
	return o, deleted, nil
}

func isDescendant(ctx context.Context, tx pgx.Tx, orgID, ancestorID int64) (bool, error) {
	var under bool
	err := tx.QueryRow(ctx, `WITH RECURSIVE up AS (
			SELECT id, parent_id FROM organizations WHERE id = $1
			UNION ALL
			SELECT o.id, o.parent_id FROM organizations o JOIN up ON o.id = up.parent_id
		) SELECT EXISTS (SELECT 1 FROM up WHERE id = $2)`, orgID, ancestorID).Scan(&under)
	if err != nil {
		return false, fmt.Errorf("orgmerge: tree: %w", err)
	}
	return under, nil
}

// checkDeferrable refuses to run when a foreign key that may carry an
// organization column is not deferrable (a table added after 000152).
func checkDeferrable(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `SELECT c.conrelid::regclass::text || '.' || c.conname
		FROM pg_constraint c JOIN pg_namespace n ON n.oid = c.connamespace
		WHERE c.contype = 'f' AND n.nspname = 'public' AND NOT c.condeferrable
		  AND (c.confrelid = 'organizations'::regclass OR array_length(c.conkey, 1) > 1)
		ORDER BY 1`)
	if err != nil {
		return fmt.Errorf("orgmerge: constraints: %w", err)
	}
	defer rows.Close()
	var bad []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return err
		}
		bad = append(bad, s)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(bad) > 0 {
		return fmt.Errorf("%w: %s (make it DEFERRABLE in a migration)", ErrNotDeferrable, strings.Join(bad, ", "))
	}
	return nil
}

// orgColumns lists the organization columns of the public base tables.
func orgColumns(ctx context.Context, tx pgx.Tx) ([]Column, error) {
	rows, err := tx.Query(ctx, `SELECT cl.relname::text, a.attname::text
		FROM pg_attribute a
		JOIN pg_class cl ON cl.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = cl.relnamespace
		WHERE n.nspname = 'public' AND cl.relkind IN ('r', 'p') AND NOT cl.relispartition
		  AND a.attnum > 0 AND NOT a.attisdropped AND a.attgenerated = '' AND a.atttypid = 'int8'::regtype
		  AND (a.attname = 'organization_id' OR a.attname LIKE '%\_org\_id' OR a.attname LIKE '%\_organization\_id'
		       OR EXISTS (
		           SELECT 1 FROM pg_constraint k
		           WHERE k.contype = 'f' AND k.confrelid = 'organizations'::regclass
		             AND k.conrelid = cl.oid AND k.conkey = ARRAY[a.attnum]))
		ORDER BY 1, 2`)
	if err != nil {
		return nil, fmt.Errorf("orgmerge: columns: %w", err)
	}
	defer rows.Close()
	var out []Column
	for rows.Next() {
		var c Column
		if err := rows.Scan(&c.Table, &c.Column); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func primaryKey(ctx context.Context, tx pgx.Tx, table string) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT a.attname FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY (i.indkey)
		WHERE i.indrelid = format('public.%I', $1::text)::regclass AND i.indisprimary
		ORDER BY array_position(i.indkey::int2[], a.attnum)`, table)
	if err != nil {
		return nil, fmt.Errorf("orgmerge: primary key of %s: %w", table, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: %s has no primary key", ErrInvalid, table)
	}
	return out, nil
}

func pkJSON(pk []string, alias string) string {
	parts := make([]string, len(pk))
	for i, k := range pk {
		parts[i] = literal(k) + ", " + alias + "." + ident(k)
	}
	return "jsonb_build_object(" + strings.Join(parts, ", ") + ")"
}

// moveTable moves the organization columns of a table from source to
// target in one statement per row set and records each moved (row, column).
// A unique conflict falls back to row by row; colliding rows stay on the
// source (counted as Kept on the table's first column).
func moveTable(ctx context.Context, tx pgx.Tx, runID int64, table string, cols []colSpec, source, target int64) ([]TableReport, error) {
	out := make([]TableReport, len(cols))
	match := make([]string, len(cols))
	sets := make([]string, len(cols))
	flags := make([]string, len(cols))
	for i, spec := range cols {
		c := spec.Col
		out[i] = TableReport{Column: Column{Table: table, Column: c}}
		cond := fmt.Sprintf("t.%s = $1", ident(c))
		oldCond := fmt.Sprintf("old.%s = $1", ident(c))
		if spec.TypeCol != "" {
			cond = fmt.Sprintf("(t.%s IN %s AND %s)", ident(spec.TypeCol), orgOwnerTypes, cond)
			oldCond = fmt.Sprintf("(old.%s IN %s AND %s)", ident(spec.TypeCol), orgOwnerTypes, oldCond)
		}
		match[i] = cond
		sets[i] = fmt.Sprintf("%s = CASE WHEN %s THEN $2 ELSE t.%s END", ident(c), cond, ident(c))
		flags[i] = fmt.Sprintf("CASE WHEN %s THEN %s END", oldCond, literal(c))
	}
	where := "(" + strings.Join(match, " OR ") + ")"
	var pending int64
	if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s t WHERE %s`, ident(table), where), source).Scan(&pending); err != nil {
		return nil, fmt.Errorf("orgmerge: count %s: %w", table, err)
	}
	if pending == 0 {
		return out, nil
	}
	pk, err := primaryKey(ctx, tx, table)
	if err != nil {
		return nil, err
	}
	stmt := func(extra string) string {
		return fmt.Sprintf(`WITH moved AS (
				UPDATE %[1]s t SET %[2]s WHERE %[3]s %[4]s
				RETURNING %[5]s AS pk, ARRAY[%[6]s]::text[] AS cols)
			INSERT INTO org_merge_changes (run_id, kind, table_name, column_name, pk)
			SELECT $3, 'update', %[7]s, c.col, moved.pk FROM moved, unnest(moved.cols) AS c (col)
			WHERE c.col IS NOT NULL
			RETURNING column_name`,
			ident(table), strings.Join(sets, ", "), where, extra, pkJSON(pk, "t"), strings.Join(flags, ", "), literal(table))
	}
	count := func(colNames []string) {
		for _, n := range colNames {
			for i := range out {
				if out[i].Column.Column == n {
					out[i].Moved++
				}
			}
		}
	}
	move := func(extra string, args ...any) ([]string, error) {
		var names []string
		_, err := savepoint(ctx, tx, func(sp pgx.Tx) (int64, error) {
			rows, err := sp.Query(ctx, stmt(extra), append([]any{source, target, runID}, args...)...)
			if err != nil {
				return 0, err
			}
			defer rows.Close()
			for rows.Next() {
				var n string
				if err := rows.Scan(&n); err != nil {
					return 0, err
				}
				names = append(names, n)
			}
			return int64(len(names)), rows.Err()
		})
		return names, err
	}
	names, err := move("")
	switch {
	case err == nil:
		count(names)
	case errors.Is(err, errUniqueConflict):
		keys, err := rowKeys(ctx, tx, table, where, pk, source)
		if err != nil {
			return nil, err
		}
		cond := make([]string, len(pk))
		for i, k := range pk {
			cond[i] = fmt.Sprintf("t.%s::text = ($4::jsonb)->>%s", ident(k), literal(k))
		}
		for _, key := range keys {
			names, err := move("AND "+strings.Join(cond, " AND "), key)
			switch {
			case err == nil:
				count(names)
			case errors.Is(err, errUniqueConflict):
				out[0].Kept++
			default:
				return nil, fmt.Errorf("orgmerge: move %s: %w", table, err)
			}
		}
	default:
		return nil, fmt.Errorf("orgmerge: move %s: %w", table, err)
	}
	return out, nil
}

func rowKeys(ctx context.Context, tx pgx.Tx, table, where string, pk []string, source int64) ([]string, error) {
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT %s::text FROM %s t WHERE %s`,
		pkJSON(pk, "t"), ident(table), where), source)
	if err != nil {
		return nil, fmt.Errorf("orgmerge: keys of %s: %w", table, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// savepoint runs fn in a nested transaction; a unique violation rolls it
// back and returns errUniqueConflict.
func savepoint(ctx context.Context, tx pgx.Tx, fn func(pgx.Tx) (int64, error)) (int64, error) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return 0, err
	}
	n, err := fn(sp)
	if err != nil {
		_ = sp.Rollback(ctx)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return 0, errUniqueConflict
		}
		return 0, err
	}
	return n, sp.Commit(ctx)
}

// ledgerTriggers are the append-only / guard row triggers of a table.
func ledgerTriggers(ctx context.Context, tx pgx.Tx, table string) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT tgname FROM pg_trigger
		WHERE tgrelid = format('public.%I', $1::text)::regclass AND NOT tgisinternal
		  AND (tgname LIKE '%\_append\_only' OR tgname LIKE '%\_guard') ORDER BY 1`, table)
	if err != nil {
		return nil, fmt.Errorf("orgmerge: triggers of %s: %w", table, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ledgerTables reports which tables with an organization column are
// append-only ledgers (for the report).
func ledgerTables(ctx context.Context, tx pgx.Tx, cols []Column) (map[string]bool, error) {
	out := map[string]bool{}
	for _, c := range cols {
		if _, seen := out[c.Table]; seen {
			continue
		}
		trg, err := ledgerTriggers(ctx, tx, c.Table)
		if err != nil {
			return nil, err
		}
		out[c.Table] = len(trg) > 0
	}
	return out, nil
}

// ledgerBypass turns olex.org_merge on or off for the rest of the
// transaction.
func ledgerBypass(ctx context.Context, tx pgx.Tx, on bool) error {
	v := "off"
	if on {
		v = "on"
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('olex.org_merge', $1, true)`, v); err != nil {
		return fmt.Errorf("orgmerge: ledger bypass: %w", err)
	}
	return nil
}

// deleteRows deletes the rows matching where ($1 = arg) and records them.
func deleteRows(ctx context.Context, tx pgx.Tx, runID int64, table, where string, arg any) (int64, error) {
	pk, err := primaryKey(ctx, tx, table)
	if err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, fmt.Sprintf(`WITH del AS (DELETE FROM %[1]s t WHERE %[2]s RETURNING t.*)
		INSERT INTO org_merge_changes (run_id, kind, table_name, pk, row_data)
		SELECT $2, 'delete', %[3]s, %[4]s, to_jsonb(del.*) FROM del`,
		ident(table), where, literal(table), pkJSON(pk, "del")), arg, runID)
	if err != nil {
		return 0, fmt.Errorf("orgmerge: delete %s: %w", table, err)
	}
	return tag.RowsAffected(), nil
}

// addTargetMembers gives every source member a target membership with the
// center counterpart of their roles (same roles when the target is not a
// center). Members already at the target keep their target roles.
func addTargetMembers(ctx context.Context, tx pgx.Tx, runID int64, src, tgt Org) ([]MemberReport, error) {
	rows, err := tx.Query(ctx, `SELECT m.user_id,
			COALESCE(array_agg(r.slug ORDER BY r.slug) FILTER (WHERE r.slug IS NOT NULL), '{}'),
			EXISTS (SELECT 1 FROM organization_members x WHERE x.organization_id = $2 AND x.user_id = m.user_id)
		FROM organization_members m
		LEFT JOIN organization_member_roles mr ON mr.member_id = m.id
		LEFT JOIN roles r ON r.id = mr.role_id
		WHERE m.organization_id = $1
		GROUP BY m.id, m.user_id ORDER BY m.user_id`, src.ID, tgt.ID)
	if err != nil {
		return nil, fmt.Errorf("orgmerge: members: %w", err)
	}
	var out []MemberReport
	for rows.Next() {
		var m MemberReport
		if err := rows.Scan(&m.UserID, &m.SourceRoles, &m.AlreadyAtTarget); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		m := &out[i]
		if m.AlreadyAtTarget {
			continue
		}
		set := map[string]bool{}
		for _, slug := range m.SourceRoles {
			if tgt.Type != rbac.OrgTypeCenter {
				set[slug] = true
				continue
			}
			if c, ok := rbac.CenterRoleFor(slug); ok {
				set[c] = true
			}
		}
		if len(set) == 0 && tgt.Type == rbac.OrgTypeCenter {
			set[rbac.RoleCenterService] = true // least privileged center role
		}
		for slug := range set {
			m.TargetRoles = append(m.TargetRoles, slug)
		}
		sort.Strings(m.TargetRoles)
		var memberID int64
		if err := tx.QueryRow(ctx, `INSERT INTO organization_members (organization_id, user_id, role)
			VALUES ($1, $2, 'staff') RETURNING id`, tgt.ID, m.UserID).Scan(&memberID); err != nil {
			return nil, fmt.Errorf("orgmerge: member %d: %w", m.UserID, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_merge_changes (run_id, kind, table_name, pk)
			VALUES ($1, 'insert', 'organization_members', jsonb_build_object('id', $2::bigint))`, runID, memberID); err != nil {
			return nil, fmt.Errorf("orgmerge: record member: %w", err)
		}
		tag, err := tx.Exec(ctx, `INSERT INTO organization_member_roles (member_id, role_id)
			SELECT $1, r.id FROM roles r WHERE r.slug = ANY ($2::text[]) ON CONFLICT DO NOTHING`, memberID, m.TargetRoles)
		if err != nil {
			return nil, fmt.Errorf("orgmerge: member roles %d: %w", m.UserID, err)
		}
		if tag.RowsAffected() != int64(len(m.TargetRoles)) {
			return nil, fmt.Errorf("%w: roles %v are not seeded", ErrInvalid, m.TargetRoles)
		}
	}
	return out, nil
}

func changeTables(ctx context.Context, tx pgx.Tx, runID int64, kind string) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT table_name FROM org_merge_changes WHERE run_id = $1 AND kind = $2
		GROUP BY table_name ORDER BY max(id) DESC`, runID, kind)
	if err != nil {
		return nil, fmt.Errorf("orgmerge: changes: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// balances sums the cari and finance balance views of the organizations.
func balances(ctx context.Context, tx pgx.Tx, orgs []int64) ([]Balance, error) {
	rows, err := tx.Query(ctx, `
		SELECT 'cari', organization_id, currency, sum(balance)::text FROM cari_account_balances
		WHERE organization_id = ANY ($1) GROUP BY 2, 3
		UNION ALL
		SELECT 'finance', organization_id, currency, sum(balance)::text FROM finance_account_balances
		WHERE organization_id = ANY ($1) GROUP BY 2, 3
		ORDER BY 1, 2, 3`, orgs)
	if err != nil {
		return nil, fmt.Errorf("orgmerge: balances: %w", err)
	}
	defer rows.Close()
	out := []Balance{}
	for rows.Next() {
		var b Balance
		if err := rows.Scan(&b.View, &b.OrgID, &b.Currency, &b.Balance); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// totals sums the cari and finance balances of the organizations per view
// and currency: a merge moves balances between them, never changes them.
func totals(ctx context.Context, tx pgx.Tx, orgs []int64) (map[string]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT 'cari/' || currency, sum(balance)::text FROM cari_account_balances
		WHERE organization_id = ANY ($1) GROUP BY currency
		UNION ALL
		SELECT 'finance/' || currency, sum(balance)::text FROM finance_account_balances
		WHERE organization_id = ANY ($1) GROUP BY currency`, orgs)
	if err != nil {
		return nil, fmt.Errorf("orgmerge: totals: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func ident(s string) string { return pgx.Identifier{s}.Sanitize() }

func literal(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
