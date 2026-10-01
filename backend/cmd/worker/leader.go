package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// schedulerLockKey guards the periodic-task scheduler: with more than one
// worker running SCHEDULER_ENABLED=true (a scaled worker-core, a deploy
// overlap), only the lock holder registers cron tasks, so nothing is
// enqueued twice.
const schedulerLockKey = "worker:scheduler:leader"

// Compare-and-act scripts: only the holder may extend or release the lock.
var (
	renewScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0`)
	releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0`)
)

// leaderLock is a single-key Redis lease (SET NX PX + holder-checked renew).
type leaderLock struct {
	rdb redis.UniversalClient
	key string
	id  string
	ttl time.Duration
}

// hold acquires the lock, or extends it when this process already holds it.
// It reports whether this process is the leader afterwards.
func (l *leaderLock) hold(ctx context.Context) (bool, error) {
	ok, err := l.rdb.SetNX(ctx, l.key, l.id, l.ttl).Result()
	if err != nil || ok {
		return ok, err
	}
	n, err := renewScript.Run(ctx, l.rdb, []string{l.key}, l.id, l.ttl.Milliseconds()).Int64()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

func (l *leaderLock) release(ctx context.Context) error {
	return releaseScript.Run(ctx, l.rdb, []string{l.key}, l.id).Err()
}

// runAsLeader polls the lock every interval (keep interval well below the
// lock TTL). On election it calls start; when the lease is lost, Redis is
// unreachable, or ctx ends, it calls the returned stop function. It returns
// after ctx is done and the lock is released.
func runAsLeader(ctx context.Context, l *leaderLock, interval time.Duration, log *slog.Logger, start func() (stop func())) {
	var stop func()
	step := func() {
		leader, err := l.hold(ctx)
		if err != nil && ctx.Err() == nil {
			log.Warn("scheduler_leader_lock_failed", "error", err)
		}
		switch {
		case leader && stop == nil:
			log.Info("scheduler_leader_elected", "id", l.id)
			stop = start()
		case !leader && stop != nil:
			log.Warn("scheduler_leader_lost", "id", l.id)
			stop()
			stop = nil
		}
	}
	step()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if stop != nil {
				stop()
			}
			// ctx is done: release with a short fresh context.
			rctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = l.release(rctx)
			cancel()
			return
		case <-ticker.C:
			step()
		}
	}
}
