import { Handle, Position, type NodeProps } from 'reactflow';
import type { Device } from '../types/api';
import { StatusDot } from './StatusDot';
import { DeviceIcon } from './icons/DeviceIcon';

export interface DeviceNodeData {
  device: Device;
  pinned: boolean;
  highlighted?: boolean;
}

const universalHandleClassName =
  '!h-2 !w-2 !rounded-full !border-2 !border-bg-canvas !bg-[#8899a6] shadow-none';

function displayName(device: Device): string {
  return device.hostname || device.sys_name || device.ip;
}

function secondaryText(device: Device, primaryLabel: string): string {
  if (device.hardware_model && device.hardware_model !== 'Unknown') {
    return device.hardware_model;
  }

  if (device.sys_name && device.sys_name !== primaryLabel) {
    return device.sys_name;
  }

  return device.managed ? 'Managed device' : 'Discovered neighbor';
}

export default function DeviceCard({
  data,
  selected,
}: NodeProps<DeviceNodeData>) {
  const label = displayName(data.device);
  const detail = secondaryText(data.device, label);
  const addressLabel =
    data.device.ip.includes(':') && !data.device.ip.includes('.') ? 'MAC' : 'IP';

  const highlightClass = data.highlighted
    ? 'ring-2 ring-accent shadow-[0_0_28px_rgba(0,212,255,0.35)]'
    : selected
      ? 'ring-2 ring-accent shadow-[0_0_22px_rgba(0,212,255,0.18)]'
      : 'ring-1 ring-border-subtle';

  return (
    <div
      className={`group relative flex w-[260px] flex-col overflow-visible rounded-[12px] bg-bg-surface text-left shadow-canvas transition-colors duration-150 ${highlightClass}`}
    >
      <Handle
        id="top"
        type="source"
        position={Position.Top}
        className={`${universalHandleClassName} !-top-1 !left-1/2 !-translate-x-1/2 opacity-0 transition-opacity duration-200 group-hover:opacity-100 z-10`}
      />
      <Handle
        id="right"
        type="source"
        position={Position.Right}
        className={`${universalHandleClassName} !-right-1 !top-1/2 !-translate-y-1/2 opacity-0 transition-opacity duration-200 group-hover:opacity-100 z-10`}
      />
      <Handle
        id="bottom"
        type="source"
        position={Position.Bottom}
        className={`${universalHandleClassName} !-bottom-1 !left-1/2 !-translate-x-1/2 opacity-0 transition-opacity duration-200 group-hover:opacity-100 z-10`}
      />
      <Handle
        id="left"
        type="source"
        position={Position.Left}
        className={`${universalHandleClassName} !-left-1 !top-1/2 !-translate-y-1/2 opacity-0 transition-opacity duration-200 group-hover:opacity-100 z-10`}
      />

      {/* HEADER SECTION */}
      <div className="flex items-center justify-between rounded-t-[12px] border-t-[3px] border-accent-purple bg-[#1a1a24] px-4 py-3">
        <div className="flex items-center gap-3">
          <div className="flex items-center justify-center text-accent-purple">
            <DeviceIcon type={data.device.device_type} size={20} />
          </div>
          <span className="flex items-center text-[15px] font-bold tracking-wide text-text-primary">
            {label}
          </span>
        </div>
        <div className="flex items-center justify-center">
          {data.pinned && (
            <span
              title="Pinned position"
              className="mr-2 inline-flex h-5 w-5 items-center justify-center rounded-full bg-accent/10 text-accent"
            >
              <svg viewBox="0 0 24 24" className="h-3 w-3" fill="none">
                <path
                  d="M9 4H15L14 9L17.5 12.5V14H12.75L12 20L11.25 14H6.5V12.5L10 9L9 4Z"
                  fill="currentColor"
                />
              </svg>
            </span>
          )}
          <StatusDot status={data.device.status} />
        </div>
      </div>

      {/* BODY SECTION */}
      <div className="flex flex-col rounded-b-[12px] bg-[#12121a] px-4 pt-3 pb-6">
        <span className="text-[13px] font-medium text-text-secondary/90">
          {detail}
        </span>
        <div className="mt-4 flex items-center justify-between">
          <span className="text-[13px] font-bold text-text-secondary/70">
            {addressLabel}:
          </span>
          <span className="font-mono text-[14px] font-bold text-text-primary">
            {data.device.ip}
          </span>
        </div>
      </div>

      {/* DECORATIVE BOTTOM PORTS */}
      <div className="absolute -bottom-1 left-0 flex w-full justify-around px-8 pointer-events-none">
        {[...Array(6)].map((_, i) => (
          <div
            key={i}
            className="h-2 w-2 rounded-full border-2 border-bg-canvas bg-[#8899a6]/50"
          />
        ))}
      </div>
    </div>
  );
}
