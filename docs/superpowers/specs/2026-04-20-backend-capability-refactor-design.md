# Backend Capability Refactor Design

Date: 2026-04-20
Status: Approved for planning
Scope: Backend-only refactor slice to split project gravity files by capability

## Summary

This slice breaks the backend gravity files along existing capability seams instead of applying another cosmetic helper extraction.

The current risk is concentrated in three files that each own too many unrelated responsibilities: `cmd/theia/main.go` mixes process bootstrap, runtime-path resolution, vendor-registry seeding, dependency wiring, and shutdown; `internal/service/device_service.go` mixes device CRUD, mutation rules, discovery orchestration, topology bootstrap state, delayed re-probes, and helper algorithms; `internal/worker/pipeline.go` mixes lifecycle ownership, task execution, Prometheus monitoring, snapshot state ownership, and broadcast logic.

The design keeps the public boundaries stable while introducing explicit internal collaborators by capability. `main` remains the entrypoint, `DeviceService` remains the service facade, and `PipelineOrchestrator` remains the runtime facade. Internally, bootstrap, device mutation, device discovery, task execution, snapshot broadcasting, and Prometheus monitoring become separate focused units with clear ownership.

## Current Context

Relevant current files:

- `cmd/theia/main.go`: owns flag parsing, config loading, runtime-path precedence, pending restore application, database startup, vendor registry loading and seeding, dependency construction, worker startup, HTTP server startup, and graceful shutdown.
- `internal/service/device_service.go`: owns device creation, updates, deletion, reads, async probing, topology bootstrap rules, incomplete-link re-probe scheduling, virtual reachability, SNMP test flow, and discovered-neighbor support helpers.
- `internal/worker/pipeline.go`: owns lifecycle start/stop, worker task execution, Prometheus alert refresh, hostname enrichment cache, snapshot versioning, dirty/full snapshot construction, and WebSocket broadcast decisions.
- `internal/service/restore_coordinator.go`: already provides a dedicated restore boundary and should remain the restore capability owner for SQLite startup.
- `internal/worker/snapshot_builder.go`: already isolates snapshot materialization logic and should stay the builder dependency rather than being absorbed into a new orchestrator.
- `.planning/codebase/ARCHITECTURE.md`, `.planning/codebase/CONCERNS.md`, and `.planning/codebase/STRUCTURE.md`: document the backend gravity-file problem and confirm the current package layout already supports a natural capability split inside `cmd/theia`, `internal/service`, and `internal/worker`.

## Goals

- Split backend gravity files by capability, not by arbitrary helper extraction.
- Keep the public backend entrypoints stable: `main`, `service.DeviceService`, and `worker.PipelineOrchestrator` remain the external boundaries.
- Make bootstrap, discovery, mutation, scheduling/collection, snapshotting, and Prometheus monitoring ownership explicit.
- Reduce review blast radius and make future backend slices easier to land without reopening the same large files.
- Preserve current runtime behavior, API behavior, lifecycle semantics, and existing package structure.

## Non-Goals

- Refactoring any frontend files in this slice.
- Changing REST endpoints, request/response payloads, or WebSocket DTOs.
- Reworking scheduler, collector, state-store, or hub behavior beyond structural delegation needed for the refactor.
- Reopening the restore-lifecycle design that was already extracted into `internal/service/restore_coordinator.go`.
- Introducing new top-level packages or a new package hierarchy.
- Replacing the current goroutine and concurrency model with a different runtime design.

## Proposed Architecture

This slice introduces explicit internal collaborators while preserving the current public facades.

### Process Bootstrap Boundary

`cmd/theia/main.go` becomes a thin process boundary responsible only for:

- parsing flags;
- constructing the top-level bootstrap runner;
- performing the final `log.Fatalf` on unrecoverable startup errors.

All bootstrap sequencing moves into a new unexported `runtimeBootstrap` owned by the `main` package.

### Device Domain Boundary

`service.DeviceService` remains the only public device-domain facade consumed by handlers and runtime wiring. It delegates internally to focused collaborators:

