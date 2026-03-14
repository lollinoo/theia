import { memo, useState } from 'react';
import {
  BaseEdge,
  EdgeLabelRenderer,
  getSmoothStepPath,
  type EdgeProps,
} from 'reactflow';
import type { Link } from '../types/api';
import { utilizationColor, type AlertStatus, type LinkMetricsDTO } from '../types/metrics';

export interface LinkEdgeData {
  link?: Link;
  bandwidthLabel?: string;
  manual?: boolean;
  parallelIndex?: number;
  onContextMenu?: (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeID: string) => void;
  metrics?: LinkMetricsDTO | null;
  throughputLabel?: string;
  utilization?: number | null;
  alertStatus?: AlertStatus;
  sourceIfStatus?: string;
  targetIfStatus?: string;
  sourceDeviceStatus?: string;
  targetDeviceStatus?: string;
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

function LinkEdgeInner({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  selected,
  data,
}: EdgeProps<LinkEdgeData>) {
  const [hovered, setHovered] = useState(false);
  const isActive = selected || hovered;
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
  const alertStatus = data?.alertStatus;

  // Device-level status (from probe_success / Prometheus-down override)
  const srcDevDown = data?.sourceDeviceStatus === 'down';
  const tgtDevDown = data?.targetDeviceStatus === 'down';
  const bothDevDown = srcDevDown && tgtDevDown;
  const oneDevDown = srcDevDown !== tgtDevDown && (srcDevDown || tgtDevDown);

  // Interface-level oper_status
  const sourceUp = data?.sourceIfStatus === 'up';
  const targetUp = data?.targetIfStatus === 'up';
  const bothUp = sourceUp && targetUp;
  const oneIfDown = (sourceUp && !targetUp) || (!sourceUp && targetUp);
  const bothIfDown = !sourceUp && !targetUp && data?.sourceIfStatus != null;

  // Priority: alerts → device status → interface oper_status → utilization → default
  let strokeColor: string;
  let strokeWidth: number;
  if (alertStatus === 'down') {
    strokeColor = '#ff1744';
    strokeWidth = 3;
  } else if (alertStatus === 'degraded') {
    strokeColor = '#ffc107';
    strokeWidth = 2.5;
  } else if (bothDevDown) {
    strokeColor = '#ff1744';
    strokeWidth = 2;
  } else if (oneDevDown) {
    strokeColor = '#ffc107';
    strokeWidth = 2;
  } else if (bothIfDown) {
    strokeColor = '#ff1744';
    strokeWidth = 2;
  } else if (oneIfDown) {
    strokeColor = '#ffc107';
    strokeWidth = 2;
  } else if (bothUp && utilization === null) {
    strokeColor = '#00c853';
    strokeWidth = 2;
  } else {
    strokeColor = utilization === null ? '#4a4a5e' : utilizationColor(utilization);
    strokeWidth = 2;
  }

  // Throughput label color matches link color
  const throughputColor = (bothDevDown || bothIfDown || alertStatus === 'down')
    ? '#ff1744'
    : (oneDevDown || oneIfDown || alertStatus === 'degraded')
      ? '#ffc107'
      : utilization === null
        ? '#8899a6'
        : utilizationColor(utilization);

  const activeStrokeWidth = isActive ? strokeWidth + 1.5 : strokeWidth;
  const activeStrokeColor = isActive
    ? strokeColor === '#4a4a5e'
      ? '#7a7a9e'
      : strokeColor
    : strokeColor;

  return (
    <>
      <path
        d={edgePath}
        fill="none"
        stroke="transparent"
        strokeWidth={20}
        className="cursor-pointer"
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        onContextMenu={(event) => {
          if (!data?.onContextMenu) {
            return;
          }

          event.preventDefault();
          event.stopPropagation();
          data.onContextMenu(event, id);
        }}
      />
      <BaseEdge
        id={id}
        path={edgePath}
        style={{
          stroke: activeStrokeColor,
          strokeWidth: activeStrokeWidth,
          filter: isActive ? `drop-shadow(0 0 4px ${activeStrokeColor})` : undefined,
          transition: 'stroke-width 0.1s, filter 0.1s',
          ...(alertStatus === 'down' ? { animation: 'pulse 1.5s ease-in-out infinite' } : {}),
        }}
      />
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

const LinkEdge = memo(LinkEdgeInner, (prev, next) => {
  return (
    prev.id === next.id &&
    prev.selected === next.selected &&
    prev.data?.utilization === next.data?.utilization &&
    prev.data?.throughputLabel === next.data?.throughputLabel &&
    prev.data?.alertStatus === next.data?.alertStatus &&
    prev.data?.bandwidthLabel === next.data?.bandwidthLabel &&
    prev.data?.sourceIfStatus === next.data?.sourceIfStatus &&
    prev.data?.targetIfStatus === next.data?.targetIfStatus &&
    prev.data?.sourceDeviceStatus === next.data?.sourceDeviceStatus &&
    prev.data?.targetDeviceStatus === next.data?.targetDeviceStatus &&
    prev.sourceX === next.sourceX &&
    prev.sourceY === next.sourceY &&
    prev.targetX === next.targetX &&
    prev.targetY === next.targetY
  );
});

export default LinkEdge;
