# Phase 3.01 Summary

**Completed:** 2026-03-07
**Plan:** Prometheus + snmp_exporter Docker infra, metrics domain types, and Go Prometheus client

## What Changed

- Added a dev metrics stack in `docker-compose.yml` with `prometheus` and `snmp-exporter`, including healthchecks and simulator dependencies.
- Added `docker/prometheus/prometheus.yml` so Prometheus scrapes the three simulator IPs through `snmp-exporter` using relabeled `/snmp` targets.
- Added `docker/prometheus/snmp.yml` with an `if_mib` module that exposes system, interface, CPU, memory, and sensor metrics needed for phase 3.
- Added `internal/domain/metrics.go` with `DeviceMetrics`, `LinkMetrics`, `AlertState`, and `AlertStatus`.
- Added `internal/metrics/prometheus.go` with a lightweight `PromClient` that queries Prometheus over HTTP and parses device metrics, link metrics, and alerts.
- Added `internal/metrics/prometheus_test.go` covering PromQL query construction, response parsing, empty-result handling, throughput/utilization parsing, and alert parsing.
- Updated `Makefile` dev output to include the Prometheus and snmp_exporter endpoints.

## Verification

- `docker compose --profile dev config --quiet`
- `docker compose --profile test run --rm --no-deps backend gofmt -w internal/domain/metrics.go internal/metrics/prometheus.go internal/metrics/prometheus_test.go`
- `docker compose --profile test run --rm --no-deps backend go test ./internal/metrics/... -v -count=1`
- `docker compose --profile test run --rm --no-deps backend go build -buildvcs=false ./...`
- `docker compose --profile dev up -d snmp-router snmp-switch snmp-ap snmp-exporter prometheus`
- `docker compose ps`
- `curl -sf http://localhost:9090/-/healthy`
- `curl -sf 'http://localhost:9090/api/v1/query?query=sysUpTime'`
- `curl -sf 'http://localhost:9090/api/v1/query?query=avg%20by%20(instance)%20(hrProcessorLoad)'`
- `curl -sf 'http://localhost:9090/api/v1/query?query=100%20*%20(hrStorageUsed%7BhrStorageDescr%3D%22Physical%20memory%22%2Cinstance%3D~%22%5E(%3F%3A172%5C%5C.28%5C%5C.10%5C%5C.10%7C172%5C%5C.28%5C%5C.10%5C%5C.11%7C172%5C%5C.28%5C%5C.10%5C%5C.12)%24%22%7D%20%2F%20hrStorageSize%7BhrStorageDescr%3D%22Physical%20memory%22%2Cinstance%3D~%22%5E(%3F%3A172%5C%5C.28%5C%5C.10%5C%5C.10%7C172%5C%5C.28%5C%5C.10%5C%5C.11%7C172%5C%5C.28%5C%5C.10%5C%5C.12)%24%22%7D)'`
- `curl -sf 'http://localhost:9090/api/v1/query?query=rate(ifHCOutOctets%7Binstance%3D~%22%5E(%3F%3A172%5C%5C.28%5C%5C.10%5C%5C.10%7C172%5C%5C.28%5C%5C.10%5C%5C.11%7C172%5C%5C.28%5C%5C.10%5C%5C.12)%24%22%7D%5B5m%5D)%20*%208'`
- `curl -sf 'http://localhost:9090/api/v1/query?query=max%20by%20(instance)%20(entPhySensorValue%7BentPhySensorType%3D%228%22%2Cinstance%3D~%22%5E(%3F%3A172%5C%5C.28%5C%5C.10%5C%5C.10%7C172%5C%5C.28%5C%5C.10%5C%5C.11%7C172%5C%5C.28%5C%5C.10%5C%5C.12)%24%22%7D)'`

## Notes

- The dev simulators expose CPU, memory, uptime, and interface counters through Prometheus. They do not currently expose temperature sensor series, so the temperature query correctly returns an empty result.
- `go build` required `-buildvcs=false` in the backend container because VCS stamping was unavailable in that environment.

## Outcome

- Phase 3 now has a working Prometheus ingestion path in dev and a tested backend client for querying the metrics needed by the real-time pipeline.
- Plan `03-02` can build on this by wiring the Prometheus client into a WebSocket-backed metrics collector.
