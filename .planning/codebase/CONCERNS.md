# Codebase Concerns

**Analysis Date:** 2026-04-19

## Tech Debt

**Missing access-control boundary across the API surface:**
- Issue: The HTTP stack applies `CORS -> Logger -> MaxBodySize -> JSONContentType` but no authentication or authorization middleware, and CORS is wildcarded.
- Files: `internal/api/middleware.go`, `internal/api/router.go`, `internal/ws/handler.go`, `internal/api/device_credential_profile_handler.go`, `internal/api/bridge_handler.go`
- Impact: Any reachable client can call device, backup, settings, bridge-token, and WinBox credential endpoints; WebSocket connections are also accepted from any origin.
- Fix approach: Add an authn/authz middleware before business handlers, tighten CORS to trusted origins, and reject cross-origin WebSocket upgrades in `internal/ws/handler.go`.

**Secrets are handled in application code instead of a dedicated secret boundary:**
- Issue: Runtime settings, SNMP profile secrets, and WinBox credentials move through normal API handlers and frontend state instead of staying server-side.
- Files: `internal/api/settings_handler.go`, `internal/repository/sqlite/settings_repo.go`, `internal/api/snmp_profile_handler.go`, `internal/api/device_credential_profile_handler.go`, `frontend/src/components/SettingsPanel.tsx`, `frontend/src/components/Canvas.tsx`, `frontend/src/types/api.ts`
- Impact: Sensitive values are easier to leak through logs, browser memory, screenshots, support dumps, and future feature work.
- Fix approach: Stop returning secrets from read APIs, encrypt sensitive settings at rest, and restrict WinBox launches to token-based flows handled entirely server-side.

**Large multi-responsibility files increase change risk:**
- Issue: Core orchestration and UI files are very large and combine data fetching, validation, state transitions, and rendering.
- Files: `frontend/src/components/DeviceConfigPanel.tsx` (~1243 lines), `frontend/src/components/canvas/useCanvasData.ts` (~1030 lines), `internal/service/device_service.go` (~1151 lines), `internal/worker/pipeline.go` (~1204 lines), `cmd/theia/main.go` (~820 lines)
- Impact: Small changes have broad blast radius, code review is harder, and regression risk rises because unrelated concerns live together.
- Fix approach: Split handlers/hooks/services by capability, move validation and mapping into focused helpers, and keep startup/restore logic out of `cmd/theia/main.go`.

## Known Bugs

**`bridge_secret` is stored and returned in plaintext despite the domain comment claiming otherwise:**
- Symptoms: `GET /api/v1/settings` and `GET /api/v1/settings/bridge_secret` return the live bridge key, and the frontend loads it into state.
- Files: `internal/domain/settings.go`, `internal/repository/sqlite/settings_repo.go`, `internal/api/settings_handler.go`, `frontend/src/components/SettingsPanel.tsx`, `frontend/src/components/Canvas.tsx`
- Trigger: Any caller that can reach the settings endpoints.
- Workaround: No safe built-in workaround; avoid populating `bridge_secret` in shared or untrusted environments.

**WinBox password retrieval bypasses the newer token flow:**
- Symptoms: `GET /api/v1/devices/{id}/winbox-credentials` returns decrypted `ip`, `username`, and `password` directly to the caller.
- Files: `internal/api/device_credential_profile_handler.go`, `frontend/src/api/client.ts`, `frontend/src/types/api.ts`
- Trigger: Any caller that hits the WinBox credentials endpoint for a device with a designated profile.
- Workaround: Prefer `POST /api/v1/bridge/token/{deviceId}` from `internal/api/bridge_handler.go` and avoid using the plaintext credential endpoint.

## Security Considerations

**Wildcard CORS plus unauthenticated endpoints:**
- Risk: `Access-Control-Allow-Origin: *` in `internal/api/middleware.go` exposes every API route to any browser origin, and `internal/ws/handler.go` accepts every WebSocket origin with `CheckOrigin: return true`.
- Files: `internal/api/middleware.go`, `internal/api/router.go`, `internal/ws/handler.go`
- Current mitigation: Request logging and a 1 MB body limit for the default middleware path.
- Recommendations: Require authentication, scope CORS to trusted origins, and enforce explicit origin checks for WebSockets.

**Sensitive values are exposed through read APIs:**
- Risk: SNMP communities, SNMPv3 auth/priv secrets, bridge secrets, and WinBox passwords are all retrievable through normal application routes.
- Files: `internal/api/snmp_profile_handler.go`, `internal/api/settings_handler.go`, `internal/api/device_credential_profile_handler.go`, `frontend/src/types/api.ts`
- Current mitigation: `internal/api/credential_profile_handler.go` omits encrypted SSH secrets from responses.
- Recommendations: Redact secret fields from all read responses, rotate any already-shared secrets, and move secret access behind one-time or write-only workflows.

**Weak secret-key derivation for stored encryption material:**
- Risk: `internal/crypto/encrypt.go` derives the AES key from `THEIA_ENCRYPTION_KEY` with a single SHA-256 hash, which provides no work factor against weak passphrases.
- Files: `internal/crypto/encrypt.go`
- Current mitigation: AES-GCM is used once a key exists.
- Recommendations: Require a high-entropy raw key or switch to a password KDF such as Argon2id/scrypt with parameters stored alongside ciphertext.

**Backup and restore artifacts are broadly readable on disk:**
- Risk: App data directories are created with `0755`, backup exports and restore markers are written with `0644`, and instance backup artifacts inherit default file modes.
- Files: `cmd/theia/main.go`, `internal/service/backup_service.go`, `internal/service/instance_backup_service.go`
- Current mitigation: `cmd/winbox-bridge/config.go` correctly uses `0700`/`0600`, but the main app paths do not.
- Recommendations: Create backup/data directories with owner-only permissions, write sensitive artifacts as `0600`, and audit existing on-disk files for exposure.

