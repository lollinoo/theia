import type { PositionState } from '../../hooks/usePositions';
import type { Device } from '../../types/api';
import type { DeviceNode } from '../DeviceCard';
import type { buildPositionPayload } from './canvasHelpers';

function hasUsablePosition(position: PositionState | undefined): position is PositionState {
  return position !== undefined && Number.isFinite(position.x) && Number.isFinite(position.y);
}

export function buildUsablePositionState(
  devices: Device[],
  currentPositions: Map<string, PositionState>,
  savedPositions: Map<string, PositionState>,
): string {
  return devices
    .map((device) => {
      const currentPosition = currentPositions.get(device.id);
      const savedPosition = savedPositions.get(device.id);

      if (hasUsablePosition(currentPosition) || hasUsablePosition(savedPosition)) {
        return device.id;
      }

      return null;
    })
    .filter((deviceId): deviceId is string => deviceId !== null)
    .sort()
    .join('|');
}

export function positionsChanged(
  nextPositions: ReturnType<typeof buildPositionPayload>,
  savedPositions: Map<string, PositionState>,
): boolean {
  if (nextPositions.length !== savedPositions.size) {
    return true;
  }

  for (const position of nextPositions) {
    const savedPosition = savedPositions.get(position.device_id);
    if (
      !savedPosition ||
      savedPosition.x !== position.x ||
      savedPosition.y !== position.y ||
      savedPosition.pinned !== position.pinned
    ) {
      return true;
    }
  }

  return false;
}

export function nodePositionsToPositionMap(nodes: DeviceNode[]): Map<string, PositionState> {
  return new Map(
    nodes.map((node) => [
      node.id,
      {
        x: node.position.x,
        y: node.position.y,
        pinned: node.data.pinned ?? false,
      },
    ]),
  );
}

export function mergeNodePresentationState(
  nextNodes: DeviceNode[],
  currentNodes: DeviceNode[],
): DeviceNode[] {
  const currentNodesById = new Map(currentNodes.map((node) => [node.id, node]));

  return nextNodes.map((node) => {
    const currentNode = currentNodesById.get(node.id);
    if (!currentNode) {
      return node;
    }

    return {
      ...node,
      selected: currentNode.selected,
      dragging: currentNode.dragging,
      width: currentNode.width,
      height: currentNode.height,
      initialWidth: currentNode.initialWidth,
      initialHeight: currentNode.initialHeight,
      measured: currentNode.measured,
      data: {
        ...node.data,
        highlighted: currentNode.data.highlighted,
      },
    };
  });
}
