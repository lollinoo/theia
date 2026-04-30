import {
  BaseEdge,
  type Edge,
  EdgeLabelRenderer,
  type EdgeProps,
  getBezierPath,
  useStore,
} from '@xyflow/react';
import { memo, useMemo, useState } from 'react';
import { buildSelfLoopPathModel } from './linkEdgeGeometry';
import { type LinkEdgeData, resolveEdgeTone, resolveLinkBadgePresentation } from './linkSemantics';

export type LinkEdgeType = Edge<LinkEdgeData>;

function LinkEdgeInner({
  id,
  source,
  target,
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
  const zoom = useStore((state) => state.transform[2]);
  const isActive = selected || hovered;
  const isConnected = data?.emphasis === 'connected';
  const isMuted = data?.emphasis === 'muted';
  const index = data?.parallelIndex || 0;
  const isSelfLoop =
    source === target || data?.link?.source_device_id === data?.link?.target_device_id;
  const { edgePath, labelX, labelY } = isSelfLoop
    ? buildSelfLoopPathModel({
        sourceX,
        sourceY,
        targetX,
        targetY,
        parallelIndex: index,
      })
    : (() => {
        const [path, x, y] = getBezierPath({
          sourceX,
          sourceY,
          targetX,
          targetY,
          sourcePosition,
          targetPosition,
        });

        return {
          edgePath: path,
          labelX: x,
          labelY: y,
        };
      })();

  const sign = index % 2 === 0 ? 1 : -1;
  const magnitude = Math.ceil(index / 2) * 20;
  const labelOffsetY = sign * magnitude;
  const tone = resolveEdgeTone(data);
  const haloColor =
    isConnected &&
    data?.areaColor &&
    tone.semanticState !== 'warning' &&
    tone.semanticState !== 'critical'
      ? data.areaColor
      : tone.haloColor;
  const strokeOpacity = isMuted ? 0.22 : isConnected ? 0.98 : isActive ? 0.94 : 0.72;
  const strokeWidth = isActive || isConnected ? tone.width + 0.7 : tone.width;
  const labelYOffset = labelY + labelOffsetY;
  const badgePresentation = useMemo(
    () =>
      resolveLinkBadgePresentation({
        data,
        zoom,
        path: edgePath,
        fallbackX: labelX,
        fallbackY: labelYOffset,
        edgeTone: tone,
        parallelIndex: data?.parallelIndex,
        isActive,
        isConnected,
        isMuted,
      }),
    [data, edgePath, isActive, isConnected, isMuted, labelX, labelYOffset, tone, zoom],
  );

  return (
    <>
      <path
        d={edgePath}
        fill="none"
        stroke="transparent"
        strokeWidth={18}
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

      {(isActive || isConnected) && (
        <BaseEdge
          id={`${id}-halo`}
          path={edgePath}
          style={{
            stroke: haloColor,
            strokeOpacity: isConnected ? 0.22 : 0.18,
            strokeWidth: strokeWidth + 4,
            transition: 'stroke-width 120ms ease, stroke-opacity 120ms ease',
          }}
        />
      )}

      <BaseEdge
        id={id}
        path={edgePath}
        style={{
          stroke: tone.color,
          strokeOpacity,
          strokeWidth,
          strokeDasharray: isMuted ? '10 12' : undefined,
          transition: 'stroke-width 120ms ease, stroke-opacity 120ms ease, stroke 120ms ease',
        }}
      />

      {badgePresentation.items.length > 0 ? (
        <EdgeLabelRenderer>
          <div
            data-testid={`${id}-badge-stack`}
            className="pointer-events-none absolute top-0 left-0 z-10 flex flex-col items-center gap-1.5 transition-[opacity,transform] duration-150"
            style={{
              position: 'absolute',
              transform: `translate(-50%, -50%) translate(${badgePresentation.anchor.x}px, ${badgePresentation.anchor.y}px) scale(${badgePresentation.scale})`,
              opacity: badgePresentation.opacity,
            }}
          >
            {badgePresentation.items.map((badge) => (
              <span
                key={`${id}-${badge.key}`}
                data-testid={`${id}-badge-${badge.key}`}
                title={badge.title}
                className={`inline-flex min-h-7 items-center gap-2 whitespace-nowrap rounded-full border bg-surface-container-high px-2.5 py-1.5 font-mono text-[11px] font-bold leading-none tracking-[0.06em] shadow-pill transition-[border-color,color] duration-150 ${badge.className}`}
                style={badge.style}
              >
                <span>{badge.text}</span>
                {badge.warningIndicator ? (
                  <span
                    data-testid={`${id}-badge-${badge.key}-warning`}
                    title={badge.warningIndicator.title}
                    className={`inline-flex h-4 min-w-4 items-center justify-center rounded-full border text-[10px] font-bold leading-none ${badge.warningIndicator.className}`}
                  >
                    {badge.warningIndicator.text}
                  </span>
                ) : null}
              </span>
            ))}
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
    prev.data?.alertStatus === next.data?.alertStatus &&
    prev.data?.bandwidthLabel === next.data?.bandwidthLabel &&
    prev.data?.speedLabel === next.data?.speedLabel &&
    prev.data?.throughputLabel === next.data?.throughputLabel &&
    prev.data?.negotiationTitle === next.data?.negotiationTitle &&
    prev.data?.autonegTitle === next.data?.autonegTitle &&
    prev.data?.speedMismatch === next.data?.speedMismatch &&
    prev.data?.negotiationState === next.data?.negotiationState &&
    prev.data?.sourceIfStatus === next.data?.sourceIfStatus &&
    prev.data?.targetIfStatus === next.data?.targetIfStatus &&
    prev.data?.sourceDeviceStatus === next.data?.sourceDeviceStatus &&
    prev.data?.targetDeviceStatus === next.data?.targetDeviceStatus &&
    prev.data?.sourceDeviceAlertStatus === next.data?.sourceDeviceAlertStatus &&
    prev.data?.targetDeviceAlertStatus === next.data?.targetDeviceAlertStatus &&
    prev.data?.sourceDeviceHealth === next.data?.sourceDeviceHealth &&
    prev.data?.targetDeviceHealth === next.data?.targetDeviceHealth &&
    prev.data?.sourceDevicePrimaryHealth === next.data?.sourceDevicePrimaryHealth &&
    prev.data?.targetDevicePrimaryHealth === next.data?.targetDevicePrimaryHealth &&
    prev.data?.sourceDeviceReachability === next.data?.sourceDeviceReachability &&
    prev.data?.targetDeviceReachability === next.data?.targetDeviceReachability &&
    prev.data?.sourceDeviceNetworkReachable === next.data?.sourceDeviceNetworkReachable &&
    prev.data?.targetDeviceNetworkReachable === next.data?.targetDeviceNetworkReachable &&
    prev.data?.sourceDeviceSnmpReachable === next.data?.sourceDeviceSnmpReachable &&
    prev.data?.targetDeviceSnmpReachable === next.data?.targetDeviceSnmpReachable &&
    prev.data?.areaColor === next.data?.areaColor &&
    prev.data?.emphasis === next.data?.emphasis &&
    prev.source === next.source &&
    prev.target === next.target &&
    prev.sourceX === next.sourceX &&
    prev.sourceY === next.sourceY &&
    prev.targetX === next.targetX &&
    prev.targetY === next.targetY &&
    prev.sourcePosition === next.sourcePosition &&
    prev.targetPosition === next.targetPosition
  );
});

export default LinkEdge;
