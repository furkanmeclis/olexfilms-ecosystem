package sysconfig

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// CacheTTL bounds how long a snapshot is served without a database read
// (design §6: 30 s Redis cache). Writes drop the snapshot at once.
const CacheTTL = 30 * time.Second

// opTimeout bounds one Redis call so an outage costs a request at most this
// much before the database answers.
const opTimeout = 250 * time.Millisecond

// Snapshot is every stored override, keyed by setting key. Keys without a
// row are absent and resolve to their catalog default.
type Snapshot map[string]json.RawMessage

// Cache stores the override snapshot.
type Cache interface {
	Get(ctx context.Context) (Snapshot, bool)
	Set(ctx context.Context, snap Snapshot)
	Invalidate(ctx context.Context)
}

// NoCache always misses (no Redis configured).
type NoCache struct{}

func (NoCache) Get(context.Context) (Snapshot, bool) { return nil, false }
func (NoCache) Set(context.Context, Snapshot)        {}
func (NoCache) Invalidate(context.Context)           {}

// RedisCache keeps the snapshot under one key. Redis errors read as a miss.
type RedisCache struct {
	rdb   *redis.Client
	key   string
	ttl   time.Duration
	onErr func(op string, err error)
}

// NewRedisCache returns a cache under "<env>:sysconfig:snapshot". onErr may
// be nil.
func NewRedisCache(rdb *redis.Client, env string, onErr func(op string, err error)) *RedisCache {
	if onErr == nil {
		onErr = func(string, error) {}
	}
	key := "sysconfig:snapshot"
	if env != "" {
		key = env + ":" + key
	}
	return &RedisCache{rdb: rdb, key: key, ttl: CacheTTL, onErr: onErr}
}

// Key returns the Redis key (tests check its presence).
func (c *RedisCache) Key() string { return c.key }

func (c *RedisCache) Get(ctx context.Context) (Snapshot, bool) {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	raw, err := c.rdb.Get(ctx, c.key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false
	}
	if err != nil {
		c.onErr("get", err)
		return nil, false
	}
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		c.onErr("decode", err)
		return nil, false
	}
	if snap == nil {
		snap = Snapshot{}
	}
	return snap, true
}

func (c *RedisCache) Set(ctx context.Context, snap Snapshot) {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	raw, err := json.Marshal(snap)
	if err != nil {
		c.onErr("encode", err)
		return
	}
	if err := c.rdb.Set(ctx, c.key, raw, c.ttl).Err(); err != nil {
		c.onErr("set", err)
	}
}

func (c *RedisCache) Invalidate(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	if err := c.rdb.Del(ctx, c.key).Err(); err != nil {
		c.onErr("del", err)
	}
}
