package logging

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type memWriter struct {
	mu      sync.Mutex
	entries []Entry
}

func (m *memWriter) WriteLog(_ context.Context, entry Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry)
	return nil
}

func (m *memWriter) all() []Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Entry, len(m.entries))
	copy(out, m.entries)
	return out
}

func TestShouldPersist(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		level slog.Level
		want  bool
	}{
		{name: "debug", level: slog.LevelDebug, want: true},
		{name: "info", level: slog.LevelInfo, want: false},
		{name: "warn", level: slog.LevelWarn, want: true},
		{name: "error", level: slog.LevelError, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ShouldPersist(tc.level); got != tc.want {
				t.Fatalf("ShouldPersist(%s) = %v, want %v", tc.level, got, tc.want)
			}
		})
	}
}

func TestPersistLevel(t *testing.T) {
	t.Parallel()
	if PersistLevel(slog.LevelDebug) != "debug" {
		t.Fatal("expected debug")
	}
	if PersistLevel(slog.LevelWarn) != "warn" {
		t.Fatal("expected warn")
	}
	if PersistLevel(slog.LevelError) != "error" {
		t.Fatal("expected error")
	}
}

func TestAttachSkipsInfo(t *testing.T) {
	t.Parallel()
	w := &memWriter{}
	base := slog.New(slog.NewTextHandler(discard{}, nil))
	p := Attach(base, w)
	defer p.Close()

	log := p.Logger()
	log.Info("http_listen", "addr", ":8080")
	log.Warn("readyz_storage_soft_check", "error", "timeout")
	log.Error("queue_task_failed", "type", "app:ping")

	deadline := time.Now().Add(time.Second)
	var got []Entry
	for time.Now().Before(deadline) {
		got = w.all()
		if len(got) >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(got) != 2 {
		t.Fatalf("got %d persisted entries, want 2 (warn+error)", len(got))
	}
	if got[0].Level != "warn" || got[1].Level != "error" {
		t.Fatalf("unexpected levels: %+v", got)
	}
	for _, e := range got {
		if e.Message == "http_listen" {
			t.Fatal("info record must not be persisted")
		}
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
