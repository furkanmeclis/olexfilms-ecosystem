// Package guide publishes the Markdown user guides of docs/guides to the
// document center (TEC-510, F5-10c). Each language file
// (<dir>/<slug>.<lang>.md) is rendered Markdown → HTML (goldmark), wrapped in
// the document template skeleton (pdfrender.Document: CSP, RTL, fonts),
// converted by Gotenberg and stored as a new version of the language in one
// library item of the brand center (access all_network). Publishing is
// idempotent: the sha256 of the rendered HTML is part of the version's file
// name, so an unchanged language does not open a new version.
package guide

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	library "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/library/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

// Folder is the document center folder of the user guides.
const Folder = "Kılavuzlar"

// Tag is the tag every published guide item carries.
const Tag = "user-guide"

// Guide describes one guide: its file slug and library item metadata.
type Guide struct {
	Slug        string
	Name        string
	Description string
}

// Guides are the publishable guides, by slug.
var Guides = map[string]Guide{
	"eklentiler": {
		Slug:        "eklentiler",
		Name:        "Eklentiler kılavuzu",
		Description: "Eklenti modülleri: ne yapar, kim açar, nasıl talep edilir, ekran ekran kullanım.",
	},
}

// Converter turns a full HTML document into a PDF (pdfrender.Client).
type Converter interface {
	Convert(ctx context.Context, req pdfrender.Request) ([]byte, error)
}

// Library is the part of the library service the publisher uses.
type Library interface {
	ListFolders(ctx context.Context, actor library.Actor) ([]library.FolderView, error)
	CreateFolder(ctx context.Context, actor library.Actor, in library.FolderInput) (library.FolderView, error)
	ListItems(ctx context.Context, actor library.Actor, in library.ListInput) ([]library.ItemView, int64, error)
	CreateItem(ctx context.Context, actor library.Actor, in library.ItemInput) (library.ItemView, error)
	AddVersion(ctx context.Context, actor library.Actor, itemID uuid.UUID, in library.UploadInput) (library.VersionView, error)
}

// Store reads the brand center and the stored versions of an item.
type Store interface {
	GetBrandCenter(ctx context.Context, brandID int64) (db.Organization, error)
	GetLibraryItemByUUID(ctx context.Context, id uuid.UUID) (db.LibraryItem, error)
	ListLatestLibraryItemVersions(ctx context.Context, itemID int64) ([]db.LibraryItemVersion, error)
}

// FeatureChecker reports whether a module is on for an organization.
type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

// Publisher renders and stores guides.
type Publisher struct {
	q        Store
	library  Library
	pdf      Converter
	features FeatureChecker
	fonts    pdfrender.FontMode
	log      *slog.Logger
}

// New creates a publisher. features may be nil (no module check).
func New(q Store, lib Library, pdf Converter, checker FeatureChecker, fonts pdfrender.FontMode, log *slog.Logger) *Publisher {
	if log == nil {
		log = slog.Default()
	}
	return &Publisher{q: q, library: lib, pdf: pdf, features: checker, fonts: fonts, log: log}
}

// Source is one language file of a guide.
type Source struct {
	Lang     string
	Markdown []byte
}

// LanguageResult reports one language of a publication.
type LanguageResult struct {
	Lang      string `json:"lang"`
	SHA256    string `json:"sha256"`
	Published bool   `json:"published"`
	VersionNo int32  `json:"version_no,omitempty"`
}

// Result reports one publication.
type Result struct {
	BrandID   int64            `json:"brand_id"`
	Skipped   bool             `json:"skipped,omitempty"`
	ItemUUID  uuid.UUID        `json:"item_uuid"`
	Languages []LanguageResult `json:"languages,omitempty"`
}

var fileRE = regexp.MustCompile(`^([a-z0-9-]+)\.([a-z]{2}(?:_[A-Z]{2})?)\.md$`)

