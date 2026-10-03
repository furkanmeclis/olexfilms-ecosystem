package usecase

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
)

// TEC-207: the end-of-day report PDF. Same flow as the service PDF
// (TEC-196): an I/O engine export job (queue "exports", worker-docs)
// renders the stored report through Gotenberg in the job locale on the
// organization's letterhead (pdfrender skeleton: lang/dir, RTL for ar).

// ResourceEODPDF is the export resource of the report PDF
// (export_jobs.resource); the job is polled and downloaded through
// /v1/warehouse/eod-report-pdfs.
const ResourceEODPDF = "tenant.warehouse.eod_report.pdf"

// EODPDFQueryReportUUID is the export query key of the report, written by
// the HTTP handler after it authorized the request. The worker loads the
// report in the job organization only.
const EODPDFQueryReportUUID = "eod_report_uuid"

// EODPDFDoc is the data of one report PDF.
type EODPDFDoc struct {
	Report       EODReport
	Organization string
	GeneratedAt  time.Time
	Letterhead   ioengine.Letterhead
}

// EODPDF builds report PDFs.
type EODPDF struct {
	eod   *EOD
	store storage.Driver
	log   *slog.Logger
	now   func() time.Time
}

// NewEODPDF builds the report PDF use case; store loads the organization
// logo (nil prints the letterhead without logo).
func NewEODPDF(eod *EOD, store storage.Driver, log *slog.Logger) *EODPDF {
	if log == nil {
		log = slog.Default()
	}
	return &EODPDF{eod: eod, store: store, log: log, now: func() time.Time { return time.Now().UTC() }}
}

// Authorize checks a panel request: the report must belong to the active
// organization (else ErrEODReportNotFound).
func (p *EODPDF) Authorize(ctx context.Context, c Caller, id uuid.UUID) (EODReport, error) {
	return p.eod.Get(ctx, c, id)
}

// Build loads the PDF data of an export job: the report of the job
// organization.
func (p *EODPDF) Build(ctx context.Context, q ioengine.ExportQuery) (EODPDFDoc, error) {
	id, err := uuid.Parse(strings.TrimSpace(q[EODPDFQueryReportUUID]))
	if err != nil {
		return EODPDFDoc{}, ErrEODReportNotFound
	}
	jobOrg, err := strconv.ParseInt(strings.TrimSpace(q[ioengine.QueryOrganizationID]), 10, 64)
	if err != nil || jobOrg <= 0 {
		return EODPDFDoc{}, ErrEODReportNotFound
	}
	qs := p.eod.q
	row, err := qs.GetEODReportByUUID(ctx, db.GetEODReportByUUIDParams{Uuid: id, OrganizationID: jobOrg})
	if err != nil {
		return EODPDFDoc{}, notFound(err, ErrEODReportNotFound)
	}
	rep, err := p.eod.view(ctx, qs, row)
	if err != nil {
		return EODPDFDoc{}, err
	}
	org, err := qs.GetOrganizationByID(ctx, jobOrg)
	if err != nil {
		return EODPDFDoc{}, fmt.Errorf("warehouse eod pdf: organization: %w", err)
	}
	settings, err := qs.GetAppSettings(ctx)
	if err != nil {
		return EODPDFDoc{}, fmt.Errorf("warehouse eod pdf: settings: %w", err)
	}
	d := EODPDFDoc{Report: rep, Organization: org.Name, GeneratedAt: p.now(),
		Letterhead: ioengine.LetterheadFromOrganization(org, settings)}
	if p.store != nil {
		if lh, err := ioengine.LoadOrganizationLetterhead(ctx, p.store, org, settings); err != nil {
			p.log.Warn("eod_pdf_logo_failed", "error", err)
		} else {
			d.Letterhead = lh
		}
	}
	return d, nil
}

// --- Export adapter ------------------------------------------------------------

// EODPDFAdapter exports the report PDF (panel export job, read only).
type EODPDFAdapter struct{ svc *EODPDF }

