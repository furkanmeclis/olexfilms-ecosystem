package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// CacheTTL bounds how long a snapshot may be served after a change that did
// not invalidate it (design §6: "Redis 30 sn cache").
const CacheTTL = 30 * time.Second

// opTimeout bounds one cache call, so a Redis outage slows a request by at
// most this much before the database answers.
const opTimeout = 250 * time.Millisecond

// Cache stores per-organization snapshots.
//
// Keys are feat:v{gen}:org:{id}. A system-wide change (system switch,
// module default) bumps the generation, so every snapshot is dropped at once
// without SCAN; an organization change deletes the snapshots of that
// organization and the organizations below it.
type Cache interface {
	Get(ctx context.Context, orgID int64) ([]State, bool)
	Set(ctx context.Context, orgID int64, states []State)
	Invalidate(ctx context.Context, orgIDs ...int64)
	BumpGeneration(ctx context.Context)
}

// NoCache always misses (no Redis configured).
type NoCache struct{}

func (NoCache) Get(context.Context, int64) ([]State, bool) { return nil, false }
func (NoCache) Set(context.Context, int64, []State)        {}
func (NoCache) Invalidate(context.Context, ...int64)       {}
func (NoCache) BumpGeneration(context.Context)             {}

// RedisCache is the Redis-backed Cache. Redis errors read as a miss, so the
// caller falls back to the database (never fail-open).
type RedisCache struct {
	rdb    *redis.Client
	prefix string
	ttl    time.Duration
	onErr  func(op string, err error)
}

// NewRedisCache returns a cache under "<env>:feat:". onErr may be nil.
func NewRedisCache(rdb *redis.Client, env string, onErr func(op string, err error)) *RedisCache {
	if onErr == nil {
		onErr = func(string, error) {}
	}
	prefix := "feat:"
	if env != "" {
		prefix = env + ":feat:"
	}
	return &RedisCache{rdb: rdb, prefix: prefix, ttl: CacheTTL, onErr: onErr}
}

func (c *RedisCache) genKey() string { return c.prefix + "gen" }

func (c *RedisCache) gen(ctx context.Context) (int64, error) {
	v, err := c.rdb.Get(ctx, c.genKey()).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(v, 10, 64)
}

func (c *RedisCache) key(gen, orgID int64) string {
	return fmt.Sprintf("%sv%d:org:%d", c.prefix, gen, orgID)
}

// Get returns a cached snapshot.
func (c *RedisCache) Get(ctx context.Context, orgID int64) ([]State, bool) {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	gen, err := c.gen(ctx)
	if err != nil {
		c.onErr("gen", err)
		return nil, false
	}
	raw, err := c.rdb.Get(ctx, c.key(gen, orgID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false
	}
	if err != nil {
		c.onErr("get", err)
		return nil, false
	}
	var out []State
	if err := json.Unmarshal(raw, &out); err != nil {
		c.onErr("decode", err)
		return nil, false
	}
	return out, true
}

// Set stores a snapshot for CacheTTL.
func (c *RedisCache) Set(ctx context.Context, orgID int64, states []State) {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	gen, err := c.gen(ctx)
	if err != nil {
		c.onErr("gen", err)
		return
	}
	raw, err := json.Marshal(states)
	if err != nil {
		c.onErr("encode", err)
		return
	}
	if err := c.rdb.Set(ctx, c.key(gen, orgID), raw, c.ttl).Err(); err != nil {
		c.onErr("set", err)
	}
}

// Invalidate drops the snapshots of the given organizations.
func (c *RedisCache) Invalidate(ctx context.Context, orgIDs ...int64) {
	if len(orgIDs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	gen, err := c.gen(ctx)
	if err != nil {
		c.onErr("gen", err)
		c.BumpGeneration(ctx)
		return
	}
	keys := make([]string, 0, len(orgIDs))
	for _, id := range orgIDs {
		keys = append(keys, c.key(gen, id))
	}
	if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
		c.onErr("del", err)
	}
}

// BumpGeneration drops every snapshot.
func (c *RedisCache) BumpGeneration(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	if err := c.rdb.Incr(ctx, c.genKey()).Err(); err != nil {
		c.onErr("incr", err)
	}
}
