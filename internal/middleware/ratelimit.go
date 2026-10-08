package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"api-sync-go/internal/cache"
)

const (
	rateLimitMax    = 60
	rateLimitWindow = 1 * time.Minute
)

func RateLimit(rc *cache.RedisClient) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-KEY")
			key := fmt.Sprintf("sync:rl:%s", apiKey)

			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()

			count, resetIn, err := rc.Incr(ctx, key, rateLimitWindow)
			if err != nil {
				// On Redis error, allow request through (fail open)
				next.ServeHTTP(w, r)
				return
			}

			remaining := rateLimitMax - int(count)
			if remaining < 0 {
				remaining = 0
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rateLimitMax))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(int64(resetIn.Seconds()), 10))

			if count > rateLimitMax {
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded", "RATE_LIMITED")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
