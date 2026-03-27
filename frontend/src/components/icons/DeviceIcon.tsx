import type { DeviceType } from '../../types/api';

interface DeviceIconProps {
  type: DeviceType;
  size?: number;
}

function RouterIcon({ size }: { size: number }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} fill="none" aria-hidden="true">
      <rect x="3" y="9" width="18" height="8" rx="3" fill="currentColor" />
      <path d="M8 7L10 4M16 7L14 4" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <circle cx="9" cy="13" r="1.2" fill="var(--nt-bg)" />
      <circle cx="12" cy="13" r="1.2" fill="var(--nt-bg)" />
      <circle cx="15" cy="13" r="1.2" fill="var(--nt-bg)" />
    </svg>
  );
}

function SwitchIcon({ size }: { size: number }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} fill="none" aria-hidden="true">
      <rect x="2.5" y="6" width="19" height="12" rx="3" fill="currentColor" />
      <rect x="5" y="9" width="3" height="2.2" rx="0.7" fill="var(--nt-bg)" />
      <rect x="9.5" y="9" width="3" height="2.2" rx="0.7" fill="var(--nt-bg)" />
      <rect x="14" y="9" width="3" height="2.2" rx="0.7" fill="var(--nt-bg)" />
      <path d="M5 14H19" stroke="var(--nt-bg)" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  );
}

function ApIcon({ size }: { size: number }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} fill="none" aria-hidden="true">
      <circle cx="12" cy="12" r="3.2" fill="currentColor" />
      <path d="M7.5 9.5C9 8 10.3 7.4 12 7.4C13.7 7.4 15 8 16.5 9.5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <path d="M5 7C7 4.9 9.1 4 12 4C14.9 4 17 4.9 19 7" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <path d="M9.1 14.5C10 15.2 10.8 15.6 12 15.6C13.2 15.6 14 15.2 14.9 14.5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  );
}

function UnknownIcon({ size }: { size: number }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} fill="none" aria-hidden="true">
      <circle cx="12" cy="12" r="9" fill="currentColor" opacity="0.15" />
      <path
        d="M9.8 9.2C9.8 7.7 11 6.8 12.5 6.8C14 6.8 15.2 7.7 15.2 9C15.2 10.2 14.5 10.9 13.4 11.5C12.4 12.1 12 12.6 12 13.8"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
      />
      <circle cx="12" cy="16.8" r="1.2" fill="currentColor" />
    </svg>
  );
}

export function DeviceIcon({ type, size = 24 }: DeviceIconProps) {
  return (
    <span className="inline-flex items-center justify-center text-tertiary">
      {type === 'router' && <RouterIcon size={size} />}
      {type === 'switch' && <SwitchIcon size={size} />}
      {type === 'ap' && <ApIcon size={size} />}
      {type === 'unknown' && <UnknownIcon size={size} />}
    </span>
  );
}
