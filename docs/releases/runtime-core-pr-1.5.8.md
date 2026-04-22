# runtime-core PR Summary

## Scope

This document summarizes the changes already implemented on branch `runtime-core`. It is intended to be used as the starting point for the pull request description.

## Summary

`runtime-core` consolidates a broad runtime-quality slice across backend, frontend, deployment defaults, and CI. The branch reduces coupling in the runtime path, normalizes runtime telemetry, makes overview streaming versioned and non-blocking, completes the frontend capability split needed for the new runtime contract, hardens startup and filesystem behavior, and strengthens required quality gates around realtime delivery, collector contracts, formatting, and browser coverage.

## Main Change Areas

### Runtime and backend structure

- Extracted runtime bootstrap concerns into more focused boundaries such as runtime bootstrap helpers, runtime paths, filesystem helpers, and vendor-registry bootstrap wiring.
- Split backend runtime responsibilities into clearer capabilities, including device discovery, device mutation, restore coordination, pipeline task execution, pipeline runtime state, snapshot broadcasting, and runtime normalization.
- Reduced large-file coupling in the runtime hot path so backend behavior is easier to reason about and test.

### Realtime overview and websocket flow

- Normalized runtime telemetry before websocket serialization so the frontend consumes a cleaner runtime model.
- Made overview streaming non-blocking and versioned to improve behavior under client backpressure and burst updates.
- Preserved detail subscriptions and alert/detail state while slimming and hardening overview delivery.
- Added websocket and replay coverage around resync behavior, burst handling, and deterministic topology replay outcomes.

### Frontend runtime and canvas refactor

- Finished the frontend capability split across canvas helpers, runtime adapters, panel adapters, dashboard runtime rows, form models, and submitters.
- Moved more runtime interpretation into focused frontend adapters instead of spreading it across large UI components.
- Added focused tests around detail subscription routing, topology composition, runtime adapters, canvas helpers, and form-model boundaries.

### Deployment and runtime hardening

- Made PostgreSQL the default production path and aligned config, compose, setup, and bootstrap behavior with that policy.
- Hardened runtime-managed filesystem permissions for sensitive local paths.
- Hardened restore startup lifecycle behavior so pending restore application is safer and more explicit.
- Made browser e2e backend targeting explicit in CI.

### Quality gates and contracts

- Added a realtime quality gate for high-risk websocket and runtime flows.
- Added collector contract coverage and supporting fixtures for Prometheus and SNMP validation.
- Enforced the frontend Biome quality gate in the branch's frontend validation flow.
- Stabilized the SNMP contract gate and added supporting test coverage.

## Reviewer Notes

- The largest coordinated change is the runtime-overview contract: backend runtime normalization, websocket delivery behavior, and frontend runtime consumers changed together in this branch.
- Deployment reviewers should inspect the PostgreSQL-default and filesystem-hardening changes together with bootstrap changes because they affect startup expectations.
- CI and test additions are a significant part of the branch and are meant to lock down the new runtime behavior rather than just document it.

## Validation Highlights

- Added or expanded coverage in `internal/ws`, `internal/worker`, `internal/service`, `internal/collector`, and `frontend/src`.
- Added branch-level quality gates for realtime stress behavior, collector contracts, frontend quality, and browser e2e.
- Strengthened local and CI feedback loops around runtime regressions and transport edge cases.
