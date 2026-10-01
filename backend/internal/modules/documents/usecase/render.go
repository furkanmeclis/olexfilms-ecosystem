package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Render statuses (document_renders.status).
const (
	RenderPending    = "pending"
	RenderProcessing = "processing"
	RenderReady      = "ready"
	RenderFailed     = "failed"
)

// RenderInput asks for one document of a business record.
type RenderInput struct {
	Kind     string
	SourceID string
	Locale   string
}

// RequestRender returns the document for a source: the cached PDF when the
// cache key (template hash+version, source version, locale, brand, org) is
// already rendered, otherwise a pending row and one queued task. Repeated
// requests for the same key never enqueue twice (task id = cache key).
func (s *Service) RequestRender(ctx context.Context, viewer model.Viewer, in RenderInput) (model.RenderView, bool, error) {
	if _, ok := model.Spec(in.Kind); !ok {
		return model.RenderView{}, false, fmt.Errorf("%w: unknown kind", ErrInvalidRequest)
	}
	sourceID := strings.TrimSpace(in.SourceID)
	if sourceID == "" || len(sourceID) > 64 {
		return model.RenderView{}, false, fmt.Errorf("%w: source_id is required", ErrInvalidRequest)
	}
	loader, ok := s.loader(in.Kind)
	if !ok {
		return model.RenderView{}, false, fmt.Errorf("%w: no source registered for kind %s", ErrInvalidRequest, in.Kind)
	}
	locale := model.NormalizeLanguage(in.Locale)
	if locale == "" {
		locale = model.FallbackLanguage
	}
	src, err := loader.Load(ctx, viewer, sourceID, locale)
	if err != nil {
		if errors.Is(err, model.ErrSourceNotFound) {
			return model.RenderView{}, false, ErrNotFound
		}
		return model.RenderView{}, false, err
	}
	// Organization + brand scope (K20): a source of another organization is
	// invisible even if a loader forgot to check.
	if src.OrganizationID != viewer.OrganizationID || (viewer.BrandID != 0 && src.BrandID != viewer.BrandID) {
		return model.RenderView{}, false, ErrNotFound
	}
	org, err := s.q.GetOrganizationByID(ctx, src.OrganizationID)
	if err != nil {
		return model.RenderView{}, false, notFound(err)
	}
	tpl, err := s.resolveTemplate(ctx, in.Kind, src.BrandID, locale)
	if err != nil {
		return model.RenderView{}, false, err
	}
	key := cacheKey(tpl, in.Kind, loader.SourceType(), sourceID, src.Version, locale, src.BrandID, org, s.fonts)
	row, err := s.q.UpsertDocumentRender(ctx, db.UpsertDocumentRenderParams{
		OrganizationID: src.OrganizationID, BrandID: src.BrandID, Kind: in.Kind,
		TemplateID: tpl.ID, TemplateVersion: tpl.Version,
		SourceType: loader.SourceType(), SourceID: sourceID, SourceVersion: src.Version,
		Locale: locale, CacheKey: key,
		RequestedBy: pgtype.Int8{Int64: viewer.UserID, Valid: viewer.UserID > 0},
	})
	if err != nil {
		return model.RenderView{}, false, err
	}
	switch row.Status {
	case RenderReady:
		if s.objectPresent(ctx, row) {
			return renderView(row), true, nil
		}
		// stored object vanished: render again under a new attempt
		_ = s.q.MarkDocumentRenderFailed(ctx, db.MarkDocumentRenderFailedParams{ID: row.ID, Error: pgtype.Text{String: "stored object missing", Valid: true}})
		fallthrough
	case RenderFailed:
		if row, err = s.q.RetryDocumentRender(ctx, row.ID); err != nil {
			return model.RenderView{}, false, err
		}
	}
	if s.queue == nil {
		if err := s.ProcessRender(ctx, row.ID); err != nil {
			return model.RenderView{}, false, err
		}
		row, err = s.q.GetDocumentRenderByID(ctx, row.ID)
		if err != nil {
			return model.RenderView{}, false, err
		}
		return renderView(row), row.Status == RenderReady, nil
	}
	if err := s.enqueue(row); err != nil {
		return model.RenderView{}, false, err
	}
	return renderView(row), false, nil
}

