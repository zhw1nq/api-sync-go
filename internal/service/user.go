package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"api-sync-go/internal/cache"
	"api-sync-go/internal/config"
	"api-sync-go/internal/model"
	"api-sync-go/pkg/singleflight"
)

var ErrUserNotFound = errors.New("user not found")

type UserService struct {
	db     *sql.DB
	cache  *cache.RedisClient
	cfg    *config.Config
	logger *slog.Logger
	sf     singleflight.Group
}

func NewUserService(db *sql.DB, rc *cache.RedisClient, cfg *config.Config, logger *slog.Logger) *UserService {
	return &UserService{
		db:     db,
		cache:  rc,
		cfg:    cfg,
		logger: logger,
	}
}

const userQuery = `
	SELECT u.name, u.image
	FROM steam_profile sp
	INNER JOIN user u ON u.id = sp.user_id
	WHERE sp.steam_id_64 = ?
	  AND u.banned = 0
	  AND u.status = 'active'
	LIMIT 1
`

func (s *UserService) GetBySteamID(ctx context.Context, steamID64 string) (*model.UserInfo, error) {
	// 1. Check cache
	cached, err := s.cache.GetUser(ctx, steamID64)
	if err == nil && cached != "" {
		if cached == "__NOT_FOUND__" {
			s.logger.Debug("cache hit (not found)", slog.String("steamid", steamID64))
			return nil, ErrUserNotFound
		}
		var info model.UserInfo
		if json.Unmarshal([]byte(cached), &info) == nil {
			s.logger.Debug("cache hit", slog.String("steamid", steamID64))
			return &info, nil
		}
	}

	// 2. Query with singleflight to prevent cache stampede
	val, err, _ := s.sf.Do(steamID64, func() (any, error) {
		// Double-check cache
		if recheck, rErr := s.cache.GetUser(ctx, steamID64); rErr == nil && recheck != "" {
			if recheck == "__NOT_FOUND__" {
				return nil, ErrUserNotFound
			}
			var cachedInfo model.UserInfo
			if json.Unmarshal([]byte(recheck), &cachedInfo) == nil {
				return &cachedInfo, nil
			}
		}

		var name string
		var image sql.NullString

		dbCtx, dbCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer dbCancel()
		qErr := s.db.QueryRowContext(dbCtx, userQuery, steamID64).Scan(&name, &image)
		if qErr != nil {
			if errors.Is(qErr, sql.ErrNoRows) {
				_ = s.cache.SetUserWithTTL(ctx, steamID64, "__NOT_FOUND__", s.cfg.NegativeCacheTTL)
				return nil, ErrUserNotFound
			}
			return nil, fmt.Errorf("query user: %w", qErr)
		}

		info := &model.UserInfo{
			Name:  name,
			Image: image.String,
		}

		data, _ := json.Marshal(info)
		if setErr := s.cache.SetUser(ctx, steamID64, string(data)); setErr != nil {
			s.logger.Warn("failed to cache user", slog.String("steamid", steamID64), slog.String("error", setErr.Error()))
		}

		return info, nil
	})

	if err != nil {
		return nil, err
	}

	return val.(*model.UserInfo), nil
}
