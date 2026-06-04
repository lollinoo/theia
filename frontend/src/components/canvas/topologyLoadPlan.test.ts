import { describe, expect, it } from 'vitest';

import type { ManualEdgeMigrationTopologyLoadPlan } from './manualEdgeMigrationOrchestrator';
import {
  buildNotModifiedTopologyLoadPlan,
  buildShouldFitViewAfterTopologyLoad,
  buildTopologySourceRequestPlan,
} from './topologyLoadPlan';

function manualMigrationPlan(
  overrides: Partial<ManualEdgeMigrationTopologyLoadPlan> = {},
): ManualEdgeMigrationTopologyLoadPlan {
  return {
    pendingStorageValue: null,
    hadPendingManualEdgeMigration: false,
    canRunLegacyManualEdgeMigration: true,
    shouldBypassReadModelEtagForManualEdgeMigration: false,
    ...overrides,
  };
}

describe('topologyLoadPlan', () => {
  it('requests runtime bootstrap on initial load and bypasses cached ETags', () => {
    expect(
      buildTopologySourceRequestPlan({
        trigger: 'initial_load',
        options: {},
        mapKey: 'default:',
        nodesOwnerMapKey: 'default:',
        lastCanvasTopologyEtag: '"topo-1"',
        manualEdgeMigrationPlan: manualMigrationPlan(),
      }),
    ).toEqual({
      includeRuntimeBootstrap: true,
      forceRuntimeBootstrap: false,
      etag: null,
    });
  });

  it('forces runtime bootstrap when explicitly requested', () => {
    expect(
      buildTopologySourceRequestPlan({
        trigger: 'backend_resync',
        options: { includeRuntimeBootstrap: true },
        mapKey: 'default:',
        nodesOwnerMapKey: 'default:',
        lastCanvasTopologyEtag: '"topo-1"',
        manualEdgeMigrationPlan: manualMigrationPlan(),
      }),
    ).toEqual({
      includeRuntimeBootstrap: true,
      forceRuntimeBootstrap: true,
      etag: null,
    });
  });

  it('bypasses cached ETags while default-map manual edge migration is pending', () => {
    expect(
      buildTopologySourceRequestPlan({
        trigger: 'manual_refresh',
        options: {},
        mapKey: 'default:',
        nodesOwnerMapKey: 'default:',
        lastCanvasTopologyEtag: '"topo-1"',
        manualEdgeMigrationPlan: manualMigrationPlan({
          pendingStorageValue: '[{"source":"dev-1","target":"dev-2"}]',
          hadPendingManualEdgeMigration: true,
          shouldBypassReadModelEtagForManualEdgeMigration: true,
        }),
      }),
    ).toEqual({
      includeRuntimeBootstrap: false,
      forceRuntimeBootstrap: false,
      etag: null,
    });
  });

  it('bypasses cached ETags when rendered nodes belong to a different map', () => {
    expect(
      buildTopologySourceRequestPlan({
        trigger: 'manual_refresh',
        options: {},
        mapKey: 'map:new',
        nodesOwnerMapKey: 'map:old',
        lastCanvasTopologyEtag: '"topo-1"',
        manualEdgeMigrationPlan: manualMigrationPlan(),
      }),
    ).toEqual({
      includeRuntimeBootstrap: false,
      forceRuntimeBootstrap: false,
      etag: null,
    });
  });

  it('uses the cached ETag for unchanged non-bootstrap loads owned by the active map', () => {
    expect(
      buildTopologySourceRequestPlan({
        trigger: 'manual_refresh',
        options: {},
        mapKey: 'map:current',
        nodesOwnerMapKey: 'map:current',
        lastCanvasTopologyEtag: '"topo-1"',
        manualEdgeMigrationPlan: manualMigrationPlan(),
      }),
    ).toEqual({
      includeRuntimeBootstrap: false,
      forceRuntimeBootstrap: false,
      etag: '"topo-1"',
    });
  });

  it('keeps the response ETag for not-modified topology loads', () => {
    expect(
      buildNotModifiedTopologyLoadPlan({
        responseEtag: '"topo-2"',
        lastCanvasTopologyEtag: '"topo-1"',
        forceFitView: false,
      }),
    ).toEqual({
      etag: '"topo-2"',
      shouldFitView: false,
    });
  });

  it('falls back to the previous ETag for not-modified loads without a response ETag', () => {
    expect(
      buildNotModifiedTopologyLoadPlan({
        responseEtag: undefined,
        lastCanvasTopologyEtag: '"topo-1"',
        forceFitView: true,
      }),
    ).toEqual({
      etag: '"topo-1"',
      shouldFitView: true,
    });
  });

  it('fits view after forced, initial, or unpositioned topology loads', () => {
    expect(
      buildShouldFitViewAfterTopologyLoad({
        trigger: 'manual_refresh',
        forceFitView: true,
        usablePositionState: 'dev-1',
      }),
    ).toBe(true);
    expect(
      buildShouldFitViewAfterTopologyLoad({
        trigger: 'initial_load',
        forceFitView: false,
        usablePositionState: 'dev-1',
      }),
    ).toBe(true);
    expect(
      buildShouldFitViewAfterTopologyLoad({
        trigger: 'manual_refresh',
        forceFitView: false,
        usablePositionState: '',
      }),
    ).toBe(true);
  });

  it('skips fitView for ordinary refreshes with usable positions', () => {
    expect(
      buildShouldFitViewAfterTopologyLoad({
        trigger: 'manual_refresh',
        forceFitView: false,
        usablePositionState: 'dev-1',
      }),
    ).toBe(false);
  });
});
