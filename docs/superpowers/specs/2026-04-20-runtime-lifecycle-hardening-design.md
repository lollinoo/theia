# Runtime Lifecycle Hardening Design

Date: 2026-04-20
Status: Approved for planning
Scope: Runtime lifecycle hardening slice focused on restore orchestration and explicit startup semantics

## Summary

This slice removes remaining lifecycle fragility from startup and restore paths without broadening into a full runtime bootstrap refactor.

`internal/scheduler/scheduler.go`, `internal/state/store.go`, and `internal/worker/pipeline.go` already return explicit `ErrAlreadyStarted` errors instead of panicking on duplicate `Start`. This slice keeps that contract, locks it in with focused lifecycle tests, and removes the larger remaining risk: restore orchestration living inside `cmd/theia/main.go` with helper-level `log.Fatalf` calls and file-system side effects mixed into top-level process wiring.

The design extracts restore application into a dedicated coordinator under `internal/service`, gives restore a typed marker shared with staging code, and makes restore failures observable as ordinary startup errors returned to `main` rather than process termination buried inside helpers.

## Current Context

Relevant current files:

- `cmd/theia/main.go`: owns pending-restore detection, marker parsing, staged DB replacement, backup/known_hosts replacement, cleanup, and several helper functions. Some restore helper paths call `log.Fatalf` directly.
- `cmd/theia/main_test.go`: contains restore application tests even though restore is not main-specific behavior.
- `internal/service/instance_backup_service.go`: validates backup archives, stages restore artifacts under `.restore-staging`, and writes `.theia-restore-pending`, but writes marker JSON through an ad hoc `map[string]string`.
- `internal/scheduler/scheduler.go`: duplicate `Start` already returns `scheduler.ErrAlreadyStarted`; `Stop` is idempotent and restart is supported.
- `internal/state/store.go`: duplicate `Start` already returns `state.ErrAlreadyStarted`; `Stop` is idempotent and restart is supported.
- `internal/worker/pipeline.go`: duplicate `Start` already returns `worker.ErrAlreadyStarted`; startup rollback already stops child components on partial failure.

## Goals

- Remove restore-application logic from `cmd/theia/main.go` and place it behind a dedicated service/coordinator boundary.
- Eliminate helper-level `log.Fatalf` from restore application paths.
- Make pending restore application return explicit `(applied bool, err error)` results.
- Share one typed restore-marker schema between restore staging and restore application.
- Preserve explicit lifecycle semantics for scheduler, state store, and pipeline: duplicate `Start` remains observable and non-panicking.
- Add focused regression tests around restore coordination and lifecycle behavior.

## Non-Goals

- Introducing a broad runtime bootstrap coordinator for all services.
- Changing runtime startup order beyond delegating pending-restore handling to a dedicated component.
- Changing API behavior for instance-backup validation or staging beyond typed marker writing.
- Adding backward-compatibility layers beyond keeping the existing marker JSON field names stable.
- Refactoring unrelated startup wiring in `cmd/theia/main.go`.

## Proposed Architecture

This slice introduces one new boundary and preserves existing runtime-lifecycle boundaries.

### Restore Coordinator Boundary

Add `internal/service/restore_coordinator.go` with a focused `RestoreCoordinator` responsible only for applying already-staged restore artifacts.

Responsibilities:

- locate `.theia-restore-pending` next to configured DB path;
- read and validate typed marker contents;
- back up live DB to `.pre-restore.bak` when present;
- activate staged DB;
- activate staged backup directory and `known_hosts` file when present;
- clean marker and staging on success;
- preserve retry artifacts when activation fails in paths where safe retry is intended;
- return explicit errors instead of terminating process.

Non-responsibilities:

- validating uploaded archives;
- staging restore artifacts;
- owning runtime scheduler/store/pipeline startup;
- logging process-fatal startup errors.

### Existing Runtime Lifecycle Boundaries

`Scheduler`, `state.Store`, and `PipelineOrchestrator` keep their current explicit lifecycle contracts.

- duplicate `Start` while running returns component-local `ErrAlreadyStarted`;
- `Stop()` remains safe and idempotent;
- restart after `Stop()` remains supported where already implemented;
- partial startup rollback remains owned by `PipelineOrchestrator.Start`.

This slice does not switch these components to silent idempotent `nil` behavior because explicit duplicate-start errors are already implemented, observable, and testable.

## Component Design

### Restore Marker Type

Introduce package-local typed marker model in `internal/service` named `restoreMarker`, with same JSON field names already used on disk:

- `staged_db`
- `staged_backups`
- `staged_known_hosts`
- `db_path`
- `device_backup_dir`
- `known_hosts_path`
- `timestamp`

`InstanceBackupService.ValidateAndStageRestore()` writes this typed marker instead of an ad hoc `map[string]string`.

