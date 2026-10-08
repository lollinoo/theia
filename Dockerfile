# =============================================================================
# MikroTik Theia - Multi-stage Dockerfile
# =============================================================================
# Stages:
#   dev        - Development with Air hot-reload (used by docker-compose)
#   builder    - Compiles production binary
#   production - Minimal runtime image
# =============================================================================

# ---------------------------------------------------------------------------
# Stage: postgres-tools — PostgreSQL 18 client binaries
# ---------------------------------------------------------------------------
FROM postgres:18-bookworm AS postgres-tools

# Normalize the libpq location for both AMD64 and ARM64 development images.
RUN mkdir -p /theia-libs && cp -L "$(pg_config --libdir)/libpq.so.5" /theia-libs/libpq.so.5

# ---------------------------------------------------------------------------
# Stage: dev — Development with Air hot-reload
# ---------------------------------------------------------------------------
FROM golang:1.27.1-bookworm AS dev

RUN apt-get update && \
    apt-get install -y --no-install-recommends curl libpq5 libreadline8 && \
    rm -rf /var/lib/apt/lists/*

COPY --from=postgres-tools /usr/lib/postgresql/18/bin/pg_dump /usr/local/bin/pg_dump
COPY --from=postgres-tools /usr/lib/postgresql/18/bin/pg_restore /usr/local/bin/pg_restore
COPY --from=postgres-tools /usr/lib/postgresql/18/bin/psql /usr/local/bin/psql
COPY --from=postgres-tools /theia-libs/libpq.so.5 /usr/local/lib/libpq.so.5
RUN ldconfig

# Install dev/test tooling.
RUN go install github.com/air-verse/air@v1.67.4 && \
    go install golang.org/x/vuln/cmd/govulncheck@latest

ENV CGO_ENABLED=0

WORKDIR /app

RUN git config --global --add safe.directory /app

# Cache dependencies
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true

EXPOSE 8080

CMD ["air", "-c", ".air.toml"]

# ---------------------------------------------------------------------------
# Stage: builder — Compile production binary
# ---------------------------------------------------------------------------
FROM golang:1.27.1-bookworm AS builder

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

ENV CGO_ENABLED=0

WORKDIR /build

COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true

COPY . .

RUN go build -o /app/theia ./cmd/theia/

# ---------------------------------------------------------------------------
# Stage: production — Minimal runtime image
# ---------------------------------------------------------------------------
FROM postgres:18-bookworm AS production

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates libc6 curl libpq5 libreadline8 && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/theia /usr/local/bin/theia
# The PostgreSQL tools and server in this base image verify every managed backup
# in an isolated local cluster; the live database stays in its own service.

RUN mkdir -p /data

EXPOSE 8080

CMD ["theia"]
