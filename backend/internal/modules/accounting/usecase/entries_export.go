package usecase

// TEC-379 (DT-BE-8): ledger entry list export (I/O engine). The HTTP
// handler resolves the book like GET /v1/accounting/entries (accounting.read
// scope) and stores it as book_organization_id next to the list
// parameters; the worker re-checks the book against the job organization
// (exportBook) and reads the parameters with ParseEntryFilter, so the file
// holds the rows of the list (filters, q and sort). CSV/XLSX/PDF use the
// generic encoders.

import (
	"context"
	"strconv"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
)

// ResourceEntries is the export resource of the entry list.
const ResourceEntries = "tenant.accounting.entries"

const entryExportPage = 500

// EntryExportQuery validates the list parameters of an export body
// (400 on a bad value) and returns the job query of the book.
func EntryExportQuery(bookID int64, body map[string]string) (ioengine.ExportQuery, error) {
	values := EntryListValues(body)
	if _, err := ParseEntryFilter(values); err != nil {
		return nil, err
	}
	out := ioengine.ExportQuery{QueryBookOrganizationID: strconv.FormatInt(bookID, 10)}
	for k := range values {
		out[k] = values.Get(k)
	}
	return out, nil
}

// EntriesAdapter exports the entry list of a book.
type EntriesAdapter struct {
	exportOnly
	svc *Service
}

// NewEntriesAdapter creates the entry list export adapter.
func NewEntriesAdapter(svc *Service) *EntriesAdapter { return &EntriesAdapter{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *EntriesAdapter) Resource() string { return ResourceEntries }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *EntriesAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "date", LabelKey: "accounting.statement.date", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "direction", LabelKey: "accounting.export.direction", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "category", LabelKey: "accounting.reports.category", Type: ioengine.ColumnTypeString, Weight: 1.2},
		{Key: "account", LabelKey: "accounting.export.account", Type: ioengine.ColumnTypeString},
		{Key: "counterparty", LabelKey: "accounting.statement.counterparty", Type: ioengine.ColumnTypeString, Weight: 1.2},
		{Key: "description", LabelKey: "accounting.statement.description", Type: ioengine.ColumnTypeString, Weight: 1.6},
		{Key: "source", LabelKey: "accounting.statement.source", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "orig_amount", LabelKey: "accounting.export.orig_amount", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "orig_currency", LabelKey: "accounting.export.orig_currency", Type: ioengine.ColumnTypeString, Weight: 0.5},
		{Key: "amount", LabelKey: "accounting.export.amount", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "currency", LabelKey: "accounting.statement.currency", Type: ioengine.ColumnTypeString, Weight: 0.5},
	}
}

// Export implements ioengine.ResourceAdapter.
func (a *EntriesAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	book, err := a.svc.exportBook(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f, err := ParseEntryFilter(EntryListValues(q))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f.Limit, f.Offset = entryExportPage, 0
	t := func(key string) string { return i18n.Translate(loc, key) }
	rows := []map[string]any{}
	for {
		page, total, err := a.svc.searchEntries(ctx, book.ID, f)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, e := range page {
			row := map[string]any{
				"date":      e.CreatedAt.UTC().Format(time.DateOnly),
				"direction": t("accounting.direction." + e.Direction), "category": t(e.CategoryLabelKey),
				"account": "", "counterparty": "", "description": "", "source": "",
				"orig_amount": e.OrigAmount, "orig_currency": e.OrigCurrency,
				"amount": e.Amount, "currency": e.Currency,
			}
			if e.Account != nil {
				row["account"] = e.Account.Name
			}
			if e.CounterpartyOrganization != nil {
				row["counterparty"] = e.CounterpartyOrganization.Name
			}
			if e.Description != nil {
				row["description"] = *e.Description
			}
			if e.SourceType != nil {
				row["source"] = SourceLabel(loc, *e.SourceType)
			}
			rows = append(rows, row)
		}
		f.Offset += int32(len(page)) //nolint:gosec // bounded by entryExportPage
		if len(page) == 0 || int64(f.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{
		Resource: ResourceEntries, Columns: a.ExportColumns(), Rows: rows,
		Info: []ioengine.InfoLine{
			{LabelKey: "accounting.statement.organization", Value: book.Name},
			{LabelKey: "accounting.statement.currency", Value: book.Currency},
		},
	}, nil
}
