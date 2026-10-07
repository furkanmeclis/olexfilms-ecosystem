package whatsapp

import (
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp/wuzapi"
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
