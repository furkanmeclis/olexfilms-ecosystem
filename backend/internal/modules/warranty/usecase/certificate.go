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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-188 (F1-06d): the warranty certificate. One PDF per service
// (TEC-98 decision 2) lists every active warranty of the service with its
// product, unit / barcode, period and its own QR code pointing at the
// public page {PUBLIC_FRONTEND_URL}/garanti/{public_code} (decision 1). It
// runs as an I/O engine export job (queue "exports", worker-docs) and is
// rendered by Gotenberg in the job locale (pdfrender skeleton: lang/dir,
// RTL for ar, embedded Noto fonts). The certificate is handed to the
// vehicle owner, so the plate and VIN are printed in full; the public page
// stays masked.

// Export resources (export_jobs.resource).
const (
	// ResourceCertificate: requested from the panel (warranties.read in
	// the active organization); downloaded through /v1/warranty-certificates.
	ResourceCertificate = "tenant.warranty.certificate"
	// ResourcePortalCertificate: requested by the customer from the portal;
	// downloaded through /v1/portal/exports (exports.PortalResourcePrefix).
	ResourcePortalCertificate = "portal.warranty_certificate"
)

// Export query keys written by the HTTP handler after it authorized the
// request. The worker re-checks the scope.
const (
	QueryServiceUUID  = "service_uuid"
	QueryBrandID      = "brand_id"
	QueryHolderUserID = "holder_user_id"
)

var (
	// ErrCertificateNotFound: the service does not exist, is outside the
	// caller's scope or (portal) holds no warranty of the caller.
	ErrCertificateNotFound = errors.New("warranty: service not found")
	// ErrNoActiveWarranty: the service has no active warranty to print.
	ErrNoActiveWarranty = errors.New("warranty: service has no active warranty")
	// errCertificateScope: the job's service is outside the job scope.
	errCertificateScope = errors.New("warranty certificate: service is outside the job scope")
)

// Certificate is the data of one warranty certificate.
type Certificate struct {
	ServiceUUID uuid.UUID
	ServiceNo   string
	ServiceDate string
	Dealer      CertificateDealer
	Vehicle     CertificateVehicle
	HolderName  string
	Items       []CertificateItem
	GeneratedAt time.Time
	// Letterhead is the letterhead of the organization that performed the
	// service (the dealer), not of the requester.
	Letterhead ioengine.Letterhead
}

// CertificateDealer is the organization that performed the service.
type CertificateDealer struct {
	Name    string
	City    string
	Address string
	Phone   string
	Email   string
}

// CertificateVehicle is the covered vehicle (service snapshot first).
type CertificateVehicle struct {
	Brand string
	Model string
	Year  string
	Plate string
	VIN   string
}

// CertificateItem is one active warranty of the service.
type CertificateItem struct {
	PublicCode string
	Product    string
	SKU        string
	Barcode    string
	Kind       string
	Meters     string
	StartDate  string
	EndDate    string
	VerifyURL  string
}

// CertificateService builds warranty certificates.
type CertificateService struct {
	q             *db.Queries
	store         storage.Driver
	verifyBaseURL string
	log           *slog.Logger
	now           func() time.Time
}

// NewCertificate builds the certificate service. verifyBaseURL is the
// public frontend origin (PUBLIC_FRONTEND_URL); store loads the dealer logo
// (nil prints the letterhead without logo).
func NewCertificate(q *db.Queries, store storage.Driver, verifyBaseURL string, log *slog.Logger) *CertificateService {
	if log == nil {
		log = slog.Default()
	}
	return &CertificateService{
		q: q, store: store, verifyBaseURL: strings.TrimRight(verifyBaseURL, "/"), log: log,
		now: func() time.Time { return time.Now().UTC() },
	}
}

// VerifyURL is the public warranty page of a public code.
func (s *CertificateService) VerifyURL(publicCode string) string {
	return verifyURL(s.verifyBaseURL, publicCode)
}

