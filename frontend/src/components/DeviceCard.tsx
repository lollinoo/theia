import { memo } from 'react';
import { Handle, Position, type Node, type NodeProps } from '@xyflow/react';
import type { Device } from '../types/api';
import {
  formatUptime,
  metricColor,
  type AlertStatus,
  type DeviceMetricsDTO,
} from '../types/metrics';
import { StatusDot } from './StatusDot';
import { VendorIcon } from './icons/VendorIcon';

export interface DeviceNodeData {
  device: Device;
  pinned: boolean;
  highlighted?: boolean;
  editMode?: boolean;
  metrics?: DeviceMetricsDTO | null;
  alertStatus?: AlertStatus;
  areaColor?: string;
  onContextMenu?: (event: React.MouseEvent, deviceId: string) => void;
  isGhost?: boolean;
  onGhostClick?: (deviceId: string) => void;
  [key: string]: unknown;
}

export type DeviceNode = Node<DeviceNodeData>;

const universalHandleClassName =
  '!h-2 !w-2 !rounded-full !border-2 !border-bg !bg-on-bg-secondary shadow-none';

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

function DeviceCardInner({
  data,
  selected,
}: NodeProps<DeviceNode>) {
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

  const label = displayName(data.device);
  const detail = secondaryText(data.device, label);
  const addressLabel =
    data.device.ip.includes(':') && !data.device.ip.includes('.') ? 'MAC' : 'IP';
  const metrics = data.metrics ?? null;
  const cpuPercent = metrics?.cpu_percent ?? null;
  const memPercent = metrics?.mem_percent ?? null;
  const tempCelsius = metrics?.temp_celsius ?? null;
  const uptimeSecs = metrics?.uptime_secs ?? null;
  const statusForDot =
    data.alertStatus === 'down'
      ? 'down'
      : data.alertStatus === 'degraded'
        ? 'degraded'
        : data.device.status;

  const cardRingClass =
    data.alertStatus === 'down'
      ? 'ring-2 ring-status-down shadow-[0_0_28px_rgba(255,23,68,0.45)] animate-pulse'
      : data.alertStatus === 'degraded'
        ? 'ring-2 ring-yellow-500 shadow-[0_0_28px_rgba(255,193,7,0.35)]'
        : data.highlighted
          ? 'ring-2 ring-primary shadow-[0_0_28px_rgba(0,230,118,0.35)]'
          : selected
            ? 'ring-2 ring-primary shadow-[0_0_22px_rgba(0,230,118,0.18)]'
            : data.areaColor
              ? 'ring-[1.5px]'
              : 'ring-1 ring-outline';

  const cardStyle: React.CSSProperties | undefined = data.areaColor
    ? { '--tw-ring-color': data.areaColor } as React.CSSProperties
    : undefined;

  return (
    <div
      className={`group relative flex w-[260px] flex-col overflow-visible rounded-[12px] bg-surface text-left shadow-canvas transition-[box-shadow,ring,opacity,background-color,color,border-color] duration-200 motion-reduce:animate-none ${cardRingClass} hover:ring-[2.5px] ${data.areaColor ? '' : 'hover:ring-primary/60 hover:shadow-[0_0_20px_rgba(0,230,118,0.15)]'}`}
      style={cardStyle}
      onMouseEnter={(e) => {
        if (data.areaColor) e.currentTarget.style.boxShadow = `0 0 22px ${data.areaColor}50`;
      }}
      onMouseLeave={(e) => {
        if (data.areaColor) e.currentTarget.style.boxShadow = '';
      }}
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
          <span className="break-words text-[15px] font-bold tracking-wide text-on-bg">
            {label}
          </span>
        </div>
        <div className="flex shrink-0 items-center justify-center">
          <StatusDot status={statusForDot} />
        </div>
      </div>

      {/* BODY SECTION */}
      <div
        className={`flex flex-col rounded-b-[12px] bg-bg px-4 pt-3 pb-6 ${data.alertStatus === 'down' ? 'opacity-70' : ''
          }`}
      >
        {detail && (
          <div className="flex items-center gap-2">
            <span className="text-[13px] font-medium text-on-bg-secondary/90">
              {detail}
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
        <div className="mt-3 rounded-lg bg-surface-high px-3 py-2">
          <div className="grid grid-cols-4 gap-2">
            <div className="text-center">
              <div className="text-[10px] uppercase tracking-[0.16em] text-on-bg-secondary/70">
                CPU
              </div>
              <div
                className={`mt-1 font-mono text-[11px] font-semibold ${cpuPercent === null ? 'text-on-bg-secondary' : metricColor(cpuPercent)
                  }`}
              >
                {formatPercent(cpuPercent)}
              </div>
            </div>
            <div className="text-center">
              <div className="text-[10px] uppercase tracking-[0.16em] text-on-bg-secondary/70">
                MEM
              </div>
              <div
                className={`mt-1 font-mono text-[11px] font-semibold ${memPercent === null ? 'text-on-bg-secondary' : metricColor(memPercent)
                  }`}
              >
                {formatPercent(memPercent)}
              </div>
            </div>
            <div className="text-center">
              <div className="text-[10px] uppercase tracking-[0.16em] text-on-bg-secondary/70">
                TEMP
              </div>
              <div className="mt-1 font-mono text-[11px] font-semibold text-on-bg">
                {formatTemperature(tempCelsius)}
              </div>
            </div>
            <div className="text-center">
              <div className="text-[10px] uppercase tracking-[0.16em] text-on-bg-secondary/70">
                UP
              </div>
              <div className="mt-1 font-mono text-[11px] font-semibold text-on-bg whitespace-nowrap">
                {uptimeSecs === null ? '--' : formatUptime(uptimeSecs)}
              </div>
            </div>
          </div>
        </div>
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
    pd.device.tags?.display_name === nd.device.tags?.display_name &&
    pd.highlighted === nd.highlighted &&
    pd.alertStatus === nd.alertStatus &&
    pd.areaColor === nd.areaColor &&
    pd.isGhost === nd.isGhost &&
    pd.metrics?.cpu_percent === nd.metrics?.cpu_percent &&
    pd.metrics?.mem_percent === nd.metrics?.mem_percent &&
    pd.metrics?.temp_celsius === nd.metrics?.temp_celsius &&
    pd.metrics?.uptime_secs === nd.metrics?.uptime_secs &&
    pd.editMode === nd.editMode &&
    prev.selected === next.selected
  );
});

export default DeviceCard;
