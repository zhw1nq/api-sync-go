package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"api-sync-go/internal/cache"
)

func RateLimit(rc *cache.RedisClient, max int, window time.Duration) func(http.Handler) http.Handler {
	if max <= 0 {
		max = 300
	}
	if window <= 0 {
		window = time.Minute
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-KEY")
			key := fmt.Sprintf("sync:rl:%s", apiKey)

			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()

			count, resetIn, err := rc.Incr(ctx, key, window)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			remaining := max - int(count)
			if remaining < 0 {
				remaining = 0
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(max))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(int64(resetIn.Seconds()), 10))

			if count > int64(max) {
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded", "RATE_LIMITED")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