func (s *CertificateService) service(ctx context.Context, id uuid.UUID, brandID int64) (db.Service, error) {
	svc, err := s.q.GetServiceByUUID(ctx, db.GetServiceByUUIDParams{Uuid: id, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Service{}, ErrCertificateNotFound
	}
	if err != nil {
		return db.Service{}, fmt.Errorf("warranty certificate: service: %w", err)
	}
	return svc, nil
}

func (s *CertificateService) items(ctx context.Context, svc db.Service, holderUserID int64) ([]db.ListWarrantyCertificateItemsRow, error) {
	p := db.ListWarrantyCertificateItemsParams{ServiceID: svc.ID, BrandID: svc.BrandID}
	if holderUserID > 0 {
		p.HolderUserID = pgtype.Int8{Int64: holderUserID, Valid: true}
	}
	rows, err := s.q.ListWarrantyCertificateItems(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("warranty certificate: items: %w", err)
	}
	return rows, nil
}

// AuthorizeTenant checks a panel request: the service of the domain brand
// must be inside the caller's warranties.read scope (dealer: its
// organization, distributor: its subtree, center: the brand; else
// ErrCertificateNotFound) and hold at least one active warranty
// (ErrNoActiveWarranty).
func (s *CertificateService) AuthorizeTenant(ctx context.Context, f scopefilter.Filter, brandID int64, serviceUUID uuid.UUID) (db.Service, error) {
	svc, err := s.service(ctx, serviceUUID, brandID)
	if err != nil {
		return db.Service{}, err
	}
	if !f.AllowsOrg(svc.OrganizationID, svc.BrandID) {
		return db.Service{}, ErrCertificateNotFound
	}
	if f.UserOnly() && (!svc.CreatedByUserID.Valid || svc.CreatedByUserID.Int64 != f.UserID) {
		return db.Service{}, ErrCertificateNotFound
	}
	rows, err := s.items(ctx, svc, 0)
	if err != nil {
		return db.Service{}, err
	}
	if len(rows) == 0 {
		return db.Service{}, ErrNoActiveWarranty
	}
	return svc, nil
}

// AuthorizePortal checks a portal request: the customer must hold at least
// one active warranty of the service in the domain brand; anything else is
// ErrCertificateNotFound (existence does not leak).
func (s *CertificateService) AuthorizePortal(ctx context.Context, userID, brandID int64, serviceUUID uuid.UUID) (db.Service, error) {
	if userID <= 0 || brandID <= 0 {
		return db.Service{}, ErrCertificateNotFound
	}
	svc, err := s.service(ctx, serviceUUID, brandID)
	if err != nil {
		return db.Service{}, err
	}
	rows, err := s.items(ctx, svc, userID)
	if err != nil {
		return db.Service{}, err
	}
	if len(rows) == 0 {
		return db.Service{}, ErrCertificateNotFound
	}
	return svc, nil
}

// Build loads the certificate of an export job. Tenant jobs (job
// organization set) re-check that the service is the job organization's,
// below it, or in its brand when the job organization is the center;
// portal jobs print only the holder's warranties.
func (s *CertificateService) Build(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (Certificate, error) {
	id, err := uuid.Parse(strings.TrimSpace(q[QueryServiceUUID]))
	if err != nil {
		return Certificate{}, ErrCertificateNotFound
	}
	brandID, err := strconv.ParseInt(strings.TrimSpace(q[QueryBrandID]), 10, 64)
	if err != nil || brandID <= 0 {
		return Certificate{}, errors.New("warranty certificate: brand is required")
	}
	svc, err := s.service(ctx, id, brandID)
	if err != nil {
		return Certificate{}, err
	}
	var holder int64
	if raw := strings.TrimSpace(q[ioengine.QueryOrganizationID]); raw != "" {
		jobOrg, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || jobOrg <= 0 {
			return Certificate{}, errCertificateScope
		}
		if err := s.checkJobOrg(ctx, jobOrg, svc); err != nil {
			return Certificate{}, err
		}
	} else {
		holder, err = strconv.ParseInt(strings.TrimSpace(q[QueryHolderUserID]), 10, 64)
		if err != nil || holder <= 0 {
			return Certificate{}, errCertificateScope
		}
	}
	rows, err := s.items(ctx, svc, holder)
	if err != nil {
		return Certificate{}, err
	}
	if len(rows) == 0 {
		return Certificate{}, ErrNoActiveWarranty
	}
	return s.certificate(ctx, svc, rows, loc)
}

func (s *CertificateService) checkJobOrg(ctx context.Context, jobOrgID int64, svc db.Service) error {
	ok, err := ioengine.JobOrgCovers(ctx, s.q, jobOrgID, svc.OrganizationID, svc.BrandID)
	if err != nil {
		return fmt.Errorf("warranty certificate: %w", err)
	}
	if !ok {
		return errCertificateScope
	}
	return nil
}

func (s *CertificateService) certificate(ctx context.Context, svc db.Service, rows []db.ListWarrantyCertificateItemsRow, loc i18n.Locale) (Certificate, error) {
	org, err := s.q.GetOrganizationByID(ctx, svc.OrganizationID)
	if err != nil {
		return Certificate{}, fmt.Errorf("warranty certificate: organization: %w", err)
	}
	refs, err := s.q.GetServiceRefs(ctx, svc.ID)
	if err != nil {
		return Certificate{}, fmt.Errorf("warranty certificate: refs: %w", err)
	}
	zone, err := time.LoadLocation(org.Timezone)
	if err != nil || org.Timezone == "" {
		zone = time.UTC
	}
	c := Certificate{
		ServiceUUID: svc.Uuid, ServiceNo: svc.ServiceNo,
		Dealer: CertificateDealer{Name: org.Name, City: org.City, Address: org.Address, Phone: org.Phone,
			Email: org.Email},
		Vehicle:     CertificateVehicle{Brand: refs.CarBrandName, Model: refs.CarModelName},
		GeneratedAt: s.now(),
	}
	if svc.CompletedAt.Valid {
		c.ServiceDate = FormatCertificateDate(svc.CompletedAt.Time, zone, loc)
	}
	if svc.ModelYear.Valid {
		c.Vehicle.Year = strconv.Itoa(int(svc.ModelYear.Int16))
	}
	c.Vehicle.Plate = strings.TrimSpace(svc.Plate.String)
	c.Vehicle.VIN = strings.TrimSpace(svc.Vin.String)
	if c.Vehicle.Plate == "" || c.Vehicle.VIN == "" || c.Vehicle.Year == "" {
		if v, err := s.q.GetVehicleByUUID(ctx, refs.VehicleUuid); err == nil {
			if c.Vehicle.Plate == "" {
				c.Vehicle.Plate = strings.TrimSpace(v.Plate.String)
			}
			if c.Vehicle.VIN == "" {
				c.Vehicle.VIN = strings.TrimSpace(v.Vin.String)
			}
			if c.Vehicle.Year == "" && v.ModelYear.Valid {
				c.Vehicle.Year = strconv.Itoa(int(v.ModelYear.Int16))
			}
		}
	}
	// The holder: every active warranty of a service covers one vehicle, so
	// they share the holder; an anonymized holder is masked (K19).
	if u, err := s.q.GetUserByID(ctx, rows[0].HolderUserID); err == nil {
		if u.Status == "anonymized" {
			c.HolderName = i18n.Translate(loc, "customers.anonymized_name")
		} else {
			c.HolderName = strings.TrimSpace(u.Name + " " + u.Surname)
		}
	}
	for _, r := range rows {
		it := CertificateItem{
			PublicCode: r.PublicCode, Product: r.ProductName, SKU: r.ProductSku, Barcode: r.UnitBarcode,
			Kind:      r.ItemKind,
			StartDate: FormatCertificateDate(r.StartAt.Time, zone, loc),
			EndDate:   FormatCertificateDate(r.EndAt.Time, zone, loc),
			VerifyURL: s.VerifyURL(r.PublicCode),
		}
		if f, err := r.Meters.Float64Value(); err == nil && f.Valid {
			it.Meters = strconv.FormatFloat(f.Float64, 'f', 2, 64)
		}
		c.Items = append(c.Items, it)
	}
	settings, err := s.q.GetAppSettings(ctx)
	if err != nil {
		return Certificate{}, fmt.Errorf("warranty certificate: settings: %w", err)
	}
	if s.store != nil {
		lh, err := ioengine.LoadOrganizationLetterhead(ctx, s.store, org, settings)
		if err != nil {
			s.log.Warn("warranty_certificate_logo_failed", "error", err)
			lh = ioengine.LetterheadFromOrganization(org, settings)
		}
		c.Letterhead = lh
	} else {
		c.Letterhead = ioengine.LetterheadFromOrganization(org, settings)
	}
	return c, nil
}

// FormatCertificateDate prints the calendar day of t in the organization's
// zone: ISO (2006-01-02) for en and zh_CN, day.month.year otherwise.
func FormatCertificateDate(t time.Time, zone *time.Location, loc i18n.Locale) string {
	if zone == nil {
		zone = time.UTC
	}
	t = t.In(zone)
	if loc == i18n.LocaleEN || loc == i18n.LocaleZhCN {
		return t.Format(time.DateOnly)
	}
	return t.Format("02.01.2006")
}

// --- Export adapter ------------------------------------------------------------

// CertificateAdapter exports the warranty certificate of a service.
type CertificateAdapter struct {
	resource string
	svc      *CertificateService
}

// NewCertificateAdapter: the panel certificate (tenant export job).
func NewCertificateAdapter(svc *CertificateService) *CertificateAdapter {
	return &CertificateAdapter{resource: ResourceCertificate, svc: svc}
}

// NewPortalCertificateAdapter: the customer's certificate from the portal.
func NewPortalCertificateAdapter(svc *CertificateService) *CertificateAdapter {
	return &CertificateAdapter{resource: ResourcePortalCertificate, svc: svc}
}

// Resource implements ioengine.ResourceAdapter.
func (a *CertificateAdapter) Resource() string { return a.resource }

// ExportColumns implements ioengine.ResourceAdapter (generic fallback PDF).
func (a *CertificateAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "code", LabelKey: "warranty.certificate.code", Type: ioengine.ColumnTypeString},
		{Key: "product", LabelKey: "warranty.certificate.product", Type: ioengine.ColumnTypeString, Weight: 1.6},
		{Key: "barcode", LabelKey: "warranty.certificate.barcode", Type: ioengine.ColumnTypeString},
		{Key: "start", LabelKey: "warranty.certificate.start", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "end", LabelKey: "warranty.certificate.end", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "verify_url", LabelKey: "warranty.certificate.verify", Type: ioengine.ColumnTypeString, Weight: 2},
	}
}

