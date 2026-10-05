package usecase

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// TEC-346: export jobs of the P&L, margin, cari aging and staff cost
// reports. CSV/XLSX/PDF use the generic I/O engine encoders (the PDF is the
// generic table on the organization letterhead, F1-07e path); jobs land in
// the central export list like every other export. The job organization is
// the book: these reports never read another organization's ledger, so the
// adapters ignore any book key on the query. Purchase cost columns of the
// margin report carry pricing.purchase.read, which the export engine
// snapshots at request time and enforces on the dataset.

// Export resources of the TEC-346 reports.
const (
	ResourcePnl       = "tenant.accounting.pnl"
	ResourceMargin    = "tenant.accounting.margin"
	ResourceCariAging = "tenant.accounting.cari_aging"
	ResourceStaffCost = "tenant.accounting.staff_cost"
)

// QueryGroup is the P&L grouping key of an export query.
const QueryGroup = "group"

// IsReportResource reports whether resource is a TEC-346 report export.
func IsReportResource(resource string) bool {
	switch resource {
	case ResourcePnl, ResourceMargin, ResourceCariAging, ResourceStaffCost:
		return true
	}
	return false
}

// jobBook is the job organization (the only book a TEC-346 export reads).
func (s *Service) jobBook(ctx context.Context, q ioengine.ExportQuery) (db.Organization, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(q[ioengine.QueryOrganizationID]), 10, 64)
	if err != nil || id <= 0 {
		return db.Organization{}, errors.New("accounting export: organization is required")
	}
	return s.q.GetOrganizationByID(ctx, id)
}

func queryPeriod(q ioengine.ExportQuery) (StatementPeriod, error) {
	var p StatementPeriod
	var err error
	if p.From, err = queryDay(q, QueryFrom); err != nil {
		return p, err
	}
	if p.To, err = queryDay(q, QueryTo); err != nil {
		return p, err
	}
	return p, p.validate()
}

func periodInfo(from, to *string, loc i18n.Locale) ioengine.InfoLine {
	return ioengine.InfoLine{LabelKey: "accounting.statement.period", Value: periodLabel(from, to, loc)}
}

func moneyCol(key, label string) ioengine.Column {
	return ioengine.Column{Key: key, LabelKey: label, Type: ioengine.ColumnTypeString, AlignRight: true}
}

// --- P&L -----------------------------------------------------------------------

// PnlAdapter exports the P&L.
type PnlAdapter struct {
	exportOnly
	svc *Service
}

