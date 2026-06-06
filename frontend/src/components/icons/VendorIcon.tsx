/**
 * Renders vendor icon UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
interface VendorIconProps {
  vendor: string;
  size?: number;
}

function MikroTikIcon({ size }: { size: number }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} fill="none" aria-hidden="true">
      {/* Tower base */}
      <rect x="10.5" y="10" width="3" height="10" rx="1" fill="currentColor" />
      {/* Signal waves */}
      <path
        d="M7.5 9C9 7 10.3 6 12 6C13.7 6 15 7 16.5 9"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
      />
      <path
        d="M5 6.5C7.2 3.8 9.3 2.5 12 2.5C14.7 2.5 16.8 3.8 19 6.5"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
      />
      {/* Base plate */}
      <rect x="7" y="19" width="10" height="2" rx="1" fill="currentColor" />
    </svg>
  );
}

function CiscoIcon({ size }: { size: number }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} fill="none" aria-hidden="true">
      {/* Bridge bars */}
      <rect x="3" y="14" width="2.5" height="5" rx="1" fill="currentColor" />
      <rect x="7" y="10" width="2.5" height="9" rx="1" fill="currentColor" />
      <rect x="10.75" y="7" width="2.5" height="12" rx="1" fill="currentColor" />
      <rect x="14.5" y="10" width="2.5" height="9" rx="1" fill="currentColor" />
      <rect x="18.5" y="14" width="2.5" height="5" rx="1" fill="currentColor" />
      {/* Connecting line */}
      <path d="M4.25 14H20.75" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  );
}

function UbiquitiIcon({ size }: { size: number }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} fill="none" aria-hidden="true">
      {/* Stylized U shape */}
      <path
        d="M7 5V14C7 17.3 9.2 19.5 12 19.5C14.8 19.5 17 17.3 17 14V5"
        stroke="currentColor"
        strokeWidth="2.5"
        strokeLinecap="round"
      />
      {/* Dot */}
      <circle cx="12" cy="14" r="1.8" fill="currentColor" />
    </svg>
  );
}

function GenericIcon({ size }: { size: number }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} fill="none" aria-hidden="true">
      {/* Chip body */}
      <rect x="6" y="6" width="12" height="12" rx="2.5" fill="currentColor" opacity="0.2" />
      <rect x="6" y="6" width="12" height="12" rx="2.5" stroke="currentColor" strokeWidth="1.5" />
      {/* Pins */}
      <path
        d="M9 3.5V6M12 3.5V6M15 3.5V6M9 18V20.5M12 18V20.5M15 18V20.5M3.5 9H6M3.5 12H6M3.5 15H6M18 9H20.5M18 12H20.5M18 15H20.5"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
      />
    </svg>
  );
}

const vendorMap: Record<string, React.FC<{ size: number }>> = {
  mikrotik: MikroTikIcon,
  cisco: CiscoIcon,
  ubiquiti: UbiquitiIcon,
};

/** Renders the VendorIcon component within the UI component boundary. */
export function VendorIcon({ vendor, size = 16 }: VendorIconProps) {
  const Icon = vendorMap[vendor.toLowerCase()] ?? GenericIcon;
  return (
    <span className="inline-flex items-center justify-center text-on-bg-secondary/60">
      <Icon size={size} />
    </span>
  );
}
