interface CanvasLoadingStateProps {
  importedNodeCount?: number;
  overlay?: boolean;
}

/** Renders either the initial topology loader or the import-placement preparation state. */
export function CanvasLoadingState({
  importedNodeCount,
  overlay = false,
}: CanvasLoadingStateProps = {}) {
  const arrangingImportedNodes = importedNodeCount !== undefined;
  const nodeLabel = importedNodeCount === 1 ? 'node' : 'nodes';

  return (
    <div
      data-testid={arrangingImportedNodes ? 'imported-node-placement-overlay' : undefined}
      role="status"
      aria-atomic="true"
      aria-busy="true"
      aria-live="polite"
      className={`topology-backdrop flex items-center justify-center bg-bg ${
        overlay ? 'topology-import-placement-overlay absolute inset-0 z-[70] px-6' : 'h-full'
      }`}
    >
      {arrangingImportedNodes ? (
        <div className="max-w-md rounded-[28px] border border-outline bg-surface/88 px-7 py-6 text-center shadow-canvas backdrop-blur-sm">
          <div className="relative mx-auto mb-4 h-12 w-12" aria-hidden="true">
            <div className="absolute inset-0 animate-spin rounded-full border-2 border-outline-subtle border-t-primary" />
            <div className="absolute inset-[9px] rounded-full bg-primary/10" />
          </div>
          <p className="text-xs font-medium uppercase tracking-[0.24em] text-primary">
            Preparing topology
          </p>
          <h2 className="mt-3 text-xl font-semibold tracking-tight text-on-bg">
            Arranging {importedNodeCount} imported {nodeLabel}…
          </h2>
          <p className="mt-3 text-sm leading-6 text-on-bg-secondary">
            Finding available space and saving the layout.
          </p>
        </div>
      ) : (
        <div className="rounded-[28px] border border-outline bg-surface/88 px-6 py-5 text-center shadow-canvas backdrop-blur-sm">
          <div
            className="mx-auto mb-3 h-10 w-10 animate-spin rounded-full border-2 border-outline-subtle border-t-primary"
            aria-hidden="true"
          />
          <p className="text-sm uppercase tracking-[0.28em] text-on-bg-secondary">
            Loading topology...
          </p>
        </div>
      )}
    </div>
  );
}
