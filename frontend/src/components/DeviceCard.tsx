/**
 * Renders device card UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import { Handle, type Node, type NodeProps, Position } from '@xyflow/react';
import { type CSSProperties, memo, useLayoutEffect } from 'react';
import type { Device, Link } from '../types/api';
import {
  type AlertStatus,
  type DeviceMetricsDTO,
  type FreshnessStatus,
  formatUptime,
  metricColor,
  type RuntimeFlag,
} from '../types/metrics';
import { formatPollingEvery } from '../utils/freshness';
import { getEffectivePollingIntervalSeconds } from '../utils/polling';
import {
  isCanvasRenderMetricsEnabled,
  recordCanvasComponentRenderMetric,
} from './canvas/canvasInstrumentation';
import { resolveDeviceCardRenderModel } from './deviceCardVariant';
import {
  type DeviceMonitoringState,
  type DeviceVisualStatus,
  resolveDeviceAddressState,
  resolveDeviceNodeStatusStyles,
  resolveDeviceOperationalReadouts,
  resolveDeviceOperationalStatusState,
  resolveDeviceVisualState,
  sanitizeDeviceMetricsForDisplay,
} from './deviceVisualState';
import { MaterialIcon } from './MaterialIcon';
import { StatusDot } from './StatusDot';

/** Describes the device node data contract used by the UI component boundary. */
export interface DeviceNodeData {
  kind?: 'device' | 'ghost-device';
  device: Device;
  runtime: DeviceNodeRuntimeData;
  pinned: boolean;
  highlighted?: boolean;
  editMode?: boolean;
  areaColors?: string[];
  visualColor?: string;
  onContextMenu?: (event: React.MouseEvent, deviceId: string) => void;
  isGhost?: boolean;
  onGhostClick?: (deviceId: string) => void;
  isVirtual?: boolean;
  subtype?: string;
  selfLinks?: Link[];
  onSelfLinkClick?: (link: Link) => void;
  [key: string]: unknown;
}

/** Describes the device node contract used by the UI component boundary. */
export type DeviceNode = Node<DeviceNodeData>;

/** Describes the device node runtime data contract used by the UI component boundary. */
export interface DeviceNodeRuntimeData {
  status: Device['status'];
  metrics: DeviceMetricsDTO | null;
  alertStatus: AlertStatus;
  monitoringState: DeviceMonitoringState;
}

type ReadoutTone = 'default' | 'ok' | 'warning' | 'critical' | 'muted';

const universalHandleClassName =
  '!h-2 !w-2 !rounded-full !border-2 !border-bg !bg-surface-container-high shadow-none';

const deviceTypeLabels: Record<string, string> = {
  router: 'Router',
  switch: 'Switch',
  ap: 'AP',
  firewall: 'Firewall',
  virtual: 'Virtual',
  unknown: 'Node',
};

const subtypeLabels: Record<string, string> = {
  internet: 'Internet',
  cloud: 'Cloud',
  server: 'Server',
  generic: 'Virtual',
};

const macAddressPattern = /^([0-9A-Fa-f]{2}([:-])){5}[0-9A-Fa-f]{2}$/;
const dottedMacAddressPattern = /^([0-9A-Fa-f]{4}\.){2}[0-9A-Fa-f]{4}$/;

function displayName(device: Device): string {
  return device.tags?.display_name || device.sys_name || device.ip || device.hostname;
}

function isMacAddress(value: string): boolean {
  return macAddressPattern.test(value) || dottedMacAddressPattern.test(value);
}

function deviceTypeLabel(device: Device, isVirtual: boolean, subtype?: string): string {
  if (isVirtual) {
    return subtypeLabels[subtype ?? 'generic'] ?? 'Virtual';
  }
  return deviceTypeLabels[device.device_type] ?? 'Node';
}

function formatSelfLinkLabel(link: Link): string {
  const protocol = link.discovery_protocol.trim().toUpperCase();
  return protocol ? `Self ${protocol}` : 'Self link';
}

function formatSelfLinkSummary(link: Link): string {
  const source = link.source_if_name.trim() || '?';
  const target = link.target_if_name.trim() || '?';
  return `${source} -> ${target}`;
}

function sameSelfLinks(previous: Link[] | undefined, next: Link[] | undefined): boolean {
  if (previous?.length !== next?.length) {
    return false;
  }

  return (previous ?? []).every((link, index) => {
    const candidate = next?.[index];
    return (
      !!candidate &&
      link.id === candidate.id &&
      link.source_if_name === candidate.source_if_name &&
      link.target_if_name === candidate.target_if_name &&
      link.discovery_protocol === candidate.discovery_protocol
    );
  });
}

function sameRuntimeFlags(
  previous: RuntimeFlag[] | undefined,
  next: RuntimeFlag[] | undefined,
): boolean {
  if (previous?.length !== next?.length) {
    return false;
  }

  return (previous ?? []).every((flag, index) => flag === next?.[index]);
}

function freshnessTone(tier: 'Fresh' | 'Stale' | 'Dead'): ReadoutTone {
  switch (tier) {
    case 'Fresh':
      return 'ok';
    case 'Stale':
      return 'warning';
    case 'Dead':
      return 'critical';
  }
}

