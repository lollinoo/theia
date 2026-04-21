import type { ActiveView } from '../App';
import type { Area } from '../types/api';

interface WatermarkProps {
  activeView: ActiveView;
  selectedAreaId: string | null;
  areas: Area[];
}

/** Fixed-position atmospheric watermark with contextual text. Only visible on canvas view. */
export function Watermark({ activeView, selectedAreaId, areas }: WatermarkProps) {
  if (activeView !== 'canvas') return null;

  const text = selectedAreaId
    ? (areas.find((a) => a.id === selectedAreaId)?.name ?? 'AREA').toUpperCase()
    : 'GLOBAL TOPOLOGY';

  return (
    <div
      className="fixed bottom-[170px] right-3 z-10 pointer-events-none select-none"
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
