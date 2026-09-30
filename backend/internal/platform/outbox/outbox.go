// Package outbox provides a transactional outbox for platform domain events.
//
// Domain mutations insert pending rows in the same DB transaction as the
// business write. A Publisher claims batches (FOR UPDATE SKIP LOCKED),
// publishes to events.Bus, and marks rows published or retries with backoff.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/dbtx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StatusPending   = "pending"
	StatusPublished = "published"
	StatusFailed    = "failed"

	DefaultMaxAttempts = 8
	DefaultBatchSize   = 50
	DefaultInterval    = 500 * time.Millisecond
	DefaultLease       = 30 * time.Second
)

// Enqueuer inserts an outbox row in a transaction.
type Enqueuer interface {
	Enqueue(ctx context.Context, tx pgx.Tx, ev events.Event) error
}

// Drainer publishes pending outbox rows to the bus.
type Drainer interface {
	Drain(ctx context.Context) (int, error)
}

// Row is a durable outbox record (memory or DB).
type Row struct {
	ID          int64
	UUID        uuid.UUID
	TenantID    *int64
	EventName   string
	Payload     []byte
	Status      string
	Attempts    int32
	LastError   string
	AvailableAt time.Time
	PublishedAt *time.Time
	CreatedAt   time.Time
}

// envelope is stored inside payload JSONB alongside the free-form event data.
type envelope struct {
	EventID     uuid.UUID      `json:"event_id"`
	ActorUserID *int64         `json:"actor_user_id,omitempty"`
	EntityType  string         `json:"entity_type,omitempty"`
	EntityID    *int64         `json:"entity_id,omitempty"`
	EntityUUID  *uuid.UUID     `json:"entity_uuid,omitempty"`
	OccurredAt  time.Time      `json:"occurred_at"`
	Data        map[string]any `json:"data"`
}

// Store is the Postgres-backed outbox.
type Store struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewStore creates a Postgres outbox store.
func NewStore(pool *pgxpool.Pool, q *db.Queries) *Store {
	return &Store{pool: pool, q: q}
}

// Enqueue inserts a pending outbox row using tx when provided, otherwise the
// ambient transaction from dbtx context. Returns an error if neither is set.
func (s *Store) Enqueue(ctx context.Context, tx pgx.Tx, ev events.Event) error {
	if s == nil {
		return fmt.Errorf("outbox: store is nil")
	}
	if tx == nil {
		var ok bool
		tx, ok = dbtx.Tx(ctx)
		if !ok {
			return fmt.Errorf("outbox: transaction required")
		}
	}
	body, err := marshalEvent(ev)
	if err != nil {
		return err
	}
	_, err = s.q.WithTx(tx).InsertOutboxEvent(ctx, db.InsertOutboxEventParams{
		EventName: ev.Name,
		Payload:   body,
	})
	if err != nil {
		return fmt.Errorf("outbox: insert: %w", err)
	}
	return nil
}

func marshalEvent(ev events.Event) ([]byte, error) {
	if err := events.ValidateEventName(ev.Name); err != nil {
		return nil, err
	}
	if ev.EventID == uuid.Nil {
		ev.EventID = uuid.New()
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now().UTC()
	}
	if ev.Payload == nil {
		ev.Payload = map[string]any{}
	}
	env := envelope{
		EventID:     ev.EventID,
		ActorUserID: ev.ActorUserID,
		EntityType:  ev.EntityType,
		EntityID:    ev.EntityID,
		EntityUUID:  ev.EntityUUID,
		OccurredAt:  ev.OccurredAt.UTC(),
		Data:        ev.Payload,
	}
	body, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("outbox: marshal payload: %w", err)
	}
	return body, nil
}

func unmarshalEvent(eventName string, raw []byte) (events.Event, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return events.Event{}, fmt.Errorf("outbox: unmarshal payload: %w", err)
	}
	if env.Data == nil {
		env.Data = map[string]any{}
	}
	env.Data = normalizeJSONNumbers(env.Data).(map[string]any)
	return events.Event{
		Name:        eventName,
		ActorUserID: env.ActorUserID,
		EntityType:  env.EntityType,
		EntityID:    env.EntityID,
		EntityUUID:  env.EntityUUID,
		Payload:     env.Data,
		OccurredAt:  env.OccurredAt,
		EventID:     env.EventID,
	}, nil
}

