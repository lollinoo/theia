/**
 * Renders status dot UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import { type DeviceVisualStatus, resolveDeviceStatusDotStyles } from './deviceVisualState';

interface StatusDotProps {
  status: DeviceVisualStatus;
}

/** Renders the StatusDot component within the UI component boundary. */
export function StatusDot({ status }: StatusDotProps) {
  const dotStyles = resolveDeviceStatusDotStyles(status);

  return (
    <span
      className={`inline-flex h-2.5 w-2.5 rounded-full transition-[box-shadow] transition-transform duration-200 ${dotStyles.className}`}
      style={dotStyles.style}
    />
  );
}
