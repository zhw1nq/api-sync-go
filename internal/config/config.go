package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	APIKeys        []string
	AllowedDomains []string

	DatabaseURL string
	RedisURL    string

	UserCacheTTL     time.Duration
	NegativeCacheTTL time.Duration
	AvatarCacheTTL   time.Duration

	AvatarSize    int
	AvatarQuality int

	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration
	DBConnMaxIdleTime time.Duration

	LogLevel   string
	LegacyMode bool
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	dbURL, err := getRequiredEnv("DATABASE_URL")
	if err != nil {
		return nil, err
	}

	keys, err := getRequiredEnv("API_KEYS")
	if err != nil {
		return nil, err
	}

	// Đọc ALLOWED_DOMAIN bắt buộc từ env
	domainRaw := os.Getenv("ALLOWED_DOMAIN")
	if domainRaw == "" {
		domainRaw = os.Getenv("ALLOWED_DOMAINS")
	}
	if domainRaw == "" {
		domainRaw = os.Getenv("ALLOWED_ORIGINS")
	}
	if domainRaw == "" {
		return nil, fmt.Errorf("missing required environment variable: ALLOWED_DOMAIN")
	}

	cfg := &Config{
		DatabaseURL:       dbURL,
		RedisURL:          getEnvOrDefault("REDIS_URL", "redis://localhost:6379"),
		UserCacheTTL:      parseDuration("USER_CACHE_TTL", 5*time.Minute),
		NegativeCacheTTL:  parseDuration("NEGATIVE_CACHE_TTL", 1*time.Minute),
		AvatarCacheTTL:    parseDuration("AVATAR_CACHE_TTL", 30*time.Minute),
		AvatarSize:        parseInt("AVATAR_SIZE", 184),
		AvatarQuality:     parseInt("AVATAR_QUALITY", 80),
		DBMaxOpenConns:    parseInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:    parseInt("DB_MAX_IDLE_CONNS", 10),
		DBConnMaxLifetime: parseDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		DBConnMaxIdleTime: parseDuration("DB_CONN_MAX_IDLE_TIME", 2*time.Minute),
		LogLevel:          strings.ToLower(getEnvOrDefault("LOG_LEVEL", "info")),
		LegacyMode:        strings.EqualFold(os.Getenv("LEGACY_MODE"), "true"),
	}

	for _, k := range strings.Split(keys, ",") {
		k = strings.TrimSpace(k)
		if k != "" {
			cfg.APIKeys = append(cfg.APIKeys, k)
		}
	}
	if len(cfg.APIKeys) == 0 {
		return nil, fmt.Errorf("API_KEYS must contain at least one valid key")
	}

	for _, d := range strings.Split(domainRaw, ",") {
		d = strings.TrimSpace(d)
		if d != "" {
			cfg.AllowedDomains = append(cfg.AllowedDomains, d)
		}
	}
	if len(cfg.AllowedDomains) == 0 {
		return nil, fmt.Errorf("ALLOWED_DOMAIN must contain at least one valid domain")
	}

	return cfg, nil
}

// ParseDSN converts mysql://user:pass@host:port/dbname to Go DSN format.
func (c *Config) ParseDSN() (string, error) {
	raw := c.DatabaseURL

	// Strip "mysql://" prefix
	raw = strings.TrimPrefix(raw, "mysql://")

	// Split user:pass@host:port/dbname
	atIdx := strings.LastIndex(raw, "@")
	if atIdx < 0 {
		return "", fmt.Errorf("invalid DATABASE_URL: missing @")
	}

	userPass := raw[:atIdx]
	hostDBPart := raw[atIdx+1:]

	slashIdx := strings.Index(hostDBPart, "/")
	if slashIdx < 0 {
		return "", fmt.Errorf("invalid DATABASE_URL: missing /dbname")
	}

	hostPort := hostDBPart[:slashIdx]
	dbName := hostDBPart[slashIdx+1:]

	// Trim query params from dbName if present
	if qIdx := strings.Index(dbName, "?"); qIdx >= 0 {
		dbName = dbName[:qIdx]
	}

	return fmt.Sprintf("%s@tcp(%s)/%s?parseTime=true&loc=Local&charset=utf8mb4&interpolateParams=true", userPass, hostPort, dbName), nil
}

func getRequiredEnv(key string) (string, error) {
	val := os.Getenv(key)
	if val == "" {
		return "", fmt.Errorf("missing required environment variable: %s", key)
	}
	return val, nil
}

func getEnvOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func parseDuration(key string, fallback time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return fallback
	}
	return d
}

func parseInt(key string, fallback int) int {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return n
}
