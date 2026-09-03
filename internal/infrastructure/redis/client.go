package redis

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"

	"github.com/fullwa/fullwa/internal/infrastructure/config"
)

// New returns a redis.Client configured from cfg. It does NOT ping — call
// Ping separately if you want a synchronous connection check.
func New(cfg config.RedisConfig) *goredis.Client {
	opts := &goredis.Options{
		Addr:     cfg.Addr,
		DB:       cfg.DB,
		Password: cfg.Password,
	}
	if cfg.PoolSize > 0 {
		opts.PoolSize = cfg.PoolSize
	}
	return goredis.NewClient(opts)
}

// Ping verifies the Redis server is reachable.
func Ping(ctx context.Context, c *goredis.Client) error {
	if err := c.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}
