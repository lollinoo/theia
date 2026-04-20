import { useMemo, useState } from 'react';
import type { Device, Area } from '../types/api';
import type { SnapshotPayload } from '../types/metrics';
import { DeviceTable } from './dashboard/DeviceTable';
import { fetchDeviceCredentialProfiles } from '../api/client';
import { FilterSelect, type FilterOption } from './dashboard/FilterSelect';
import { MaterialIcon } from './MaterialIcon';
import { useTheme, adaptAreaColor } from '../contexts/ThemeContext';
import { SSHCredentialForm } from './dashboard/SSHCredentialForm';
import { BackupPanel } from './dashboard/BackupPanel';
import { BackupHistoryTable } from './dashboard/BackupHistoryTable';
import { BulkBackupPanel } from './dashboard/BulkBackupPanel';
import { ConfigViewer } from './dashboard/ConfigViewer';
import { VendorSettingsPanel } from './dashboard/VendorSettingsPanel';
import { SidePanel } from './SidePanel';
import { buildRuntimeDeviceRows } from './dashboard/runtimeDeviceRows';

type PanelType =
  | { kind: 'ssh-credentials'; device: Device }
  | { kind: 'backup'; device: Device }
  | { kind: 'backup-history'; device: Device }
  | { kind: 'config-viewer'; device: Device }
  | { kind: 'vendor-settings' }
  | { kind: 'bulk-backup' };

interface DashboardProps {
  devices: Device[];
  areas: Area[];
  snapshot: SnapshotPayload | null;
}

export function Dashboard({ devices, areas, snapshot }: DashboardProps) {
  const { resolvedTheme } = useTheme();
  const [panel, setPanel] = useState<PanelType | null>(null);

  // Current credential profile ID for the open ssh-credentials panel.
  // Fetched via fetchDeviceCredentialProfiles when the panel opens (Option A: live source of truth
  // after ssh_profile_id removal — avoids stale field dependency).
  const [sshPanelProfileId, setSSHPanelProfileId] = useState<string | undefined>(undefined);

  // Filters
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [typeFilter, setTypeFilter] = useState<string>('all');
  const [areaFilter, setAreaFilter] = useState<string>('all');
  const [search, setSearch] = useState('');

  const areaMap = useMemo(() => {
    const map = new Map<string, Area>();
    for (const a of areas) map.set(a.id, a);
    return map;
  }, [areas]);

  const rows = useMemo(
    () => buildRuntimeDeviceRows({ devices, snapshot }),
    [devices, snapshot],
  );

  const rowsWithAreaSortNames = useMemo(
    () => rows.map((row) => ({
      ...row,
      areaSortName: row.areaIds[0] ? areaMap.get(row.areaIds[0])?.name ?? '' : '',
    })),
    [areaMap, rows],
  );

  const filteredRows = rowsWithAreaSortNames.filter((row) => {
    if (statusFilter !== 'all' && row.statusState.dotStatus !== statusFilter) return false;
    if (typeFilter !== 'all' && row.deviceType !== typeFilter) return false;
    if (areaFilter !== 'all') {
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

  const types = [...new Set(devices.map((d) => d.device_type))].sort();

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
    ...types.map(t => ({ value: t, label: t })),
  ];

  const areaOptions: FilterOption[] = [
    { value: 'all', label: 'All' },
    ...areas.map(a => ({
      value: a.id,
      label: a.name,
      color: adaptAreaColor(a.color, resolvedTheme),
    })),
    { value: 'unassigned', label: 'Unassigned' },
  ];

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
    <div className="h-full pt-10 flex flex-col transition-colors duration-200">
      {/* Filter bar */}
      <div className="flex items-center gap-3 px-4 py-3 bg-surface/50 transition-colors duration-200">
        <FilterSelect value={statusFilter} onChange={setStatusFilter} options={statusOptions} label="Status" />
        <FilterSelect value={typeFilter} onChange={setTypeFilter} options={typeOptions} label="Type" />
        <FilterSelect value={areaFilter} onChange={setAreaFilter} options={areaOptions} label="Area" />

        <div className="relative flex-1">
          <MaterialIcon name="search" size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-on-bg-muted" />
          <input
            type="text"
            placeholder="Search devices..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full rounded-md bg-surface-high pl-8 pr-3 py-1.5 text-xs text-on-bg placeholder-on-bg-muted outline-none focus:ring-1 focus:ring-primary/30 transition-colors"
          />
        </div>

        <button
          onClick={() => setPanel({ kind: 'bulk-backup' })}
          className="flex items-center gap-1.5 rounded-md bg-primary/10 px-2.5 py-1.5 text-xs text-primary hover:bg-primary/20 transition-colors"
        >
          <MaterialIcon name="backup" size={14} />
          Backup All
        </button>

        <button
          onClick={() => setPanel({ kind: 'vendor-settings' })}
          className="rounded-md bg-surface-high px-2.5 py-1.5 text-xs text-on-bg-secondary hover:text-on-bg hover:bg-elevated transition-colors"
        >
          Vendor Settings
        </button>

        <span className="font-mono text-xs text-on-bg-secondary bg-surface-high rounded-full px-2.5 py-1">
          {filteredRows.length} / {devices.length}
        </span>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto px-4 py-2">
        {devices.length === 0 ? (
          /* D-16: Skeleton loading rows */
          <SkeletonTable />
        ) : filteredRows.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-40 gap-2">
            <p className="text-on-bg-secondary text-sm">No devices match your filters</p>
            <button
              type="button"
              onClick={() => { setStatusFilter('all'); setTypeFilter('all'); setAreaFilter('all'); setSearch(''); }}
              className="text-primary hover:text-primary/80 text-xs font-medium transition-colors"
            >
              Clear filters
            </button>
          </div>
        ) : (
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
              void fetchDeviceCredentialProfiles(device.id).then((profiles) => {
                // Guard against stale fetch: only apply if the panel is still open
                // for the same device (prevents race when user switches devices quickly).
                setPanel((current) => {
                  if (current?.kind === 'ssh-credentials' && current.device.id === targetDeviceId) {
                    // Use first non-WinBox profile as the "current" SSH profile, matching
                    // GetBackupProfileForDevice ordering (is_winbox ASC).
                    const nonWinbox = profiles.find((p) => !p.is_winbox);
                    setSSHPanelProfileId(nonWinbox?.profile_id);
                  }
                  return current;
                });
              }).catch(() => {/* non-fatal — panel starts with no selection */});
            }}
            onBackup={(device) => setPanel({ kind: 'backup', device })}
            onBackupHistory={(device) => setPanel({ kind: 'backup-history', device })}
            onViewConfig={(device) => setPanel({ kind: 'config-viewer', device })}
          />
        )}
      </div>

      {/* Side panel */}
      <SidePanel
        open={panel !== null}
        onClose={() => setPanel(null)}
        title={panelTitle}
      >
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
        {panel?.kind === 'backup' && (
          <BackupPanel device={panel.device} />
        )}
        {panel?.kind === 'backup-history' && (
          <BackupHistoryTable
            deviceId={panel.device.id}
            onViewConfig={() => setPanel({ kind: 'config-viewer', device: panel.device })}
          />
        )}
        {panel?.kind === 'config-viewer' && (
          <ConfigViewer deviceId={panel.device.id} />
        )}
        {panel?.kind === 'bulk-backup' && (
          <BulkBackupPanel devices={devices} />
        )}
        {panel?.kind === 'vendor-settings' && (
          <VendorSettingsPanel />
        )}
      </SidePanel>
    </div>
  );
}

