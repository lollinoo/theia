/**
 * Defines alert status patch behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { AlertDTO, SnapshotPayload } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { patchAlertStatuses } from './canvasPresentationPatches';

type AlertNodeUpdater = (updater: (currentNodes: DeviceNode[]) => DeviceNode[]) => void;
type AlertEdgeUpdater = (updater: (currentEdges: LinkEdgeType[]) => LinkEdgeType[]) => void;

interface ApplyAlertStatusPatchOptions {
  snapshot: SnapshotPayload | null;
  alerts: AlertDTO[];
  setNodes: AlertNodeUpdater;
  setEdges: AlertEdgeUpdater;
  nodeIndexById?: ReadonlyMap<string, number>;
  edgeIndexById?: ReadonlyMap<string, number>;
}

// applyAlertStatusPatch updates alert presentation on existing nodes and edges.
export function applyAlertStatusPatch({
  snapshot,
  alerts,
  setNodes,
  setEdges,
  nodeIndexById,
  edgeIndexById,
}: ApplyAlertStatusPatchOptions): void {
  setNodes(
    (currentNodes) =>
      patchAlertStatuses(currentNodes, [], { nodeIndexById }, snapshot, alerts).nodes,
  );

  setEdges(
    (currentEdges) =>
      patchAlertStatuses([], currentEdges, { edgeIndexById }, snapshot, alerts).edges,
  );
}
