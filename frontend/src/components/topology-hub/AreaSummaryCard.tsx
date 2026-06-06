/**
 * Defines area summary card behavior for the topology hub.
 * Keeps saved-map and area workflows separate from the live canvas surface.
 */
import type { Area } from '../../types/api';
import { MaterialIcon } from '../MaterialIcon';
import type { TopologyHubAreaModel } from './topologyHubModel';

/** Defines the props contract for AreaSummaryCard within the topology hub. */
export interface AreaSummaryCardProps {
  areaModel: TopologyHubAreaModel;
  savedMapsEnabled: boolean;
  onOpenArea: (areaId: string) => void;
  onCreateMapFromArea: (area: Area) => void;
}

/** Renders the AreaSummaryCard component within the topology hub. */
export function AreaSummaryCard({
  areaModel,
  savedMapsEnabled,
  onOpenArea,
  onCreateMapFromArea,
}: AreaSummaryCardProps) {
  const { area } = areaModel;

  return (
    <article className="rounded-lg border border-outline bg-surface p-4 shadow-panel transition-colors">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span
              className="h-2.5 w-2.5 shrink-0 rounded-full"
              style={{ backgroundColor: area.color }}
            />
            <h3 className="truncate text-sm font-semibold text-on-bg">{area.name}</h3>
          </div>
          {area.description && (
            <p className="mt-1 line-clamp-2 text-xs text-on-bg-secondary">{area.description}</p>
          )}
        </div>
        {savedMapsEnabled && (
          <button
            type="button"
            aria-label={`Create map from area ${area.name}`}
            title="Create map"
            onClick={() => onCreateMapFromArea(area)}
            className="rounded-full p-1.5 text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
          >
            <MaterialIcon name="add_location_alt" size={18} />
          </button>
        )}
      </div>

      <dl className="mt-4 grid grid-cols-3 gap-2 text-xs">
        <div>
          <dt className="text-on-bg-secondary">Health</dt>
          <dd
            className={
              areaModel.degradedDeviceCount > 0
                ? 'font-medium text-critical'
                : 'font-medium text-status-up'
            }
          >
            {areaModel.healthLabel}
          </dd>
        </div>
        <div>
          <dt className="text-on-bg-secondary">Devices</dt>
          <dd className="font-mono text-sm text-on-bg">{areaModel.deviceCount}</dd>
        </div>
        <div>
          <dt className="text-on-bg-secondary">Links</dt>
          <dd className="font-mono text-sm text-on-bg">{areaModel.activeLinkCount}</dd>
        </div>
      </dl>

      <button
        type="button"
        aria-label={`Open area ${area.name}`}
        onClick={() => onOpenArea(area.id)}
        className="mt-4 inline-flex items-center gap-1.5 rounded-lg border border-outline px-3 py-2 text-xs font-medium text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
      >
        <MaterialIcon name="hub" size={16} />
        Open
      </button>
    </article>
  );
}

export default AreaSummaryCard;
