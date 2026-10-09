package usecase

// TEC-506 (F5-09b): io engine adapters of the recommended prices: the
// staged CSV / XLSX publication import (preview = dry run, confirm = one
// publication batch whose batch_id is the import job) and the price
// discipline list export.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Resources of the io engine.
const (
	ResourceRecommendedImport = "tenant.pricing.recommended"
	ResourceDisciplineExport  = "pricing.discipline"
)

// ImportDefaultEffectiveFrom / ImportDefaultNote are the job defaults (the
// day of rows without effective_from, the version note).
const (
	ImportDefaultEffectiveFrom = "effective_from"
	ImportDefaultNote          = "note"
)

// Row statuses of the import.
const (
	ImportRowNew       = "new"
	ImportRowDuplicate = "duplicate"
	ImportRowInvalid   = "invalid"
	ImportRowApplied   = "applied"
)

// ImportErrorKey is the i18n key of a row error code.
func ImportErrorKey(code string) string {
	return "pricing_import.error." + strings.ToLower(strings.TrimPrefix(code, "PRICING_IMPORT_"))
}

// ImportStatusKey is the i18n key of a row status.
func ImportStatusKey(status string) string { return "pricing_import.status." + status }

// RecommendedImporter is the staged recommended price import.
type RecommendedImporter struct{ svc *Recommended }

// NewRecommendedImporter creates the importer.
func NewRecommendedImporter(svc *Recommended) *RecommendedImporter {
	return &RecommendedImporter{svc: svc}
}

var (
	_ ioengine.ResourceAdapter = (*RecommendedImporter)(nil)
	_ ioengine.StagedImporter  = (*RecommendedImporter)(nil)
)

// Resource implements ioengine.ResourceAdapter.
func (im *RecommendedImporter) Resource() string { return ResourceRecommendedImport }

// ExportColumns implements ioengine.ResourceAdapter (import only).
func (im *RecommendedImporter) ExportColumns() []ioengine.Column { return nil }

// Export implements ioengine.ResourceAdapter (import only).
func (im *RecommendedImporter) Export(context.Context, ioengine.ExportQuery, i18n.Locale) (ioengine.Dataset, error) {
	return ioengine.Dataset{}, errors.New("recommended price import is import only")
}

// ImportSchema implements ioengine.ResourceAdapter.
func (im *RecommendedImporter) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "sku", LabelKey: "pricing_import.product", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "country", LabelKey: "pricing_import.country", Type: ioengine.ColumnTypeString},
		{Key: "currency", LabelKey: "pricing_import.currency", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "price", LabelKey: "pricing_import.price", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "effective_from", LabelKey: "pricing_import.effective_from", Type: ioengine.ColumnTypeString},
	}
}

// SampleRows is the sample file content.
func (im *RecommendedImporter) SampleRows() []map[string]any {
	return []map[string]any{
		{"sku": "PPF-190", "country": "TR", "currency": "TRY", "price": "45000.00", "effective_from": ""},
		{"sku": "PPF-190", "country": "", "currency": "EUR", "price": "1250.00", "effective_from": "2026-11-01"},
	}
}

// ApplyRow implements ioengine.ResourceAdapter (staged; never called).
func (im *RecommendedImporter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "recommended price import is staged"}, nil
}

// RevertRow implements ioengine.ResourceAdapter (staged; never called).
func (im *RecommendedImporter) RevertRow(context.Context, string, string, map[string]any) error {
	return errors.New("recommended price import is staged")
}

func rejected(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ioengine.ErrImportRejected}, args...)...)
}

