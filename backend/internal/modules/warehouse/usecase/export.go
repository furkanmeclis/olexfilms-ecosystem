package usecase

// TEC-761 (F2.3): warehouse exports through the I/O engine (queue
// "exports"). Every list screen of the warehouse module queues a job with
// its list parameters (filters, q, sort) and, when rows are selected, their
// uuids (query key "uuid", CSV): the export then holds only those rows.
// The job stores the caller's resolved warehouse.read scope
// (ioengine.EncodeScopeFilter); the worker rebuilds the caller against the
// job organization (ioengine.JobScopeFilter) and reads through the same
// use cases as the lists, so the module guard (center / distributor, K12)
// and the active organization's own warehouses apply again. Names replace
// ids and enums are translated (backend catalog warehouse.export.*).

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Export resources (export_jobs.resource).
const (
	ResourceCountReportExport     = "warehouse.stock_count_report"
	ResourceCountsExport          = "warehouse.stock_counts"
	ResourceEntriesExport         = "warehouse.stock_entries"
	ResourceTransfersExport       = "warehouse.transfers"
	ResourceTransferSummaryExport = "warehouse.transfer_summary"
	ResourceLocationsExport       = "warehouse.locations"
	ResourceEODListExport         = "warehouse.eod_reports"
)

// Export query keys. QueryExportUUIDs is the client's row selection (CSV);
// the underscore keys are set by the server only.
const (
	QueryExportUUIDs = "uuid"
	queryCountUUID   = "_count_uuid"
)

// exportPage is the page size the exports read the lists with.
const exportPage = 200

// ErrExportResource: the resource is not a warehouse list export.
var ErrExportResource = errors.New("warehouse: unknown export resource")

// Exporter builds the warehouse export jobs and their datasets.
type Exporter struct {
	q         *db.Queries
	counts    *Counts
	entries   *StockEntries
	transfers *WarehouseTransfers
	eod       *EOD
}

// NewExporter builds the exporter (read only: no outbox, no scanner).
func NewExporter(pool TxBeginner, q *db.Queries) *Exporter {
	return &Exporter{
		q: q, counts: NewCounts(pool, q, nil, nil), entries: NewStockEntries(pool, q, nil),
		transfers: NewWarehouseTransfers(pool, q, nil), eod: NewEOD(pool, q),
	}
}

// ListExportQuery builds the job query of a list export of resource: the
// list parameters of body, validated with the list parser of the resource
// (a bad value is a 400 at request time), plus the caller's scope.
func (x *Exporter) ListExportQuery(c Caller, resource string, body map[string]string) (ioengine.ExportQuery, error) {
	if _, err := guard(c); err != nil {
		return nil, err
	}
	values := exportValues(body)
	if err := validateExportValues(resource, values); err != nil {
		return nil, err
	}
	out, err := scopedQuery(c)
	if err != nil {
		return nil, err
	}
	for k := range values {
		out[k] = values.Get(k)
	}
	return out, nil
}

// CountReportExportQuery builds the job query of a count report export:
// the count must be completed (as for GET .../report).
func (x *Exporter) CountReportExportQuery(ctx context.Context, c ScanCaller, id uuid.UUID) (ioengine.ExportQuery, error) {
	if _, err := x.counts.Report(ctx, c, id); err != nil {
		return nil, err
	}
	out, err := scopedQuery(c.Caller)
	if err != nil {
		return nil, err
	}
	out[queryCountUUID] = id.String()
	return out, nil
}

func scopedQuery(c Caller) (ioengine.ExportQuery, error) {
	scope, ok := ioengine.EncodeScopeFilter(c.Filter)
	if !ok {
		return nil, ErrForbidden
	}
	return ioengine.ExportQuery{ioengine.QueryScopeFilter: scope}, nil
}

func validateExportValues(resource string, values url.Values) error {
	if _, err := uuidList(values, QueryExportUUIDs); err != nil {
		return err
	}
	var err error
	switch resource {
	case ResourceEntriesExport:
		_, err = ParseEntryListFilter(values)
	case ResourceCountsExport:
		_, err = ParseCountListFilter(values)
	case ResourceTransfersExport:
		_, err = ParseTransferListFilter(values)
	case ResourceTransferSummaryExport:
		if _, err = ParseTransferListFilter(values); err == nil {
			_, err = uuidList(values, "product_uuid")
		}
	case ResourceLocationsExport:
		_, err = parseLocationExportFilter(values)
	case ResourceEODListExport:
		_, err = ParseEODListInput(values)
	default:
		err = ErrExportResource
	}
	return err
}

