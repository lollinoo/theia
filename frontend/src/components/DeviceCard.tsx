import { memo, type CSSProperties } from 'react';
import { Handle, Position, useStore, type Node, type NodeProps } from '@xyflow/react';
import type { Device } from '../types/api';
import {
  formatUptime,
  metricColor,
  type AlertStatus,
  type DeviceMetricsDTO,
} from '../types/metrics';
import { formatFreshness, formatPollingEvery } from '../utils/freshness';
import { isNodeVisibleInViewport } from '../utils/canvasVisibility';
import { useDocumentVisibility } from '../hooks/useDocumentVisibility';
import { useFreshnessClock } from '../hooks/useFreshnessClock';
import { getEffectivePollingIntervalSeconds } from '../utils/polling';
import { StatusDot } from './StatusDot';
import {
  type DeviceMonitoringState,
  resolveDeviceAddressState,
  resolveDeviceMonitoringState,
  resolveDeviceNodeStatusStyles,
  resolveDeviceOperationalReadouts,
  resolveDeviceVisualState,
  sanitizeDeviceMetricsForDisplay,
} from './deviceVisualState';
import { resolveDeviceCardRenderModel } from './deviceCardVariant';
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
  [key: string]: unknown;
}

export type DeviceNode = Node<DeviceNodeData>;

interface Readout {
  label: string;
  value: string;
  tone?: 'default' | 'ok' | 'warning' | 'critical' | 'muted';
}

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

function formatPercent(value: number | null): string {
  return value === null ? '--' : `${Math.round(value)}%`;
}

function deviceTypeLabel(device: Device, isVirtual: boolean, subtype?: string): string {
  if (isVirtual) {
    return subtypeLabels[subtype ?? 'generic'] ?? 'Virtual';
  }
  return deviceTypeLabels[device.device_type] ?? 'Node';
}

function freshnessTone(tier: 'Fresh' | 'Stale' | 'Dead'): Readout['tone'] {
  switch (tier) {
    case 'Fresh':
      return 'ok';
    case 'Stale':
      return 'warning';
    case 'Dead':
      return 'critical';
  }
}

