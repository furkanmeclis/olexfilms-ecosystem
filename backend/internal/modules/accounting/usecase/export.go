package usecase

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/google/uuid"
)

// TEC-175: statement and balance report exports run as I/O engine export
// jobs (queue "exports", consumed by worker-docs). CSV/XLSX use the generic
// encoders with headers in the job locale; PDF is a styled HTML document
// rendered by Gotenberg (pdfrender skeleton: lang/dir, RTL for ar, embedded
// Noto fonts) on the organization letterhead.

// Export resources (export_jobs.resource).
const (
	ResourceCariStatement = "tenant.accounting.cari_statement"
	ResourceBalances      = "tenant.accounting.balances"
)

// Export query keys written by the HTTP handler after it authorized the
// request. The worker re-checks that the book is the job organization or an
// organization below it.
const (
	QueryBookOrganizationID = "book_organization_id"
	QueryCariUUID           = "cari_uuid"
	QueryFrom               = "from"
	QueryTo                 = "to"
	QueryAsOf               = "as_of"
)

// errExportScope: the job's book is outside the job organization's tree.
var errExportScope = errors.New("accounting export: book is outside the organization tree")

// exportBook loads the book of an export job and checks it against the job
// organization (ioengine.QueryOrganizationID).
func (s *Service) exportBook(ctx context.Context, q ioengine.ExportQuery) (db.Organization, error) {
	jobOrg, err := strconv.ParseInt(q[ioengine.QueryOrganizationID], 10, 64)
	if err != nil || jobOrg <= 0 {
		return db.Organization{}, errors.New("accounting export: organization is required")
	}
	bookID := jobOrg
	if raw := strings.TrimSpace(q[QueryBookOrganizationID]); raw != "" {
		if bookID, err = strconv.ParseInt(raw, 10, 64); err != nil || bookID <= 0 {
			return db.Organization{}, errors.New("accounting export: invalid book organization")
		}
	}
	if bookID != jobOrg {
		below, err := s.q.Descendants(ctx, jobOrg)
		if err != nil {
			return db.Organization{}, fmt.Errorf("accounting export: descendants: %w", err)
		}
		found := false
		for _, o := range below {
			if o.ID == bookID {
				found = true
				break
			}
		}
		if !found {
			return db.Organization{}, errExportScope
		}
	}
	return s.q.GetOrganizationByID(ctx, bookID)
}

func queryDay(q ioengine.ExportQuery, key string) (*time.Time, error) {
	raw := strings.TrimSpace(q[key])
	if raw == "" {
		return nil, nil
	}
	d, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return nil, invalid(key, "must be a date (YYYY-MM-DD)")
	}
	return &d, nil
}

// exportOnly implements the import half of ioengine.ResourceAdapter (these
// resources are export only).
type exportOnly struct{}

func (exportOnly) ImportSchema() []ioengine.ImportField { return nil }
func (exportOnly) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "export only"}, nil
}
func (exportOnly) RevertRow(context.Context, string, string, map[string]any) error { return nil }

// --- Statement ---------------------------------------------------------------

// StatementAdapter exports a cari statement.
type StatementAdapter struct {
	exportOnly
	svc *Service
}

// NewStatementAdapter creates the statement export adapter.
func NewStatementAdapter(svc *Service) *StatementAdapter { return &StatementAdapter{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *StatementAdapter) Resource() string { return ResourceCariStatement }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *StatementAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "date", LabelKey: "accounting.statement.date", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "description", LabelKey: "accounting.statement.description", Type: ioengine.ColumnTypeString, Weight: 1.8},
		{Key: "source", LabelKey: "accounting.statement.source", Type: ioengine.ColumnTypeString, Weight: 0.9},
		{Key: "debit", LabelKey: "accounting.statement.debit", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "credit", LabelKey: "accounting.statement.credit", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "balance", LabelKey: "accounting.statement.balance", Type: ioengine.ColumnTypeString, AlignRight: true},
	}
}

// Export implements ioengine.ResourceAdapter. Query: book_organization_id,
// cari_uuid, from, to. The first row is the opening balance, the last the
// period totals and closing balance (CSV has no summary area).
func (a *StatementAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	book, err := a.svc.exportBook(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(q[QueryCariUUID]))
	if err != nil {
		return ioengine.Dataset{}, ErrCariNotFound
	}
	var p StatementPeriod
	if p.From, err = queryDay(q, QueryFrom); err != nil {
		return ioengine.Dataset{}, err
	}
	if p.To, err = queryDay(q, QueryTo); err != nil {
		return ioengine.Dataset{}, err
	}
	st, err := a.svc.statement(ctx, book, id, p, loc)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	return StatementDataset(st, loc), nil
}

