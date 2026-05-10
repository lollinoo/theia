import type { ActiveView } from '../App';
import type { Area } from '../types/api';

interface WatermarkProps {
  activeView: ActiveView;
  selectedAreaId: string | null;
  areas: Area[];
  hidden?: boolean;
}

/** Canvas-relative atmospheric watermark with contextual text. Only visible on canvas view. */
export function Watermark({ activeView, selectedAreaId, areas, hidden = false }: WatermarkProps) {
  if (hidden || activeView !== 'canvas') return null;

  const text = selectedAreaId
    ? (areas.find((a) => a.id === selectedAreaId)?.name ?? 'AREA').toUpperCase()
    : 'GLOBAL TOPOLOGY';

  return (
    <div
      className="absolute bottom-[calc(15.5rem+env(safe-area-inset-bottom))] right-4 z-10 pointer-events-none select-none sm:bottom-[184px]"
      aria-hidden="true"
    >
      <span
        className="font-sans font-semibold text-xs tracking-[0.2em] uppercase
                   text-on-bg-muted opacity-30 dark:opacity-40
                   transition-opacity duration-150"
      >
        {text}
      </span>
    </div>
  );
}
