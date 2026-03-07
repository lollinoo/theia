import {
  BaseEdge,
  EdgeLabelRenderer,
  getSmoothStepPath,
  type EdgeProps,
} from 'reactflow';
import type { Link } from '../types/api';
import { utilizationColor, type LinkMetricsDTO } from '../types/metrics';

export interface LinkEdgeData {
  link?: Link;
  bandwidthLabel?: string;
  manual?: boolean;
  parallelIndex?: number;
  onContextMenu?: (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeID: string) => void;
  metrics?: LinkMetricsDTO | null;
  throughputLabel?: string;
  utilization?: number | null;
}

export function formatBandwidth(speed: number): string {
  if (!speed || speed <= 0) {
    return 'Unknown';
  }

  if (speed >= 1_000_000_000) {
    return `${Math.round(speed / 1_000_000_000)} Gbps`;
  }

  if (speed >= 1_000_000) {
    return `${Math.round(speed / 1_000_000)} Mbps`;
  }

  if (speed >= 1_000) {
    return `${Math.round(speed / 1_000)} Kbps`;
  }

  return `${speed} bps`;
}

export default function LinkEdge({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  data,
}: EdgeProps<LinkEdgeData>) {
  const [edgePath, labelX, labelY] = getSmoothStepPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
    borderRadius: 18,
  });

  const index = data?.parallelIndex || 0;
  // Alternate offsets: 0, 26, -26, 52, -52
  const sign = index % 2 === 0 ? 1 : -1;
  const magnitude = Math.ceil(index / 2) * 26;
  const labelOffsetY = sign * magnitude;
  const utilization = data?.utilization ?? data?.metrics?.utilization ?? null;
  const strokeColor = utilization === null ? '#4a4a5e' : utilizationColor(utilization);
  const throughputColor = utilization === null ? '#8899a6' : utilizationColor(utilization);

  return (
    <>
      <path
        d={edgePath}
        fill="none"
        stroke="transparent"
        strokeWidth={20}
        className="cursor-pointer"
        onContextMenu={(event) => {
          if (!data?.manual || !data.onContextMenu) {
            return;
          }

          event.preventDefault();
          event.stopPropagation();
          data.onContextMenu(event, id);
        }}
      />
      <BaseEdge id={id} path={edgePath} style={{ stroke: strokeColor, strokeWidth: 2 }} />
      {data?.bandwidthLabel ? (
        <EdgeLabelRenderer>
          <div
            className="pointer-events-none absolute rounded-md border border-border-subtle bg-bg-canvas/95 px-2 py-1 text-[11px] font-medium text-text-secondary shadow-[0_8px_24px_rgba(0,0,0,0.35)]"
            style={{
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY + labelOffsetY}px)`,
            }}
          >
            {data.bandwidthLabel}
          </div>
        </EdgeLabelRenderer>
      ) : null}
      {data?.throughputLabel ? (
        <EdgeLabelRenderer>
          <div
            className="pointer-events-none absolute rounded-md border border-border-subtle bg-bg-canvas/95 px-2 py-1 text-[10px] font-medium shadow-[0_8px_24px_rgba(0,0,0,0.35)]"
            style={{
              color: throughputColor,
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY + labelOffsetY + 20}px)`,
            }}
          >
            {data.throughputLabel}
          </div>
        </EdgeLabelRenderer>
      ) : null}
    </>
  );
}
