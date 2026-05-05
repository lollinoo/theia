import type { Link } from '../../types/api';

export interface LegacyManualEdge {
  id?: string;
  source: string;
  target: string;
  migrationKey: string;
}

export type ManualEdgeMigrationStatus = 'idle' | 'pending' | 'retried' | 'applied' | 'failed';
export const manualEdgeMigrationMaxAttempts = 3;

export interface ManualEdgeMigrationState {
  schema_version: 1;
  status: ManualEdgeMigrationStatus;
  attempt_count: number;
  pending_count: number;
  applied_count: number;
  failed_count: number;
  skipped_count: number;
  applied_keys: string[];
  failed_keys: string[];
  last_attempt_at?: string;
  last_completed_at?: string;
  last_error?: string;
}

interface StoredManualEdge {
  id?: string;
  source: string;
  target: string;
}

export interface MigrateStoredManualEdgesOptions {
  storage: Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;
  pendingStorageKey: string;
  stateStorageKey: string;
  existingLinks: Link[];
  createLink: (payload: {
    source_device_id: string;
    source_if_name: string;
    target_device_id: string;
    target_if_name: string;
    migration_source?: 'browser_localstorage';
  }) => Promise<unknown>;
  now?: () => string;
}

export interface ManualEdgeMigrationResult {
  state: ManualEdgeMigrationState;
  attemptedCount: number;
  appliedCount: number;
  failedCount: number;
  skippedCount: number;
}

const migrationStatuses = new Set<ManualEdgeMigrationStatus>([
  'idle',
  'pending',
  'retried',
  'applied',
  'failed',
]);
const retryLimitError = 'Manual edge migration retry limit reached';

