package auth

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient creates a new Redis client from a DSN and verifies connectivity.
func NewRedisClient(dsn string) (*redis.Client, error) {
	opt, err := redis.ParseURL(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse Redis DSN: %w", err)
	}
	client := redis.NewClient(opt)
	if _, err := client.Ping(context.Background()).Result(); err != nil {
		return nil, fmt.Errorf("ping Redis: %w", err)
	}
	return client, nil
}