`RestoreCoordinator` reads the same type. This removes schema drift risk between staging and apply paths while keeping on-disk compatibility with already-staged artifacts created by current code.

### Restore Coordinator API

`RestoreCoordinator` is constructed from explicit runtime paths:

- `dbPath`
- `deviceBackupDir`
- `knownHostsPath`

Primary method:

- `ApplyPendingRestore() (bool, error)`

Return contract:

- no marker present: `(false, nil)`;
- marker present and restore applied successfully: `(true, nil)`;
- marker present but unreadable or invalid: `(false, error)`;
- marker present and activation fails: `(false, error)`.

The coordinator validates that marker target paths match configured runtime paths before applying changes. If they do not match, startup fails with explicit error rather than applying staged artifacts to an unexpected runtime layout.

### File-System Helpers

Current helpers in `cmd/theia/main.go` move into coordinator-owned implementation, still using small helper functions for:

- single-file copy;
- directory copy;
- file replacement with rollback of previous file when possible;
- directory replacement with rollback of previous directory when possible.

These helpers return wrapped `error` values with step-specific context. They do not log fatal or decide process exit.

## Startup Flow

SQLite startup flow becomes:

1. `main` loads config and resolves runtime paths.
2. `main` constructs `RestoreCoordinator` for SQLite deployments.
3. `main` calls `ApplyPendingRestore()` before opening DB.
4. If result is `(true, nil)`, `main` logs one success message and continues normal startup.
5. If result is `(false, nil)`, startup continues normally.
6. If result is `(false, err)`, `main` treats it as startup failure and exits from top-level startup handling.

Top-level fatal logging remains acceptable in `main` because it is the process bootstrap boundary. Hidden fatal exits inside restore helpers are not.

## Error Handling

### Marker Read and Parse Failures

- Missing marker file is ordinary no-op.
- Existing marker that cannot be read returns explicit error.
- Existing marker with invalid JSON returns explicit error.

This is intentionally stricter than current best-effort behavior because a pending restore marker indicates an interrupted or staged restore that should not be ignored silently.

### Restore Activation Failures

- DB backup failure returns error before staged DB activation completes.
- Staged DB activation failure returns error.
- Backup-directory or `known_hosts` activation failures return error.
- Where rollback/retry preservation is already implemented, coordinator keeps marker and staging content intact so operator can retry safely after fixing local cause.

Coordinator does not attempt broad automatic recovery beyond existing narrow rollback behavior in file/dir replacement helpers.

### Runtime Lifecycle Failures

- Duplicate `Start` on scheduler/store/pipeline remains explicit error, not panic.
- `PipelineOrchestrator.Start` continues to roll back partial child startup.
- Context cancellation remains normal shutdown path, not startup failure.

## Testing Strategy

### Restore Coordinator Tests

Create `internal/service/restore_coordinator_test.go` and move restore-application coverage there from `cmd/theia/main_test.go`.

Required coverage:

- no pending marker returns `(false, nil)`;
- successful staged restore swaps DB and optional artifacts, then removes marker and staging directory;
- backup artifact replacement failure preserves live artifacts where expected and leaves marker/staging for retry;
- malformed marker returns explicit error;
- marker path mismatch versus configured runtime paths returns explicit error;
- typed marker written by `InstanceBackupService.ValidateAndStageRestore()` can be read by coordinator without schema translation.

### Main Wiring Tests

`cmd/theia/main_test.go` keeps only tests that are truly main-specific. Restore application tests should no longer live there once restore logic is extracted.

### Lifecycle Regression Tests

Reuse existing lifecycle test files in:

- `internal/scheduler/scheduler_test.go`;
- `internal/state/store_test.go`;
- `internal/worker/pipeline_test.go`.

Required assertions for this slice:

- duplicate `Start` returns `ErrAlreadyStarted`;
- `Stop()` is idempotent;
- restart after `Stop()` still works where already supported;
- pipeline child-start rollback still leaves store restartable after scheduler startup failure.

## Acceptance Criteria

This slice is complete when all of the following are true:

- `cmd/theia/main.go` no longer owns restore marker parsing and restore activation logic;
- restore application does not call `log.Fatalf` from helper-level paths;
- pending restore application is exposed as explicit `(applied bool, err error)` service behavior;
- restore staging and restore apply share one typed marker schema;
- duplicate runtime `Start` calls do not panic and remain covered by focused tests;
- restore regression tests live with restore service/coordinator code, not in `main`.

## Follow-On Work

Deliberately deferred:

- full runtime bootstrap coordinator spanning restore, pipeline, schedulers, and HTTP server;
- broader decomposition of `cmd/theia/main.go` into multiple startup modules;
- stronger operator tooling for inspecting or clearing failed restore staging;
- broader lifecycle abstractions shared across unrelated services.
