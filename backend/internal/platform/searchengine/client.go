package searchengine

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/meilisearch/meilisearch-go"
)

// Client wraps Meilisearch index operations.
type Client struct {
	cfg    config.SearchConfig
	client meilisearch.ServiceManager
	log    *slog.Logger
}

// NewClient builds a Meilisearch client. Returns nil when search is disabled.
func NewClient(cfg config.SearchConfig, log *slog.Logger) *Client {
	if !cfg.Enabled || cfg.Driver != "meilisearch" {
		return nil
	}
	if log == nil {
		log = slog.Default()
	}
	return &Client{
		cfg:    cfg,
		client: meilisearch.New(cfg.MeiliHost, meilisearch.WithAPIKey(cfg.MeiliKey)),
		log:    log,
	}
}

// Enabled reports whether search indexing is active.
func (c *Client) Enabled() bool {
	return c != nil && c.client != nil
}

// Ping checks Meilisearch health.
func (c *Client) Ping(ctx context.Context) error {
	if !c.Enabled() {
		return nil
	}
	_, err := c.client.HealthWithContext(ctx)
	if err != nil {
		return fmt.Errorf("searchengine: meili health: %w", err)
	}
	return nil
}

func (c *Client) indexUID(spec string) string {
	return fmt.Sprintf("%s_%s", c.cfg.IndexPrefix, spec)
}

// EnsureIndex creates or updates index settings for a spec.
func (c *Client) EnsureIndex(ctx context.Context, spec Spec) error {
	if !c.Enabled() {
		return nil
	}
	uid := c.indexUID(spec.ID)
	task, err := c.client.CreateIndexWithContext(ctx, &meilisearch.IndexConfig{Uid: uid, PrimaryKey: "id"})
	if err != nil {
		return fmt.Errorf("searchengine: create index %q: %w", uid, err)
	}
	if _, err := c.client.WaitForTaskWithContext(ctx, task.TaskUID, 0); err != nil {
		return fmt.Errorf("searchengine: wait create index %q: %w", uid, err)
	}

	searchable := spec.Searchable
	if len(searchable) == 0 {
		searchable = []string{"title", "subtitle", "keywords"}
	}
	settings := &meilisearch.Settings{
		SearchableAttributes: searchable,
		DisplayedAttributes:  []string{"id", "spec", "title", "subtitle", "keywords", "href", "icon", "organization_slug"},
		FilterableAttributes: append([]string{"spec"}, spec.Filterable...),
	}
	task, err = c.client.Index(uid).UpdateSettingsWithContext(ctx, settings)
	if err != nil {
		return fmt.Errorf("searchengine: update settings %q: %w", uid, err)
	}
	if _, err := c.client.WaitForTaskWithContext(ctx, task.TaskUID, 0); err != nil {
		return fmt.Errorf("searchengine: wait settings %q: %w", uid, err)
	}
	return nil
}

// UpsertDocuments adds or replaces documents in a spec index.
func (c *Client) UpsertDocuments(ctx context.Context, spec string, docs []Document) error {
	if !c.Enabled() || len(docs) == 0 {
		return nil
	}
	uid := c.indexUID(spec)
	task, err := c.client.Index(uid).AddDocumentsWithContext(ctx, docs)
	if err != nil {
		return fmt.Errorf("searchengine: upsert %q: %w", uid, err)
	}
	if _, err := c.client.WaitForTaskWithContext(ctx, task.TaskUID, 0); err != nil {
		return fmt.Errorf("searchengine: wait upsert %q: %w", uid, err)
	}
	return c.ensureTaskSucceeded(ctx, task.TaskUID, "upsert", uid)
}

// DeleteDocument removes one document from a spec index.
func (c *Client) DeleteDocument(ctx context.Context, spec, id string) error {
	if !c.Enabled() || strings.TrimSpace(id) == "" {
		return nil
	}
	uid := c.indexUID(spec)
	task, err := c.client.Index(uid).DeleteDocumentWithContext(ctx, id)
	if err != nil {
		return fmt.Errorf("searchengine: delete %q/%s: %w", uid, id, err)
	}
	if _, err := c.client.WaitForTaskWithContext(ctx, task.TaskUID, 0); err != nil {
		return fmt.Errorf("searchengine: wait delete %q/%s: %w", uid, id, err)
	}
	return c.ensureTaskSucceeded(ctx, task.TaskUID, "delete", uid)
}