func (s *Service) enqueue(row db.DocumentRender) error {
	task, opts, err := queue.NewDocsRenderTask(row.ID, row.CacheKey, row.Attempts)
	if err != nil {
		return err
	}
	if _, err := s.queue.Enqueue(task, opts...); err != nil {
		if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
			return nil // already queued for this cache key
		}
		return err
	}
	return nil
}

// ProcessRender is the worker-docs handler for app:docs:render. It is
// idempotent: a ready row whose object is in storage is left untouched.
func (s *Service) ProcessRender(ctx context.Context, renderID int64) error {
	row, err := s.q.GetDocumentRenderByID(ctx, renderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // row deleted (organization removed): nothing to do
		}
		return err
	}
	if row.Status == RenderReady && s.objectPresent(ctx, row) {
		return nil
	}
	if s.pdf == nil {
		return ErrUnavailable
	}
	if row, err = s.q.MarkDocumentRenderProcessing(ctx, row.ID); err != nil {
		return err
	}
	data, err := s.renderRow(ctx, row)
	if err != nil {
		msg := err.Error()
		if len(msg) > 500 {
			msg = msg[:500]
		}
		_ = s.q.MarkDocumentRenderFailed(ctx, db.MarkDocumentRenderFailedParams{ID: row.ID, Error: pgtype.Text{String: msg, Valid: true}})
		return err
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	org, err := s.q.GetOrganizationByID(ctx, row.OrganizationID)
	if err != nil {
		return err
	}
	objectKey := storage.DocumentObjectKey(org.Uuid, row.Kind, row.Uuid)
	if err := s.store.Upload(ctx, storage.File{
		Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: "application/pdf",
		Filename: downloadName(row), Metadata: map[string]string{"sha256": digest, "cache-key": row.CacheKey},
	}, objectKey); err != nil {
		_ = s.q.MarkDocumentRenderFailed(ctx, db.MarkDocumentRenderFailedParams{ID: row.ID, Error: pgtype.Text{String: err.Error(), Valid: true}})
		return err
	}
	_, err = s.q.MarkDocumentRenderReady(ctx, db.MarkDocumentRenderReadyParams{
		ID: row.ID, StorageKey: pgtype.Text{String: objectKey, Valid: true},
		Sha256: pgtype.Text{String: digest, Valid: true}, SizeBytes: pgtype.Int8{Int64: int64(len(data)), Valid: true},
	})
	return err
}

func (s *Service) renderRow(ctx context.Context, row db.DocumentRender) ([]byte, error) {
	loader, ok := s.loader(row.Kind)
	if !ok {
		return nil, fmt.Errorf("documents: no source registered for kind %s", row.Kind)
	}
	tpl, err := s.q.GetDocumentTemplateByID(ctx, row.TemplateID)
	if err != nil {
		return nil, err
	}
	src, err := loader.Load(ctx, model.Viewer{OrganizationID: row.OrganizationID, BrandID: row.BrandID, System: true}, row.SourceID, row.Locale)
	if err != nil {
		return nil, err
	}
	if src.OrganizationID != row.OrganizationID {
		return nil, fmt.Errorf("documents: source moved to another organization")
	}
	if src.Version != row.SourceVersion {
		s.log.Info("document_source_changed_since_request", "render", row.Uuid, "kind", row.Kind)
	}
	org, err := s.q.GetOrganizationByID(ctx, row.OrganizationID)
	if err != nil {
		return nil, err
	}
	vars := s.organizationVars(ctx, org)
	for k, v := range src.Vars {
		vars[strings.ToLower(k)] = v
	}
	title := src.Title
	if title == "" {
		title = tpl.Name
	}
	return s.convert(ctx, tpl, vars, title, org.PrimaryColor, org.Name)
}

