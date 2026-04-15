import { memo } from 'react';
import { Handle, Position, useStore, type Node, type NodeProps } from '@xyflow/react';
import type { Device } from '../types/api';
import {
  formatUptime,
  metricColor,
  type AlertStatus,
  type DeviceMetricsDTO,
} from '../types/metrics';
import {
  formatFreshness,
  formatPollingEvery,
} from '../utils/freshness';
import { isNodeVisibleInViewport } from '../utils/canvasVisibility';
import { useDocumentVisibility } from '../hooks/useDocumentVisibility';
import { useFreshnessClock } from '../hooks/useFreshnessClock';
import { getEffectivePollingIntervalSeconds } from '../utils/polling';
import { MaterialIcon } from './MaterialIcon';
import { StatusDot } from './StatusDot';
import { resolveDeviceVisualState } from './deviceVisualState';
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
  subtype?: string;
  [key: string]: unknown;
}

export type DeviceNode = Node<DeviceNodeData>;

const universalHandleClassName =
  '!h-2 !w-2 !rounded-full !border-2 !border-bg !bg-on-bg-secondary shadow-none';

const subtypeIconMap: Record<string, string> = {
  internet: 'language',
  cloud: 'cloud',
  server: 'dns',
  generic: 'hub',
};

const macAddressPattern = /^([0-9A-Fa-f]{2}([:-])){5}[0-9A-Fa-f]{2}$/;
const dottedMacAddressPattern = /^([0-9A-Fa-f]{4}\.){2}[0-9A-Fa-f]{4}$/;

function displayName(device: Device): string {
  return device.tags?.display_name || device.sys_name || device.ip;
}

function secondaryText(device: Device, primaryLabel: string): string | null {
  if (device.sys_name && device.sys_name !== primaryLabel) {
    return device.sys_name;
  }
  if (device.hardware_model && device.hardware_model !== 'Unknown') {
    return device.hardware_model;
  }
  if (device.sys_descr) {
    const desc = device.sys_descr.trim();
    return desc.length > 35 ? `${desc.slice(0, 34)}\u2026` : desc;
  }
  return null;
}

function formatPercent(value: number | null): string {
  return value === null ? '--%' : `${Math.round(value)}%`;
}

function formatTemperature(value: number | null): string {
  return value === null ? 'N/A' : `${Math.round(value)}C`;
}

function isMacAddress(value: string): boolean {
  return macAddressPattern.test(value) || dottedMacAddressPattern.test(value);
}

function freshnessClassName(tier: 'Fresh' | 'Stale' | 'Dead'): string {
  switch (tier) {
    case 'Fresh':
      return 'bg-surface-high text-on-bg-secondary';
    case 'Stale':
      return 'bg-warning/10 text-warning';
    case 'Dead':
      return 'bg-critical/10 text-critical';
  }
}

