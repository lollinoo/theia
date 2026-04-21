# Filesystem Hardening Design

Date: 2026-04-21
Status: Approved for planning
Scope: Runtime filesystem hygiene slice focused on restrictive permissions for local operational data

## Summary

This slice hardens the local runtime filesystem footprint without broadening into full secret management or a generic storage refactor.

The current runtime still creates several operational paths with group/world-readable defaults such as `0755` directories and `0644` files. That is acceptable for many development workflows but too loose for production paths that contain local backups, restore staging artifacts, restore markers, and runtime metadata. This slice introduces an explicit permission policy for those paths, enforces it both at creation time and against already-existing paths, and fails closed when the runtime cannot bring a sensitive path back to the required mode.

## Current Context

Relevant current files:

- `cmd/theia/runtime_bootstrap.go`: creates `dbDir`, `appDataDir`, `backupDir`, and `instanceBackupDir` with `0755` and is the natural bootstrap boundary for runtime path preparation.
- `internal/service/instance_backup_service.go`: creates backup subdirectories and restore staging paths with `0755`, writes restore marker JSON with `0644`, and copies restore artifacts into staging.
- `internal/service/restore_coordinator.go`: applies staged restore artifacts and creates some parent directories with `0755` during replacement.
- `internal/ssh/known_hosts.go`: already writes `known_hosts` with `0600`, which is a useful precedent for stricter file modes.
- `internal/service/backup_service.go`: writes exported device backup files with `0644`.

The recently completed runtime lifecycle hardening slice already established restore-specific boundaries and explicit restore failure behavior. This filesystem slice should build on that shape rather than re-opening restore orchestration design.

## Goals

- Define one explicit permission policy for runtime-sensitive filesystem paths.
- Create new sensitive directories and files with restrictive modes by default.
- Reconcile already-existing sensitive paths at startup or restore preparation time when they are more permissive than allowed.
- Fail closed when the runtime cannot reduce permissions on a sensitive operational path.
- Keep the slice narrowly focused on production hygiene for known runtime paths.

## Non-Goals

- Broad secret-management redesign.
- Ownership (`chown`) management, ACL management, or SELinux/AppArmor integration.
- Recursive hardening of every historical file under every runtime directory.
- Hardening unrelated developer outputs or test fixtures.
- Refactoring the full runtime bootstrap flow beyond what is needed to enforce permission policy.

## Proposed Architecture

Introduce a small shared runtime filesystem-hardening boundary that owns permission policy and enforcement for known sensitive paths.

Recommended shape:

- new focused helper under runtime-facing code, for example `cmd/theia/runtime_fs.go` or a similarly small internal helper used by bootstrap and restore code;
- one permission-policy table or explicit helper methods that describe expected mode per path type;
- small helpers for:
  - ensuring a directory exists with target mode;
  - ensuring a file exists or is written with target mode;
  - reconciling an existing path by narrowing permissions when current mode is too broad;
  - validating path type so a file path is not silently treated as a directory or vice versa.

This boundary is intentionally small. It should not become a generic virtual filesystem layer.

## Permission Policy

The policy for this slice is:

- runtime-sensitive directories: `0700`
- runtime-sensitive files and metadata: `0600`

Covered paths:

- `appDataDir`: `0700`
- SQLite parent directory when under runtime-managed local storage: `0700`
- `backupDir`: `0700`
- `instanceBackupDir`: `0700`
- restore staging directory `.restore-staging`: `0700`
- restore marker `.theia-restore-pending`: `0600`
- runtime `known_hosts`: `0600`
- instance backup archives and backup metadata written by the application: `0600`
- device backup files written by the application under managed backup storage: `0600`

Important constraint:

- the implementation narrows permissions only for known files and directories it owns or writes directly;
- it does not recursively walk existing backup trees just to chmod all descendants retroactively.

This keeps the slice deterministic and avoids broad side effects on legacy content while still protecting newly written artifacts and top-level runtime containers.

## Enforcement Model

### Bootstrap-Time Reconciliation

