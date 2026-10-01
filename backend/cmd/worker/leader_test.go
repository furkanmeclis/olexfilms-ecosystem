package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newLock(t *testing.T, mr *miniredis.Miniredis, id string) *leaderLock {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return &leaderLock{rdb: rdb, key: schedulerLockKey, id: id, ttl: 30 * time.Second}
}

func TestLeaderLockSingleHolder(t *testing.T) {
	mr := miniredis.RunT(t)
	ctx := context.Background()
	a, b := newLock(t, mr, "a"), newLock(t, mr, "b")

	if ok, err := a.hold(ctx); err != nil || !ok {
		t.Fatalf("a should acquire: ok=%v err=%v", ok, err)
	}
	if ok, err := b.hold(ctx); err != nil || ok {
		t.Fatalf("b must not acquire a held lock: ok=%v err=%v", ok, err)
	}
	// The holder renews its own lease.
	mr.FastForward(20 * time.Second)
	if ok, err := a.hold(ctx); err != nil || !ok {
		t.Fatalf("a should renew: ok=%v err=%v", ok, err)
	}
	mr.FastForward(20 * time.Second)
	if ok, _ := b.hold(ctx); ok {
		t.Fatal("renewed lease must still block b")
	}
	// b cannot release a's lock.
	if err := b.release(ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := mr.Get(schedulerLockKey); got != "a" {
		t.Fatalf("lock holder = %q, want a", got)
	}
	// Expiry hands the lock over.
	mr.FastForward(31 * time.Second)
	if ok, err := b.hold(ctx); err != nil || !ok {
		t.Fatalf("b should take over an expired lock: ok=%v err=%v", ok, err)
	}
	if ok, _ := a.hold(ctx); ok {
		t.Fatal("a lost the lease and must not renew b's lock")
	}
}

func TestRunAsLeaderStartsOnceAndReleases(t *testing.T) {
	mr := miniredis.RunT(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	a, b := newLock(t, mr, "a"), newLock(t, mr, "b")

	var startsA, stopsA, startsB atomic.Int32
	ctxA, cancelA := context.WithCancel(context.Background())
	doneA := make(chan struct{})
	go func() {
		runAsLeader(ctxA, a, 10*time.Millisecond, log, func() func() {
			startsA.Add(1)
			return func() { stopsA.Add(1) }
		})
		close(doneA)
	}()
	waitFor(t, func() bool { return startsA.Load() == 1 })

	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()
	go runAsLeader(ctxB, b, 10*time.Millisecond, log, func() func() {
		startsB.Add(1)
		return func() {}
	})
	time.Sleep(50 * time.Millisecond)
	if startsB.Load() != 0 {
		t.Fatal("second worker must not start a scheduler while a holds the lock")
	}
	if startsA.Load() != 1 {
		t.Fatalf("leader started %d times, want 1", startsA.Load())
	}

	// Shutdown of a stops its scheduler and releases the lock; b takes over.
	cancelA()
	<-doneA
	if stopsA.Load() != 1 {
		t.Fatalf("stop called %d times, want 1", stopsA.Load())
	}
	waitFor(t, func() bool { return startsB.Load() == 1 })
}

func TestRunHealthFileTouchesOnPing(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = rdb.Close() }()
	path := filepath.Join(t.TempDir(), "worker.health")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runHealthFile(ctx, rdb, path, time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	waitFor(t, func() bool { _, err := os.Stat(path); return err == nil })
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}
