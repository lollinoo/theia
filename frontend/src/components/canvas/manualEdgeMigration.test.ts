/**
 * Exercises manual edge migration topology canvas behavior so refactors preserve the documented contract.
 */
import { describe, expect, it, vi } from 'vitest';

import type { Link } from '../../types/api';
import {
  type ManualEdgeMigrationResult,
  type ManualEdgeMigrationState,
  type MigrateStoredManualEdgesOptions,
  manualEdgeMigrationKey,
  manualEdgeMigrationMaxAttempts,
  migrateStoredManualEdges,
  parseStoredManualEdges,
  readManualEdgeMigrationState,
  writeManualEdgeMigrationState,
} from './manualEdgeMigration';

class FakeStorage implements Pick<Storage, 'getItem' | 'setItem' | 'removeItem'> {
  readonly setCalls: Array<{ key: string; value: string }> = [];
  readonly removeCalls: string[] = [];

  private readonly values = new Map<string, string>();

  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value);
    this.setCalls.push({ key, value });
  }

  removeItem(key: string): void {
    this.values.delete(key);
    this.removeCalls.push(key);
  }
}

const pendingStorageKey = 'pending-manual-edges';
const stateStorageKey = 'manual-edge-migration-state';

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

function migrationState(
  overrides: Partial<ManualEdgeMigrationState> = {},
): ManualEdgeMigrationState {
  return {
    schema_version: 1,
    status: 'idle',
    attempt_count: 0,
    pending_count: 0,
    applied_count: 0,
    failed_count: 0,
    skipped_count: 0,
    applied_keys: [],
    failed_keys: [],
    ...overrides,
  };
}