func cell(row map[string]any, key string) string {
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

// importCenter is the job organization; only the brand center publishes.
func (im *RecommendedImporter) importCenter(ctx context.Context, q *db.Queries, job ioengine.ImportJob) (db.Organization, error) {
	org, err := q.GetOrganizationByID(ctx, job.OrganizationID)
	if err != nil {
		return db.Organization{}, rejected("the job organization is unknown")
	}
	if org.Type != OrgCenter {
		return db.Organization{}, rejected("only the brand center publishes recommended prices")
	}
	return org, nil
}

func defaultString(defaults map[string]any, key string) string {
	if v, ok := defaults[key]; ok && v != nil {
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return ""
}

func rowData(row map[string]any) map[string]any {
	return map[string]any{
		"sku": cell(row, "sku"), "country": strings.ToUpper(cell(row, "country")),
		"currency": strings.ToUpper(cell(row, "currency")), "price": cell(row, "price"),
		"effective_from": cell(row, "effective_from"),
	}
}

func publishRowOf(data map[string]any) PublishRow {
	str := func(k string) string { s, _ := data[k].(string); return s }
	return PublishRow{
		SKU: str("sku"), Country: str("country"), Currency: str("currency"), Price: str("price"),
		EffectiveFrom: str("effective_from"),
	}
}

// classify validates the rows against today; duplicates of an earlier
// valid row are marked duplicate.
func (im *RecommendedImporter) classify(ctx context.Context, q *db.Queries, center db.Organization, day time.Time, data []map[string]any, loc i18n.Locale) (ioengine.PreviewSummary, []preparedRow, error) {
	today := LocalDay(im.svc.now(), center.Timezone)
	sum := ioengine.PreviewSummary{Total: len(data), Counts: map[string]int{}}
	countries := map[string]int64{}
	seen := map[string]bool{}
	var valid []preparedRow
	for i, d := range data {
		row, code, err := im.svc.prepare(ctx, q, center.BrandID, today, day, publishRowOf(d), countries)
		if err != nil {
			return sum, nil, err
		}
		status := ImportRowNew
		if code == "" && seen[row.key()] {
			code = RowErrDuplicate
		}
		target := map[string]any{}
		switch code {
		case "":
			seen[row.key()] = true
			valid = append(valid, row)
			sum.Valid++
			target["product_uuid"] = row.productUUID.String()
			target["effective_from"] = row.effective.Format(time.DateOnly)
		case RowErrDuplicate:
			status = ImportRowDuplicate
		default:
			status = ImportRowInvalid
		}
		if code != "" {
			sum.Invalid++
			sum.Errors = append(sum.Errors, ioengine.RowError{
				Index: i + 1, Field: rowErrField[code], Code: code, Error: i18n.Translate(loc, ImportErrorKey(code)),
			})
		}
		sum.Counts[status]++
		sum.Rows = append(sum.Rows, ioengine.RowPreview{
			Index: i + 1, Data: d, Status: status, StatusLabel: i18n.Translate(loc, ImportStatusKey(status)), Target: target,
		})
	}
	return sum, valid, nil
}

// Stage implements ioengine.StagedImporter: the dry run. Nothing is written.
func (im *RecommendedImporter) Stage(ctx context.Context, job ioengine.ImportJob, rows []map[string]any, defaults map[string]any) (ioengine.PreviewSummary, error) {
	if len(rows) > MaxPublishRows {
		return ioengine.PreviewSummary{}, rejected("at most %d rows per file", MaxPublishRows)
	}
	q := im.svc.q
	center, err := im.importCenter(ctx, q, job)
	if err != nil {
		return ioengine.PreviewSummary{}, err
	}
	day, err := batchDay(defaultString(defaults, ImportDefaultEffectiveFrom), LocalDay(im.svc.now(), center.Timezone))
	if err != nil {
		return ioengine.PreviewSummary{}, rejected("%v", err)
	}
	data := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		data = append(data, rowData(r))
	}
	sum, _, err := im.classify(ctx, q, center, day, data, i18n.Normalize(job.Locale))
	if err != nil {
		return ioengine.PreviewSummary{}, err
	}
	sum.BatchUUID = job.UUID.String()
	return sum, nil
}

// Apply implements ioengine.StagedImporter: the valid rows are re-checked
// and published as one batch (batch_id = the job) in one transaction. A
// second run returns the published summary and writes nothing.
func (im *RecommendedImporter) Apply(ctx context.Context, job ioengine.ImportJob) (ioengine.PreviewSummary, error) {
	var out ioengine.PreviewSummary
	err := im.svc.inTx(ctx, func(tx pgx.Tx, q *db.Queries) error {
		row, err := q.GetImportJobByID(ctx, job.ID)
		if err != nil {
			return fmt.Errorf("pricing import: job: %w", err)
		}
		var staged ioengine.PreviewSummary
		if len(row.PreviewJson) == 0 || json.Unmarshal(row.PreviewJson, &staged) != nil || staged.BatchUUID == "" {
			return rejected("the import has no preview")
		}
		center, err := im.importCenter(ctx, q, job)
		if err != nil {
			return err
		}
		loc := i18n.Normalize(job.Locale)
		published, err := q.ListRecommendedPriceVersionsByBatch(ctx, db.ListRecommendedPriceVersionsByBatchParams{
			BrandID: center.BrandID, BatchID: job.UUID,
		})
		if err != nil {
			return fmt.Errorf("pricing import: batch: %w", err)
		}
		if len(published) > 0 {
			out = staged
			return nil
		}
		defaults := map[string]any{}
		_ = json.Unmarshal(row.DefaultsJson, &defaults)
		day, err := batchDay(defaultString(defaults, ImportDefaultEffectiveFrom), LocalDay(im.svc.now(), center.Timezone))
		if err != nil {
			return rejected("%v", err)
		}
		data := make([]map[string]any, 0, len(staged.Rows))
		for _, r := range staged.Rows {
			data = append(data, r.Data)
		}
		sum, valid, err := im.classify(ctx, q, center, day, data, loc)
		if err != nil {
			return err
		}
		if len(valid) > 0 {
			if _, err := im.svc.publishTx(ctx, tx, q, center, job.ActorID, job.UUID, model.SourcePublish,
				defaultString(defaults, ImportDefaultNote), valid, LocalDay(im.svc.now(), center.Timezone)); err != nil {
				return err
			}
		}
		sum.Counts = map[string]int{}
		for i := range sum.Rows {
			if sum.Rows[i].Status == ImportRowNew {
				sum.Rows[i].Status = ImportRowApplied
				sum.Rows[i].StatusLabel = i18n.Translate(loc, ImportStatusKey(ImportRowApplied))
			}
			sum.Counts[sum.Rows[i].Status]++
		}
		sum.BatchUUID = job.UUID.String()
		out = sum
		return nil
	})
	return out, err
}

// Undo implements ioengine.StagedImporter: published versions are
// append-only, so an import is never undone (publish again to correct).
func (im *RecommendedImporter) Undo(_ context.Context, job ioengine.ImportJob) (ioengine.PreviewSummary, error) {
	return ioengine.PreviewSummary{}, rejected("%s", i18n.Translate(i18n.Normalize(job.Locale), ImportErrorKey("PRICING_IMPORT_UNDO_NOT_SUPPORTED")))
}

// --- Discipline export -------------------------------------------------------------

// DisciplineExportQuery builds the export query: the caller scope and the
// list filters of GET /v1/pricing/discipline.
func DisciplineExportQuery(scope DisciplineScope, values url.Values) ioengine.ExportQuery {
	out := ioengine.ExportQuery{"brand_id": strconv.FormatInt(scope.BrandID, 10)}
	if scope.OrgIDs != nil {
		ids := make([]string, 0, len(scope.OrgIDs))
		for _, id := range scope.OrgIDs {
			ids = append(ids, strconv.FormatInt(id, 10))
		}
		out["scope_org_ids"] = strings.Join(ids, ",")
	}
	for k := range values {
		out[k] = values.Get(k)
	}
	return out
}

// DisciplineExportAdapter exports the deviation list (CSV / XLSX / PDF).
type DisciplineExportAdapter struct {
	svc   *Recommended
	parse func(url.Values) (DisciplineFilter, error)
}

// NewDisciplineExportAdapter creates the export adapter; parse reads the
// list filters (the handler's parser).
func NewDisciplineExportAdapter(svc *Recommended, parse func(url.Values) (DisciplineFilter, error)) *DisciplineExportAdapter {
	return &DisciplineExportAdapter{svc: svc, parse: parse}
}

func (a *DisciplineExportAdapter) Resource() string                     { return ResourceDisciplineExport }
func (a *DisciplineExportAdapter) ImportSchema() []ioengine.ImportField { return nil }
func (a *DisciplineExportAdapter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "export only"}, nil
}
func (a *DisciplineExportAdapter) RevertRow(context.Context, string, string, map[string]any) error {
	return nil
}