func (s *Service) convert(ctx context.Context, tpl db.DocumentTemplate, vars map[string]string, title, color, footer string) ([]byte, error) {
	html := BuildHTML(tpl.Kind, tpl.Html, tpl.Language, vars, title, color, s.fonts)
	return s.pdf.Convert(ctx, pdfrender.Request{HTML: html, FooterHTML: pdfrender.FooterHTML(tpl.Language, footer)})
}

// BuildHTML is the render hot path: sanitize the template, fill escaped
// values (html blocks raw), wrap in the CSP/RTL/font skeleton. lang is the
// template language actually used (fallback may differ from the request).
func BuildHTML(kind, templateHTML, lang string, vars map[string]string, title, color string, fonts pdfrender.FontMode) string {
	spec, _ := model.Spec(kind)
	body := pdfrender.Fill(pdfrender.SanitizeHTML(templateHTML), vars, spec.RawHTMLKeys())
	return pdfrender.Document{
		Lang: strings.ReplaceAll(lang, "_", "-"), Title: title, Body: body, PrimaryColor: color, Fonts: fonts,
	}.HTML()
}

// resolveTemplate picks the active template: brand+locale, platform+locale,
// brand+en, platform+en.
func (s *Service) resolveTemplate(ctx context.Context, kind string, brandID int64, locale string) (db.DocumentTemplate, error) {
	brand := pgtype.Int8{Int64: brandID, Valid: brandID > 0}
	candidates := []struct {
		brand pgtype.Int8
		lang  string
	}{{brand, locale}, {pgtype.Int8{}, locale}, {brand, model.FallbackLanguage}, {pgtype.Int8{}, model.FallbackLanguage}}
	for _, c := range candidates {
		row, err := s.q.GetActiveDocumentTemplate(ctx, db.GetActiveDocumentTemplateParams{Kind: kind, Language: c.lang, BrandID: c.brand})
		if err == nil {
			return row, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return db.DocumentTemplate{}, err
		}
	}
	return db.DocumentTemplate{}, fmt.Errorf("%w: no active template for kind %s", ErrNotFound, kind)
}

