import {
  forceCenter,
  forceCollide,
  forceLink,
  forceManyBody,
  forceSimulation,
  type SimulationLinkDatum,
  type SimulationNodeDatum,
} from 'd3-force';

export interface AutoLayoutNode {
  id: string;
  x?: number;
  y?: number;
  pinned?: boolean;
}

export interface AutoLayoutEdge {
  source: string;
  target: string;
}

interface ForceNode extends SimulationNodeDatum {
  id: string;
  pinned: boolean;
}

interface ForceEdge extends SimulationLinkDatum<ForceNode> {
  source: string;
  target: string;
}

const NODE_MARGIN = 140;

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

function fallbackCoordinate(index: number, axisSize: number, axis: 'x' | 'y'): number {
  const columns = Math.max(1, Math.floor(axisSize / 260));
  if (axis === 'x') {
    return NODE_MARGIN + (index % columns) * 240;
  }
  return NODE_MARGIN + Math.floor(index / columns) * 180;
}

export function computeForceLayout(
  nodes: AutoLayoutNode[],
  edges: AutoLayoutEdge[],
  width: number,
  height: number,
): Map<string, { x: number; y: number }> {
  const safeWidth = Math.max(width, 960);
  const safeHeight = Math.max(height, 720);

  const simulationNodes: ForceNode[] = nodes.map((node, index) => {
    const x = node.x ?? fallbackCoordinate(index, safeWidth, 'x');
    const y = node.y ?? fallbackCoordinate(index, safeHeight, 'y');

    return {
      id: node.id,
      pinned: Boolean(node.pinned),
      x,
      y,
      fx: node.pinned ? x : undefined,
      fy: node.pinned ? y : undefined,
    };
  });

  const simulationEdges: ForceEdge[] = edges.map((edge) => ({
    source: edge.source,
    target: edge.target,
  }));

  const simulation = forceSimulation(simulationNodes)
    .force('link', forceLink<ForceNode, ForceEdge>(simulationEdges).id((node) => node.id).distance(220))
    .force('charge', forceManyBody().strength(-340))
    .force('center', forceCenter(safeWidth / 2, safeHeight / 2))
    .force('collide', forceCollide(96))
    .stop();

  for (let i = 0; i < 300; i += 1) {
    simulation.tick();
  }

  simulation.stop();

  return new Map(
    simulationNodes.map((node) => [
      node.id,
      {
        x: clamp(node.x ?? safeWidth / 2, NODE_MARGIN, safeWidth - NODE_MARGIN),
        y: clamp(node.y ?? safeHeight / 2, NODE_MARGIN, safeHeight - NODE_MARGIN),
      },
    ]),
  );
}

export const useAutoLayout = computeForceLayout;