function DeviceCardInner({
  data,
  width,
  height,
  positionAbsoluteX,
  positionAbsoluteY,
  selected,
}: NodeProps<DeviceNode>) {
  const metrics = data.metrics ?? null;
  const transform = useStore((state) => state.transform);
  const viewportWidth = useStore((state) => state.width);
  const viewportHeight = useStore((state) => state.height);
  const documentVisible = useDocumentVisibility();
  const fallbackWidth = data.isGhost ? 120 : data.isVirtual ? (data.device.ip ? 200 : 160) : 260;
  const fallbackHeight = data.isGhost ? 44 : data.isVirtual ? (data.device.ip ? 96 : 64) : 168;
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
  const freshness = metrics
    ? formatFreshness(
        metrics.last_polled_at,
        metrics.expected_poll_interval_seconds,
        nowMs,
      )
    : null;
  const pollingEvery = metrics
    ? formatPollingEvery(
        metrics.expected_poll_interval_seconds ?? getEffectivePollingIntervalSeconds(data.device),
      )
    : null;

  // Ghost node: small muted card with hostname only, dashed border
  if (data.isGhost) {
    return (
      <>
        <Handle type="target" position={Position.Top} className={universalHandleClassName} />
        <div
          className="w-[120px] rounded-xl border border-dashed border-outline-subtle
                     bg-surface/40 px-3 py-2 text-center cursor-pointer
                     hover:border-outline hover:bg-surface/60 transition-colors"
          onClick={() => data.onGhostClick?.(data.device.id)}
          role="button"
          tabIndex={0}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              data.onGhostClick?.(data.device.id);
            }
          }}
        >
          <p className="text-xs text-on-bg-muted truncate font-sans">
            {data.device.sys_name || data.device.ip}
          </p>
        </div>
        <Handle type="source" position={Position.Bottom} className={universalHandleClassName} />
      </>
    );
  }

  // Virtual node: compact card with subtype icon, dashed border, centered layout
  if (data.isVirtual) {
    const hasIP = !!data.device.ip;
    const subtypeIcon = subtypeIconMap[data.subtype ?? ''] ?? 'hub';
    const virtualLabel = data.device.tags?.display_name || data.device.sys_name || data.device.ip || 'Virtual';

    const colors = data.areaColors ?? [];
    const hasArea = colors.length > 0;
    const firstColor = colors[0];
    const isCriticalHealth = data.device.status === 'up' && metrics?.health === 'critical';
    const isWarningHealth = data.device.status === 'up' && metrics?.health === 'warning';
    const isProbing = data.device.status === 'probing';

    const conicGradient = colors.length >= 2
      ? `conic-gradient(${colors.map((c, i, arr) =>
          `${c} ${(i * 360) / arr.length}deg ${((i + 1) * 360) / arr.length}deg`
        ).join(', ')})`
      : undefined;

    const wrapperBg: string =
      data.highlighted || selected
        ? 'var(--color-primary)'
        : conicGradient ?? (hasArea ? firstColor : 'var(--color-outline)');

    const wrapperPadding = data.highlighted || selected || isCriticalHealth || isWarningHealth ? '2px' : '1.5px';

    const wrapperStatusClass =
      isCriticalHealth ? 'shadow-[0_0_28px_rgba(255,23,68,0.45)] animate-pulse'
        : isProbing ? 'shadow-[0_0_24px_rgba(255,234,0,0.28)]'
        : isWarningHealth ? 'shadow-[0_0_28px_rgba(255,193,7,0.35)]'
          : data.highlighted ? 'shadow-[0_0_28px_rgba(0,230,118,0.35)]'
            : selected ? 'shadow-[0_0_22px_rgba(0,230,118,0.18)]' : '';

    const hoverGlowColor = hasArea ? `${firstColor}50` : undefined;

    const virtualCard = (
      <div
        className={`group relative flex ${hasIP ? 'w-[200px]' : 'w-[160px]'} flex-col overflow-visible rounded-[12px] border border-dashed border-outline-subtle bg-surface text-center shadow-canvas transition-[box-shadow,opacity,background-color,color,border-color] duration-200 motion-reduce:animate-none`}
        onContextMenu={(e) => {
          if (data.onContextMenu) {
            e.preventDefault();
            e.stopPropagation();
            data.onContextMenu(e, data.device.id);
          }
        }}
      >
        <Handle id="top" type="source" position={Position.Top}
          isConnectable={!!data.editMode}
          style={{ pointerEvents: data.editMode ? 'auto' : 'none' }}
          className={`${universalHandleClassName} !-top-1 !left-1/2 !-translate-x-1/2 opacity-0 transition-opacity duration-200 group-hover:opacity-100 z-10`}
        />
        <Handle id="right" type="source" position={Position.Right}
          isConnectable={!!data.editMode}
          style={{ pointerEvents: data.editMode ? 'auto' : 'none' }}
          className={`${universalHandleClassName} !-right-1 !top-1/2 !-translate-y-1/2 opacity-0 transition-opacity duration-200 group-hover:opacity-100 z-10`}
        />
        <Handle id="bottom" type="source" position={Position.Bottom}
          isConnectable={!!data.editMode}
          style={{ pointerEvents: data.editMode ? 'auto' : 'none' }}
          className={`${universalHandleClassName} !-bottom-1 !left-1/2 !-translate-x-1/2 opacity-0 transition-opacity duration-200 group-hover:opacity-100 z-10`}
        />
        <Handle id="left" type="source" position={Position.Left}
          isConnectable={!!data.editMode}
          style={{ pointerEvents: data.editMode ? 'auto' : 'none' }}
          className={`${universalHandleClassName} !-left-1 !top-1/2 !-translate-y-1/2 opacity-0 transition-opacity duration-200 group-hover:opacity-100 z-10`}
        />

        {/* HEADER SECTION -- centered vertical layout per D-04 */}
        <div className="flex flex-col items-center px-3 py-2">
          <MaterialIcon name={subtypeIcon} size={24} className="text-on-bg-secondary" />
          <div className="mt-1 flex items-center gap-1.5 max-w-full">
            <span className="font-mono text-[13px] font-semibold text-on-bg truncate">
              {virtualLabel}
            </span>
            {hasIP && <StatusDot status={headerState.dotStatus} />}
            {hasIP && <span className={headerState.labelClass}>{headerState.label}</span>}
          </div>
          {hasIP && metrics && freshness && pollingEvery && (
            <div className="mt-2 flex w-full items-center justify-between gap-2 text-[12px]">
              <span className={`rounded-full px-2 py-1 font-semibold ${freshnessClassName(freshness.tier)}`}>
                {freshness.text}
              </span>
              <span className="font-mono text-on-bg-secondary">
                {pollingEvery}
              </span>
            </div>
          )}
        </div>

        {/* BODY SECTION -- IP-bearing only per D-07 */}
        {hasIP && (
          <div className="rounded-b-[12px] bg-bg px-3 py-2">
            <div className="flex items-center justify-between">
              <span className="text-[11px] font-bold text-on-bg-secondary/70">IP:</span>
              <span className="font-mono text-[14px] font-bold text-on-bg">{data.device.ip}</span>
            </div>
          </div>
        )}
      </div>
    );

    return (
      <div
        className={`rounded-[13.5px] transition-[box-shadow,padding] duration-200 ${wrapperStatusClass} ${hasArea ? '' : 'hover:shadow-[0_0_20px_rgba(0,230,118,0.15)]'}`}
        style={{ background: wrapperBg, padding: wrapperPadding }}
        onMouseEnter={(e) => {
          if (hoverGlowColor) e.currentTarget.style.boxShadow = `0 0 22px ${hoverGlowColor}`;
        }}
        onMouseLeave={(e) => {
          if (hoverGlowColor) e.currentTarget.style.boxShadow = '';
        }}
      >
        {virtualCard}
      </div>
    );
  }

  const label = displayName(data.device);
  const detail = secondaryText(data.device, label);
  const addressLabel = isMacAddress(data.device.ip) ? 'MAC' : 'IP';
  const cpuPercent = metrics?.cpu_percent ?? null;
  const memPercent = metrics?.mem_percent ?? null;
  const tempCelsius = metrics?.temp_celsius ?? null;
  const uptimeSecs = metrics?.uptime_secs ?? null;

  const isDeviceDown = data.device.status === 'down';
  const displayCpuPercent = isDeviceDown ? null : cpuPercent;
  const displayMemPercent = isDeviceDown ? null : memPercent;
  const displayTempCelsius = isDeviceDown ? null : tempCelsius;
  const displayUptimeSecs = isDeviceDown ? null : uptimeSecs;

  const colors = data.areaColors ?? [];
  const hasArea = colors.length > 0;
  const firstColor = colors[0];

  // Unified wrapper border: background determines border color(s)
  const isCriticalHealth = data.device.status === 'up' && metrics?.health === 'critical';
  const isWarningHealth = data.device.status === 'up' && metrics?.health === 'warning';
  const isProbing = data.device.status === 'probing';

  const conicGradient = colors.length >= 2
    ? `conic-gradient(${colors.map((c, i, arr) =>
        `${c} ${(i * 360) / arr.length}deg ${((i + 1) * 360) / arr.length}deg`
      ).join(', ')})`
    : undefined;

  const wrapperBg: string =
    data.highlighted || selected
      ? 'var(--color-primary)'
      : conicGradient ?? (hasArea ? firstColor : 'var(--color-outline)');

  const wrapperPadding = data.highlighted || selected || isCriticalHealth || isWarningHealth ? '2px' : '1.5px';

  const wrapperStatusClass =
    isCriticalHealth
      ? 'shadow-[0_0_28px_rgba(255,23,68,0.45)] animate-pulse'
      : isProbing
        ? 'shadow-[0_0_24px_rgba(255,234,0,0.28)]'
      : isWarningHealth
        ? 'shadow-[0_0_28px_rgba(255,193,7,0.35)]'
        : data.highlighted
          ? 'shadow-[0_0_28px_rgba(0,230,118,0.35)]'
          : selected
            ? 'shadow-[0_0_22px_rgba(0,230,118,0.18)]'
            : '';

  const hoverGlowColor = hasArea ? `${firstColor}50` : undefined;

  const cardElement = (
    <div
      className="group relative flex w-[260px] flex-col overflow-visible rounded-[12px] bg-surface text-left shadow-canvas transition-[box-shadow,opacity,background-color,color,border-color] duration-200 motion-reduce:animate-none"
      onContextMenu={(e) => {
        if (data.onContextMenu) {
          e.preventDefault();
          e.stopPropagation();
          data.onContextMenu(e, data.device.id);
        }
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

      {/* HEADER SECTION */}
      <div className="flex items-center justify-between gap-2 rounded-t-[12px] bg-surface px-4 py-3">
        <div className="flex min-w-0 items-center gap-2.5">
          <div className="flex shrink-0 items-center justify-center text-on-bg-secondary">
            <VendorIcon vendor={data.device.vendor} size={20} />
          </div>
          <span className="min-w-0 line-clamp-2 break-words text-[15px] font-bold tracking-wide text-on-bg">
            {label}
          </span>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <StatusDot status={headerState.dotStatus} />
          <span className={headerState.labelClass}>{headerState.label}</span>
        </div>
      </div>

      {/* BODY SECTION */}
      <div
        className={`flex flex-col rounded-b-[12px] bg-bg px-4 pt-3 pb-6 ${isDeviceDown ? 'opacity-70' : ''}`}
      >
        {detail && (
          <div className="flex items-center gap-2">
            <span className="text-[13px] font-medium text-on-bg-secondary/90">
              {detail}
            </span>
          </div>
        )}
        {metrics && freshness && pollingEvery && (
          <div className="mt-3 flex items-center justify-between gap-2 text-[12px]">
            <span className={`rounded-full px-2 py-1 font-semibold ${freshnessClassName(freshness.tier)}`}>
              {freshness.text}
            </span>
            <span className="font-mono text-on-bg-secondary">
              {pollingEvery}
            </span>
          </div>
        )}
        <div className={`${detail ? 'mt-3' : 'mt-1'} flex items-center justify-between`}>
          <span className="text-[13px] font-bold text-on-bg-secondary/70">
            {addressLabel}:
          </span>
          <span className="font-mono text-[14px] font-bold text-on-bg">
            {data.device.ip}
          </span>
        </div>
        <div className={`mt-3 rounded-lg px-3 py-2 ${isDeviceDown ? 'bg-status-down/10' : 'bg-surface-high'}`}>
          <div className="grid grid-cols-4 gap-2">
            <div className="text-center">
              <div className="text-[10px] uppercase tracking-[0.16em] text-on-bg-secondary/70">
                CPU
              </div>
              <div
                className={`mt-1 font-mono text-[11px] font-semibold ${isDeviceDown ? 'text-status-down/70' : displayCpuPercent === null ? 'text-on-bg-secondary' : metricColor(displayCpuPercent)}`}
              >
                {formatPercent(displayCpuPercent)}
              </div>
            </div>
            <div className="text-center">
              <div className="text-[10px] uppercase tracking-[0.16em] text-on-bg-secondary/70">
                MEM
              </div>
              <div
                className={`mt-1 font-mono text-[11px] font-semibold ${isDeviceDown ? 'text-status-down/70' : displayMemPercent === null ? 'text-on-bg-secondary' : metricColor(displayMemPercent)}`}
              >
                {formatPercent(displayMemPercent)}
              </div>
            </div>
            <div className="text-center">
              <div className="text-[10px] uppercase tracking-[0.16em] text-on-bg-secondary/70">
                TEMP
              </div>
              <div className={`mt-1 font-mono text-[11px] font-semibold ${isDeviceDown ? 'text-status-down/70' : 'text-on-bg'}`}>
                {formatTemperature(displayTempCelsius)}
              </div>
            </div>
            <div className="text-center">
              <div className="text-[10px] uppercase tracking-[0.16em] text-on-bg-secondary/70">
                UP
              </div>
              <div className={`mt-1 font-mono text-[11px] font-semibold whitespace-nowrap ${isDeviceDown ? 'text-status-down/70' : 'text-on-bg'}`}>
                {displayUptimeSecs === null ? '--' : formatUptime(displayUptimeSecs)}
              </div>
            </div>
          </div>
        </div>
      </div>

    </div>
  );

  return (
    <div
      className={`rounded-[13.5px] transition-[box-shadow,padding] duration-200 ${wrapperStatusClass} ${hasArea ? '' : 'hover:shadow-[0_0_20px_rgba(0,230,118,0.15)]'}`}
      style={{ background: wrapperBg, padding: wrapperPadding }}
      onMouseEnter={(e) => {
        if (hoverGlowColor) e.currentTarget.style.boxShadow = `0 0 22px ${hoverGlowColor}`;
      }}
      onMouseLeave={(e) => {
        if (hoverGlowColor) e.currentTarget.style.boxShadow = '';
      }}
    >
      {cardElement}
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
    pd.highlighted === nd.highlighted &&
    pd.alertStatus === nd.alertStatus &&
    pd.areaColors?.length === nd.areaColors?.length && (pd.areaColors ?? []).every((c, i) => c === nd.areaColors?.[i]) &&
    pd.isGhost === nd.isGhost &&
    pd.isVirtual === nd.isVirtual &&
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
