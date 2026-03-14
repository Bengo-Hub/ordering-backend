package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/bengobox/ordering-backend/internal/config"
)

// NewClient builds a Redis client for caching and rate limiting.
func NewClient(cfg config.RedisConfig) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Username:     cfg.Username,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  1 * time.Second,
		WriteTimeout: 1 * time.Second,
	})
}

// Ping performs a quick health check against Redis.
func Ping(ctx context.Context, client *redis.Client) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return client.Ping(ctx).Err()
}
