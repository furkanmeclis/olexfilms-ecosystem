// Package usecase implements PDF document templates (center admin editor)
// and the render pipeline (source → template → Gotenberg → storage).
package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrInvalidRequest = errors.New("invalid request")
	ErrConflict       = errors.New("conflict")
	ErrUnavailable    = errors.New("pdf renderer unavailable")
)

// UnknownVariablesError lists placeholders the kind does not define.
type UnknownVariablesError struct{ Keys []string }

func (e *UnknownVariablesError) Error() string {
	return "unknown template variables: " + strings.Join(e.Keys, ", ")
}

// MaxTemplateBytes bounds stored template HTML (editor output).
const MaxTemplateBytes = 512 * 1024

// Renderer converts HTML to PDF (satisfied by *pdfrender.Client).
type Renderer interface {
	Convert(ctx context.Context, req pdfrender.Request) ([]byte, error)
}

// Enqueuer schedules background tasks (satisfied by *queue.Client).
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Service is the documents use case.
type Service struct {
	pool    *pgxpool.Pool
	q       *db.Queries
	store   storage.Driver
	pdf     Renderer
	queue   Enqueuer
	fonts   pdfrender.FontMode
	log     *slog.Logger
	mu      sync.RWMutex
	loaders map[string]model.SourceLoader
}

// New builds the service. enq nil renders synchronously (dev, tests).
func New(pool *pgxpool.Pool, q *db.Queries, store storage.Driver, pdf Renderer, enq Enqueuer, fonts pdfrender.FontMode, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	if fonts == "" {
		fonts = pdfrender.FontsEmbedded
	}
	return &Service{
		pool: pool, q: q, store: store, pdf: pdf, queue: enq, fonts: fonts, log: log,
		loaders: map[string]model.SourceLoader{},
	}
}

