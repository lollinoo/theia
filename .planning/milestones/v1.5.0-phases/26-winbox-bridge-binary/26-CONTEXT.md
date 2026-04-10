# Phase 26: WinBox Bridge Binary - Context

**Gathered:** 2026-04-08
**Status:** Ready for planning

<domain>
## Phase Boundary

Build the `winbox-bridge` Go binary — a small HTTP server that runs on `localhost:1337`, accepts WinBox launch requests from Theia frontend, validates Origin + Host headers (DNS rebinding protection per BRIDGE-03), and spawns WinBox pre-authenticated (BRIDGE-04). Cross-compile for all 6 targets (Windows/Linux/macOS × amd64/arm64) and wire into Makefile + CI release pipeline.

No frontend changes — the frontend already calls `GET /health` and `POST /launch` (hardcoded in Phase 25). No changes to the main Theia server — the download endpoint already exists (Phase 24). This phase is entirely a new Go binary in `cmd/winbox-bridge/`.

Requirements in scope: BRIDGE-03, BRIDGE-04.

</domain>

<decisions>
## Implementation Decisions

### WinBox Executable Discovery
- **D-01:** Search PATH first (run `winbox` on Linux/macOS, `winbox64.exe` or `winbox.exe` on Windows), then fall back to platform-specific defaults:
  - Windows: `C:\Program Files\WinBox\winbox64.exe`
  - Linux: `/usr/bin/winbox`
  - macOS: `/Applications/WinBox.app/Contents/MacOS/WinBox` (or equivalent)
- **D-02:** `--winbox-path` CLI flag overrides auto-discovery entirely. If provided, skip PATH/default search and use the given path directly (validate it exists and is executable at startup).
- **D-03:** If discovery fails (no PATH entry, no default found, no flag), bridge starts but `/launch` returns 503 with `{"error": "winbox executable not found — use --winbox-path to specify location"}`. Do NOT refuse to start.

### Theia Origin Validation (BRIDGE-03)
- **D-04:** `--theia-origin` CLI flag specifies the accepted Theia origin, defaulting to `http://localhost:3000`. Bridge rejects any request whose `Origin` header does not exactly match this value.
- **D-05:** Bridge also validates `Host` header on every request — rejects any request where Host is not `localhost:1337` (DNS rebinding protection).
- **D-06:** Both validations apply to ALL endpoints (`/health`, `/launch`), not just `/launch`.
- **D-07:** Rejected requests return HTTP 403 (not 200, not 404). No information leak beyond the rejection.

### WinBox Process Launch (BRIDGE-04)
- **D-08:** `/launch` accepts `{"ip": "...", "username": "...", "password": "..."}` POST body. Bridge constructs the command as `winbox <ip> <username> <password>` using the discovered/configured executable path.
- **D-09:** Bridge is hardcoded to launch only the winbox binary — no `executable` or `command` field in the request body. Passing arbitrary commands is structurally impossible (no code path exists).
- **D-10:** Process is detached (stdout/stderr discarded, no process tracking). Returns HTTP 200 `{"ok": true}` immediately after `os.StartProcess` succeeds — fire-and-forget.
- **D-11:** If `os.StartProcess` fails (e.g., winbox not found at launch time), returns 500 `{"error": "failed to launch WinBox"}`. No internal error detail exposed in response.

### Health Endpoint
- **D-12:** `GET /health` returns HTTP 200 `{"ok": true}` when bridge is running. Also subject to Origin + Host validation (D-06). Frontend polls this every 30s to detect bridge status.

### Build Pipeline
- **D-13:** `make bridge-build-all` cross-compiles all 6 binaries using `GOOS/GOARCH` env vars (no CGO). Output goes to `bridge_binaries/` at repo root, matching the `bridge_binaries_dir` config field expected by the main Theia server (Phase 24 D-12). Binaries named `winbox-bridge-{os}-{arch}[.exe]`.
- **D-14:** CI release workflow (`.github/workflows/ci.yml`) gains a `build-bridge` job triggered on tag push (`refs/tags/v*`). Job cross-compiles all 6 binaries and uploads them as GitHub Release assets using `softprops/action-gh-release`. This is separate from Docker image builds.
- **D-15:** Bridge binary compiles with `CGO_ENABLED=0` — no CGO, no glibc dependency. Pure Go only.

### Source Location
- **D-16:** Bridge source lives at `cmd/winbox-bridge/main.go` (plus sub-packages if needed). Follows existing `cmd/theia/` pattern. Single entry point.

### Claude's Discretion
- CORS response headers (`Access-Control-Allow-Origin` etc.) — add if needed for browser preflight, omit if browser doesn't send OPTION for cross-origin fetch to localhost
- Logging verbosity at startup (print discovered winbox path and accepted origin)
- Whether to use `net/http` stdlib directly or a minimal router — stdlib is consistent with Theia main server patterns

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Bridge API contract (already live in frontend)
- `frontend/src/hooks/useBridgeHealth.ts` — health endpoint URL (`http://localhost:1337/health`), poll interval, response handling
- `frontend/src/components/Canvas.tsx` lines 242–256 — launch endpoint URL, POST body shape `{ip, username, password}`, silent error handling
- `frontend/src/components/Dashboard.tsx` lines 106–115 — same launch logic (Dashboard variant)

### Download handler (Phase 24 — binary naming convention)
- `internal/api/bridge_handler.go` — binary naming pattern `winbox-bridge-{os}-{arch}[.exe]`, valid OS/arch values

### Existing binary entry point pattern
- `cmd/theia/main.go` — Go binary entry point convention to follow

### Release CI (to extend)
- `.github/workflows/ci.yml` — existing release job structure to add `build-bridge` job alongside

### Requirements
- `.planning/REQUIREMENTS.md` §WinBox Bridge — BRIDGE-03, BRIDGE-04 acceptance criteria

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `cmd/theia/main.go`: Entry point pattern — the bridge binary should follow the same minimal main() structure
- No existing CORS middleware in Theia main server (it uses a custom `corsMiddleware` in `internal/api/middleware.go`) — bridge implements its own simpler version

### Established Patterns
- No CGO in bridge: consistent with `CGO_ENABLED=0` constraint. Main Theia server uses CGO for SQLite, but bridge has no DB — pure stdlib HTTP server.
- Error responses: `{"error": "message"}` JSON format consistent with Theia API handler pattern
- Binary naming: `winbox-bridge-{os}-{arch}[.exe]` — already hardcoded in `bridge_handler.go`

### Integration Points
- Bridge is standalone — connects to Theia only via HTTP from the frontend (not Go-to-Go)
- `bridge_binaries_dir` config field (Phase 24 D-12) points to where compiled binaries land. Local dev: set to `./bridge_binaries/` after running `make bridge-build-all`
- `--theia-origin` default `http://localhost:3000` matches Vite dev server port. Production Theia serves nginx on port 80/443 — users running production need to pass the flag.

</code_context>

<specifics>
## Specific Ideas

- Configurable `--theia-origin` was chosen over hardcoding `localhost:3000 + localhost:80` to handle non-standard Theia deployments (e.g., reverse proxy on a custom port, production HTTPS). The default of `http://localhost:3000` covers the common dev case.
- Bridge should NOT refuse to start if WinBox is not found — it starts and serves 503 from `/launch` instead. This lets the frontend health check work correctly while giving a clear error at launch time.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 26-winbox-bridge-binary*
*Context gathered: 2026-04-08*
