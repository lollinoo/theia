---
phase: 08-virtual-device-backend
verified: 2026-03-31T20:34:26Z
status: passed
score: 5/5 must-haves verified
re_verification: false
---

# Phase 8: Virtual Device Backend Verification Report

**Phase Goal:** Backend fully supports virtual devices as a first-class device type with appropriate probe behavior
**Verified:** 2026-03-31T20:34:26Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (from Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | POST a device with device_type "virtual" and subtype via API returns it with those fields | VERIFIED | `HandleCreate` virtual branch in `device_handler.go:105-148`; `deviceToResource` serializes `device_type`; `TestDeviceHandlerCreate_VirtualHappyPath` and `TestDeviceHandlerCreate_VirtualWithIP` pass |
| 2 | Multiple virtual devices with empty IPs coexist without unique constraint violations | VERIFIED | Migration `000009_partial_unique_ip.up.sql` creates `CREATE UNIQUE INDEX idx_devices_ip ON devices(ip) WHERE ip != ''`; service `AddDevice` accepts empty IP for virtual devices |
| 3 | Virtual device with IP shows up/down status based on ping reachability | VERIFIED | `probeDevice` guard marks "up" as initial value; `MetricsCollector` `probe_success` path (lines 594-626) overrides with real ping-based status for any device with IP and prometheus metrics source; virtual devices use default prometheus source |
| 4 | Virtual device without IP persists with "unknown" status and no probe attempts | VERIFIED | `AddDevice` sets `initialStatus = DeviceStatusUnknown` for virtual; skips `probeWg`/`probeDevice` entirely; `MetricsCollector` skips `dev.IP == ""` at line 605; `TestAddDevice_VirtualNoIP` passes |
| 5 | SNMP poller re-probe cycle completes without attempting SNMP on any virtual device | VERIFIED | `poller.go:99-102` adds `DeviceTypeVirtual` skip before `ReprobeDevice`; `TestPollAllDevices_SkipsVirtualDevices` passes with 1 discover call for physical only |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/domain/device.go` | DeviceTypeVirtual constant | VERIFIED | `DeviceTypeVirtual DeviceType = "virtual"` at line 16, between DeviceTypeAP and DeviceTypeUnknown |
| `internal/repository/sqlite/migrations/000009_partial_unique_ip.up.sql` | Partial unique index WHERE ip != '' | VERIFIED | Contains `DROP INDEX IF EXISTS idx_devices_ip` and `CREATE UNIQUE INDEX idx_devices_ip ON devices(ip) WHERE ip != ''` |
| `internal/repository/sqlite/migrations/000009_partial_unique_ip.down.sql` | Rollback to full unique index | VERIFIED | Contains `DROP INDEX IF EXISTS idx_devices_ip` and `CREATE UNIQUE INDEX idx_devices_ip ON devices(ip)` (without WHERE) |
| `internal/service/device_service.go` | AddDevice with deviceType param; probeDevice virtual guard | VERIFIED | `deviceType domain.DeviceType` at signature line 69; virtual skip at lines 96-127 (AddDevice) and 159-168 (probeDevice) |
| `internal/service/device_service_test.go` | Virtual device service tests | VERIFIED | `TestAddDevice_VirtualNoIP` (line 710), `TestAddDevice_VirtualWithIP` (line 749), `TestAddDevice_RegularStillRequiresProbe` (line 787) — all pass |
| `internal/api/device_handler.go` | Virtual validation in HandleCreate; DeviceType in createDeviceRequest; 12-arg AddDevice calls | VERIFIED | `DeviceType string json:"device_type,omitempty"` at line 49; virtual branch at lines 105-148; both HandleCreate and HandleBatchAdd use 12-arg AddDevice with deviceType |
| `internal/api/device_handler_test.go` | 7 virtual device handler tests | VERIFIED | All 7 tests found and passing: VirtualHappyPath, VirtualWithIP, VirtualMissingDisplayName, VirtualInvalidSubtype, VirtualMissingSubtype, VirtualNoTags, RegularStillRequiresIP |
| `internal/api/link_handler.go` | Virtual-side if_name relaxation; both-virtual rejection | VERIFIED | `srcIsVirtual`/`tgtIsVirtual` at lines 94-95; "at least one device must be non-virtual" at line 97; conditional if_name checks at lines 102-108 |
| `internal/api/link_handler_test.go` | 4 virtual link tests + seedVirtualDevice helper | VERIFIED | VirtualSourceEmptyIfName, VirtualTargetEmptyIfName, BothVirtualRejected, BothPhysicalRequiresBothIfNames — all pass |
| `internal/worker/poller.go` | DeviceTypeVirtual skip in pollAllDevices | VERIFIED | `devices[i].DeviceType == domain.DeviceTypeVirtual` skip at lines 99-102 with comment "Per VIRT-05" |
| `internal/worker/poller_test.go` | TestPollAllDevices_SkipsVirtualDevices | VERIFIED | Found at line 177; passes; verifies only 1 discover call for physical device |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/service/device_service.go` | `internal/domain/device.go` | `domain.DeviceTypeVirtual` in AddDevice and probeDevice | VERIFIED | Used at lines 96, 121, 159 |
| `internal/service/device_service_test.go` | `internal/service/device_service.go` | AddDevice calls with DeviceTypeVirtual as 4th arg | VERIFIED | `domain.DeviceTypeVirtual` at lines 724, 763 in test calls |
| `internal/api/device_handler.go` | `internal/service/device_service.go` | HandleCreate calls AddDevice with 12 args including deviceType | VERIFIED | Line 137-139: `h.svc.AddDevice(r.Context(), req.IP, req.Hostname, domain.DeviceTypeVirtual, ...)` |
| `internal/api/link_handler.go` | `internal/service/device_service.go` | HandleCreate fetches devices to check DeviceType | VERIFIED | Lines 82-91: `h.deviceService.GetDevice(...)` for both src and tgt |
| `internal/worker/poller.go` | `internal/domain/device.go` | pollAllDevices checks DeviceType before ReprobeDevice | VERIFIED | Line 100: `devices[i].DeviceType == domain.DeviceTypeVirtual` |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `device_handler.go` HandleCreate virtual branch | `device` returned from AddDevice | `deviceRepo.Create` via service | Yes — real DB write, not stub | FLOWING |
| `link_handler.go` HandleCreate | `srcDevice`, `tgtDevice` | `deviceService.GetDevice` → `deviceRepo.GetByID` | Yes — real DB reads | FLOWING |
| `poller.go` pollAllDevices | `devices` slice | `cache.GetDevices()` → `deviceRepo.GetAll()` | Yes — real DB read | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| All service tests pass (14 tests, incl. 3 new virtual) | `/usr/local/go/bin/go test ./internal/service/ -count=1` | PASS — 14 tests, 0 failures | PASS |
| All device handler tests pass (17 tests, incl. 7 new virtual) | `/usr/local/go/bin/go test ./internal/api/ -run "TestDeviceHandler" -count=1` | PASS — 17 tests, 0 failures | PASS |
| All link handler tests pass (10 tests, incl. 4 new virtual) | `/usr/local/go/bin/go test ./internal/api/ -run "TestLinkHandler" -count=1` | PASS — 10 tests, 0 failures | PASS |
| Poller virtual skip test passes | `/usr/local/go/bin/go test ./internal/worker/ -run "TestPollAllDevices_SkipsVirtualDevices" -count=1` | PASS | PASS |
| All worker tests pass (12 tests, incl. 1 new virtual skip) | `/usr/local/go/bin/go test ./internal/worker/ -count=1` | PASS — 12 tests, 0 failures | PASS |
| Project builds without errors | `/usr/local/go/bin/go build ./internal/... ./cmd/...` | No output (success) | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| VIRT-01 | 08-01 | Create virtual node with device_type "virtual", display name, and subtype | SATISFIED | `DeviceTypeVirtual` constant; `HandleCreate` virtual branch validates display_name and virtual_subtype; API returns device with device_type field |
| VIRT-02 | 08-01 | Virtual nodes can exist without an IP address (multiple empty-IP nodes coexist) | SATISFIED | Migration 000009 partial unique index; `AddDevice` accepts empty IP for virtual; service test `TestAddDevice_VirtualNoIP` |
| VIRT-03 | 08-01 | Virtual nodes with IP are ping-probed for status (up/down glow) | SATISFIED | `probeDevice` sets initial "up" for IP-bearing virtuals; `MetricsCollector` `probe_success` override (lines 594-626) provides real ping-based refinement via Prometheus blackbox exporter |
| VIRT-04 | 08-01 | Virtual nodes without IP have "unknown" status with no probing | SATISFIED | `initialStatus = DeviceStatusUnknown` for virtual in `AddDevice`; virtual skip bypasses probe launch; `MetricsCollector` skips `dev.IP == ""` |
| VIRT-05 | 08-02 | SNMP poller skips virtual devices during re-probe cycle | SATISFIED | `poller.go` lines 99-102 skip `DeviceTypeVirtual` before `ReprobeDevice`; `TestPollAllDevices_SkipsVirtualDevices` passes |

All 5 phase requirements (VIRT-01 through VIRT-05) are satisfied.

No orphaned requirements: REQUIREMENTS.md maps VIRT-01 to VIRT-05 to Phase 8, and all are claimed by 08-01 and 08-02 plans.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/service/device_service.go` | 162 | `markDeviceStatus(..., DeviceStatusUp)` for virtual with IP — marks "up" without actual ping | Info | Not a stub: this is intentional placeholder; `MetricsCollector probe_success` overrides on next cycle. Plan explicitly documents this behavior (D-05/D-07). |

No blockers or warnings found.

### Human Verification Required

None for the backend layer. All behaviors are verified via tests and code inspection.

One item to note for completeness (not a gap):

**Ping-based status for virtual devices with IP (VIRT-03)** requires Prometheus with a blackbox exporter configured for probe_success. In a fresh environment without Prometheus, virtual devices with IPs will show "up" status (from the initial probe guard) rather than real ping-based status. This is the documented design behavior and matches the requirements notes ("may use blackbox exporter"). No action needed for this phase.

### Gaps Summary

No gaps. All 5 success criteria are met:

1. Virtual device POST/GET with device_type and subtype fields — handler, service, domain, and serialization all wired end-to-end.
2. Empty-IP coexistence — migration 000009 creates the correct partial unique index.
3. Virtual with IP gets "up/down" status — via MetricsCollector probe_success path (Prometheus blackbox exporter).
4. Virtual without IP stays "unknown" with no probing — enforced in AddDevice and MetricsCollector.
5. Poller skips virtual devices — defense-in-depth skip in pollAllDevices plus guard in probeDevice.

All 4 git commits documented in SUMMARYs verified in git history: `ae80f2e`, `8b47068`, `e0ef434`, `55a032a`, `c5d0689`.

---

_Verified: 2026-03-31T20:34:26Z_
_Verifier: Claude (gsd-verifier)_
