import type { MouseEvent as ReactMouseEvent } from 'react';

import type { Device, Link } from '../../types/api';
import type { AlertDTO, PrometheusStatusPayload, SnapshotPayload } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { measureCanvasWork } from './canvasInstrumentation';
import { buildRuntimeState } from './runtimeAdapters';
import {
  buildRuntimePatchPlan,
  hasRuntimePatchWork,
  patchRuntimeEdges,
  patchRuntimeNodes,
} from './runtimePatches';

type RuntimeNodeUpdater = (updater: (currentNodes: DeviceNode[]) => DeviceNode[]) => void;
type RuntimeEdgeUpdater = (updater: (currentEdges: LinkEdgeType[]) => LinkEdgeType[]) => void;

interface ApplyRuntimeSnapshotPatchOptions {
  previousSnapshot: SnapshotPayload | null;
  snapshot: SnapshotPayload;
  devices: Device[];
  links: Link[];
  alerts: AlertDTO[];
  prometheusStatus: PrometheusStatusPayload | null;
  setNodes: RuntimeNodeUpdater;
  setEdges: RuntimeEdgeUpdater;
  openEdgeMenu: (event: MouseEvent | ReactMouseEvent<SVGPathElement>, edgeID: string) => void;
  nodeIndexById?: ReadonlyMap<string, number>;
  edgeIndexById?: ReadonlyMap<string, number>;
}

// applyRuntimeSnapshotPatch applies runtime-only node and edge updates without recomposing topology.
export function applyRuntimeSnapshotPatch({
  previousSnapshot,
  snapshot,
  devices,
  links,
  alerts,
  prometheusStatus,
  setNodes,
  setEdges,
  openEdgeMenu,
  nodeIndexById,
  edgeIndexById,
}: ApplyRuntimeSnapshotPatchOptions): SnapshotPayload | null {
  let appliedSnapshot = previousSnapshot;

  measureCanvasWork('theia:canvas:snapshot-apply', 'snapshot', () => {
    if (devices.length === 0) {
      return;
    }

    const patchPlan = buildRuntimePatchPlan({
      previousSnapshot,
      nextSnapshot: snapshot,
      links,
    });
    appliedSnapshot = snapshot;

    if (!hasRuntimePatchWork(patchPlan)) {
      return;
    }

    const runtimeState = buildRuntimeState({
      devices,
      links,
      snapshot,
      alerts,
      prometheusStatus,
    });

    setNodes((currentNodes) =>
      patchRuntimeNodes({
        nodes: currentNodes,
        runtimeState,
        plan: patchPlan,
        nodeIndexById,
      }),
    );
    setEdges((currentEdges) =>
      patchRuntimeEdges({
        edges: currentEdges,
        links,
        runtimeState,
        alerts,
        onEdgeContextMenu: openEdgeMenu,
        plan: patchPlan,
        edgeIndexById,
      }),
    );
  });

  return appliedSnapshot;
}
