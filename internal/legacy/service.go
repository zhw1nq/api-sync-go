package legacy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"api-sync-go/internal/cache"
	"api-sync-go/internal/config"
	"api-sync-go/internal/model"
	"api-sync-go/internal/service"
	"api-sync-go/pkg/singleflight"
)

// LegacyUserService reads user data from legacy tables:
// - users (display_name, status, banned)
// - user_steam_profiles (steam_id)
// - user_discord_profiles (discord_id)
type LegacyUserService struct {
	db     *sql.DB
	cache  *cache.RedisClient
	cfg    *config.Config
	logger *slog.Logger
	sf     singleflight.Group
}

func NewUserService(db *sql.DB, rc *cache.RedisClient, cfg *config.Config, logger *slog.Logger) *LegacyUserService {
	return &LegacyUserService{
		db:     db,
		cache:  rc,
		cfg:    cfg,
		logger: logger,
	}
}

const legacyUserQuery = `
	SELECT 
		u.display_name,
		udp.discord_id
	FROM user_steam_profiles usp
	INNER JOIN users u ON u.id = usp.user_id
	LEFT JOIN user_discord_profiles udp ON udp.user_id = u.id
	WHERE usp.steam_id = ?
	  AND u.banned = 0
	  AND u.status = 'active'
	LIMIT 1
`

func (s *LegacyUserService) GetBySteamID(ctx context.Context, steamID64 string) (*model.UserInfo, error) {
	// 1. Check Redis cache
	cached, err := s.cache.GetUser(ctx, steamID64)
	if err == nil && cached != "" {
		if cached == "__NOT_FOUND__" {
			s.logger.Debug("legacy cache hit (not found)", slog.String("steamid", steamID64))
			return nil, service.ErrUserNotFound
		}
		var info model.UserInfo
		if json.Unmarshal([]byte(cached), &info) == nil {
			s.logger.Debug("legacy cache hit", slog.String("steamid", steamID64))
			return &info, nil
		}
	}

	// 2. Query with singleflight to prevent cache stampede
	val, err, _ := s.sf.Do(steamID64, func() (any, error) {
		// Double-check cache
		if recheck, rErr := s.cache.GetUser(ctx, steamID64); rErr == nil && recheck != "" {
			if recheck == "__NOT_FOUND__" {
				return nil, service.ErrUserNotFound
			}
			var cachedInfo model.UserInfo
			if json.Unmarshal([]byte(recheck), &cachedInfo) == nil {
				return &cachedInfo, nil
			}
		}

		var displayName string
		var discordID sql.NullString

		dbCtx, dbCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer dbCancel()
		qErr := s.db.QueryRowContext(dbCtx, legacyUserQuery, steamID64).Scan(&displayName, &discordID)
		if qErr != nil {
			if errors.Is(qErr, sql.ErrNoRows) {
				_ = s.cache.SetUserWithTTL(ctx, steamID64, "__NOT_FOUND__", s.cfg.NegativeCacheTTL)
				return nil, service.ErrUserNotFound
			}
			return nil, fmt.Errorf("query legacy user: %w", qErr)
		}

		imageURL := ""
		if discordID.Valid && strings.TrimSpace(discordID.String) != "" {
			imageURL = fmt.Sprintf("https://7-mau.com/api/v3/lookup/discord/avatar/%s", strings.TrimSpace(discordID.String))
		}

		info := &model.UserInfo{
			Name:  displayName,
			Image: imageURL,
		}

		data, _ := json.Marshal(info)
		if setErr := s.cache.SetUser(ctx, steamID64, string(data)); setErr != nil {
			s.logger.Warn("failed to cache legacy user", slog.String("steamid", steamID64), slog.String("error", setErr.Error()))
		}

		return info, nil
	})

	if err != nil {
		return nil, err
	}

	return val.(*model.UserInfo), nil
}
