package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Memory is an in-process outbox for unit tests.
type Memory struct {
	mu     sync.Mutex
	nextID int64
	rows   map[int64]*Row
}

// NewMemory creates an empty memory outbox.
func NewMemory() *Memory {
	return &Memory{
		nextID: 1,
		rows:   map[int64]*Row{},
	}
}

// Enqueue stores a pending row. tx may be nil.
func (m *Memory) Enqueue(_ context.Context, _ pgx.Tx, ev events.Event) error {
	if m == nil {
		return fmt.Errorf("outbox: memory store is nil")
	}
	body, err := marshalEvent(ev)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextID
	m.nextID++
	now := time.Now().UTC()
	m.rows[id] = &Row{
		ID:          id,
		UUID:        uuid.New(),
		EventName:   ev.Name,
		Payload:     body,
		Status:      StatusPending,
		AvailableAt: now,
		CreatedAt:   now,
	}
	return nil
}

// Pending returns a snapshot of currently available pending rows (tests).
func (m *Memory) Pending() []Row {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Row, 0)
	now := time.Now().UTC()
	for _, r := range m.rows {
		if r.Status == StatusPending && !r.AvailableAt.After(now) {
			cp := *r
			out = append(out, cp)
		}
	}
	return out
}

// All returns all rows (tests).
func (m *Memory) All() []Row {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Row, 0, len(m.rows))
	for _, r := range m.rows {
		cp := *r
		out = append(out, cp)
	}
	return out
}

func (m *Memory) claimBatch(_ context.Context, limit int32, lease time.Duration) ([]Row, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if lease <= 0 {
		lease = DefaultLease
	}
	now := time.Now().UTC()
	out := make([]Row, 0)
	for _, r := range m.rows {
		if r.Status != StatusPending || r.AvailableAt.After(now) {
			continue
		}
		r.AvailableAt = now.Add(lease)
		cp := *r
		out = append(out, cp)
		if int32(len(out)) >= limit {
			break
		}
	}
	return out, nil
}

func (m *Memory) markPublished(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok {
		return fmt.Errorf("outbox: row %d not found", id)
	}
	now := time.Now().UTC()
	r.Status = StatusPublished
	r.PublishedAt = &now
	r.LastError = ""
	return nil
}

func (m *Memory) markRetry(_ context.Context, id int64, attempts int32, lastErr string, availableAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok {
		return fmt.Errorf("outbox: row %d not found", id)
	}
	r.Status = StatusPending
	r.Attempts = attempts
	r.LastError = lastErr
	r.AvailableAt = availableAt.UTC()
	return nil
}

func (m *Memory) markFailed(_ context.Context, id int64, attempts int32, lastErr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok {
		return fmt.Errorf("outbox: row %d not found", id)
	}
	r.Status = StatusFailed
	r.Attempts = attempts
	r.LastError = lastErr
	r.AvailableAt = time.Now().UTC()
	return nil
}

// ForceAvailable sets all pending rows' available_at to the past (tests).
func (m *Memory) ForceAvailable() {
	m.mu.Lock()
	defer m.mu.Unlock()
	past := time.Now().UTC().Add(-time.Second)
	for _, r := range m.rows {
		if r.Status == StatusPending {
			r.AvailableAt = past
		}
	}
}

// NewMemoryPublisher builds a publisher over a memory store.
func NewMemoryPublisher(mem *Memory, bus events.Bus, log *slog.Logger, opts ...PublisherOption) *Publisher {
	return newPublisher(mem, bus, log, opts...)
}
