package queue

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/errtrack"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// schedulerLockKey guards the periodic-task scheduler: with more than one
// process running it (a scaled worker-core, a deploy overlap, an API with
// QUEUE_WORKER_INPROCESS next to a worker), only the lock holder registers
// cron tasks, so nothing is enqueued twice.
const schedulerLockKey = "worker:scheduler:leader"

const (
	schedulerLockTTL      = 30 * time.Second
	schedulerPollInterval = 10 * time.Second
)

// SchedulerEnabled reports SCHEDULER_ENABLED (default true): processes with
// "false" never run for the scheduler.
func SchedulerEnabled() bool {
	return os.Getenv("SCHEDULER_ENABLED") != "false"
}

// RunScheduler runs the periodic scheduler (every entry of Schedules) under
// the Redis leader lock until ctx ends, then stops it and releases the lock.
// cmd/worker and the in-process worker (QUEUE_WORKER_INPROCESS) both call
// it, so they share one lock and one task list (TEC-143).
func RunScheduler(ctx context.Context, cfg config.Config, rdb redis.UniversalClient, log *slog.Logger) {
	runScheduler(ctx, cfg, newSchedulerLock(rdb), schedulerPollInterval, 0, log)
}

func newSchedulerLock(rdb redis.UniversalClient) *leaderLock {
	hostname, _ := os.Hostname()
	return &leaderLock{
		rdb: rdb,
		key: schedulerLockKey,
		// The suffix keeps two schedulers of one process (API + in-process
		// worker in tests) distinct holders.
		id:  fmt.Sprintf("%s:%d:%s", hostname, os.Getpid(), uuid.NewString()[:8]),
		ttl: schedulerLockTTL,
	}
}

func runScheduler(ctx context.Context, cfg config.Config, lock *leaderLock, interval, heartbeat time.Duration, log *slog.Logger) {
	if log == nil {
		log = slog.Default()
	}
	runAsLeader(ctx, lock, interval, log, func() func() {
		scheduler, err := newScheduler(cfg, log, heartbeat)
		if err == nil {
			err = scheduler.Start()
		}
		if err != nil {
			log.Error("scheduler_failed", "error", err)
			errtrack.CaptureTask(ctx, errtrack.TaskInfo{
				Type: "scheduler", Queue: QueueMaintenance, Scheduler: true,
			}, err)
			return func() {}
		}
		return scheduler.Shutdown
	})
}

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
