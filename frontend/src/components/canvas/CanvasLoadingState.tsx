export function CanvasLoadingState() {
  return (
    <div className="topology-backdrop flex h-full items-center justify-center bg-bg">
      <div className="rounded-[28px] border border-outline bg-surface/88 px-6 py-5 text-center shadow-canvas backdrop-blur-sm">
        <div className="mx-auto mb-3 h-10 w-10 animate-spin rounded-full border-2 border-outline-subtle border-t-primary" />
        <p className="text-sm uppercase tracking-[0.28em] text-on-bg-secondary">
          Loading topology...
        </p>
      </div>
    </div>
  );
}
