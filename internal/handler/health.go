package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"api-sync-go/internal/cache"
)

type HealthHandler struct {
	db     *sql.DB
	cache  *cache.RedisClient
	logger *slog.Logger
}

func NewHealthHandler(db *sql.DB, rc *cache.RedisClient, logger *slog.Logger) *HealthHandler {
	return &HealthHandler{db: db, cache: rc, logger: logger}
}

func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	dbStatus := "connected"
	redisStatus := "connected"

	var wg sync.WaitGroup
	wg.Add(2)

	// Check DB in parallel with dedicated timeout
	go func() {
		defer wg.Done()
		dbCtx, cancel := context.WithTimeout(r.Context(), 2500*time.Millisecond)
		defer cancel()
		if err := h.db.PingContext(dbCtx); err != nil {
			dbStatus = "disconnected"
			if h.logger != nil {
				h.logger.Warn("health check DB ping failed", slog.String("error", err.Error()))
			}
		}
	}()

	// Check Redis in parallel with dedicated timeout
	go func() {
		defer wg.Done()
		redisCtx, cancel := context.WithTimeout(r.Context(), 2500*time.Millisecond)
		defer cancel()
		if err := h.cache.Ping(redisCtx); err != nil {
			redisStatus = "disconnected"
			if h.logger != nil {
				h.logger.Warn("health check Redis ping failed", slog.String("error", err.Error()))
			}
		}
	}()

	wg.Wait()

	status := http.StatusOK
	respStatus := "ok"
	if dbStatus != "connected" || redisStatus != "connected" {
		status = http.StatusServiceUnavailable
		respStatus = "unhealthy"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": respStatus,
		"db":     dbStatus,
		"redis":  redisStatus,
	})
}
