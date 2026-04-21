import { MaterialIcon } from './MaterialIcon';

interface ZoomControlsProps {
  onZoomIn: () => void;
  onZoomOut: () => void;
  onFitView: () => void;
}

export default function ZoomControls({ onZoomIn, onZoomOut, onFitView }: ZoomControlsProps) {
  const buttonClassName =
    'flex h-11 w-11 items-center justify-center rounded-2xl border border-transparent text-on-bg-secondary transition-[background-color,color,border-color,transform] duration-150 hover:-translate-y-0.5 hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg';

  return (
    <div className="pointer-events-none fixed left-5 bottom-5 z-20">
      <div className="topology-glass topology-floating-shadow pointer-events-auto flex flex-col gap-1 rounded-[20px] p-1.5 transition-colors duration-200">
        <button type="button" onClick={onZoomIn} className={buttonClassName}>
          <MaterialIcon name="zoom_in" />
        </button>
        <button type="button" onClick={onZoomOut} className={buttonClassName}>
          <MaterialIcon name="zoom_out" />
        </button>
        <button type="button" onClick={onFitView} className={buttonClassName}>
          <MaterialIcon name="fit_screen" />
        </button>
      </div>
    </div>
  );
}