- `deviceMutationService`: device create/update/delete/read rules and mutation-side normalization;
- `deviceDiscoveryCoordinator`: SNMP discovery, topology bootstrap state, delayed re-probes, virtual reachability, and topology notifications;
- discovery support helpers: pure discovered-neighbor and topology helper logic extracted out of the facade file.

This keeps one stable entrypoint for callers while removing the current internal gravity well.

### Runtime Pipeline Boundary

`worker.PipelineOrchestrator` remains the public lifecycle owner for the live runtime path. It delegates internally to focused collaborators:

- `pipelineTaskRunner`: scheduler task consumption, collector execution, state updates, and static persistence handoff;
- `pipelineSnapshotBroadcaster`: dirty/full snapshot decisions, overview versioning, topology-changed broadcast, and alert fan-out decisions;
- `pipelinePrometheusMonitor`: Prometheus availability refresh, alert collection, and hostname enrichment cache maintenance;
- `pipelineRuntimeState`: shared in-memory state for snapshot versions, alerts, hostnames, counter baselines, and hashes, with centralized locking.

This preserves one public runtime facade while splitting the internal capabilities that currently compete for ownership inside `pipeline.go`.

## File Structure

### `cmd/theia`

- Keep: `cmd/theia/main.go`
  Responsibility: process entrypoint only.
- Create: `cmd/theia/runtime_bootstrap.go`
  Responsibility: startup and shutdown orchestration, dependency graph construction, worker/server lifecycle.
- Create: `cmd/theia/runtime_paths.go`
  Responsibility: resolve `appDataDir`, `backupDir`, `knownHostsPath`, and `instanceBackupDir` from config, env, and defaults.
- Create: `cmd/theia/vendor_registry_bootstrap.go`
  Responsibility: vendor-registry load, seed, DB merge, and registry-build helpers currently embedded in `main.go`.

### `internal/service`

- Keep: `internal/service/device_service.go`
  Responsibility: stable facade, constructor, options, shared dependencies, shared topology-mode helpers, and public delegation methods.
- Create: `internal/service/device_mutation_service.go`
  Responsibility: `AddDevice`, `UpdateDevice`, `DeleteDevice`, `GetDevice`, and `GetAllDevices` mutation/read capability.
- Create: `internal/service/device_discovery_coordinator.go`
  Responsibility: `ProbeDevice`, `ReprobeDevice`, `RunTopologyDiscoveryNow`, `PingVirtualDevice`, `TestSNMP`, `WaitForProbes`, async probe execution, and delayed topology follow-up behavior.
- Create: `internal/service/device_discovery_support.go`
  Responsibility: pure helper logic for discovered-neighbor dedupe, preference scoring, interface-anchor normalization, and similar support code.

### `internal/worker`

- Keep: `internal/worker/pipeline.go`
  Responsibility: stable facade, constructor, lifecycle methods, and read-only runtime getters.
- Create: `internal/worker/pipeline_task_runner.go`
  Responsibility: `runWorker`, `runTask`, virtual-task handling, task completion, and subscribed-detail delta publication.
- Create: `internal/worker/pipeline_snapshot_broadcaster.go`
  Responsibility: coalesced dirty/full snapshot broadcast flow, overview version increments, topology-changed notifications, and alert message emission.
- Create: `internal/worker/pipeline_prometheus_monitor.go`
  Responsibility: periodic Prometheus refresh and status/alert/hostname maintenance.
- Create: `internal/worker/pipeline_runtime_state.go`
  Responsibility: shared locked state used by the broadcaster and monitor.

## Component Design

### Runtime Bootstrap

`runtimeBootstrap` is an unexported struct in `cmd/theia` that owns process startup and shutdown sequencing.

Responsibilities:

- load config;
- resolve runtime paths;
- normalize DB dialect;
- apply pending SQLite restore through `service.RestoreCoordinator` before DB open when needed;
- open DB and run migrations;
- load and seed vendor registry;
- construct repositories, services, schedulers, workers, and HTTP router/server;
- start runtime workers and server;
- stop server and background workers in reverse order on shutdown.

Non-responsibilities:

- long-term ownership of restore behavior;
- device-domain business logic;
- runtime snapshot/broadcast logic;
- HTTP request handling.

