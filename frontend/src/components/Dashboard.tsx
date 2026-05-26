import { useEffect, useMemo, useState } from 'react';
import { deleteDevice, fetchDeviceCredentialProfiles, fetchOrphanDevices } from '../api/client';
import { adaptAreaColor, useTheme } from '../contexts/ThemeContext';
import type { Area, Device } from '../types/api';
import type { SnapshotPayload } from '../types/metrics';
import { MaterialIcon } from './MaterialIcon';
import { SidePanel } from './SidePanel';
import { BackupHistoryTable } from './dashboard/BackupHistoryTable';
import { BackupPanel } from './dashboard/BackupPanel';
import { BulkBackupPanel } from './dashboard/BulkBackupPanel';
import { ConfigViewer } from './dashboard/ConfigViewer';
import { DeviceTable } from './dashboard/DeviceTable';
import { type FilterOption, FilterSelect } from './dashboard/FilterSelect';
import { SSHCredentialForm } from './dashboard/SSHCredentialForm';
import { VendorSettingsPanel } from './dashboard/VendorSettingsPanel';
import { buildRuntimeDeviceRows } from './dashboard/runtimeDeviceRows';

type PanelType =
  | { kind: 'ssh-credentials'; device: Device }
  | { kind: 'backup'; device: Device }
  | { kind: 'backup-history'; device: Device }
  | { kind: 'config-viewer'; device: Device }
  | { kind: 'vendor-settings' }
  | { kind: 'bulk-backup' };

type DeviceInventoryScope = 'all' | 'unassigned';

interface DashboardProps {
  devices: Device[];
  areas: Area[];
  snapshot: SnapshotPayload | null;
  selectedAreaId?: string | null;
  onAreaSelect?: (areaId: string | null) => void;
  onOpenMap?: () => void;
  loading?: boolean;
}

