# api-sync-go

Go API service for syncing user data from 7gc-portal's MariaDB database. Protected by `X-API-KEY` header authentication.

## Endpoints

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/sync/{steamid}` | Returns user `name` + `image` URL as JSON (Singleflight deduplicated) |
| `GET` | `/api/sync/{steamid}/avatar.jpg` | Proxies and compresses avatar to 184×184 JPEG with `ETag` / `304 Not Modified` support |
| `GET` | `/health` | Public health check for DB + Redis (unauthenticated) |

## Features & Optimizations

- **Singleflight Deduplication**: Prevents cache stampede / thundering herd across concurrent requests for identical SteamIDs.
- **Embedded Static Assets**: Default avatar is embedded directly into binary (`//go:embed`), removing disk I/O at runtime.
- **High-Speed Avatar Compression**: Powered by CatmullRom resampling, zero-alloc buffer pooling (`sync.Pool`), and single-pass JPEG tuning.
- **Network Caching**: HTTP `ETag` + `304 Not Modified` on avatars saves 100% bandwidth for repeat clients.
- **Atomic Rate Limiter**: Redis rate limiter powered by atomic Lua scripts.
- **Zero-Allocation Validation**: Regex-free SteamID validation.
- **Fast CORS Engine**: Pre-compiled $O(1)$ domain/origin lookup maps.
- **Hardened Docker Image**: Non-root container (`appuser`) with automated health checks and Slowloris mitigation (`ReadHeaderTimeout`).

## Setup

```bash
# Copy env template
cp .env.example .env

# Edit .env with your values
vim .env

# Install dependencies & test build
go build ./cmd/server

# Run with default port (:8080)
go run ./cmd/server

# Run with custom port via argument
go run ./cmd/server -port 8080
# or shorthand
go run ./cmd/server -p 8080
```

## Usage

```bash
# Get user info
curl -H "X-API-KEY: your-key" http://localhost:8080/api/sync/76561198012345678

# Get compressed avatar (supports conditional HTTP requests)
curl -H "X-API-KEY: your-key" http://localhost:8080/api/sync/76561198012345678/avatar.jpg -o avatar.jpg

# Health check (unauthenticated)
curl http://localhost:8080/health
```

## Docker

```bash
docker build -t api-sync-go .
docker run -p 8080:8080 --env-file .env api-sync-go -port 8080
```

## Environment Variables

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `API_KEYS` | Yes | — | Comma-separated API keys |
| `ALLOWED_DOMAIN` | Yes | — | Tên domain được phép gọi API (ví dụ: `localhost:3000` hoặc `domain.com`, NO fallback) |
| `DATABASE_URL` | Yes | — | MariaDB connection (`mysql://user:pass@host:port/db`) |
| `REDIS_URL` | No | `redis://localhost:6379` | Redis connection URL (nếu có pass có `@` thì mã hóa thành `%40`) |
| `USER_CACHE_TTL` | No | `5m` | User info cache duration |
| `NEGATIVE_CACHE_TTL` | No | `1m` | Cache duration cho user không tồn tại |
| `AVATAR_CACHE_TTL` | No | `30m` | Avatar bytes cache duration |
| `AVATAR_SIZE` | No | `184` | Avatar output dimensions (px) |
| `AVATAR_QUALITY` | No | `80` | Initial JPEG quality |
| `DB_MAX_OPEN_CONNS` | No | `25` | Tối đa kết nối mở tới MariaDB |
| `DB_MAX_IDLE_CONNS` | No | `10` | Số kết nối rảnh rỗi giữ trong pool |
| `DB_CONN_MAX_LIFETIME` | No | `0` | Thời gian sống tối đa của một kết nối DB (`0` = vĩnh viễn, không bao giờ ngắt) |
| `DB_CONN_MAX_IDLE_TIME` | No | `0` | Thời gian rảnh rỗi trước khi đóng kết nối (`0` = duy trì vĩnh viễn, không bao giờ ngắt do idle) |
| `DB_QUERY_TIMEOUT` | No | `5s` | Timeout tối đa cho mỗi câu query DB |
| `DB_KEEPALIVE_INTERVAL` | No | `20s` | Chu kỳ ping ngầm giữ kết nối DB luôn ấm và sống qua firewall |
| `LOG_LEVEL` | No | `info` | Cấp độ ghi log (`debug`, `info`, `warn`, `error`) |
| `LEGACY_MODE` | No | `false` | Bật chế độ đọc DB cũ (`users`, `user_steam_profiles`, `user_discord_profiles`) |

## License

Private — 7gc internal use only.