## Performance Bottlenecks

**WebSocket broadcast backpressure blocks producers instead of shedding load:**
- Problem: When `Hub.broadcast` is full, `Broadcast` records overflow and then performs a blocking send anyway.
- Files: `internal/ws/hub.go`, `internal/ws/hub_test.go`, `internal/worker/pipeline.go`
- Cause: The fallback path at `internal/ws/hub.go` lines 92-100 waits for channel space rather than dropping/coalescing the message immediately.
- Improvement path: Replace the blocking fallback with lossy/coalesced delivery, enqueue only resync markers under pressure, and move heavy producers off the hot path.

**SQLite remains the default persistence bottleneck:**
- Problem: The default database path is SQLite with a `5000ms` busy timeout, only `3` write retries, and a max pool capped at `16` connections.
- Files: `cmd/theia/main.go`, `internal/repository/sqlite/db_tuning.go`
- Cause: The app is optimized for small installs first, while write-heavy topology, backup, and settings traffic still share one local database.
- Improvement path: Use PostgreSQL for multi-user or larger installs, add operational guidance for when to migrate, and keep SQLite-specific features from becoming mandatory in core flows.

## Fragile Areas

**Lifecycle methods panic on duplicate starts:**
- Files: `internal/scheduler/scheduler.go`, `internal/state/store.go`, `internal/worker/pipeline.go`
- Why fragile: Re-entrant startup, test harness mistakes, or future orchestration changes crash the process instead of returning an actionable error.
- Safe modification: Replace panics with explicit errors or idempotent start semantics before adding more lifecycle callers.
- Test coverage: `internal/ws/hub_test.go` covers backpressure behavior, but there is no comparable regression suite asserting safe repeated lifecycle start/stop behavior across all three components.

**Restore application is entangled with startup and uses fatal exits:**
- Files: `cmd/theia/main.go`, `cmd/theia/main_test.go`, `internal/service/instance_backup_service.go`
- Why fragile: `applyPendingRestore` mutates the live DB, backups directory, and `known_hosts` during process startup and calls `log.Fatalf` on some failure paths.
- Safe modification: Isolate restore execution behind a service-level transaction-like coordinator and keep `cmd/theia/main.go` limited to wiring and error propagation.
- Test coverage: `cmd/theia/main_test.go` exercises success and some failure paths, but the startup flow still depends on real filesystem sequencing and is easy to destabilize.

**Canvas and device configuration flows are hard to change safely:**
- Files: `frontend/src/components/DeviceConfigPanel.tsx`, `frontend/src/components/canvas/useCanvasData.ts`, `frontend/src/components/Canvas.tsx`
- Why fragile: WinBox state, settings fetches, topology refresh, form validation, and panel rendering are spread across large hooks/components with shared local state.
- Safe modification: Extract WinBox handling, topology fetching, and form sections into smaller hooks/components with narrower props.
- Test coverage: Component tests exist, but large files still encourage integration-style assertions over focused unit coverage.

## Scaling Limits

**SQLite-backed installs top out quickly under concurrency:**
- Current capacity: `internal/repository/sqlite/db_tuning.go` caps SQLite at `4-16` open connections with `5000ms` busy timeout and `3` busy retries.
- Limit: Write bursts from polling, topology updates, backup jobs, and settings changes can still serialize on one local DB file.
- Scaling path: Promote PostgreSQL earlier, keep plan checks in `internal/repository/sqlite/postgres_plan.go` in CI, and document a migration threshold for operators.

**WebSocket fan-out has small fixed buffers:**
- Current capacity: `Hub.broadcast` is buffered to `32` messages and each client send queue is buffered to `16` messages in `internal/ws/hub.go`.
- Limit: Slow consumers or bursty snapshots force backpressure quickly and currently block producers.
- Scaling path: Add per-message coalescing, snapshot diffs with stronger dropping semantics, and external pub/sub if fan-out grows beyond one process.

## Dependencies at Risk

**`github.com/mattn/go-sqlite3` keeps the default path tied to CGO and SQLite locking behavior:**
- Risk: Build portability and concurrency behavior depend on CGO and SQLite-specific tuning.
- Impact: Deployment simplicity and write scalability both degrade as the app grows.
- Migration plan: Treat PostgreSQL as the standard deployment target for larger environments and keep SQLite as an explicitly small-install mode.

## Missing Critical Features

**No user/session model or role-based authorization:**
- Problem: There is no built-in identity boundary for settings, secrets, backups, or device control endpoints.
- Blocks: Safe multi-user deployments, internet-exposed deployments, and principle-of-least-privilege access for operations teams.

## Test Coverage Gaps

**No regression guard that secrets stay redacted in API responses:**
- What's not tested: Redaction/auth boundaries for `bridge_secret`, SNMP secret fields, and plaintext WinBox credentials.
- Files: `internal/api/settings_handler.go`, `internal/api/snmp_profile_handler.go`, `internal/api/device_credential_profile_handler.go`, `internal/api/settings_handler_test.go`, `internal/api/snmp_profile_handler_test.go`, `internal/api/device_credential_profile_handler_test.go`
- Risk: Future work can continue normalizing secret exposure because current APIs and tests do not enforce redaction.
- Priority: High

**No stress-style tests for producer blocking under WebSocket overflow:**
- What's not tested: End-to-end latency impact when `internal/ws/hub.go` blocks on a full broadcast buffer.
- Files: `internal/ws/hub.go`, `internal/ws/hub_test.go`, `internal/worker/pipeline.go`
- Risk: Throughput regressions can appear only under burst traffic or many slow clients.
- Priority: Medium

---

*Concerns audit: 2026-04-19*