export function Dashboard({
  devices,
  areas,
  snapshot,
  selectedAreaId,
  onAreaSelect,
  onOpenMap,
  loading = devices.length === 0,
}: DashboardProps) {
  const { resolvedTheme } = useTheme();
  const [panel, setPanel] = useState<PanelType | null>(null);
  const [inventoryScope, setInventoryScope] = useState<DeviceInventoryScope>('all');
  const [orphanDevices, setOrphanDevices] = useState<Device[]>([]);
  const [orphanLoading, setOrphanLoading] = useState(false);
  const [orphanError, setOrphanError] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Device | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  // Current credential profile ID for the open ssh-credentials panel.
  // Fetched via fetchDeviceCredentialProfiles when the panel opens (Option A: live source of truth
  // after ssh_profile_id removal — avoids stale field dependency).
  const [sshPanelProfileId, setSSHPanelProfileId] = useState<string | undefined>(undefined);

  // Filters
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [typeFilter, setTypeFilter] = useState<string>('all');
  const [areaFilter, setAreaFilter] = useState<string>('all');
  const [search, setSearch] = useState('');

  useEffect(() => {
    if (inventoryScope !== 'unassigned') {
      return;
    }

    let cancelled = false;
    setOrphanLoading(true);
    setOrphanError(null);
    void fetchOrphanDevices()
      .then((devicesData) => {
        if (!cancelled) {
          setOrphanDevices(devicesData);
        }
      })
      .catch((error) => {
        if (!cancelled) {
          const message =
            error instanceof Error ? error.message : 'Failed to load unassigned devices';
          setOrphanError(message);
          setOrphanDevices([]);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setOrphanLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [inventoryScope]);

  useEffect(() => {
    if (selectedAreaId === undefined) {
      return;
    }
    setAreaFilter(selectedAreaId ?? 'all');
  }, [selectedAreaId]);

  const handleAreaFilterChange = (value: string) => {
    setAreaFilter(value);
    if (onAreaSelect && value !== 'unassigned') {
      onAreaSelect(value === 'all' ? null : value);
    }
  };

  const clearFilters = () => {
    setStatusFilter('all');
    setTypeFilter('all');
    handleAreaFilterChange('all');
    setSearch('');
  };

  const areaMap = useMemo(() => {
    const map = new Map<string, Area>();
    for (const a of areas) map.set(a.id, a);
    return map;
  }, [areas]);

  const activeDevices = inventoryScope === 'unassigned' ? orphanDevices : devices;
  const activeSnapshot = inventoryScope === 'unassigned' ? null : snapshot;
  const activeLoading = inventoryScope === 'unassigned' ? orphanLoading : loading;

  const rows = useMemo(
    () => buildRuntimeDeviceRows({ devices: activeDevices, snapshot: activeSnapshot }),
    [activeDevices, activeSnapshot],
  );

  const rowsWithAreaSortNames = useMemo(
    () =>
      rows.map((row) => ({
        ...row,
        areaSortName: row.areaIds[0] ? (areaMap.get(row.areaIds[0])?.name ?? '') : '',
      })),
    [areaMap, rows],
  );

  const filteredRows = rowsWithAreaSortNames.filter((row) => {
    if (statusFilter !== 'all' && row.statusState.dotStatus !== statusFilter) return false;
    if (typeFilter !== 'all' && row.deviceType !== typeFilter) return false;
    if (inventoryScope === 'all' && areaFilter !== 'all') {
      if (areaFilter === 'unassigned') {
        if (row.areaIds.length) return false;
      } else {
        if (!row.areaIds.includes(areaFilter)) return false;
      }
    }
    if (search) {
      const s = search.toLowerCase();
      if (!row.searchText.includes(s)) return false;
    }
    return true;
  });

  const types = [...new Set(activeDevices.map((d) => d.device_type))].sort();

  const statusOptions: FilterOption[] = [
    { value: 'all', label: 'All' },
    { value: 'up', label: 'Up' },
    { value: 'down', label: 'Down' },
    { value: 'probing', label: 'Probing' },
    { value: 'unknown', label: 'Unknown' },
    { value: 'unmonitored', label: 'Unmonitored' },
  ];

  const typeOptions: FilterOption[] = [
    { value: 'all', label: 'All' },
    ...types.map((t) => ({ value: t, label: t })),
  ];

  const areaOptions: FilterOption[] = [
    { value: 'all', label: 'All' },
    ...areas.map((a) => ({
      value: a.id,
      label: a.name,
      color: adaptAreaColor(a.color, resolvedTheme),
    })),
    { value: 'unassigned', label: 'Unassigned' },
  ];

  async function handleConfirmPermanentDelete() {
    if (!deleteTarget) {
      return;
    }

    setDeleteLoading(true);
    setDeleteError(null);
    try {
      await deleteDevice(deleteTarget.id);
      const deletedID = deleteTarget.id;
      setOrphanDevices((current) => current.filter((device) => device.id !== deletedID));
      setDeleteTarget(null);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to delete device';
      setDeleteError(message);
    } finally {
      setDeleteLoading(false);
    }
  }

  const panelTitle = panel
    ? panel.kind === 'ssh-credentials'
      ? 'SSH Credentials'
      : panel.kind === 'backup'
        ? 'Backup'
        : panel.kind === 'backup-history'
          ? 'Backup History'
          : panel.kind === 'bulk-backup'
            ? 'Backup All Devices'
            : panel.kind === 'vendor-settings'
              ? 'Vendor Settings'
              : 'Configuration'
    : '';

  return (
    <div className="flex h-full flex-col pt-32 transition-colors duration-200 sm:pt-[86px]">
      {/* Filter bar */}
      <div className="flex flex-wrap items-center gap-3 px-4 py-3 bg-surface/50 transition-colors duration-200">
        <div className="inline-flex rounded-md bg-surface-high p-0.5 text-xs">
          <button
            type="button"
            aria-label="Show all devices"
            aria-pressed={inventoryScope === 'all'}
            onClick={() => setInventoryScope('all')}
            className={`rounded px-2.5 py-1.5 transition-colors ${
              inventoryScope === 'all'
                ? 'bg-primary text-on-primary'
                : 'text-on-bg-secondary hover:text-on-bg'
            }`}
          >
            All
          </button>
          <button
            type="button"
            aria-label="Show unassigned devices"
            aria-pressed={inventoryScope === 'unassigned'}
            onClick={() => setInventoryScope('unassigned')}
            className={`rounded px-2.5 py-1.5 transition-colors ${
              inventoryScope === 'unassigned'
                ? 'bg-primary text-on-primary'
                : 'text-on-bg-secondary hover:text-on-bg'
            }`}
          >
            Unassigned
          </button>
        </div>
        <FilterSelect
          value={statusFilter}
          onChange={setStatusFilter}
          options={statusOptions}
          label="Status"
        />
        <FilterSelect
          value={typeFilter}
          onChange={setTypeFilter}
          options={typeOptions}
          label="Type"
        />
        <FilterSelect
          value={areaFilter}
          onChange={handleAreaFilterChange}
          options={areaOptions}
          label="Area"
        />

        <div
          data-testid="devices-search-field"
          className="relative min-w-[min(26rem,100%)] flex-[2_1_24rem]"
        >
          <MaterialIcon
            name="search"
            size={14}
            className="absolute left-2.5 top-1/2 -translate-y-1/2 text-on-bg-muted"
          />
          <input
            type="text"
            placeholder="Search devices..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full rounded-md bg-surface-high pl-8 pr-3 py-1.5 text-xs text-on-bg placeholder-on-bg-muted outline-none focus:ring-1 focus:ring-primary/30 transition-colors"
          />
        </div>

        <button
          type="button"
          onClick={onOpenMap}
          className="flex items-center gap-1.5 rounded-md bg-surface-high px-2.5 py-1.5 text-xs text-on-bg-secondary hover:bg-elevated hover:text-on-bg transition-colors"
        >
          <MaterialIcon name="map" size={14} />
          Open map
        </button>

        <button
          type="button"
          onClick={() => setPanel({ kind: 'bulk-backup' })}
          className="flex items-center gap-1.5 rounded-md bg-primary/10 px-2.5 py-1.5 text-xs text-primary hover:bg-primary/20 transition-colors"
        >
          <MaterialIcon name="backup" size={14} />
          Backup All
        </button>

        <button
          type="button"
          onClick={() => setPanel({ kind: 'vendor-settings' })}
          className="rounded-md bg-surface-high px-2.5 py-1.5 text-xs text-on-bg-secondary hover:text-on-bg hover:bg-elevated transition-colors"
        >
          Vendor Settings
        </button>

        <span className="font-mono text-xs text-on-bg-secondary bg-surface-high rounded-full px-2.5 py-1">
          {filteredRows.length} / {activeDevices.length}
        </span>
        {orphanError && inventoryScope === 'unassigned' && (
          <span className="text-xs text-status-down">{orphanError}</span>
        )}
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto px-4 py-2">
        {activeDevices.length === 0 ? (
          activeLoading ? (
            /* D-16: Skeleton loading rows */
            <SkeletonTable />
          ) : inventoryScope === 'unassigned' ? (
            <EmptyUnassignedState />
          ) : (
            <EmptyMapState />
          )
        ) : filteredRows.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-40 gap-2">
            <p className="text-on-bg-secondary text-sm">No devices match your filters</p>
            <button
              type="button"
              onClick={clearFilters}
              className="text-primary hover:text-primary/80 text-xs font-medium transition-colors"
            >
              Clear filters
            </button>
          </div>
        ) : (
          <div data-testid="dashboard-table">
            <DeviceTable
              rows={filteredRows}
              areaMap={areaMap}
              resolvedTheme={resolvedTheme}
              onSSHCredentials={(device) => {
                // Fetch current credential profile assignment when opening the panel
                // (Option A: live source of truth after ssh_profile_id removal)
                setSSHPanelProfileId(undefined);
                setPanel({ kind: 'ssh-credentials', device });
                const targetDeviceId = device.id;
                void fetchDeviceCredentialProfiles(device.id)
                  .then((profiles) => {
                    // Guard against stale fetch: only apply if the panel is still open
                    // for the same device (prevents race when user switches devices quickly).
                    setPanel((current) => {
                      if (
                        current?.kind === 'ssh-credentials' &&
                        current.device.id === targetDeviceId
                      ) {
                        const nonWinbox = profiles.find((p) => !p.is_winbox);
                        setSSHPanelProfileId((nonWinbox ?? profiles[0])?.profile_id);
                      }
                      return current;
                    });
                  })
                  .catch(() => {
                    /* non-fatal — panel starts with no selection */
                  });
              }}
              onBackup={(device) => setPanel({ kind: 'backup', device })}
              onBackupHistory={(device) => setPanel({ kind: 'backup-history', device })}
              onViewConfig={(device) => setPanel({ kind: 'config-viewer', device })}
              onDeletePermanently={
                inventoryScope === 'unassigned'
                  ? (device) => {
                      setDeleteError(null);
                      setDeleteTarget(device);
                    }
                  : undefined
              }
            />
          </div>
        )}
      </div>

      {/* Side panel */}
      <SidePanel open={panel !== null} onClose={() => setPanel(null)} title={panelTitle}>
        {panel?.kind === 'ssh-credentials' && (
          <SSHCredentialForm
            deviceId={panel.device.id}
            currentProfileId={sshPanelProfileId}
            onProfileChanged={(profileId) => {
              // Update local panel profile state so Save/Test reflect the new assignment
              setSSHPanelProfileId(profileId);
            }}
          />
        )}
        {panel?.kind === 'backup' && <BackupPanel device={panel.device} />}
        {panel?.kind === 'backup-history' && (
          <BackupHistoryTable
            deviceId={panel.device.id}
            onViewConfig={() => setPanel({ kind: 'config-viewer', device: panel.device })}
          />
        )}
        {panel?.kind === 'config-viewer' && <ConfigViewer deviceId={panel.device.id} />}
        {panel?.kind === 'bulk-backup' && <BulkBackupPanel devices={activeDevices} />}
        {panel?.kind === 'vendor-settings' && <VendorSettingsPanel />}
      </SidePanel>

      <DeleteOrphanDeviceDialog
        device={deleteTarget}
        deleting={deleteLoading}
        error={deleteError}
        onClose={() => {
          if (!deleteLoading) {
            setDeleteTarget(null);
            setDeleteError(null);
          }
        }}
        onDelete={() => {
          void handleConfirmPermanentDelete();
        }}
      />
    </div>
  );
}

function SkeletonTable() {
  return (
    <table className="w-full text-xs">
      <thead className="sticky top-0 z-10 bg-bg">
        <tr className="text-left text-on-bg-secondary">
          {[
            'Name',
            'IP Address',
            'Status',
            'Area',
            'Vendor',
            'Model',
            'OS Version',
            'Uptime',
            'Actions',
          ].map((h) => (
            <th key={h} className="px-3 py-2 text-[12px] font-normal uppercase tracking-[0.16em]">
              {h}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {[0, 1, 2, 3, 4, 5, 6, 7].map((rowNumber) => (
          <tr key={rowNumber} className={rowNumber % 2 === 0 ? '' : 'bg-surface-high/30'}>
            <td className="px-3 py-2.5">
              <div className="h-4 w-28 bg-surface-high rounded animate-pulse" />
            </td>
            <td className="px-3 py-2.5">
              <div className="h-4 w-24 bg-surface-high rounded animate-pulse" />
            </td>
            <td className="px-3 py-2.5">
              <div className="h-4 w-14 bg-surface-high rounded animate-pulse" />
            </td>
            <td className="px-3 py-2.5">
              <div className="h-4 w-20 bg-surface-high rounded animate-pulse" />
            </td>
            <td className="px-3 py-2.5">
              <div className="h-4 w-20 bg-surface-high rounded animate-pulse" />
            </td>
            <td className="px-3 py-2.5">
              <div className="h-4 w-8 bg-surface-high rounded animate-pulse" />
            </td>
            <td className="px-3 py-2.5">
              <div className="h-4 w-14 bg-surface-high rounded animate-pulse" />
            </td>
            <td className="px-3 py-2.5">
              <div className="h-4 w-20 bg-surface-high rounded animate-pulse" />
            </td>
            <td className="px-3 py-2.5">
              <div className="h-4 w-24 bg-surface-high rounded animate-pulse" />
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function EmptyMapState() {
  return (
    <div className="flex h-40 flex-col items-center justify-center gap-2">
      <p className="text-sm text-on-bg-secondary">No devices in this map</p>
      <p className="text-xs text-on-bg-muted">Add devices from the topology canvas.</p>
    </div>
  );
}

function EmptyUnassignedState() {
  return (
    <div className="flex h-40 flex-col items-center justify-center gap-2">
      <p className="text-sm text-on-bg-secondary">No unassigned devices</p>
      <p className="text-xs text-on-bg-muted">Every device is present in at least one saved map.</p>
    </div>
  );
}

function DeleteOrphanDeviceDialog({
  device,
  deleting,
  error,
  onClose,
  onDelete,
}: {
  device: Device | null;
  deleting: boolean;
  error: string | null;
  onClose: () => void;
  onDelete: () => void;
}) {
  if (!device) {
    return null;
  }

  const deviceLabel = device.hostname || device.ip || device.id;

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/40 px-4">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="delete-orphan-device-title"
        className="w-full max-w-md rounded-lg border border-outline bg-surface p-5 shadow-panel"
      >
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 id="delete-orphan-device-title" className="text-base font-semibold text-on-bg">
              Delete device permanently
            </h2>
            <p className="mt-1 text-sm text-on-bg-secondary">{deviceLabel}</p>
          </div>
          <button
            type="button"
            aria-label="Close delete device dialog"
            onClick={onClose}
            disabled={deleting}
            className="rounded-full p-1.5 text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg disabled:pointer-events-none disabled:opacity-50"
          >
            <MaterialIcon name="close" size={18} />
          </button>
        </div>

        <p className="mt-5 text-sm text-on-bg-secondary">
          This permanently deletes {deviceLabel} from the global inventory and cannot be undone. The
          device is not present in any saved map.
        </p>
        {error && <p className="mt-3 text-sm text-status-down">{error}</p>}

        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            disabled={deleting}
            className="rounded-lg border border-outline px-3 py-2 text-sm font-medium text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg disabled:pointer-events-none disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={onDelete}
            disabled={deleting}
            className="rounded-lg bg-status-down px-3 py-2 text-sm font-semibold text-white transition-colors hover:bg-status-down/90 disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
          >
            {deleting ? 'Deleting...' : 'Delete permanently'}
          </button>
        </div>
      </div>
    </div>
  );
}
