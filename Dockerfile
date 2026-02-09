# ============================== BINARY BUILDER ==============================
FROM golang:1.24-alpine AS builder

WORKDIR /src

# Download dependencies first (better caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .

# Build both binaries
# -ldflags="-s -w" strips debug information for smaller binaries
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o api .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o scraper ./cmd/scraper

# =============================== API IMAGE ===============================
FROM alpine:3.19 AS api

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /src/api /usr/local/bin/api

USER nobody
ENTRYPOINT ["/usr/local/bin/api"]

# ============================= SCRAPER IMAGE =============================
FROM alpine:3.19 AS scraper

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /src/scraper /usr/local/bin/scraper

USER nobody
ENTRYPOINT ["/usr/local/bin/scraper"]
