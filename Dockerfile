# =============================================================================
# MikroTik Theia - Multi-stage Dockerfile
# =============================================================================
# Stages:
#   dev        - Development with Air hot-reload (used by docker-compose)
#   builder    - Compiles production binary with CGO
#   production - Minimal runtime image
# =============================================================================

# ---------------------------------------------------------------------------
# Stage: postgres-tools — PostgreSQL 17 client binaries
# ---------------------------------------------------------------------------
FROM postgres:17-bookworm AS postgres-tools

# ---------------------------------------------------------------------------
# Stage: dev — Development with Air hot-reload
# ---------------------------------------------------------------------------
# Debian-based (not Alpine) because CGO + mattn/go-sqlite3 requires glibc
FROM golang:1.24-bookworm AS dev

RUN apt-get update && \
    apt-get install -y --no-install-recommends gcc libc6-dev curl libpq5 libreadline8 && \
    rm -rf /var/lib/apt/lists/*

COPY --from=postgres-tools /usr/lib/postgresql/17/bin/pg_dump /usr/local/bin/pg_dump
COPY --from=postgres-tools /usr/lib/postgresql/17/bin/pg_restore /usr/local/bin/pg_restore
COPY --from=postgres-tools /usr/lib/postgresql/17/bin/psql /usr/local/bin/psql

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

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
RUN go build -ldflags "\
    -X github.com/lollinoo/theia/internal/version.Version=${VERSION} \
    -X github.com/lollinoo/theia/internal/version.GitCommit=${GIT_COMMIT} \
    -X github.com/lollinoo/theia/internal/version.BuildDate=${BUILD_DATE}" \
    -o /app/theia ./cmd/theia/

# ---------------------------------------------------------------------------
# Stage: production — Minimal runtime image
# ---------------------------------------------------------------------------
FROM debian:bookworm-slim AS production

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates libc6 curl libpq5 libreadline8 && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/theia /usr/local/bin/theia
COPY --from=postgres-tools /usr/lib/postgresql/17/bin/pg_dump /usr/local/bin/pg_dump
COPY --from=postgres-tools /usr/lib/postgresql/17/bin/pg_restore /usr/local/bin/pg_restore
COPY --from=postgres-tools /usr/lib/postgresql/17/bin/psql /usr/local/bin/psql

RUN mkdir -p /data

EXPOSE 8080

CMD ["theia"]
