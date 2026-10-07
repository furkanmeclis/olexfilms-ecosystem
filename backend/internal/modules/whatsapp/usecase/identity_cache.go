package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/redis/go-redis/v9"
)

// IdentityCacheTTL bounds how long a resolved number is served after a
// change that did not invalidate it (F4-02b: Redis 10 min).
const IdentityCacheTTL = 10 * time.Minute

// identityCacheOpTimeout bounds one cache call; a Redis outage falls back to
// the database.
const identityCacheOpTimeout = 250 * time.Millisecond

// IdentityCache stores identity snapshots per E.164 number. Keys are
// waid:v{gen}:{e164}; Invalidate bumps the generation, so every snapshot is
// dropped at once (user / membership changes are rare and a user id does
// not map back to its number without a query).
type IdentityCache interface {
	Get(ctx context.Context, e164 string) (IdentitySnapshot, bool)
	Set(ctx context.Context, e164 string, snap IdentitySnapshot)
	Invalidate(ctx context.Context)
}

// NoIdentityCache always misses (no Redis configured).
type NoIdentityCache struct{}

func (NoIdentityCache) Get(context.Context, string) (IdentitySnapshot, bool) {
	return IdentitySnapshot{}, false
}
func (NoIdentityCache) Set(context.Context, string, IdentitySnapshot) {}
func (NoIdentityCache) Invalidate(context.Context)                    {}

// RedisIdentityCache is the Redis-backed IdentityCache. Redis errors read as
// a miss.
type RedisIdentityCache struct {
	rdb    *redis.Client
	prefix string
	ttl    time.Duration
	log    *slog.Logger
}

// NewRedisIdentityCache returns a cache under "<env>:waid:". log may be nil.
func NewRedisIdentityCache(rdb *redis.Client, env string, log *slog.Logger) *RedisIdentityCache {
	if log == nil {
		log = slog.Default()
	}
	prefix := "waid:"
	if env != "" {
		prefix = env + ":waid:"
	}
	return &RedisIdentityCache{rdb: rdb, prefix: prefix, ttl: IdentityCacheTTL, log: log}
}

func (c *RedisIdentityCache) onErr(op string, err error) {
	c.log.Warn("whatsapp_identity_cache_error", "op", op, "error", err)
}

func (c *RedisIdentityCache) genKey() string { return c.prefix + "gen" }

func (c *RedisIdentityCache) key(ctx context.Context, e164 string) (string, error) {
	gen := int64(0)
	v, err := c.rdb.Get(ctx, c.genKey()).Result()
	switch {
	case errors.Is(err, redis.Nil):
	case err != nil:
		return "", err
	default:
		if gen, err = strconv.ParseInt(v, 10, 64); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%sv%d:%s", c.prefix, gen, e164), nil
}

// Get returns a cached snapshot.
func (c *RedisIdentityCache) Get(ctx context.Context, e164 string) (IdentitySnapshot, bool) {
	ctx, cancel := context.WithTimeout(ctx, identityCacheOpTimeout)
	defer cancel()
	key, err := c.key(ctx, e164)
	if err != nil {
		c.onErr("gen", err)
		return IdentitySnapshot{}, false
	}
	raw, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return IdentitySnapshot{}, false
	}
	if err != nil {
		c.onErr("get", err)
		return IdentitySnapshot{}, false
	}
	var snap IdentitySnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		c.onErr("decode", err)
		return IdentitySnapshot{}, false
	}
	return snap, true
}

// Set stores a snapshot for IdentityCacheTTL.
func (c *RedisIdentityCache) Set(ctx context.Context, e164 string, snap IdentitySnapshot) {
	ctx, cancel := context.WithTimeout(ctx, identityCacheOpTimeout)
	defer cancel()
	key, err := c.key(ctx, e164)
	if err != nil {
		c.onErr("gen", err)
		return
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		c.onErr("encode", err)
		return
	}
	if err := c.rdb.Set(ctx, key, raw, c.ttl).Err(); err != nil {
		c.onErr("set", err)
	}
}

// Invalidate drops every snapshot.
func (c *RedisIdentityCache) Invalidate(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, identityCacheOpTimeout)
	defer cancel()
	if err := c.rdb.Incr(ctx, c.genKey()).Err(); err != nil {
		c.onErr("incr", err)
	}
}

// IdentityInvalidationEvents are the user / membership / organization
// changes that drop the identity cache: a new membership, a customer created
// or merged, an organization created or edited (status, access window).
var IdentityInvalidationEvents = []string{
	events.TenantMemberAdded,
	events.CustomerCreated,
	events.CustomerMerged,
	"organization.*",
}

// RegisterIdentityInvalidation subscribes the cache to
// IdentityInvalidationEvents.
func RegisterIdentityInvalidation(bus events.Bus, cache IdentityCache) {
	if bus == nil || cache == nil {
		return
	}
	for _, name := range IdentityInvalidationEvents {
		bus.Subscribe(name, func(ctx context.Context, _ events.Event) error {
			cache.Invalidate(ctx)
			return nil
		})
	}
}
