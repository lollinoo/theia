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

const NODE_MARGIN = 128;
const LAYER_GAP = 280;
const ROW_GAP = 156;
const COMPONENT_GAP = 240;

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

function buildAdjacency(
  nodes: AutoLayoutNode[],
  edges: AutoLayoutEdge[],
): Map<string, Set<string>> {
  const adjacency = new Map<string, Set<string>>();

  nodes.forEach((node) => adjacency.set(node.id, new Set()));
  edges.forEach((edge) => {
    if (!adjacency.has(edge.source) || !adjacency.has(edge.target)) return;
    adjacency.get(edge.source)?.add(edge.target);
    adjacency.get(edge.target)?.add(edge.source);
  });

  return adjacency;
}

function collectComponent(
  startId: string,
  adjacency: Map<string, Set<string>>,
  visited: Set<string>,
): string[] {
  const queue = [startId];
  const component: string[] = [];
  visited.add(startId);

  while (queue.length > 0) {
    const current = queue.shift()!;
    component.push(current);
    const neighbors = adjacency.get(current) ?? new Set<string>();

    [...neighbors]
      .sort((a, b) => (adjacency.get(b)?.size ?? 0) - (adjacency.get(a)?.size ?? 0) || a.localeCompare(b))
      .forEach((neighbor) => {
        if (visited.has(neighbor)) return;
        visited.add(neighbor);
        queue.push(neighbor);
      });
  }

  return component;
}

function computeLayerSeedPositions(
  nodes: AutoLayoutNode[],
  edges: AutoLayoutEdge[],
  width: number,
  height: number,
): Map<string, { x: number; y: number }> {
  const safeWidth = Math.max(width, 960);
  const safeHeight = Math.max(height, 720);
  const nodesById = new Map(nodes.map((node) => [node.id, node]));
  const adjacency = buildAdjacency(nodes, edges);
  const visited = new Set<string>();
  const componentStarts = [...nodes]
    .sort((a, b) => (adjacency.get(b.id)?.size ?? 0) - (adjacency.get(a.id)?.size ?? 0) || a.id.localeCompare(b.id))
    .map((node) => node.id);
  const positions = new Map<string, { x: number; y: number }>();

  let cursorX = NODE_MARGIN;
  let cursorY = NODE_MARGIN;
  let currentRowHeight = 0;

  componentStarts.forEach((startId) => {
    if (visited.has(startId)) return;

    const component = collectComponent(startId, adjacency, visited);
    const componentSet = new Set(component);
    const root = [...component]
      .sort((a, b) => {
        const aDegree = adjacency.get(a)?.size ?? 0;
        const bDegree = adjacency.get(b)?.size ?? 0;
        if (aDegree !== bDegree) return bDegree - aDegree;
        return a.localeCompare(b);
      })[0] ?? startId;

    const layers = new Map<number, string[]>();
    const queue: Array<{ id: string; depth: number }> = [{ id: root, depth: 0 }];
    const layeredVisited = new Set<string>([root]);

    while (queue.length > 0) {
      const item = queue.shift()!;
      const bucket = layers.get(item.depth) ?? [];
      bucket.push(item.id);
      layers.set(item.depth, bucket);

      const neighbors = [...(adjacency.get(item.id) ?? new Set<string>())]
        .filter((neighbor) => componentSet.has(neighbor))
        .sort((a, b) => {
          const aNode = nodesById.get(a);
          const bNode = nodesById.get(b);
          if (aNode?.y !== undefined && bNode?.y !== undefined && aNode.y !== bNode.y) {
            return aNode.y - bNode.y;
          }
          return (adjacency.get(b)?.size ?? 0) - (adjacency.get(a)?.size ?? 0) || a.localeCompare(b);
        });

      neighbors.forEach((neighbor) => {
        if (layeredVisited.has(neighbor)) return;
        layeredVisited.add(neighbor);
        queue.push({ id: neighbor, depth: item.depth + 1 });
      });
    }

    component.forEach((id) => {
      if (layeredVisited.has(id)) return;
      const bucket = layers.get(0) ?? [];
      bucket.push(id);
      layers.set(0, bucket);
    });

    const sortedLayers = [...layers.entries()].sort((a, b) => a[0] - b[0]);
    const widestLayer = Math.max(...sortedLayers.map(([, ids]) => ids.length), 1);
    const componentWidth = Math.max((sortedLayers.length - 1) * LAYER_GAP, 0);
    const componentHeight = Math.max((widestLayer - 1) * ROW_GAP, 0) + 120;

    if (cursorX + componentWidth > safeWidth - NODE_MARGIN && cursorX > NODE_MARGIN) {
      cursorX = NODE_MARGIN;
      cursorY += currentRowHeight + COMPONENT_GAP;
      currentRowHeight = 0;
    }

    sortedLayers.forEach(([depth, ids]) => {
      const sortedIds = [...ids].sort((a, b) => {
        const aNode = nodesById.get(a);
        const bNode = nodesById.get(b);
        if (aNode?.y !== undefined && bNode?.y !== undefined && aNode.y !== bNode.y) {
          return aNode.y - bNode.y;
        }
        if (aNode?.x !== undefined && bNode?.x !== undefined && aNode.x !== bNode.x) {
          return aNode.x - bNode.x;
        }
        return a.localeCompare(b);
      });
      const layerHeight = Math.max((sortedIds.length - 1) * ROW_GAP, 0);
      const layerBaseY = clamp(
        cursorY + (componentHeight - layerHeight) / 2,
        NODE_MARGIN,
        safeHeight - NODE_MARGIN - layerHeight,
      );

      sortedIds.forEach((id, index) => {
        positions.set(id, {
          x: clamp(cursorX + depth * LAYER_GAP, NODE_MARGIN, safeWidth - NODE_MARGIN),
          y: clamp(layerBaseY + index * ROW_GAP, NODE_MARGIN, safeHeight - NODE_MARGIN),
        });
      });
    });

    cursorX += componentWidth + COMPONENT_GAP;
    currentRowHeight = Math.max(currentRowHeight, componentHeight);
  });

  return positions;
}

export function computeForceLayout(
  nodes: AutoLayoutNode[],
  edges: AutoLayoutEdge[],
  width: number,
  height: number,
): Map<string, { x: number; y: number }> {
  const safeWidth = Math.max(width, 960);
  const safeHeight = Math.max(height, 720);
  const seedPositions = computeLayerSeedPositions(nodes, edges, safeWidth, safeHeight);

  const simulationNodes: ForceNode[] = nodes.map((node, index) => {
    const seeded = seedPositions.get(node.id);
    const x = node.x ?? seeded?.x ?? fallbackCoordinate(index, safeWidth, 'x');
    const y = node.y ?? seeded?.y ?? fallbackCoordinate(index, safeHeight, 'y');

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
    .force('link', forceLink<ForceNode, ForceEdge>(simulationEdges).id((node) => node.id).distance(210).strength(0.7))
    .force('charge', forceManyBody().strength(-280))
    .force('center', forceCenter(safeWidth / 2, safeHeight / 2))
    .force('collide', forceCollide(104))
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
