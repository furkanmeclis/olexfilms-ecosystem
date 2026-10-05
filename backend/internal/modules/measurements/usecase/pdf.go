package usecase

// TEC-298 (F3-02f): the timestamped measurement PDF.
//
// A measurement does not change once it is accepted and normalized, so its
// PDF is rendered once on worker-docs (task measurement:pdf), stored and
// remembered in measurement_results.pdf_key; every later request streams the
// stored object. The document is the active "measurement" template
// (documents module, seed 000030) on the organization letterhead, in the
// organization locale, with measured_at in the organization timezone, the
// VIN, the plate, the device serial, the reading tables, the tires, the
// legacy NexPTG part map (measurements/svg) and the generation timestamp.
//
// The service PDF (TEC-196) does not embed a before/after summary: it is
// handed to the customer and measurements are closed to customers (K28), so
// only this separate panel PDF exists. There is no portal route.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/svg"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// MeasurementDocumentSourceType is document_renders.source_type of the
// measurement kind.
const MeasurementDocumentSourceType = "measurement_result"

var (
	// ErrPDFVINPending: a vin_pending measurement may still change (VIN
	// completion), so it has no PDF yet (422 MEASUREMENT_VIN_PENDING).
	ErrPDFVINPending = errors.New("measurements: the measurement has no VIN yet")
	// ErrPDFNotNormalized: the raw upload is not normalized, the PDF would
	// have no readings (422 MEASUREMENT_NOT_NORMALIZED).
	ErrPDFNotNormalized = errors.New("measurements: the measurement is not normalized")
	// ErrPDFUnavailable: no PDF renderer or storage is configured (503).
	ErrPDFUnavailable = errors.New("measurements: pdf renderer unavailable")
)

// PDFRenderer converts HTML to PDF (*pdfrender.Client).
type PDFRenderer interface {
	Convert(ctx context.Context, req pdfrender.Request) ([]byte, error)
}

// PDFEnqueuer schedules the worker-docs task (*queue.Client).
type PDFEnqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// PDFService renders and serves measurement PDFs.
type PDFService struct {
	tx    TxBeginner
	q     *db.Queries
	store storage.Driver
	pdf   PDFRenderer
	queue PDFEnqueuer
	fonts pdfrender.FontMode
	now   func() time.Time
	log   *slog.Logger
}

// NewPDF builds the measurement PDF use case. With a nil queue the PDF is
// rendered inside the request (tests, in-process setups).
func NewPDF(tx TxBeginner, q *db.Queries, store storage.Driver, pdf PDFRenderer, enq PDFEnqueuer, fonts pdfrender.FontMode, log *slog.Logger) *PDFService {
	if log == nil {
		log = slog.Default()
	}
	if fonts == "" {
		fonts = pdfrender.FontsEmbedded
	}
	return &PDFService{tx: tx, q: q, store: store, pdf: pdf, queue: enq, fonts: fonts, now: time.Now, log: log}
}

// SetClock overrides the generation timestamp source (tests).
func (p *PDFService) SetClock(now func() time.Time) { p.now = now }

// PDFFile is a ready PDF; Pending is set while worker-docs renders it.
type PDFFile struct {
	Pending  bool
	UUID     uuid.UUID
	Body     io.ReadCloser
	Filename string
}

