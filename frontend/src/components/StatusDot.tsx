import type { DeviceStatus } from '../types/api';

type StatusDotStatus = DeviceStatus | 'degraded';

interface StatusDotProps {
  status: StatusDotStatus;
}

const statusClassNames: Record<StatusDotStatus, string> = {
  up: 'bg-status-up shadow-[0_0_8px_rgba(0,230,118,var(--nt-glow-shadow-opacity))]',
  down: 'bg-status-down shadow-[0_0_16px_rgba(255,23,68,var(--nt-glow-shadow-opacity))] animate-pulse',
  degraded: 'bg-yellow-500 shadow-[0_0_14px_rgba(255,193,7,var(--nt-glow-shadow-opacity))] animate-pulse',
  probing: 'bg-status-probing shadow-[0_0_12px_rgba(255,234,0,var(--nt-glow-shadow-opacity))] animate-pulse',
  unknown: 'bg-status-unknown shadow-[0_0_6px_rgba(158,158,158,var(--nt-glow-shadow-opacity))]',
};

export function StatusDot({ status }: StatusDotProps) {
  return <span className={`motion-reduce:animate-none inline-flex h-2.5 w-2.5 rounded-full transition-[box-shadow] duration-200 ${statusClassNames[status]}`} />;
}