// ImportSchema implements ioengine.ResourceAdapter (export only).
func (a *CertificateAdapter) ImportSchema() []ioengine.ImportField { return nil }

// ApplyRow implements ioengine.ResourceAdapter (export only).
func (a *CertificateAdapter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "export only"}, nil
}

// RevertRow implements ioengine.ResourceAdapter (export only).
func (a *CertificateAdapter) RevertRow(context.Context, string, string, map[string]any) error {
	return nil
}

// Export implements ioengine.ResourceAdapter. Query: service_uuid,
// brand_id and, for portal jobs, holder_user_id.
func (a *CertificateAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	if a.resource == ResourcePortalCertificate {
		// Portal jobs never carry an organization (exports keeps it unset).
		delete(q, ioengine.QueryOrganizationID)
	} else if strings.TrimSpace(q[ioengine.QueryOrganizationID]) == "" {
		return ioengine.Dataset{}, errCertificateScope
	}
	c, err := a.svc.Build(ctx, q, loc)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	return CertificateDataset(a.resource, c, loc), nil
}

// CertificateDataset maps a certificate to an export dataset.
func CertificateDataset(resource string, c Certificate, loc i18n.Locale) ioengine.Dataset {
	rows := make([]map[string]any, 0, len(c.Items))
	for _, it := range c.Items {
		rows = append(rows, map[string]any{
			"code": it.PublicCode, "product": it.Product, "barcode": it.Barcode,
			"start": it.StartDate, "end": it.EndDate, "verify_url": it.VerifyURL,
		})
	}
	return ioengine.Dataset{
		Resource: resource,
		Columns:  (&CertificateAdapter{}).ExportColumns(),
		Rows:     rows,
		Info: []ioengine.InfoLine{
			{LabelKey: "warranty.certificate.service_no", Value: c.ServiceNo},
			{LabelKey: "warranty.certificate.service_date", Value: c.ServiceDate},
			{LabelKey: "warranty.certificate.dealer", Value: c.Dealer.Name},
			{LabelKey: "warranty.certificate.vehicle", Value: vehicleLabel(c.Vehicle)},
			{LabelKey: "warranty.certificate.plate", Value: c.Vehicle.Plate},
			{LabelKey: "warranty.certificate.vin", Value: c.Vehicle.VIN},
			{LabelKey: "warranty.certificate.holder", Value: c.HolderName},
		},
		Doc: c,
	}
}

