import type { ActiveView } from '../App';
import type { Area } from '../types/api';

interface WatermarkProps {
  activeView: ActiveView;
  selectedAreaId: string | null;
  areas: Area[];
  mapName?: string;
  compact?: boolean;
  hidden?: boolean;
}

/** Canvas-relative atmospheric watermark with contextual text. Only visible on canvas view. */
export function Watermark({
  activeView,
  selectedAreaId,
  areas,
  mapName,
  compact = false,
  hidden = false,
}: WatermarkProps) {
  if (hidden || activeView !== 'canvas') return null;

  const text = selectedAreaId
    ? (areas.find((a) => a.id === selectedAreaId)?.name ?? 'AREA').toUpperCase()
    : 'GLOBAL TOPOLOGY';

  return (
    <div
      className={`absolute right-4 z-10 pointer-events-none select-none ${
        compact
          ? 'bottom-[calc(1rem+env(safe-area-inset-bottom))]'
          : 'bottom-[calc(15.5rem+env(safe-area-inset-bottom))] sm:bottom-[184px]'
      }`}
      aria-hidden="true"
    >
      <div className="flex flex-col items-end gap-0.5">
        <span
          className="font-sans font-semibold text-xs tracking-[0.2em] uppercase
                     text-on-bg-muted opacity-30 dark:opacity-40
                     transition-opacity duration-150"
        >
          {text}
        </span>
        {mapName && (
          <span
            className="font-sans text-[11px] italic tracking-[0.08em]
                       text-on-bg-muted opacity-25 dark:opacity-35"
          >
            {mapName}
          </span>
        )}
      </div>
    </div>
  );
}
