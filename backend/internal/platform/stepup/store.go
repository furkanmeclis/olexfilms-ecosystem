package stepup

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	challengeTTL = 2 * time.Minute
	rateLimitTTL = 15 * time.Minute
	maxAttempts  = 5
)

// Store persists step-up grants and ephemeral challenges in Redis.
type Store struct {
	rdb *redis.Client
	env string
	now func() time.Time
}

// NewStore creates a Redis-backed step-up store.
func NewStore(rdb *redis.Client, appEnv string) *Store {
	return &Store{
		rdb: rdb,
		env: appEnv,
		now: time.Now,
	}
}

func (s *Store) grantKey(userUUID uuid.UUID) string {
	return fmt.Sprintf("app:%s:stepup:grant:%s", s.env, userUUID.String())
}

func (s *Store) challengeKey(userUUID uuid.UUID) string {
	return fmt.Sprintf("app:%s:stepup:challenge:%s", s.env, userUUID.String())
}

func (s *Store) rateKey(userUUID uuid.UUID) string {
	return fmt.Sprintf("app:%s:stepup:rate:%s", s.env, userUUID.String())
}

// SetGrant stores a grant with TTL derived from policy hours.
func (s *Store) SetGrant(ctx context.Context, userUUID uuid.UUID, method string, ttl time.Duration) (time.Time, error) {
	expiresAt := s.now().UTC().Add(ttl)
	payload := fmt.Sprintf("%s:%d", method, expiresAt.Unix())
	if err := s.rdb.Set(ctx, s.grantKey(userUUID), payload, ttl).Err(); err != nil {
		return time.Time{}, err
	}
	return expiresAt, nil
}

// GetGrant returns whether a grant exists and when it expires.
func (s *Store) GetGrant(ctx context.Context, userUUID uuid.UUID) (bool, time.Time, error) {
	if s.rdb == nil {
		// No Redis: no grant can exist, sensitive endpoints fail closed.
		return false, time.Time{}, nil
	}
	val, err := s.rdb.Get(ctx, s.grantKey(userUUID)).Result()
	if err == redis.Nil {
		return false, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, err
	}
	parts := strings.SplitN(val, ":", 2)
	if len(parts) != 2 {
		return false, time.Time{}, nil
	}
	var unix int64
	if _, scanErr := fmt.Sscanf(parts[1], "%d", &unix); scanErr != nil {
		return false, time.Time{}, nil
	}
	return true, time.Unix(unix, 0).UTC(), nil
}

// RevokeGrant removes an active grant.
func (s *Store) RevokeGrant(ctx context.Context, userUUID uuid.UUID) error {
	return s.rdb.Del(ctx, s.grantKey(userUUID)).Err()
}

// SetChallenge stores a WebAuthn session blob.
func (s *Store) SetChallenge(ctx context.Context, userUUID uuid.UUID, data []byte) error {
	return s.rdb.Set(ctx, s.challengeKey(userUUID), data, challengeTTL).Err()
}

// GetChallenge loads and deletes a WebAuthn session blob.
func (s *Store) GetChallenge(ctx context.Context, userUUID uuid.UUID) ([]byte, error) {
	key := s.challengeKey(userUUID)
	val, err := s.rdb.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	_ = s.rdb.Del(ctx, key).Err()
	return val, nil
}

// IncrRateLimit increments failed password attempts; returns current count.
func (s *Store) IncrRateLimit(ctx context.Context, userUUID uuid.UUID) (int64, error) {
	key := s.rateKey(userUUID)
	n, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if n == 1 {
		_ = s.rdb.Expire(ctx, key, rateLimitTTL).Err()
	}
	return n, nil
}

// ClearRateLimit resets failed password attempts after success.
func (s *Store) ClearRateLimit(ctx context.Context, userUUID uuid.UUID) error {
	return s.rdb.Del(ctx, s.rateKey(userUUID)).Err()
}

// IsRateLimited reports whether the user exceeded failed attempts.
func (s *Store) IsRateLimited(ctx context.Context, userUUID uuid.UUID) (bool, error) {
	n, err := s.rdb.Get(ctx, s.rateKey(userUUID)).Int64()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return n >= maxAttempts, nil
}
