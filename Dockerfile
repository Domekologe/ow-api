# ============================== BINARY BUILDER ==============================
FROM golang:1.22-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go test ./...
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server

# =============================== FINAL IMAGE ===============================
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /src/server /usr/local/bin/server

USER nobody
ENTRYPOINT ["/usr/local/bin/server"]