`cmd/theia/runtime_bootstrap.go` becomes the primary enforcement point for runtime-owned top-level paths.

During startup, after runtime paths are resolved and before services use them, bootstrap should:

1. ensure each managed sensitive directory exists;
2. stat each directory;
3. if its mode is broader than policy, call `chmod` to tighten it;
4. return an explicit startup error if tightening fails.

This applies to at least:

- local application data directory;
- device backup directory;
- instance backup directory;
- local DB parent directory when that directory is runtime-managed.

The bootstrap layer remains the right place for this because it already prepares runtime path layout and can stop startup before the process begins using insecure paths.

### Write-Time Enforcement

Code paths that create or overwrite sensitive files must write with explicit restrictive modes, not rely on process `umask`.

That means:

- restore marker writes use `0600`;
- backup files written by services use `0600`;
- backup archive sidecars or equivalent local metadata use `0600`;
- directories created by backup/restore helpers use `0700`.

If a file already exists and is broader than allowed, the owning code should tighten it before or immediately after writing.

### Restore-Path Enforcement

Restore staging and apply flows must use the same permission helpers so restore does not reintroduce looser modes.

Specific expectations:

- staging directory creation uses `0700`;
- staged `known_hosts` and restore marker use `0600`;
- replacement helpers preserve narrow modes on the final activated files they write or copy;
- restore should fail with explicit error if it cannot create or tighten a required sensitive path.

## Path Ownership Rules

To keep the slice safe and narrow, permission enforcement follows explicit ownership rules.

The runtime may automatically tighten:

- top-level runtime directories it creates and manages;
- sensitive files it writes directly;
- restore marker and restore staging artifacts;
- backup files and archives emitted by Theia itself.

The runtime should not attempt broad automatic chmod over arbitrary nested user-managed content just because it is located under a managed directory. If the application copies legacy content during restore, the restore flow only needs to guarantee the permissions of the destination containers and directly written sensitive files for this slice.

## Error Handling

Fail-closed behavior is required for sensitive runtime paths.

- If a managed sensitive path does not exist and cannot be created with the target mode, return an explicit error.
- If a managed sensitive path exists with broader permissions and cannot be tightened, return an explicit error.
- If a path exists with the wrong type, return an explicit error.
- The implementation never widens permissions to match content already on disk.

This behavior is intentionally stricter than today's best-effort defaults because the problem being addressed is production hygiene, not cosmetic consistency.

## Testing Strategy

Add focused tests around the shared hardening helpers and the affected runtime/service entry points.

Required coverage:

- creating a missing managed directory yields `0700`;
- creating a missing managed file yields `0600`;
- an existing managed directory with `0755` is tightened to `0700`;
- an existing managed file with `0644` is tightened to `0600`;
- wrong-type path returns explicit error;
- failure to tighten permissions returns explicit error;
- restore marker writing produces `0600`;
- restore staging directory creation produces `0700`;
- newly written device backup and instance backup artifacts use `0600`.

Tests should stay close to the code that owns the behavior:

- bootstrap/runtime tests for top-level directory preparation;
- service tests for backup and restore artifacts;
- avoid introducing a large integration suite unless unit-level coverage cannot express the contract clearly.

## Acceptance Criteria

This slice is complete when all of the following are true:

- runtime-managed sensitive directories are created with `0700`;
- runtime-managed sensitive files and metadata are written with `0600`;
- startup reconciles too-broad permissions on existing managed top-level runtime paths;
- restore staging and restore marker handling use the same restrictive policy;
- backup and instance-backup writes no longer default to world-readable files;
- failure to create or tighten a managed sensitive path stops the relevant runtime flow with explicit error;
- the implementation remains limited to known operational paths rather than a broad recursive chmod sweep.

## Follow-On Work

Deliberately deferred:

- ownership enforcement for deployed service accounts;
- recursive normalization tooling for historical backup trees;
- filesystem observability or audit commands for operators;
- broader secret-storage redesign for credentials and encryption material.
