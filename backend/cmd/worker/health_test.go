package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

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