// ExportColumns implements ioengine.ResourceAdapter.
func (a *DisciplineExportAdapter) ExportColumns() []ioengine.Column {
	s := ioengine.ColumnTypeString
	return []ioengine.Column{
		{Key: "snapshot_date", LabelKey: "pricing.discipline.snapshot_date", Type: s},
		{Key: "organization", LabelKey: "pricing.discipline.organization", Type: s},
		{Key: "organization_type", LabelKey: "pricing.discipline.organization_type", Type: s},
		{Key: "sku", LabelKey: "pricing.discipline.sku", Type: s},
		{Key: "product", LabelKey: "pricing.discipline.product", Type: s},
		{Key: "country", LabelKey: "pricing.discipline.country", Type: s},
		{Key: "currency", LabelKey: "pricing.discipline.currency", Type: s},
		{Key: "recommended_price", LabelKey: "pricing.discipline.recommended_price", Type: s},
		{Key: "list_price", LabelKey: "pricing.discipline.list_price", Type: s},
		{Key: "deviation_pct", LabelKey: "pricing.discipline.deviation_pct", Type: s},
		{Key: "avg_sale_price", LabelKey: "pricing.discipline.avg_sale_price", Type: s},
		{Key: "sales_quantity", LabelKey: "pricing.discipline.sales_quantity", Type: s},
		{Key: "over_threshold", LabelKey: "pricing.discipline.over_threshold", Type: s},
	}
}

