package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/redis/go-redis/v9"
)

// NewRedisClient creates and pings a Redis client.
func NewRedisClient(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("cache: redis ping: %w", err)
	}

	return client, nil
}

// Ping checks Redis availability.
func Ping(ctx context.Context, client *redis.Client) error {
	if client == nil {
		return fmt.Errorf("cache: client is nil")
	}
	return client.Ping(ctx).Err()
}
