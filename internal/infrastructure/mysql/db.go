package mysql

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql" // MySQL driver registration.

	"github.com/fullwa/fullwa/internal/infrastructure/config"
)

// Open opens the fullWA MySQL pool with the driver + pool limits from cfg.
// It pings the server before returning so the caller sees connection errors
// synchronously at boot instead of on the first query.
func Open(ctx context.Context, cfg config.MySQLConfig) (*sql.DB, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("mysql: empty DSN")
	}
	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("mysql open: %w", err)
	}
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("mysql ping: %w", err)
	}
	return db, nil
}