// StatementDataset maps a statement to an export dataset.
func StatementDataset(st Statement, loc i18n.Locale) ioengine.Dataset {
	t := func(key string) string { return i18n.Translate(loc, key) }
	rows := make([]map[string]any, 0, len(st.Lines)+2)
	openingDate := ""
	if st.From != nil {
		openingDate = *st.From
	}
	rows = append(rows, map[string]any{
		"date": openingDate, "description": t("accounting.statement.opening_balance"),
		"source": "", "debit": "", "credit": "", "balance": st.OpeningBalance,
	})
	for _, l := range st.Lines {
		rows = append(rows, map[string]any{
			"date": l.Date.Format(time.DateOnly), "description": l.Description, "source": l.SourceLabel,
			"debit": l.Debit, "credit": l.Credit, "balance": l.Balance,
		})
	}
	closingDate := ""
	if st.To != nil {
		closingDate = *st.To
	}
	rows = append(rows, map[string]any{
		"date": closingDate, "description": t("accounting.statement.closing_balance"),
		"source": "", "debit": st.TotalDebit, "credit": st.TotalCredit, "balance": st.ClosingBalance,
	})
	return ioengine.Dataset{
		Resource: ResourceCariStatement,
		Columns:  NewStatementAdapter(nil).ExportColumns(),
		Rows:     rows,
		Info: []ioengine.InfoLine{
			{LabelKey: "accounting.statement.organization", Value: st.Organization.Name},
			{LabelKey: "accounting.statement.counterparty", Value: st.Cari.Counterparty.Name},
			{LabelKey: "accounting.statement.period", Value: periodLabel(st.From, st.To, loc)},
			{LabelKey: "accounting.statement.currency", Value: st.Currency},
		},
		Doc: st,
	}
}

func periodLabel(from, to *string, loc i18n.Locale) string {
	if from == nil && to == nil {
		return i18n.Translate(loc, "accounting.statement.all_time")
	}
	f, e := "…", "…"
	if from != nil {
		f = *from
	}
	if to != nil {
		e = *to
	}
	return f + " – " + e
}

// DocumentHTML implements ioengine.DocumentRenderer: the styled statement.
func (a *StatementAdapter) DocumentHTML(ds ioengine.Dataset, locale string, lh *ioengine.Letterhead, title string) (string, error) {
	st, ok := ds.Doc.(Statement)
	if !ok {
		return "", errors.New("accounting export: dataset carries no statement")
	}
	loc := i18n.Normalize(locale)
	t := func(key string) string { return i18n.Translate(loc, key) }
	cols := []pdfrender.Column{
		{Label: t("accounting.statement.date")}, {Label: t("accounting.statement.description")},
		{Label: t("accounting.statement.source")}, {Label: t("accounting.statement.debit"), Numeric: true},
		{Label: t("accounting.statement.credit"), Numeric: true}, {Label: t("accounting.statement.balance"), Numeric: true},
	}
	rows := make([][]string, 0, len(ds.Rows))
	for _, r := range ds.Rows {
		rows = append(rows, []string{
			cellString(r["date"]), cellString(r["description"]), cellString(r["source"]),
			cellString(r["debit"]), cellString(r["credit"]), cellString(r["balance"]),
		})
	}
	totals := [][2]string{
		{t("accounting.statement.opening_balance"), st.OpeningBalance + " " + st.Currency},
		{t("accounting.statement.total_debit"), st.TotalDebit + " " + st.Currency},
		{t("accounting.statement.total_credit"), st.TotalCredit + " " + st.Currency},
		{t("accounting.statement.closing_balance"), st.ClosingBalance + " " + st.Currency},
	}
	return documentHTML(loc, lh, title, ds.Info, pdfrender.Table(cols, rows), totals, st.GeneratedAt, pdfrender.Zone(ds.Timezone, st.Timezone)), nil
}

// --- Balance report ----------------------------------------------------------

// BalancesAdapter exports the balance report of a book.
type BalancesAdapter struct {
	exportOnly
	svc *Service
}

// NewBalancesAdapter creates the balance report export adapter.
func NewBalancesAdapter(svc *Service) *BalancesAdapter { return &BalancesAdapter{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *BalancesAdapter) Resource() string { return ResourceBalances }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *BalancesAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "kind", LabelKey: "accounting.report.kind", Type: ioengine.ColumnTypeString},
		{Key: "name", LabelKey: "accounting.report.name", Type: ioengine.ColumnTypeString, Weight: 2},
		{Key: "currency", LabelKey: "accounting.statement.currency", Type: ioengine.ColumnTypeString, Weight: 0.6},
		{Key: "balance", LabelKey: "accounting.statement.balance", Type: ioengine.ColumnTypeString, AlignRight: true},
	}
}

