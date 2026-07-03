/**
 * Defines topology hub behavior for the topology hub.
 * Keeps saved-map and area workflows separate from the live canvas surface.
 */
import type { Area, CanvasMap, Device, Link } from '../../types/api';
import type { SnapshotPayload } from '../../types/metrics';
import { AreaManager } from '../AreaManager';
import { MaterialIcon } from '../MaterialIcon';
import { AreaSummaryCard } from './AreaSummaryCard';
import { SavedMapsSection } from './SavedMapsSection';
import { buildTopologyHubModel } from './topologyHubModel';

/** Defines the props contract for TopologyHub within the topology hub. */
export interface TopologyHubProps {
  devices: Device[];
  areas: Area[];
  links: Link[];
  snapshot: SnapshotPayload | null;
  maps: CanvasMap[];
  mapsLoading: boolean;
  mapsError: string | null;
  selectedMapId: string | null;
  selectedMapName: string;
  savedMapsEnabled: boolean;
  onOpenArea: (areaId: string) => void;
  onSelectMap: (map: CanvasMap) => void;
  onOpenMap: (map: CanvasMap) => void;
  onCreateEmptyMap: () => void;
  onCreateMapFromArea: (area: Area) => void;
  onAreasChange?: () => void | Promise<void>;
  onRenameMap?: (map: CanvasMap) => void;
  onDuplicateMap: (map: CanvasMap) => void;
  onDeleteMap: (map: CanvasMap) => void;
  onSetPrimaryMap?: (map: CanvasMap) => void;
  canOpenSettings?: boolean;
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

/** Renders the TopologyHub component within the topology hub. */
export function TopologyHub({
  devices,
  areas,
  links,
  snapshot,
  maps,
  mapsLoading,
  mapsError,
  selectedMapId,
  selectedMapName,
  savedMapsEnabled,
  onOpenArea,
  onSelectMap,
  onOpenMap,
  onCreateEmptyMap,
  onCreateMapFromArea,
  onAreasChange,
  onRenameMap,
  onDuplicateMap,
  onDeleteMap,
  onSetPrimaryMap,
  canOpenSettings = false,
  onOpenSettings,
}: TopologyHubProps) {
  const model = buildTopologyHubModel({ devices, areas, links, snapshot, maps });
  const selectedMap =
    maps.find((map) => map.id === selectedMapId) ??
    maps.find((map) => selectedMapId === null && map.is_default) ??
    maps.find((map) => map.is_default) ??
    maps[0] ??
    null;
  const selectedMapDisplayName = selectedMap?.name ?? selectedMapName;
  const areaMetrics = Object.fromEntries(
    model.areas.map((areaModel) => [
      areaModel.area.id,
      {
        healthLabel: areaModel.healthLabel,
        activeLinkCount: areaModel.activeLinkCount,
        degradedDeviceCount: areaModel.degradedDeviceCount,
        degradedLinkCount: areaModel.degradedLinkCount,
      },
    ]),
  );

  return (
    <div className="mx-auto flex w-full max-w-[1200px] flex-col gap-8 px-6 pb-12 pt-32 sm:px-8 sm:pt-20">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="font-sans text-3xl font-semibold tracking-tight text-on-bg">
            Topology Hub
          </h1>
          <p className="mt-1 text-sm text-on-bg-secondary">
            Saved maps, map-local areas, and topology health
          </p>
        </div>
      </header>

      <div
        className={
          savedMapsEnabled
            ? 'grid grid-cols-1 gap-6 lg:grid-cols-[minmax(18rem,24rem)_1fr]'
            : 'grid grid-cols-1'
        }
      >
        {savedMapsEnabled && (
          <SavedMapsSection
            maps={model.maps}
            selectedMapId={selectedMap?.id ?? selectedMapId}
            loading={mapsLoading}
            error={mapsError}
            onCreateEmptyMap={onCreateEmptyMap}
            onSelectMap={onSelectMap}
            onOpenMap={onOpenMap}
            onRenameMap={onRenameMap}
            onDuplicateMap={onDuplicateMap}
            onDeleteMap={onDeleteMap}
            onSetPrimaryMap={onSetPrimaryMap}
          />
        )}

        <section className="flex min-w-0 flex-col gap-6" aria-labelledby="selected-map-heading">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="min-w-0">
              <p className="text-xs font-medium text-on-bg-secondary">Selected map</p>
              <h2 id="selected-map-heading" className="truncate text-xl font-semibold text-on-bg">
                {selectedMapDisplayName}
              </h2>
            </div>
            <button
              type="button"
              aria-label="Open selected map"
              disabled={!selectedMap}
              onClick={() => {
                if (selectedMap) {
                  onOpenMap(selectedMap);
                }
              }}
              className="inline-flex items-center gap-2 rounded-lg border border-outline bg-surface px-3 py-2 text-sm font-medium text-on-bg transition-colors hover:bg-surface-container focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg disabled:pointer-events-none disabled:opacity-50"
            >
              <MaterialIcon name="open_in_full" size={18} />
              Open selected map
            </button>
          </div>

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
            {savedMapsEnabled && selectedMap ? (
              <AreaManager
                title="Map-local areas"
                titleId="areas-heading"
                mapContext={{ mapId: selectedMap.id, mapName: selectedMap.name }}
                areas={areas}
                devices={devices}
                areaMetrics={areaMetrics}
                onAreasChange={onAreasChange}
                onOpenArea={onOpenArea}
                onCreateMapFromArea={onCreateMapFromArea}
              />
            ) : model.areas.length > 0 ? (
              <>
                <div className="flex items-center justify-between gap-3">
                  <h3 id="areas-heading" className="text-base font-semibold text-on-bg">
                    Map-local areas
                  </h3>
                  <span className="font-mono text-xs text-on-bg-secondary">
                    {model.areas.length}
                  </span>
                </div>
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
              </>
            ) : (
              <div className="rounded-lg border border-dashed border-outline bg-surface p-4">
                <h3 id="areas-heading" className="text-base font-semibold text-on-bg">
                  Map-local areas
                </h3>
                <p className="mt-2 text-sm font-medium text-on-bg">No areas</p>
                {canOpenSettings && (
                  <button
                    type="button"
                    onClick={onOpenSettings}
                    className="mt-3 inline-flex items-center gap-2 rounded-lg border border-outline px-3 py-2 text-xs font-medium text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
                  >
                    <MaterialIcon name="settings" size={16} />
                    Settings
                  </button>
                )}
              </div>
            )}
          </section>
        </section>
      </div>
    </div>
  );
}

export default TopologyHub;
