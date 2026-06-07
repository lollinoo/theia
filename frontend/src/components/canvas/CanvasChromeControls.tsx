/**
 * Defines canvas chrome controls behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import { MaterialIcon } from '../MaterialIcon';

const canvasChromeButtonClassName =
  'topology-glass topology-floating-shadow flex h-11 w-11 items-center justify-center rounded-[16px] text-on-bg-secondary transition-[background-color,color,border-color,transform] duration-150 hover:-translate-y-0.5 hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg';

interface CanvasChromeControlsProps {
  chromeHidden: boolean;
  onToggleChrome: () => void;
  onSearch: () => void;
  onFitView: () => void;
}

/** Renders the CanvasChromeControls component within the topology canvas. */
export function CanvasChromeControls({
  chromeHidden,
  onToggleChrome,
  onSearch,
  onFitView,
}: CanvasChromeControlsProps) {
  const positionClassName = chromeHidden ? 'right-4 top-4' : 'right-20 top-32 sm:top-20 xl:top-4';

  return (
    <div
      data-testid="canvas-chrome-controls"
      className={`absolute ${positionClassName} z-[70] flex items-center gap-2`}
    >
      {chromeHidden && (
        <>
          <button
            type="button"
            aria-label="Search devices"
            title="Search devices"
            onClick={onSearch}
            className={canvasChromeButtonClassName}
          >
            <MaterialIcon name="search" />
          </button>
          <button
            type="button"
            aria-label="Fit view"
            title="Fit view"
            onClick={onFitView}
            className={canvasChromeButtonClassName}
          >
            <MaterialIcon name="fit_screen" />
          </button>
        </>
      )}
      <button
        type="button"
        aria-label={chromeHidden ? 'Show canvas controls' : 'Hide canvas controls'}
        title={chromeHidden ? 'Show canvas controls' : 'Hide canvas controls'}
        aria-pressed={chromeHidden}
        onClick={onToggleChrome}
        className={canvasChromeButtonClassName}
      >
        <MaterialIcon name={chromeHidden ? 'close_fullscreen' : 'open_in_full'} />
      </button>
    </div>
  );
}
