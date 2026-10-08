package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"api-sync-go/internal/cache"
	"api-sync-go/internal/config"
	"api-sync-go/pkg/imgutil"
	"api-sync-go/pkg/singleflight"
	"api-sync-go/public"
)

const DefaultAvatarTTL = 7 * 24 * time.Hour

type ImageService struct {
	cache         *cache.RedisClient
	cfg           *config.Config
	httpClient    *http.Client
	defaultAvatar []byte
	logger        *slog.Logger
	sf            singleflight.Group
}

func NewImageService(rc *cache.RedisClient, cfg *config.Config, logger *slog.Logger) *ImageService {
	defaultAvatar := public.DefaultAvatar
	if len(defaultAvatar) == 0 {
		logger.Warn("embedded default avatar is empty")
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &ImageService{
		cache:         rc,
		cfg:           cfg,
		defaultAvatar: defaultAvatar,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		},
		logger: logger,
	}
}

func (s *ImageService) GetDefaultAvatar() []byte {
	return s.defaultAvatar
}

func (s *ImageService) GetCachedAvatar(ctx context.Context, steamID64 string) ([]byte, error) {
	return s.cache.GetAvatar(ctx, steamID64)
}

func (s *ImageService) CacheDefaultAvatar(ctx context.Context, steamID64 string) error {
	if len(s.defaultAvatar) == 0 {
		return nil
	}
	return s.cache.SetAvatarWithTTL(ctx, steamID64, s.defaultAvatar, DefaultAvatarTTL)
}

func (s *ImageService) GetCompressedAvatar(ctx context.Context, steamID64, imageURL string) ([]byte, error) {
	// 1. Check avatar cache
	cached, err := s.cache.GetAvatar(ctx, steamID64)
	if err == nil && len(cached) > 0 {
		s.logger.Debug("avatar cache hit", slog.String("steamid", steamID64))
		return cached, nil
	}

	// If no image URL provided, fallback to default avatar (TTL 7 days)
	if imageURL == "" {
		if setErr := s.CacheDefaultAvatar(ctx, steamID64); setErr != nil {
			s.logger.Warn("failed to cache default avatar", slog.String("steamid", steamID64), slog.String("error", setErr.Error()))
		}
		return s.defaultAvatar, nil
	}

	// 2. Fetch + compress with singleflight deduplication
	val, err, _ := s.sf.Do(steamID64, func() (any, error) {
		// Double-check cache inside singleflight
		if recheck, rErr := s.cache.GetAvatar(ctx, steamID64); rErr == nil && len(recheck) > 0 {
			return recheck, nil
		}

		fetchURL := normalizeImageURL(imageURL)

		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
		if reqErr != nil {
			return nil, fmt.Errorf("create request: %w", reqErr)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/png,image/jpeg,image/*,*/*;q=0.8")

		resp, doErr := s.httpClient.Do(req)
		if doErr != nil {
			return nil, fmt.Errorf("fetch image: %w", doErr)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			return nil, fmt.Errorf("image fetch returned %d", resp.StatusCode)
		}

		// Limit read to 10MB
		srcBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
		if readErr != nil {
			return nil, fmt.Errorf("read image body: %w", readErr)
		}

		// Compress
		compressed, compErr := imgutil.CompressAvatar(srcBytes, s.cfg.AvatarSize, s.cfg.AvatarQuality)
		if compErr != nil {
			return nil, fmt.Errorf("compress avatar: %w", compErr)
		}

		// Cache compressed bytes
		if setErr := s.cache.SetAvatar(ctx, steamID64, compressed); setErr != nil {
			s.logger.Warn("failed to cache avatar", slog.String("steamid", steamID64), slog.String("error", setErr.Error()))
		}

		return compressed, nil
	})

	if err != nil {
		return nil, err
	}

	return val.([]byte), nil
}

func normalizeImageURL(rawURL string) string {
	// Discord media proxy: replace format=webp with format=png for maximum decoding compatibility
	if (strings.Contains(rawURL, "media.discordapp.net") || strings.Contains(rawURL, "cdn.discordapp.com")) && strings.Contains(rawURL, "format=webp") {
		return strings.Replace(rawURL, "format=webp", "format=png", 1)
	}
	return rawURL
}