function freshnessMeta(freshness: FreshnessStatus): {
  tone: ReadoutTone;
  text: string;
} {
  switch (freshness) {
    case 'fresh':
      return { tone: freshnessTone('Fresh'), text: 'Fresh telemetry' };
    case 'stale':
      return { tone: freshnessTone('Stale'), text: 'Stale telemetry' };
    case 'awaiting_poll':
      return { tone: freshnessTone('Dead'), text: 'Awaiting first poll' };
    case 'unmonitored':
      return { tone: 'muted', text: 'Unmonitored' };
  }
}

function runtimeTelemetryMeta(metrics: DeviceMetricsDTO): {
  tone: ReadoutTone;
  text: string;
} {
  if (
    metrics.primary_health === 'snmp_degraded' ||
    metrics.reachability === 'soft_down' ||
    metrics.snmp_reachable === 'false'
  ) {
    return { tone: 'warning', text: 'SNMP unreachable' };
  }

  return freshnessMeta(metrics.freshness);
}

function readoutToneClass(tone: ReadoutTone): string {
  switch (tone) {
    case 'ok':
      return 'text-status-up';
    case 'warning':
      return 'text-warning';
    case 'critical':
      return 'text-status-down';
    case 'muted':
      return 'text-on-bg-secondary';
    default:
      return 'text-on-bg';
  }
}

const compactPercentFormatter = new Intl.NumberFormat('en-US', {
  maximumFractionDigits: 0,
});
const DEVICE_NODE_SCALE_START_ZOOM = 0.9;
const DEVICE_NODE_MIN_SCALE_ZOOM = 0.6;
const DEVICE_NODE_MAX_READABILITY_SCALE = 1.12;
const DEVICE_NODE_READABILITY_SCALE_CSS_VAR = 'var(--theia-device-node-readability-scale, 1)';
const DEVICE_NODE_IDENTITY_SCALE_CSS_VAR = 'var(--theia-device-node-identity-scale, 1)';

/** Resolves device node readability scale for the UI component boundary. */
export function resolveDeviceNodeReadabilityScale(zoom: number): number {
  const safeZoom = Number.isFinite(zoom) && zoom > 0 ? zoom : 1;

  if (safeZoom >= 1) {
    return 1;
  }

  const zoomRange = DEVICE_NODE_SCALE_START_ZOOM - DEVICE_NODE_MIN_SCALE_ZOOM;
  const scaleRange = DEVICE_NODE_MAX_READABILITY_SCALE - 1;
  const progress = Math.min(1, Math.max(0, (DEVICE_NODE_SCALE_START_ZOOM - safeZoom) / zoomRange));
  const scale = 1 + progress * scaleRange;
  return Number(scale.toFixed(2));
}

function readableFontStyle(basePx: number): CSSProperties {
  return {
    fontSize: `calc(${basePx}px * ${DEVICE_NODE_READABILITY_SCALE_CSS_VAR})`,
  };
}

function readableIdentityFontStyle(basePx: number): CSSProperties {
  return {
    fontSize: `calc(${basePx}px * ${DEVICE_NODE_READABILITY_SCALE_CSS_VAR} * ${DEVICE_NODE_IDENTITY_SCALE_CSS_VAR})`,
  };
}

function readableHeightStyle(basePx: number): CSSProperties {
  return {
    height: `calc(${basePx}px * ${DEVICE_NODE_READABILITY_SCALE_CSS_VAR})`,
  };
}

function mergeReadableFontStyle(
  baseStyle: CSSProperties | undefined,
  basePx: number,
): CSSProperties | undefined {
  const fontStyle = readableFontStyle(basePx);

  if (!baseStyle) {
    return fontStyle;
  }

  return { ...baseStyle, ...fontStyle };
}

function formatRuntimePercent(value: number | null | undefined): string {
  return value === null || value === undefined ? '-' : `${compactPercentFormatter.format(value)}%`;
}

function formatRuntimeUptime(value: number | null | undefined): string {
  return value === null || value === undefined ? '-' : formatUptime(value);
}

function runtimeMetricValueClass(value: number | null | undefined): string {
  return value === null || value === undefined ? 'text-on-bg-secondary' : metricColor(value);
}

function runtimeBadgeLabel(flag: RuntimeFlag): string {
  switch (flag) {
    case 'deadline_missed':
      return 'Late';
    case 'overloaded':
      return 'Overload';
    case 'background_pending':
      return 'Background';
    case 'partial_telemetry':
      return 'Partial';
    case 'degraded_risk':
      return 'Risk';
    case 'persistence_lagging':
      return 'Persisting';
  }
}

function ghostFrameStyle(color?: string): CSSProperties | undefined {
  if (!color) return undefined;
  return {
    borderColor: color,
    color,
  };
}

interface RgbColor {
  red: number;
  green: number;
  blue: number;
}

type CSSCustomProperties = CSSProperties & Record<`--${string}`, string>;

