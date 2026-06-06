/**
 * Defines saved maps section behavior for the topology hub.
 * Keeps saved-map and area workflows separate from the live canvas surface.
 */
import type { CanvasMap } from '../../types/api';
import { MaterialIcon } from '../MaterialIcon';
import { MapSummaryCard } from './MapSummaryCard';

/** Defines the props contract for SavedMapsSection within the topology hub. */
export interface SavedMapsSectionProps {
  maps: CanvasMap[];
  selectedMapId: string | null;
  loading: boolean;
  error: string | null;
  onCreateEmptyMap: () => void;
  onSelectMap: (map: CanvasMap) => void;
  onOpenMap: (map: CanvasMap) => void;
  onRenameMap?: (map: CanvasMap) => void;
  onDuplicateMap: (map: CanvasMap) => void;
  onDeleteMap: (map: CanvasMap) => void;
  onSetPrimaryMap?: (map: CanvasMap) => void;
}

/** Renders the SavedMapsSection component within the topology hub. */
export function SavedMapsSection({
  maps,
  selectedMapId,
  loading,
  error,
  onCreateEmptyMap,
  onSelectMap,
  onOpenMap,
  onRenameMap,
  onDuplicateMap,
  onDeleteMap,
  onSetPrimaryMap,
}: SavedMapsSectionProps) {
  const hasMaps = maps.length > 0;

  return (
    <section className="flex flex-col gap-3" aria-labelledby="saved-maps-heading">
      <div className="flex items-center justify-between gap-3">
        <h2 id="saved-maps-heading" className="text-base font-semibold text-on-bg">
          Saved maps
        </h2>
        <div className="flex items-center gap-2">
          <span className="font-mono text-xs text-on-bg-secondary">{maps.length}</span>
          {loading && hasMaps && (
            <span className="rounded-full border border-outline-subtle px-2 py-0.5 text-[11px] font-medium text-on-bg-secondary">
              Refreshing
            </span>
          )}
          <button
            type="button"
            aria-label="Create empty map"
            onClick={onCreateEmptyMap}
            className="inline-flex h-8 items-center gap-1 rounded-lg border border-outline bg-surface px-2.5 text-xs font-medium text-on-bg transition-colors hover:bg-surface-container focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
          >
            <MaterialIcon name="add" size={16} />
            New
          </button>
        </div>
      </div>

      {loading && !hasMaps ? (
        <div className="rounded-lg border border-outline bg-surface p-4 text-sm text-on-bg-secondary">
          Loading maps
        </div>
      ) : error ? (
        <div className="rounded-lg border border-status-down/40 bg-surface p-4 text-sm text-critical">
          {error}
        </div>
      ) : maps.length === 0 ? (
        <div className="rounded-lg border border-dashed border-outline bg-surface p-4 text-sm text-on-bg-secondary">
          No saved maps
        </div>
      ) : (
        <ul
          aria-label="Saved maps list"
          className="flex flex-col divide-y divide-outline-subtle rounded-lg border border-outline-subtle bg-surface"
        >
          {maps.map((map) => (
            <MapSummaryCard
              key={map.id}
              map={map}
              selected={
                map.id === selectedMapId || (selectedMapId === null && map.is_default === true)
              }
              onSelect={onSelectMap}
              onOpen={onOpenMap}
              onRename={onRenameMap}
              onDuplicate={onDuplicateMap}
              onDelete={onDeleteMap}
              onSetPrimary={onSetPrimaryMap}
            />
          ))}
        </ul>
      )}
    </section>
  );
}

export default SavedMapsSection;