func vehicleLabel(v CertificateVehicle) string {
	s := ioengine.JoinNonEmpty(" ", v.Brand, v.Model)
	if v.Year != "" {
		s += " (" + v.Year + ")"
	}
	return s
}

// DocumentHTML implements ioengine.DocumentRenderer: the styled certificate
// on the dealer's letterhead, one card per warranty with its QR code, then
// the warranty terms.
func (a *CertificateAdapter) DocumentHTML(ds ioengine.Dataset, locale string, _ *ioengine.Letterhead, title string) (string, error) {
	c, ok := ds.Doc.(Certificate)
	if !ok {
		return "", errors.New("warranty certificate: dataset carries no certificate")
	}
	return CertificateHTML(c, i18n.Normalize(locale), title)
}

// CertificateHTML renders the certificate document. Every value is escaped;
// QR codes are PNG data: URIs (the document CSP allows no other source).
func CertificateHTML(c Certificate, loc i18n.Locale, title string) (string, error) {
	t := func(key string) string { return i18n.Translate(loc, key) }
	esc := html.EscapeString
	var b strings.Builder
	lh := c.Letterhead
	b.WriteString(certificateCSS)
	b.WriteString(ioengine.LetterheadHeaderHTML(&lh))
	b.WriteString(`<h1 class="doc-title">` + esc(title) + `</h1><table class="doc-meta">`)
	meta := [][2]string{
		{t("warranty.certificate.service_no"), c.ServiceNo},
		{t("warranty.certificate.service_date"), c.ServiceDate},
		{t("warranty.certificate.dealer"), ioengine.JoinNonEmpty(" · ", c.Dealer.Name, c.Dealer.City)},
		{t("warranty.certificate.dealer_contact"), ioengine.JoinNonEmpty(" · ", c.Dealer.Address, c.Dealer.Phone, c.Dealer.Email)},
		{t("warranty.certificate.vehicle"), vehicleLabel(c.Vehicle)},
		{t("warranty.certificate.plate"), c.Vehicle.Plate},
		{t("warranty.certificate.vin"), c.Vehicle.VIN},
		{t("warranty.certificate.holder"), c.HolderName},
	}
	for _, row := range meta {
		if strings.TrimSpace(row[1]) == "" {
			continue
		}
		b.WriteString(`<tr><th>` + esc(row[0]) + `</th><td>` + esc(row[1]) + `</td></tr>`)
	}
	b.WriteString(`</table><h2>` + esc(t("warranty.certificate.warranties")) + `</h2>`)
	for _, it := range c.Items {
		qr, err := pdfrender.QRCodeImageTag(it.VerifyURL, t("warranty.certificate.verify")+" "+it.PublicCode)
		if err != nil {
			return "", fmt.Errorf("warranty certificate: qr %s: %w", it.PublicCode, err)
		}
		kind := t("warranty.certificate.kind." + it.Kind)
		if it.Meters != "" {
			kind += " (" + it.Meters + " m)"
		}
		b.WriteString(`<section class="wc-card"><div class="wc-body"><table class="doc-meta">`)
		for _, row := range [][2]string{
			{t("warranty.certificate.code"), it.PublicCode},
			{t("warranty.certificate.product"), ioengine.JoinNonEmpty(" · ", it.Product, it.SKU)},
			{t("warranty.certificate.barcode"), it.Barcode},
			{t("warranty.certificate.kind"), kind},
			{t("warranty.certificate.start"), it.StartDate},
			{t("warranty.certificate.end"), it.EndDate},
		} {
			b.WriteString(`<tr><th>` + esc(row[0]) + `</th><td>` + esc(row[1]) + `</td></tr>`)
		}
		b.WriteString(`</table></div><div class="wc-qr">` + qr + `<p class="wc-url" dir="ltr">` + esc(it.VerifyURL) + `</p></div></section>`)
	}
	b.WriteString(`<p class="muted">` + esc(t("warranty.certificate.verify_hint")) + `</p>`)
	b.WriteString(`<h2>` + esc(t("warranty.certificate.terms_title")) + `</h2><ol class="wc-terms">`)
	for _, line := range strings.Split(t("warranty.certificate.terms"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			b.WriteString(`<li>` + esc(line) + `</li>`)
		}
	}
	b.WriteString(`</ol><p class="muted">` + esc(t("warranty.certificate.generated_at")) + `: ` +
		esc(c.GeneratedAt.UTC().Format("2006-01-02 15:04 UTC")) + `</p>`)
	b.WriteString(ioengine.LetterheadFooterHTML(&lh))
	return pdfrender.Document{Lang: string(loc), Title: title, Body: b.String(), PrimaryColor: lh.PrimaryColor}.HTML(), nil
}

// certificateCSS lays out the warranty cards (logical properties, so the
// layout mirrors in RTL). The skeleton allows inline styles.
const certificateCSS = `<style>
.wc-card{display:flex;gap:12pt;align-items:flex-start;border:1px solid #d1d5db;border-inline-start:4px solid var(--primary);padding:8pt;margin-block-end:10pt;break-inside:avoid}
.wc-body{flex:1}
.wc-body table.doc-meta{margin:0}
.wc-qr{width:96pt;text-align:center}
.wc-qr img{width:88pt;height:88pt;background:#fff}
.wc-url{font-size:6.5pt;word-break:break-all;color:#6b7280;margin:2pt 0 0}
.wc-terms{font-size:9pt;padding-inline-start:14pt}
.wc-terms li{margin-block-end:3pt}
</style>`

var (
	_ ioengine.ResourceAdapter  = (*CertificateAdapter)(nil)
	_ ioengine.DocumentRenderer = (*CertificateAdapter)(nil)
)