func (c *Client) ensureTaskSucceeded(ctx context.Context, taskUID int64, op, uid string) error {
	task, err := c.client.GetTaskWithContext(ctx, taskUID)
	if err != nil {
		return fmt.Errorf("searchengine: %s task %q status: %w", op, uid, err)
	}
	if task.Status == "failed" {
		msg := task.Error.Message
		if msg == "" {
			msg = "unknown meilisearch task failure"
		}
		return fmt.Errorf("searchengine: %s %q: %s", op, uid, msg)
	}
	return nil
}

// Search queries one or more spec indexes. filters maps spec id to Meilisearch filter expressions.
func (c *Client) Search(ctx context.Context, specs []string, q string, limit int, filters map[string]string) ([]Hit, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	if filters == nil {
		filters = map[string]string{}
	}

	if len(specs) == 1 {
		return c.searchOne(ctx, specs[0], q, limit, filters[specs[0]])
	}

	queries := make([]*meilisearch.SearchRequest, 0, len(specs))
	for _, spec := range specs {
		req := &meilisearch.SearchRequest{
			IndexUID: c.indexUID(spec),
			Query:    q,
			Limit:    int64(limit),
		}
		if filter := strings.TrimSpace(filters[spec]); filter != "" {
			req.Filter = filter
		}
		queries = append(queries, req)
	}
	resp, err := c.client.MultiSearchWithContext(ctx, &meilisearch.MultiSearchRequest{Queries: queries})
	if err != nil {
		return nil, fmt.Errorf("searchengine: multi search: %w", err)
	}

	out := make([]Hit, 0, limit)
	for i, result := range resp.Results {
		spec := specs[i]
		for _, hit := range result.Hits {
			out = append(out, mapHit(spec, hit))
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

func (c *Client) searchOne(ctx context.Context, spec, q string, limit int, filter string) ([]Hit, error) {
	req := &meilisearch.SearchRequest{
		Limit: int64(limit),
	}
	if filter = strings.TrimSpace(filter); filter != "" {
		req.Filter = filter
	}
	resp, err := c.client.Index(c.indexUID(spec)).SearchWithContext(ctx, q, req)
	if err != nil {
		return nil, fmt.Errorf("searchengine: search %q: %w", spec, err)
	}
	out := make([]Hit, 0, len(resp.Hits))
	for _, hit := range resp.Hits {
		out = append(out, mapHit(spec, hit))
	}
	return out, nil
}

func mapHit(spec string, hit interface{}) Hit {
	row, ok := hit.(map[string]interface{})
	if !ok {
		return Hit{Spec: spec}
	}
	getStr := func(key string) string {
		v, ok := row[key]
		if !ok || v == nil {
			return ""
		}
		switch t := v.(type) {
		case string:
			return t
		default:
			return fmt.Sprint(t)
		}
	}
	return Hit{
		Spec:     spec,
		ID:       entityIDFromDoc(getStr("id")),
		Title:    getStr("title"),
		Subtitle: getStr("subtitle"),
		Href:     getStr("href"),
		Icon:     getStr("icon"),
	}
}

func entityIDFromDoc(docID string) string {
	if i := strings.Index(docID, "_"); i >= 0 && i+1 < len(docID) {
		return docID[i+1:]
	}
	return docID
}

// Stats returns document count for a spec index (0 when missing).
func (c *Client) Stats(ctx context.Context, spec string) (int64, error) {
	if !c.Enabled() {
		return 0, nil
	}
	stats, err := c.client.Index(c.indexUID(spec)).GetStatsWithContext(ctx)
	if err != nil {
		return 0, err
	}
	return stats.NumberOfDocuments, nil
}