// NewEODPDFAdapter builds the export adapter.
func NewEODPDFAdapter(svc *EODPDF) *EODPDFAdapter { return &EODPDFAdapter{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *EODPDFAdapter) Resource() string { return ResourceEODPDF }

// ExportColumns implements ioengine.ResourceAdapter (generic fallback: the
// group totals).
func (a *EODPDFAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "group", LabelKey: "warehouse.eod.group", Type: ioengine.ColumnTypeString, Weight: 1.4},
		{Key: "movements", LabelKey: "warehouse.eod.movements", Type: ioengine.ColumnTypeString},
		{Key: "quantity_in", LabelKey: "warehouse.eod.quantity_in", Type: ioengine.ColumnTypeString},
		{Key: "quantity_out", LabelKey: "warehouse.eod.quantity_out", Type: ioengine.ColumnTypeString},
		{Key: "meters_in", LabelKey: "warehouse.eod.meters_in", Type: ioengine.ColumnTypeString},
		{Key: "meters_out", LabelKey: "warehouse.eod.meters_out", Type: ioengine.ColumnTypeString},
	}
}

// ImportSchema implements ioengine.ResourceAdapter (export only).
func (a *EODPDFAdapter) ImportSchema() []ioengine.ImportField { return nil }

// ApplyRow implements ioengine.ResourceAdapter (export only).
func (a *EODPDFAdapter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "export only"}, nil
}

// RevertRow implements ioengine.ResourceAdapter (export only).
func (a *EODPDFAdapter) RevertRow(context.Context, string, string, map[string]any) error { return nil }