// Export implements ioengine.ResourceAdapter.
func (a *DisciplineExportAdapter) Export(ctx context.Context, q ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	brandID, err := strconv.ParseInt(q["brand_id"], 10, 64)
	if err != nil || brandID <= 0 {
		return ioengine.Dataset{}, errors.New("pricing discipline export: brand is required")
	}
	orgID, _ := strconv.ParseInt(q[ioengine.QueryOrganizationID], 10, 64)
	if orgID > 0 {
		org, err := a.svc.q.GetOrganizationByID(ctx, orgID)
		if err != nil || org.BrandID != brandID {
			return ioengine.Dataset{}, errors.New("pricing discipline export: organization outside the brand")
		}
	}
	scope := DisciplineScope{BrandID: brandID}
	if raw := strings.TrimSpace(q["scope_org_ids"]); raw != "" {
		scope.OrgIDs = []int64{}
		for _, p := range strings.Split(raw, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
			if err != nil {
				return ioengine.Dataset{}, errors.New("pricing discipline export: invalid scope")
			}
			scope.OrgIDs = append(scope.OrgIDs, id)
		}
	}
	values := url.Values{}
	for k, v := range q {
		values.Set(k, v)
	}
	f, err := a.parse(values)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f.Limit, f.Offset = 500, 0
	var rows []map[string]any
	for {
		page, total, _, _, err := a.svc.ListDiscipline(ctx, scope, f)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, r := range page {
			dev, avg := "", ""
			if r.DeviationPct != nil {
				dev = *r.DeviationPct
			}
			if r.AvgSalePrice != nil {
				avg = *r.AvgSalePrice
			}
			rows = append(rows, map[string]any{
				"snapshot_date": r.SnapshotDate, "organization": r.OrganizationName, "organization_type": r.OrganizationType,
				"sku": r.ProductSKU, "product": r.ProductName, "country": r.CountryISO2, "currency": r.Currency,
				"recommended_price": r.RecommendedPrice, "list_price": r.ListPrice, "deviation_pct": dev,
				"avg_sale_price": avg, "sales_quantity": r.SalesQuantity, "over_threshold": strconv.FormatBool(r.OverThreshold),
			})
		}
		f.Offset += int32(len(page)) //nolint:gosec
		if len(page) == 0 || int64(f.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Resource: ResourceDisciplineExport, Columns: a.ExportColumns(), Rows: rows}, nil
}

// ParseDisciplineFilter reads the list filters of GET /v1/pricing/discipline
// (also the export query).
func ParseDisciplineFilter(values url.Values) (DisciplineFilter, error) {
	q := apiquery.Parse(values)
	f := DisciplineFilter{Q: q.Q, Sort: q.Sort, Limit: q.Limit, Offset: q.Offset}
	if raw := strings.TrimSpace(values.Get("date")); raw != "" {
		d, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			return f, invalid("date", "must be YYYY-MM-DD")
		}
		f.Date = &d
	}
	f.Countries = apiquery.CSVValues(values, "country")
	for _, raw := range apiquery.CSVValues(values, "currency") {
		cur, err := NormalizeCurrency(raw)
		if err != nil {
			return f, err
		}
		f.Currencies = append(f.Currencies, cur)
	}
	for _, name := range []string{"product", "distributor"} {
		for _, raw := range apiquery.CSVValues(values, name) {
			id, err := uuid.Parse(raw)
			if err != nil {
				return f, invalid(name, "must be a list of UUIDs")
			}
			if name == "product" {
				f.ProductUUIDs = append(f.ProductUUIDs, id)
			} else {
				f.DistributorUUID = append(f.DistributorUUID, id)
			}
		}
	}
	if raw := strings.TrimSpace(values.Get("deviation_pct_min")); raw != "" {
		f.DeviationMin = &raw
	}
	if raw := strings.TrimSpace(values.Get("deviation_pct_max")); raw != "" {
		f.DeviationMax = &raw
	}
	switch strings.TrimSpace(values.Get("over_threshold")) {
	case "":
	case "true":
		t := true
		f.OverThreshold = &t
	case "false":
		b := false
		f.OverThreshold = &b
	default:
		return f, invalid("over_threshold", "must be true or false")
	}
	if _, err := apiquery.ResolveSort(f.Sort, DisciplineSort); err != nil {
		return f, err
	}
	return f, nil
}