function readoutToneClass(tone: Readout['tone']): string {
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

function buildReadouts({
  cpuPercent,
  memPercent,
  uptimeSecs,
  isDeviceDown,
}: {
  cpuPercent: number | null;
  memPercent: number | null;
  uptimeSecs: number | null;
  isDeviceDown: boolean;
}): Readout[] {
  return [
    {
      label: 'CPU',
      value: formatPercent(cpuPercent),
      tone: isDeviceDown ? 'critical' : cpuPercent === null ? 'muted' : cpuPercent >= 85 ? 'critical' : cpuPercent >= 60 ? 'warning' : 'ok',
    },
    {
      label: 'MEM',
      value: formatPercent(memPercent),
      tone: isDeviceDown ? 'critical' : memPercent === null ? 'muted' : memPercent >= 85 ? 'critical' : memPercent >= 60 ? 'warning' : 'default',
    },
    {
      label: 'UP',
      value: uptimeSecs === null ? '--' : formatUptime(uptimeSecs),
      tone: isDeviceDown ? 'critical' : uptimeSecs === null ? 'muted' : 'default',
    },
  ];
}

function ghostFrameStyle(color?: string): CSSProperties | undefined {
  if (!color) return undefined;
  return {
    borderColor: color,
    color,
  };
}

function categoryBadgeClass(): string {
  return 'border-outline-strong bg-surface-container-high text-on-bg-secondary';
}

function DeviceCardInner({
  data,
  width,
  height,
  positionAbsoluteX,
  positionAbsoluteY,
  selected,
}: NodeProps<DeviceNode>) {
  const monitoringState = data.monitoringState ?? resolveDeviceMonitoringState(data.device);
  const metrics = sanitizeDeviceMetricsForDisplay(data.device, data.metrics);
  const transform = useStore((state) => state.transform);
  const viewportWidth = useStore((state) => state.width);
  const viewportHeight = useStore((state) => state.height);
  const documentVisible = useDocumentVisibility();
  const isVirtual = data.isVirtual === true;
  const fallbackWidth = data.isGhost ? 132 : isVirtual ? 208 : 236;
  const fallbackHeight = data.isGhost ? 52 : 156;
  const freshnessActive = documentVisible && isNodeVisibleInViewport({
    nodeX: positionAbsoluteX,
    nodeY: positionAbsoluteY,
    nodeWidth: width ?? fallbackWidth,
    nodeHeight: height ?? fallbackHeight,
    viewportWidth,
    viewportHeight,
    transform,
  });
  const nowMs = useFreshnessClock(
    metrics?.last_polled_at,
    metrics?.expected_poll_interval_seconds,
    freshnessActive,
  );
  const headerState = resolveDeviceVisualState(data.device, metrics);
  const freshness = monitoringState === 'monitorable' && metrics
    ? formatFreshness(
        metrics.last_polled_at,
        metrics.expected_poll_interval_seconds,
        nowMs,
      )
    : null;
  const pollingEvery = monitoringState === 'monitorable' && metrics
    ? formatPollingEvery(
        metrics.expected_poll_interval_seconds ?? getEffectivePollingIntervalSeconds(data.device),
      )
    : null;
  const label = displayName(data.device);
  const colors = data.areaColors ?? [];
  const hasArea = colors.length > 0;
  const firstColor = colors[0];
  const areaAccent = colors.length >= 2
    ? `linear-gradient(90deg, ${colors.join(', ')})`
    : firstColor;
  const addressLabel = isMacAddress(data.device.ip) ? 'MAC' : 'IP';
  const addressState = resolveDeviceAddressState(data.device);
  const {
    cpuPercent,
    memPercent,
    uptimeSecs,
    isDeviceDown,
  } = resolveDeviceOperationalReadouts(data.device, metrics);
  const renderModel = resolveDeviceCardRenderModel({
    device: data.device,
    monitoringState,
    addressState,
    hasFreshnessMeta: freshness !== null && pollingEvery !== null,
  });
  const readouts = buildReadouts({
    cpuPercent,
    memPercent,
    uptimeSecs,
    isDeviceDown,
  });
  const statusStyles = resolveDeviceNodeStatusStyles({
    status: headerState.dotStatus,
    selected,
    highlighted: data.highlighted === true,
  });

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
      className="group relative w-full rounded-[20px] border border-outline bg-surface transition-[transform,border-color,box-shadow] duration-200 hover:-translate-y-0.5 hover:border-outline-strong"
      style={statusStyles.frameStyle}
      onContextMenu={(event) => {
        if (!data.onContextMenu) return;
        event.preventDefault();
        event.stopPropagation();
        data.onContextMenu(event, data.device.id);
      }}
    >
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
              <div className="min-w-0">
                <div className="flex items-center gap-2 text-[10px] uppercase tracking-[0.18em] text-on-bg-secondary">
                  <VendorIcon vendor={data.device.vendor} size={16} />
                  <span>{deviceTypeLabel(data.device, isVirtual, data.subtype)}</span>
                </div>
                <div className="mt-2 min-w-0 text-[15px] font-semibold leading-tight tracking-tight text-on-bg">
                  <span className="line-clamp-2 break-words">{label}</span>
                </div>
              </div>

              <div
                className={`inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1 text-[10px] font-semibold uppercase tracking-[0.14em] ${statusStyles.badgeClass}`}
                style={statusStyles.badgeStyle}
              >
                <StatusDot status={headerState.dotStatus} />
                <span>{headerState.label}</span>
              </div>
            </div>

            <div className="mt-3 flex items-center justify-between gap-3">
              {addressState === 'address' ? (
                <span className="rounded-full border border-outline bg-surface-container px-2.5 py-1 font-mono text-[11px] text-on-bg">
                  {addressLabel} {data.device.ip}
                </span>
              ) : (
                <span className="rounded-full border border-outline bg-surface-container px-2.5 py-1 text-[10px] font-medium uppercase tracking-[0.14em] text-on-bg-secondary">
                  No IP
                </span>
              )}

              {renderModel.showFreshnessMeta ? (
                <div className="min-w-0 text-right">
                  <div className={`text-[10px] font-medium ${readoutToneClass(freshnessTone(freshness!.tier))}`}>
                    {freshness!.text}
                  </div>
                  <div className="mt-0.5 text-[10px] text-on-bg-secondary">
                    {pollingEvery}
                  </div>
                </div>
              ) : null}
            </div>

            {renderModel.showOperationalReadouts ? (
              <div className="mt-3 grid grid-cols-3 gap-1.5">
                {readouts.map((readout) => (
                  <div key={readout.label} className="rounded-2xl border border-outline bg-surface-container px-2.5 py-2">
                    <div className="text-[9px] uppercase tracking-[0.16em] text-on-bg-secondary">
                      {readout.label}
                    </div>
                    <div className={`mt-1 truncate font-mono text-[11px] font-semibold ${readoutToneClass(readout.tone)}`}>
                      {readout.tone === 'default' && readout.label === 'CPU' && cpuPercent !== null ? (
                        <span className={metricColor(cpuPercent)}>{readout.value}</span>
                      ) : readout.tone === 'default' && readout.label === 'MEM' && memPercent !== null ? (
                        <span className={metricColor(memPercent)}>{readout.value}</span>
                      ) : (
                        readout.value
                      )}
                    </div>
                  </div>
                ))}
              </div>
            ) : null}
          </div>
        ) : (
          <div className="px-4 pb-4 pt-3.5">
            <div className="flex items-start gap-3">
              <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl border border-outline bg-surface-container-high text-on-bg">
                <VendorIcon vendor={data.device.vendor} size={18} />
              </div>

              <div className="min-w-0 flex-1">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="text-[10px] uppercase tracking-[0.18em] text-on-bg-secondary">
                      {deviceTypeLabel(data.device, isVirtual, data.subtype)}
                    </div>
                    <div className="mt-1.5 min-w-0 text-[16px] font-semibold leading-tight tracking-tight text-on-bg">
                      <span className="line-clamp-2 break-words">{label}</span>
                    </div>
                  </div>

                  {renderModel.showVirtualCategoryBadge ? (
                    <div className={`inline-flex shrink-0 rounded-full border px-2.5 py-1 text-[10px] font-semibold uppercase tracking-[0.14em] ${categoryBadgeClass()}`}>
                      <span>Virtual node</span>
                    </div>
                  ) : (
                    <div
                      className={`inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1 text-[10px] font-semibold uppercase tracking-[0.14em] ${statusStyles.badgeClass}`}
                      style={statusStyles.badgeStyle}
                    >
                      <StatusDot status={headerState.dotStatus} />
                      <span>{headerState.label}</span>
                    </div>
                  )}
                </div>

                {renderModel.showVirtualStatusPanel ? (
                  <div
                    className={`mt-4 rounded-2xl border px-3.5 py-3 ${statusStyles.panelClass}`}
                    style={statusStyles.panelStyle}
                  >
                    <div className="text-[9px] uppercase tracking-[0.16em] text-on-bg-secondary">
                      Status
                    </div>
                    <div className="mt-1.5 flex items-center gap-2.5">
                      <StatusDot status={headerState.dotStatus} />
                      <span className={`text-[15px] font-semibold tracking-tight ${headerState.labelClass}`}>
                        {headerState.label}
                      </span>
                    </div>
                  </div>
                ) : null}

                <div className="mt-4 flex flex-wrap items-center gap-2">
                  {renderModel.showVirtualIdentityTag ? (
                    <span className="rounded-full border border-outline bg-surface-container px-3 py-1 text-[10px] font-medium uppercase tracking-[0.14em] text-on-bg-secondary">
                      Virtual node
                    </span>
                  ) : null}

                  {renderModel.showVirtualAddressChip ? (
                    <span className="rounded-full border border-outline bg-surface-container-high px-3 py-1 font-mono text-[11px] text-on-bg">
                      {addressLabel} {data.device.ip}
                    </span>
                  ) : null}
                </div>

                {renderModel.showFreshnessMeta ? (
                  <div className="mt-3 flex items-center justify-between gap-3 text-[10px]">
                    <div className={`font-medium ${readoutToneClass(freshnessTone(freshness!.tier))}`}>
                      {freshness!.text}
                    </div>
                    <div className="text-on-bg-secondary">
                      {pollingEvery}
                    </div>
                  </div>
                ) : null}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

const DeviceCard = memo(DeviceCardInner, (prev: NodeProps<DeviceNode>, next: NodeProps<DeviceNode>) => {
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
    pd.device.area_ids?.length === nd.device.area_ids?.length &&
    pd.highlighted === nd.highlighted &&
    pd.alertStatus === nd.alertStatus &&
    pd.areaColors?.length === nd.areaColors?.length && (pd.areaColors ?? []).every((c, i) => c === nd.areaColors?.[i]) &&
    pd.isGhost === nd.isGhost &&
    pd.isVirtual === nd.isVirtual &&
    pd.monitoringState === nd.monitoringState &&
    pd.subtype === nd.subtype &&
    pd.metrics?.cpu_percent === nd.metrics?.cpu_percent &&
    pd.metrics?.mem_percent === nd.metrics?.mem_percent &&
    pd.metrics?.temp_celsius === nd.metrics?.temp_celsius &&
    pd.metrics?.uptime_secs === nd.metrics?.uptime_secs &&
    pd.metrics?.health === nd.metrics?.health &&
    pd.metrics?.stale === nd.metrics?.stale &&
    pd.metrics?.last_polled_at === nd.metrics?.last_polled_at &&
    pd.metrics?.expected_poll_interval_seconds === nd.metrics?.expected_poll_interval_seconds &&
    pd.editMode === nd.editMode &&
    prev.positionAbsoluteX === next.positionAbsoluteX &&
    prev.positionAbsoluteY === next.positionAbsoluteY &&
    prev.width === next.width &&
    prev.height === next.height &&
    prev.selected === next.selected
  );
});

export default DeviceCard;
