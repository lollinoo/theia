/**
 * Defines manual edge migration orchestrator behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { Link } from '../../types/api';
import { recordCanvasDiagnosticEvent } from './canvasDiagnostics';
import { manualEdgeMigrationStorageKey, manualEdgeStorageKey } from './canvasHelpers';
import {
  type MigrateStoredManualEdgesOptions,
  migrateStoredManualEdges,
} from './manualEdgeMigration';
import {
  recordManualEdgeMigrationDiagnostics,
  recordPersistedManualEdgeMigrationDiagnostics,
} from './manualEdgeMigrationDiagnostics';

/** Describes the manual edge migration topology load plan contract used by the topology canvas. */
export interface ManualEdgeMigrationTopologyLoadPlan {
  pendingStorageValue: string | null;
  hadPendingManualEdgeMigration: boolean;
  canRunLegacyManualEdgeMigration: boolean;
  shouldBypassReadModelEtagForManualEdgeMigration: boolean;
}

interface PrepareManualEdgeMigrationForTopologyLoadOptions {
  storage: Pick<Storage, 'getItem'>;
  mapId: string | null;
  pendingStorageKey?: string;
}

interface RecordSavedMapManualEdgeMigrationSkipOptions {
  plan: ManualEdgeMigrationTopologyLoadPlan;
  mapId: string | null;
  mapKey: string;
  skippedKeys: Set<string>;
  topologyLoadMetadata: Record<string, unknown>;
}

/** Describes the manual edge migration topology load run result contract used by the topology canvas. */
export type ManualEdgeMigrationTopologyLoadRunResult =
  | { status: 'not-run'; appliedCount: 0 }
  | { status: 'ran'; appliedCount: number }
  | { status: 'stale'; appliedCount: number };

interface RunDefaultMapManualEdgeMigrationForTopologyLoadOptions {
  plan: ManualEdgeMigrationTopologyLoadPlan;
  storage: Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;
  existingLinks: Link[];
  createLink: MigrateStoredManualEdgesOptions['createLink'];
  isCurrentTopologyLoad: () => boolean;
  pendingStorageKey?: string;
  stateStorageKey?: string;
}

// prepareManualEdgeMigrationForTopologyLoad decides if legacy edge migration can run for this map.
export function prepareManualEdgeMigrationForTopologyLoad({
  storage,
  mapId,
  pendingStorageKey = manualEdgeStorageKey,
}: PrepareManualEdgeMigrationForTopologyLoadOptions): ManualEdgeMigrationTopologyLoadPlan {
  const pendingStorageValue = storage.getItem(pendingStorageKey);
  const hadPendingManualEdgeMigration = pendingStorageValue !== null;
  const canRunLegacyManualEdgeMigration = mapId === null;

  return {
    pendingStorageValue,
    hadPendingManualEdgeMigration,
    canRunLegacyManualEdgeMigration,
    shouldBypassReadModelEtagForManualEdgeMigration:
      canRunLegacyManualEdgeMigration && hadPendingManualEdgeMigration,
  };
}

// recordSavedMapManualEdgeMigrationSkip records one diagnostic when saved maps skip legacy migration.
export function recordSavedMapManualEdgeMigrationSkip({
  plan,
  mapId,
  mapKey,
  skippedKeys,
  topologyLoadMetadata,
}: RecordSavedMapManualEdgeMigrationSkipOptions): void {
  if (
    !plan.hadPendingManualEdgeMigration ||
    plan.canRunLegacyManualEdgeMigration ||
    plan.pendingStorageValue === null
  ) {
    return;
  }

  const skipDiagnosticKey = `${mapKey}:${plan.pendingStorageValue}`;
  if (skippedKeys.has(skipDiagnosticKey)) {
    return;
  }

  skippedKeys.add(skipDiagnosticKey);
  recordCanvasDiagnosticEvent({
    level: 'info',
    source: 'topology',
    event: 'manual_edges.migration.skipped_saved_map',
    message: 'Manual edge localStorage migration skipped for saved map',
    metadata: { ...topologyLoadMetadata, mapId },
  });
}

// runDefaultMapManualEdgeMigrationForTopologyLoad migrates legacy edges only for the default map.
export async function runDefaultMapManualEdgeMigrationForTopologyLoad({
  plan,
  storage,
  existingLinks,
  createLink,
  isCurrentTopologyLoad,
  pendingStorageKey = manualEdgeStorageKey,
  stateStorageKey = manualEdgeMigrationStorageKey,
}: RunDefaultMapManualEdgeMigrationForTopologyLoadOptions): Promise<ManualEdgeMigrationTopologyLoadRunResult> {
  if (plan.hadPendingManualEdgeMigration && plan.canRunLegacyManualEdgeMigration) {
    const result = await migrateStoredManualEdges({
      storage,
      pendingStorageKey,
      stateStorageKey,
      existingLinks,
      createLink,
    });

    if (!isCurrentTopologyLoad()) {
      return { status: 'stale', appliedCount: result.appliedCount };
    }

    recordManualEdgeMigrationDiagnostics(result, plan.hadPendingManualEdgeMigration);
    return { status: 'ran', appliedCount: result.appliedCount };
  }

  if (plan.canRunLegacyManualEdgeMigration) {
    recordPersistedManualEdgeMigrationDiagnostics(storage);
  }

  return { status: 'not-run', appliedCount: 0 };
}
