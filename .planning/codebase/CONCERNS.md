# Codebase Concerns

**Analysis Date:** 2026-04-23

## Tech Debt

**Monolithic HTTP router and manual path parsing:**
- Issue: `internal/api/router.go` registers all API routes in one `NewRouter` function and dispatches nested resources with `strings.HasSuffix`, `strings.Contains`, and custom ID extraction instead of route parameters or per-domain subrouters.
- Files: `internal/api/router.go`, `internal/api/device_handler.go`, `internal/api/device_credential_profile_handler.go`, `internal/api/bridge_handler.go`
- Impact: Route precedence is fragile for new nested endpoints; adding paths under `/api/v1/devices/`, `/api/v1/instance-backups/`, or `/api/v1/bridge/` can silently hit the wrong handler. Middleware bypasses are also hard-coded per path in `internal/api/router.go`.
- Fix approach: Introduce a small router abstraction or standard-library `ServeMux` patterns with explicit path segment parsing helpers. Move each domain's route registration into `internal/api/*_routes.go` and add route-table tests for every endpoint/method pair.

**Large UI components concentrate unrelated concerns:**
- Issue: `frontend/src/components/DeviceConfigPanel.tsx` is 1391 lines and owns SNMP settings, Prometheus settings, Grafana settings, area assignment, credential profiles, WinBox profile state, topology discovery, validation, timers, and persistence calls in one component.
- Files: `frontend/src/components/DeviceConfigPanel.tsx`, `frontend/src/components/InstanceBackupManager.tsx`, `frontend/src/components/SettingsPanel.tsx`, `frontend/src/components/canvas/useCanvasData.ts`
- Impact: Feature changes in one settings section risk regressions in unrelated UI state. Effects in `frontend/src/components/DeviceConfigPanel.tsx` use broad local state and suppressed non-fatal errors, making loading and failure states difficult to reason about.
- Fix approach: Split `frontend/src/components/DeviceConfigPanel.tsx` into section components and hooks such as `useDeviceCredentialAssignments`, `useDevicePollingOverride`, and `useTopologyDiscoveryAction`. Keep submit payload construction in `frontend/src/components/forms/deviceFormSubmitters.ts`.

**TypeScript API client is a broad mutable service module:**
- Issue: `frontend/src/api/client.ts` is 817 lines and mixes low-level fetch wrappers, response parsing, error normalization, and every resource operation.
- Files: `frontend/src/api/client.ts`, `frontend/src/types/api.ts`, `frontend/src/api/errors.ts`
- Impact: Adding an endpoint increases merge conflicts and makes it easy to miss consistent `ValidationError`/`ServerError` handling. Different helpers (`requestJSON`, `requestJSONWithBody`) expose different error behavior.
- Fix approach: Keep shared fetch/error helpers in `frontend/src/api/http.ts` and split resources into `frontend/src/api/devices.ts`, `frontend/src/api/settings.ts`, `frontend/src/api/backups.ts`, and `frontend/src/api/bridge.ts`.

**Backup manifest size metadata is stale inside archives:**
- Issue: `internal/service/instance_backup_service.go` writes `manifest.json` before calculating `TotalSizeBytes`; `CreateWithTrigger` updates `manifest.TotalSizeBytes` after `createArchive` returns, but the archived manifest already contains the old value.
- Files: `internal/service/instance_backup_service.go`
- Impact: Restored or audited archives report inaccurate `manifest.total_size_bytes`, which weakens integrity checks and can confuse backup inventory tooling.
- Fix approach: Precompute archive member sizes before writing `manifest.json`, or write the archive in two passes so `TotalSizeBytes` is final before `addBytesToTar` writes `manifest.json`.

## Known Bugs

**Backup manifest `total_size_bytes` remains `0` in generated archives:**
- Symptoms: Instance backup archives created by `CreateWithTrigger` include a `manifest.json` whose `total_size_bytes` field is initialized before archive contents are counted.
- Files: `internal/service/instance_backup_service.go`
- Trigger: Create an instance backup through `POST /api/v1/instance-backups`, then inspect `manifest.json` inside the `.tar.gz`.
- Workaround: Use the database record fields `size_bytes` and `sha256` returned by `internal/repository/sqlite/instance_backup_repo.go` for archive inventory until the manifest write order is fixed.