// normalizeJSONNumbers converts whole float64 values (from encoding/json) back to int64
// so in-process consumers see the same types as before the outbox round-trip.
func normalizeJSONNumbers(v any) any {
	switch x := v.(type) {
	case float64:
		if x == float64(int64(x)) {
			return int64(x)
		}
		return x
	case map[string]any:
		for k, vv := range x {
			x[k] = normalizeJSONNumbers(vv)
		}
		return x
	case []any:
		for i, vv := range x {
			x[i] = normalizeJSONNumbers(vv)
		}
		return x
	default:
		return v
	}
}

func (s *Store) claimBatch(ctx context.Context, limit int32, lease time.Duration) ([]Row, error) {
	if lease <= 0 {
		lease = DefaultLease
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("outbox: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := s.q.WithTx(tx).ClaimOutboxEvents(ctx, db.ClaimOutboxEventsParams{
		BatchLimit: limit,
		LeaseMs:    lease.Milliseconds(),
	})
	if err != nil {
		return nil, fmt.Errorf("outbox: claim: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("outbox: commit claim: %w", err)
	}
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowFromDB(r))
	}
	return out, nil
}

func (s *Store) markPublished(ctx context.Context, id int64) error {
	return s.q.MarkOutboxPublished(ctx, id)
}

func (s *Store) markRetry(ctx context.Context, id int64, attempts int32, lastErr string, availableAt time.Time) error {
	return s.q.MarkOutboxRetry(ctx, db.MarkOutboxRetryParams{
		ID:          id,
		Attempts:    attempts,
		LastError:   pgtype.Text{String: lastErr, Valid: lastErr != ""},
		AvailableAt: pgtype.Timestamptz{Time: availableAt.UTC(), Valid: true},
	})
}

func (s *Store) markFailed(ctx context.Context, id int64, attempts int32, lastErr string) error {
	return s.q.MarkOutboxFailed(ctx, db.MarkOutboxFailedParams{
		ID:        id,
		Attempts:  attempts,
		LastError: pgtype.Text{String: lastErr, Valid: lastErr != ""},
	})
}

func rowFromDB(r db.OutboxEvent) Row {
	out := Row{
		ID:        r.ID,
		UUID:      r.Uuid,
		EventName: r.EventName,
		Payload:   r.Payload,
		Status:    r.Status,
		Attempts:  r.Attempts,
	}
	if r.LastError.Valid {
		out.LastError = r.LastError.String
	}
	if r.AvailableAt.Valid {
		out.AvailableAt = r.AvailableAt.Time.UTC()
	}
	if r.PublishedAt.Valid {
		t := r.PublishedAt.Time.UTC()
		out.PublishedAt = &t
	}
	if r.CreatedAt.Valid {
		out.CreatedAt = r.CreatedAt.Time.UTC()
	}
	return out
}

type backend interface {
	claimBatch(ctx context.Context, limit int32, lease time.Duration) ([]Row, error)
	markPublished(ctx context.Context, id int64) error
	markRetry(ctx context.Context, id int64, attempts int32, lastErr string, availableAt time.Time) error
	markFailed(ctx context.Context, id int64, attempts int32, lastErr string) error
}

// Publisher claims pending rows and publishes them to the bus.
type Publisher struct {
	store       backend
	bus         events.Bus
	log         *slog.Logger
	maxAttempts int32
	batchSize   int32
	interval    time.Duration
	lease       time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
}

// PublisherOption configures a Publisher.
type PublisherOption func(*Publisher)

// WithMaxAttempts sets the attempts before status=failed.
func WithMaxAttempts(n int) PublisherOption {
	return func(p *Publisher) {
		if n > 0 {
			p.maxAttempts = int32(n)
		}
	}
}

// WithBatchSize sets claim batch size.
func WithBatchSize(n int) PublisherOption {
	return func(p *Publisher) {
		if n > 0 {
			p.batchSize = int32(n)
		}
	}
}

