/**
 * Renders reconnect banner UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
interface ReconnectBannerProps {
  visible: boolean;
}

/** Renders the ReconnectBanner component within the UI component boundary. */
export function ReconnectBanner({ visible }: ReconnectBannerProps) {
  return (
    <div
      data-testid="reconnect-banner"
      aria-hidden={!visible}
      className={`pointer-events-none fixed top-32 left-1/2 z-50 -translate-x-1/2 rounded-lg bg-warning/15 px-4 py-2 text-sm text-warning backdrop-blur-sm transition-colors duration-200 transition-opacity duration-300 sm:top-[86px] ${
        visible ? 'opacity-100' : 'opacity-0'
      }`}
      style={{ boxShadow: 'var(--nt-glow-status-warning)' }}
    >
      <div className="flex items-center gap-2">
        <span className="inline-flex h-3.5 w-3.5 animate-spin rounded-full border border-warning/30 border-t-warning" />
        <span>Reconnecting...</span>
      </div>
    </div>
  );
}
