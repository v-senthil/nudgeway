package health

import (
	"context"
	"database/sql"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

// MySQLProbe returns a Probe that pings the given DB pool.
func MySQLProbe(db *sql.DB) Probe {
	return Probe{
		Name: "mysql",
		Check: func(ctx context.Context) error {
			if err := db.PingContext(ctx); err != nil {
				return fmt.Errorf("mysql: %w", err)
			}
			return nil
		},
	}
}

// RedisProbe returns a Probe that pings the given Redis client.
func RedisProbe(rdb *goredis.Client) Probe {
	return Probe{
		Name: "redis",
		Check: func(ctx context.Context) error {
			if err := rdb.Ping(ctx).Err(); err != nil {
				return fmt.Errorf("redis: %w", err)
			}
			return nil
		},
	}
}
