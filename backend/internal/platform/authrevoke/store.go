// Package authrevoke makes stateless access tokens revocable before they
// expire. Logout marks the token's session, password changes mark the whole
// user; both markers live in Redis only as long as an access token can, so
// the check is one MGET per authenticated request and the set never grows.
package authrevoke

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Store keeps revocation markers in Redis.
type Store struct {
	rdb *redis.Client
	env string
	ttl time.Duration
	now func() time.Time
}

// New creates a store. ttl must cover the access token lifetime.
func New(rdb *redis.Client, appEnv string, accessTTL time.Duration) *Store {
	return &Store{rdb: rdb, env: appEnv, ttl: accessTTL + time.Minute, now: time.Now}
}

func (s *Store) sessionKey(sid uuid.UUID) string {
	return fmt.Sprintf("app:%s:auth:revoked:sid:%s", s.env, sid)
}

func (s *Store) userKey(user uuid.UUID) string {
	return fmt.Sprintf("app:%s:auth:revoked:user:%s", s.env, user)
}

// RevokeSession rejects every access token issued for the session.
func (s *Store) RevokeSession(ctx context.Context, sid uuid.UUID) error {
	if s == nil || sid == uuid.Nil {
		return nil
	}
	return s.rdb.Set(ctx, s.sessionKey(sid), "1", s.ttl).Err()
}

// RevokeUser rejects every access token the user holds right now. Tokens
// issued afterwards (a new login) are accepted.
func (s *Store) RevokeUser(ctx context.Context, user uuid.UUID) error {
	if s == nil || user == uuid.Nil {
		return nil
	}
	return s.rdb.Set(ctx, s.userKey(user), strconv.FormatInt(s.now().Unix(), 10), s.ttl).Err()
}

// IsRevoked reports whether a token for user/session issued at issuedAt was revoked.
func (s *Store) IsRevoked(ctx context.Context, user, sid uuid.UUID, issuedAt time.Time) (bool, error) {
	if s == nil {
		return false, nil
	}
	keys := []string{s.userKey(user)}
	if sid != uuid.Nil {
		keys = append(keys, s.sessionKey(sid))
	}
	vals, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return false, err
	}
	if cutoff, ok := vals[0].(string); ok {
		// Second resolution on both sides: a token minted in the same second
		// as the cutoff (the fresh login right after a password change) stays valid.
		if at, err := strconv.ParseInt(cutoff, 10, 64); err == nil && issuedAt.Unix() < at {
			return true, nil
		}
	}
	if len(vals) > 1 && vals[1] != nil {
		return true, nil
	}
	return false, nil
}