// RequestPDF is GET /v1/measurements/{uuid}/pdf: the stored PDF of a
// measurement inside the caller's measurements.read scope, or a queued
// render (Pending) on the first request.
func (p *PDFService) RequestPDF(ctx context.Context, c PanelCaller, id uuid.UUID) (PDFFile, error) {
	row, err := p.q.GetMeasurementResultPanel(ctx, db.GetMeasurementResultPanelParams{
		Uuid: id, BrandID: c.Org.BrandID, OrgIds: c.Filter.OrgIDsArg(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return PDFFile{}, ErrNotFound
	}
	if err != nil {
		return PDFFile{}, err
	}
	if row.Status != StatusAccepted {
		return PDFFile{}, ErrPDFVINPending
	}
	if !row.ParsedAt.Valid {
		return PDFFile{}, ErrPDFNotNormalized
	}
	if p.store == nil {
		return PDFFile{}, ErrPDFUnavailable
	}
	name := pdfFilename(row.Vin.String, row.Uuid)
	if f, ok := p.open(ctx, row.PdfKey, name); ok {
		f.UUID = row.Uuid
		return f, nil
	}
	if p.queue == nil {
		if err := p.GeneratePDF(ctx, row.ID); err != nil {
			return PDFFile{}, err
		}
		key, err := p.q.GetMeasurementResultByUUID(ctx, db.GetMeasurementResultByUUIDParams{Uuid: row.Uuid, OrganizationID: row.OrganizationID})
		if err != nil {
			return PDFFile{}, err
		}
		if f, ok := p.open(ctx, key.PdfKey, name); ok {
			f.UUID = row.Uuid
			return f, nil
		}
		return PDFFile{}, ErrPDFUnavailable
	}
	task, err := queue.NewMeasurementPDFTask(row.ID)
	if err != nil {
		return PDFFile{}, err
	}
	if _, err := p.queue.Enqueue(task, queue.MeasurementPDFOpts(row.ID)...); err != nil &&
		!errors.Is(err, asynq.ErrTaskIDConflict) && !errors.Is(err, asynq.ErrDuplicateTask) {
		return PDFFile{}, err
	}
	return PDFFile{Pending: true, UUID: row.Uuid}, nil
}

func (p *PDFService) open(ctx context.Context, key pgtype.Text, name string) (PDFFile, bool) {
	if !key.Valid || strings.TrimSpace(key.String) == "" {
		return PDFFile{}, false
	}
	rc, _, err := p.store.Download(ctx, key.String)
	if err != nil {
		p.log.Warn("measurement_pdf_object_missing", "key", key.String, "error", err)
		return PDFFile{}, false
	}
	return PDFFile{Body: rc, Filename: name}, true
}

func pdfFilename(vin string, id uuid.UUID) string {
	if vin = strings.TrimSpace(vin); vin != "" {
		return "measurement-" + vin + ".pdf"
	}
	return "measurement-" + id.String()[:8] + ".pdf"
}

// GeneratePDF is the worker-docs handler of measurement:pdf. It is
// idempotent and safe under concurrent delivery: the result row is locked
// for the whole render and pdf_key is written in the same transaction, so a
// second run waits and then finds the stored PDF. A vin_pending or
// unnormalized result is skipped.
func (p *PDFService) GeneratePDF(ctx context.Context, resultID int64) error {
	if p.pdf == nil || p.store == nil || p.tx == nil {
		return ErrPDFUnavailable
	}
	tx, err := p.tx.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := p.q.WithTx(tx)
	row, err := qtx.GetMeasurementResultForPDF(ctx, resultID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if row.Status != StatusAccepted || !row.ParsedAt.Valid {
		return nil
	}
	if row.PdfKey.Valid && strings.TrimSpace(row.PdfKey.String) != "" {
		if ok, err := p.store.Exists(ctx, row.PdfKey.String); err == nil && ok {
			return nil
		}
	}
	org, err := qtx.GetOrganizationByID(ctx, row.OrganizationID)
	if err != nil {
		return err
	}
	doc, err := p.buildDocument(ctx, qtx, row, org)
	if err != nil {
		return err
	}
	data, err := p.pdf.Convert(ctx, pdfrender.Request{HTML: doc.HTML, FooterHTML: pdfrender.FooterHTML(doc.Lang, org.Name)})
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	key := storage.MeasurementPDFObjectKey(org.Uuid, row.Uuid)
	if err := p.store.Upload(ctx, storage.File{
		Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: "application/pdf",
		Filename: pdfFilename(row.Vin.String, row.Uuid),
		Metadata: map[string]string{"sha256": hex.EncodeToString(sum[:]), "measurement_uuid": row.Uuid.String()},
	}, key); err != nil {
		return err
	}
	if err := qtx.SetMeasurementResultPDFKey(ctx, db.SetMeasurementResultPDFKeyParams{
		ID: row.ID, OrganizationID: row.OrganizationID, PdfKey: pgtype.Text{String: key, Valid: true},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Document is a rendered measurement HTML document.
type Document struct {
	HTML string
	Lang string
	Vars map[string]string
}

// pdfData is everything one measurement document shows.
type pdfData struct {
	row    db.MeasurementResult
	org    db.Organization
	ctx    db.GetMeasurementPDFContextRow
	values []db.MeasurementValue
	tires  []db.MeasurementTire
	loc    i18n.Locale
	zone   *time.Location
	now    time.Time
}

func (p *PDFService) load(ctx context.Context, q *db.Queries, row db.MeasurementResult, org db.Organization) (pdfData, error) {
	d := pdfData{row: row, org: org, now: p.now()}
	var err error
	d.ctx, err = q.GetMeasurementPDFContext(ctx, db.GetMeasurementPDFContextParams{ID: row.ID, OrganizationID: row.OrganizationID})
	if err != nil {
		return pdfData{}, err
	}
	d.values, err = q.ListMeasurementValuesForPDF(ctx, db.ListMeasurementValuesForPDFParams{ResultID: row.ID, OrganizationID: row.OrganizationID})
	if err != nil {
		return pdfData{}, err
	}
	d.tires, err = q.ListMeasurementTires(ctx, db.ListMeasurementTiresParams{ResultID: row.ID, OrganizationID: row.OrganizationID})
	if err != nil {
		return pdfData{}, err
	}
	d.loc = i18n.Normalize(org.Locale)
	d.zone = time.UTC
	if z, err := time.LoadLocation(strings.TrimSpace(org.Timezone)); err == nil && org.Timezone != "" {
		d.zone = z
	}
	return d, nil
}

// buildDocument fills the active measurement template.
func (p *PDFService) buildDocument(ctx context.Context, q *db.Queries, row db.MeasurementResult, org db.Organization) (Document, error) {
	d, err := p.load(ctx, q, row, org)
	if err != nil {
		return Document{}, err
	}
	vars, err := documentVars(d)
	if err != nil {
		return Document{}, err
	}
	for k, v := range p.letterhead(ctx, q, org) {
		if strings.TrimSpace(v) != "" {
			vars[k] = v
		}
	}
	lang := docmodel.NormalizeLanguage(string(d.loc))
	if lang == "" {
		lang = docmodel.FallbackLanguage
	}
	tpl, err := measurementTemplate(ctx, q, row.BrandID, lang)
	if err != nil {
		return Document{}, err
	}
	spec, _ := docmodel.Spec(docmodel.KindMeasurement)
	body := pdfrender.Fill(withMeasurementBlocks(pdfrender.SanitizeHTML(tpl.Html)), vars, spec.RawHTMLKeys())
	title := tpl.Name
	if v := vars["vin"]; v != "" {
		title += " " + v
	}
	docLang := strings.ReplaceAll(tpl.Language, "_", "-")
	out := pdfrender.Document{
		Lang: docLang, Title: title, Body: body + measurementCSS, PrimaryColor: org.PrimaryColor, Fonts: p.fonts,
	}.HTML()
	return Document{HTML: out, Lang: docLang, Vars: vars}, nil
}

// RenderHTML builds the HTML of a measurement without converting it (the
// render test and debugging).
func (p *PDFService) RenderHTML(ctx context.Context, resultID int64) (Document, error) {
	row, err := p.q.GetMeasurementResultForPDF(ctx, resultID)
	if err != nil {
		return Document{}, err
	}
	org, err := p.q.GetOrganizationByID(ctx, row.OrganizationID)
	if err != nil {
		return Document{}, err
	}
	return p.buildDocument(ctx, p.q, row, org)
}

func (p *PDFService) letterhead(ctx context.Context, q *db.Queries, org db.Organization) map[string]string {
	settings, err := q.GetAppSettings(ctx)
	if err != nil {
		p.log.Warn("measurement_pdf_settings_failed", "error", err)
	}
	lh := ioengine.LetterheadFromOrganization(org, settings)
	if p.store != nil {
		if loaded, err := ioengine.LoadOrganizationLetterhead(ctx, p.store, org, settings); err != nil {
			p.log.Warn("measurement_pdf_logo_failed", "error", err)
		} else {
			lh = loaded
		}
	}
	out := map[string]string{
		"company_name": lh.CompanyName, "company_address": lh.Address, "company_phone": lh.Phone,
		"company_email": lh.Email, "company_website": lh.Website, "footer_text": lh.FooterText,
	}
	if logo := pdfrender.ImageTag(lh.LogoMIME, lh.LogoBytes, lh.CompanyName); logo != "" {
		out["company_logo"] = logo
	}
	return out
}

func measurementTemplate(ctx context.Context, q *db.Queries, brandID int64, lang string) (db.DocumentTemplate, error) {
	brand := pgtype.Int8{Int64: brandID, Valid: brandID > 0}
	for _, c := range []struct {
		brand pgtype.Int8
		lang  string
	}{{brand, lang}, {pgtype.Int8{}, lang}, {brand, docmodel.FallbackLanguage}, {pgtype.Int8{}, docmodel.FallbackLanguage}} {
		row, err := q.GetActiveDocumentTemplate(ctx, db.GetActiveDocumentTemplateParams{
			Kind: docmodel.KindMeasurement, Language: c.lang, BrandID: c.brand,
		})
		if err == nil {
			return row, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return db.DocumentTemplate{}, err
		}
	}
	return db.DocumentTemplate{}, fmt.Errorf("measurements: no active measurement template")
}

// withMeasurementBlocks appends the TEC-298 blocks a template does not
// place itself: the seeded template (000030) predates them, and the PDF
// must always carry the header (VIN, device, timestamps), the part map and
// the tires.
func withMeasurementBlocks(tpl string) string {
	used := map[string]bool{}
	for _, k := range msgtemplate.Placeholders(tpl) {
		used[k] = true
	}
	var b strings.Builder
	if !used["measurement_summary_html"] && !used["vin"] {
		b.WriteString("{{measurement_summary_html}}")
	}
	if !used["part_map_html"] {
		b.WriteString("{{part_map_html}}")
	}
	if !used["tires_table"] {
		b.WriteString("{{tires_table}}")
	}
	if b.Len() == 0 {
		return tpl
	}
	return tpl + `<section class="doc-measurement">` + b.String() + `</section>`
}

const measurementCSS = `<style>
.mm-map{display:flex;flex-wrap:wrap;gap:8px;margin-block:8px}
.mm-card{flex:0 0 calc(33.333% - 8px);break-inside:avoid;border:1px solid #e5e7eb;border-radius:6px;padding:4px}
.mm-card.mm-main{flex-basis:calc(50% - 8px)}
.mm-card img{width:100%;height:auto;display:block}
.mm-card-title{font-size:10px;font-weight:600;margin-block-end:2px}
.mm-legend{display:flex;flex-wrap:wrap;gap:10px;font-size:10px;margin-block:6px}
.mm-swatch{display:inline-block;width:10px;height:10px;border-radius:50%;margin-inline-end:4px;vertical-align:middle}
.mm-group{font-weight:600;margin-block:10px 4px}
.doc-measurement h2{margin-block-start:14px}
</style>`

// documentVars builds the template values of one measurement.
func documentVars(d pdfData) (map[string]string, error) {
	t := func(key string) string { return i18n.Translate(d.loc, key) }
	vin := strings.TrimSpace(d.row.Vin.String)
	if vin == "" {
		vin = strings.TrimSpace(d.ctx.VehicleVin.String)
	}
	serial := strings.TrimSpace(d.ctx.RegistryDeviceSerial.String)
	if serial == "" {
		serial = strings.TrimSpace(d.row.DeviceSerial.String)
	}
	measured := d.row.CreatedAt.Time
	if d.row.MeasuredAt.Valid {
		measured = d.row.MeasuredAt.Time
	}
	measuredAt := formatStamp(measured, d.zone, d.loc)
	generatedAt := formatStamp(d.now, d.zone, d.loc)
	vehicle := strings.TrimSpace(strings.Join([]string{d.ctx.CarBrandName.String, d.ctx.CarModelName.String}, " "))
	if d.ctx.VehicleModelYear.Valid {
		vehicle = strings.TrimSpace(vehicle + " (" + strconv.Itoa(int(d.ctx.VehicleModelYear.Int16)) + ")")
	}
	bodyType := strings.TrimSpace(d.row.BodyType.String)
	if id, ok := svg.ResolveID(bodyType); ok {
		if det, err := svg.BodyTypeDetail(id); err == nil {
			bodyType = det.Name
		}
	}
	customer := ""
	if d.ctx.CustomerStatus.String == "anonymized" {
		customer = t("customers.anonymized_name")
	} else {
		customer = strings.TrimSpace(d.ctx.CustomerName.String + " " + d.ctx.CustomerSurname.String)
	}
	plate := strings.TrimSpace(d.ctx.VehiclePlate.String)

	summary := [][]string{{t("measurements.pdf.vin"), vin}, {t("measurements.pdf.plate"), plate}}
	if vehicle != "" {
		summary = append(summary, []string{t("measurements.pdf.vehicle"), vehicle})
	}
	summary = append(summary,
		[]string{t("measurements.pdf.body_type"), bodyType},
		[]string{t("measurements.pdf.device_serial"), serial},
		[]string{t("measurements.pdf.measured_at"), measuredAt},
	)
	if no := strings.TrimSpace(d.ctx.ServiceNo.String); no != "" {
		summary = append(summary, []string{t("measurements.pdf.service"), no})
	}
	summary = append(summary, []string{t("measurements.pdf.generated_at"), generatedAt})

	partMap, err := partMapHTML(d)
	if err != nil {
		return nil, err
	}
	vars := map[string]string{
		"document_number":          d.row.Uuid.String()[:8],
		"document_date":            formatStamp(d.now, d.zone, d.loc),
		"measured_at":              measuredAt,
		"generated_at":             generatedAt,
		"vin":                      vin,
		"plate":                    plate,
		"vehicle":                  vehicle,
		"device_serial":            serial,
		"body_type":                bodyType,
		"customer_name":            customer,
		"technician_name":          strings.TrimSpace(d.ctx.UploaderName.String + " " + d.ctx.UploaderSurname.String),
		"measurement_summary_html": summaryTable(summary),
		"measurements_table":       readingsHTML(d),
		"part_map_html":            partMap,
		"tires_table":              tiresHTML(d),
	}
	return vars, nil
}

// formatStamp prints a timestamp in the organization timezone with the
// zone name (dd.mm.yyyy hh:mm; yyyy-mm-dd for en and zh-CN).
func formatStamp(ts time.Time, zone *time.Location, loc i18n.Locale) string {
	ts = ts.In(zone)
	layout := "02.01.2006 15:04"
	if loc == i18n.LocaleEN || loc == i18n.LocaleZhCN {
		layout = "2006-01-02 15:04"
	}
	return ts.Format(layout) + " (" + zone.String() + ")"
}

func summaryTable(rows [][]string) string {
	var b strings.Builder
	b.WriteString(`<table class="doc-meta mm-summary"><tbody>`)
	for _, r := range rows {
		b.WriteString(`<tr><th>`)
		b.WriteString(html.EscapeString(r[0]))
		b.WriteString(`</th><td>`)
		b.WriteString(html.EscapeString(r[1]))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}

func placeOrder(place string) int {
	for i, p := range svg.Places {
		if p == place {
			return i
		}
	}
	return len(svg.Places)
}

type partRow struct {
	part      string
	substrate string
	values    map[int]float64
	all       []float64
}

// readingsHTML is the legacy measurement-place-groups table: one group per
// place (outside first, then inside), one row per part with substrate,
// lowest, highest, average and the value of each position.
func readingsHTML(d pdfData) string {
	t := func(key string) string { return i18n.Translate(d.loc, key) }
	if len(d.values) == 0 {
		return `<p class="mm-empty">` + html.EscapeString(t("measurements.pdf.no_readings")) + `</p>`
	}
	type groupKey struct {
		inside bool
		place  string
	}
	groups := map[groupKey][]*partRow{}
	index := map[groupKey]map[string]*partRow{}
	var keys []groupKey
	maxPos := 5
	for _, v := range d.values {
		k := groupKey{inside: v.IsInside, place: v.PlaceID}
		if index[k] == nil {
			index[k] = map[string]*partRow{}
			keys = append(keys, k)
		}
		r := index[k][v.PartType]
		if r == nil {
			r = &partRow{part: v.PartType, values: map[int]float64{}}
			index[k][v.PartType] = r
			groups[k] = append(groups[k], r)
		}
		if r.substrate == "" && v.SubstrateType.Valid {
			r.substrate = v.SubstrateType.String
		}
		f, ok := numericFloat(v.ValueUm)
		if !ok {
			continue
		}
		r.all = append(r.all, f)
		if v.Position.Valid {
			pos := int(v.Position.Int32)
			r.values[pos] = f
			maxPos = max(maxPos, pos)
		}
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i].inside != keys[j].inside {
			return !keys[i].inside
		}
		return placeOrder(keys[i].place) < placeOrder(keys[j].place)
	})
	cols := []pdfrender.Column{
		{Label: t("measurements.pdf.part")}, {Label: t("measurements.pdf.substrate")},
		{Label: t("measurements.pdf.lowest"), Numeric: true}, {Label: t("measurements.pdf.highest"), Numeric: true},
		{Label: t("measurements.pdf.average"), Numeric: true},
	}
	for i := 1; i <= maxPos; i++ {
		cols = append(cols, pdfrender.Column{Label: strconv.Itoa(i) + ".", Numeric: true})
	}
	var b strings.Builder
	for _, k := range keys {
		title := placeLabel(d.loc, k.place)
		if k.inside {
			title += " (" + t("measurements.pdf.inside") + ")"
		}
		b.WriteString(`<p class="mm-group">` + html.EscapeString(title) + `</p>`)
		var rows [][]string
		for _, r := range groups[k] {
			row := []string{partLabel(d.loc, r.part), r.substrate, "-", "-", "-"}
			if len(r.all) > 0 {
				lo, hi, sum := r.all[0], r.all[0], 0.0
				for _, f := range r.all {
					lo, hi, sum = min(lo, f), max(hi, f), sum+f
				}
				row[2] = strconv.FormatFloat(lo, 'f', 0, 64)
				row[3] = strconv.FormatFloat(hi, 'f', 0, 64)
				row[4] = strconv.FormatFloat(sum/float64(len(r.all)), 'f', 1, 64)
			}
			for i := 1; i <= maxPos; i++ {
				if f, ok := r.values[i]; ok {
					row = append(row, strconv.FormatFloat(f, 'f', 0, 64))
				} else {
					row = append(row, "-")
				}
			}
			rows = append(rows, row)
		}
		b.WriteString(pdfrender.Table(cols, rows))
	}
	return b.String()
}

func placeLabel(loc i18n.Locale, place string) string {
	key := "measurements.places." + place
	if v := i18n.Translate(loc, key); v != key {
		return v
	}
	return place
}

func partLabel(loc i18n.Locale, part string) string {
	key := "measurements.parts." + part
	if v := i18n.Translate(loc, key); v != key {
		return v
	}
	return part
}

var interpretationKeys = []struct {
	level int
	key   string
}{
	{svg.TooThin, "too_thin"}, {svg.Original, "original"}, {svg.SecondLayer, "second_layer"},
	{svg.ThinPutty, "thin_putty"}, {svg.ThickPutty, "thick_putty"}, {svg.Unknown, "unknown"},
}

// partMapHTML is the legacy visualization section: the composite views and
// the element parts as <img> data URIs (the document CSP allows data:
// images only), then the color legend.
func partMapHTML(d pdfData) (string, error) {
	t := func(key string) string { return i18n.Translate(d.loc, key) }
	readings := make([]svg.Reading, 0, len(d.values))
	for _, v := range d.values {
		r := svg.Reading{PartType: v.PartType}
		if v.Position.Valid {
			pos := int(v.Position.Int32)
			r.Position = &pos
		}
		if v.Interpretation.Valid {
			level := int(v.Interpretation.Int16)
			r.Interpretation = &level
		}
		readings = append(readings, r)
	}
	cards, err := svg.Cards(svg.Report{BodyType: d.row.BodyType.String, Readings: readings})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(`<h2>` + html.EscapeString(t("measurements.pdf.part_map")) + `</h2>`)
	if len(cards) == 0 {
		b.WriteString(`<p class="mm-empty">` + html.EscapeString(t("measurements.pdf.no_part_map")) + `</p>`)
		return b.String(), nil
	}
	b.WriteString(`<div class="mm-map">`)
	for _, c := range cards {
		label := partLabel(d.loc, c.Part)
		class := "mm-card"
		if c.Type == "main" {
			label = placeLabel(d.loc, c.Place)
			class += " mm-main"
		}
		b.WriteString(`<div class="` + class + `" data-part="` + html.EscapeString(c.Part) + `">`)
		b.WriteString(`<div class="mm-card-title">` + html.EscapeString(label) + `</div>`)
		b.WriteString(`<img src="data:image/svg+xml;base64,` + base64.StdEncoding.EncodeToString([]byte(c.SVG)) + `" alt="` + html.EscapeString(label) + `">`)
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`<div class="mm-legend">`)
	for _, it := range interpretationKeys {
		b.WriteString(`<span><span class="mm-swatch" style="background:` + svg.FillColor(it.level) + `"></span>`)
		b.WriteString(html.EscapeString(t("measurements.interpretations."+it.key)) + `</span>`)
	}
	b.WriteString(`</div>`)
	return b.String(), nil
}

func tiresHTML(d pdfData) string {
	if len(d.tires) == 0 {
		return ""
	}
	t := func(key string) string { return i18n.Translate(d.loc, key) }
	cols := []pdfrender.Column{
		{Label: t("measurements.pdf.tire_position")}, {Label: t("measurements.pdf.tire_size")},
		{Label: t("measurements.pdf.tire_maker")}, {Label: t("measurements.pdf.tire_season")},
		{Label: t("measurements.pdf.tread_depth"), Numeric: true},
	}
	var rows [][]string
	for _, tr := range d.tires {
		size := strings.TrimSpace(tr.Width.String)
		if tr.Profile.Valid && tr.Profile.String != "" {
			size += "/" + tr.Profile.String
		}
		if tr.Diameter.Valid && tr.Diameter.String != "" {
			size += " R" + tr.Diameter.String
		}
		var depths []string
		for _, n := range []pgtype.Numeric{tr.TreadDepth1Mm, tr.TreadDepth2Mm} {
			if s := numericOut(n); s != nil {
				depths = append(depths, *s)
			}
		}
		rows = append(rows, []string{tr.Section.String, strings.TrimSpace(size), tr.Maker.String, tr.Season.String, strings.Join(depths, " / ")})
	}
	return `<h2>` + html.EscapeString(t("measurements.pdf.tires")) + `</h2>` + pdfrender.Table(cols, rows)
}

// DocumentLoader is the documents-module SourceLoader of the measurement
// kind: the same values as the measurement PDF for the generic
// /v1/tenant/documents/render flow (template preview with real data).
func (p *PDFService) DocumentLoader() docmodel.SourceLoader { return measurementDocumentLoader{p: p} }

type measurementDocumentLoader struct{ p *PDFService }

func (l measurementDocumentLoader) SourceType() string { return MeasurementDocumentSourceType }

func (l measurementDocumentLoader) Load(ctx context.Context, viewer docmodel.Viewer, sourceID, _ string) (docmodel.Source, error) {
	id, err := uuid.Parse(strings.TrimSpace(sourceID))
	if err != nil {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	q := l.p.q
	row, err := q.GetMeasurementResultByUUID(ctx, db.GetMeasurementResultByUUIDParams{Uuid: id, OrganizationID: viewer.OrganizationID})
	if err != nil || (viewer.BrandID != 0 && row.BrandID != viewer.BrandID) {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	if row.Status != StatusAccepted || !row.ParsedAt.Valid {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	org, err := q.GetOrganizationByID(ctx, row.OrganizationID)
	if err != nil {
		return docmodel.Source{}, err
	}
	d, err := l.p.load(ctx, q, row, org)
	if err != nil {
		return docmodel.Source{}, err
	}
	vars, err := documentVars(d)
	if err != nil {
		return docmodel.Source{}, err
	}
	version := strconv.FormatInt(row.ParsedAt.Time.UnixNano(), 10) + ":" + row.Vin.String
	return docmodel.Source{
		OrganizationID: row.OrganizationID, BrandID: row.BrandID, Version: version, Vars: vars,
		Title: strings.TrimSuffix(pdfFilename(row.Vin.String, row.Uuid), ".pdf"),
	}, nil
}