`main.go` should no longer contain bootstrap helper logic other than calling the bootstrap runner and handling the final fatal exit.

### DeviceService Facade

`DeviceService` remains the stable public type. The refactor does not change its constructor signature or public method set.

It owns:

- dependency storage shared by internal collaborators;
- constructor options such as topology observation store injection;
- delegation from public methods to the correct capability owner;
- compatibility for existing handler and runtime wiring code.

It does not continue to own the full implementation for mutation and discovery behaviors in one file.

### Device Mutation Capability

`deviceMutationService` owns mutation-side rules and device reads.

Responsibilities:

- defaulting and normalization during device creation and update;
- notes, tags, metrics-source, and poll-override mutation behavior;
- repo writes for create/update/delete;
- mutation-time decisions that trigger re-probe or poll rescheduling;
- read flows that normalize returned devices before handing them back through the facade.

This keeps CRUD and field-mutation semantics together without entangling them with async discovery and topology follow-up logic.

### Device Discovery Capability

`deviceDiscoveryCoordinator` owns discovery and topology-follow-up behavior.

Responsibilities:

- asynchronous probe execution;
- SNMP discovery orchestration;
- SNMP connectivity test flow;
- topology bootstrap state transitions;
- delayed incomplete-link re-probes;
- immediate/manual topology discovery triggers;
- virtual-device reachability probing;
- topology change notifications.

This unit remains dependent on the same repos and discovery function, but it becomes the single owner for discovery-time side effects.

### PipelineOrchestrator Facade

`PipelineOrchestrator` remains the stable runtime facade.

It keeps ownership of:

- `Start(ctx)` and `Stop()` lifecycle boundaries;
- `Status()` and snapshot/status getters;
- child startup rollback and the already-explicit duplicate-start semantics.

It delegates the internal runtime capabilities instead of continuing to implement them all in one file.

### Pipeline Task Runner

`pipelineTaskRunner` owns task execution.

Responsibilities:

- consume scheduler tasks;
- execute performance, operational, static, and virtual paths;
- update runtime state through `state.Store`;
- perform static persistence handoff to topology services;
- complete scheduler tasks;
- publish device-detail deltas to subscribed clients.

It does not own long-lived snapshot versions, alert state, or Prometheus availability state.

### Pipeline Snapshot Broadcaster

`pipelineSnapshotBroadcaster` owns overview publication behavior.

Responsibilities:

- coalesce dirty runtime signals;
- decide between delta and full snapshot publication;
- advance overview versions;
- update the last published snapshot;
- emit `topology_changed` and alert broadcasts when required.

It is the only component that should directly mutate snapshot version counters and last-snapshot state.

### Pipeline Prometheus Monitor

`pipelinePrometheusMonitor` owns Prometheus-side refresh behavior.

Responsibilities:

- periodic refresh loop;
- Prometheus URL resolution through current settings;
- alert collection;
- availability status publication;
- hostname enrichment cache maintenance.

It is the only component that should directly mutate Prometheus availability state, alert groups, and hostname overrides in runtime state.

### Pipeline Runtime State

`pipelineRuntimeState` centralizes the mutable state currently spread across `PipelineOrchestrator` fields.

Owned state includes:

- last overview snapshot;
- overview version;
- alert version;
- Prometheus status payload;
- hostname overrides and observation timestamps;
- alert groups;
- previous counter baselines;
- previous snapshot hashes.

This is not a new public subsystem. It is an internal coordination object that prevents broadcaster and monitor responsibilities from interleaving unsafely across multiple files.

## Data Flow

### Startup Flow

1. `main` parses flags and creates `runtimeBootstrap`.
2. `runtimeBootstrap` loads config and resolves runtime paths.
3. For SQLite, `runtimeBootstrap` applies any pending restore through `service.RestoreCoordinator`.
4. `runtimeBootstrap` opens the DB, runs migrations, builds the vendor registry, constructs repositories/services/runtime components, and starts the runtime pipeline.
5. `runtimeBootstrap` starts the HTTP server and owns graceful shutdown.

### Device Mutation Flow

