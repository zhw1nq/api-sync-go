package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"api-sync-go/internal/cache"
)

type HealthHandler struct {
	db    *sql.DB
	cache *cache.RedisClient
}

func NewHealthHandler(db *sql.DB, rc *cache.RedisClient) *HealthHandler {
	return &HealthHandler{db: db, cache: rc}
}

func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	dbStatus := "connected"
	if err := h.db.PingContext(ctx); err != nil {
		dbStatus = "disconnected"
	}

	redisStatus := "connected"
	if err := h.cache.Ping(ctx); err != nil {
		redisStatus = "disconnected"
	}

	status := http.StatusOK
	if dbStatus != "connected" || redisStatus != "connected" {
		status = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"db":     dbStatus,
		"redis":  redisStatus,
	})
}
