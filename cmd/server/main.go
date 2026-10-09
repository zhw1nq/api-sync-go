package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"api-sync-go/internal/cache"
	"api-sync-go/internal/config"
	"api-sync-go/internal/database"
	"api-sync-go/internal/handler"
	"api-sync-go/internal/legacy"
	"api-sync-go/internal/middleware"
	"api-sync-go/internal/service"
)

func main() {
	var port string
	flag.StringVar(&port, "port", "8080", "server port to listen on (e.g. -port 8080)")
	flag.StringVar(&port, "p", "8080", "server port (shorthand)")
	flag.Parse()

	// Check positional argument if provided (e.g. ./api-sync-go 8080)
	if flag.NArg() > 0 && flag.Arg(0) != "" {
		port = flag.Arg(0)
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Fallback to PORT from environment or .env if port flag is default
	if (port == "8080" || port == ":8080") && os.Getenv("PORT") != "" {
		port = os.Getenv("PORT")
	}

	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	// Dynamic log level based on config
	logLevel := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	// Connect MariaDB
	db, err := database.Connect(cfg)
	if err != nil {
		logger.Error("failed to connect database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("database connected")

	// Connect Redis
	rc, err := cache.Connect(cfg)
	if err != nil {
		logger.Error("failed to connect redis", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer rc.Close()
	logger.Info("redis connected")

	// Init services
	var userProvider service.UserProvider
	if cfg.LegacyMode {
		logger.Info("running in LEGACY mode (reading users/user_steam_profiles/user_discord_profiles)")
		userProvider = legacy.NewUserService(db, rc, cfg, logger)
	} else {
		logger.Info("running in STANDARD mode (reading user/steam_profile)")
		userProvider = service.NewUserService(db, rc, cfg, logger)
	}
	imageService := service.NewImageService(rc, cfg, logger)

	// Init handlers
	syncHandler := handler.NewSyncHandler(userProvider, logger)
	avatarHandler := handler.NewAvatarHandler(userProvider, imageService, logger)
	healthHandler := handler.NewHealthHandler(db, rc, logger)

	// Setup API router (protected by RateLimit and APIKeyAuth)
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /api/sync/{steamid}", syncHandler.GetUser)
	apiMux.HandleFunc("GET /api/sync/{steamid}/avatar.jpg", avatarHandler.GetAvatar)

	var protectedAPI http.Handler = apiMux
	protectedAPI = middleware.RateLimit(rc)(protectedAPI)
	protectedAPI = middleware.APIKeyAuth(cfg.APIKeys)(protectedAPI)

	// Root router: /health is unauthenticated for monitoring probes
	rootMux := http.NewServeMux()
	rootMux.HandleFunc("GET /health", healthHandler.Check)
	rootMux.Handle("/api/sync/", protectedAPI)

	// Global middleware chain: Logging -> CORS -> StripTrailingSlash -> Router
	var h http.Handler = rootMux
	h = stripTrailingSlash(h)
	h = middleware.CORS(cfg.AllowedDomains)(h)
	h = middleware.Logging(logger)(h)

	// HTTP server with graceful shutdown and Slowloris mitigation
	server := &http.Server{
		Addr:              port,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info("server starting", slog.String("addr", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("server stopped")
}

// stripTrailingSlash removes trailing slash from path (e.g. /api/sync/123/ -> /api/sync/123) to prevent 404
func stripTrailingSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) > 1 && strings.HasSuffix(r.URL.Path, "/") {
			r.URL.Path = strings.TrimSuffix(r.URL.Path, "/")
			if r.URL.RawPath != "" {
				r.URL.RawPath = strings.TrimSuffix(r.URL.RawPath, "/")
			}
		}
		next.ServeHTTP(w, r)
	})
}
