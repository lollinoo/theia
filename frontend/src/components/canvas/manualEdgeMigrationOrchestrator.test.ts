/**
 * Exercises manual edge migration orchestrator topology canvas behavior so refactors preserve the documented contract.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Link } from '../../types/api';
import { exportCanvasDiagnostics, resetCanvasDiagnostics } from './canvasDiagnostics';
import { manualEdgeMigrationStorageKey, manualEdgeStorageKey } from './canvasHelpers';
import {
  type MigrateStoredManualEdgesOptions,
  readManualEdgeMigrationState,
} from './manualEdgeMigration';
import {
  prepareManualEdgeMigrationForTopologyLoad,
  recordSavedMapManualEdgeMigrationSkip,
  runDefaultMapManualEdgeMigrationForTopologyLoad,
} from './manualEdgeMigrationOrchestrator';

class FakeStorage implements Pick<Storage, 'getItem' | 'setItem' | 'removeItem'> {
  private readonly values = new Map<string, string>();

  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value);
  }

  removeItem(key: string): void {
    this.values.delete(key);
  }
}

function mockLink(overrides: Partial<Link> = {}): Link {
  return {
    id: 'link-1',
    source_device_id: 'dev-1',
    source_if_name: 'ether1',
    target_device_id: 'dev-2',
    target_if_name: 'ether2',
    discovery_protocol: 'lldp',
    source_if_speed: 1_000_000_000,
    source_if_oper_status: 'up',
    target_if_speed: 1_000_000_000,
    target_if_oper_status: 'up',
    ...overrides,
  };
}

describe('manual edge migration topology load orchestrator', () => {
  beforeEach(() => {
    resetCanvasDiagnostics();
  });

  it('prepares pending default-map migrations to bypass cached topology ETags', () => {
    const storage = new FakeStorage();
    storage.setItem(manualEdgeStorageKey, JSON.stringify([{ source: 'dev-1', target: 'dev-2' }]));

    const plan = prepareManualEdgeMigrationForTopologyLoad({ storage, mapId: null });

    expect(plan).toEqual({
      pendingStorageValue: JSON.stringify([{ source: 'dev-1', target: 'dev-2' }]),
      hadPendingManualEdgeMigration: true,
      canRunLegacyManualEdgeMigration: true,
      shouldBypassReadModelEtagForManualEdgeMigration: true,
    });
  });

  it('prepares saved-map pending migrations without allowing default-map migration', () => {
    const storage = new FakeStorage();
    storage.setItem(manualEdgeStorageKey, JSON.stringify([{ source: 'dev-1', target: 'dev-2' }]));

    const plan = prepareManualEdgeMigrationForTopologyLoad({ storage, mapId: 'saved-map-1' });

    expect(plan.hadPendingManualEdgeMigration).toBe(true);
    expect(plan.canRunLegacyManualEdgeMigration).toBe(false);
    expect(plan.shouldBypassReadModelEtagForManualEdgeMigration).toBe(false);
  });

  it('records saved-map migration skip diagnostics once per map and storage value', () => {
    const storage = new FakeStorage();
    const pendingValue = JSON.stringify([{ source: 'dev-1', target: 'dev-2' }]);
    storage.setItem(manualEdgeStorageKey, pendingValue);
    const plan = prepareManualEdgeMigrationForTopologyLoad({ storage, mapId: 'saved-map-1' });
    const skippedKeys = new Set<string>();

    recordSavedMapManualEdgeMigrationSkip({
      plan,
      mapId: 'saved-map-1',
      mapKey: 'map:saved-map-1',
      skippedKeys,
      topologyLoadMetadata: { reason: 'manual_refresh' },
    });
    recordSavedMapManualEdgeMigrationSkip({
      plan,
      mapId: 'saved-map-1',
      mapKey: 'map:saved-map-1',
      skippedKeys,
      topologyLoadMetadata: { reason: 'manual_refresh' },
    });

    const skippedEvents = exportCanvasDiagnostics().events.filter(
      (event) => event.event === 'manual_edges.migration.skipped_saved_map',
    );
    expect(skippedEvents).toHaveLength(1);
    expect(skippedEvents[0]).toMatchObject({
      level: 'info',
      metadata: expect.objectContaining({ mapId: 'saved-map-1', reason: 'manual_refresh' }),
    });
  });

  it('runs default-map migration and records diagnostics after the topology load is still current', async () => {
    const storage = new FakeStorage();
    storage.setItem(manualEdgeStorageKey, JSON.stringify([{ source: 'dev-1', target: 'dev-2' }]));
    const plan = prepareManualEdgeMigrationForTopologyLoad({ storage, mapId: null });
    const createLink = vi
      .fn<MigrateStoredManualEdgesOptions['createLink']>()
      .mockResolvedValue(undefined);

    const result = await runDefaultMapManualEdgeMigrationForTopologyLoad({
      plan,
      storage,
      existingLinks: [],
      createLink,
      isCurrentTopologyLoad: () => true,
    });

    expect(result).toEqual({ status: 'ran', appliedCount: 1 });
    expect(createLink).toHaveBeenCalledWith({
      source_device_id: 'dev-1',
      source_if_name: '',
      target_device_id: 'dev-2',
      target_if_name: '',
      migration_source: 'browser_localstorage',
    });
    expect(storage.getItem(manualEdgeStorageKey)).toBeNull();
    expect(exportCanvasDiagnostics().events.map((event) => event.event)).toContain(
      'manual_edges.migration.applied',
    );
  });

  it('does not publish migration diagnostics when the topology load becomes stale after migration', async () => {
    const storage = new FakeStorage();
    storage.setItem(manualEdgeStorageKey, JSON.stringify([{ source: 'dev-1', target: 'dev-2' }]));
    const plan = prepareManualEdgeMigrationForTopologyLoad({ storage, mapId: null });
    const createLink = vi
      .fn<MigrateStoredManualEdgesOptions['createLink']>()
      .mockResolvedValue(undefined);

    const result = await runDefaultMapManualEdgeMigrationForTopologyLoad({
      plan,
      storage,
      existingLinks: [],
      createLink,
      isCurrentTopologyLoad: () => false,
    });

    expect(result).toEqual({ status: 'stale', appliedCount: 1 });
    expect(readManualEdgeMigrationState(storage, manualEdgeMigrationStorageKey)).toMatchObject({
      status: 'applied',
      applied_count: 1,
    });
    expect(exportCanvasDiagnostics().events.map((event) => event.event)).not.toContain(
      'manual_edges.migration.applied',
    );
  });

  it('loads persisted default-map diagnostics when no pending storage exists', async () => {
    const storage = new FakeStorage();
    storage.setItem(
      manualEdgeMigrationStorageKey,
      JSON.stringify({
        schema_version: 1,
        status: 'failed',
        attempt_count: 3,
        pending_count: 0,
        applied_count: 0,
        failed_count: 1,
        skipped_count: 0,
        applied_keys: [],
        failed_keys: ['dev-1::dev-2'],
        last_error: 'still unavailable',
      }),
    );
    const plan = prepareManualEdgeMigrationForTopologyLoad({ storage, mapId: null });

    const result = await runDefaultMapManualEdgeMigrationForTopologyLoad({
      plan,
      storage,
      existingLinks: [mockLink()],
      createLink: vi.fn<MigrateStoredManualEdgesOptions['createLink']>(),
      isCurrentTopologyLoad: () => true,
    });

    expect(result).toEqual({ status: 'not-run', appliedCount: 0 });
    expect(exportCanvasDiagnostics().diagnostics.manualEdgeMigration).toMatchObject({
      status: 'failed',
      failedCount: 1,
      attemptCount: 3,
      lastError: 'still unavailable',
    });
  });
});
