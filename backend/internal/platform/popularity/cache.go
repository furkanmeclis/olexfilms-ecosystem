package popularity

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// CacheTTL is how long usage counts are served from the cache (legacy
// MobileLookupCache::popularityTtl, 15 minutes). New usage shows in the
// order after at most this long; nothing invalidates the counts earlier.
const CacheTTL = 15 * time.Minute

// opTimeout bounds one cache call, so a Redis outage slows a request by at
// most this much before the database answers.
const opTimeout = 250 * time.Millisecond

// Cache stores usage counts under Key.String().
type Cache interface {
	Get(ctx context.Context, key string) (Counts, bool)
	Set(ctx context.Context, key string, c Counts)
}

// NoCache always misses (no Redis configured).
type NoCache struct{}

func (NoCache) Get(context.Context, string) (Counts, bool) { return nil, false }
func (NoCache) Set(context.Context, string, Counts)        {}

// RedisCache is the Redis-backed Cache (same pattern as features /
// sysconfig). Redis errors read as a miss, so the database answers.
type RedisCache struct {
	rdb    *redis.Client
	prefix string
	ttl    time.Duration
	onErr  func(op string, err error)
}

// NewRedisCache returns a cache under "<env>:pop:". onErr may be nil.
func NewRedisCache(rdb *redis.Client, env string, onErr func(op string, err error)) *RedisCache {
	if onErr == nil {
		onErr = func(string, error) {}
	}
	prefix := "pop:"
	if env != "" {
		prefix = env + ":pop:"
	}
	return &RedisCache{rdb: rdb, prefix: prefix, ttl: CacheTTL, onErr: onErr}
}

// Get returns cached counts.
func (c *RedisCache) Get(ctx context.Context, key string) (Counts, bool) {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	raw, err := c.rdb.Get(ctx, c.prefix+key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false
	}
	if err != nil {
		c.onErr("get", err)
		return nil, false
	}
	var out Counts
	if err := json.Unmarshal(raw, &out); err != nil {
		c.onErr("decode", err)
		return nil, false
	}
	if out == nil {
		out = Counts{}
	}
	return out, true
}

// Set stores counts for CacheTTL.
func (c *RedisCache) Set(ctx context.Context, key string, counts Counts) {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	if counts == nil {
		counts = Counts{}
	}
	raw, err := json.Marshal(counts)
	if err != nil {
		c.onErr("encode", err)
		return
	}
	if err := c.rdb.Set(ctx, c.prefix+key, raw, c.ttl).Err(); err != nil {
		c.onErr("set", err)
	}
}
