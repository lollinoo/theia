interface ReconnectBannerProps {
  visible: boolean;
}

export function ReconnectBanner({ visible }: ReconnectBannerProps) {
  return (
    <div
      aria-hidden={!visible}
      className={`pointer-events-none fixed top-4 left-1/2 z-50 -translate-x-1/2 rounded-lg bg-yellow-900/80 px-4 py-2 text-sm text-yellow-200 backdrop-blur-sm transition-opacity duration-300 ${
        visible ? 'opacity-100' : 'opacity-0'
      }`}
    >
      <div className="flex items-center gap-2">
        <span className="inline-flex h-3.5 w-3.5 animate-spin rounded-full border border-yellow-200/30 border-t-yellow-200" />
        <span>Reconnecting...</span>
      </div>
    </div>
  );
}