func cacheKey(tpl db.DocumentTemplate, kind, sourceType, sourceID, sourceVersion, locale string, brandID int64, org db.Organization, fonts pdfrender.FontMode) string {
	parts := []string{
		"v1", tpl.ContentHash, strconv.FormatInt(tpl.ID, 10), strconv.Itoa(int(tpl.Version)),
		kind, sourceType, sourceID, sourceVersion, locale, strconv.FormatInt(brandID, 10),
		strconv.FormatInt(org.ID, 10), strconv.FormatInt(org.UpdatedAt.Time.UnixNano(), 10), string(fonts),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func (s *Service) objectPresent(ctx context.Context, row db.DocumentRender) bool {
	if !row.StorageKey.Valid || s.store == nil {
		return false
	}
	ok, err := s.store.Exists(ctx, row.StorageKey.String)
	return err == nil && ok
}

// organizationVars maps the organization letterhead to company variables.
func (s *Service) organizationVars(ctx context.Context, org db.Organization) map[string]string {
	settings, err := s.q.GetAppSettings(ctx)
	if err != nil {
		s.log.Warn("document_app_settings_failed", "error", err)
	}
	lh, err := ioengine.LoadOrganizationLetterhead(ctx, s.store, org, settings)
	if err != nil {
		s.log.Warn("document_letterhead_logo_failed", "error", err)
	}
	return letterheadVars(lh)
}

func letterheadVars(lh ioengine.Letterhead) map[string]string {
	out := map[string]string{
		"company_name":    lh.CompanyName,
		"company_address": lh.Address,
		"company_phone":   lh.Phone,
		"company_email":   lh.Email,
		"company_website": lh.Website,
		"footer_text":     lh.FooterText,
	}
	if logo := pdfrender.ImageTag(lh.LogoMIME, lh.LogoBytes, lh.CompanyName); logo != "" {
		out["company_logo"] = logo
	}
	return out
}

// Download opens a ready document of the viewer's organization. Other
// organizations' documents are reported as not found.
func (s *Service) Download(ctx context.Context, organizationID int64, id uuid.UUID) (io.ReadCloser, string, error) {
	row, err := s.q.GetDocumentRenderByUUID(ctx, id)
	if err != nil {
		return nil, "", notFound(err)
	}
	if row.OrganizationID != organizationID || row.Status != RenderReady || !row.StorageKey.Valid {
		return nil, "", ErrNotFound
	}
	rc, _, err := s.store.Download(ctx, row.StorageKey.String)
	if err != nil {
		return nil, "", err
	}
	return rc, downloadName(row), nil
}

// GetRender returns a render row of the viewer's organization.
func (s *Service) GetRender(ctx context.Context, organizationID int64, id uuid.UUID) (model.RenderView, error) {
	row, err := s.q.GetDocumentRenderByUUID(ctx, id)
	if err != nil {
		return model.RenderView{}, notFound(err)
	}
	if row.OrganizationID != organizationID {
		return model.RenderView{}, ErrNotFound
	}
	return renderView(row), nil
}

// PreviewInput renders editor content (or a stored version) with sample data.
type PreviewInput struct {
	Kind         string
	TemplateUUID *uuid.UUID
	HTML         string
	Locale       string
}

// Preview renders a template with sample values synchronously (editor side
// panel). Nothing is stored.
func (s *Service) Preview(ctx context.Context, in PreviewInput) ([]byte, error) {
	if s.pdf == nil {
		return nil, ErrUnavailable
	}
	var tpl db.DocumentTemplate
	if in.TemplateUUID != nil {
		row, err := s.q.GetDocumentTemplateByUUID(ctx, *in.TemplateUUID)
		if err != nil {
			return nil, notFound(err)
		}
		tpl = row
	} else {
		spec, ok := model.Spec(in.Kind)
		if !ok {
			return nil, fmt.Errorf("%w: unknown kind", ErrInvalidRequest)
		}
		html, _, _, err := prepareHTML(spec, in.HTML)
		if err != nil {
			return nil, err
		}
		lang := model.NormalizeLanguage(in.Locale)
		if lang == "" {
			lang = model.FallbackLanguage
		}
		tpl = db.DocumentTemplate{Kind: in.Kind, Html: html, Language: lang, Name: in.Kind}
	}
	spec, _ := model.Spec(tpl.Kind)
	vars := spec.SampleVars(tpl.Language)
	if settings, err := s.q.GetAppSettings(ctx); err == nil {
		lh, _ := ioengine.LoadLetterheadLogo(ctx, s.store, settings)
		for k, v := range letterheadVars(lh) {
			if strings.TrimSpace(v) != "" {
				vars[k] = v
			}
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.convert(ctx, tpl, vars, tpl.Name, "", "olexfilms.app")
}

func downloadName(row db.DocumentRender) string {
	return fmt.Sprintf("%s-%s.pdf", strings.ReplaceAll(row.Kind, "_", "-"), row.Uuid.String()[:8])
}

func renderView(row db.DocumentRender) model.RenderView {
	v := model.RenderView{
		UUID: row.Uuid, Kind: row.Kind, SourceType: row.SourceType, SourceID: row.SourceID,
		Locale: row.Locale, Status: row.Status, CreatedAt: row.CreatedAt.Time,
	}
	if row.SizeBytes.Valid {
		n := row.SizeBytes.Int64
		v.SizeBytes = &n
	}
	if row.Sha256.Valid {
		h := row.Sha256.String
		v.SHA256 = &h
	}
	if row.Error.Valid {
		e := row.Error.String
		v.Error = &e
	}
	if row.RenderedAt.Valid {
		t := row.RenderedAt.Time
		v.RenderedAt = &t
	}
	if row.Status == RenderReady {
		u := "/v1/tenant/documents/" + row.Uuid.String() + "/download"
		v.DownloadURL = &u
	}
	return v
}
