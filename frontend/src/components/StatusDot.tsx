import type { DeviceStatus } from '../types/api';
import type { CSSProperties } from 'react';

type StatusDotStatus = DeviceStatus | 'degraded' | 'critical';

interface StatusDotProps {
  status: StatusDotStatus;
}

const statusClassNames: Record<StatusDotStatus, string> = {
  up: 'bg-status-up',
  critical: 'bg-status-critical motion-reduce:animate-none animate-pulse',
  down: 'bg-status-down motion-reduce:animate-none animate-pulse',
  degraded: 'bg-warning motion-reduce:animate-none animate-pulse',
  probing: 'bg-status-probing motion-reduce:animate-none animate-pulse',
  unknown: 'bg-status-unknown',
};

const statusStyles: Record<StatusDotStatus, CSSProperties> = {
  up: { boxShadow: 'var(--nt-glow-status-ok)' },
  critical: { boxShadow: 'var(--nt-glow-status-critical)' },
  down: { boxShadow: 'var(--nt-glow-status-down)' },
  degraded: { boxShadow: 'var(--nt-glow-status-warning)' },
  probing: { boxShadow: 'var(--nt-glow-status-warning)' },
  unknown: { boxShadow: 'var(--nt-glow-status-unknown)' },
};

export function StatusDot({ status }: StatusDotProps) {
  return (
    <span
      className={`inline-flex h-2.5 w-2.5 rounded-full transition-[box-shadow] transition-transform duration-200 ${statusClassNames[status]}`}
      style={statusStyles[status]}
    />
  );
}
