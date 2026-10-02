package usecase

// TEC-161: personal data export (K19). The export runs as an I/O engine
// export job (queue "exports", worker-docs) in two formats: JSON (one
// structured document: profile, vehicles, services, warranties) and PDF (a
// styled HTML document rendered by Gotenberg on the letterhead, lang/dir in
// the job locale). Identity numbers are exported masked (last four
// characters, TEC-100 decision 3); they are never decrypted into a file.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Export resources (export_jobs.resource).
const (
	// ResourceDataExport: requested by the center for a customer of its
	// brand (job organization = the center).
	ResourceDataExport = "customers.data_export"
	// ResourcePortalDataExport: requested by the customer from the portal
	// (job without organization, owned by the customer).
	ResourcePortalDataExport = "portal.customer_data_export"
)

// Export query keys, written by the HTTP handler after authorization.
const (
	QueryCustomerUUID = "customer_uuid"
	QueryBrandID      = "brand_id"
)

// DataExportVersion is the schema version of the JSON document.
const DataExportVersion = 1

// errExportScope: the customer is not linked to the job organization's brand.
var errExportScope = errors.New("customers export: customer is outside the organization brand")

// DataExport is the personal data document of one customer.
type DataExport struct {
	Version    int              `json:"version"`
	ExportedAt time.Time        `json:"exported_at"`
	Profile    ExportProfile    `json:"profile"`
	Vehicles   []ExportVehicle  `json:"vehicles"`
	Services   []ExportService  `json:"services"`
	Warranties []ExportWarranty `json:"warranties"`
}

// ExportProfile is the identity part. For an anonymized customer the
// personal fields are masked (null) like in the API.
type ExportProfile struct {
	UUID              uuid.UUID       `json:"uuid"`
	Name              string          `json:"name"`
	Surname           string          `json:"surname"`
	Email             *string         `json:"email"`
	Phone             *string         `json:"phone"`
	Locale            *string         `json:"locale"`
	Timezone          *string         `json:"timezone"`
	Status            string          `json:"status"`
	Anonymized        bool            `json:"anonymized"`
	AnonymizedAt      *time.Time      `json:"anonymized_at"`
	Type              string          `json:"type"`
	CompanyName       *string         `json:"company_name"`
	TaxOffice         *string         `json:"tax_office"`
	NationalIDLast4   *string         `json:"national_id_last4"`
	TaxNoLast4        *string         `json:"tax_no_last4"`
	Address           json.RawMessage `json:"address"`
	NotificationPrefs json.RawMessage `json:"notification_prefs"`
	CreatedAt         time.Time       `json:"created_at"`
}

// ExportVehicle is one vehicle of the customer.
type ExportVehicle struct {
	UUID         uuid.UUID `json:"uuid"`
	Plate        *string   `json:"plate"`
	PlateCountry *string   `json:"plate_country"`
	VIN          *string   `json:"vin"`
	CarBrand     *string   `json:"car_brand"`
	CarModel     *string   `json:"car_model"`
	ModelYear    *int16    `json:"model_year"`
	CreatedAt    time.Time `json:"created_at"`
}