// LoadSources reads <dir>/<slug>.<lang>.md files, sorted by language.
func LoadSources(dir, slug string) ([]Source, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("guide: read dir: %w", err)
	}
	var out []Source
	for _, e := range entries {
		m := fileRE.FindStringSubmatch(e.Name())
		if e.IsDir() || m == nil || m[1] != slug {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("guide: read %s: %w", e.Name(), err)
		}
		out = append(out, Source{Lang: m[2], Markdown: body})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("guide: no %s.<lang>.md file in %s", slug, dir)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Lang < out[j].Lang })
	return out, nil
}

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
)

// guideCSS styles the Markdown elements the document skeleton leaves plain.
const guideCSS = `<style>
.guide h1{font-size:20pt;margin:0 0 10pt}
.guide h2{font-size:14pt;margin:18pt 0 6pt;border-block-end:1px solid #d1d5db;padding-block-end:3pt;break-after:avoid}
.guide h3{font-size:11.5pt;margin:12pt 0 4pt;break-after:avoid}
.guide h4{font-size:10.5pt;margin:10pt 0 4pt;break-after:avoid}
.guide ul,.guide ol{margin:0 0 6pt;padding-inline-start:16pt}
.guide li{margin-block-end:2pt}
.guide table{border-collapse:collapse;width:100%;margin-block-end:8pt;font-size:9.5pt}
.guide th,.guide td{border:1px solid #d1d5db;padding:3pt 5pt;text-align:start;vertical-align:top}
.guide th{background:#f3f4f6}
.guide code{font-family:"DejaVu Sans Mono",monospace;font-size:9pt;background:#f3f4f6;padding:0 2pt}
.guide blockquote{margin:6pt 0;padding:6pt 8pt;border-inline-start:3pt solid var(--primary);background:#f9fafb;color:#374151;break-inside:avoid}
.guide blockquote p:last-child{margin:0}
</style>
`

// RenderHTML converts a guide's Markdown to the full HTML document handed to
// Gotenberg. Raw HTML in the Markdown is not rendered (goldmark default) and
// the body passes the template sanitizer like every document template.
func RenderHTML(src Source, fonts pdfrender.FontMode) (string, error) {
	var body bytes.Buffer
	if err := md.Convert(src.Markdown, &body); err != nil {
		return "", fmt.Errorf("guide: markdown %s: %w", src.Lang, err)
	}
	return pdfrender.Document{
		Lang:  src.Lang,
		Title: Title(src.Markdown),
		Body:  pdfrender.SanitizeHTML(guideCSS + `<article class="guide">` + body.String() + `</article>`),
		Fonts: fonts,
	}.HTML(), nil
}

// Title is the first level-one heading of the Markdown.
func Title(markdown []byte) string {
	for _, line := range strings.Split(string(markdown), "\n") {
		if t, ok := strings.CutPrefix(strings.TrimSpace(line), "# "); ok {
			return strings.TrimSpace(t)
		}
	}
	return ""
}

// Hash is the sha256 (hex) of the rendered HTML.
func Hash(html string) string {
	sum := sha256.Sum256([]byte(html))
	return hex.EncodeToString(sum[:])
}

// fileName carries the content hash, the idempotency key of a version.
func fileName(slug, lang, sum string) string {
	return fmt.Sprintf("%s-kilavuzu.%s.%s.pdf", slug, lang, sum[:16])
}

