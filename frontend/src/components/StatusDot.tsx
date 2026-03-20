import type { DeviceStatus } from '../types/api';

type StatusDotStatus = DeviceStatus | 'degraded';

interface StatusDotProps {
  status: StatusDotStatus;
}

const statusClassNames: Record<StatusDotStatus, string> = {
  up: 'bg-status-up shadow-[0_0_14px_rgba(0,200,83,0.55)]',
  down: 'bg-status-down shadow-[0_0_14px_rgba(255,23,68,0.45)]',
  degraded: 'bg-yellow-500 animate-pulse shadow-[0_0_14px_rgba(255,193,7,0.45)]',
  probing: 'bg-status-probing animate-pulse shadow-[0_0_14px_rgba(255,193,7,0.45)]',
  unknown: 'bg-status-unknown shadow-[0_0_12px_rgba(101,119,134,0.35)]',
};

export function StatusDot({ status }: StatusDotProps) {
  return <span className={`inline-flex h-2.5 w-2.5 rounded-full ${statusClassNames[status]}`} />;
}