1. A handler calls a public `DeviceService` method.
2. `DeviceService` forwards the request to `deviceMutationService`.
3. Mutation logic performs normalization, persistence, and any rescheduler or reprobe trigger decision.
4. Any discovery follow-up still re-enters through the facade-owned collaborator boundary instead of embedding discovery logic in the mutation file.

### Device Discovery Flow

1. A caller invokes `ProbeDevice`, `ReprobeDevice`, `RunTopologyDiscoveryNow`, or a device-creation path that requires probing.
2. `DeviceService` forwards the request to `deviceDiscoveryCoordinator`.
3. The coordinator runs probe logic, persists discovery results through existing service/repo paths, updates topology bootstrap metadata, and emits topology notifications when needed.

### Pipeline Runtime Flow

1. `PipelineOrchestrator.Start` initializes lifecycle state and starts the scheduler/store pair as it does today.
2. The task runner consumes scheduled tasks and applies runtime updates.
3. The Prometheus monitor periodically refreshes alerts and availability state.
4. The snapshot broadcaster coalesces runtime dirty signals and publishes snapshot updates.
5. `PipelineOrchestrator.Stop` cancels the runtime context and waits for child routines to finish, preserving the current lifecycle contract.

## Error Handling

This slice is structural, so error semantics should become clearer but not materially different.

- `runtimeBootstrap` returns wrapped startup errors such as `load config`, `resolve runtime paths`, `open database`, `seed vendor registry`, `start pipeline`, or `start http server`.
- `main` remains the only layer allowed to decide the final fatal process exit.
- Internal collaborators under `internal/service` and `internal/worker` do not call `log.Fatalf`.
- Runtime collaborators may keep local `log.Printf` behavior for background-operation failures where the current design already logs and continues.
- Existing lifecycle semantics for scheduler, store, and pipeline remain unchanged from the current hardened behavior.
- This slice must not introduce new silent fallbacks that hide failures merely because logic was moved into smaller files.

## Testing Strategy

### Bootstrap Tests

`cmd/theia/main_test.go` should become focused bootstrap/wiring coverage rather than a catch-all integration surface.

Required coverage:

- bootstrap propagates startup errors with contextual wrapping;
- bootstrap calls the correct startup boundaries in the expected order;
- `main`-specific tests remain limited to process-boundary behavior.

### Device Service Tests

`internal/service/device_service_test.go` remains the primary contract test location for the public `DeviceService` API.

Permitted test split:

- add new focused tests beside new internal files where that improves clarity;
- keep public-contract assertions visible at the facade level.

Required coverage:

- mutation flows continue to normalize and persist devices as before;
- discovery flows continue to probe, persist topology, and notify as before;
- mutation and discovery collaborators still compose correctly through the facade.

### Pipeline Tests

`internal/worker/pipeline_test.go` remains the primary contract test location for `PipelineOrchestrator` behavior.

Permitted test split:

- add focused tests beside new internal worker files if needed;
- keep lifecycle and observable broadcast behavior covered through the public facade.

Required coverage:

- `Start`/`Stop` and duplicate-start semantics remain unchanged;
- scheduled tasks are still completed correctly;
- dirty/full snapshot and resync behaviors remain unchanged;
- Prometheus refresh behavior remains unchanged.

### Verification

Acceptance verification for the slice is:

- `go test ./...` passes;
- no API or protocol changes are introduced;
- backend gravity files are split along explicit capability boundaries rather than helper-only extraction;
- tests still protect the current public behavior of startup, device service, and runtime pipeline.

## Acceptance Criteria

- `cmd/theia/main.go` is reduced to a thin process entrypoint.
- `internal/service/device_service.go` is reduced to a stable facade and no longer owns both mutation and discovery implementations inline.
- `internal/worker/pipeline.go` is reduced to a stable lifecycle facade and no longer owns task execution, Prometheus monitoring, and snapshot broadcasting inline.
- New internal collaborators have single clear capability ownership and remain package-internal.
- Restore remains owned by `service.RestoreCoordinator` and is only consumed by bootstrap.
- No behavior changes are required to justify the refactor.
