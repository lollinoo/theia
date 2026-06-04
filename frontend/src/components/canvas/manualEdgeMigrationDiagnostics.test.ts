import { beforeEach, describe, expect, it } from 'vitest';

import { exportCanvasDiagnostics, resetCanvasDiagnostics } from './canvasDiagnostics';
import { manualEdgeMigrationStorageKey } from './canvasHelpers';
import type { ManualEdgeMigrationResult } from './manualEdgeMigration';
import {
  recordManualEdgeMigrationDiagnostics,
  recordPersistedManualEdgeMigrationDiagnostics,
} from './manualEdgeMigrationDiagnostics';

function migrationResult(
  overrides: Partial<ManualEdgeMigrationResult> = {},
): ManualEdgeMigrationResult {
  return {
    attemptedCount: 1,
    appliedCount: 1,
    failedCount: 0,
    skippedCount: 0,
    state: {
      schema_version: 1,
      status: 'applied',
      attempt_count: 1,
      pending_count: 0,
      applied_count: 1,
      failed_count: 0,
      skipped_count: 0,
      applied_keys: ['dev-a:ether1->dev-b:ether2'],
      failed_keys: [],
    },
    ...overrides,
  };
}

describe('manual edge migration diagnostics', () => {
  beforeEach(() => {
    resetCanvasDiagnostics();
  });

  it('records persisted visible migration state', () => {
    const state = {
      schema_version: 1,
      status: 'failed',
      attempt_count: 3,
      pending_count: 2,
      applied_count: 1,
      failed_count: 1,
      skipped_count: 0,
      applied_keys: ['applied-key'],
      failed_keys: ['failed-key'],
      last_error: 'offline',
    };
    const storage = {
      getItem(key: string) {
        return key === manualEdgeMigrationStorageKey ? JSON.stringify(state) : null;
      },
    };

    recordPersistedManualEdgeMigrationDiagnostics(storage);

    expect(exportCanvasDiagnostics().diagnostics.manualEdgeMigration).toMatchObject({
      status: 'failed',
      pendingCount: 2,
      appliedCount: 1,
      failedCount: 1,
      attemptCount: 3,
      lastError: 'offline',
    });
  });

  it('records applied migration result state and event metadata', () => {
    recordManualEdgeMigrationDiagnostics(migrationResult(), true);

    const diagnostics = exportCanvasDiagnostics();
    expect(diagnostics.diagnostics.manualEdgeMigration).toMatchObject({
      status: 'applied',
      appliedCount: 1,
      failedCount: 0,
      skippedCount: 0,
      attemptCount: 1,
    });
    expect(diagnostics.events).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          level: 'info',
          event: 'manual_edges.migration.applied',
          metadata: expect.objectContaining({ appliedCount: 1 }),
        }),
      ]),
    );
  });

  it('skips invisible idle results without persisted storage', () => {
    recordManualEdgeMigrationDiagnostics(
      migrationResult({
        attemptedCount: 0,
        appliedCount: 0,
        state: {
          schema_version: 1,
          status: 'idle',
          attempt_count: 0,
          pending_count: 0,
          applied_count: 0,
          failed_count: 0,
          skipped_count: 0,
          applied_keys: [],
          failed_keys: [],
        },
      }),
      false,
    );

    const diagnostics = exportCanvasDiagnostics();
    expect(diagnostics.diagnostics.manualEdgeMigration).toMatchObject({
      status: 'idle',
      attemptCount: 0,
    });
    expect(diagnostics.events).toEqual([]);
  });
});
