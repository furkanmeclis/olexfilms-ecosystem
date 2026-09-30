package searchengine

import (
	"context"
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
)

// Indexer enqueues search index mutations (fail-soft).
type Indexer struct {
	client *Client
	reg    *Registry
	queue  *queue.Client
	log    *slog.Logger
}

// NewIndexer builds an indexer. queue may be nil (sync no-op enqueue).
func NewIndexer(client *Client, reg *Registry, q *queue.Client, log *slog.Logger) *Indexer {
	if log == nil {
		log = slog.Default()
	}
	return &Indexer{client: client, reg: reg, queue: q, log: log}
}

// Enabled reports whether indexing is configured.
func (i *Indexer) Enabled() bool {
	return i != nil && i.client != nil && i.client.Enabled()
}

// EnqueueUpsert schedules indexing one entity.
func (i *Indexer) EnqueueUpsert(ctx context.Context, spec, id string) {
	if i == nil || !i.Enabled() || spec == "" || id == "" {
		return
	}
	if i.queue == nil {
		i.processUpsert(ctx, spec, id)
		return
	}
	task, err := queue.NewSearchUpsertTask(spec, id)
	if err != nil {
		i.log.Warn("search_enqueue_upsert_failed", "spec", spec, "id", id, "error", err)
		return
	}
	if _, err := i.queue.Enqueue(task, asynq.Queue(queue.QueueSearch)); err != nil {
		i.log.Warn("search_enqueue_upsert_failed", "spec", spec, "id", id, "error", err)
	}
}

// EnqueueDelete schedules removing one entity from the index.
func (i *Indexer) EnqueueDelete(ctx context.Context, spec, id string) {
	if i == nil || !i.Enabled() || spec == "" || id == "" {
		return
	}
	if i.queue == nil {
		i.processDelete(ctx, spec, id)
		return
	}
	task, err := queue.NewSearchDeleteTask(spec, id)
	if err != nil {
		i.log.Warn("search_enqueue_delete_failed", "spec", spec, "id", id, "error", err)
		return
	}
	if _, err := i.queue.Enqueue(task, asynq.Queue(queue.QueueSearch)); err != nil {
		i.log.Warn("search_enqueue_delete_failed", "spec", spec, "id", id, "error", err)
	}
}

// EnqueueReindex schedules a full reindex. Empty spec reindexes all adapters.
func (i *Indexer) EnqueueReindex(ctx context.Context, spec string) {
	if i == nil || !i.Enabled() {
		return
	}
	if i.queue == nil {
		i.processReindex(ctx, spec)
		return
	}
	task, err := queue.NewSearchReindexTask(spec)
	if err != nil {
		i.log.Warn("search_enqueue_reindex_failed", "spec", spec, "error", err)
		return
	}
	if _, err := i.queue.Enqueue(task, asynq.Queue(queue.QueueSearch)); err != nil {
		i.log.Warn("search_enqueue_reindex_failed", "spec", spec, "error", err)
	}
}

// ProcessUpsert indexes one document (worker handler).
func (i *Indexer) ProcessUpsert(ctx context.Context, spec, id string) error {
	i.processUpsert(ctx, spec, id)
	return nil
}

// ProcessDelete removes one document (worker handler).
func (i *Indexer) ProcessDelete(ctx context.Context, spec, id string) error {
	i.processDelete(ctx, spec, id)
	return nil
}

// ProcessReindex rebuilds one or all indexes (worker handler).
func (i *Indexer) ProcessReindex(ctx context.Context, spec string) error {
	i.processReindex(ctx, spec)
	return nil
}

// Bootstrap ensures indexes exist and triggers reindex when empty.
func (i *Indexer) Bootstrap(ctx context.Context) {
	if i == nil || !i.Enabled() || i.reg == nil {
		return
	}
	for _, spec := range i.reg.Specs() {
		if err := i.client.EnsureIndex(ctx, spec); err != nil {
			i.log.Warn("search_bootstrap_ensure_failed", "spec", spec.ID, "error", err)
			continue
		}
		count, err := i.client.Stats(ctx, spec.ID)
		if err != nil {
			i.log.Warn("search_bootstrap_stats_failed", "spec", spec.ID, "error", err)
			continue
		}
		if count == 0 {
			i.log.Info("search_bootstrap_reindex", "spec", spec.ID)
			i.EnqueueReindex(ctx, spec.ID)
		}
	}
}

func (i *Indexer) processUpsert(ctx context.Context, spec, id string) {
	if i.reg == nil {
		return
	}
	adapter, err := i.reg.Get(spec)
	if err != nil {
		i.log.Warn("search_upsert_unknown_spec", "spec", spec, "error", err)
		return
	}
	doc, err := adapter.Document(ctx, id)
	if err != nil {
		i.log.Warn("search_upsert_document_failed", "spec", spec, "id", id, "error", err)
		return
	}
	if err := i.client.EnsureIndex(ctx, adapter.Spec()); err != nil {
		i.log.Warn("search_upsert_ensure_failed", "spec", spec, "error", err)
		return
	}
	if err := i.client.UpsertDocuments(ctx, spec, []Document{doc}); err != nil {
		i.log.Warn("search_upsert_failed", "spec", spec, "id", id, "error", err)
	}
}

func (i *Indexer) processDelete(ctx context.Context, spec, id string) {
	if err := i.client.DeleteDocument(ctx, spec, id); err != nil {
		i.log.Warn("search_delete_failed", "spec", spec, "id", id, "error", err)
	}
}

func (i *Indexer) processReindex(ctx context.Context, spec string) {
	if i.reg == nil {
		return
	}
	specs := i.reg.Specs()
	if spec != "" {
		adapter, err := i.reg.Get(spec)
		if err != nil {
			i.log.Warn("search_reindex_unknown_spec", "spec", spec, "error", err)
			return
		}
		specs = []Spec{adapter.Spec()}
	}
	for _, s := range specs {
		adapter, err := i.reg.Get(s.ID)
		if err != nil {
			i.log.Warn("search_reindex_adapter_failed", "spec", s.ID, "error", err)
			continue
		}
		if err := i.client.EnsureIndex(ctx, s); err != nil {
			i.log.Warn("search_reindex_ensure_failed", "spec", s.ID, "error", err)
			continue
		}
		docs, err := adapter.ListAll(ctx)
		if err != nil {
			i.log.Warn("search_reindex_list_failed", "spec", s.ID, "error", err)
			continue
		}
		const batch = 200
		for start := 0; start < len(docs); start += batch {
			end := start + batch
			if end > len(docs) {
				end = len(docs)
			}
			if err := i.client.UpsertDocuments(ctx, s.ID, docs[start:end]); err != nil {
				i.log.Warn("search_reindex_batch_failed", "spec", s.ID, "error", err)
				break
			}
		}
		i.log.Info("search_reindex_completed", "spec", s.ID, "count", len(docs))
	}
}
