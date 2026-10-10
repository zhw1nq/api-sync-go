package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"api-sync-go/internal/config"
)

func Connect(cfg *config.Config) (*sql.DB, error) {
	dsn, err := cfg.ParseDSN()
	if err != nil {
		return nil, fmt.Errorf("parse DSN: %w", err)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	if cfg.DBConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)
	} else {
		db.SetConnMaxLifetime(0) // 0: reuse forever
	}
	if cfg.DBConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(cfg.DBConnMaxIdleTime)
	} else {
		db.SetConnMaxIdleTime(0) // 0: never close due to idle time
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}

// StartKeepAlive runs a background goroutine that periodically pings the database.
// This prevents NAT/firewall idle connection drops, resets MariaDB wait_timeout,
// and ensures connection pool stays warm.
func StartKeepAlive(ctx context.Context, db *sql.DB, interval time.Duration, logger *slog.Logger) {
	if interval <= 0 {
		interval = 20 * time.Second
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := db.PingContext(pingCtx)
				cancel()
				if err != nil {
					if logger != nil {
						logger.Warn("db keepalive ping failed", slog.String("error", err.Error()))
					}
				} else {
					if logger != nil {
						logger.Debug("db keepalive ping ok")
					}
				}
			}
		}
	}()
}
