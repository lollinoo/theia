/**
 * Defines manual edge migration diagnostics behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import { recordCanvasDiagnosticEvent, updateCanvasDiagnosticsState } from './canvasDiagnostics';
import { manualEdgeMigrationStorageKey } from './canvasHelpers';
import {
  type ManualEdgeMigrationResult,
  type ManualEdgeMigrationState,
  readManualEdgeMigrationState,
} from './manualEdgeMigration';

// manualEdgeMigrationHasVisibleResult decides whether migration diagnostics should be surfaced.
function manualEdgeMigrationHasVisibleResult(
  result: ManualEdgeMigrationResult,
  hadPendingStorage: boolean,
): boolean {
  return (
    hadPendingStorage ||
    result.attemptedCount > 0 ||
    result.appliedCount > 0 ||
    result.failedCount > 0 ||
    result.skippedCount > 0 ||
    manualEdgeMigrationStateHasVisibleResult(result.state)
  );
}

// manualEdgeMigrationStateHasVisibleResult detects persisted migration state worth reporting.
function manualEdgeMigrationStateHasVisibleResult(state: ManualEdgeMigrationState): boolean {
  return (
    state.status !== 'idle' ||
    state.attempt_count > 0 ||
    state.pending_count > 0 ||
    state.applied_count > 0 ||
    state.failed_count > 0 ||
    state.skipped_count > 0 ||
    state.last_error !== undefined
  );
}

// updateManualEdgeMigrationDiagnosticsState mirrors migration state into canvas diagnostics.
function updateManualEdgeMigrationDiagnosticsState(state: ManualEdgeMigrationState): void {
  updateCanvasDiagnosticsState({
    manualEdgeMigration: {
      status: state.status,
      pendingCount: state.pending_count,
      appliedCount: state.applied_count,
      failedCount: state.failed_count,
      skippedCount: state.skipped_count,
      attemptCount: state.attempt_count,
      lastAttemptAt: state.last_attempt_at,
      lastCompletedAt: state.last_completed_at,
      lastError: state.last_error,
    },
  });
}

// recordPersistedManualEdgeMigrationDiagnostics restores visible migration diagnostics from storage.
export function recordPersistedManualEdgeMigrationDiagnostics(
  storage: Pick<Storage, 'getItem'>,
): void {
  if (storage.getItem(manualEdgeMigrationStorageKey) === null) {
    return;
  }

  const state = readManualEdgeMigrationState(storage, manualEdgeMigrationStorageKey);
  if (manualEdgeMigrationStateHasVisibleResult(state)) {
    updateManualEdgeMigrationDiagnosticsState(state);
  }
}

// recordManualEdgeMigrationDiagnostics records migration state plus applied/failed/skipped events.
export function recordManualEdgeMigrationDiagnostics(
  result: ManualEdgeMigrationResult,
  hadPendingStorage: boolean,
): void {
  if (!manualEdgeMigrationHasVisibleResult(result, hadPendingStorage)) {
    return;
  }

  const state = result.state;
  updateManualEdgeMigrationDiagnosticsState(state);

  const metadata = {
    status: state.status,
    attemptCount: state.attempt_count,
    pendingCount: state.pending_count,
    appliedCount: result.appliedCount,
    failedCount: result.failedCount,
    skippedCount: result.skippedCount,
  };

  if (result.appliedCount > 0) {
    recordCanvasDiagnosticEvent({
      level: 'info',
      source: 'topology',
      event: 'manual_edges.migration.applied',
      message: 'Manual edge localStorage migration applied',
      metadata,
    });
  }

  if (result.failedCount > 0) {
    recordCanvasDiagnosticEvent({
      level: 'warn',
      source: 'topology',
      event: 'manual_edges.migration.failed',
      message: 'Manual edge localStorage migration failed',
      metadata: {
        ...metadata,
        error: state.last_error,
      },
    });
  }

  if (result.skippedCount > 0) {
    recordCanvasDiagnosticEvent({
      level: 'info',
      source: 'topology',
      event: 'manual_edges.migration.skipped',
      message: 'Manual edge localStorage migration skipped existing links',
      metadata,
    });
  }
}
