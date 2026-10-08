FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /api-sync-go ./cmd/server

# ---

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder /api-sync-go /api-sync-go

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

CMD ["/api-sync-go"]
