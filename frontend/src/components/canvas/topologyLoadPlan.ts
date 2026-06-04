import type { CanvasMeasurementTrigger } from './canvasInstrumentation';
import type { ManualEdgeMigrationTopologyLoadPlan } from './manualEdgeMigrationOrchestrator';

export interface TopologySourceRequestOptions {
  includeRuntimeBootstrap?: boolean;
}

export interface BuildTopologySourceRequestPlanInput {
  trigger: CanvasMeasurementTrigger;
  options: TopologySourceRequestOptions;
  mapKey: string;
  nodesOwnerMapKey: string;
  lastCanvasTopologyEtag: string | null;
  manualEdgeMigrationPlan: ManualEdgeMigrationTopologyLoadPlan;
}

export interface TopologySourceRequestPlan {
  includeRuntimeBootstrap: boolean;
  forceRuntimeBootstrap: boolean;
  etag: string | null;
}

interface BuildNotModifiedTopologyLoadPlanInput {
  responseEtag: string | undefined;
  lastCanvasTopologyEtag: string | null;
  forceFitView: boolean;
}

interface NotModifiedTopologyLoadPlan {
  etag: string | null;
  shouldFitView: boolean;
}

interface ShouldFitViewAfterTopologyLoadInput {
  trigger: CanvasMeasurementTrigger;
  forceFitView: boolean;
  usablePositionState: string;
}

/**
 * Builds the fetch flags for a topology load without touching hook state.
 * This keeps ETag bypass decisions tied to map ownership, bootstrap needs,
 * and pending manual-edge migration in one place.
 */
export function buildTopologySourceRequestPlan({
  trigger,
  options,
  mapKey,
  nodesOwnerMapKey,
  lastCanvasTopologyEtag,
  manualEdgeMigrationPlan,
}: BuildTopologySourceRequestPlanInput): TopologySourceRequestPlan {
  const includeRuntimeBootstrap =
    options.includeRuntimeBootstrap === true || trigger === 'initial_load';
  const forceRuntimeBootstrap = options.includeRuntimeBootstrap === true;
  const renderedNodesOwnedByMap = nodesOwnerMapKey === mapKey;
  const etag =
    includeRuntimeBootstrap ||
    manualEdgeMigrationPlan.shouldBypassReadModelEtagForManualEdgeMigration ||
    !renderedNodesOwnedByMap
      ? null
      : lastCanvasTopologyEtag;

  return {
    includeRuntimeBootstrap,
    forceRuntimeBootstrap,
    etag,
  };
}

/**
 * Normalizes a 304 response into the state updates the hook should still make,
 * including the fallback ETag used when the backend omits one.
 */
export function buildNotModifiedTopologyLoadPlan({
  responseEtag,
  lastCanvasTopologyEtag,
  forceFitView,
}: BuildNotModifiedTopologyLoadPlanInput): NotModifiedTopologyLoadPlan {
  return {
    etag: responseEtag ?? lastCanvasTopologyEtag,
    shouldFitView: forceFitView,
  };
}

/**
 * Decides whether a completed topology load should refit the viewport.
 * Ordinary refreshes keep the user's viewport once positions are usable.
 */
export function buildShouldFitViewAfterTopologyLoad({
  trigger,
  forceFitView,
  usablePositionState,
}: ShouldFitViewAfterTopologyLoadInput): boolean {
  return forceFitView || trigger === 'initial_load' || usablePositionState.length === 0;
}
