import type { Area, Device, Link } from '../types/api';
import type { SnapshotPayload } from '../types/metrics';
import AreaCard from './AreaCard';

/** Props for the AreaHub component. */
interface AreaHubProps {
  devices: Device[];
  areas: Area[];
  links: Link[];
  snapshot: SnapshotPayload | null;
  onAreaSelect: (areaId: string) => void;
  onOpenSettings: () => void;
}

/** Compute health stats from devices and optional live snapshot statuses. */
function computeHealth(
  devices: Device[],
  snapshot: SnapshotPayload | null,
): { percentage: number; label: string; color: string } {
  if (devices.length === 0) {
    return { percentage: 100, label: 'N/A', color: 'text-on-bg-secondary' };
  }

  const upCount = devices.filter((d) => {
    const liveStatus = snapshot?.device_statuses[d.id] ?? d.status;
    return liveStatus === 'up';
  }).length;

  const percentage = (upCount / devices.length) * 100;

  if (percentage >= 95) {
    return { percentage, label: 'Optimal', color: 'text-status-up' };
  }
  if (percentage >= 80) {
    return { percentage, label: 'Degraded', color: 'text-warning' };
  }
  return { percentage, label: 'Critical', color: 'text-status-down' };
}


/** Hub view with aggregate stats header and area card grid. */
export default function AreaHub({
  devices,
  areas,
  links,
  snapshot,
  onAreaSelect,
  onOpenSettings,
}: AreaHubProps) {
  // --- Aggregate stats ---
  const aggregateHealth = computeHealth(devices, snapshot);

  return (
    <div className="w-full max-w-[1200px] mx-auto mt-20 px-8 pb-12 flex flex-col gap-12">
      {/* HEADER SECTION */}
      <div>
        <h1 className="font-sans font-semibold text-4xl tracking-tight text-on-bg">
          OSPF Area Hub
        </h1>
        <p className="text-on-bg-secondary text-base mt-1">
          Global Network Aggregate Overview
        </p>
      </div>

      {/* AGGREGATE STATS */}
      <div className="flex flex-wrap gap-4">
        {/* Stat 1: Aggregate Health */}
        <div className="flex-1 bg-surface border border-outline rounded-xl p-6 shadow-panel min-w-[200px] transition-colors duration-200">
          <p className="text-xs font-semibold text-on-bg-secondary uppercase tracking-wider mb-4">
            Aggregate Health
          </p>
          <p className="text-4xl font-mono text-on-bg">
            {devices.length > 0 ? `${Math.round(aggregateHealth.percentage)}%` : 'N/A'}
          </p>
          <p className={`text-sm mt-1 ${aggregateHealth.color}`}>
            {aggregateHealth.label}
          </p>
        </div>

        {/* Stat 3: Total Devices */}
        <div className="flex-1 bg-surface border border-outline rounded-xl p-6 shadow-panel min-w-[200px] transition-colors duration-200">
          <p className="text-xs font-semibold text-on-bg-secondary uppercase tracking-wider mb-4">
            Total Devices
          </p>
          <p className="text-4xl font-mono text-on-bg">
            {devices.length}
          </p>
        </div>

        {/* Stat 4: Active Links */}
        <div className="flex-1 bg-surface border border-outline rounded-xl p-6 shadow-panel min-w-[200px] transition-colors duration-200">
          <p className="text-xs font-semibold text-on-bg-secondary uppercase tracking-wider mb-4">
            Active Links
          </p>
          <p className="text-4xl font-mono text-on-bg">
            {links.length}
          </p>
        </div>
      </div>

      {/* AREA CARD GRID or EMPTY STATE */}
      {areas.length > 0 ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {areas.map((area) => {
            const areaDevices = devices.filter((d) => d.area_id === area.id);
            const areaHealth = computeHealth(areaDevices, snapshot);
            // Count links where at least one endpoint is in this area
            const areaDeviceIds = new Set(areaDevices.map((d) => d.id));
            const activeLinkCount = links.filter(
              (l) => areaDeviceIds.has(l.source_device_id) || areaDeviceIds.has(l.target_device_id),
            ).length;

            return (
              <AreaCard
                key={area.id}
                area={area}
                healthPercentage={areaHealth.percentage}
                healthLabel={areaHealth.label}
                healthColor={areaHealth.color}
                deviceCount={areaDevices.length}
                activeLinkCount={activeLinkCount}
                onClick={() => onAreaSelect(area.id)}
              />
            );
          })}
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          <div className="bg-surface border border-dashed border-outline rounded-xl p-6 flex flex-col items-center justify-center text-center min-h-[180px] transition-colors duration-200">
            <p className="text-on-bg font-semibold text-lg">No areas yet</p>
            <p className="text-on-bg-secondary text-sm mt-1">
              Create your first area in Settings
            </p>
            <button
              type="button"
              className="text-primary hover:text-primary/80 text-sm font-medium mt-3 transition-colors"
              onClick={onOpenSettings}
            >
              Open Settings
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