**Settings validation accepts unbounded numeric values:**
- Symptoms: `PUT /api/v1/settings/{key}` accepts any integer for worker pool size, SNMP timeout, retry count, retention count, and bridge port before downstream code applies partial defaults or uses the value directly.
- Files: `internal/api/settings_handler.go`, `internal/worker/poller.go`, `internal/pollingbudget/pollingbudget.go`, `internal/worker/backup_scheduler.go`, `internal/worker/device_backup_scheduler.go`
- Trigger: Submit very large values for `snmp_worker_pool_size`, `snmp_worker_pool_performance_size`, `snmp_timeout_seconds`, `snmp_retries`, `device_backup_retention_count`, or `bridge_port` through `PUT /api/v1/settings/{key}`.
- Workaround: Keep UI inputs constrained in `frontend/src/components/SettingsPanel.tsx` and validate operational settings before deployment; backend-side min/max validation is the durable fix.

## Security Considerations

**No application authentication or authorization boundary:**
- Risk: Every registered API endpoint is reachable by any network client that can connect to `cfg.ListenAddr`, including device CRUD, credential profile management, backup creation/deletion/download, restore upload, vendor config updates, settings updates, bridge token creation, and WebSocket snapshots.
- Files: `cmd/theia/runtime_bootstrap.go`, `internal/api/router.go`, `internal/api/middleware.go`, `internal/ws/handler.go`
- Current mitigation: SQLite is gated for small-install deployments in `cmd/theia/runtime_bootstrap.go`, secrets require `THEIA_ENCRYPTION_KEY` in `internal/crypto/encrypt.go`, and directories are prepared with private file modes in `cmd/theia/runtime_bootstrap.go`.
- Recommendations: Add an auth middleware before `CORS`, `RequestLogger`, and route handlers in `internal/api/router.go`. Require authorization for mutating endpoints and sensitive reads. Bind production `listen_addr` to a private interface or reverse proxy until auth exists.

**Permissive CORS and WebSocket origins:**
- Risk: `internal/api/middleware.go` sets `Access-Control-Allow-Origin: *`, and `internal/ws/handler.go` accepts all WebSocket origins. If the service is reachable from a browser, a malicious site can read unauthenticated API responses or subscribe to live topology data.
- Files: `internal/api/middleware.go`, `internal/ws/handler.go`, `cmd/theia/runtime_bootstrap.go`
- Current mitigation: Not detected at the application layer; deployment network isolation is the effective control.
- Recommendations: Configure allowed origins from `config.yaml` or environment, reject unknown `Origin` in `internal/ws/handler.go`, and keep CORS disabled or same-origin by default in production.

**Plaintext credential exposure endpoints:**
- Risk: `GET /api/v1/devices/{id}/winbox-credentials` returns `ip`, `username`, and plaintext `password`; `POST /api/v1/bridge/token/{deviceId}` decrypts the same credential and encrypts it with a caller-supplied bridge secret. Without auth, any network client can obtain or wrap WinBox credentials.
- Files: `internal/api/device_credential_profile_handler.go`, `internal/api/bridge_handler.go`, `internal/service/backup_service.go`, `internal/api/router.go`
- Current mitigation: Credential profile responses omit `EncryptedSecret` in `internal/api/device_credential_profile_handler.go`, and stored SSH/SNMP secrets are encrypted via `internal/service/backup_service.go` and `internal/repository/sqlite/snmp_crypto.go`.
- Recommendations: Remove the plaintext credentials endpoint or restrict it to authenticated local bridge flows. Add short-lived signed tokens, request auditing, rate limits, and never expose decrypted passwords in JSON responses.

**Settings API can expose sensitive settings:**
- Risk: `SettingsRepo.GetAll` returns every key/value from `settings`, and `SettingsHandler.HandleGetAll` serializes that map directly. `SettingBridgeSecret` is a valid setting key, but `internal/repository/sqlite/settings_repo.go` stores settings as plaintext values.
- Files: `internal/api/settings_handler.go`, `internal/repository/sqlite/settings_repo.go`, `internal/domain/settings.go`
- Current mitigation: `SettingBridgeSecret` is not part of `DefaultSettings`, and bridge token flow in `internal/api/bridge_handler.go` expects the bridge secret per request.
- Recommendations: Maintain a redact/denylist for sensitive settings in `HandleGetAll` and `HandleGet`; encrypt sensitive settings at rest or move them out of the generic settings table.

## Performance Bottlenecks

**Unbounded SNMP goroutines during metrics snapshots:**
- Problem: `MetricsCollector.buildSnapshot` starts one goroutine per eligible device for SNMP metric polling and one goroutine per SNMP-sourced device for link counter polling, independent of configured worker-pool limits.
- Files: `internal/worker/metrics_collector.go`, `internal/worker/settings.go`, `internal/pollingbudget/pollingbudget.go`
- Cause: The worker-pool settings are used by polling orchestration (`internal/worker/poller.go` and `internal/pollingbudget/pollingbudget.go`) but not by the SNMP fallback/link sections in `internal/worker/metrics_collector.go`.
- Improvement path: Reuse `pollingbudget.Total` or volatility-class budgets in `MetricsCollector.buildSnapshot`; add a semaphore around SNMP fallback and link counter goroutines and test with many unreachable devices.