// NewPnlAdapter creates the P&L export adapter.
func NewPnlAdapter(svc *Service) *PnlAdapter { return &PnlAdapter{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *PnlAdapter) Resource() string { return ResourcePnl }

// ExportColumns implements ioengine.ResourceAdapter (month grouping; the
// dataset relabels the first column for the category grouping).
func (a *PnlAdapter) ExportColumns() []ioengine.Column { return pnlColumns(PnlGroupMonth) }

func pnlColumns(group string) []ioengine.Column {
	first := "accounting.reports.month"
	if group == PnlGroupCategory {
		first = "accounting.reports.category"
	}
	return []ioengine.Column{
		{Key: "label", LabelKey: first, Type: ioengine.ColumnTypeString, Weight: 1.6},
		moneyCol("income", "accounting.reports.income"),
		moneyCol("expense", "accounting.reports.expense"),
		moneyCol("net", "accounting.reports.net"),
	}
}

// Export implements ioengine.ResourceAdapter. Query: from, to, group.
func (a *PnlAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	book, err := a.svc.jobBook(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	p, err := queryPeriod(q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	group, err := PnlGroup(q[QueryGroup])
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rep, err := a.svc.pnlReport(ctx, book, p, group, loc)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	return PnlDataset(rep, loc), nil
}

// PnlDataset maps a P&L to an export dataset: one row per line, totals in
// the summary row.
func PnlDataset(rep PnlReport, loc i18n.Locale) ioengine.Dataset {
	rows := make([]map[string]any, 0, len(rep.Lines))
	for _, l := range rep.Lines {
		rows = append(rows, map[string]any{"label": l.Label, "income": l.Income, "expense": l.Expense, "net": l.Net})
	}
	return ioengine.Dataset{
		Resource: ResourcePnl,
		Columns:  pnlColumns(rep.Group),
		Rows:     rows,
		Totals: map[string]any{
			"label":  i18n.Translate(loc, "accounting.report.total"),
			"income": rep.Totals.Income, "expense": rep.Totals.Expense, "net": rep.Totals.Net,
		},
		Info: []ioengine.InfoLine{
			{LabelKey: "accounting.statement.organization", Value: rep.Organization.Name},
			periodInfo(rep.From, rep.To, loc),
			{LabelKey: "accounting.statement.currency", Value: rep.Currency},
		},
		Doc: rep,
	}
}

// --- Margin --------------------------------------------------------------------

// MarginAdapter exports the margin report.
type MarginAdapter struct {
	exportOnly
	svc *Service
}

// NewMarginAdapter creates the margin export adapter.
func NewMarginAdapter(svc *Service) *MarginAdapter { return &MarginAdapter{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *MarginAdapter) Resource() string { return ResourceMargin }

// ExportColumns implements ioengine.ResourceAdapter. Cost, profit and
// margin need pricing.purchase.read.
func (a *MarginAdapter) ExportColumns() []ioengine.Column {
	cost := func(key, label string) ioengine.Column {
		c := moneyCol(key, label)
		c.Permission = rbac.PermPricingPurchaseRead
		return c
	}
	return []ioengine.Column{
		{Key: "kind", LabelKey: "accounting.report.kind", Type: ioengine.ColumnTypeString},
		{Key: "name", LabelKey: "accounting.report.name", Type: ioengine.ColumnTypeString, Weight: 1.8},
		{Key: "sku", LabelKey: "accounting.reports.sku", Type: ioengine.ColumnTypeString},
		moneyCol("quantity", "accounting.reports.quantity"),
		moneyCol("revenue", "accounting.reports.revenue"),
		cost("cost", "accounting.reports.cost"),
		cost("gross_profit", "accounting.reports.gross_profit"),
		cost("margin_pct", "accounting.reports.margin_pct"),
	}
}

// Export implements ioengine.ResourceAdapter. Query: from, to.
func (a *MarginAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	book, err := a.svc.jobBook(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	p, err := queryPeriod(q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rep, err := a.svc.marginReport(ctx, book, p, ioengine.Granted(q, rbac.PermPricingPurchaseRead))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	return MarginDataset(rep, loc), nil
}

func marginRow(kind, name, sku, qty string, f MarginFigures) map[string]any {
	return map[string]any{
		"kind": kind, "name": name, "sku": sku, "quantity": qty, "revenue": f.Revenue,
		"cost": strOrEmpty(f.Cost), "gross_profit": strOrEmpty(f.GrossProfit), "margin_pct": strOrEmpty(f.MarginPct),
	}
}

func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// MarginDataset maps a margin report to an export dataset: the services
// row first, then one row per product; the total is the summary row.
func MarginDataset(rep MarginReport, loc i18n.Locale) ioengine.Dataset {
	t := func(key string) string { return i18n.Translate(loc, key) }
	rows := make([]map[string]any, 0, len(rep.Products)+1)
	rows = append(rows, marginRow(t("accounting.reports.kind_service"), t("accounting.reports.services"), "",
		strconv.FormatInt(rep.Services.ServiceCount, 10), rep.Services.MarginFigures))
	for _, p := range rep.Products {
		rows = append(rows, marginRow(t("accounting.reports.kind_product_sale"), p.Name, p.SKU, p.Quantity, p.MarginFigures))
	}
	return ioengine.Dataset{
		Resource: ResourceMargin,
		Columns:  NewMarginAdapter(nil).ExportColumns(),
		Rows:     rows,
		Totals:   marginRow(t("accounting.report.total"), "", "", "", rep.Total),
		Info: []ioengine.InfoLine{
			{LabelKey: "accounting.statement.organization", Value: rep.Organization.Name},
			periodInfo(rep.From, rep.To, loc),
			{LabelKey: "accounting.statement.currency", Value: rep.Currency},
		},
		Doc: rep,
	}
}

// --- Cari aging ------------------------------------------------------------------

// CariAgingAdapter exports the cari aging report.
type CariAgingAdapter struct {
	exportOnly
	svc *Service
}

// NewCariAgingAdapter creates the cari aging export adapter.
func NewCariAgingAdapter(svc *Service) *CariAgingAdapter { return &CariAgingAdapter{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *CariAgingAdapter) Resource() string { return ResourceCariAging }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *CariAgingAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "side", LabelKey: "accounting.reports.side", Type: ioengine.ColumnTypeString},
		{Key: "counterparty", LabelKey: "accounting.statement.counterparty", Type: ioengine.ColumnTypeString, Weight: 1.8},
		{Key: "kind", LabelKey: "accounting.report.kind", Type: ioengine.ColumnTypeString},
		moneyCol("balance", "accounting.statement.balance"),
		moneyCol("days_0_30", "accounting.reports.days_0_30"),
		moneyCol("days_31_60", "accounting.reports.days_31_60"),
		moneyCol("days_61_90", "accounting.reports.days_61_90"),
		moneyCol("days_90_plus", "accounting.reports.days_90_plus"),
	}
}

// Export implements ioengine.ResourceAdapter. Query: as_of.
func (a *CariAgingAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	book, err := a.svc.jobBook(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	asOf, err := queryDay(q, QueryAsOf)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rep, err := a.svc.agingReport(ctx, book, asOf)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	return CariAgingDataset(rep, loc), nil
}

// CariAgingDataset maps an aging report to an export dataset: one row per
// open cari; the side totals are info lines (two sides do not fit one
// summary row).
func CariAgingDataset(rep AgingReport, loc i18n.Locale) ioengine.Dataset {
	t := func(key string) string { return i18n.Translate(loc, key) }
	rows := make([]map[string]any, 0, len(rep.Lines))
	for _, l := range rep.Lines {
		kind := t("accounting.reports.counterparty_organization")
		if l.Counterparty.Type == CounterpartyTypeUser {
			kind = t("accounting.reports.counterparty_customer")
		}
		rows = append(rows, map[string]any{
			"side": t("accounting.reports.side_" + l.Side), "counterparty": l.Counterparty.Name, "kind": kind,
			"balance": l.Balance, "days_0_30": l.Buckets.Days0To30, "days_31_60": l.Buckets.Days31To60,
			"days_61_90": l.Buckets.Days61To90, "days_90_plus": l.Buckets.Days90Plus,
		})
	}
	return ioengine.Dataset{
		Resource: ResourceCariAging,
		Columns:  NewCariAgingAdapter(nil).ExportColumns(),
		Rows:     rows,
		Info: []ioengine.InfoLine{
			{LabelKey: "accounting.statement.organization", Value: rep.Organization.Name},
			{LabelKey: "accounting.report.as_of", Value: rep.AsOf},
			{LabelKey: "accounting.statement.currency", Value: rep.Currency},
			{LabelKey: "accounting.report.receivable", Value: rep.Totals.Receivable.Balance},
			{LabelKey: "accounting.report.payable", Value: rep.Totals.Payable.Balance},
		},
		Doc: rep,
	}
}

// --- Staff cost --------------------------------------------------------------------

// StaffCostAdapter exports the staff cost report.
type StaffCostAdapter struct {
	exportOnly
	svc *Service
}

// NewStaffCostAdapter creates the staff cost export adapter.
func NewStaffCostAdapter(svc *Service) *StaffCostAdapter { return &StaffCostAdapter{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *StaffCostAdapter) Resource() string { return ResourceStaffCost }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *StaffCostAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "name", LabelKey: "accounting.report.name", Type: ioengine.ColumnTypeString, Weight: 1.6},
		{Key: "title", LabelKey: "accounting.reports.staff_title", Type: ioengine.ColumnTypeString},
		moneyCol("salary", "accounting.category.salary"),
		moneyCol("advance", "accounting.category.staff_advance"),
		moneyCol("bonus", "accounting.category.staff_bonus"),
		moneyCol("total", "accounting.report.total"),
		moneyCol("payment_count", "accounting.reports.payment_count"),
	}
}

// Export implements ioengine.ResourceAdapter. Query: from, to.
func (a *StaffCostAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	book, err := a.svc.jobBook(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	p, err := queryPeriod(q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rep, err := a.svc.staffCostReport(ctx, book, p)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	return StaffCostDataset(rep, loc), nil
}

// StaffCostDataset maps a staff cost report to an export dataset.
func StaffCostDataset(rep StaffCostReport, loc i18n.Locale) ioengine.Dataset {
	rows := make([]map[string]any, 0, len(rep.Lines))
	for _, l := range rep.Lines {
		rows = append(rows, map[string]any{
			"name": l.Name, "title": strOrEmpty(l.Title), "salary": l.Salary, "advance": l.Advance,
			"bonus": l.Bonus, "total": l.Total, "payment_count": strconv.FormatInt(l.PaymentCount, 10),
		})
	}
	return ioengine.Dataset{
		Resource: ResourceStaffCost,
		Columns:  NewStaffCostAdapter(nil).ExportColumns(),
		Rows:     rows,
		Totals: map[string]any{
			"name": i18n.Translate(loc, "accounting.report.total"), "salary": rep.Totals.Salary,
			"advance": rep.Totals.Advance, "bonus": rep.Totals.Bonus, "total": rep.Totals.Total,
		},
		Info: []ioengine.InfoLine{
			{LabelKey: "accounting.statement.organization", Value: rep.Organization.Name},
			periodInfo(rep.From, rep.To, loc),
			{LabelKey: "accounting.statement.currency", Value: rep.Currency},
		},
		Doc: rep,
	}
}