interface VirtualStatusTone {
  capsuleStyle: CSSProperties;
  markerStyle: CSSProperties;
  textStyle: CSSProperties;
}

interface PhysicalStatusTone {
  bodyStyle: CSSProperties;
}

const virtualAreaToneSurface: RgbColor = { red: 17, green: 26, blue: 38 };
const whiteRgb: RgbColor = { red: 255, green: 255, blue: 255 };
const blackRgb: RgbColor = { red: 0, green: 0, blue: 0 };
const minimumVirtualAreaToneContrast = 4.5;

function hexToRgb(color: string): RgbColor | null {
  const normalized = color.trim().replace(/^#/, '');
  const hex =
    normalized.length === 3
      ? normalized
          .split('')
          .map((character) => `${character}${character}`)
          .join('')
      : normalized;

  if (!/^[0-9a-fA-F]{6}$/.test(hex)) {
    return null;
  }

  return {
    red: Number.parseInt(hex.slice(0, 2), 16),
    green: Number.parseInt(hex.slice(2, 4), 16),
    blue: Number.parseInt(hex.slice(4, 6), 16),
  };
}

function rgbToCss({ red, green, blue }: RgbColor): string {
  return `rgb(${red}, ${green}, ${blue})`;
}

function rgbToRgba({ red, green, blue }: RgbColor, alpha: number): string {
  return `rgba(${red}, ${green}, ${blue}, ${alpha})`;
}

function hexToRgba(color: string, alpha: number): string | null {
  const rgb = hexToRgb(color);
  return rgb ? rgbToRgba(rgb, alpha) : null;
}

function relativeLuminance({ red, green, blue }: RgbColor): number {
  const channelLuminance = (value: number) => {
    const channel = value / 255;
    return channel <= 0.03928 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
  };

  return (
    0.2126 * channelLuminance(red) +
    0.7152 * channelLuminance(green) +
    0.0722 * channelLuminance(blue)
  );
}

function contrastRatio(foreground: RgbColor, background: RgbColor): number {
  const foregroundLuminance = relativeLuminance(foreground);
  const backgroundLuminance = relativeLuminance(background);
  const lighter = Math.max(foregroundLuminance, backgroundLuminance);
  const darker = Math.min(foregroundLuminance, backgroundLuminance);
  return (lighter + 0.05) / (darker + 0.05);
}

function mixRgb(start: RgbColor, end: RgbColor, amount: number): RgbColor {
  return {
    red: Math.round(start.red + (end.red - start.red) * amount),
    green: Math.round(start.green + (end.green - start.green) * amount),
    blue: Math.round(start.blue + (end.blue - start.blue) * amount),
  };
}

function readableVirtualAreaTone(
  color: string,
  background: RgbColor,
  mixTarget: RgbColor,
): string | null {
  const rgb = hexToRgb(color);
  if (!rgb) return null;

  for (let mixAmount = 0; mixAmount <= 1; mixAmount += 0.04) {
    const candidate = mixRgb(rgb, mixTarget, mixAmount);
    if (contrastRatio(candidate, background) >= minimumVirtualAreaToneContrast) {
      return rgbToCss(candidate);
    }
  }

  return rgbToCss(mixTarget);
}

function virtualAreaToneStyle(color?: string): CSSCustomProperties | undefined {
  if (!color) return undefined;

  const darkTone = readableVirtualAreaTone(color, virtualAreaToneSurface, whiteRgb);
  const lightTone = readableVirtualAreaTone(color, whiteRgb, blackRgb);
  if (!darkTone || !lightTone) return undefined;

  return {
    '--theia-virtual-node-tone-dark': darkTone,
    '--theia-virtual-node-tone-light': lightTone,
  };
}

function areaTintStyle(colors: string[] | undefined, alpha = 0.1): CSSProperties | undefined {
  const tintColors = (colors ?? [])
    .map((color) => hexToRgba(color, alpha))
    .filter((color): color is string => !!color);

  if (tintColors.length === 0) return undefined;

  if (tintColors.length === 1) {
    return {
      backgroundColor: tintColors[0],
    };
  }

  return {
    background: `linear-gradient(135deg, ${tintColors.join(', ')})`,
  };
}

function virtualAreaMarkerStyle(color?: string): CSSProperties | undefined {
  if (!color) return undefined;

  const rgb = hexToRgb(color);
  const toneStyle = virtualAreaToneStyle(color);
  if (!rgb || !toneStyle) return undefined;

  return {
    ...toneStyle,
    backgroundColor: rgbToRgba(rgb, 0.14),
    borderColor: rgbToRgba(rgb, 0.32),
  };
}

function virtualAreaTextStyle(color?: string): CSSProperties | undefined {
  return virtualAreaToneStyle(color);
}

function virtualPrimaryStatusTone(status: DeviceVisualStatus): VirtualStatusTone | null {
  switch (status) {
    case 'down':
      return {
        capsuleStyle: {
          backgroundColor: 'var(--nt-node-down-card-bg)',
        },
        markerStyle: {
          backgroundColor: 'var(--nt-node-down-badge-bg)',
          borderColor: 'var(--nt-node-down-border)',
          boxShadow: '0 0 0 1px var(--nt-node-down-ring), 0 0 18px var(--nt-node-down-glow)',
          color: 'var(--nt-status-down)',
        },
        textStyle: { color: 'var(--nt-status-down)' },
      };
    case 'probing':
      return {
        capsuleStyle: {
          backgroundColor: 'var(--nt-node-probing-card-bg)',
        },
        markerStyle: {
          backgroundColor: 'var(--nt-node-probing-badge-bg)',
          borderColor: 'var(--nt-node-probing-border)',
          boxShadow: '0 0 0 1px var(--nt-node-probing-ring), 0 0 18px var(--nt-node-probing-glow)',
          color: 'var(--nt-status-probing)',
        },
        textStyle: { color: 'var(--nt-status-probing)' },
      };
    default:
      return null;
  }
}

function physicalPrimaryStatusTone(status: DeviceVisualStatus): PhysicalStatusTone | null {
  switch (status) {
    case 'down':
      return {
        bodyStyle: {
          backgroundColor: 'var(--nt-node-down-card-bg)',
        },
      };
    case 'probing':
      return {
        bodyStyle: {
          backgroundColor: 'var(--nt-node-probing-card-bg)',
        },
      };
    default:
      return null;
  }
}

function physicalStatusAccent(status: DeviceVisualStatus): string | undefined {
  switch (status) {
    case 'down':
      return 'var(--nt-node-down-border)';
    case 'probing':
      return 'var(--nt-node-probing-border)';
    default:
      return undefined;
  }
}

function PollingDisabledNotice({ className = '' }: { className?: string }) {
  return (
    <div
      className={`rounded-2xl border border-outline-strong bg-surface-container-high px-3 py-2 text-center text-[11px] font-semibold uppercase text-on-bg-secondary ${className}`}
    >
      Continuous polling disabled
    </div>
  );
}

function DeviceCardInner({ data, selected }: NodeProps<DeviceNode>) {
  const renderStartedAt =
    isCanvasRenderMetricsEnabled() && typeof performance !== 'undefined' ? performance.now() : null;
  const runtime = data.runtime;
  const runtimeDevice =
    data.device.status === runtime.status
      ? data.device
      : { ...data.device, status: runtime.status };
  const monitoringState = runtime.monitoringState;
  const isPollingDisabled =
    monitoringState === 'monitorable' && data.device.polling_enabled === false;
  const isVirtual = data.isVirtual === true;
  const metrics = sanitizeDeviceMetricsForDisplay(runtimeDevice, runtime.metrics, monitoringState);
  const headerState =
    metrics || isVirtual
      ? resolveDeviceVisualState(runtimeDevice, metrics, monitoringState)
      : resolveDeviceOperationalStatusState(runtimeDevice, monitoringState);
  const telemetryFallback =
    monitoringState === 'monitorable' && !isVirtual && !metrics
      ? headerState.dotStatus === 'probing'
        ? { tone: 'critical' as const, text: 'Awaiting first poll' }
        : { tone: 'muted' as const, text: 'Unmonitored' }
      : null;
  const freshness =
    monitoringState === 'monitorable' && metrics
      ? runtimeTelemetryMeta(metrics)
      : telemetryFallback;
  const pollingEvery =
    monitoringState === 'monitorable' && metrics
      ? formatPollingEvery(
          metrics.expected_poll_interval_seconds ?? getEffectivePollingIntervalSeconds(data.device),
        )
      : null;
  const label = displayName(data.device);
  const colors = data.areaColors ?? [];
  const hasArea = colors.length > 0;
  const firstColor = colors[0];
  const virtualToneColor = isVirtual ? (data.visualColor ?? firstColor) : firstColor;
  const areaAccent =
    colors.length >= 2 ? `linear-gradient(90deg, ${colors.join(', ')})` : firstColor;
  const addressLabel = isMacAddress(data.device.ip) ? 'MAC' : 'IP';
  const addressState = resolveDeviceAddressState(data.device);
  const renderModel = resolveDeviceCardRenderModel({
    device: data.device,
    monitoringState,
    addressState,
    hasFreshnessMeta: freshness !== null,
  });
  const selfLinks = data.selfLinks ?? [];
  const primarySelfLink = selfLinks[0];
  const statusStyles = resolveDeviceNodeStatusStyles({
    status: headerState.dotStatus,
    selected,
    highlighted: data.highlighted === true,
  });
  const runtimeBadges = metrics?.runtime_flags.map(runtimeBadgeLabel) ?? [];
  const renderKind = data.kind === 'ghost-device' || data.isGhost ? 'ghost-device' : 'device';
  const renderVariant = renderKind === 'device' ? renderModel.variant : undefined;
  const isVirtualMonitorable = renderModel.variant === 'virtual-monitorable';
  const virtualStatusTone = isVirtualMonitorable
    ? virtualPrimaryStatusTone(headerState.dotStatus)
    : null;
  const physicalStatusTone =
    renderModel.variant === 'physical' ? physicalPrimaryStatusTone(headerState.dotStatus) : null;
  const physicalAccentBackground =
    hasArea && areaAccent ? areaAccent : physicalStatusAccent(headerState.dotStatus);
  const showPendingPhysicalReadouts =
    renderModel.variant === 'physical' && !metrics && headerState.dotStatus === 'probing';
  const physicalReadouts =
    renderModel.variant === 'physical' && (metrics || showPendingPhysicalReadouts)
      ? resolveDeviceOperationalReadouts(runtimeDevice, metrics, monitoringState)
      : null;
  const cardShapeClass = !isVirtual
    ? 'min-h-[140px] min-w-[268px] max-w-[370px] rounded-[20px]'
    : isVirtualMonitorable
      ? 'min-h-[128px] min-w-[292px] max-w-[430px] rounded-[24px]'
      : 'min-h-[102px] min-w-[242px] max-w-[350px] rounded-[24px]';
  const virtualCapsuleHeightClass = isVirtualMonitorable ? 'min-h-[126px]' : 'min-h-[100px]';
  const virtualCapsulePaddingClass = isVirtualMonitorable
    ? 'py-3 pl-3.5 pr-4'
    : 'py-2.5 pl-3.5 pr-3.5';

  useLayoutEffect(() => {
    if (renderStartedAt === null) {
      return;
    }

    recordCanvasComponentRenderMetric(
      'DeviceCard',
      Math.max(0, performance.now() - renderStartedAt),
      {
        deviceId: data.device.id,
        kind: renderKind,
        hasMetrics: metrics !== null,
        selected,
        ...(renderVariant ? { variant: renderVariant } : {}),
      },
    );
  });

  if (data.kind === 'ghost-device' || data.isGhost) {
    const ghostLabel =
      data.device.sys_name || data.device.tags?.display_name || data.device.ip || 'Ghost';
    return (
      <>
        <Handle type="target" position={Position.Top} className={universalHandleClassName} />
        <div
          data-testid="device-node-card"
          data-topology-node-variant="ghost-device"
          className="topology-node-card topology-render-contained relative w-[132px] cursor-pointer rounded-2xl border border-dashed border-outline bg-surface/72 text-center transition-[border-color,background-color,color] duration-150 hover:bg-surface-container"
          style={{ ...ghostFrameStyle(firstColor), boxShadow: 'var(--nt-node-shadow)' }}
          onClick={() => data.onGhostClick?.(data.device.id)}
          role="button"
          tabIndex={0}
          onKeyDown={(event) => {
            if (event.key === 'Enter' || event.key === ' ') {
              data.onGhostClick?.(data.device.id);
            }
          }}
        >
          <div data-testid="semantic-detail-node" className="topology-semantic-card px-3 py-2">
            <p className="topology-semantic-detail-only truncate text-[11px] font-medium uppercase text-on-bg-secondary">
              cross-area
            </p>
            <p
              className="topology-semantic-identity mt-1 text-sm font-semibold text-on-bg"
              style={readableIdentityFontStyle(14)}
            >
              <span className="topology-semantic-identity-text block truncate">{ghostLabel}</span>
            </p>
          </div>
        </div>
        <Handle type="source" position={Position.Bottom} className={universalHandleClassName} />
      </>
    );
  }

  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: The card shell only owns pointer context-menu plumbing; child controls expose keyboard actions.
    <div
      data-testid="device-node-card"
      data-topology-node-variant={renderVariant}
      className={`topology-node-card topology-render-contained group relative w-full border border-outline bg-surface transition-[border-color] duration-150 hover:border-outline-strong ${cardShapeClass} ${statusStyles.frameClass ?? ''}`}
      style={statusStyles.frameStyle}
      onContextMenu={(event) => {
        if (!data.onContextMenu) return;
        event.preventDefault();
        event.stopPropagation();
        data.onContextMenu(event, data.device.id);
      }}
    >
      {primarySelfLink ? (
        <button
          type="button"
          className="topology-semantic-detail-only absolute left-1/2 top-0 z-20 flex max-w-[calc(100%-1rem)] -translate-x-1/2 -translate-y-1/2 items-center gap-2 rounded-full border border-primary/25 bg-surface-container-high px-3 py-1.5 text-left transition-[border-color,background-color] duration-150 hover:border-primary/45"
          onMouseDown={(event) => {
            event.stopPropagation();
          }}
          onClick={(event) => {
            event.stopPropagation();
            data.onSelfLinkClick?.(primarySelfLink);
          }}
          aria-label={`View details for self link ${formatSelfLinkSummary(primarySelfLink)}`}
        >
          <span className="shrink-0 rounded-full bg-primary/10 px-2 py-0.5 text-[11px] font-semibold uppercase text-primary">
            {formatSelfLinkLabel(primarySelfLink)}
          </span>
          <span className="min-w-0 truncate font-mono text-[11px] text-on-bg-secondary">
            {formatSelfLinkSummary(primarySelfLink)}
          </span>
          {selfLinks.length > 1 ? (
            <span className="shrink-0 rounded-full border border-outline px-1.5 py-0.5 text-[11px] font-semibold text-on-bg-secondary">
              +{selfLinks.length - 1}
            </span>
          ) : null}
        </button>
      ) : null}
      <Handle
        id="top"
        type="source"
        position={Position.Top}
        isConnectable={!!data.editMode}
        style={{ pointerEvents: data.editMode ? 'auto' : 'none' }}
        className={`${universalHandleClassName} !-top-1 !left-1/2 !-translate-x-1/2 opacity-0 transition-opacity duration-200 group-hover:opacity-100 z-10`}
      />
      <Handle
        id="right"
        type="source"
        position={Position.Right}
        isConnectable={!!data.editMode}
        style={{ pointerEvents: data.editMode ? 'auto' : 'none' }}
        className={`${universalHandleClassName} !-right-1 !top-1/2 !-translate-y-1/2 opacity-0 transition-opacity duration-200 group-hover:opacity-100 z-10`}
      />
      <Handle
        id="bottom"
        type="source"
        position={Position.Bottom}
        isConnectable={!!data.editMode}
        style={{ pointerEvents: data.editMode ? 'auto' : 'none' }}
        className={`${universalHandleClassName} !-bottom-1 !left-1/2 !-translate-x-1/2 opacity-0 transition-opacity duration-200 group-hover:opacity-100 z-10`}
      />
      <Handle
        id="left"
        type="source"
        position={Position.Left}
        isConnectable={!!data.editMode}
        style={{ pointerEvents: data.editMode ? 'auto' : 'none' }}
        className={`${universalHandleClassName} !-left-1 !top-1/2 !-translate-y-1/2 opacity-0 transition-opacity duration-200 group-hover:opacity-100 z-10`}
      />

      <div
        data-testid="semantic-detail-node"
        className={
          isVirtual
            ? 'topology-semantic-card overflow-hidden rounded-[23px]'
            : 'topology-semantic-card flex min-h-[inherit] flex-col overflow-hidden rounded-[19px]'
        }
      >
        {renderModel.variant === 'physical' ? (
          <div
            data-testid="physical-node-area-accent"
            className="h-1.5 w-full"
            style={physicalAccentBackground ? { background: physicalAccentBackground } : undefined}
          />
        ) : null}

        {renderModel.variant === 'physical' ? (
          <div
            data-testid="physical-node-body"
            className="topology-physical-node-body flex-1 px-4 pb-3.5 pt-3"
            style={physicalStatusTone?.bodyStyle ?? areaTintStyle(colors, 0.18)}
          >
            <div className="topology-semantic-header flex items-start justify-between gap-3">
              <div className="topology-semantic-identity-frame min-w-0 flex-1">
                <div
                  data-testid="physical-node-hostname"
                  className="topology-semantic-identity min-w-0 text-[15px] font-semibold leading-snug text-on-bg"
                  style={readableIdentityFontStyle(15)}
                >
                  <span className="topology-semantic-identity-text line-clamp-2 break-words">
                    {label}
                  </span>
                </div>
              </div>

              <div
                data-testid="physical-node-status-badge"
                className={`topology-semantic-status-badge inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-semibold ${statusStyles.badgeClass}`}
                style={mergeReadableFontStyle(statusStyles.badgeStyle, 11)}
              >
                <StatusDot status={headerState.dotStatus} />
                <span className="topology-semantic-status-label">{headerState.label}</span>
              </div>
            </div>

            <div className="topology-semantic-summary-row mt-3 flex items-center justify-between gap-3">
              {addressState === 'address' ? (
                <span
                  data-testid="physical-node-address"
                  className="topology-semantic-summary-field min-w-0 truncate rounded-full border border-outline bg-surface-container px-2.5 py-1 font-mono text-[11px] font-medium text-on-bg"
                  style={readableFontStyle(11)}
                >
                  {addressLabel} {data.device.ip}
                </span>
              ) : (
                <span
                  data-testid="physical-node-address"
                  className="topology-semantic-summary-field rounded-full border border-outline bg-surface-container px-2.5 py-1 text-[11px] font-semibold text-on-bg-secondary"
                  style={readableFontStyle(11)}
                >
                  No IP
                </span>
              )}

              {renderModel.showFreshnessMeta ? (
                <div className="min-w-0 truncate text-right">
                  <div
                    data-testid="physical-node-freshness"
                    className={`topology-semantic-detail-only truncate text-[11px] font-semibold ${readoutToneClass(freshness!.tone)}`}
                    style={readableFontStyle(11)}
                  >
                    {freshness!.text}
                  </div>
                </div>
              ) : null}
            </div>

            {physicalReadouts ? (
              <div
                className="topology-semantic-detail-only mt-2 grid h-[40px] grid-cols-3 overflow-hidden rounded-xl border border-outline-subtle bg-surface-container/55"
                data-testid="physical-runtime-readouts"
                style={readableHeightStyle(40)}
              >
                <div className="flex min-w-0 flex-col justify-center border-outline-subtle border-r px-2.5">
                  <span
                    className="truncate text-[11px] font-semibold uppercase leading-none text-on-bg-secondary"
                    style={readableFontStyle(9)}
                  >
                    CPU
                  </span>
                  <span
                    className={`mt-1 truncate font-mono text-[12px] font-semibold leading-none ${runtimeMetricValueClass(physicalReadouts.cpuPercent)}`}
                    style={readableFontStyle(12)}
                  >
                    {formatRuntimePercent(physicalReadouts.cpuPercent)}
                  </span>
                </div>
                <div className="flex min-w-0 flex-col justify-center border-outline-subtle border-r px-2.5">
                  <span
                    className="truncate text-[11px] font-semibold uppercase leading-none text-on-bg-secondary"
                    style={readableFontStyle(9)}
                  >
                    MEM
                  </span>
                  <span
                    className={`mt-1 truncate font-mono text-[12px] font-semibold leading-none ${runtimeMetricValueClass(physicalReadouts.memPercent)}`}
                    style={readableFontStyle(12)}
                  >
                    {formatRuntimePercent(physicalReadouts.memPercent)}
                  </span>
                </div>
                <div className="flex min-w-0 flex-col justify-center px-2.5">
                  <span
                    className="truncate text-[11px] font-semibold uppercase leading-none text-on-bg-secondary"
                    style={readableFontStyle(9)}
                  >
                    Uptime
                  </span>
                  <span
                    className="mt-1 truncate font-mono text-[12px] font-semibold leading-none text-on-bg"
                    style={readableFontStyle(12)}
                  >
                    {formatRuntimeUptime(physicalReadouts.uptimeSecs)}
                  </span>
                </div>
              </div>
            ) : null}
          </div>
        ) : (
          <div
            data-testid="virtual-node-capsule"
            className={`topology-virtual-node-capsule relative flex ${virtualCapsuleHeightClass} items-center gap-3 rounded-[23px] ${virtualCapsulePaddingClass}`}
            style={
              virtualStatusTone?.capsuleStyle ??
              areaTintStyle(data.visualColor ? [data.visualColor] : colors, 0.18)
            }
          >
            {hasArea && areaAccent ? (
              <div
                data-testid="virtual-node-area-accent"
                className="absolute inset-y-0 left-0 w-1 rounded-l-[23px]"
                style={{ background: areaAccent }}
              />
            ) : null}

            <div
              data-testid="virtual-node-icon-shell"
              className="topology-virtual-node-icon-shell relative z-10 flex h-[50px] w-[50px] shrink-0 items-center justify-center rounded-[18px] border border-primary/25 bg-primary/10"
              style={virtualStatusTone?.markerStyle ?? virtualAreaMarkerStyle(virtualToneColor)}
            >
              <MaterialIcon name="hub" size={24} />
            </div>

            <div className="topology-virtual-node-content relative z-10 min-w-0 flex-1 text-left">
              <div className="topology-semantic-header flex min-w-0 items-start justify-between gap-2">
                <div className="topology-semantic-identity-frame min-w-0 flex-1">
                  <div
                    data-testid="virtual-node-type-label"
                    className="topology-virtual-node-type-label truncate text-[11px] font-semibold uppercase"
                    style={{
                      ...readableFontStyle(10),
                      ...(virtualStatusTone?.textStyle ?? virtualAreaTextStyle(virtualToneColor)),
                    }}
                  >
                    {deviceTypeLabel(data.device, isVirtual, data.subtype)}
                  </div>
                  <div
                    data-testid="virtual-node-hostname"
                    className="topology-semantic-identity mt-1 text-[17px] font-semibold leading-tight text-on-bg"
                    style={readableIdentityFontStyle(17)}
                  >
                    <span className="topology-semantic-identity-text block truncate">{label}</span>
                  </div>
                </div>

                {renderModel.showVirtualStatusBadge ? (
                  <div
                    data-testid="virtual-node-status-badge"
                    className={`topology-semantic-status-badge inline-flex max-w-[82px] shrink-0 items-center gap-1.5 rounded-full border px-2 py-0.5 text-[11px] font-semibold ${statusStyles.badgeClass}`}
                    style={mergeReadableFontStyle(statusStyles.badgeStyle, 10)}
                  >
                    <StatusDot status={headerState.dotStatus} />
                    <span className="topology-semantic-status-label truncate">
                      {headerState.label}
                    </span>
                  </div>
                ) : null}
              </div>

              {renderModel.showVirtualAddressChip ? (
                <span
                  data-testid="virtual-node-address"
                  className="topology-semantic-summary-field mt-1.5 inline-block max-w-full truncate rounded-full border border-outline bg-surface-container-high px-2.5 py-0.5 font-mono text-[11px] text-on-bg"
                  style={readableFontStyle(11)}
                >
                  {addressLabel} {data.device.ip}
                </span>
              ) : null}

              {renderModel.showFreshnessMeta ? (
                <div
                  data-testid="virtual-node-runtime-meta"
                  className="topology-semantic-detail-only mt-1.5 flex w-full items-center justify-between gap-2 overflow-hidden text-[11px]"
                >
                  <div
                    className={`min-w-0 truncate font-medium ${readoutToneClass(freshness!.tone)}`}
                    style={readableFontStyle(10)}
                  >
                    {freshness!.text}
                  </div>
                  <div
                    data-testid="virtual-node-polling-meta"
                    className="min-w-0 shrink-0 truncate text-right text-on-bg-secondary"
                    style={readableFontStyle(10)}
                  >
                    {pollingEvery}
                  </div>
                </div>
              ) : null}

              {isPollingDisabled ? (
                <PollingDisabledNotice className="topology-semantic-detail-only mt-2 w-full" />
              ) : null}

              {runtimeBadges.length > 0 ? (
                <div
                  data-testid="virtual-node-runtime-flags"
                  className="topology-semantic-detail-only mt-2 flex w-full flex-wrap gap-1.5"
                >
                  {runtimeBadges.map((badge) => (
                    <span
                      key={badge}
                      className="rounded-full border border-warning/30 bg-warning/10 px-2 py-0.5 text-[11px] font-semibold uppercase text-warning"
                    >
                      {badge}
                    </span>
                  ))}
                </div>
              ) : null}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

/** Returns device render signature for the UI component boundary. */
export function getDeviceRenderSignature(props: NodeProps<DeviceNode>) {
  const data = props.data;
  const runtime = data.runtime;
  const metrics = runtime.metrics;

  return {
    deviceId: data.device.id,
    staticStatus: data.device.status,
    runtimeStatus: runtime.status,
    vendor: data.device.vendor,
    sysName: data.device.sys_name,
    hardwareModel: data.device.hardware_model,
    displayName: data.device.tags?.display_name,
    ip: data.device.ip,
    pollingEnabled: data.device.polling_enabled,
    areaIds: data.device.area_ids ?? [],
    highlighted: data.highlighted,
    alertStatus: runtime.alertStatus,
    areaColors: data.areaColors ?? [],
    visualColor: data.visualColor,
    kind: data.kind,
    isGhost: data.isGhost,
    isVirtual: data.isVirtual,
    monitoringState: runtime.monitoringState,
    subtype: data.subtype,
    selfLinks: data.selfLinks,
    cpuPercent: metrics?.cpu_percent,
    memPercent: metrics?.mem_percent,
    uptimeSecs: metrics?.uptime_secs,
    health: metrics?.health,
    primaryHealth: metrics?.primary_health,
    reachability: metrics?.reachability,
    networkReachable: metrics?.network_reachable,
    snmpReachable: metrics?.snmp_reachable,
    runtimeFlags: metrics?.runtime_flags,
    freshness: metrics?.freshness,
    expectedPollIntervalSeconds: metrics?.expected_poll_interval_seconds,
    editMode: data.editMode,
    selected: props.selected,
  };
}

type DeviceRenderSignature = ReturnType<typeof getDeviceRenderSignature>;

function sameStringArray(previous: string[] | undefined, next: string[] | undefined): boolean {
  if (previous?.length !== next?.length) {
    return false;
  }

  return (previous ?? []).every((value, index) => value === next?.[index]);
}

function sameDeviceRenderSignature(
  previous: DeviceRenderSignature,
  next: DeviceRenderSignature,
): boolean {
  return (
    previous.deviceId === next.deviceId &&
    previous.staticStatus === next.staticStatus &&
    previous.runtimeStatus === next.runtimeStatus &&
    previous.vendor === next.vendor &&
    previous.sysName === next.sysName &&
    previous.hardwareModel === next.hardwareModel &&
    previous.displayName === next.displayName &&
    previous.ip === next.ip &&
    previous.pollingEnabled === next.pollingEnabled &&
    sameStringArray(previous.areaIds, next.areaIds) &&
    previous.highlighted === next.highlighted &&
    previous.alertStatus === next.alertStatus &&
    sameStringArray(previous.areaColors, next.areaColors) &&
    previous.visualColor === next.visualColor &&
    previous.kind === next.kind &&
    previous.isGhost === next.isGhost &&
    previous.isVirtual === next.isVirtual &&
    previous.monitoringState === next.monitoringState &&
    previous.subtype === next.subtype &&
    sameSelfLinks(previous.selfLinks, next.selfLinks) &&
    previous.cpuPercent === next.cpuPercent &&
    previous.memPercent === next.memPercent &&
    previous.uptimeSecs === next.uptimeSecs &&
    previous.health === next.health &&
    previous.primaryHealth === next.primaryHealth &&
    previous.reachability === next.reachability &&
    previous.networkReachable === next.networkReachable &&
    previous.snmpReachable === next.snmpReachable &&
    sameRuntimeFlags(previous.runtimeFlags, next.runtimeFlags) &&
    previous.freshness === next.freshness &&
    previous.expectedPollIntervalSeconds === next.expectedPollIntervalSeconds &&
    previous.editMode === next.editMode &&
    previous.selected === next.selected
  );
}

const DeviceCard = memo(
  DeviceCardInner,
  (prev: NodeProps<DeviceNode>, next: NodeProps<DeviceNode>) =>
    sameDeviceRenderSignature(getDeviceRenderSignature(prev), getDeviceRenderSignature(next)),
);

export default DeviceCard;
