import type { Area, CanvasMap, Device, Link } from '../../types/api';
import type { SnapshotPayload } from '../../types/metrics';
import { MaterialIcon } from '../MaterialIcon';
import { AreaSummaryCard } from './AreaSummaryCard';
import { SavedMapsSection } from './SavedMapsSection';
import { buildTopologyHubModel } from './topologyHubModel';

export interface TopologyHubProps {
  devices: Device[];
  areas: Area[];
  links: Link[];
  snapshot: SnapshotPayload | null;
  maps: CanvasMap[];
  mapsLoading: boolean;
  mapsError: string | null;
  savedMapsEnabled: boolean;
  onOpenGlobal: () => void;
  onOpenArea: (areaId: string) => void;
  onOpenMap: (map: CanvasMap) => void;
  onCreateEmptyMap: () => void;
  onCreateMapFromArea: (area: Area) => void;
  onDuplicateMap: (map: CanvasMap) => void;
  onDeleteMap: (map: CanvasMap) => void;
  onOpenSettings: () => void;
}

function StatBlock({
  label,
  value,
  tone,
}: {
  label: string;
  value: string | number;
  tone?: 'critical' | 'normal';
}) {
  return (
    <div className="rounded-lg border border-outline bg-surface p-4 shadow-panel">
      <p className="text-xs font-medium text-on-bg-secondary">{label}</p>
      <p
        className={
          tone === 'critical'
            ? 'mt-2 font-mono text-2xl text-critical'
            : 'mt-2 font-mono text-2xl text-on-bg'
        }
      >
        {value}
      </p>
    </div>
  );
}

export function TopologyHub({
  devices,
  areas,
  links,
  snapshot,
  maps,
  mapsLoading,
  mapsError,
  savedMapsEnabled,
  onOpenGlobal,
  onOpenArea,
  onOpenMap,
  onCreateEmptyMap,
  onCreateMapFromArea,
  onDuplicateMap,
  onDeleteMap,
  onOpenSettings,
}: TopologyHubProps) {
  const model = buildTopologyHubModel({ devices, areas, links, snapshot, maps });

  return (
    <div className="mx-auto flex w-full max-w-[1200px] flex-col gap-8 px-6 pb-12 pt-20 sm:px-8">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="font-sans text-3xl font-semibold tracking-tight text-on-bg">
            Topology Hub
          </h1>
          <p className="mt-1 text-sm text-on-bg-secondary">Network aggregate</p>
        </div>
        <button
          type="button"
          aria-label="Open global map"
          onClick={onOpenGlobal}
          className="inline-flex items-center gap-2 rounded-lg border border-outline bg-surface px-3 py-2 text-sm font-medium text-on-bg transition-colors hover:bg-surface-container focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
        >
          <MaterialIcon name="public" size={18} />
          Global
        </button>
      </header>

      <section className="grid grid-cols-2 gap-3 lg:grid-cols-5" aria-label="Topology summary">
        <StatBlock label="Health" value={`${model.aggregate.healthPercentage}%`} />
        <StatBlock label="Devices" value={model.aggregate.totalDevices} />
        <StatBlock label="Links" value={model.aggregate.activeLinks} />
        <StatBlock
          label="Attention"
          value={model.aggregate.degradedDevices}
          tone={model.aggregate.degradedDevices > 0 ? 'critical' : 'normal'}
        />
        <StatBlock label="Unassigned" value={model.unassignedDevices.length} />
      </section>

      <section className="flex flex-col gap-3" aria-labelledby="areas-heading">
        <div className="flex items-center justify-between gap-3">
          <h2 id="areas-heading" className="text-base font-semibold text-on-bg">
            Areas
          </h2>
          <span className="font-mono text-xs text-on-bg-secondary">{model.areas.length}</span>
        </div>

        {model.areas.length > 0 ? (
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
            {model.areas.map((areaModel) => (
              <AreaSummaryCard
                key={areaModel.area.id}
                areaModel={areaModel}
                savedMapsEnabled={savedMapsEnabled}
                onOpenArea={onOpenArea}
                onCreateMapFromArea={onCreateMapFromArea}
              />
            ))}
          </div>
        ) : (
          <div className="rounded-lg border border-dashed border-outline bg-surface p-4">
            <p className="text-sm font-medium text-on-bg">No areas</p>
            <button
              type="button"
              onClick={onOpenSettings}
              className="mt-3 inline-flex items-center gap-2 rounded-lg border border-outline px-3 py-2 text-xs font-medium text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
            >
              <MaterialIcon name="settings" size={16} />
              Settings
            </button>
          </div>
        )}
      </section>

      {savedMapsEnabled && (
        <SavedMapsSection
          maps={model.maps}
          loading={mapsLoading}
          error={mapsError}
          onCreateEmptyMap={onCreateEmptyMap}
          onOpenMap={onOpenMap}
          onDuplicateMap={onDuplicateMap}
          onDeleteMap={onDeleteMap}
        />
      )}
    </div>
  );
}

export default TopologyHub;
