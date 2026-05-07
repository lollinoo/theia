import type { CanvasMap } from '../../types/api';
import { MapSummaryCard } from './MapSummaryCard';

export interface SavedMapsSectionProps {
  maps: CanvasMap[];
  loading: boolean;
  error: string | null;
  onOpenMap: (map: CanvasMap) => void;
  onDuplicateMap: (map: CanvasMap) => void;
  onDeleteMap: (map: CanvasMap) => void;
}

export function SavedMapsSection({
  maps,
  loading,
  error,
  onOpenMap,
  onDuplicateMap,
  onDeleteMap,
}: SavedMapsSectionProps) {
  return (
    <section className="flex flex-col gap-3" aria-labelledby="saved-maps-heading">
      <div className="flex items-center justify-between gap-3">
        <h2 id="saved-maps-heading" className="text-base font-semibold text-on-bg">
          Saved maps
        </h2>
        <span className="font-mono text-xs text-on-bg-secondary">{maps.length}</span>
      </div>

      {loading ? (
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
        <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
          {maps.map((map) => (
            <MapSummaryCard
              key={map.id}
              map={map}
              onOpen={onOpenMap}
              onDuplicate={onDuplicateMap}
              onDelete={onDeleteMap}
            />
          ))}
        </div>
      )}
    </section>
  );
}

export default SavedMapsSection;