// WithInterval sets the ticker interval for Run.
func WithInterval(d time.Duration) PublisherOption {
	return func(p *Publisher) {
		if d > 0 {
			p.interval = d
		}
	}
}

// WithLease sets the claim lease duration.
func WithLease(d time.Duration) PublisherOption {
	return func(p *Publisher) {
		if d > 0 {
			p.lease = d
		}
	}
}

// NewPublisher wires a Postgres store to the event bus.
func NewPublisher(store *Store, bus events.Bus, log *slog.Logger, opts ...PublisherOption) *Publisher {
	return newPublisher(store, bus, log, opts...)
}

func newPublisher(store backend, bus events.Bus, log *slog.Logger, opts ...PublisherOption) *Publisher {
	if log == nil {
		log = slog.Default()
	}
	p := &Publisher{
		store:       store,
		bus:         bus,
		log:         log,
		maxAttempts: DefaultMaxAttempts,
		batchSize:   DefaultBatchSize,
		interval:    DefaultInterval,
		lease:       DefaultLease,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Drain claims and publishes one batch. Safe to call concurrently.
func (p *Publisher) Drain(ctx context.Context) (int, error) {
	if p == nil || p.store == nil || p.bus == nil {
		return 0, nil
	}
	rows, err := p.store.claimBatch(ctx, p.batchSize, p.lease)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}

	published := 0
	for _, row := range rows {
		if err := p.publishOne(ctx, row); err != nil {
			p.log.Error(
				"outbox_publish_failed",
				"outbox_id", row.ID,
				"event", row.EventName,
				"attempts", row.Attempts+1,
				"error", err,
			)
			continue
		}
		published++
	}
	return published, nil
}

func (p *Publisher) publishOne(ctx context.Context, row Row) error {
	ev, err := unmarshalEvent(row.EventName, row.Payload)
	if err != nil {
		_ = p.store.markFailed(ctx, row.ID, row.Attempts+1, err.Error())
		return err
	}
	if err := p.bus.Publish(ctx, ev); err != nil {
		return p.failOrRetry(ctx, row, err)
	}
	if err := p.store.markPublished(ctx, row.ID); err != nil {
		return fmt.Errorf("outbox: mark published: %w", err)
	}
	return nil
}

func (p *Publisher) failOrRetry(ctx context.Context, row Row, publishErr error) error {
	attempts := row.Attempts + 1
	msg := publishErr.Error()
	if attempts >= p.maxAttempts {
		if err := p.store.markFailed(ctx, row.ID, attempts, msg); err != nil {
			return fmt.Errorf("outbox: mark failed: %w", err)
		}
		p.log.Error(
			"outbox_event_failed_permanently",
			"outbox_id", row.ID,
			"event", row.EventName,
			"attempts", attempts,
			"error", publishErr,
		)
		return publishErr
	}
	availableAt := time.Now().UTC().Add(backoff(attempts))
	if err := p.store.markRetry(ctx, row.ID, attempts, msg, availableAt); err != nil {
		return fmt.Errorf("outbox: mark retry: %w", err)
	}
	return publishErr
}

func backoff(attempts int32) time.Duration {
	d := time.Second << (attempts - 1)
	if d > 5*time.Minute {
		d = 5 * time.Minute
	}
	if d < time.Second {
		d = time.Second
	}
	return d
}

// Run polls until ctx is cancelled.
func (p *Publisher) Run(ctx context.Context) {
	if p == nil {
		return
	}
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		if _, err := p.Drain(ctx); err != nil && ctx.Err() == nil {
			p.log.Error("outbox_drain_failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// StartRun launches Run in a goroutine and returns a stop function.
func (p *Publisher) StartRun(parent context.Context) (stop func()) {
	if p == nil {
		return func() {}
	}
	ctx, cancel := context.WithCancel(parent)
	p.mu.Lock()
	p.cancel = cancel
	p.mu.Unlock()
	go p.Run(ctx)
	return func() {
		cancel()
		p.mu.Lock()
		p.cancel = nil
		p.mu.Unlock()
	}
}