// Export implements ioengine.ResourceAdapter. Query: book_organization_id,
// as_of. Accounts come first, then the cari accounts, then the totals.
func (a *BalancesAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	book, err := a.svc.exportBook(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	asOf, err := queryDay(q, QueryAsOf)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rep, err := a.svc.balanceReport(ctx, book, asOf)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	return BalancesDataset(rep, loc), nil
}

// BalancesDataset maps a balance report to an export dataset.
func BalancesDataset(rep BalanceReport, loc i18n.Locale) ioengine.Dataset {
	t := func(key string) string { return i18n.Translate(loc, key) }
	rows := make([]map[string]any, 0, len(rep.Accounts)+len(rep.Cari)+4)
	for _, a := range rep.Accounts {
		rows = append(rows, map[string]any{
			"kind": t("accounting.account_type." + a.Type), "name": a.Name, "currency": a.Currency, "balance": a.Balance,
		})
	}
	for _, c := range rep.Cari {
		rows = append(rows, map[string]any{
			"kind": t("accounting.report.kind_cari"), "name": c.Counterparty.Name, "currency": c.Currency, "balance": c.Balance,
		})
	}
	total := t("accounting.report.total")
	for _, line := range [][2]string{
		{"accounting.report.cash_total", rep.Totals.Cash},
		{"accounting.report.bank_total", rep.Totals.Bank},
		{"accounting.report.receivable", rep.Totals.Receivable},
		{"accounting.report.payable", rep.Totals.Payable},
	} {
		rows = append(rows, map[string]any{"kind": total, "name": t(line[0]), "currency": rep.Currency, "balance": line[1]})
	}
	asOf := t("accounting.statement.all_time")
	if rep.AsOf != nil {
		asOf = *rep.AsOf
	}
	return ioengine.Dataset{
		Resource: ResourceBalances,
		Columns:  NewBalancesAdapter(nil).ExportColumns(),
		Rows:     rows,
		Info: []ioengine.InfoLine{
			{LabelKey: "accounting.statement.organization", Value: rep.Organization.Name},
			{LabelKey: "accounting.report.as_of", Value: asOf},
			{LabelKey: "accounting.statement.currency", Value: rep.Currency},
		},
		Doc: rep,
	}
}

// DocumentHTML implements ioengine.DocumentRenderer.
func (a *BalancesAdapter) DocumentHTML(ds ioengine.Dataset, locale string, lh *ioengine.Letterhead, title string) (string, error) {
	rep, ok := ds.Doc.(BalanceReport)
	if !ok {
		return "", errors.New("accounting export: dataset carries no balance report")
	}
	loc := i18n.Normalize(locale)
	t := func(key string) string { return i18n.Translate(loc, key) }
	cols := []pdfrender.Column{
		{Label: t("accounting.report.kind")}, {Label: t("accounting.report.name")},
		{Label: t("accounting.statement.currency")}, {Label: t("accounting.statement.balance"), Numeric: true},
	}
	n := len(rep.Accounts) + len(rep.Cari)
	rows := make([][]string, 0, n)
	for _, r := range ds.Rows[:n] {
		rows = append(rows, []string{cellString(r["kind"]), cellString(r["name"]), cellString(r["currency"]), cellString(r["balance"])})
	}
	totals := [][2]string{
		{t("accounting.report.cash_total"), rep.Totals.Cash + " " + rep.Currency},
		{t("accounting.report.bank_total"), rep.Totals.Bank + " " + rep.Currency},
		{t("accounting.report.receivable"), rep.Totals.Receivable + " " + rep.Currency},
		{t("accounting.report.payable"), rep.Totals.Payable + " " + rep.Currency},
	}
	return documentHTML(loc, lh, title, ds.Info, pdfrender.Table(cols, rows), totals, rep.GeneratedAt, pdfrender.Zone(ds.Timezone, rep.Timezone)), nil
}

// --- HTML document -------------------------------------------------------------

func cellString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// documentHTML lays out a report on the letterhead: header (logo, company
// contact), title, info table, the data table and a totals block. Every
// value is escaped; the skeleton sets lang/dir (RTL for ar) and fonts.
func documentHTML(loc i18n.Locale, lh *ioengine.Letterhead, title string, info []ioengine.InfoLine, table string, totals [][2]string, generated time.Time, zone *time.Location) string {
	esc := html.EscapeString
	var b strings.Builder
	color := ""
	if lh != nil {
		color = lh.PrimaryColor
	}
	b.WriteString(ioengine.LetterheadHeaderHTML(lh))
	b.WriteString(`<h1 class="doc-title">`)
	b.WriteString(esc(title))
	b.WriteString(`</h1><table class="doc-meta">`)
	for _, line := range info {
		b.WriteString(`<tr><th>`)
		b.WriteString(esc(i18n.Translate(loc, line.LabelKey)))
		b.WriteString(`</th><td>`)
		b.WriteString(esc(line.Value))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table>`)
	b.WriteString(table)
	b.WriteString(`<table class="doc-totals">`)
	for _, row := range totals {
		b.WriteString(`<tr><th>`)
		b.WriteString(esc(row[0]))
		b.WriteString(`</th><td>`)
		b.WriteString(esc(row[1]))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table><p class="muted">`)
	b.WriteString(esc(i18n.Translate(loc, "accounting.statement.generated_at")))
	b.WriteString(`: `)
	b.WriteString(esc(pdfrender.IssuedAt(generated, zone)))
	b.WriteString(`</p>`)
	b.WriteString(ioengine.LetterheadFooterHTML(lh))
	return pdfrender.Document{
		Lang: string(loc), Title: title, Body: b.String(), PrimaryColor: color,
	}.HTML()
}
