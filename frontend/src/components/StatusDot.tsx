import { type DeviceVisualStatus, resolveDeviceStatusDotStyles } from './deviceVisualState';

interface StatusDotProps {
  status: DeviceVisualStatus;
}

export function StatusDot({ status }: StatusDotProps) {
  const dotStyles = resolveDeviceStatusDotStyles(status);

  return (
    <span
      className={`inline-flex h-2.5 w-2.5 rounded-full transition-[box-shadow] transition-transform duration-200 ${dotStyles.className}`}
      style={dotStyles.style}
    />
  );
}
