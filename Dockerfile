# ============================== BINARY BUILDER ==============================
FROM golang:1.24-alpine AS builder

WORKDIR /src

# Go 1.24.2 Toolchain
RUN go install golang.org/dl/go1.24.2@latest \
    && go1.24.2 download

ENV GOTOOLCHAIN=go1.24.2

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go test ./...

# Build both binaries
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o api .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o scraper ./cmd/scraper

# =============================== API IMAGE ===============================
FROM alpine:3.19 AS api

RUN apk add --no-cache ca-certificates

COPY --from=builder /src/api /usr/local/bin/api

USER nobody
ENTRYPOINT ["/usr/local/bin/api"]

# ============================= SCRAPER IMAGE =============================
FROM alpine:3.19 AS scraper

RUN apk add --no-cache ca-certificates

COPY --from=builder /src/scraper /usr/local/bin/scraper

USER nobody
ENTRYPOINT ["/usr/local/bin/scraper"]

