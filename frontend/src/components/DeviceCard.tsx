import { Handle, type Node, type NodeProps, Position, useStore } from '@xyflow/react';
import { type CSSProperties, memo } from 'react';
import type { Device, Link } from '../types/api';
import {
  type AlertStatus,
  type DeviceMetricsDTO,
  type FreshnessStatus,
  type RuntimeFlag,
  formatUptime,
  metricColor,
} from '../types/metrics';
import { formatPollingEvery } from '../utils/freshness';
import { getEffectivePollingIntervalSeconds } from '../utils/polling';
import { StatusDot } from './StatusDot';
import { resolveDeviceCardRenderModel } from './deviceCardVariant';
import {
  type DeviceMonitoringState,
  resolveDeviceAddressState,
  resolveDeviceMonitoringState,
  resolveDeviceNodeStatusStyles,
  resolveDeviceOperationalReadouts,
  resolveDeviceOperationalStatusState,
  resolveDeviceVisualState,
  sanitizeDeviceMetricsForDisplay,
} from './deviceVisualState';
import { VendorIcon } from './icons/VendorIcon';

export interface DeviceNodeData {
  kind?: 'device' | 'ghost-device';
  device: Device;
  pinned: boolean;
  highlighted?: boolean;
  editMode?: boolean;
  metrics?: DeviceMetricsDTO | null;
  alertStatus?: AlertStatus;
  areaColors?: string[];
  onContextMenu?: (event: React.MouseEvent, deviceId: string) => void;
  isGhost?: boolean;
  onGhostClick?: (deviceId: string) => void;
  isVirtual?: boolean;
  monitoringState?: DeviceMonitoringState;
  subtype?: string;
  selfLinks?: Link[];
  onSelfLinkClick?: (link: Link) => void;
  [key: string]: unknown;
}

export type DeviceNode = Node<DeviceNodeData>;

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

