package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/places"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
)

// ActionGoogleRatingRefreshFailed is the activity log entry written once a
// showcase's Places refresh failed RatingFailureAlertAt times in a row.
const ActionGoogleRatingRefreshFailed = "showcase.google_rating.refresh_failed"

const (
	// RatingFreshFor: a Places rating younger than this is not fetched
	// again (a second run the same day is a no-op).
	RatingFreshFor = 20 * time.Hour
	// RatingFailureAlertAt consecutive failures write the activity log.
	RatingFailureAlertAt = 3
	ratingBackoffBase    = 20 * time.Hour
	ratingBackoffMax     = 7 * 24 * time.Hour
)

// RatingBackoff is the wait after the n-th consecutive failure of an
// organization: 20h, 40h, 80h, … capped at 7 days (the job runs daily, so
// the first failure retries the next day).
func RatingBackoff(n int) time.Duration {
	if n < 1 {
		return 0
	}
	if n > 10 {
		return ratingBackoffMax
	}
	return min(ratingBackoffBase<<(n-1), ratingBackoffMax)
}

// FailureStore keeps the consecutive Places failures of an organization
// and the earliest next attempt.
type FailureStore interface {
	NextAttempt(ctx context.Context, orgID int64) (time.Time, error)
	// Fail counts a failure at now and returns the consecutive count.
	Fail(ctx context.Context, orgID int64, now time.Time) (int, error)
	Reset(ctx context.Context, orgID int64) error
}

// RatingRefresher is the daily Places worker (worker-core, low queue): it
// refreshes google_rating / google_review_count of showcases with a place
// id and the module on. A failure keeps the old value (the source never
// falls back to manual) and backs the organization off exponentially; the
// third failure in a row writes the activity log. Without an API key the
// run is a no-op.
type RatingRefresher struct {
	q        *db.Queries
	places   PlacesClient
	features FeatureChecker
	failures FailureStore
	log      *slog.Logger
	now      func() time.Time
}

// NewRatingRefresher creates the worker. features nil means every module
// is on; failures nil keeps the backoff in memory.
func NewRatingRefresher(q *db.Queries, p PlacesClient, f FeatureChecker, failures FailureStore, log *slog.Logger) *RatingRefresher {
	if failures == nil {
		failures = NewMemoryFailures()
	}
	if log == nil {
		log = slog.Default()
	}
	return &RatingRefresher{q: q, places: p, features: f, failures: failures, log: log, now: time.Now}
}

// RefreshResult counts one run.
type RefreshResult struct {
	Updated int // rating written
	Skipped int // module off, backing off or no rating at Google yet
	Failed  int // Places error (old value kept)
}

// Task runs one refresh (the asynq processor).
func (r *RatingRefresher) Task(ctx context.Context) error {
	res, err := r.Run(ctx)
	if err == nil && (res.Updated > 0 || res.Failed > 0) {
		r.log.Info("showcase_google_rating_refreshed", "updated", res.Updated, "skipped", res.Skipped, "failed", res.Failed)
	}
	return err
}

// Run refreshes every due showcase. Per-showcase Places errors are counted
// and backed off, not returned; a quota error stops the run (the rest is
// tried on the next run).
func (r *RatingRefresher) Run(ctx context.Context) (RefreshResult, error) {
	var res RefreshResult
	if r.places == nil || !r.places.Configured() {
		return res, nil
	}
	now := r.now()
	rows, err := r.q.ListDealerShowcasesForPlacesRefresh(ctx, pgtype.Timestamptz{Time: now.Add(-RatingFreshFor), Valid: true})
	if err != nil {
		return res, err
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if r.features != nil {
			on, err := r.features.Enabled(ctx, row.OrganizationID, features.ModuleDealerShowcase)
			if err != nil {
				return res, err
			}
			if !on {
				res.Skipped++
				continue
			}
		}
		if next, err := r.failures.NextAttempt(ctx, row.OrganizationID); err != nil {
			r.log.Warn("showcase_google_rating_backoff_read_failed", "organization_id", row.OrganizationID, "error", err)
		} else if next.After(now) {
			res.Skipped++
			continue
		}
		got, err := r.places.Rating(ctx, row.GooglePlaceID.String)
		if errors.Is(err, places.ErrNotConfigured) {
			return res, nil
		}
		if err != nil {
			res.Failed++
			r.fail(ctx, row, now, err)
			if errors.Is(err, places.ErrQuota) {
				break
			}
			continue
		}
		if err := r.failures.Reset(ctx, row.OrganizationID); err != nil {
			r.log.Warn("showcase_google_rating_backoff_reset_failed", "organization_id", row.OrganizationID, "error", err)
		}
		if got.Rating == nil || *got.Rating < MinRating || *got.Rating > MaxRating {
			res.Skipped++ // no reviews at Google yet: keep what we have
			continue
		}
		rating, err := ratingNumeric(*got.Rating)
		if err != nil {
			return res, err
		}
		count := min(max(got.ReviewCount, 0), math.MaxInt32)
		n, err := r.q.SetDealerShowcasePlacesRating(ctx, db.SetDealerShowcasePlacesRatingParams{
			ID: row.ID, GooglePlaceID: row.GooglePlaceID, GoogleRating: rating,
			GoogleReviewCount: pgtype.Int4{Int32: int32(count), Valid: true},
		})
		if err != nil {
			return res, err
		}
		if n == 1 {
			res.Updated++
		} else {
			res.Skipped++ // place id changed meanwhile
		}
	}
	return res, nil
}

