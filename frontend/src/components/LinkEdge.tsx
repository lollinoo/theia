import { memo, useState } from 'react';
import {
  BaseEdge,
  EdgeLabelRenderer,
  getBezierPath,
  type Edge,
  type EdgeProps,
} from '@xyflow/react';
import type { Link } from '../types/api';
import { utilizationColor, type AlertStatus, type LinkMetricsDTO } from '../types/metrics';

export interface LinkEdgeData {
  link?: Link;
  bandwidthLabel?: string;
  speedMismatch?: boolean;
  inertVirtualLink?: boolean;
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
  areaColor?: string;
  [key: string]: unknown;
}

export type LinkEdgeType = Edge<LinkEdgeData>;

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

export function normalizeInterfaceStatusForLink(status: string | undefined): string | undefined {
  const normalized = status?.trim().toLowerCase();
  if (!normalized || normalized === 'unknown') {
    return undefined;
  }
  return normalized;
}

export function normalizeLinkStateForColor(data: LinkEdgeData | undefined): {
  inertVirtualLink: boolean;
  alertStatus: AlertStatus | undefined;
  sourceDeviceStatus: string | undefined;
  targetDeviceStatus: string | undefined;
  sourceIfStatus: string | undefined;
  targetIfStatus: string | undefined;
  utilization: number | null;
} {
  const inertVirtualLink = data?.inertVirtualLink === true;
  return {
    inertVirtualLink,
    alertStatus: data?.alertStatus,
    sourceDeviceStatus: inertVirtualLink ? undefined : data?.sourceDeviceStatus,
    targetDeviceStatus: inertVirtualLink ? undefined : data?.targetDeviceStatus,
    sourceIfStatus: normalizeInterfaceStatusForLink(data?.sourceIfStatus),
    targetIfStatus: normalizeInterfaceStatusForLink(data?.targetIfStatus),
    utilization: data?.utilization ?? data?.metrics?.utilization ?? null,
  };
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
}: EdgeProps<LinkEdgeType>) {
  const [hovered, setHovered] = useState(false);
  const isActive = selected || hovered;
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
  });

  const index = data?.parallelIndex || 0;
  // Alternate offsets: 0, 26, -26, 52, -52
  const sign = index % 2 === 0 ? 1 : -1;
  const magnitude = Math.ceil(index / 2) * 26;
  const labelOffsetY = sign * magnitude;
  const {
    inertVirtualLink,
    alertStatus,
    sourceDeviceStatus,
    targetDeviceStatus,
    sourceIfStatus,
    targetIfStatus,
    utilization,
  } = normalizeLinkStateForColor(data);

  // Device-level status (from probe_success / Prometheus-down override)
  const srcDevDown = sourceDeviceStatus === 'down';
  const tgtDevDown = targetDeviceStatus === 'down';
  const srcDevProbing = sourceDeviceStatus === 'probing';
  const tgtDevProbing = targetDeviceStatus === 'probing';
  const srcDevInactive = srcDevDown || srcDevProbing;
  const tgtDevInactive = tgtDevDown || tgtDevProbing;
  const bothDevDown = srcDevDown && tgtDevDown;
  const oneDevDown = (srcDevDown || tgtDevDown) && !(srcDevDown && tgtDevDown);
  const bothDevInactive = srcDevInactive && tgtDevInactive && !bothDevDown;
  const oneDevInactive = srcDevInactive !== tgtDevInactive;

  // Interface-level oper_status
  // Treat null/undefined/unknown (e.g. virtual device side with no interface)
  // as neutral — only meaningful oper_status values participate in link color decisions.
  // Support links to no-IP virtual nodes also opt out of live port-state coloring.
  const sourceIfKnown = sourceIfStatus != null;
  const targetIfKnown = targetIfStatus != null;
  const sourceUp = sourceIfStatus === 'up' || !sourceIfKnown;
  const targetUp = targetIfStatus === 'up' || !targetIfKnown;
  const bothUp = sourceUp && targetUp;
  const singleKnownIf = sourceIfKnown !== targetIfKnown;
  const singleKnownIfUp = sourceIfKnown ? sourceUp : targetIfKnown ? targetUp : false;
  const singleKnownIfDown = singleKnownIf && !singleKnownIfUp;
  const oneIfDown = (sourceIfKnown || targetIfKnown) && ((sourceUp && !targetUp) || (!sourceUp && targetUp));
  const bothIfDown = sourceIfKnown && targetIfKnown && !sourceUp && !targetUp;
  const inertUtilDown = inertVirtualLink && utilization !== null && utilization > 0.8;
  const inertUtilWarn = inertVirtualLink && utilization !== null && utilization >= 0.5 && utilization <= 0.8;

  // Priority: alerts → device status → interface oper_status → utilization → default
  let strokeColor: string;
  let strokeWidth: number;
  if (alertStatus === 'down') {
    strokeColor = 'var(--color-status-down)';
    strokeWidth = 3;
  } else if (alertStatus === 'degraded') {
    strokeColor = 'var(--color-status-probing)';
    strokeWidth = 2.5;
  } else if (inertVirtualLink && singleKnownIfDown) {
    strokeColor = 'var(--color-status-down)';
    strokeWidth = 2;
  } else if (inertVirtualLink && utilization !== null) {
    strokeColor = utilizationColor(utilization);
    strokeWidth = 2;
  } else if (inertVirtualLink && singleKnownIfUp) {
    strokeColor = 'var(--color-status-up)';
    strokeWidth = 2;
  } else if (bothDevDown) {
    strokeColor = 'var(--color-status-down)';
    strokeWidth = 2;
  } else if (oneDevDown) {
    strokeColor = 'var(--color-status-probing)';
    strokeWidth = 2;
  } else if (bothDevInactive) {
    // Both devices probing (or mix of probing+down not caught above)
    strokeColor = 'var(--color-status-probing)';
    strokeWidth = 2;
  } else if (oneDevInactive) {
    // One device probing, other is up
    strokeColor = 'var(--color-status-probing)';
    strokeWidth = 2;
  } else if (bothIfDown) {
    strokeColor = 'var(--color-status-down)';
    strokeWidth = 2;
  } else if (oneIfDown) {
    strokeColor = 'var(--color-status-probing)';
    strokeWidth = 2;
  } else if (data?.areaColor && !inertVirtualLink) {
    strokeColor = data.areaColor;
    strokeWidth = 2;
  } else if (!inertVirtualLink && bothUp && utilization === null) {
    strokeColor = 'var(--color-status-up)';
    strokeWidth = 2;
  } else {
    strokeColor = utilization === null ? 'var(--color-outline)' : utilizationColor(utilization);
    strokeWidth = 2;
  }

  // Throughput label color matches link color
  const throughputColor = (alertStatus === 'down' || inertUtilDown || (inertVirtualLink && singleKnownIfDown) || bothDevDown || bothIfDown)
    ? 'var(--color-status-down)'
    : (alertStatus === 'degraded' || inertUtilWarn || oneDevDown || oneDevInactive || bothDevInactive || oneIfDown)
      ? 'var(--color-status-probing)'
      : inertVirtualLink && utilization !== null
        ? utilizationColor(utilization)
        : inertVirtualLink && singleKnownIfUp
          ? 'var(--color-status-up)'
      : utilization === null
        ? 'var(--nt-on-bg-secondary)'
        : utilizationColor(utilization);

  const activeStrokeWidth = isActive ? strokeWidth + 1.5 : strokeWidth;
  const activeStrokeColor = isActive
    ? data?.areaColor && !inertVirtualLink
      ? data.areaColor
      : strokeColor === 'var(--color-outline)'
        ? 'var(--nt-on-bg-muted)'
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
            className={`pointer-events-none z-10 rounded-md border bg-surface px-2 py-1 text-[11px] font-medium shadow-pill transition-colors duration-200 ${
              (alertStatus === 'down' || inertUtilDown || (inertVirtualLink && singleKnownIfDown) || bothDevDown || bothIfDown)
                ? 'border-status-down/40 text-status-down'
                : (alertStatus === 'degraded' || inertUtilWarn || oneDevDown || oneDevInactive || bothDevInactive || oneIfDown)
                  ? 'border-status-probing/40 text-status-probing'
                  : data.speedMismatch
                    ? 'border-status-probing/40 text-status-probing'
                    : data.areaColor && !inertVirtualLink
                      ? ''
                      : inertVirtualLink && singleKnownIfUp
                        ? 'border-status-up/40 text-status-up'
                        : 'border-outline-subtle text-on-bg-secondary'
            }`}
            style={{
              position: 'absolute',
              top: 0,
              left: 0,
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY + labelOffsetY}px)`,
              ...(data.areaColor && !inertVirtualLink && !data.speedMismatch && !bothDevDown && !oneDevDown && !bothDevInactive && !oneDevInactive && !bothIfDown && !oneIfDown && alertStatus !== 'down' && alertStatus !== 'degraded'
                ? { borderColor: data.areaColor, color: data.areaColor }
                : {}),
            }}
            title={data.speedMismatch ? 'Speed negotiation mismatch between interfaces' : undefined}
          >
            {data.bandwidthLabel}
          </div>
        </EdgeLabelRenderer>
      ) : null}
      {data?.throughputLabel ? (
        <EdgeLabelRenderer>
          <div
            className={`pointer-events-none z-10 rounded-md border bg-surface px-2 py-1 text-[10px] font-medium shadow-pill transition-colors duration-200 ${data.areaColor ? '' : 'border-outline-subtle'}`}
            style={{
              position: 'absolute',
              top: 0,
              left: 0,
              color: data.areaColor && !inertVirtualLink ? data.areaColor : throughputColor,
              ...(data.areaColor && !inertVirtualLink ? { borderColor: data.areaColor } : {}),
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
    prev.data?.inertVirtualLink === next.data?.inertVirtualLink &&
    prev.data?.utilization === next.data?.utilization &&
    prev.data?.throughputLabel === next.data?.throughputLabel &&
    prev.data?.alertStatus === next.data?.alertStatus &&
    prev.data?.bandwidthLabel === next.data?.bandwidthLabel &&
    prev.data?.speedMismatch === next.data?.speedMismatch &&
    prev.data?.sourceIfStatus === next.data?.sourceIfStatus &&
    prev.data?.targetIfStatus === next.data?.targetIfStatus &&
    prev.data?.sourceDeviceStatus === next.data?.sourceDeviceStatus &&
    prev.data?.targetDeviceStatus === next.data?.targetDeviceStatus &&
    prev.data?.areaColor === next.data?.areaColor &&
    prev.sourceX === next.sourceX &&
    prev.sourceY === next.sourceY &&
    prev.targetX === next.targetX &&
    prev.targetY === next.targetY &&
    prev.sourcePosition === next.sourcePosition &&
    prev.targetPosition === next.targetPosition
  );
});

export default LinkEdge;
