package queue

import (
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/hibiken/asynq"
)

// Client enqueues background jobs onto Redis via Asynq.
type Client struct {
	asynq *asynq.Client
}

// NewClient builds an Asynq client from Redis config.
func NewClient(cfg config.RedisConfig) *Client {
	return &Client{
		asynq: asynq.NewClient(asynq.RedisClientOpt{
			Addr:     cfg.Addr,
			Password: cfg.Password,
			DB:       cfg.DB,
		}),
	}
}

// Enqueue schedules a task.
func (c *Client) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	if c == nil || c.asynq == nil {
		return nil, fmt.Errorf("queue: client is nil")
	}
	info, err := c.asynq.Enqueue(task, opts...)
	if err != nil {
		return nil, fmt.Errorf("queue: enqueue %s: %w", task.Type(), err)
	}
	return info, nil
}

// Close releases the underlying Redis connection.
func (c *Client) Close() error {
	if c == nil || c.asynq == nil {
		return nil
	}
	return c.asynq.Close()
}

// RedisOpt exposes Asynq Redis options for the worker.
func RedisOpt(cfg config.RedisConfig) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
}
