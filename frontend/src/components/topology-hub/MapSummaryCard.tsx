import type { MouseEvent } from 'react';
import type { CanvasMap } from '../../types/api';
import { MaterialIcon } from '../MaterialIcon';

export interface MapSummaryCardProps {
  map: CanvasMap;
  selected: boolean;
  onSelect: (map: CanvasMap) => void;
  onOpen: (map: CanvasMap) => void;
  onRename?: (map: CanvasMap) => void;
  onDuplicate: (map: CanvasMap) => void;
  onDelete: (map: CanvasMap) => void;
  onSetPrimary?: (map: CanvasMap) => void;
}

export function MapSummaryCard({
  map,
  selected,
  onSelect,
  onOpen,
  onRename,
  onDuplicate,
  onDelete,
  onSetPrimary,
}: MapSummaryCardProps) {
  const selectMap = () => onSelect(map);
  const handleActionClick = (
    event: MouseEvent<HTMLButtonElement>,
    action: (map: CanvasMap) => void,
  ) => {
    event.stopPropagation();
    action(map);
  };

  return (
    <li
      aria-label={`Map ${map.name}`}
      className={`relative border-l-2 px-3 py-3 transition-colors first:rounded-t-lg last:rounded-b-lg hover:bg-surface-container ${
        selected ? 'border-l-primary bg-surface-container' : 'border-l-transparent'
      }`}
    >
      <button
        type="button"
        aria-label={`Select map ${map.name}`}
        onClick={selectMap}
        className="absolute inset-0 z-10 rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
      />
      <div className="pointer-events-none relative z-20 flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex min-w-0 items-center gap-2">
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
        <div className="pointer-events-auto flex shrink-0 items-center gap-1">
          <button
            type="button"
            aria-label={`Open map ${map.name}`}
            title="Open"
            onClick={(event) => handleActionClick(event, onOpen)}
            className="rounded-full p-1.5 text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
          >
            <MaterialIcon name="open_in_full" size={18} />
          </button>
          {onRename && (
            <button
              type="button"
              aria-label={`Rename ${map.name}`}
              title="Rename"
              onClick={(event) => handleActionClick(event, onRename)}
              className="rounded-full p-1.5 text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
            >
              <MaterialIcon name="edit" size={18} />
            </button>
          )}
          <button
            type="button"
            aria-label={`Duplicate ${map.name}`}
            title="Duplicate"
            onClick={(event) => handleActionClick(event, onDuplicate)}
            className="rounded-full p-1.5 text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
          >
            <MaterialIcon name="content_copy" size={18} />
          </button>
          {!map.is_default && onSetPrimary && (
            <button
              type="button"
              aria-label={`Set ${map.name} as primary`}
              title="Set as primary"
              onClick={(event) => handleActionClick(event, onSetPrimary)}
              className="rounded-full p-1.5 text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
            >
              <MaterialIcon name="check_circle" size={18} />
            </button>
          )}
          {!map.is_default && (
            <button
              type="button"
              aria-label={`Delete ${map.name}`}
              title="Delete"
              onClick={(event) => handleActionClick(event, onDelete)}
              className="rounded-full p-1.5 text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-critical focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
            >
              <MaterialIcon name="delete" size={18} />
            </button>
          )}
        </div>
      </div>

      <dl className="pointer-events-none relative z-20 mt-4 grid grid-cols-3 gap-2 text-xs">
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
    </li>
  );
}

export default MapSummaryCard;
