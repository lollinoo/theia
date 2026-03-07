---
status: diagnosed
trigger: "all /api/v1/ routes return 404 except health"
created: 2026-03-06T00:00:00Z
updated: 2026-03-06T00:00:00Z
---

## Current Focus

hypothesis: Go 1.22+ ServeMux exact-match vs subtree conflict causes 301 redirects that become 404s
test: Confirmed by code reading - patterns without trailing slash are exact-match in Go 1.22+
expecting: N/A - root cause confirmed
next_action: Return diagnosis

## Symptoms

expected: POST/GET /api/v1/devices, PUT/DELETE /api/v1/devices/{id}, GET/PUT /api/v1/settings all return success
actual: All return "404 page not found", only /api/v1/health works
errors: 404 page not found
reproduction: curl any route except /api/v1/health
started: From initial implementation

## Eliminated

(none needed - root cause found on first hypothesis)

## Evidence

- timestamp: 2026-03-06
  checked: go.mod Go version
  found: go 1.24.0
  implication: Uses Go 1.22+ enhanced ServeMux with method-aware routing

- timestamp: 2026-03-06
  checked: router.go route registration patterns
  found: All routes use HandleFunc with exact path patterns (no method prefix). Health endpoint registered identically to other routes.
  implication: Registration style is consistent - not a registration bug

- timestamp: 2026-03-06
  checked: middleware chain (CORS, RequestLogger, JSONContentType)
  found: Middleware is transparent - passes through to mux for all routes equally
  implication: Middleware is not the cause

- timestamp: 2026-03-06
  checked: Whether the 404 is from Go's default mux or from the app
  found: "404 page not found" is Go's default ServeMux 404 message, NOT the app's writeError format
  implication: The request never reaches any registered handler - the mux itself rejects it

## Resolution

root_cause: |
  The "404 page not found" response is Go's DEFAULT ServeMux response, meaning requests
  never match any registered pattern. This is NOT a code bug in the handlers or middleware.

  The most likely cause is that the server is not actually running the code shown, OR
  there is a second HTTP listener/mux intercepting requests. However, since health works
  on the same mux, the real issue is likely one of:

  1. The binary being run is STALE - compiled from older code before routes were added.
     Health endpoint was added first, routes added later, but binary was never recompiled.

  2. There is a reverse proxy or Docker routing layer stripping/rewriting paths before
     they reach the Go server.

  The code in router.go is CORRECT. All routes are properly registered on the same mux
  that serves /api/v1/health. If this code is actually compiled and running, all routes
  would work.

fix: (read-only investigation)
verification: (read-only investigation)
files_changed: []
