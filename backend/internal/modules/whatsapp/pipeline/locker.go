package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Locker serializes the runs of one conversation across workers.
type Locker interface {
	// Acquire takes key for ttl; ok is false while another holder has it.
	// release frees it (only when still held by this caller).
	Acquire(ctx context.Context, key string, ttl time.Duration) (release func(), ok bool, err error)
}

// RedisLocker is a SET NX PX lock with an owner token.
type RedisLocker struct {
	rdb    *redis.Client
	prefix string
}

// NewRedisLocker builds the lock on Redis under "<env>:walock:".
func NewRedisLocker(rdb *redis.Client, env string) *RedisLocker {
	prefix := "walock:"
	if env != "" {
		prefix = env + ":walock:"
	}
	return &RedisLocker{rdb: rdb, prefix: prefix}
}

var releaseScript = redis.NewScript(`if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) end return 0`)

// Acquire implements Locker.
func (l *RedisLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (func(), bool, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, false, err
	}
	token := hex.EncodeToString(b[:])
	full := l.prefix + key
	ok, err := l.rdb.SetNX(ctx, full, token, ttl).Result()
	if err != nil || !ok {
		return func() {}, false, err
	}
	return func() {
		_ = releaseScript.Run(context.WithoutCancel(ctx), l.rdb, []string{full}, token).Err()
	}, true, nil
}

// MemoryLocker is an in-process Locker (tests, single process).
type MemoryLocker struct {
	mu   sync.Mutex
	held map[string]time.Time
}

// Acquire implements Locker.
func (l *MemoryLocker) Acquire(_ context.Context, key string, ttl time.Duration) (func(), bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held == nil {
		l.held = map[string]time.Time{}
	}
	if until, ok := l.held[key]; ok && time.Now().Before(until) {
		return func() {}, false, nil
	}
	l.held[key] = time.Now().Add(ttl)
	return func() {
		l.mu.Lock()
		delete(l.held, key)
		l.mu.Unlock()
	}, true, nil
}