// ExportService is one service received by the customer.
type ExportService struct {
	UUID         uuid.UUID  `json:"uuid"`
	ServiceNo    string     `json:"service_no"`
	Status       string     `json:"status"`
	Organization string     `json:"organization"`
	Plate        *string    `json:"plate"`
	PlateCountry *string    `json:"plate_country"`
	VIN          *string    `json:"vin"`
	CarBrand     string     `json:"car_brand"`
	CarModel     string     `json:"car_model"`
	ModelYear    *int16     `json:"model_year"`
	Km           *int32     `json:"km"`
	Package      *string    `json:"package"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	CancelledAt  *time.Time `json:"cancelled_at"`
}

// ExportWarranty is one warranty held by the customer.
type ExportWarranty struct {
	UUID         uuid.UUID  `json:"uuid"`
	PublicCode   string     `json:"public_code"`
	Kind         string     `json:"kind"`
	Status       string     `json:"status"`
	Product      string     `json:"product"`
	ProductSKU   string     `json:"product_sku"`
	ServiceNo    string     `json:"service_no"`
	Organization string     `json:"organization"`
	Plate        *string    `json:"plate"`
	PlateCountry *string    `json:"plate_country"`
	VIN          *string    `json:"vin"`
	StartAt      time.Time  `json:"start_at"`
	EndAt        time.Time  `json:"end_at"`
	VoidedAt     *time.Time `json:"voided_at"`
}

// BuildDataExport collects the customer's data; brandID 0 = every brand.
func (s *Service) BuildDataExport(ctx context.Context, user db.User, brandID int64, loc i18n.Locale) (DataExport, error) {
	brand := pgtype.Int8{Int64: brandID, Valid: brandID != 0}
	out := DataExport{
		Version: DataExportVersion, ExportedAt: time.Now().UTC(),
		Profile: ExportProfile{
			UUID: user.Uuid, Name: user.Name, Surname: user.Surname, Email: strOrNil(user.Email),
			Phone: strOrNil(user.PhoneE164), Locale: strOrNil(user.Locale), Timezone: strOrNil(user.Timezone),
			Status: user.Status, Type: TypeIndividual, CreatedAt: user.CreatedAt.Time,
			Address: json.RawMessage("{}"), NotificationPrefs: json.RawMessage("{}"),
		},
		Vehicles: []ExportVehicle{}, Services: []ExportService{}, Warranties: []ExportWarranty{},
	}
	prof, err := s.q.GetCustomerProfile(ctx, user.ID)
	switch {
	case err == nil:
		p := &out.Profile
		p.Type = prof.Type
		p.CompanyName, p.TaxOffice = strOrNil(prof.CompanyName), strOrNil(prof.TaxOffice)
		p.NationalIDLast4, p.TaxNoLast4 = strOrNil(prof.NationalIDLast4), strOrNil(prof.TaxNoLast4)
		p.AnonymizedAt = timePtr(prof.AnonymizedAt)
		if len(prof.Address) > 0 {
			p.Address = json.RawMessage(prof.Address)
		}
		if len(prof.NotificationPrefs) > 0 {
			p.NotificationPrefs = json.RawMessage(prof.NotificationPrefs)
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return DataExport{}, fmt.Errorf("customers export: profile: %w", err)
	}
	if user.Status == StatusAnonymized {
		p := &out.Profile
		p.Anonymized = true
		p.Name, p.Surname = i18n.Translate(loc, AnonymizedNameKey), ""
		p.Email, p.Phone, p.Timezone, p.CompanyName, p.TaxOffice = nil, nil, nil, nil, nil
		p.NationalIDLast4, p.TaxNoLast4 = nil, nil
		p.Address = json.RawMessage("{}")
	}

	vehicles, err := s.q.ListVehiclesByUser(ctx, db.ListVehiclesByUserParams{UserID: user.ID, BrandID: brand})
	if err != nil {
		return DataExport{}, fmt.Errorf("customers export: vehicles: %w", err)
	}
	for _, v := range vehicles {
		out.Vehicles = append(out.Vehicles, ExportVehicle{
			UUID: v.Uuid, Plate: strOrNil(v.Plate), PlateCountry: strOrNil(v.PlateCountry), VIN: strOrNil(v.Vin),
			CarBrand: strOrNil(v.CarBrandName), CarModel: strOrNil(v.CarModelName), ModelYear: int2Ptr(v.ModelYear),
			CreatedAt: v.CreatedAt.Time,
		})
	}
	services, err := s.q.ListCustomerExportServices(ctx, db.ListCustomerExportServicesParams{CustomerUserID: user.ID, BrandID: brand})
	if err != nil {
		return DataExport{}, fmt.Errorf("customers export: services: %w", err)
	}
	for _, sv := range services {
		var km *int32
		if sv.Km.Valid {
			v := sv.Km.Int32
			km = &v
		}
		out.Services = append(out.Services, ExportService{
			UUID: sv.Uuid, ServiceNo: sv.ServiceNo, Status: sv.Status, Organization: sv.OrganizationName,
			Plate: strOrNil(sv.Plate), PlateCountry: strOrNil(sv.PlateCountry), VIN: strOrNil(sv.Vin),
			CarBrand: sv.CarBrandName, CarModel: sv.CarModelName, ModelYear: int2Ptr(sv.ModelYear), Km: km,
			Package: strOrNil(sv.Package), CreatedAt: sv.CreatedAt.Time,
			CompletedAt: timePtr(sv.CompletedAt), CancelledAt: timePtr(sv.CancelledAt),
		})
	}
	warranties, err := s.q.ListCustomerExportWarranties(ctx, db.ListCustomerExportWarrantiesParams{HolderUserID: user.ID, BrandID: brand})
	if err != nil {
		return DataExport{}, fmt.Errorf("customers export: warranties: %w", err)
	}
	for _, w := range warranties {
		out.Warranties = append(out.Warranties, ExportWarranty{
			UUID: w.Uuid, PublicCode: w.PublicCode, Kind: w.ItemKind, Status: w.Status,
			Product: w.ProductName, ProductSKU: w.ProductSku, ServiceNo: w.ServiceNo, Organization: w.OrganizationName,
			Plate: strOrNil(w.Plate), PlateCountry: strOrNil(w.PlateCountry), VIN: strOrNil(w.Vin),
			StartAt: w.StartAt.Time, EndAt: w.EndAt.Time, VoidedAt: timePtr(w.VoidedAt),
		})
	}
	return out, nil
}

func int2Ptr(v pgtype.Int2) *int16 {
	if !v.Valid {
		return nil
	}
	x := v.Int16
	return &x
}

// --- Export adapter ----------------------------------------------------------

// DataExportAdapter is the ioengine adapter of both export resources.
type DataExportAdapter struct {
	resource string
	svc      *Service
}

// NewDataExportAdapter: center export of a customer (ResourceDataExport).
func NewDataExportAdapter(svc *Service) *DataExportAdapter {
	return &DataExportAdapter{resource: ResourceDataExport, svc: svc}
}

// NewPortalDataExportAdapter: the customer's own export from the portal.
func NewPortalDataExportAdapter(svc *Service) *DataExportAdapter {
	return &DataExportAdapter{resource: ResourcePortalDataExport, svc: svc}
}

// Resource implements ioengine.ResourceAdapter.
func (a *DataExportAdapter) Resource() string { return a.resource }

// ExportColumns implements ioengine.ResourceAdapter (flat form used only by
// the generic encoders; the handler offers JSON and PDF).
func (a *DataExportAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "section", LabelKey: "customers.export.section", Type: ioengine.ColumnTypeString},
		{Key: "item", LabelKey: "customers.export.item", Type: ioengine.ColumnTypeString},
		{Key: "field", LabelKey: "customers.export.field", Type: ioengine.ColumnTypeString},
		{Key: "value", LabelKey: "customers.export.value", Type: ioengine.ColumnTypeString, Weight: 2},
	}
}

// ImportSchema implements ioengine.ResourceAdapter (export only).
func (a *DataExportAdapter) ImportSchema() []ioengine.ImportField { return nil }

// ApplyRow implements ioengine.ResourceAdapter (export only).
func (a *DataExportAdapter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "export only"}, nil
}

// RevertRow implements ioengine.ResourceAdapter (export only).
func (a *DataExportAdapter) RevertRow(context.Context, string, string, map[string]any) error {
	return nil
}

// Export implements ioengine.ResourceAdapter. The center resource re-checks
// that the customer is linked to the job organization's brand.
func (a *DataExportAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	id, err := uuid.Parse(strings.TrimSpace(q[QueryCustomerUUID]))
	if err != nil {
		return ioengine.Dataset{}, ErrCustomerNotFound
	}
	user, err := a.svc.q.GetUserByUUID(ctx, id)
	if err != nil {
		return ioengine.Dataset{}, ErrCustomerNotFound
	}
	var brandID int64
	if a.resource == ResourceDataExport {
		orgID, err := strconv.ParseInt(q[ioengine.QueryOrganizationID], 10, 64)
		if err != nil || orgID <= 0 {
			return ioengine.Dataset{}, errors.New("customers export: organization is required")
		}
		org, err := a.svc.q.GetOrganizationByID(ctx, orgID)
		if err != nil {
			return ioengine.Dataset{}, fmt.Errorf("customers export: organization: %w", err)
		}
		linked, err := a.svc.q.CustomerLinkedToBrand(ctx, db.CustomerLinkedToBrandParams{UserID: user.ID, BrandID: org.BrandID})
		if err != nil {
			return ioengine.Dataset{}, fmt.Errorf("customers export: scope: %w", err)
		}
		if !linked {
			return ioengine.Dataset{}, errExportScope
		}
		brandID = org.BrandID
	} else if raw := strings.TrimSpace(q[QueryBrandID]); raw != "" {
		if brandID, err = strconv.ParseInt(raw, 10, 64); err != nil {
			return ioengine.Dataset{}, errors.New("customers export: invalid brand")
		}
	}
	doc, err := a.svc.BuildDataExport(ctx, user, brandID, loc)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	return DataExportDataset(a.resource, doc, loc), nil
}

// section is one titled block of the document: records of label/value pairs.
type section struct {
	key     string
	records [][][2]string
}

func dataSections(doc DataExport, loc i18n.Locale) []section {
	t := func(key string) string { return i18n.Translate(loc, key) }
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	day := func(tm time.Time) string { return tm.UTC().Format(time.DateOnly) }
	plate := func(p, c *string) string {
		if p == nil {
			return ""
		}
		if c != nil {
			return *p + " (" + *c + ")"
		}
		return *p
	}
	year := func(y *int16) string {
		if y == nil {
			return ""
		}
		return strconv.Itoa(int(*y))
	}
	p := doc.Profile
	profile := [][2]string{
		{t("customers.export.name"), strings.TrimSpace(p.Name + " " + p.Surname)},
		{t("customers.export.email"), str(p.Email)},
		{t("customers.export.phone"), str(p.Phone)},
		{t("customers.export.locale"), str(p.Locale)},
		{t("customers.export.status"), t("users.status." + p.Status)},
		{t("customers.export.type"), t("customers.export.type." + p.Type)},
		{t("customers.export.company_name"), str(p.CompanyName)},
		{t("customers.export.tax_office"), str(p.TaxOffice)},
		{t("customers.export.national_id"), maskLast4(p.NationalIDLast4)},
		{t("customers.export.tax_no"), maskLast4(p.TaxNoLast4)},
		{t("customers.export.address"), addressLine(p.Address)},
		{t("customers.export.created_at"), day(p.CreatedAt)},
	}
	out := []section{{key: "customers.export.profile", records: [][][2]string{profile}}}

	vs := section{key: "customers.export.vehicles"}
	for _, v := range doc.Vehicles {
		vs.records = append(vs.records, [][2]string{
			{t("customers.export.plate"), plate(v.Plate, v.PlateCountry)},
			{t("customers.export.vin"), str(v.VIN)},
			{t("customers.export.vehicle"), strings.TrimSpace(str(v.CarBrand) + " " + str(v.CarModel))},
			{t("customers.export.model_year"), year(v.ModelYear)},
			{t("customers.export.created_at"), day(v.CreatedAt)},
		})
	}
	ss := section{key: "customers.export.services"}
	for _, sv := range doc.Services {
		ss.records = append(ss.records, [][2]string{
			{t("customers.export.service_no"), sv.ServiceNo},
			{t("customers.export.date"), day(sv.CreatedAt)},
			{t("customers.export.status"), t("services.status." + sv.Status)},
			{t("customers.export.organization"), sv.Organization},
			{t("customers.export.plate"), plate(sv.Plate, sv.PlateCountry)},
			{t("customers.export.vehicle"), strings.TrimSpace(sv.CarBrand + " " + sv.CarModel)},
			{t("customers.export.product"), str(sv.Package)},
		})
	}
	ws := section{key: "customers.export.warranties"}
	for _, w := range doc.Warranties {
		ws.records = append(ws.records, [][2]string{
			{t("customers.export.code"), w.PublicCode},
			{t("customers.export.product"), w.Product},
			{t("customers.export.status"), t("customers.export.warranty_status." + w.Status)},
			{t("customers.export.period"), day(w.StartAt) + " – " + day(w.EndAt)},
			{t("customers.export.service_no"), w.ServiceNo},
			{t("customers.export.organization"), w.Organization},
			{t("customers.export.plate"), plate(w.Plate, w.PlateCountry)},
		})
	}
	return append(out, vs, ss, ws)
}

func maskLast4(p *string) string {
	if p == nil || *p == "" {
		return ""
	}
	return "•••• " + *p
}

// addressLine flattens the address object into one line (sorted keys).
func addressLine(raw json.RawMessage) string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil || len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if s := strings.TrimSpace(fmt.Sprint(m[k])); s != "" && m[k] != nil {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

// DataExportDataset maps the document to a flat dataset (section, item,
// field, value); Doc carries the document for the JSON and PDF renderers.
func DataExportDataset(resource string, doc DataExport, loc i18n.Locale) ioengine.Dataset {
	rows := []map[string]any{}
	for _, sec := range dataSections(doc, loc) {
		title := i18n.Translate(loc, sec.key)
		for i, rec := range sec.records {
			for _, kv := range rec {
				rows = append(rows, map[string]any{"section": title, "item": strconv.Itoa(i + 1), "field": kv[0], "value": kv[1]})
			}
		}
	}
	name := strings.TrimSpace(doc.Profile.Name + " " + doc.Profile.Surname)
	return ioengine.Dataset{
		Resource: resource,
		Columns:  (&DataExportAdapter{}).ExportColumns(),
		Rows:     rows,
		Info:     []ioengine.InfoLine{{LabelKey: "customers.export.subject", Value: name}},
		Doc:      doc,
	}
}

// DocumentJSON implements ioengine.JSONDocumentRenderer: the structured
// document instead of the generic column/row JSON.
func (a *DataExportAdapter) DocumentJSON(ds ioengine.Dataset, _ string) ([]byte, error) {
	doc, ok := ds.Doc.(DataExport)
	if !ok {
		return nil, errors.New("customers export: dataset carries no document")
	}
	return json.MarshalIndent(doc, "", "  ")
}

// DocumentHTML implements ioengine.DocumentRenderer: one table per record
// group under section titles, on the letterhead, in the job locale.
func (a *DataExportAdapter) DocumentHTML(ds ioengine.Dataset, locale string, lh *ioengine.Letterhead, title string) (string, error) {
	doc, ok := ds.Doc.(DataExport)
	if !ok {
		return "", errors.New("customers export: dataset carries no document")
	}
	loc := i18n.Normalize(locale)
	t := func(key string) string { return i18n.Translate(loc, key) }
	esc := html.EscapeString
	var b strings.Builder
	color := ""
	if lh != nil {
		color = lh.PrimaryColor
		b.WriteString(`<header class="doc-header"><div class="doc-logo">`)
		b.WriteString(pdfrender.ImageTag(lh.LogoMIME, lh.LogoBytes, lh.CompanyName))
		b.WriteString(`</div><div class="doc-company"><strong>`)
		b.WriteString(esc(lh.CompanyName))
		b.WriteString(`</strong></div></header>`)
	}
	b.WriteString(`<h1 class="doc-title">` + esc(title) + `</h1><table class="doc-meta">`)
	for _, line := range ds.Info {
		b.WriteString(`<tr><th>` + esc(t(line.LabelKey)) + `</th><td>` + esc(line.Value) + `</td></tr>`)
	}
	b.WriteString(`<tr><th>` + esc(t("export.generated_at")) + `</th><td>` +
		esc(doc.ExportedAt.UTC().Format("2006-01-02 15:04 UTC")) + `</td></tr></table>`)
	for _, sec := range dataSections(doc, loc) {
		b.WriteString(`<h2>` + esc(t(sec.key)) + `</h2>`)
		if len(sec.records) == 0 {
			b.WriteString(`<p class="muted">` + esc(t("customers.export.empty")) + `</p>`)
			continue
		}
		cols := make([]pdfrender.Column, 0, len(sec.records[0]))
		for _, kv := range sec.records[0] {
			cols = append(cols, pdfrender.Column{Label: kv[0]})
		}
		rows := make([][]string, 0, len(sec.records))
		for _, rec := range sec.records {
			row := make([]string, 0, len(rec))
			for _, kv := range rec {
				row = append(row, kv[1])
			}
			rows = append(rows, row)
		}
		if len(sec.records) == 1 && sec.key == "customers.export.profile" {
			b.WriteString(`<table class="doc-meta">`)
			for _, kv := range sec.records[0] {
				b.WriteString(`<tr><th>` + esc(kv[0]) + `</th><td>` + esc(kv[1]) + `</td></tr>`)
			}
			b.WriteString(`</table>`)
			continue
		}
		b.WriteString(pdfrender.Table(cols, rows))
	}
	b.WriteString(`<p class="muted">` + esc(t("customers.export.kvkk_note")) + `</p>`)
	if lh != nil && strings.TrimSpace(lh.FooterText) != "" {
		b.WriteString(`<p class="doc-footer">` + esc(lh.FooterText) + `</p>`)
	}
	return pdfrender.Document{Lang: string(loc), Title: title, Body: b.String(), PrimaryColor: color}.HTML(), nil
}

var (
	_ ioengine.ResourceAdapter      = (*DataExportAdapter)(nil)
	_ ioengine.DocumentRenderer     = (*DataExportAdapter)(nil)
	_ ioengine.JSONDocumentRenderer = (*DataExportAdapter)(nil)
)
