package cache

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"api-sync-go/internal/config"
)

const (
	PrefixUser   = "sync:user:"
	PrefixAvatar = "sync:avatar:"
)

// rateLimitLua runs atomic INCR and sets TTL on key creation, returning {count, ttl_remaining}
var rateLimitLua = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
end
local ttl = redis.call('TTL', KEYS[1])
return {count, ttl}
`)

type RedisClient struct {
	client *redis.Client
	cfg    *config.Config
}

func Connect(cfg *config.Config) (*RedisClient, error) {
	opts, err := parseRedisURL(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}

	opts.PoolSize = 50
	opts.MinIdleConns = 10
	opts.DialTimeout = 3 * time.Second
	opts.ReadTimeout = 2 * time.Second
	opts.WriteTimeout = 2 * time.Second

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return &RedisClient{client: client, cfg: cfg}, nil
}

func (r *RedisClient) GetUser(ctx context.Context, steamID string) (string, error) {
	return r.client.Get(ctx, PrefixUser+steamID).Result()
}

func (r *RedisClient) SetUser(ctx context.Context, steamID string, data string) error {
	return r.client.Set(ctx, PrefixUser+steamID, data, r.cfg.UserCacheTTL).Err()
}

func (r *RedisClient) SetUserWithTTL(ctx context.Context, steamID string, data string, ttl time.Duration) error {
	return r.client.Set(ctx, PrefixUser+steamID, data, ttl).Err()
}

func (r *RedisClient) GetAvatar(ctx context.Context, steamID string) ([]byte, error) {
	return r.client.Get(ctx, PrefixAvatar+steamID).Bytes()
}

func (r *RedisClient) SetAvatar(ctx context.Context, steamID string, data []byte) error {
	return r.client.Set(ctx, PrefixAvatar+steamID, data, r.cfg.AvatarCacheTTL).Err()
}

func (r *RedisClient) SetAvatarWithTTL(ctx context.Context, steamID string, data []byte, ttl time.Duration) error {
	return r.client.Set(ctx, PrefixAvatar+steamID, data, ttl).Err()
}

func (r *RedisClient) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}

// Incr atomically increments key and returns current count + TTL remaining.
func (r *RedisClient) Incr(ctx context.Context, key string, ttl time.Duration) (int64, time.Duration, error) {
	res, err := rateLimitLua.Run(ctx, r.client, []string{key}, int(ttl.Seconds())).Result()
	if err != nil {
		return 0, 0, err
	}

	vals, ok := res.([]any)
	if !ok || len(vals) < 2 {
		return 0, 0, fmt.Errorf("invalid response from rate limit lua script")
	}

	count, _ := vals[0].(int64)
	ttlSec, _ := vals[1].(int64)
	if ttlSec < 0 {
		ttlSec = int64(ttl.Seconds())
	}

	return count, time.Duration(ttlSec) * time.Second, nil
}

// parseRedisURL attempts standard ParseURL, and falls back to escaping unencoded special characters in the password.
func parseRedisURL(rawURL string) (*redis.Options, error) {
	opts, err := redis.ParseURL(rawURL)
	if err == nil {
		return opts, nil
	}

	// If failed, check if password contains unencoded special characters like '@'
	if strings.HasPrefix(rawURL, "redis://") {
		trimmed := strings.TrimPrefix(rawURL, "redis://")
		lastAt := strings.LastIndex(trimmed, "@")
		if lastAt != -1 {
			userinfo := trimmed[:lastAt]
			hostPart := trimmed[lastAt+1:]

			colonIdx := strings.Index(userinfo, ":")
			if colonIdx != -1 {
				user := userinfo[:colonIdx]
				pass := userinfo[colonIdx+1:]
				encodedPass := url.QueryEscape(pass)
				sanitized := fmt.Sprintf("redis://%s:%s@%s", user, encodedPass, hostPart)
				if sanitizedOpts, sErr := redis.ParseURL(sanitized); sErr == nil {
					return sanitizedOpts, nil
				}
			}
		}
	}

	return nil, err
}
