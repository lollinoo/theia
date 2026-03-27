import { MaterialIcon } from './MaterialIcon';

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
    'flex h-12 w-12 items-center justify-center bg-surface/90 text-on-bg transition-colors duration-150 hover:bg-elevated';

  return (
    <div className="pointer-events-none fixed left-5 bottom-5 z-20">
      <div className="pointer-events-auto overflow-hidden rounded-2xl shadow-canvas dark:backdrop-blur-xl transition-colors duration-200">
        <button type="button" onClick={onZoomIn} className={`${buttonClassName} rounded-t-2xl`}>
          <MaterialIcon name="zoom_in" />
        </button>
        <button type="button" onClick={onZoomOut} className={buttonClassName}>
          <MaterialIcon name="zoom_out" />
        </button>
        <button
          type="button"
          onClick={onFitView}
          className={`${buttonClassName} rounded-b-2xl`}
        >
          <MaterialIcon name="fit_screen" />
        </button>
      </div>
    </div>
  );
}
