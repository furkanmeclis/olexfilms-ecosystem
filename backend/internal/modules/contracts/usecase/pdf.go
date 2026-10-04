package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/model"
	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
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

// ProcessExecutedEvent renders and stores the immutable PDF for a
// contract.executed event. It is idempotent: an instance with pdf_key set is
// left untouched.
func (s *Service) ProcessExecutedEvent(ctx context.Context, event events.Event) error {
	if event.EntityID != nil {
		return s.GenerateExecutedPDF(ctx, *event.EntityID)
	}
	if event.EntityUUID != nil {
		row, err := s.repo.Queries().GetContractInstanceByUUID(ctx, *event.EntityUUID)
		if err != nil {
			if errorsIsNoRows(err) {
				return nil
			}
			return err
		}
		return s.GenerateExecutedPDF(ctx, row.ID)
	}
	if raw, ok := event.Payload["contract_uuid"].(string); ok {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil
		}
		row, err := s.repo.Queries().GetContractInstanceByUUID(ctx, id)
		if err != nil {
			if errorsIsNoRows(err) {
				return nil
			}
			return err
		}
		return s.GenerateExecutedPDF(ctx, row.ID)
	}
	return nil
}

// GenerateExecutedPDF renders, uploads and records the executed contract PDF.
func (s *Service) GenerateExecutedPDF(ctx context.Context, instanceID int64) error {
	q := s.repo.Queries()
	row, err := q.GetContractInstanceByID(ctx, instanceID)
	if err != nil {
		if errorsIsNoRows(err) {
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
	if s.storage == nil {
		return ErrStorageRequired
	}
	if s.pdf == nil {
		return pdfrender.ErrNotConfigured
	}
	pdf, objectKey, err := s.renderExecutedPDF(ctx, row)
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
	if _, err := q.SetContractInstancePDFKey(ctx, db.SetContractInstancePDFKeyParams{ID: row.ID, PdfKey: objectKey}); err != nil {
		if errorsIsNoRows(err) {
			return nil
		}
		return err
	}
	return nil
}

func (s *Service) renderExecutedPDF(ctx context.Context, row db.ContractInstance) ([]byte, string, error) {
	q := s.repo.Queries()
	org, err := q.GetOrganizationByID(ctx, row.OrganizationID)
	if err != nil {
		return nil, "", err
	}
	vars, err := s.contractDocumentVars(ctx, row, org)
	if err != nil {
		return nil, "", err
	}
	locale := docmodel.NormalizeLanguage(row.Locale)
	if locale == "" {
		locale = docmodel.FallbackLanguage
	}
	tpl, err := s.contractDocumentTemplate(ctx, row.BrandID, locale)
	if err != nil {
		return nil, "", err
	}
	spec, _ := docmodel.Spec(docmodel.KindContract)
	body := pdfrender.Fill(pdfrender.SanitizeHTML(tpl.Html), vars, spec.RawHTMLKeys())
	title := fmt.Sprintf("Contract %d", row.ContractNo)
	htmlDoc := pdfrender.Document{
		Lang: tpl.Language, Title: title, Body: body, PrimaryColor: org.PrimaryColor, Fonts: pdfrender.FontsEmbedded,
	}.HTML()
	data, err := s.pdf.Convert(ctx, pdfrender.Request{HTML: htmlDoc, FooterHTML: pdfrender.FooterHTML(tpl.Language, org.Name)})
	if err != nil {
		return nil, "", err
	}
	return data, platstorage.ContractExecutedPDFObjectKey(org.Uuid, row.Uuid), nil
}

func (s *Service) contractDocumentTemplate(ctx context.Context, brandID int64, locale string) (db.DocumentTemplate, error) {
	q := s.repo.Queries()
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

func (s *Service) contractDocumentVars(ctx context.Context, row db.ContractInstance, org db.Organization) (map[string]string, error) {
	signers, err := s.repo.Queries().ListContractPDFSigners(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	media, err := s.repo.Queries().ListContractMedia(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	vars := map[string]string{
		"company_name":       org.Name,
		"company_address":    org.Address,
		"company_phone":      org.Phone,
		"company_email":      org.Email,
		"company_website":    org.Website,
		"footer_text":        org.FooterText,
		"document_number":    strconv.FormatInt(row.ContractNo, 10),
		"contract_title":     "Contract " + strconv.FormatInt(row.ContractNo, 10),
		"contract_body_html": pdfrender.SanitizeHTML(row.RenderedHtml.String),
		"content_sha256":     row.ContentSha256.String,
		"otp_proof_html":     otpProofHTML(signers, row.ContentSha256.String),
		"signatures_html":    s.signaturesHTML(ctx, signers),
		"signature_image":    s.signaturesHTML(ctx, signers),
		"media_html":         s.mediaHTML(ctx, media),
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
	return vars, nil
}

func (s *Service) signaturesHTML(ctx context.Context, signers []db.ListContractPDFSignersRow) string {
	if len(signers) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="doc-sign">`)
	for _, sg := range signers {
		alt := sg.Name + " signature"
		img := ""
		if strings.TrimSpace(sg.SignatureStorageKey) != "" {
			img = s.imageTag(ctx, sg.SignatureStorageKey, "image/png", alt)
		}
		b.WriteString(`<div><strong>`)
		b.WriteString(html.EscapeString(sg.Name))
		b.WriteString(`</strong><br>`)
		b.WriteString(html.EscapeString(sg.Role))
		if img != "" {
			b.WriteString(`<br>`)
			b.WriteString(img)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func (s *Service) mediaHTML(ctx context.Context, media []db.ContractMedium) string {
	if len(media) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section><h2>Media</h2>`)
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
	return pdfrender.ImageTag(mime, data, alt)
}

func otpProofHTML(rows []db.ListContractPDFSignersRow, contentSHA string) string {
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
		{Label: "Signer"}, {Label: "Masked phone"}, {Label: "Verified at"}, {Label: "KVKK notice version"}, {Label: "Content SHA-256"},
	}, table)
}

func maskPhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if len(phone) <= 6 {
		return phone
	}
	return phone[:len(phone)-4] + "****" + phone[len(phone)-2:]
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

// DownloadPDF opens the ready executed PDF for a panel caller.
func (s *Service) DownloadPDF(ctx context.Context, c Caller, id uuid.UUID) (io.ReadCloser, string, error) {
	if c.BrandID <= 0 || len(c.Filter.OrgIDs) == 0 {
		return nil, "", ErrNotFound
	}
	row, err := s.repo.Queries().GetContractInstanceByUUIDScoped(ctx, db.GetContractInstanceByUUIDScopedParams{
		Uuid: id, BrandID: pgtype.Int8{Int64: c.BrandID, Valid: true}, OrgIds: c.Filter.OrgIDs,
	})
	if err != nil {
		return nil, "", notFound(err)
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
	row, err := l.s.repo.Queries().GetContractInstanceByUUID(ctx, id)
	if err != nil {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	if !viewer.System && (row.OrganizationID != viewer.OrganizationID || (viewer.BrandID > 0 && row.BrandID != viewer.BrandID)) {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	org, err := l.s.repo.Queries().GetOrganizationByID(ctx, row.OrganizationID)
	if err != nil {
		return docmodel.Source{}, err
	}
	vars, err := l.s.contractDocumentVars(ctx, row, org)
	if err != nil {
		return docmodel.Source{}, err
	}
	version := row.UpdatedAt.Time.UTC().Format(time.RFC3339Nano)
	if row.ContentSha256.Valid {
		version = row.ContentSha256.String + ":" + version
	}
	return docmodel.Source{
		OrganizationID: row.OrganizationID, BrandID: row.BrandID, Version: version,
		Vars: vars, Title: fmt.Sprintf("Contract %d", row.ContractNo),
	}, nil
}

func errorsIsNoRows(err error) bool {
	return err == pgx.ErrNoRows
}