**Prometheus collection is fail-fast and partially serial:**
- Problem: `MetricsCollector.buildSnapshot` loops over vendor/label groups and stops remaining Prometheus queries after the first device metrics or link metrics error.
- Files: `internal/worker/metrics_collector.go`, `internal/metrics/prometheus.go`
- Cause: `promQueryErrors > 0` breaks subsequent group collection, so one broken vendor/label group can suppress data for healthy groups during the same snapshot.
- Improvement path: Query independent groups concurrently with per-group error isolation. Mark Prometheus health from aggregate results without discarding successful groups.

**Full snapshot cloning and hashing can become expensive:**
- Problem: Snapshot construction clones devices, builds full DTO maps, computes section hashes, and broadcasts snapshots/deltas on every collection interval.
- Files: `internal/worker/metrics_collector.go`, `internal/ws/messages.go`, `internal/ws/hub.go`, `frontend/src/hooks/useWebSocket.ts`
- Cause: Snapshot state is recalculated from full device/link lists each cycle, even when only a small subset of runtime metrics changes.
- Improvement path: Preserve per-device dirty state in `internal/worker/pipeline_runtime_state.go` and only recompute hash sections affected by changed devices, links, alerts, or settings.

## Fragile Areas

**Instance backup and restore paths:**
- Files: `internal/service/instance_backup_service.go`, `internal/service/restore_coordinator.go`, `internal/api/instance_backup_handler.go`, `internal/repository/sqlite/instance_backup_repo.go`
- Why fragile: Backup creation crosses live database access, external `pg_dump`/restore commands, tar/gzip archive construction, file permissions, key-hash checks, and pending restore state. Several non-fatal file walk errors are skipped or logged.
- Safe modification: Add end-to-end tests for archive manifest contents, sidecar hash verification, PostgreSQL restore, and partial device backup file inclusion before changing archive structure.
- Test coverage: Unit coverage exists in `internal/service/instance_backup_service_test.go`, `internal/service/postgres_instance_backup_test.go`, and `internal/api/instance_backup_handler_test.go`; browser-level restore/download coverage is not detected.

**Realtime pipeline and WebSocket state synchronization:**
- Files: `internal/worker/pipeline.go`, `internal/worker/pipeline_runtime_state.go`, `internal/worker/pipeline_snapshot_broadcaster.go`, `internal/ws/hub.go`, `frontend/src/hooks/useWebSocket.ts`, `frontend/e2e/realtime.spec.ts`
- Why fragile: Runtime state uses deltas, snapshot versions, topology notifications, reconnect behavior, and client-side resync handling. A mismatch in message shape or version semantics can create stale topology or metric displays.
- Safe modification: Preserve message contracts in `internal/ws/messages.go` and `frontend/src/types/metrics.ts`. Add tests that cover reconnect after missed deltas and mixed snapshot/delta streams.
- Test coverage: Unit/stress tests exist in `internal/ws/*_test.go`, `internal/worker/pipeline_*_test.go`, and `frontend/src/hooks/useWebSocket.test.ts`; E2E coverage is limited to `frontend/e2e/realtime.spec.ts`.

**SNMP discovery and vendor-specific metrics:**
- Files: `internal/snmp/discovery.go`, `internal/snmp/client.go`, `internal/collector/performance.go`, `internal/vendor/registry.go`, `vendors/default.yaml`, `vendors/mikrotik.yaml`
- Why fragile: Discovery depends on device-specific OIDs, LLDP/CDP availability, per-vendor YAML, SNMPv2/v3 credential behavior, and network timeouts.
- Safe modification: Add vendor fixture tests under `internal/collector/testdata/` and `internal/scalelab/testdata/` before changing OIDs or fallback rules.
- Test coverage: Unit tests exist in `internal/snmp/discovery_test.go`, `internal/snmp/client_test.go`, and `internal/collector/*_test.go`; real device integration coverage is not detected in the repository.

**Manual frontend runtime validation:**
- Files: `frontend/src/types/api.ts`, `frontend/src/types/metrics.ts`, `frontend/src/api/client.ts`
- Why fragile: Runtime parsers hand-check JSON:API and WebSocket payloads with many `throw new Error` branches. Backend and frontend schemas can drift without generated contracts.
- Safe modification: Treat `frontend/src/types/api.test.ts` and `frontend/src/types/metrics.test.ts` as contract tests; add fixtures for every backend DTO when adding fields.
- Test coverage: Parser unit tests exist, but no shared OpenAPI/JSON schema contract is detected.