// exportValues keeps the client keys of a query (the underscore keys are
// the server's).
func exportValues(m map[string]string) url.Values {
	v := url.Values{}
	for k, val := range m {
		if strings.HasPrefix(k, "_") || strings.TrimSpace(val) == "" {
			continue
		}
		v.Set(k, val)
	}
	return v
}

// jobCaller rebuilds the caller of an export job: the job organization and
// the stored scope re-authorized against it.
func (x *Exporter) jobCaller(ctx context.Context, q ioengine.ExportQuery) (Caller, error) {
	orgID, err := strconv.ParseInt(q[ioengine.QueryOrganizationID], 10, 64)
	if err != nil || orgID <= 0 {
		return Caller{}, errors.New("warehouse export: job organization is required")
	}
	org, err := x.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return Caller{}, fmt.Errorf("warehouse export: organization: %w", err)
	}
	f, err := ioengine.JobScopeFilter(ctx, x.q, org, q[ioengine.QueryScopeFilter])
	if err != nil {
		return Caller{}, err
	}
	return Caller{
		Org:    orgctx.Scope{InternalID: org.ID, UUID: org.Uuid, Name: org.Name, OrgType: org.Type, BrandID: org.BrandID},
		Filter: f,
	}, nil
}

// selection is the selected row set of an export (nil: every row).
type selection map[uuid.UUID]bool

func selectedRows(values url.Values) selection {
	ids, _ := uuidList(values, QueryExportUUIDs)
	if len(ids) == 0 {
		return nil
	}
	s := make(selection, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}

func (s selection) has(id uuid.UUID) bool { return s == nil || s[id] }

// exportOnly implements the import half of ioengine.ResourceAdapter.
type exportOnly struct{}

func (exportOnly) ImportSchema() []ioengine.ImportField { return nil }
func (exportOnly) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "export only"}, nil
}
func (exportOnly) RevertRow(context.Context, string, string, map[string]any) error { return nil }

// exportAdapter is one warehouse export resource.
type exportAdapter struct {
	exportOnly
	resource string
	columns  []ioengine.Column
	build    func(ctx context.Context, c Caller, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error)
	x        *Exporter
}

func (a *exportAdapter) Resource() string                 { return a.resource }
func (a *exportAdapter) ExportColumns() []ioengine.Column { return a.columns }

func (a *exportAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	c, err := a.x.jobCaller(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	ds, err := a.build(ctx, c, q, loc)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	ds.Resource, ds.Columns = a.resource, a.columns
	if ds.Rows == nil {
		ds.Rows = []map[string]any{}
	}
	return ds, nil
}

// Adapters returns the warehouse export adapters (register them on the
// API and the worker registry).
func (x *Exporter) Adapters() []ioengine.ResourceAdapter {
	return []ioengine.ResourceAdapter{
		&exportAdapter{resource: ResourceCountReportExport, columns: countReportColumns, build: x.countReport, x: x},
		&exportAdapter{resource: ResourceCountsExport, columns: countListColumns, build: x.countList, x: x},
		&exportAdapter{resource: ResourceEntriesExport, columns: entryColumns, build: x.entryList, x: x},
		&exportAdapter{resource: ResourceTransfersExport, columns: transferColumns, build: x.transferList, x: x},
		&exportAdapter{resource: ResourceTransferSummaryExport, columns: transferSummaryColumns, build: x.transferSummary, x: x},
		&exportAdapter{resource: ResourceLocationsExport, columns: locationColumns, build: x.locationList, x: x},
		&exportAdapter{resource: ResourceEODListExport, columns: eodListColumns, build: x.eodList, x: x},
	}
}

// --- cell helpers ------------------------------------------------------------

// tr translates key; an unknown key falls back to the raw value.
func tr(loc i18n.Locale, prefix, value string) string {
	if value == "" {
		return ""
	}
	key := prefix + value
	if s := i18n.Translate(loc, key); s != key {
		return s
	}
	return value
}

func label(loc i18n.Locale, key string) string { return i18n.Translate(loc, key) }

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func txt(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// when returns a datetime cell value (nil prints empty).
func when(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC()
}

func whenTS(t pgtype.Timestamptz) any {
	if !t.Valid {
		return nil
	}
	return t.Time.UTC()
}

// dateRange renders a created_from / created_to filter for an info line.
func dateRange(values url.Values) string {
	from, to := strings.TrimSpace(values.Get("created_from")), strings.TrimSpace(values.Get("created_to"))
	if from == "" && to == "" {
		return ""
	}
	return from + " – " + to
}

func warehouseLabel(code, name string) string {
	if code == "" {
		return name
	}
	return code + " · " + name
}

func exportValuesOf(q ioengine.ExportQuery) url.Values { return exportValues(q) }