function freshnessMeta(freshness: FreshnessStatus): { tone: ReadoutTone; text: string } {
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

function runtimeTelemetryMeta(metrics: DeviceMetricsDTO): { tone: ReadoutTone; text: string } {
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

const compactPercentFormatter = new Intl.NumberFormat('en-US', { maximumFractionDigits: 0 });
const DEVICE_NODE_SCALE_START_ZOOM = 0.9;
const DEVICE_NODE_MIN_SCALE_ZOOM = 0.6;
const DEVICE_NODE_MAX_READABILITY_SCALE = 1.12;

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

function scaledPx(basePx: number, scale: number): string {
  return `${Number((basePx * scale).toFixed(2))}px`;
}

function readableFontStyle(scale: number, basePx: number): CSSProperties | undefined {
  return scale > 1 ? { fontSize: scaledPx(basePx, scale) } : undefined;
}

function readableHeightStyle(scale: number, basePx: number): CSSProperties | undefined {
  return scale > 1 ? { height: scaledPx(basePx, scale) } : undefined;
}

function mergeReadableFontStyle(
  baseStyle: CSSProperties | undefined,
  scale: number,
  basePx: number,
): CSSProperties | undefined {
  const fontStyle = readableFontStyle(scale, basePx);

  if (!baseStyle) {
    return fontStyle;
  }

  return fontStyle ? { ...baseStyle, ...fontStyle } : baseStyle;
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

function PollingDisabledNotice({ className = '' }: { className?: string }) {
  return (
    <div
      className={`rounded-2xl border border-outline-strong bg-surface-container-high px-3 py-2 text-center text-[10px] font-semibold uppercase tracking-[0.14em] text-on-bg-secondary ${className}`}
    >
      Continuous polling disabled
    </div>
  );
}

function DeviceCardInner({ data, selected }: NodeProps<DeviceNode>) {
  const zoom = useStore((state) => state.transform[2]);
  const readabilityScale = resolveDeviceNodeReadabilityScale(zoom);
  const monitoringState = data.monitoringState ?? resolveDeviceMonitoringState(data.device);
  const isPollingDisabled =
    monitoringState === 'monitorable' && data.device.polling_enabled === false;
  const isVirtual = data.isVirtual === true;
  const metrics = sanitizeDeviceMetricsForDisplay(data.device, data.metrics, monitoringState);
  const headerState =
    metrics || isVirtual
      ? resolveDeviceVisualState(data.device, metrics, monitoringState)
      : resolveDeviceOperationalStatusState(data.device, monitoringState);
  const telemetryFallback =
    monitoringState === 'monitorable' && !isVirtual && !metrics
      ? { tone: 'muted' as const, text: 'Unmonitored' }
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
  const operationalReadouts =
    renderModel.variant === 'physical' && metrics
      ? resolveDeviceOperationalReadouts(data.device, metrics, monitoringState)
      : null;
  const isVirtualUnmonitored = renderModel.variant === 'virtual-unmonitored';
  const selfLinks = data.selfLinks ?? [];
  const primarySelfLink = selfLinks[0];
  const statusStyles = resolveDeviceNodeStatusStyles({
    status: headerState.dotStatus,
    selected,
    highlighted: data.highlighted === true,
  });
  const runtimeBadges = metrics?.runtime_flags.map(runtimeBadgeLabel) ?? [];

  if (data.kind === 'ghost-device' || data.isGhost) {
    return (
      <>
        <Handle type="target" position={Position.Top} className={universalHandleClassName} />
        <div
          className="w-[132px] cursor-pointer rounded-2xl border border-dashed border-outline bg-surface/72 px-3 py-2 text-center transition-[border-color,background-color,color] duration-150 hover:bg-surface-container"
          style={ghostFrameStyle(firstColor)}
          onClick={() => data.onGhostClick?.(data.device.id)}
          role="button"
          tabIndex={0}
          onKeyDown={(event) => {
            if (event.key === 'Enter' || event.key === ' ') {
              data.onGhostClick?.(data.device.id);
            }
          }}
        >
          <p className="truncate text-[11px] font-medium uppercase tracking-[0.14em] text-on-bg-secondary">
            cross-area
          </p>
          <p className="mt-1 truncate text-sm font-semibold text-on-bg">
            {data.device.sys_name || data.device.tags?.display_name || data.device.ip || 'Ghost'}
          </p>
        </div>
        <Handle type="source" position={Position.Bottom} className={universalHandleClassName} />
      </>
    );
  }

  return (
    <div
      data-testid="device-node-card"
      className={`group relative w-full rounded-[20px] border border-outline bg-surface transition-[transform,border-color,box-shadow] duration-200 hover:-translate-y-0.5 hover:border-outline-strong ${isVirtual ? 'min-h-[160px] min-w-[200px] max-h-[235px] max-w-[285px]' : 'min-h-[140px]'} ${statusStyles.frameClass ?? ''}`}
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
          className="absolute left-1/2 top-0 z-20 flex max-w-[calc(100%-1rem)] -translate-x-1/2 -translate-y-1/2 items-center gap-2 rounded-full border border-primary/25 bg-surface-container-high/95 px-3 py-1.5 text-left shadow-floating backdrop-blur-sm transition-[border-color,transform] duration-150 hover:-translate-y-[55%] hover:border-primary/45"
          onMouseDown={(event) => {
            event.stopPropagation();
          }}
          onClick={(event) => {
            event.stopPropagation();
            data.onSelfLinkClick?.(primarySelfLink);
          }}
          aria-label={`View details for self link ${formatSelfLinkSummary(primarySelfLink)}`}
        >
          <span className="shrink-0 rounded-full bg-primary/10 px-2 py-0.5 text-[9px] font-semibold uppercase tracking-[0.18em] text-primary">
            {formatSelfLinkLabel(primarySelfLink)}
          </span>
          <span className="min-w-0 truncate font-mono text-[10px] text-on-bg-secondary">
            {formatSelfLinkSummary(primarySelfLink)}
          </span>
          {selfLinks.length > 1 ? (
            <span className="shrink-0 rounded-full border border-outline px-1.5 py-0.5 text-[9px] font-semibold text-on-bg-secondary">
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

      <div className="overflow-hidden rounded-[19px]">
        <div
          className="h-1.5 w-full"
          style={hasArea && areaAccent ? { background: areaAccent } : undefined}
        />

        {renderModel.variant === 'physical' ? (
          <div className="px-4 pb-3.5 pt-3">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0 flex-1">
                <div
                  data-testid="physical-node-hostname"
                  className="min-w-0 text-[15px] font-semibold leading-snug text-on-bg"
                  style={readableFontStyle(readabilityScale, 15)}
                >
                  <span className="line-clamp-2 break-words">{label}</span>
                </div>
              </div>

              <div
                data-testid="physical-node-status-badge"
                className={`inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-semibold ${statusStyles.badgeClass}`}
                style={mergeReadableFontStyle(statusStyles.badgeStyle, readabilityScale, 11)}
              >
                <StatusDot status={headerState.dotStatus} />
                <span>{headerState.label}</span>
              </div>
            </div>

            <div className="mt-3 flex items-center justify-between gap-3">
              {addressState === 'address' ? (
                <span
                  data-testid="physical-node-address"
                  className="min-w-0 truncate rounded-full border border-outline bg-surface-container px-2.5 py-1 font-mono text-[11px] font-medium text-on-bg"
                  style={readableFontStyle(readabilityScale, 11)}
                >
                  {addressLabel} {data.device.ip}
                </span>
              ) : (
                <span
                  data-testid="physical-node-address"
                  className="rounded-full border border-outline bg-surface-container px-2.5 py-1 text-[11px] font-semibold text-on-bg-secondary"
                  style={readableFontStyle(readabilityScale, 11)}
                >
                  No IP
                </span>
              )}

              {renderModel.showFreshnessMeta ? (
                <div className="min-w-0 truncate text-right">
                  <div
                    data-testid="physical-node-freshness"
                    className={`truncate text-[11px] font-semibold ${readoutToneClass(freshness!.tone)}`}
                    style={readableFontStyle(readabilityScale, 11)}
                  >
                    {freshness!.text}
                  </div>
                </div>
              ) : null}
            </div>

            {operationalReadouts ? (
              <div
                className="mt-2 grid h-[40px] grid-cols-3 overflow-hidden rounded-xl border border-outline-subtle bg-surface-container/55"
                data-testid="physical-runtime-readouts"
                style={readableHeightStyle(readabilityScale, 40)}
              >
                <div className="flex min-w-0 flex-col justify-center border-outline-subtle border-r px-2.5">
                  <span
                    className="truncate text-[9px] font-semibold uppercase leading-none tracking-[0.14em] text-on-bg-secondary"
                    style={readableFontStyle(readabilityScale, 9)}
                  >
                    CPU
                  </span>
                  <span
                    className={`mt-1 truncate font-mono text-[12px] font-semibold leading-none ${runtimeMetricValueClass(operationalReadouts.cpuPercent)}`}
                    style={readableFontStyle(readabilityScale, 12)}
                  >
                    {formatRuntimePercent(operationalReadouts.cpuPercent)}
                  </span>
                </div>
                <div className="flex min-w-0 flex-col justify-center border-outline-subtle border-r px-2.5">
                  <span
                    className="truncate text-[9px] font-semibold uppercase leading-none tracking-[0.14em] text-on-bg-secondary"
                    style={readableFontStyle(readabilityScale, 9)}
                  >
                    MEM
                  </span>
                  <span
                    className={`mt-1 truncate font-mono text-[12px] font-semibold leading-none ${runtimeMetricValueClass(operationalReadouts.memPercent)}`}
                    style={readableFontStyle(readabilityScale, 12)}
                  >
                    {formatRuntimePercent(operationalReadouts.memPercent)}
                  </span>
                </div>
                <div className="flex min-w-0 flex-col justify-center px-2.5">
                  <span
                    className="truncate text-[9px] font-semibold uppercase leading-none tracking-[0.14em] text-on-bg-secondary"
                    style={readableFontStyle(readabilityScale, 9)}
                  >
                    Uptime
                  </span>
                  <span
                    className="mt-1 truncate font-mono text-[12px] font-semibold leading-none text-on-bg"
                    style={readableFontStyle(readabilityScale, 12)}
                  >
                    {formatRuntimeUptime(operationalReadouts.uptimeSecs)}
                  </span>
                </div>
              </div>
            ) : null}
          </div>
        ) : (
          <div
            className={`px-3.5 text-center ${isVirtualUnmonitored ? 'pb-4 pt-3.5' : 'pb-3 pt-2.5'}`}
          >
            <div className="flex flex-col items-center">
              {renderModel.showVirtualStatusBadge ? (
                <div className="mb-1.5 flex w-full justify-end">
                  <div
                    className={`inline-flex max-w-full shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1 text-[10px] font-semibold uppercase tracking-[0.14em] ${statusStyles.badgeClass}`}
                    style={mergeReadableFontStyle(statusStyles.badgeStyle, readabilityScale, 10)}
                  >
                    <StatusDot status={headerState.dotStatus} />
                    <span className="truncate">{headerState.label}</span>
                  </div>
                </div>
              ) : null}

              <div className="flex h-[56px] w-[56px] items-center justify-center rounded-[22px] border border-outline bg-surface-container-high text-on-bg shadow-[inset_0_1px_0_rgba(255,255,255,0.85)]">
                <VendorIcon vendor={data.device.vendor} size={20} />
              </div>

              <div
                className="mt-2.5 max-w-full truncate text-[10px] uppercase tracking-[0.14em] text-on-bg-secondary"
                style={readableFontStyle(readabilityScale, 10)}
              >
                {deviceTypeLabel(data.device, isVirtual, data.subtype)}
              </div>
              <div
                className="mt-1.5 w-full max-w-full text-[17px] font-semibold leading-tight tracking-tight text-on-bg"
                style={readableFontStyle(readabilityScale, 17)}
              >
                <span className="block w-full truncate">{label}</span>
              </div>

              <div className="mt-3 flex w-full flex-col items-center gap-1.5">
                {renderModel.showVirtualAddressChip ? (
                  <span
                    className="inline-block max-w-full truncate rounded-full border border-outline bg-surface-container-high px-3 py-1 font-mono text-[11px] text-on-bg"
                    style={readableFontStyle(readabilityScale, 11)}
                  >
                    {addressLabel} {data.device.ip}
                  </span>
                ) : null}
              </div>

              {renderModel.showFreshnessMeta ? (
                <div className="mt-3 flex w-full items-center justify-between gap-2 text-[10px]">
                  <div
                    className={`min-w-0 truncate font-medium ${readoutToneClass(freshness!.tone)}`}
                    style={readableFontStyle(readabilityScale, 10)}
                  >
                    {freshness!.text}
                  </div>
                  <div
                    className="min-w-0 truncate text-on-bg-secondary"
                    style={readableFontStyle(readabilityScale, 10)}
                  >
                    {pollingEvery}
                  </div>
                </div>
              ) : null}

              {isPollingDisabled ? <PollingDisabledNotice className="mt-3 w-full" /> : null}

              {runtimeBadges.length > 0 ? (
                <div className="mt-2 flex w-full flex-wrap justify-center gap-1.5">
                  {runtimeBadges.map((badge) => (
                    <span
                      key={badge}
                      className="rounded-full border border-warning/30 bg-warning/10 px-2 py-0.5 text-[9px] font-semibold uppercase tracking-[0.14em] text-warning"
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

export function getDeviceRenderSignature(props: NodeProps<DeviceNode>) {
  const data = props.data;
  const metrics = data.metrics;

  return {
    deviceId: data.device.id,
    status: data.device.status,
    vendor: data.device.vendor,
    sysName: data.device.sys_name,
    hardwareModel: data.device.hardware_model,
    displayName: data.device.tags?.display_name,
    ip: data.device.ip,
    pollingEnabled: data.device.polling_enabled,
    areaIds: data.device.area_ids ?? [],
    highlighted: data.highlighted,
    alertStatus: data.alertStatus,
    areaColors: data.areaColors ?? [],
    kind: data.kind,
    isGhost: data.isGhost,
    isVirtual: data.isVirtual,
    monitoringState: data.monitoringState,
    subtype: data.subtype,
    selfLinks: data.selfLinks,
    cpuPercent: metrics?.cpu_percent,
    memPercent: metrics?.mem_percent,
    tempCelsius: metrics?.temp_celsius,
    uptimeSecs: metrics?.uptime_secs,
    health: metrics?.health,
    primaryHealth: metrics?.primary_health,
    reachability: metrics?.reachability,
    networkReachable: metrics?.network_reachable,
    snmpReachable: metrics?.snmp_reachable,
    runtimeFlags: metrics?.runtime_flags,
    freshness: metrics?.freshness,
    lastPolledAt: metrics?.last_polled_at,
    expectedPollIntervalSeconds: metrics?.expected_poll_interval_seconds,
    editMode: data.editMode,
    positionAbsoluteX: props.positionAbsoluteX,
    positionAbsoluteY: props.positionAbsoluteY,
    width: props.width,
    height: props.height,
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
    previous.status === next.status &&
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
    previous.kind === next.kind &&
    previous.isGhost === next.isGhost &&
    previous.isVirtual === next.isVirtual &&
    previous.monitoringState === next.monitoringState &&
    previous.subtype === next.subtype &&
    sameSelfLinks(previous.selfLinks, next.selfLinks) &&
    previous.cpuPercent === next.cpuPercent &&
    previous.memPercent === next.memPercent &&
    previous.tempCelsius === next.tempCelsius &&
    previous.uptimeSecs === next.uptimeSecs &&
    previous.health === next.health &&
    previous.primaryHealth === next.primaryHealth &&
    previous.reachability === next.reachability &&
    previous.networkReachable === next.networkReachable &&
    previous.snmpReachable === next.snmpReachable &&
    sameRuntimeFlags(previous.runtimeFlags, next.runtimeFlags) &&
    previous.freshness === next.freshness &&
    previous.lastPolledAt === next.lastPolledAt &&
    previous.expectedPollIntervalSeconds === next.expectedPollIntervalSeconds &&
    previous.editMode === next.editMode &&
    previous.positionAbsoluteX === next.positionAbsoluteX &&
    previous.positionAbsoluteY === next.positionAbsoluteY &&
    previous.width === next.width &&
    previous.height === next.height &&
    previous.selected === next.selected
  );
}

const DeviceCard = memo(
  DeviceCardInner,
  (prev: NodeProps<DeviceNode>, next: NodeProps<DeviceNode>) =>
    sameDeviceRenderSignature(getDeviceRenderSignature(prev), getDeviceRenderSignature(next)),
);

export default DeviceCard;
