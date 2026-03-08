interface ZoomControlsProps {
  onZoomIn: () => void;
  onZoomOut: () => void;
  onFitView: () => void;
}

export default function ZoomControls({
  onZoomIn,
  onZoomOut,
  onFitView,
}: ZoomControlsProps) {
  const buttonClassName =
    'flex h-12 w-12 items-center justify-center border-b border-border-subtle bg-bg-surface/90 text-lg text-text-primary transition-colors duration-150 hover:bg-bg-elevated last:border-b-0';

  return (
    <div className="pointer-events-none fixed left-5 bottom-5 z-20">
      <div className="pointer-events-auto overflow-hidden rounded-2xl border border-border-subtle shadow-canvas backdrop-blur-xl">
        <button type="button" onClick={onZoomIn} className={`${buttonClassName} rounded-t-2xl`}>
          +
        </button>
        <button type="button" onClick={onZoomOut} className={buttonClassName}>
          -
        </button>
        <button
          type="button"
          onClick={onFitView}
          className={`${buttonClassName} rounded-b-2xl text-sm font-semibold`}
        >
          Fit
        </button>
      </div>
    </div>
  );
}
