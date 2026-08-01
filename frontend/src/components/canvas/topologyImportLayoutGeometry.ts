/** One finite canvas point used by Bootstrap-Once geometry scoring. */
export interface TopologyLayoutPoint {
  x: number;
  y: number;
}

/** One canonical link path, optionally preserving fixed map-local waypoints. */
export interface TopologyLayoutGeometryLink {
  source: string;
  target: string;
  waypoints?: TopologyLayoutPoint[];
}

/** Lexicographic geometry costs affected by one candidate node movement. */
export interface TopologyLayoutCandidateGeometryScore {
  nodeLinkIntersections: number;
  linkCrossings: number;
  incidentLinkLength: number;
}

const NODE_WIDTH = 430;
const NODE_HEIGHT = 140;
const LINK_CLEARANCE = 12;
const GEOMETRY_EPSILON = 0.000001;

interface Segment {
  source: TopologyLayoutPoint;
  target: TopologyLayoutPoint;
}

function nodeCenter(position: TopologyLayoutPoint): TopologyLayoutPoint {
  return {
    x: position.x + NODE_WIDTH / 2,
    y: position.y + NODE_HEIGHT / 2,
  };
}

function positionFor(
  deviceId: string,
  movingDeviceId: string,
  candidate: TopologyLayoutPoint,
  positions: ReadonlyMap<string, TopologyLayoutPoint>,
): TopologyLayoutPoint | undefined {
  return deviceId === movingDeviceId ? candidate : positions.get(deviceId);
}

function linkSegments(
  link: TopologyLayoutGeometryLink,
  movingDeviceId: string,
  candidate: TopologyLayoutPoint,
  positions: ReadonlyMap<string, TopologyLayoutPoint>,
): Segment[] {
  const sourcePosition = positionFor(link.source, movingDeviceId, candidate, positions);
  const targetPosition = positionFor(link.target, movingDeviceId, candidate, positions);
  if (!sourcePosition || !targetPosition) return [];

  const points = [
    nodeCenter(sourcePosition),
    ...(link.waypoints ?? []).filter(
      (waypoint) => Number.isFinite(waypoint.x) && Number.isFinite(waypoint.y),
    ),
    nodeCenter(targetPosition),
  ];
  const segments: Segment[] = [];
  for (let index = 1; index < points.length; index += 1) {
    segments.push({ source: points[index - 1], target: points[index] });
  }
  return segments;
}

function segmentIntersectsNode(segment: Segment, position: TopologyLayoutPoint): boolean {
  const minimum = {
    x: position.x - LINK_CLEARANCE,
    y: position.y - LINK_CLEARANCE,
  };
  const maximum = {
    x: position.x + NODE_WIDTH + LINK_CLEARANCE,
    y: position.y + NODE_HEIGHT + LINK_CLEARANCE,
  };
  const delta = {
    x: segment.target.x - segment.source.x,
    y: segment.target.y - segment.source.y,
  };
  let lower = 0;
  let upper = 1;

  for (const axis of ['x', 'y'] as const) {
    if (Math.abs(delta[axis]) < GEOMETRY_EPSILON) {
      if (segment.source[axis] <= minimum[axis] || segment.source[axis] >= maximum[axis]) {
        return false;
      }
      continue;
    }
    const first = (minimum[axis] - segment.source[axis]) / delta[axis];
    const second = (maximum[axis] - segment.source[axis]) / delta[axis];
    lower = Math.max(lower, Math.min(first, second));
    upper = Math.min(upper, Math.max(first, second));
    if (lower >= upper) return false;
  }

  return upper > GEOMETRY_EPSILON && lower < 1 - GEOMETRY_EPSILON;
}

function orientation(
  first: TopologyLayoutPoint,
  second: TopologyLayoutPoint,
  third: TopologyLayoutPoint,
): number {
  return (second.x - first.x) * (third.y - first.y) - (second.y - first.y) * (third.x - first.x);
}

function segmentsCross(left: Segment, right: Segment): boolean {
  const leftSource = orientation(left.source, left.target, right.source);
  const leftTarget = orientation(left.source, left.target, right.target);
  const rightSource = orientation(right.source, right.target, left.source);
  const rightTarget = orientation(right.source, right.target, left.target);

  return (
    ((leftSource > GEOMETRY_EPSILON && leftTarget < -GEOMETRY_EPSILON) ||
      (leftSource < -GEOMETRY_EPSILON && leftTarget > GEOMETRY_EPSILON)) &&
    ((rightSource > GEOMETRY_EPSILON && rightTarget < -GEOMETRY_EPSILON) ||
      (rightSource < -GEOMETRY_EPSILON && rightTarget > GEOMETRY_EPSILON))
  );
}

function pathIntersectsNode(segments: Segment[], position: TopologyLayoutPoint): boolean {
  return segments.some((segment) => segmentIntersectsNode(segment, position));
}

function pathsCross(left: Segment[], right: Segment[]): boolean {
  return left.some((leftSegment) =>
    right.some((rightSegment) => segmentsCross(leftSegment, rightSegment)),
  );
}

function pathLength(segments: Segment[]): number {
  return segments.reduce(
    (total, segment) =>
      total + Math.hypot(segment.target.x - segment.source.x, segment.target.y - segment.source.y),
    0,
  );
}

/** Scores only geometry affected by moving one node to a candidate position. */
export function scoreTopologyLayoutCandidate(
  movingDeviceId: string,
  candidate: TopologyLayoutPoint,
  positions: ReadonlyMap<string, TopologyLayoutPoint>,
  links: TopologyLayoutGeometryLink[],
): TopologyLayoutCandidateGeometryScore {
  const incidentLinks = links.filter(
    (link) => link.source === movingDeviceId || link.target === movingDeviceId,
  );
  const otherLinks = links.filter(
    (link) => link.source !== movingDeviceId && link.target !== movingDeviceId,
  );
  const incidentSegments = new Map(
    incidentLinks.map((link) => [link, linkSegments(link, movingDeviceId, candidate, positions)]),
  );
  const otherSegments = new Map(
    otherLinks.map((link) => [link, linkSegments(link, movingDeviceId, candidate, positions)]),
  );
  let nodeLinkIntersections = 0;
  let linkCrossings = 0;
  let incidentLinkLength = 0;

  for (const segments of otherSegments.values()) {
    if (pathIntersectsNode(segments, candidate)) nodeLinkIntersections += 1;
  }

  for (const link of incidentLinks) {
    const segments = incidentSegments.get(link) ?? [];
    incidentLinkLength += pathLength(segments);

    for (const [deviceId, position] of positions) {
      if (deviceId === movingDeviceId || deviceId === link.source || deviceId === link.target) {
        continue;
      }
      if (pathIntersectsNode(segments, position)) nodeLinkIntersections += 1;
    }

    for (const otherLink of otherLinks) {
      if (pathsCross(segments, otherSegments.get(otherLink) ?? [])) linkCrossings += 1;
    }
  }

  return { nodeLinkIntersections, linkCrossings, incidentLinkLength };
}
