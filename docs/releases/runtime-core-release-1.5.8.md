# Release 1.5.8

## Overview

Version `1.5.8` packages the `runtime-core` branch work into a runtime hardening release. It improves overview-stream resilience, cleans up runtime telemetry handling across backend and frontend, tightens deployment defaults and filesystem behavior, and adds stronger automated quality gates around realtime and browser-facing flows.

## Added

- Realtime quality gates for websocket burst handling, replay stability, collector contracts, and browser e2e coverage.
- Additional backend runtime boundaries for bootstrap, filesystem preparation, restore coordination, task execution, runtime state management, and telemetry normalization.
- Additional frontend runtime helpers and test coverage for canvas composition, detail routing, panel adapters, and form-model boundaries.

## Changed

- Overview websocket delivery is now versioned and non-blocking, reducing coupling between runtime production and client backpressure.
- Frontend runtime consumers now use a cleaner capability-oriented runtime shape across canvas, dashboard, and panel flows.
- PostgreSQL is now the default deployment path for production-oriented setups.
- Frontend validation now enforces the Biome quality gate as part of the standard quality flow.

## Fixed

- Overview streaming now preserves detail and alert state while runtime payload handling is tightened.
- Restore startup handling is more explicit and resilient during pending-restore application.
- Runtime-managed filesystem paths now use stricter permissions for sensitive local artifacts.
- Browser e2e CI now targets the backend explicitly, reducing environment ambiguity.
- SNMP contract validation is more stable under CI.