## Scaling Limits

**SQLite is explicitly limited to small installations:**
- Current capacity: `cmd/theia/runtime_bootstrap.go` documents SQLite as suitable for up to 50 devices, one Theia process, and one active admin when `THEIA_ALLOW_SQLITE_SMALL_INSTALL=true` is set.
- Limit: SQLite is rejected by default; multiple processes or larger topologies require PostgreSQL.
- Scaling path: Use PostgreSQL via `THEIA_DB_DSN`, keep `db_driver` as `postgres`, and test migrations with `cmd/theia-db-migrate/main.go` and `internal/repository/sqlite/postgres_plan.go`.

**Polling and runtime collection scale with managed devices and links:**
- Current capacity: Default settings are 5 total SNMP workers (`internal/domain/settings.go`) and a 60-second polling interval.
- Limit: Large numbers of unreachable SNMP devices can consume goroutines and full timeout windows in `internal/worker/metrics_collector.go` and `internal/worker/poller.go`.
- Scaling path: Enforce upper bounds in `internal/api/settings_handler.go`, apply worker budgets to metrics collector SNMP fallback, and load-test with `cmd/theia-scale-lab/main.go` and `internal/scalelab/`.

## Dependencies at Risk

**Go toolchain version is ahead of common stable environments:**
- Risk: `go.mod` declares `go 1.24.0`, which can limit CI/developer compatibility if environments have older Go releases.
- Impact: Builds, tests, or module downloads fail before code runs.
- Migration plan: Keep CI pinned to the declared version in `.github/workflows/ci.yml`, or lower `go.mod` only after verifying all dependencies and language features.

**SQLite CGO dependency affects builds and deployment:**
- Risk: `github.com/mattn/go-sqlite3` requires CGO and platform-specific compiler support.
- Impact: Cross-compilation and minimal container images can fail or require larger build stages.
- Migration plan: Prefer PostgreSQL for production, keep SQLite for lab/small installs, and document CGO build requirements in `SETUP.md` and Docker build files.

## Missing Critical Features

**Application-level access control:**
- Problem: Authentication, authorization, sessions/tokens, CSRF protection, and role-based permissions are not detected in `internal/api/router.go`, `internal/api/middleware.go`, or frontend API calls.
- Blocks: Safe exposure beyond a trusted private network, multi-user administration, audit trails for credential access, and safe browser access from untrusted origins.

**Centralized API contract:**
- Problem: Backend DTOs are manually encoded in Go and manually parsed in TypeScript without a shared schema.
- Blocks: Automated client generation, reliable compatibility checks, and contract validation for WebSocket and JSON:API payloads.

## Test Coverage Gaps

**Security middleware and auth behavior:**
- What's not tested: Authentication/authorization behavior is not applicable because no auth middleware is present. CORS and WebSocket origin rejection tests are not detected.
- Files: `internal/api/middleware.go`, `internal/ws/handler.go`, `internal/api/router.go`
- Risk: Future auth work can leave bypass paths for downloads, restore upload, bridge token, or WebSocket routes because those paths bypass parts of the middleware chain in `internal/api/router.go`.
- Priority: High

**Backup archive manifest correctness:**
- What's not tested: Archived `manifest.json` final `total_size_bytes` value and consistency against archived members.
- Files: `internal/service/instance_backup_service.go`, `internal/service/instance_backup_service_test.go`
- Risk: Backups pass current tests while carrying inaccurate audit metadata.
- Priority: Medium

**Frontend E2E coverage for destructive flows:**
- What's not tested: Browser-level tests for device deletion, backup deletion, restore upload, settings mutation, credential profile changes, and WinBox credential launch flows are not detected.
- Files: `frontend/e2e/realtime.spec.ts`, `frontend/src/components/DeviceConfigPanel.tsx`, `frontend/src/components/InstanceBackupManager.tsx`, `frontend/src/components/CredentialProfileManager.tsx`
- Risk: UI regressions in high-impact flows can ship despite unit tests.
- Priority: High

**Large-topology/load behavior:**
- What's not tested: Repository includes scale-lab fixtures and commands, but automated CI load thresholds for thousands of devices/links or many unreachable SNMP targets are not detected.
- Files: `cmd/theia-scale-lab/main.go`, `internal/scalelab/`, `internal/worker/metrics_collector.go`, `internal/scheduler/scheduler.go`
- Risk: Performance regressions can appear only in brownfield deployments with large WISP-style topologies.
- Priority: Medium

---

*Concerns audit: 2026-04-23*