function idleManualEdgeMigrationState(): ManualEdgeMigrationState {
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
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isNonNegativeInteger(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value >= 0;
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((entry) => typeof entry === 'string');
}

function sortedUniqueKeys(keys: Iterable<string>): string[] {
  return [...new Set(keys)].sort();
}

function buildMigrationState({
  status,
  attemptCount,
  pendingCount,
  appliedKeys,
  failedKeys,
  skippedCount,
  lastAttemptAt,
  lastCompletedAt,
  lastError,
}: {
  status: ManualEdgeMigrationStatus;
  attemptCount: number;
  pendingCount: number;
  appliedKeys: string[];
  failedKeys: string[];
  skippedCount: number;
  lastAttemptAt?: string;
  lastCompletedAt?: string;
  lastError?: string;
}): ManualEdgeMigrationState {
  const normalizedAppliedKeys = sortedUniqueKeys(appliedKeys);
  const normalizedFailedKeys = sortedUniqueKeys(failedKeys);
  const state: ManualEdgeMigrationState = {
    schema_version: 1,
    status,
    attempt_count: attemptCount,
    pending_count: pendingCount,
    applied_count: normalizedAppliedKeys.length,
    failed_count: normalizedFailedKeys.length,
    skipped_count: skippedCount,
    applied_keys: normalizedAppliedKeys,
    failed_keys: normalizedFailedKeys,
  };

  if (lastAttemptAt !== undefined) {
    state.last_attempt_at = lastAttemptAt;
  }
  if (lastCompletedAt !== undefined) {
    state.last_completed_at = lastCompletedAt;
  }
  if (lastError !== undefined) {
    state.last_error = lastError;
  }

  return state;
}

function parseMigrationState(value: unknown): ManualEdgeMigrationState | null {
  if (!isRecord(value)) {
    return null;
  }

  if (
    value.schema_version !== 1 ||
    !migrationStatuses.has(value.status as ManualEdgeMigrationStatus) ||
    !isNonNegativeInteger(value.attempt_count) ||
    !isNonNegativeInteger(value.pending_count) ||
    !isNonNegativeInteger(value.applied_count) ||
    !isNonNegativeInteger(value.failed_count) ||
    !isNonNegativeInteger(value.skipped_count) ||
    !isStringArray(value.applied_keys) ||
    !isStringArray(value.failed_keys)
  ) {
    return null;
  }

  const state = buildMigrationState({
    status: value.status as ManualEdgeMigrationStatus,
    attemptCount: value.attempt_count,
    pendingCount: value.pending_count,
    appliedKeys: value.applied_keys,
    failedKeys: value.failed_keys,
    skippedCount: value.skipped_count,
    lastAttemptAt: typeof value.last_attempt_at === 'string' ? value.last_attempt_at : undefined,
    lastCompletedAt:
      typeof value.last_completed_at === 'string' ? value.last_completed_at : undefined,
    lastError: typeof value.last_error === 'string' ? value.last_error : undefined,
  });

  return state;
}

function storedManualEdge(edge: LegacyManualEdge): StoredManualEdge {
  if (edge.id !== undefined) {
    return {
      id: edge.id,
      source: edge.source,
      target: edge.target,
    };
  }

  return {
    source: edge.source,
    target: edge.target,
  };
}

function safeErrorMessage(reason: unknown): string {
  if (reason instanceof Error && reason.message.trim().length > 0) {
    return reason.message;
  }

  if (typeof reason === 'string' && reason.trim().length > 0) {
    return reason;
  }

  return 'Unknown migration error';
}

export function manualEdgeMigrationKey(source: string, target: string): string {
  return [source.trim(), target.trim()].sort().join('::');
}

export function parseStoredManualEdges(raw: string | null): LegacyManualEdge[] {
  if (raw === null) {
    return [];
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }

  if (!Array.isArray(parsed)) {
    return [];
  }

  const seenKeys = new Set<string>();
  const edges: LegacyManualEdge[] = [];

  for (const entry of parsed) {
    if (!isRecord(entry) || typeof entry.source !== 'string' || typeof entry.target !== 'string') {
      continue;
    }

    const source = entry.source.trim();
    const target = entry.target.trim();
    if (source.length === 0 || target.length === 0 || source === target) {
      continue;
    }

    const migrationKey = manualEdgeMigrationKey(source, target);
    if (seenKeys.has(migrationKey)) {
      continue;
    }

    seenKeys.add(migrationKey);
    const edge: LegacyManualEdge = {
      source,
      target,
      migrationKey,
    };

    if (typeof entry.id === 'string') {
      edge.id = entry.id;
    }

    edges.push(edge);
  }

  return edges;
}

export function readManualEdgeMigrationState(
  storage: Pick<Storage, 'getItem'>,
  stateStorageKey: string,
): ManualEdgeMigrationState {
  const raw = storage.getItem(stateStorageKey);
  if (raw === null) {
    return idleManualEdgeMigrationState();
  }

  try {
    return parseMigrationState(JSON.parse(raw)) ?? idleManualEdgeMigrationState();
  } catch {
    return idleManualEdgeMigrationState();
  }
}

export function writeManualEdgeMigrationState(
  storage: Pick<Storage, 'setItem'>,
  stateStorageKey: string,
  state: ManualEdgeMigrationState,
): void {
  storage.setItem(stateStorageKey, JSON.stringify(state));
}

export async function migrateStoredManualEdges({
  storage,
  pendingStorageKey,
  stateStorageKey,
  existingLinks,
  createLink,
  now = () => new Date().toISOString(),
}: MigrateStoredManualEdgesOptions): Promise<ManualEdgeMigrationResult> {
  const previous = readManualEdgeMigrationState(storage, stateStorageKey);
  const pendingRaw = storage.getItem(pendingStorageKey);
  if (pendingRaw === null) {
    return {
      state: previous,
      attemptedCount: 0,
      appliedCount: 0,
      failedCount: 0,
      skippedCount: 0,
    };
  }

  const pendingEdges = parseStoredManualEdges(pendingRaw);
  const previousAppliedKeys = new Set(previous.applied_keys);
  const previousFailedKeys = new Set(previous.failed_keys);
  const existingLinkKeys = new Set(
    existingLinks.map((link) =>
      manualEdgeMigrationKey(link.source_device_id, link.target_device_id),
    ),
  );
  const skippedEdges: LegacyManualEdge[] = [];
  const exhaustedEdges: LegacyManualEdge[] = [];
  const edgesToAttempt: LegacyManualEdge[] = [];

  for (const edge of pendingEdges) {
    if (previousAppliedKeys.has(edge.migrationKey) || existingLinkKeys.has(edge.migrationKey)) {
      skippedEdges.push(edge);
      continue;
    }

    if (
      previousFailedKeys.has(edge.migrationKey) &&
      previous.attempt_count >= manualEdgeMigrationMaxAttempts
    ) {
      exhaustedEdges.push(edge);
      continue;
    }

    edgesToAttempt.push(edge);
  }

  const attemptedCount = edgesToAttempt.length;
  let attemptCount = previous.attempt_count;
  let lastAttemptAt = previous.last_attempt_at;
  let results: PromiseSettledResult<unknown>[] = [];

  if (attemptedCount > 0) {
    attemptCount += 1;
    lastAttemptAt = now();
    const isRetry =
      previous.status === 'failed' ||
      edgesToAttempt.some((edge) => previousFailedKeys.has(edge.migrationKey));
    const inProgressState = buildMigrationState({
      status: isRetry ? 'retried' : 'pending',
      attemptCount,
      pendingCount: attemptedCount,
      appliedKeys: previous.applied_keys,
      failedKeys: previous.failed_keys,
      skippedCount: skippedEdges.length,
      lastAttemptAt,
      lastCompletedAt: previous.last_completed_at,
    });

    writeManualEdgeMigrationState(storage, stateStorageKey, inProgressState);
    results = await Promise.allSettled(
      edgesToAttempt.map((edge) =>
        createLink({
          source_device_id: edge.source,
          source_if_name: '',
          target_device_id: edge.target,
          target_if_name: '',
          migration_source: 'browser_localstorage',
        }),
      ),
    );
  }

  const appliedKeys: string[] = [];
  const failedEdges: LegacyManualEdge[] = [];
  let firstFailureReason: unknown;

  for (let index = 0; index < results.length; index += 1) {
    const result = results[index];
    const edge = edgesToAttempt[index];
    if (result === undefined || edge === undefined) {
      continue;
    }

    if (result.status === 'fulfilled') {
      appliedKeys.push(edge.migrationKey);
      continue;
    }

    failedEdges.push(edge);
    firstFailureReason ??= result.reason;
  }

  const retryLimitReached =
    failedEdges.length > 0 && attemptCount >= manualEdgeMigrationMaxAttempts;
  const pendingFailedEdges = retryLimitReached ? [] : failedEdges;
  const terminalFailedEdges = retryLimitReached
    ? [...exhaustedEdges, ...failedEdges]
    : exhaustedEdges;

  if (pendingFailedEdges.length === 0) {
    storage.removeItem(pendingStorageKey);
  } else {
    storage.setItem(pendingStorageKey, JSON.stringify(pendingFailedEdges.map(storedManualEdge)));
  }

  const finalFailedKeys = [...pendingFailedEdges, ...terminalFailedEdges].map(
    (edge) => edge.migrationKey,
  );
  const lastError =
    firstFailureReason === undefined
      ? terminalFailedEdges.length > 0
        ? retryLimitError
        : undefined
      : safeErrorMessage(firstFailureReason);
  const finalState = buildMigrationState({
    status: finalFailedKeys.length === 0 ? 'applied' : 'failed',
    attemptCount,
    pendingCount: pendingFailedEdges.length,
    appliedKeys: [...previous.applied_keys, ...appliedKeys],
    failedKeys: finalFailedKeys,
    skippedCount: skippedEdges.length,
    lastAttemptAt,
    lastCompletedAt: now(),
    lastError,
  });

  writeManualEdgeMigrationState(storage, stateStorageKey, finalState);

  return {
    state: finalState,
    attemptedCount,
    appliedCount: appliedKeys.length,
    failedCount: finalFailedKeys.length,
    skippedCount: skippedEdges.length,
  };
}
