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
