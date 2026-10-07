package whatsapp

import (
	"context"
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp/wuzapi"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// NewService builds the WhatsApp use case on the wuzapi driver (server and
// worker share it). notifier may be nil.
func NewService(cfg config.WuzapiConfig, pool *pgxpool.Pool, q *db.Queries, box usecase.SecretBox, notifier usecase.Notifier, log *slog.Logger) *usecase.Service {
	gw := wuzapi.New(wuzapi.Config{
		BaseURL:    cfg.URL,
		AdminToken: cfg.AdminToken,
		HMACKey:    cfg.WebhookSecret,
		WebhookURL: cfg.WebhookURL,
	})
	var tx usecase.TxBeginner
	if pool != nil {
		tx = pool
	}
	return usecase.New(q, tx, gw, box, outbox.NewStore(pool, q), notifier, log)
}

// NewIdentityCache is the WhatsApp identity cache on Redis (no Redis: no
// cache).
func NewIdentityCache(rdb *redis.Client, env string, log *slog.Logger) usecase.IdentityCache {
	if rdb == nil {
		return usecase.NoIdentityCache{}
	}
	return usecase.NewRedisIdentityCache(rdb, env, log)
}

// RegisterIdentityInvalidation drops the identity cache on user, membership
// and organization events (TEC-394). It runs where the outbox is published.
func RegisterIdentityInvalidation(bus events.Bus, rdb *redis.Client, env string, log *slog.Logger) {
	usecase.RegisterIdentityInvalidation(bus, NewIdentityCache(rdb, env, log))
}

// MessagingDeps are the outside services of conversation messaging; any
// may be nil.
type MessagingDeps struct {
	Storage       usecase.ObjectStore
	Queue         usecase.SendQueue
	Limiter       usecase.SendLimiter
	SendPerMinute func(ctx context.Context) int
	Publisher     realtime.Publisher
}

// NewMessaging builds the conversation messaging use case (TEC-395) on the
// service's gateway and attaches it to the webhook.
func NewMessaging(svc *usecase.Service, pool *pgxpool.Pool, q *db.Queries, d MessagingDeps, log *slog.Logger) *usecase.Messaging {
	var tx usecase.TxBeginner
	if pool != nil {
		tx = pool
	}
	m := usecase.NewMessaging(usecase.MessagingDeps{
		Queries: q, Tx: tx, Provider: svc.Provider(), Downloader: svc.MediaDownloader(),
		Storage: d.Storage, Queue: d.Queue, Limiter: d.Limiter, SendPerMinute: d.SendPerMinute,
		Publisher: d.Publisher, Log: log,
	})
	svc.AttachMessaging(m)
	return m
}

// NewInbox builds the panel conversation use case (TEC-398) on the
// messaging queue.
func NewInbox(m *usecase.Messaging, pool *pgxpool.Pool, q *db.Queries, storage usecase.ObjectStore, pub realtime.Publisher) *usecase.Inbox {
	var tx usecase.TxBeginner
	if pool != nil {
		tx = pool
	}
	return usecase.NewInbox(usecase.InboxDeps{
		Queries: q, Tx: tx, Messaging: m, Storage: storage, Outbox: outbox.NewStore(pool, q), Publisher: pub,
	})
}
