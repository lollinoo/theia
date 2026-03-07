# Phase 3.02 Summary

**Completed:** 2026-03-07
**Plan:** WebSocket hub, handler, metrics collector, and main.go wiring

## What Changed

- Added `internal/ws/messages.go` with the WebSocket message envelope, snapshot payload, DTO types, and domain-to-DTO conversion helpers.
- Added `internal/ws/hub.go` with client registration, unregister, broadcast fanout, and ping/pong keepalive handling.
- Added `internal/ws/handler.go` to upgrade `/api/v1/ws` requests, register clients, and send the current snapshot on connect.
- Added `internal/worker/metrics_collector.go` to query Prometheus on the polling interval, map metrics and alerts onto device/link IDs, cache the latest snapshot, and broadcast full-state `snapshot` messages.
- Updated `cmd/theia/main.go` to initialize the default Prometheus URL, create the hub and collector, start them with the existing app lifecycle, and stop the collector during shutdown.
- Updated `internal/api/router.go` to expose `/api/v1/ws` and bypass the normal JSON/logger middleware chain for WebSocket upgrades.
- Extended alert parsing so Prometheus alert instance labels can be mapped back to device IDs during collection.

## Verification

- `docker compose --profile test run --rm --no-deps backend go build ./internal/ws/...`
- `docker compose --profile test run --rm --no-deps backend go build -buildvcs=false ./cmd/theia/...`
- `docker compose --profile test run --rm --no-deps backend go build -buildvcs=false ./...`
- `docker compose --profile test run --rm --no-deps backend go test ./internal/metrics/... -v -count=1`
- `docker compose --profile dev up -d --build backend`
- `curl -sf http://localhost:8080/api/v1/health`
- `curl -sf http://localhost:8080/api/v1/settings`
- `docker compose exec -T frontend node -e "... WebSocket connect to ws://backend:8080/api/v1/ws ..."` returning an initial `snapshot`
- Runtime snapshot inspection confirmed non-empty live data: 5 devices in the snapshot, CPU for 3 devices, memory for 3 devices, and throughput data on 4 linked interfaces

## Notes

- The WebSocket endpoint is intentionally routed outside the JSON/logger middleware chain because the wrapped `ResponseWriter` does not support HTTP hijacking required by WebSocket upgrade.
- The metrics collector now sends an initial snapshot immediately on startup and each client receives the cached snapshot immediately on connect.
- Prometheus alerts are still empty in the current dev stack because no alerting rules are configured yet; the alert transport path is in place.

## Outcome

- The backend now exposes `/api/v1/ws`, maintains live client connections, and pushes full snapshot updates sourced from Prometheus on the polling interval.
- Phase `03-03` can focus entirely on frontend WebSocket consumption and rendering because the backend real-time transport path is now working.
