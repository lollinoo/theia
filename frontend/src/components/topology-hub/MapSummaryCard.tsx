import type { CanvasMap } from '../../types/api';
import { MaterialIcon } from '../MaterialIcon';

export interface MapSummaryCardProps {
  map: CanvasMap;
  onOpen: (map: CanvasMap) => void;
  onDuplicate: (map: CanvasMap) => void;
  onDelete: (map: CanvasMap) => void;
}

export function MapSummaryCard({ map, onOpen, onDuplicate, onDelete }: MapSummaryCardProps) {
  return (
    <article className="rounded-lg border border-outline bg-surface p-4 shadow-panel transition-colors">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="truncate text-sm font-semibold text-on-bg">{map.name}</h3>
            {map.is_default && (
              <span className="rounded-full border border-outline-subtle px-2 py-0.5 text-[11px] font-medium text-on-bg-secondary">
                Primary
              </span>
            )}
          </div>
          {map.description && (
            <p className="mt-1 line-clamp-2 text-xs text-on-bg-secondary">{map.description}</p>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <button
            type="button"
            aria-label={`Open map ${map.name}`}
            title="Open"
            onClick={() => onOpen(map)}
            className="rounded-full p-1.5 text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
          >
            <MaterialIcon name="open_in_full" size={18} />
          </button>
          <button
            type="button"
            aria-label={`Duplicate ${map.name}`}
            title="Duplicate"
            onClick={() => onDuplicate(map)}
            className="rounded-full p-1.5 text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
          >
            <MaterialIcon name="content_copy" size={18} />
          </button>
          {!map.is_default && (
            <button
              type="button"
              aria-label={`Delete ${map.name}`}
              title="Delete"
              onClick={() => onDelete(map)}
              className="rounded-full p-1.5 text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-critical focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
            >
              <MaterialIcon name="delete" size={18} />
            </button>
          )}
        </div>
      </div>

      <dl className="mt-4 grid grid-cols-3 gap-2 text-xs">
        <div>
          <dt className="text-on-bg-secondary">Devices</dt>
          <dd className="font-mono text-sm text-on-bg">{map.device_count}</dd>
        </div>
        <div>
          <dt className="text-on-bg-secondary">Links</dt>
          <dd className="font-mono text-sm text-on-bg">{map.link_count}</dd>
        </div>
        <div>
          <dt className="text-on-bg-secondary">Positions</dt>
          <dd className="font-mono text-sm text-on-bg">{map.position_count}</dd>
        </div>
      </dl>
    </article>
  );
}

export default MapSummaryCard;