// RegisterLoader binds a source loader to a document kind (one per kind).
func (s *Service) RegisterLoader(kind string, l model.SourceLoader) error {
	if _, ok := model.Spec(kind); !ok {
		return fmt.Errorf("documents: unknown kind %q", kind)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loaders[kind] = l
	return nil
}

func (s *Service) loader(kind string) (model.SourceLoader, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.loaders[kind]
	return l, ok
}

// Kinds returns every kind with its variable schema and whether a source
// loader is registered (F1 modules register theirs).
func (s *Service) Kinds() []KindInfo {
	specs := model.AllSpecs()
	out := make([]KindInfo, 0, len(specs))
	for _, sp := range specs {
		_, ok := s.loader(sp.Kind)
		out = append(out, KindInfo{KindSpec: sp, HasSource: ok})
	}
	return out
}

// KindInfo is a kind spec plus loader availability.
type KindInfo struct {
	model.KindSpec
	HasSource bool `json:"has_source"`
}

// ListFilter filters template listings.
type ListFilter struct {
	Kind        string
	Language    string
	CurrentOnly bool
	Limit       int32
	Offset      int32
}

// ListTemplates lists template versions (without HTML bodies).
func (s *Service) ListTemplates(ctx context.Context, f ListFilter) ([]model.TemplateView, int64, error) {
	kind := pgtype.Text{String: f.Kind, Valid: f.Kind != ""}
	lang := pgtype.Text{String: f.Language, Valid: f.Language != ""}
	rows, err := s.q.ListDocumentTemplates(ctx, db.ListDocumentTemplatesParams{
		Kind: kind, Language: lang, CurrentOnly: f.CurrentOnly, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountDocumentTemplates(ctx, db.CountDocumentTemplatesParams{Kind: kind, Language: lang, CurrentOnly: f.CurrentOnly})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.TemplateView, 0, len(rows))
	for _, r := range rows {
		v := templateView(db.DocumentTemplate{
			ID: r.ID, Uuid: r.Uuid, Kind: r.Kind, BrandID: r.BrandID, Language: r.Language, Name: r.Name,
			Version: r.Version, IsActive: r.IsActive, Variables: r.Variables, ContentHash: r.ContentHash,
			PublishedAt: r.PublishedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}, false)
		if r.BrandSlug.Valid {
			slug := r.BrandSlug.String
			v.BrandSlug = &slug
		}
		out = append(out, v)
	}
	return out, total, nil
}

// GetTemplate returns one template version with its HTML and editor state.
func (s *Service) GetTemplate(ctx context.Context, id uuid.UUID) (model.TemplateView, error) {
	row, err := s.q.GetDocumentTemplateByUUID(ctx, id)
	if err != nil {
		return model.TemplateView{}, notFound(err)
	}
	return s.view(ctx, row, true), nil
}

// Versions lists every version of the template's (kind, brand, language).
func (s *Service) Versions(ctx context.Context, id uuid.UUID) ([]model.TemplateView, error) {
	row, err := s.q.GetDocumentTemplateByUUID(ctx, id)
	if err != nil {
		return nil, notFound(err)
	}
	rows, err := s.q.ListDocumentTemplateVersions(ctx, db.ListDocumentTemplateVersionsParams{
		Kind: row.Kind, Language: row.Language, BrandID: row.BrandID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.TemplateView, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.view(ctx, r, false))
	}
	return out, nil
}

// SaveInput is the editor payload.
type SaveInput struct {
	Kind        string
	BrandSlug   string
	Language    string
	Name        string
	HTML        string
	LexicalJSON json.RawMessage
}

// SaveDraft stores the editor content as the open draft of (kind, brand,
// language): the existing draft is updated, otherwise a new version opens.
// HTML is sanitized server-side; unknown placeholders are rejected.
func (s *Service) SaveDraft(ctx context.Context, actorID int64, in SaveInput) (model.TemplateView, error) {
	spec, ok := model.Spec(in.Kind)
	if !ok {
		return model.TemplateView{}, fmt.Errorf("%w: unknown kind", ErrInvalidRequest)
	}
	lang := model.NormalizeLanguage(in.Language)
	if lang == "" {
		return model.TemplateView{}, fmt.Errorf("%w: unsupported language", ErrInvalidRequest)
	}
	brandID, err := s.brandID(ctx, in.BrandSlug)
	if err != nil {
		return model.TemplateView{}, err
	}
	html, vars, hash, err := prepareHTML(spec, in.HTML)
	if err != nil {
		return model.TemplateView{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 150 {
		return model.TemplateView{}, fmt.Errorf("%w: name is required (max 150)", ErrInvalidRequest)
	}
	lexical, err := lexicalBytes(in.LexicalJSON)
	if err != nil {
		return model.TemplateView{}, err
	}
	draft, err := s.q.GetDraftDocumentTemplate(ctx, db.GetDraftDocumentTemplateParams{Kind: in.Kind, Language: lang, BrandID: brandID})
	if err == nil {
		row, err := s.q.UpdateDocumentTemplateDraft(ctx, db.UpdateDocumentTemplateDraftParams{
			ID: draft.ID, Name: name, LexicalJson: lexical, Html: html, Variables: vars, ContentHash: hash,
		})
		if err != nil {
			return model.TemplateView{}, err
		}
		return s.view(ctx, row, true), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.TemplateView{}, err
	}
	next, err := s.q.NextDocumentTemplateVersion(ctx, db.NextDocumentTemplateVersionParams{Kind: in.Kind, Language: lang, BrandID: brandID})
	if err != nil {
		return model.TemplateView{}, err
	}
	row, err := s.q.CreateDocumentTemplate(ctx, db.CreateDocumentTemplateParams{
		Kind: in.Kind, BrandID: brandID, Language: lang, Name: name, Version: next,
		LexicalJson: lexical, Html: html, Variables: vars, ContentHash: hash,
		CreatedBy: pgtype.Int8{Int64: actorID, Valid: actorID > 0},
	})
	if err != nil {
		return model.TemplateView{}, err
	}
	return s.view(ctx, row, true), nil
}

// UpdateDraft edits a draft version in place; published versions are
// immutable (ErrConflict).
func (s *Service) UpdateDraft(ctx context.Context, id uuid.UUID, in SaveInput) (model.TemplateView, error) {
	row, err := s.q.GetDocumentTemplateByUUID(ctx, id)
	if err != nil {
		return model.TemplateView{}, notFound(err)
	}
	if row.PublishedAt.Valid {
		return model.TemplateView{}, fmt.Errorf("%w: published versions are immutable; save a new draft", ErrConflict)
	}
	spec, _ := model.Spec(row.Kind)
	html, vars, hash, err := prepareHTML(spec, in.HTML)
	if err != nil {
		return model.TemplateView{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = row.Name
	}
	lexical, err := lexicalBytes(in.LexicalJSON)
	if err != nil {
		return model.TemplateView{}, err
	}
	updated, err := s.q.UpdateDocumentTemplateDraft(ctx, db.UpdateDocumentTemplateDraftParams{
		ID: row.ID, Name: name, LexicalJson: lexical, Html: html, Variables: vars, ContentHash: hash,
	})
	if err != nil {
		return model.TemplateView{}, notFound(err)
	}
	return s.view(ctx, updated, true), nil
}

// Publish activates a draft and retires the previous active version in one
// transaction. Cached PDFs of the old version stop matching: the cache key
// contains the template hash and version.
func (s *Service) Publish(ctx context.Context, id uuid.UUID) (model.TemplateView, error) {
	row, err := s.q.GetDocumentTemplateByUUID(ctx, id)
	if err != nil {
		return model.TemplateView{}, notFound(err)
	}
	if row.PublishedAt.Valid {
		return model.TemplateView{}, fmt.Errorf("%w: version is already published", ErrConflict)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.TemplateView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)
	if err := qtx.DeactivateDocumentTemplates(ctx, db.DeactivateDocumentTemplatesParams{
		Kind: row.Kind, Language: row.Language, BrandID: row.BrandID,
	}); err != nil {
		return model.TemplateView{}, err
	}
	published, err := qtx.PublishDocumentTemplate(ctx, row.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.TemplateView{}, fmt.Errorf("%w: version is already published", ErrConflict)
		}
		return model.TemplateView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.TemplateView{}, err
	}
	return s.view(ctx, published, true), nil
}

func prepareHTML(spec model.KindSpec, raw string) (html string, vars []byte, hash string, err error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil, "", fmt.Errorf("%w: html is required", ErrInvalidRequest)
	}
	if len(raw) > MaxTemplateBytes {
		return "", nil, "", fmt.Errorf("%w: html exceeds %d bytes", ErrInvalidRequest, MaxTemplateBytes)
	}
	html = pdfrender.SanitizeHTML(raw)
	if unknown := spec.Unknown(html); len(unknown) > 0 {
		return "", nil, "", &UnknownVariablesError{Keys: unknown}
	}
	used := msgtemplate.Placeholders(html)
	vars, err = json.Marshal(used)
	if err != nil {
		return "", nil, "", err
	}
	sum := sha256.Sum256([]byte(html))
	return html, vars, hex.EncodeToString(sum[:]), nil
}

func lexicalBytes(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("%w: lexical_json is not valid JSON", ErrInvalidRequest)
	}
	if len(raw) > 4*MaxTemplateBytes {
		return nil, fmt.Errorf("%w: lexical_json is too large", ErrInvalidRequest)
	}
	return raw, nil
}

func (s *Service) brandID(ctx context.Context, slug string) (pgtype.Int8, error) {
	slug = strings.TrimSpace(strings.ToLower(slug))
	if slug == "" {
		return pgtype.Int8{}, nil
	}
	b, err := s.q.GetBrandBySlug(ctx, slug)
	if err != nil {
		return pgtype.Int8{}, fmt.Errorf("%w: unknown brand", ErrInvalidRequest)
	}
	return pgtype.Int8{Int64: b.ID, Valid: true}, nil
}

func (s *Service) view(ctx context.Context, row db.DocumentTemplate, full bool) model.TemplateView {
	v := templateView(row, full)
	if row.BrandID.Valid {
		if b, err := s.q.GetBrandByID(ctx, row.BrandID.Int64); err == nil {
			id, slug := b.Uuid, b.Slug
			v.BrandUUID, v.BrandSlug = &id, &slug
		}
	}
	return v
}

func templateView(row db.DocumentTemplate, full bool) model.TemplateView {
	v := model.TemplateView{
		UUID: row.Uuid, Kind: row.Kind, Language: row.Language, Name: row.Name, Version: row.Version,
		IsActive: row.IsActive, ContentHash: row.ContentHash, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		Variables: []string{},
	}
	_ = json.Unmarshal(row.Variables, &v.Variables)
	switch {
	case !row.PublishedAt.Valid:
		v.Status = model.StatusDraft
	case row.IsActive:
		v.Status = model.StatusActive
	default:
		v.Status = model.StatusSuperseded
	}
	if row.PublishedAt.Valid {
		t := row.PublishedAt.Time
		v.PublishedAt = &t
	}
	if full {
		html := row.Html
		v.HTML = &html
		if len(row.LexicalJson) > 0 {
			v.LexicalJSON = json.RawMessage(row.LexicalJson)
		}
	}
	return v
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
