---
phase: 29-winbox-bridge-system-tray-configure-path-port-and-origin-sta
plan: "03"
subsystem: build
tags: [ci, makefile, cgo, cross-compilation, windows, macos, winbox-bridge]
dependency_graph:
  requires: ["29-01"]
  provides: ["correct-bridge-binaries-all-6-targets"]
  affects: [".github/workflows/ci.yml", "Makefile"]
tech_stack:
  added: []
  patterns:
    - "Dynamic matrix runs-on for per-platform runner selection in GitHub Actions"
    - "Per-matrix ldflags for platform-specific linker flags (-H=windowsgui)"
    - "BRIDGE_TARGETS_NOCGO variable to document CGO constraints per OS"
key_files:
  created: []
  modified:
    - Makefile
    - .github/workflows/ci.yml
decisions:
  - "Renamed BRIDGE_TARGETS to BRIDGE_TARGETS_NOCGO to make CGO constraint explicit in variable name"
  - "macOS darwin targets removed from local Makefile target — they require CGO=1 and a native Mac; documented via NOTE echo"
  - "CI matrix gains runs-on/cgo/ldflags fields per entry rather than using conditional steps — cleaner, each entry is self-describing"
metrics:
  duration: "5 minutes"
  completed: "2026-04-09T13:34:29Z"
  tasks_completed: 2
  tasks_total: 2
  files_changed: 2
---

# Phase 29 Plan 03: CI and Makefile — Bridge Build Split (Windows -H=windowsgui, macOS CGO) Summary

**One-liner:** Updated Makefile and GitHub Actions to build Windows bridge binaries with `-H=windowsgui` (no console window) and macOS binaries on `macos-latest` with `CGO_ENABLED=1` (required by fyne.io/systray Cocoa binding).

## What Was Built

### Task 1: Makefile bridge-build-all (commit ef9a726)

Updated `Makefile` bridge section:

- `BRIDGE_TARGETS` renamed to `BRIDGE_TARGETS_NOCGO` — darwin entries removed (darwin requires CGO=1, cannot cross-compile from Linux)
- New `ldextra` variable in the for loop: set to `-H=windowsgui` for Windows targets, empty otherwise
- `go build -ldflags="-s -w ${ldextra}"` — Windows builds suppress the console window; Linux builds unchanged
- `NOTE` echo added at end explaining how to build macOS locally with `CGO_ENABLED=1`

### Task 2: CI build-bridge job (commit cdc3ad1)

Updated `.github/workflows/ci.yml` `build-bridge` job:

- Job-level `runs-on: ubuntu-latest` replaced with `runs-on: ${{ matrix.runs-on }}` (dynamic per matrix entry)
- Each of the 6 matrix entries now has `runs-on`, `cgo`, and `ldflags` fields
- Darwin entries: `runs-on: macos-latest`, `cgo: "1"`, `ldflags: "-s -w"` — macOS systray requires Cocoa via CGO
- Windows entries: `runs-on: ubuntu-latest`, `cgo: "0"`, `ldflags: "-s -w -H=windowsgui"` — suppress console window
- Linux entries: `runs-on: ubuntu-latest`, `cgo: "0"`, `ldflags: "-s -w"` — unchanged
- `CGO_ENABLED` and `-ldflags` values now driven by `matrix.cgo` and `matrix.ldflags`
- Job name updated to `"Bridge: ${{ matrix.goos }}/${{ matrix.goarch }}"` for clearer CI display
- All 6 matrix entries preserved; `softprops/action-gh-release@v2` upload step unchanged

## Verification Results

All 5 plan verification checks passed:
1. `macos-latest` present in CI — PASS
2. `windowsgui` present in CI — PASS
3. `windowsgui` present in Makefile — PASS
4. `darwin` NOT in `BRIDGE_TARGETS_NOCGO` — PASS
5. Exactly 6 `goos:` entries in CI matrix — PASS

## Deviations from Plan

None — plan executed exactly as written.

## Threat Surface Scan

No new network endpoints, auth paths, file access patterns, or schema changes introduced. Build tooling changes only.

Threat mitigations from plan's threat register applied:
- T-29-09: Actions pinned to major versions (checkout@v4, setup-go@v5, gh-release@v2); GITHUB_TOKEN used
- T-29-10: `-ldflags="-s -w"` strips symbol/debug info maintained for all 6 binaries
- T-29-11: `-H=windowsgui` accepted — standard for GUI/tray apps; no privilege change
- T-29-12: macOS CGO via GitHub-managed macos-latest runner — Apple-provided Xcode/clang

## Self-Check: PASSED

| Item | Status |
|------|--------|
| Makefile | FOUND |
| .github/workflows/ci.yml | FOUND |
| 29-03-SUMMARY.md | FOUND |
| Commit ef9a726 (Makefile) | FOUND |
| Commit cdc3ad1 (CI) | FOUND |