// Export implements ioengine.ResourceAdapter. Query: eod_report_uuid and
// the job organization injected by the worker.
func (a *EODPDFAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	d, err := a.svc.Build(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	return EODPDFDataset(d, loc), nil
}

// EODPDFDataset maps the report to an export dataset.
func EODPDFDataset(d EODPDFDoc, loc i18n.Locale) ioengine.Dataset {
	rows := make([]map[string]any, 0, len(d.Report.Summary.Groups))
	for _, g := range d.Report.Summary.Groups {
		rows = append(rows, map[string]any{
			"group": i18n.Translate(loc, "warehouse.eod.group."+g.Group), "movements": strconv.FormatInt(g.MovementCount, 10),
			"quantity_in": strconv.FormatInt(g.QuantityIn, 10), "quantity_out": strconv.FormatInt(g.QuantityOut, 10),
			"meters_in": g.MetersIn, "meters_out": g.MetersOut,
		})
	}
	return ioengine.Dataset{
		Resource: ResourceEODPDF,
		Columns:  (&EODPDFAdapter{}).ExportColumns(),
		Rows:     rows,
		Info: []ioengine.InfoLine{
			{LabelKey: "warehouse.eod.organization", Value: d.Organization},
			{LabelKey: "warehouse.eod.scope", Value: eodScopeLabel(d.Report, loc)},
			{LabelKey: "warehouse.eod.date", Value: d.Report.ReportDate},
		},
		Doc: d,
	}
}

func eodScopeLabel(r EODReport, loc i18n.Locale) string {
	if r.Warehouse == nil {
		return i18n.Translate(loc, "warehouse.eod.scope_system")
	}
	return r.Warehouse.Code + " · " + r.Warehouse.Name
}

// eodTypeLabel translates a movement type (warehouse.eod.type.<type>); an
// unknown type is printed as is.
func eodTypeLabel(t string, loc i18n.Locale) string {
	key := "warehouse.eod.type." + t
	if l := i18n.Translate(loc, key); l != key {
		return l
	}
	return t
}

// DocumentHTML implements ioengine.DocumentRenderer.
func (a *EODPDFAdapter) DocumentHTML(ds ioengine.Dataset, locale string, _ *ioengine.Letterhead, title string) (string, error) {
	d, ok := ds.Doc.(EODPDFDoc)
	if !ok {
		return "", errors.New("warehouse eod pdf: dataset carries no report")
	}
	return EODPDFHTML(d, i18n.Normalize(locale), title), nil
}

// EODPDFHTML renders the report document. Every value is escaped.
func EODPDFHTML(d EODPDFDoc, loc i18n.Locale, title string) string {
	t := func(key string) string { return i18n.Translate(loc, key) }
	esc := html.EscapeString
	r := d.Report
	s := r.Summary
	var b strings.Builder
	b.Grow(8192 + 512*len(s.Products))
	lh := d.Letterhead
	zone, _ := zoneOf(r.Timezone)
	period := r.PeriodStart.In(zone).Format("2006-01-02 15:04") + " – " + r.PeriodEnd.In(zone).Format("2006-01-02 15:04") + " (" + r.Timezone + ")"

	b.WriteString(eodPDFCSS)
	b.WriteString(ioengine.LetterheadHeaderHTML(&lh))
	b.WriteString(`<h1 class="doc-title">` + esc(title) + `</h1><table class="doc-meta">`)
	meta := func(label, value string, ltr bool) {
		dir := ""
		if ltr {
			dir = ` dir="ltr"`
		}
		b.WriteString(`<tr><th>` + esc(label) + `</th><td` + dir + `>` + esc(value) + `</td></tr>`)
	}
	meta(t("warehouse.eod.organization"), d.Organization, false)
	meta(t("warehouse.eod.scope"), eodScopeLabel(r, loc), false)
	meta(t("warehouse.eod.date"), r.ReportDate, true)
	meta(t("warehouse.eod.period"), period, true)
	meta(t("warehouse.eod.kind"), t("warehouse.eod.kind."+r.Kind), false)
	b.WriteString(`</table>`)

	head := func(first string) {
		b.WriteString(`<table class="eod"><thead><tr><th>` + esc(first) + `</th>`)
		for _, k := range []string{"movements", "units", "quantity_in", "quantity_out", "meters_in", "meters_out"} {
			b.WriteString(`<th class="num">` + esc(t("warehouse.eod."+k)) + `</th>`)
		}
		b.WriteString(`</tr></thead><tbody>`)
	}
	cells := func(x EODTotals) {
		for _, v := range []string{
			strconv.FormatInt(x.MovementCount, 10), strconv.FormatInt(x.UnitCount, 10),
			strconv.FormatInt(x.QuantityIn, 10), strconv.FormatInt(x.QuantityOut, 10), x.MetersIn, x.MetersOut,
		} {
			b.WriteString(`<td class="num" dir="ltr">` + esc(v) + `</td>`)
		}
		b.WriteString(`</tr>`)
	}

	b.WriteString(`<h2>` + esc(t("warehouse.eod.summary")) + `</h2>`)
	if s.Totals.MovementCount == 0 {
		b.WriteString(`<p class="muted">` + esc(t("warehouse.eod.empty")) + `</p>`)
	} else {
		head(t("warehouse.eod.group"))
		for _, g := range s.Groups {
			if g.MovementCount == 0 {
				continue
			}
			b.WriteString(`<tr><td>` + esc(t("warehouse.eod.group."+g.Group)) + `</td>`)
			cells(g.EODTotals)
		}
		b.WriteString(`<tr class="total"><td>` + esc(t("warehouse.eod.total")) + `</td>`)
		cells(s.Totals)
		b.WriteString(`</tbody></table>`)

		b.WriteString(`<h2>` + esc(t("warehouse.eod.by_type")) + `</h2>`)
		head(t("warehouse.eod.type"))
		for _, x := range s.Types {
			b.WriteString(`<tr><td>` + esc(eodTypeLabel(x.Type, loc)) + `</td>`)
			cells(x.EODTotals)
		}
		b.WriteString(`</tbody></table>`)

		b.WriteString(`<h2>` + esc(t("warehouse.eod.products")) + `</h2>`)
		head(t("warehouse.eod.product"))
		for _, p := range s.Products {
			b.WriteString(`<tr><td><strong>` + esc(p.ProductName) + `</strong>`)
			if p.SKU != "" {
				b.WriteString(` <span class="muted" dir="ltr">` + esc(p.SKU) + `</span>`)
			}
			b.WriteString(`<br><span class="muted">` + esc(eodTypeLabel(p.Type, loc)) + `</span></td>`)
			cells(p.EODTotals)
		}
		b.WriteString(`</tbody></table>`)
	}
	b.WriteString(`<p class="muted">` + esc(t("warehouse.eod.generated_at")) + `: ` +
		esc(r.GeneratedAt.UTC().Format("2006-01-02 15:04 UTC")) + `</p>`)
	b.WriteString(ioengine.LetterheadFooterHTML(&lh))
	return pdfrender.Document{Lang: string(loc), Title: title, Body: b.String(), PrimaryColor: lh.PrimaryColor}.HTML()
}

// eodPDFCSS lays out the report with logical properties, so the layout
// mirrors in RTL. The skeleton allows inline styles.
const eodPDFCSS = `<style>
table.eod{width:100%;border-collapse:collapse;font-size:9pt;margin-block-end:8pt}
table.eod th,table.eod td{border-block-end:1px solid #e5e7eb;padding:4pt;text-align:start;vertical-align:top}
table.eod thead th{background:#f3f4f6;border-block-end:2px solid var(--primary)}
table.eod .num{text-align:end;white-space:nowrap}
table.eod tr.total td{font-weight:700;border-block-start:2px solid #9ca3af}
</style>`
