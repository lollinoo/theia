/**
 * Defines runtime snapshot patch behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { Dispatch, MouseEvent as ReactMouseEvent, SetStateAction } from 'react';

import type { Device, Link } from '../../types/api';
import type { AlertDTO, PrometheusStatusPayload, SnapshotPayload } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { measureCanvasWork } from './canvasInstrumentation';
import { buildRuntimeState, type RuntimeState } from './runtimeAdapters';
import {
  buildRuntimePatchPlan,
  hasRuntimePatchWork,
  patchRuntimeDevices,
  patchRuntimeEdges,
  patchRuntimeNodes,
} from './runtimePatches';

type RuntimeNodeUpdater = (updater: (currentNodes: DeviceNode[]) => DeviceNode[]) => void;
type RuntimeEdgeUpdater = (updater: (currentEdges: LinkEdgeType[]) => LinkEdgeType[]) => void;

export interface ApplyRuntimeSnapshotPatchInput {
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
  runtimeState?: RuntimeState;
  setDevices?: Dispatch<SetStateAction<Device[]>>;
}

export type ApplyRuntimeSnapshotPatchOptions = ApplyRuntimeSnapshotPatchInput;

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
  runtimeState,
  setDevices,
}: ApplyRuntimeSnapshotPatchInput): SnapshotPayload | null {
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

    const nextRuntimeState =
      runtimeState ??
      buildRuntimeState({
        devices,
        links,
        snapshot,
        alerts,
        prometheusStatus,
      });

    if (setDevices && patchPlan.deviceIds.size > 0) {
      setDevices((currentDevices) =>
        patchRuntimeDevices({
          devices: currentDevices,
          runtimeState: nextRuntimeState,
          plan: patchPlan,
        }),
      );
    }

    setNodes((currentNodes) =>
      patchRuntimeNodes({
        nodes: currentNodes,
        runtimeState: nextRuntimeState,
        plan: patchPlan,
        nodeIndexById,
      }),
    );
    setEdges((currentEdges) =>
      patchRuntimeEdges({
        edges: currentEdges,
        links,
        runtimeState: nextRuntimeState,
        alerts,
        onEdgeContextMenu: openEdgeMenu,
        plan: patchPlan,
        edgeIndexById,
      }),
    );
  });

  return appliedSnapshot;
}