function SkeletonTable() {
  return (
    <table className="w-full text-xs">
      <thead className="sticky top-0 z-10 bg-bg">
        <tr className="text-left text-on-bg-secondary">
          {['Name', 'IP Address', 'Status', 'Area', 'Vendor', 'Model', 'OS Version', 'Uptime', 'Actions'].map(h => (
            <th key={h} className="px-3 py-2 text-[12px] font-normal uppercase tracking-[0.16em]">{h}</th>
          ))}
        </tr>
      </thead>
      <tbody>
        {Array.from({ length: 8 }).map((_, i) => (
          <tr key={i} className={i % 2 === 0 ? '' : 'bg-surface-high/30'}>
            <td className="px-3 py-2.5"><div className="h-4 w-28 bg-surface-high rounded animate-pulse" /></td>
            <td className="px-3 py-2.5"><div className="h-4 w-24 bg-surface-high rounded animate-pulse" /></td>
            <td className="px-3 py-2.5"><div className="h-4 w-14 bg-surface-high rounded animate-pulse" /></td>
            <td className="px-3 py-2.5"><div className="h-4 w-20 bg-surface-high rounded animate-pulse" /></td>
            <td className="px-3 py-2.5"><div className="h-4 w-20 bg-surface-high rounded animate-pulse" /></td>
            <td className="px-3 py-2.5"><div className="h-4 w-8 bg-surface-high rounded animate-pulse" /></td>
            <td className="px-3 py-2.5"><div className="h-4 w-14 bg-surface-high rounded animate-pulse" /></td>
            <td className="px-3 py-2.5"><div className="h-4 w-20 bg-surface-high rounded animate-pulse" /></td>
            <td className="px-3 py-2.5"><div className="h-4 w-24 bg-surface-high rounded animate-pulse" /></td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
