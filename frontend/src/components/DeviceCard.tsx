import { Handle, type Node, type NodeProps, Position } from '@xyflow/react';
import { type CSSProperties, memo } from 'react';
import type { Device, Link } from '../types/api';
import {
  type AlertStatus,
  type DeviceMetricsDTO,
  type FreshnessStatus,
  type RuntimeFlag,
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
  resolveDeviceOperationalStatusState,
  resolveDeviceVisualState,
  sanitizeDeviceMetricsForDisplay,
} from './deviceVisualState';
import { VendorIcon } from './icons/VendorIcon';

export interface DeviceNodeData {
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
    monitoringState === 'monitorable' && metrics ? runtimeTelemetryMeta(metrics) : telemetryFallback;
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
  const isVirtualUnmonitored = renderModel.variant === 'virtual-unmonitored';
  const selfLinks = data.selfLinks ?? [];
  const primarySelfLink = selfLinks[0];
  const statusStyles = resolveDeviceNodeStatusStyles({
    status: headerState.dotStatus,
    selected,
    highlighted: data.highlighted === true,
  });
  const runtimeBadges = metrics?.runtime_flags.map(runtimeBadgeLabel) ?? [];

  if (data.isGhost) {
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
      className={`group relative w-full rounded-[20px] border border-outline bg-surface transition-[transform,border-color,box-shadow] duration-200 hover:-translate-y-0.5 hover:border-outline-strong ${isVirtual ? 'min-h-[160px] min-w-[200px] max-h-[235px] max-w-[285px]' : ''} ${statusStyles.frameClass ?? ''}`}
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
                <div className="min-w-0 text-[15px] font-semibold leading-snug text-on-bg">
                  <span className="line-clamp-2 break-words">{label}</span>
                </div>
              </div>

              <div
                className={`inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-semibold ${statusStyles.badgeClass}`}
                style={statusStyles.badgeStyle}
              >
                <StatusDot status={headerState.dotStatus} />
                <span>{headerState.label}</span>
              </div>
            </div>

            <div className="mt-3 flex items-center justify-between gap-3">
              {addressState === 'address' ? (
                <span className="min-w-0 truncate rounded-full border border-outline bg-surface-container px-2.5 py-1 font-mono text-[11px] font-medium text-on-bg">
                  {addressLabel} {data.device.ip}
                </span>
              ) : (
                <span className="rounded-full border border-outline bg-surface-container px-2.5 py-1 text-[11px] font-semibold text-on-bg-secondary">
                  No IP
                </span>
              )}

              {renderModel.showFreshnessMeta ? (
                <div className="min-w-0 truncate text-right">
                  <div
                    className={`truncate text-[11px] font-semibold ${readoutToneClass(freshness!.tone)}`}
                  >
                    {freshness!.text}
                  </div>
                </div>
              ) : null}
            </div>
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
                    style={statusStyles.badgeStyle}
                  >
                    <StatusDot status={headerState.dotStatus} />
                    <span className="truncate">{headerState.label}</span>
                  </div>
                </div>
              ) : null}

              <div className="flex h-[56px] w-[56px] items-center justify-center rounded-[22px] border border-outline bg-surface-container-high text-on-bg shadow-[inset_0_1px_0_rgba(255,255,255,0.85)]">
                <VendorIcon vendor={data.device.vendor} size={20} />
              </div>

              <div className="mt-2.5 max-w-full truncate text-[10px] uppercase tracking-[0.14em] text-on-bg-secondary">
                {deviceTypeLabel(data.device, isVirtual, data.subtype)}
              </div>
              <div className="mt-1.5 w-full max-w-full text-[17px] font-semibold leading-tight tracking-tight text-on-bg">
                <span className="block w-full truncate">{label}</span>
              </div>

              <div className="mt-3 flex w-full flex-col items-center gap-1.5">
                {renderModel.showVirtualAddressChip ? (
                  <span className="inline-block max-w-full truncate rounded-full border border-outline bg-surface-container-high px-3 py-1 font-mono text-[11px] text-on-bg">
                    {addressLabel} {data.device.ip}
                  </span>
                ) : null}
              </div>

              {renderModel.showFreshnessMeta ? (
                <div className="mt-3 flex w-full items-center justify-between gap-2 text-[10px]">
                  <div
                    className={`min-w-0 truncate font-medium ${readoutToneClass(freshness!.tone)}`}
                  >
                    {freshness!.text}
                  </div>
                  <div className="min-w-0 truncate text-on-bg-secondary">{pollingEvery}</div>
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

const DeviceCard = memo(
  DeviceCardInner,
  (prev: NodeProps<DeviceNode>, next: NodeProps<DeviceNode>) => {
    const pd = prev.data;
    const nd = next.data;
    return (
      pd.device.id === nd.device.id &&
      pd.device.status === nd.device.status &&
      pd.device.vendor === nd.device.vendor &&
      pd.device.sys_name === nd.device.sys_name &&
      pd.device.hardware_model === nd.device.hardware_model &&
      pd.device.tags?.display_name === nd.device.tags?.display_name &&
      pd.device.ip === nd.device.ip &&
      pd.device.polling_enabled === nd.device.polling_enabled &&
      pd.device.area_ids?.length === nd.device.area_ids?.length &&
      pd.highlighted === nd.highlighted &&
      pd.alertStatus === nd.alertStatus &&
      pd.areaColors?.length === nd.areaColors?.length &&
      (pd.areaColors ?? []).every((c, i) => c === nd.areaColors?.[i]) &&
      pd.isGhost === nd.isGhost &&
      pd.isVirtual === nd.isVirtual &&
      pd.monitoringState === nd.monitoringState &&
      pd.subtype === nd.subtype &&
      sameSelfLinks(pd.selfLinks, nd.selfLinks) &&
      pd.metrics?.cpu_percent === nd.metrics?.cpu_percent &&
      pd.metrics?.mem_percent === nd.metrics?.mem_percent &&
      pd.metrics?.temp_celsius === nd.metrics?.temp_celsius &&
      pd.metrics?.uptime_secs === nd.metrics?.uptime_secs &&
      pd.metrics?.health === nd.metrics?.health &&
      pd.metrics?.primary_health === nd.metrics?.primary_health &&
      pd.metrics?.reachability === nd.metrics?.reachability &&
      pd.metrics?.network_reachable === nd.metrics?.network_reachable &&
      pd.metrics?.snmp_reachable === nd.metrics?.snmp_reachable &&
      sameRuntimeFlags(pd.metrics?.runtime_flags, nd.metrics?.runtime_flags) &&
      pd.metrics?.freshness === nd.metrics?.freshness &&
      pd.metrics?.last_polled_at === nd.metrics?.last_polled_at &&
      pd.metrics?.expected_poll_interval_seconds === nd.metrics?.expected_poll_interval_seconds &&
      pd.editMode === nd.editMode &&
      prev.positionAbsoluteX === next.positionAbsoluteX &&
      prev.positionAbsoluteY === next.positionAbsoluteY &&
      prev.width === next.width &&
      prev.height === next.height &&
      prev.selected === next.selected
    );
  },
);

export default DeviceCard;
