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

	// Retry ping up to 5 times with exponential backoff to handle
	// transient network issues (DNS, slow WAN, DB cold start).
	const maxRetries = 5
	backoff := 2 * time.Second
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		lastErr = db.PingContext(ctx)
		cancel()
		if lastErr == nil {
			return db, nil
		}
		if i < maxRetries-1 {
			time.Sleep(backoff)
			backoff *= 2 // 2s -> 4s -> 8s -> 16s
		}
	}

	_ = db.Close()
	return nil, fmt.Errorf("ping database after %d retries: %w", maxRetries, lastErr)
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
