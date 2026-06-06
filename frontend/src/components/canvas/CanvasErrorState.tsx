interface CanvasErrorStateProps {
  error: string;
  onRetry: () => void;
}

export function CanvasErrorState({ error, onRetry }: CanvasErrorStateProps) {
  return (
    <div className="topology-backdrop flex h-full items-center justify-center bg-bg px-6">
      <div className="max-w-md rounded-[28px] border border-outline bg-surface/88 px-6 py-6 text-center shadow-canvas backdrop-blur-sm">
        <p className="text-sm uppercase tracking-[0.28em] text-status-down">Topology Error</p>
        <h2 className="mt-3 text-2xl font-semibold tracking-tight text-on-bg">
          Canvas data could not load
        </h2>
        <p className="mt-3 text-sm text-on-bg-secondary">{error}</p>
        <button
          type="button"
          onClick={onRetry}
          className="mt-6 rounded-full border border-primary/40 bg-primary/10 px-5 py-2 text-sm font-medium text-primary transition-colors duration-150 hover:bg-primary/20"
        >
          Retry
        </button>
      </div>
    </div>
  );
}
