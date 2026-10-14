package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/model"
	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	psmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/photostandard/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/imagefit"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	platstorage "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const contractDocumentSourceType = "contract_instance"

// PortalCaller is the portal owner scope for contract PDFs.
type PortalCaller struct {
	UserID  int64
	BrandID int64
}

// GenerateExecutedPDF renders, uploads and records the executed contract PDF
// (worker-docs task contract:pdf). It is idempotent and safe under
// concurrent delivery: the instance row is locked FOR UPDATE for the whole
// render, so a second run waits and then sees pdf_key already set; pdf_key
// is written in the same transaction.
func (s *Service) GenerateExecutedPDF(ctx context.Context, instanceID int64) error {
	if s.storage == nil {
		return ErrStorageRequired
	}
	if s.pdf == nil {
		return pdfrender.ErrNotConfigured
	}
	tx, qtx, err := s.repo.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := qtx.GetContractInstanceForUpdate(ctx, instanceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if row.Status != model.StatusExecuted {
		return nil
	}
	if row.PdfKey.Valid && strings.TrimSpace(row.PdfKey.String) != "" {
		return nil
	}
	pdf, objectKey, err := s.renderExecutedPDF(ctx, qtx, row)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(pdf)
	if err := s.storage.Upload(ctx, platstorage.File{
		Body: bytes.NewReader(pdf), Size: int64(len(pdf)), ContentType: "application/pdf",
		Filename: fmt.Sprintf("contract-%d.pdf", row.ContractNo),
		Metadata: map[string]string{"sha256": hex.EncodeToString(sum[:]), "contract_uuid": row.Uuid.String()},
	}, objectKey); err != nil {
		return err
	}
	if _, err := qtx.SetContractInstancePDFKey(ctx, db.SetContractInstancePDFKeyParams{ID: row.ID, PdfKey: objectKey}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) renderExecutedPDF(ctx context.Context, q *db.Queries, row db.ContractInstance) ([]byte, string, error) {
	org, err := q.GetOrganizationByID(ctx, row.OrganizationID)
	if err != nil {
		return nil, "", err
	}
	vars, err := s.contractDocumentVars(ctx, q, row, org)
	if err != nil {
		return nil, "", err
	}
	locale := docmodel.NormalizeLanguage(row.Locale)
	if locale == "" {
		locale = docmodel.FallbackLanguage
	}
	tpl, err := s.contractDocumentTemplate(ctx, q, row.BrandID, locale)
	if err != nil {
		return nil, "", err
	}
	spec, _ := docmodel.Spec(docmodel.KindContract)
	loc := contractLocale(row.Locale)
	hashLabel := i18n.Translate(loc, "contracts.pdf.proof.content_sha256")
	body := pdfrender.Fill(withEvidenceBlocks(pdfrender.SanitizeHTML(tpl.Html), hashLabel), vars, spec.RawHTMLKeys())
	// TEC-526: the document language (and so its direction) follows the
	// contract's locale, not the document template's: an ar contract rendered
	// through the tr/en fallback template still comes out RTL.
	lang := string(loc)
	htmlDoc := pdfrender.Document{
		Lang: lang, Title: contractTitle(loc, row.ContractNo), Body: body, PrimaryColor: org.PrimaryColor, Fonts: pdfrender.FontsEmbedded,
	}.HTML()
	data, err := s.pdf.Convert(ctx, pdfrender.Request{HTML: htmlDoc, FooterHTML: pdfrender.FooterHTML(lang, org.Name)})
	if err != nil {
		return nil, "", err
	}
	return data, platstorage.ContractExecutedPDFObjectKey(org.Uuid, row.Uuid), nil
}

// withEvidenceBlocks appends the signature, OTP proof, content hash and
// media placeholders a template does not reference itself. The seeded
// system contract template (000030) predates them, and the executed PDF must
// always carry the signing evidence. hashLabel is the localized caption of
// the content hash line.
func withEvidenceBlocks(tpl, hashLabel string) string {
	used := map[string]bool{}
	for _, k := range msgtemplate.Placeholders(tpl) {
		used[k] = true
	}
	var b strings.Builder
	if !used["signatures_html"] && !used["signature_image"] {
		b.WriteString("{{signatures_html}}")
	}
	if !used["otp_proof_html"] {
		b.WriteString("{{otp_proof_html}}")
	}
	if !used["content_sha256"] && !used["otp_proof_html"] {
		b.WriteString(`<p class="doc-hash">`)
		b.WriteString(html.EscapeString(hashLabel))
		b.WriteString(`: {{content_sha256}}</p>`)
	}
	if !used["media_html"] {
		b.WriteString("{{media_html}}")
	}
	if b.Len() == 0 {
		return tpl
	}
	return tpl + `<section class="doc-evidence">` + b.String() + `</section>`
}

func (s *Service) contractDocumentTemplate(ctx context.Context, q *db.Queries, brandID int64, locale string) (db.DocumentTemplate, error) {
	brand := pgtype.Int8{Int64: brandID, Valid: brandID > 0}
	for _, c := range []struct {
		brand pgtype.Int8
		lang  string
	}{{brand, locale}, {pgtype.Int8{}, locale}, {brand, docmodel.FallbackLanguage}, {pgtype.Int8{}, docmodel.FallbackLanguage}} {
		row, err := q.GetActiveDocumentTemplate(ctx, db.GetActiveDocumentTemplateParams{
			Kind: docmodel.KindContract, Language: c.lang, BrandID: c.brand,
		})
		if err == nil {
			return row, nil
		}
		if !errorsIsNoRows(err) {
			return db.DocumentTemplate{}, err
		}
	}
	return db.DocumentTemplate{}, ErrNotFound
}

func (s *Service) contractDocumentVars(ctx context.Context, q *db.Queries, row db.ContractInstance, org db.Organization) (map[string]string, error) {
	signers, err := q.ListContractPDFSigners(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	media, err := q.ListContractMedia(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	loc := contractLocale(row.Locale)
	vars := map[string]string{
		"company_name":       org.Name,
		"company_address":    org.Address,
		"company_phone":      org.Phone,
		"company_email":      org.Email,
		"company_website":    org.Website,
		"footer_text":        org.FooterText,
		"document_number":    strconv.FormatInt(row.ContractNo, 10),
		"contract_title":     contractTitle(loc, row.ContractNo),
		"label_contract_no":  i18n.Translate(loc, "contracts.pdf.number"),
		"label_date":         i18n.Translate(loc, "contracts.pdf.date"),
		"label_party":        i18n.Translate(loc, "contracts.pdf.party"),
		"label_signed_at":    i18n.Translate(loc, "contracts.pdf.signed_at"),
		"contract_body_html": pdfrender.SanitizeHTML(row.RenderedHtml.String),
		"content_sha256":     row.ContentSha256.String,
		"otp_proof_html":     otpProofHTML(loc, signers, row.ContentSha256.String),
		"signatures_html":    s.signaturesHTML(ctx, loc, signers),
		"signature_image":    s.signaturesHTML(ctx, loc, signers),
		"media_html":         s.mediaHTML(ctx, loc, media),
	}
	if row.ExecutedAt.Valid {
		ts := row.ExecutedAt.Time.UTC()
		vars["signed_at"] = ts.Format(time.RFC3339)
		vars["document_date"] = ts.Format("2006-01-02")
	}
	for _, sg := range signers {
		if sg.Role == model.SignerCustomer {
			vars["party_name"] = sg.Name
			break
		}
	}
	if org.LogoObjectKey.Valid {
		if tag := s.imageTag(ctx, org.LogoObjectKey.String, "image/png", org.Name); tag != "" {
			vars["company_logo"] = tag
		}
	}
	// TEC-499: intake photo grid (angle name + capture time, no EXIF
	// location); separate from the contract's own media_html.
	vars["intake_photos_html"] = ""
	if s.intake != nil && row.SubjectType == subjectService {
		grid, err := s.intake.IntakePhotosHTML(ctx, q, psmodel.ServiceRef{
			ID: row.SubjectID, OrganizationID: row.OrganizationID, BrandID: row.BrandID,
		}, row.Locale)
		if err != nil {
			return nil, err
		}
		vars["intake_photos_html"] = grid
	}
	return vars, nil
}

func (s *Service) signaturesHTML(ctx context.Context, loc i18n.Locale, signers []db.ListContractPDFSignersRow) string {
	if len(signers) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="doc-sign">`)
	for _, sg := range signers {
		role := signerRoleLabel(loc, sg.Role)
		img := ""
		if strings.TrimSpace(sg.SignatureStorageKey) != "" {
			img = s.imageTag(ctx, sg.SignatureStorageKey, "image/png", sg.Name+" / "+role)
		}
		b.WriteString(`<div><strong>`)
		b.WriteString(html.EscapeString(sg.Name))
		b.WriteString(`</strong><br>`)
		b.WriteString(html.EscapeString(role))
		if img != "" {
			b.WriteString(`<br>`)
			b.WriteString(img)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func (s *Service) mediaHTML(ctx context.Context, loc i18n.Locale, media []db.ContractMedium) string {
	if len(media) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section><h2>`)
	b.WriteString(html.EscapeString(i18n.Translate(loc, "contracts.pdf.attachments")))
	b.WriteString(`</h2>`)
	for _, m := range media {
		tag := s.imageTag(ctx, m.StorageKey, m.MimeType, m.Title.String)
		if tag == "" {
			continue
		}
		b.WriteString(`<figure>`)
		b.WriteString(tag)
		if m.Title.Valid {
			b.WriteString(`<figcaption>`)
			b.WriteString(html.EscapeString(m.Title.String))
			b.WriteString(`</figcaption>`)
		}
		b.WriteString(`</figure>`)
	}
	b.WriteString(`</section>`)
	return b.String()
}

func (s *Service) imageTag(ctx context.Context, key, mime, alt string) string {
	if s.storage == nil || strings.TrimSpace(key) == "" {
		return ""
	}
	rc, _, err := s.storage.Download(ctx, key)
	if err != nil {
		return ""
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, MaxMediaBytes))
	if err != nil {
		return ""
	}
	// TEC-642: a phone photo embedded as is is decoded to a raw bitmap in
	// the PDF (tens of MB); shrink it, upright, as JPEG.
	if small, err := imagefit.JPEG(data, mime, PDFMediaMaxPx); err == nil {
		data, mime = small, "image/jpeg"
	}
	return pdfrender.ImageTag(mime, data, alt)
}

func otpProofHTML(loc i18n.Locale, rows []db.ListContractPDFSignersRow, contentSHA string) string {
	table := [][]string{}
	for _, sg := range rows {
		if !sg.OtpVerifiedAt.Valid && !sg.KvkkVersion.Valid {
			continue
		}
		table = append(table, []string{
			sg.Name,
			maskPhone(sg.PhoneE164.String),
			formatTime(sg.OtpVerifiedAt),
			kvkkVersion(sg.KvkkLocale, sg.KvkkVersion),
			contentSHA,
		})
	}
	if len(table) == 0 {
		table = append(table, []string{"-", "-", "-", "-", contentSHA})
	}
	return pdfrender.Table([]pdfrender.Column{
		{Label: i18n.Translate(loc, "contracts.pdf.proof.signer")},
		{Label: i18n.Translate(loc, "contracts.pdf.proof.phone")},
		{Label: i18n.Translate(loc, "contracts.pdf.proof.verified_at")},
		{Label: i18n.Translate(loc, "contracts.pdf.proof.kvkk_version")},
		{Label: i18n.Translate(loc, "contracts.pdf.proof.content_sha256")},
	}, table)
}

// contractLocale maps the instance's stored locale (tr, en, ..., zh_CN) to the
// backend catalog locale; empty or unknown falls back to the default (tr).
func contractLocale(raw string) i18n.Locale {
	return i18n.Normalize(raw)
}

// contractTitle is the localized "Contract N" caption used for the document
// title, the {{contract_title}} variable and the document-center source name.
func contractTitle(loc i18n.Locale, no int64) string {
	return i18n.TranslateParams(loc, "contracts.pdf.title", map[string]string{"no": strconv.FormatInt(no, 10)})
}

// signerRoleLabel translates a fixed signer slot (customer, staff); an
// unknown role is printed as is rather than as a dotted key.
func signerRoleLabel(loc i18n.Locale, role string) string {
	key := "contracts.pdf.role." + strings.TrimSpace(role)
	if v := i18n.Translate(loc, key); v != key {
		return v
	}
	return role
}

func maskPhone(e164 string) string {
	e164 = strings.TrimSpace(e164)
	if e164 == "" {
		return ""
	}
	return phone.Mask(e164)
}

func formatTime(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.UTC().Format(time.RFC3339)
}

func kvkkVersion(locale pgtype.Text, version pgtype.Int4) string {
	if !version.Valid {
		return ""
	}
	if locale.Valid && strings.TrimSpace(locale.String) != "" {
		return strings.TrimSpace(locale.String) + " v" + strconv.FormatInt(int64(version.Int32), 10)
	}
	return "v" + strconv.FormatInt(int64(version.Int32), 10)
}

// DownloadPDF opens the ready executed PDF for a panel caller, under the
// same scope rules as GetContract (brand/all reach, own/assigned user check).
func (s *Service) DownloadPDF(ctx context.Context, c Caller, id uuid.UUID) (io.ReadCloser, string, error) {
	row, err := s.scopedInstance(ctx, c, id)
	if err != nil {
		return nil, "", err
	}
	return s.downloadPDF(ctx, row)
}

// DownloadPortalPDF opens the ready executed PDF when the portal user owns the
// service (same rule as ListPortalContracts).
func (s *Service) DownloadPortalPDF(ctx context.Context, c PortalCaller, id uuid.UUID) (io.ReadCloser, string, error) {
	if c.UserID <= 0 || c.BrandID <= 0 {
		return nil, "", ErrNotFound
	}
	row, err := s.repo.Queries().GetPortalContractPDF(ctx, db.GetPortalContractPDFParams{Uuid: id, UserID: c.UserID, BrandID: c.BrandID})
	if err != nil {
		return nil, "", notFound(err)
	}
	return s.downloadPDF(ctx, row)
}

func (s *Service) downloadPDF(ctx context.Context, row db.ContractInstance) (io.ReadCloser, string, error) {
	if row.Status != model.StatusExecuted || !row.PdfKey.Valid || strings.TrimSpace(row.PdfKey.String) == "" {
		return nil, "", ErrNotFound
	}
	if s.storage == nil {
		return nil, "", ErrStorageRequired
	}
	rc, _, err := s.storage.Download(ctx, row.PdfKey.String)
	if err != nil {
		return nil, "", ErrNotFound
	}
	return rc, fmt.Sprintf("contract-%d.pdf", row.ContractNo), nil
}

type contractDocumentLoader struct{ s *Service }

func (l contractDocumentLoader) SourceType() string { return contractDocumentSourceType }

func (l contractDocumentLoader) Load(ctx context.Context, viewer docmodel.Viewer, sourceID string, _ string) (docmodel.Source, error) {
	id, err := uuid.Parse(strings.TrimSpace(sourceID))
	if err != nil {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	q := l.s.repo.Queries()
	row, err := q.GetContractInstanceByUUID(ctx, id)
	if err != nil {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	if !viewer.System && (row.OrganizationID != viewer.OrganizationID || (viewer.BrandID > 0 && row.BrandID != viewer.BrandID)) {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	org, err := q.GetOrganizationByID(ctx, row.OrganizationID)
	if err != nil {
		return docmodel.Source{}, err
	}
	vars, err := l.s.contractDocumentVars(ctx, q, row, org)
	if err != nil {
		return docmodel.Source{}, err
	}
	version := row.UpdatedAt.Time.UTC().Format(time.RFC3339Nano)
	if row.ContentSha256.Valid {
		version = row.ContentSha256.String + ":" + version
	}
	return docmodel.Source{
		OrganizationID: row.OrganizationID, BrandID: row.BrandID, Version: version,
		Vars: vars, Title: contractTitle(contractLocale(row.Locale), row.ContractNo),
	}, nil
}

func errorsIsNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
