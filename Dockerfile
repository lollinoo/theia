# =============================================================================
# MikroTik Theia - Multi-stage Dockerfile
# =============================================================================
# Stages:
#   dev        - Development with Air hot-reload (used by docker-compose)
#   builder    - Compiles production binary with CGO
#   production - Minimal runtime image
# =============================================================================

# ---------------------------------------------------------------------------
# Stage: dev — Development with Air hot-reload
# ---------------------------------------------------------------------------
# Debian-based (not Alpine) because CGO + mattn/go-sqlite3 requires glibc
FROM golang:1.24-bookworm AS dev

RUN apt-get update && \
    apt-get install -y --no-install-recommends gcc libc6-dev curl && \
    rm -rf /var/lib/apt/lists/*

# Install Air for hot-reload (pinned version compatible with Go 1.24)
RUN go install github.com/air-verse/air@v1.61.5

ENV CGO_ENABLED=1

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true

EXPOSE 8080

CMD ["air", "-c", ".air.toml"]

# ---------------------------------------------------------------------------
# Stage: builder — Compile production binary
# ---------------------------------------------------------------------------
FROM golang:1.24-bookworm AS builder

RUN apt-get update && \
    apt-get install -y --no-install-recommends gcc libc6-dev && \
    rm -rf /var/lib/apt/lists/*

ENV CGO_ENABLED=1

WORKDIR /build

COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true

COPY . .

RUN go build -o /app/theia ./cmd/theia/

# ---------------------------------------------------------------------------
# Stage: production — Minimal runtime image
# ---------------------------------------------------------------------------
FROM debian:bookworm-slim AS production

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates libc6 curl && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/theia /usr/local/bin/theia

RUN mkdir -p /data

EXPOSE 8080

CMD ["theia"]
