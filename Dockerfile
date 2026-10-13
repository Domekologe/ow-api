# syntax=docker/dockerfile:1

# ============================== BINARY BUILDER ==============================
FROM golang:1.25-alpine AS builder

WORKDIR /src

# Download dependencies first for better layer caching.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# Build both binaries. -ldflags="-s -w" strips debug info for smaller binaries.
# The build cache mount makes rebuilds substantially faster.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api . && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/scraper ./cmd/scraper

# =============================== API IMAGE ===============================
FROM alpine:3.22 AS api

RUN apk add --no-cache ca-certificates tzdata wget && \
    mkdir -p /var/lib/ow-api && chown nobody:nobody /var/lib/ow-api

COPY --from=builder /out/api /usr/local/bin/api

USER nobody
WORKDIR /var/lib/ow-api
ENV DATA_DIR=/var/lib/ow-api
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:8080/healthcheck || exit 1

ENTRYPOINT ["/usr/local/bin/api"]

# ============================= SCRAPER IMAGE =============================
FROM alpine:3.22 AS scraper

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/scraper /usr/local/bin/scraper

USER nobody
ENTRYPOINT ["/usr/local/bin/scraper"]