describe('manual edge migration helpers', () => {
  it('is a storage no-op when pending manual edge storage is absent', async () => {
    const storage = new FakeStorage();
    const createLink = vi.fn<Parameters<typeof migrateStoredManualEdges>[0]['createLink']>();

    const result = await migrateStoredManualEdges({
      storage,
      pendingStorageKey,
      stateStorageKey,
      existingLinks: [],
      createLink,
      now: () => '2026-05-05T00:00:00.000Z',
    });

    expect(createLink).not.toHaveBeenCalled();
    expect(storage.setCalls).toEqual([]);
    expect(storage.removeCalls).toEqual([]);
    expect(result).toEqual({
      state: migrationState(),
      attemptedCount: 0,
      appliedCount: 0,
      failedCount: 0,
      skippedCount: 0,
    });
  });

  it('reads idle migration state when stored state is missing or malformed', () => {
    const storage = new FakeStorage();

    expect(readManualEdgeMigrationState(storage, stateStorageKey)).toEqual(migrationState());

    storage.setItem(stateStorageKey, '{not-json');
    expect(readManualEdgeMigrationState(storage, stateStorageKey)).toEqual(migrationState());

    storage.setItem(
      stateStorageKey,
      JSON.stringify({
        schema_version: 1,
        status: 'failed',
        attempt_count: 1,
      }),
    );
    expect(readManualEdgeMigrationState(storage, stateStorageKey)).toEqual(migrationState());
  });

  it('writes migration state as stable JSON storage', () => {
    const storage = new FakeStorage();
    const state = migrationState({
      status: 'failed',
      attempt_count: 2,
      pending_count: 1,
      applied_count: 1,
      failed_count: 1,
      applied_keys: ['dev-1::dev-2'],
      failed_keys: ['dev-3::dev-4'],
      last_attempt_at: '2026-05-05T00:00:00.000Z',
      last_completed_at: '2026-05-05T00:00:01.000Z',
      last_error: 'backend unavailable',
    });

    writeManualEdgeMigrationState(storage, stateStorageKey, state);

    expect(storage.getItem(stateStorageKey)).toBe(JSON.stringify(state));
  });

  it('parses valid edges with trimmed endpoints and stable de-duped keys', () => {
    const parsed = parseStoredManualEdges(
      JSON.stringify([
        { id: 'edge-1', source: ' dev-2 ', target: ' dev-1 ' },
        { id: 'edge-2', source: 'dev-1', target: 'dev-2' },
        { id: 'missing-source', target: 'dev-3' },
        { id: 'blank-source', source: '  ', target: 'dev-3' },
        { id: 'self-loop', source: 'dev-3', target: ' dev-3 ' },
        null,
      ]),
    );

    expect(manualEdgeMigrationKey(' dev-2 ', 'dev-1')).toBe('dev-1::dev-2');
    expect(parsed).toEqual([
      {
        id: 'edge-1',
        source: 'dev-2',
        target: 'dev-1',
        migrationKey: 'dev-1::dev-2',
      },
    ]);
  });

  it('returns an empty list for malformed stored JSON', () => {
    expect(parseStoredManualEdges('{not-json')).toEqual([]);
  });

  it('skips an existing topology link with reversed endpoints and clears pending storage', async () => {
    const storage = new FakeStorage();
    storage.setItem(pendingStorageKey, JSON.stringify([{ source: 'dev-1', target: 'dev-2' }]));
    const createLink = vi.fn<Parameters<typeof migrateStoredManualEdges>[0]['createLink']>();

    const options: MigrateStoredManualEdgesOptions = {
      storage,
      pendingStorageKey,
      stateStorageKey,
      existingLinks: [
        mockLink({
          source_device_id: 'dev-2',
          source_if_name: 'ether9',
          target_device_id: 'dev-1',
          target_if_name: 'ether8',
          discovery_protocol: 'manual',
        }),
      ],
      createLink,
      now: () => '2026-05-05T00:00:00.000Z',
    };

    const result: ManualEdgeMigrationResult = await migrateStoredManualEdges(options);

    expect(createLink).not.toHaveBeenCalled();
    expect(storage.getItem(pendingStorageKey)).toBeNull();
    expect(result).toMatchObject({
      attemptedCount: 0,
      appliedCount: 0,
      failedCount: 0,
      skippedCount: 1,
      state: {
        status: 'applied',
        attempt_count: 0,
        pending_count: 0,
        applied_count: 0,
        failed_count: 0,
        skipped_count: 1,
      },
    });
  });

  it('skips a stale pending edge when the key was previously applied', async () => {
    const storage = new FakeStorage();
    storage.setItem(pendingStorageKey, JSON.stringify([{ source: 'dev-1', target: 'dev-2' }]));
    storage.setItem(
      stateStorageKey,
      JSON.stringify(migrationState({ status: 'applied', applied_keys: ['dev-1::dev-2'] })),
    );
    const createLink = vi.fn<Parameters<typeof migrateStoredManualEdges>[0]['createLink']>();

    const result = await migrateStoredManualEdges({
      storage,
      pendingStorageKey,
      stateStorageKey,
      existingLinks: [],
      createLink,
      now: () => '2026-05-05T00:00:00.000Z',
    });

    expect(createLink).not.toHaveBeenCalled();
    expect(storage.getItem(pendingStorageKey)).toBeNull();
    expect(result.state).toMatchObject({
      status: 'applied',
      attempt_count: 0,
      pending_count: 0,
      applied_count: 1,
      failed_count: 0,
      skipped_count: 1,
      applied_keys: ['dev-1::dev-2'],
      failed_keys: [],
    });
  });

  it('keeps only rejected creates in pending storage and records failed state', async () => {
    const storage = new FakeStorage();
    storage.setItem(
      pendingStorageKey,
      JSON.stringify([
        { id: 'edge-1', source: 'dev-1', target: 'dev-2' },
        { id: 'edge-2', source: 'dev-3', target: 'dev-4' },
      ]),
    );
    const createLink = vi
      .fn<Parameters<typeof migrateStoredManualEdges>[0]['createLink']>()
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(new Error('backend unavailable'));

    const result = await migrateStoredManualEdges({
      storage,
      pendingStorageKey,
      stateStorageKey,
      existingLinks: [],
      createLink,
      now: () => '2026-05-05T00:00:00.000Z',
    });

    expect(createLink).toHaveBeenCalledTimes(2);
    expect(createLink).toHaveBeenNthCalledWith(1, {
      source_device_id: 'dev-1',
      source_if_name: '',
      target_device_id: 'dev-2',
      target_if_name: '',
      migration_source: 'browser_localstorage',
    });
    expect(createLink).toHaveBeenNthCalledWith(2, {
      source_device_id: 'dev-3',
      source_if_name: '',
      target_device_id: 'dev-4',
      target_if_name: '',
      migration_source: 'browser_localstorage',
    });
    expect(storage.getItem(pendingStorageKey)).toBe(
      JSON.stringify([{ id: 'edge-2', source: 'dev-3', target: 'dev-4' }]),
    );
    expect(result).toMatchObject({
      attemptedCount: 2,
      appliedCount: 1,
      failedCount: 1,
      skippedCount: 0,
      state: {
        status: 'failed',
        attempt_count: 1,
        pending_count: 1,
        applied_count: 1,
        failed_count: 1,
        skipped_count: 0,
        applied_keys: ['dev-1::dev-2'],
        failed_keys: ['dev-3::dev-4'],
        last_attempt_at: '2026-05-05T00:00:00.000Z',
        last_completed_at: '2026-05-05T00:00:00.000Z',
        last_error: 'backend unavailable',
      },
    });
  });

  it('stops retaining failed edges after the retry limit is reached', async () => {
    const storage = new FakeStorage();
    storage.setItem(pendingStorageKey, JSON.stringify([{ source: 'dev-3', target: 'dev-4' }]));
    storage.setItem(
      stateStorageKey,
      JSON.stringify(
        migrationState({
          status: 'failed',
          attempt_count: manualEdgeMigrationMaxAttempts - 1,
          pending_count: 1,
          failed_count: 1,
          failed_keys: ['dev-3::dev-4'],
          last_error: 'backend unavailable',
        }),
      ),
    );
    const createLink = vi
      .fn<Parameters<typeof migrateStoredManualEdges>[0]['createLink']>()
      .mockRejectedValueOnce(new Error('still unavailable'));

    const result = await migrateStoredManualEdges({
      storage,
      pendingStorageKey,
      stateStorageKey,
      existingLinks: [],
      createLink,
      now: () => '2026-05-05T00:02:00.000Z',
    });

    expect(createLink).toHaveBeenCalledTimes(1);
    expect(storage.getItem(pendingStorageKey)).toBeNull();
    expect(result).toMatchObject({
      attemptedCount: 1,
      appliedCount: 0,
      failedCount: 1,
      skippedCount: 0,
      state: {
        status: 'failed',
        attempt_count: manualEdgeMigrationMaxAttempts,
        pending_count: 0,
        applied_count: 0,
        failed_count: 1,
        failed_keys: ['dev-3::dev-4'],
        last_error: 'still unavailable',
      },
    });
  });

  it('clears exhausted pending failures without another create attempt', async () => {
    const storage = new FakeStorage();
    storage.setItem(pendingStorageKey, JSON.stringify([{ source: 'dev-3', target: 'dev-4' }]));
    storage.setItem(
      stateStorageKey,
      JSON.stringify(
        migrationState({
          status: 'failed',
          attempt_count: manualEdgeMigrationMaxAttempts,
          pending_count: 1,
          failed_count: 1,
          failed_keys: ['dev-3::dev-4'],
          last_error: 'backend unavailable',
        }),
      ),
    );
    const createLink = vi.fn<Parameters<typeof migrateStoredManualEdges>[0]['createLink']>();

    const result = await migrateStoredManualEdges({
      storage,
      pendingStorageKey,
      stateStorageKey,
      existingLinks: [],
      createLink,
      now: () => '2026-05-05T00:03:00.000Z',
    });

    expect(createLink).not.toHaveBeenCalled();
    expect(storage.getItem(pendingStorageKey)).toBeNull();
    expect(result).toMatchObject({
      attemptedCount: 0,
      appliedCount: 0,
      failedCount: 1,
      skippedCount: 0,
      state: {
        status: 'failed',
        attempt_count: manualEdgeMigrationMaxAttempts,
        pending_count: 0,
        failed_count: 1,
        failed_keys: ['dev-3::dev-4'],
        last_error: 'Manual edge migration retry limit reached',
      },
    });
  });

  it('marks retry attempts before a later successful retry is finalized', async () => {
    const storage = new FakeStorage();
    storage.setItem(pendingStorageKey, JSON.stringify([{ source: 'dev-3', target: 'dev-4' }]));
    storage.setItem(
      stateStorageKey,
      JSON.stringify(
        migrationState({
          status: 'failed',
          attempt_count: 1,
          pending_count: 1,
          failed_count: 1,
          failed_keys: ['dev-3::dev-4'],
          last_error: 'backend unavailable',
        }),
      ),
    );
    const createLink = vi
      .fn<Parameters<typeof migrateStoredManualEdges>[0]['createLink']>()
      .mockResolvedValue(undefined);

    const result = await migrateStoredManualEdges({
      storage,
      pendingStorageKey,
      stateStorageKey,
      existingLinks: [],
      createLink,
      now: () => '2026-05-05T00:01:00.000Z',
    });

    expect(createLink).toHaveBeenCalledWith({
      source_device_id: 'dev-3',
      source_if_name: '',
      target_device_id: 'dev-4',
      target_if_name: '',
      migration_source: 'browser_localstorage',
    });

    const stateWrites = storage.setCalls
      .filter((call) => call.key === stateStorageKey)
      .map((call) => JSON.parse(call.value) as ManualEdgeMigrationState);

    expect(stateWrites[stateWrites.length - 2]).toMatchObject({
      status: 'retried',
      attempt_count: 2,
      pending_count: 1,
      failed_keys: ['dev-3::dev-4'],
    });
    expect(result.state).toMatchObject({
      status: 'applied',
      attempt_count: 2,
      pending_count: 0,
      applied_count: 1,
      failed_count: 0,
      skipped_count: 0,
      applied_keys: ['dev-3::dev-4'],
      failed_keys: [],
    });
    expect(result.state.last_error).toBeUndefined();
    expect(storage.getItem(pendingStorageKey)).toBeNull();
  });

  it('finalizes malformed pending storage after a previous failed state without create attempts', async () => {
    const storage = new FakeStorage();
    storage.setItem(pendingStorageKey, '{not-json');
    storage.setItem(
      stateStorageKey,
      JSON.stringify(
        migrationState({
          status: 'failed',
          attempt_count: 1,
          pending_count: 1,
          failed_count: 1,
          failed_keys: ['dev-3::dev-4'],
          last_attempt_at: '2026-05-05T00:00:00.000Z',
          last_completed_at: '2026-05-05T00:00:01.000Z',
          last_error: 'backend unavailable',
        }),
      ),
    );
    const createLink = vi.fn<Parameters<typeof migrateStoredManualEdges>[0]['createLink']>();

    const result = await migrateStoredManualEdges({
      storage,
      pendingStorageKey,
      stateStorageKey,
      existingLinks: [],
      createLink,
      now: () => '2026-05-05T00:01:00.000Z',
    });

    expect(createLink).not.toHaveBeenCalled();
    expect(storage.getItem(pendingStorageKey)).toBeNull();
    expect(result).toMatchObject({
      attemptedCount: 0,
      appliedCount: 0,
      failedCount: 0,
      skippedCount: 0,
      state: {
        status: 'applied',
        attempt_count: 1,
        pending_count: 0,
        applied_count: 0,
        failed_count: 0,
        skipped_count: 0,
        applied_keys: [],
        failed_keys: [],
        last_attempt_at: '2026-05-05T00:00:00.000Z',
        last_completed_at: '2026-05-05T00:01:00.000Z',
      },
    });
    expect(result.state.last_error).toBeUndefined();
  });
});
