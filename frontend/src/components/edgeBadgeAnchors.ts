/**
 * Renders edge badge anchors UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
const SVG_NS = 'http://www.w3.org/2000/svg';

/** Describes the edge badge anchor contract used by the UI component boundary. */
export interface EdgeBadgeAnchor {
  x: number;
  y: number;
}

interface ComputeLinkBadgeAnchorOptions {
  path: string;
  fallbackX: number;
  fallbackY: number;
  parallelIndex?: number;
}

let measurementPath: SVGPathElement | null = null;

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}

function getMeasurementPath(): SVGPathElement | null {
  if (typeof document === 'undefined' || typeof document.createElementNS !== 'function') {
    return null;
  }

  if (!measurementPath) {
    measurementPath = document.createElementNS(SVG_NS, 'path') as SVGPathElement;
  }

  return measurementPath;
}

function resolveParallelPathOffset(parallelIndex: number | undefined, laneStep: number): number {
  const index = parallelIndex ?? 0;

  if (index === 0) {
    return 0;
  }

  const sign = index % 2 === 0 ? 1 : -1;
  const magnitude = Math.ceil(index / 2) * laneStep;
  return sign * magnitude;
}

/** Measures edge path length for the UI component boundary. */
export function measureEdgePathLength(path: string): number {
  const pathElement = getMeasurementPath();

  if (!pathElement || typeof pathElement.getTotalLength !== 'function') {
    return 0;
  }

  pathElement.setAttribute('d', path);
  const totalLength = pathElement.getTotalLength();
  return Number.isFinite(totalLength) && totalLength > 0 ? totalLength : 0;
}

/** Resolves badge path lengths for the UI component boundary. */
export function resolveBadgePathLengths(totalLength: number, badgeCount: number): number[] {
  if (badgeCount <= 0 || !Number.isFinite(totalLength) || totalLength <= 0) {
    return [];
  }

  if (badgeCount === 1) {
    return [totalLength / 2];
  }

  const edgePadding = clamp(totalLength * 0.18, 18, 42);
  const minLength = Math.min(edgePadding, totalLength / 2);
  const maxLength = Math.max(totalLength - edgePadding, totalLength / 2);
  const span = Math.max(maxLength - minLength, 0);
  const preferredSpacing = 92;
  const spacing = span === 0 ? 0 : Math.min(preferredSpacing, span / (badgeCount - 1));
  const center = totalLength / 2;

  return Array.from({ length: badgeCount }, (_, index) => {
    const offset = (index - (badgeCount - 1) / 2) * spacing;
    return clamp(center + offset, minLength, maxLength);
  });
}

/** Computes link badge anchor for the UI component boundary. */
export function computeLinkBadgeAnchor({
  path,
  fallbackX,
  fallbackY,
  parallelIndex,
}: ComputeLinkBadgeAnchorOptions): EdgeBadgeAnchor {
  const pathElement = getMeasurementPath();
  if (
    !pathElement ||
    typeof pathElement.getTotalLength !== 'function' ||
    typeof pathElement.getPointAtLength !== 'function'
  ) {
    return { x: fallbackX, y: fallbackY };
  }

  pathElement.setAttribute('d', path);
  const totalLength = pathElement.getTotalLength();

  if (!Number.isFinite(totalLength) || totalLength <= 0) {
    return { x: fallbackX, y: fallbackY };
  }

  const anchorSlots = resolveBadgePathLengths(totalLength, 3);
  const centerLength = anchorSlots[1] ?? totalLength / 2;
  const minLength = anchorSlots[0] ?? clamp(totalLength * 0.2, 18, totalLength / 2);
  const maxLength = anchorSlots[2] ?? clamp(totalLength * 0.8, totalLength / 2, totalLength - 18);
  const anchorLength = clamp(
    centerLength + resolveParallelPathOffset(parallelIndex, clamp(totalLength * 0.05, 14, 28)),
    minLength,
    maxLength,
  );
  const point = pathElement.getPointAtLength(anchorLength);

  return {
    x: point.x,
    y: point.y,
  };
}