func (r *RatingRefresher) fail(ctx context.Context, row db.ListDealerShowcasesForPlacesRefreshRow, now time.Time, cause error) {
	n, err := r.failures.Fail(ctx, row.OrganizationID, now)
	if err != nil {
		r.log.Warn("showcase_google_rating_backoff_write_failed", "organization_id", row.OrganizationID, "error", err)
	}
	r.log.Warn("showcase_google_rating_refresh_failed", "organization_id", row.OrganizationID,
		"failures", n, "error", cause)
	if n != RatingFailureAlertAt {
		return
	}
	id := row.Uuid
	if err := activity.Write(ctx, r.q, nil, ActionGoogleRatingRefreshFailed, "dealer_showcases", &id, map[string]any{
		"organization_uuid": row.OrganizationUuid.String(), "google_place_id": row.GooglePlaceID.String,
		"failures": n, "error": cause.Error(),
	}, activity.Meta{}); err != nil {
		r.log.Warn("showcase_google_rating_activity_failed", "organization_id", row.OrganizationID, "error", err)
	}
}

// MemoryFailures is an in-process FailureStore (tests, no Redis).
type MemoryFailures struct {
	mu   sync.Mutex
	n    map[int64]int
	next map[int64]time.Time
}

// NewMemoryFailures creates an empty in-memory store.
func NewMemoryFailures() *MemoryFailures {
	return &MemoryFailures{n: map[int64]int{}, next: map[int64]time.Time{}}
}

func (m *MemoryFailures) NextAttempt(_ context.Context, orgID int64) (time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.next[orgID], nil
}

func (m *MemoryFailures) Fail(_ context.Context, orgID int64, now time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.n[orgID]++
	m.next[orgID] = now.Add(RatingBackoff(m.n[orgID]))
	return m.n[orgID], nil
}

func (m *MemoryFailures) Reset(_ context.Context, orgID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.n, orgID)
	delete(m.next, orgID)
	return nil
}

// RedisFailures keeps the backoff in Redis (survives worker restarts).
type RedisFailures struct {
	rdb *redis.Client
	env string
}

// NewRedisFailures creates the Redis store; keys are namespaced by env.
func NewRedisFailures(rdb *redis.Client, appEnv string) *RedisFailures {
	return &RedisFailures{rdb: rdb, env: appEnv}
}

const redisFailuresTTL = 30 * 24 * time.Hour

func (s *RedisFailures) key(orgID int64) string {
	return fmt.Sprintf("app:%s:showcase:places_fail:%d", s.env, orgID)
}

func (s *RedisFailures) NextAttempt(ctx context.Context, orgID int64) (time.Time, error) {
	v, err := s.rdb.HGet(ctx, s.key(orgID), "next").Result()
	if errors.Is(err, redis.Nil) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	sec, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return time.Time{}, nil
	}
	return time.Unix(sec, 0), nil
}

func (s *RedisFailures) Fail(ctx context.Context, orgID int64, now time.Time) (int, error) {
	k := s.key(orgID)
	n, err := s.rdb.HIncrBy(ctx, k, "n", 1).Result()
	if err != nil {
		return 0, err
	}
	pipe := s.rdb.TxPipeline()
	pipe.HSet(ctx, k, "next", now.Add(RatingBackoff(int(n))).Unix())
	pipe.Expire(ctx, k, redisFailuresTTL)
	_, err = pipe.Exec(ctx)
	return int(n), err
}

func (s *RedisFailures) Reset(ctx context.Context, orgID int64) error {
	return s.rdb.Del(ctx, s.key(orgID)).Err()
}