// Publish renders every source and adds a version for each language whose
// content changed since the item's latest version of that language.
func (p *Publisher) Publish(ctx context.Context, brandID int64, g Guide, sources []Source) (Result, error) {
	res := Result{BrandID: brandID}
	center, err := p.q.GetBrandCenter(ctx, brandID)
	if errors.Is(err, pgx.ErrNoRows) {
		res.Skipped = true
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("guide: center: %w", err)
	}
	if p.features != nil {
		on, err := p.features.Enabled(ctx, center.ID, features.ModuleAnnouncements)
		if err != nil {
			return res, fmt.Errorf("guide: feature: %w", err)
		}
		if !on {
			p.log.Warn("guide_publish_skipped_library_off", "brand_id", brandID, "guide", g.Slug)
			res.Skipped = true
			return res, nil
		}
	}
	actor := library.Actor{OrganizationID: center.ID, BrandID: center.BrandID, OrgType: library.OrgCenter}
	item, err := p.item(ctx, actor, g)
	if err != nil {
		return res, err
	}
	res.ItemUUID = item.UUID
	latest, err := p.latestFiles(ctx, item.UUID)
	if err != nil {
		return res, err
	}
	for _, src := range sources {
		html, err := RenderHTML(src, p.fonts)
		if err != nil {
			return res, err
		}
		sum := Hash(html)
		name := fileName(g.Slug, src.Lang, sum)
		lr := LanguageResult{Lang: src.Lang, SHA256: sum}
		if prev, ok := latest[libraryLocale(src.Lang)]; ok && filepath.Base(prev.StorageKey) == name {
			lr.VersionNo = prev.VersionNo
			res.Languages = append(res.Languages, lr)
			continue
		}
		pdf, err := p.pdf.Convert(ctx, pdfrender.Request{HTML: html, FooterHTML: pdfrender.FooterHTML(src.Lang, center.Name)})
		if err != nil {
			return res, fmt.Errorf("guide: render %s: %w", src.Lang, err)
		}
		v, err := p.library.AddVersion(ctx, actor, item.UUID, library.UploadInput{
			Locale: src.Lang, Filename: name, Size: int64(len(pdf)), Body: bytes.NewReader(pdf),
		})
		if err != nil {
			return res, fmt.Errorf("guide: library version %s: %w", src.Lang, err)
		}
		lr.Published, lr.VersionNo = true, v.VersionNo
		res.Languages = append(res.Languages, lr)
	}
	p.log.Info("guide_published", "brand_id", brandID, "guide", g.Slug, "item", item.UUID)
	return res, nil
}

// latestFiles maps library locale → newest version of the item.
func (p *Publisher) latestFiles(ctx context.Context, itemUUID uuid.UUID) (map[string]db.LibraryItemVersion, error) {
	row, err := p.q.GetLibraryItemByUUID(ctx, itemUUID)
	if err != nil {
		return nil, fmt.Errorf("guide: item row: %w", err)
	}
	versions, err := p.q.ListLatestLibraryItemVersions(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("guide: versions: %w", err)
	}
	out := make(map[string]db.LibraryItemVersion, len(versions))
	for _, v := range versions {
		out[v.Locale] = v
	}
	return out, nil
}

// libraryLocale mirrors the library's locale normalization (zh_CN → zh-CN).
func libraryLocale(lang string) string {
	return strings.ReplaceAll(lang, "_", "-")
}

func itemTag(slug string) string { return Tag + ":" + slug }

// item finds (or creates) the guide's library item in the guides folder.
func (p *Publisher) item(ctx context.Context, actor library.Actor, g Guide) (library.ItemView, error) {
	folders, err := p.library.ListFolders(ctx, actor)
	if err != nil {
		return library.ItemView{}, fmt.Errorf("guide: folders: %w", err)
	}
	var folder *library.FolderView
	for i := range folders {
		if folders[i].ParentUUID == nil && strings.EqualFold(folders[i].Name, Folder) {
			folder = &folders[i]
			break
		}
	}
	if folder == nil {
		name := Folder
		f, err := p.library.CreateFolder(ctx, actor, library.FolderInput{Name: &name})
		if err != nil {
			return library.ItemView{}, fmt.Errorf("guide: folder: %w", err)
		}
		folder = &f
	}
	items, _, err := p.library.ListItems(ctx, actor, library.ListInput{FolderUUID: &folder.UUID, Tags: []string{itemTag(g.Slug)}, Limit: 1})
	if err != nil {
		return library.ItemView{}, fmt.Errorf("guide: items: %w", err)
	}
	if len(items) > 0 {
		return items[0], nil
	}
	name, desc, access := g.Name, g.Description, library.AccessAllNetwork
	created, err := p.library.CreateItem(ctx, actor, library.ItemInput{
		FolderUUID: &folder.UUID, Name: &name, Description: &desc, AccessLevel: &access,
		Tags: []string{Tag, itemTag(g.Slug)},
	})
	if err != nil {
		return library.ItemView{}, fmt.Errorf("guide: item: %w", err)
	}
	return created, nil
}
