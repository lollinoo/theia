import { useCallback, useState } from 'react';
import { type Device } from '../types/api';
import { DeviceTable } from './dashboard/DeviceTable';
import { SSHCredentialForm } from './dashboard/SSHCredentialForm';
import { BackupPanel } from './dashboard/BackupPanel';
import { BackupHistoryTable } from './dashboard/BackupHistoryTable';
import { BulkBackupPanel } from './dashboard/BulkBackupPanel';
import { ConfigViewer } from './dashboard/ConfigViewer';
import { VendorSettingsPanel } from './dashboard/VendorSettingsPanel';
import { SidePanel } from './SidePanel';

type PanelType =
  | { kind: 'ssh-credentials'; device: Device }
  | { kind: 'backup'; device: Device }
  | { kind: 'backup-history'; device: Device }
  | { kind: 'config-viewer'; device: Device }
  | { kind: 'vendor-settings' }
  | { kind: 'bulk-backup' };

interface DashboardProps {
  devices: Device[];
}

export function Dashboard({ devices }: DashboardProps) {
  const [panel, setPanel] = useState<PanelType | null>(null);

  // Local overrides for ssh_profile_id (survives panel close/reopen until devices prop refreshes)
  const [sshOverrides, setSSHOverrides] = useState<Record<string, string | undefined>>({});

  const applyOverrides = useCallback((device: Device): Device => {
    if (device.id in sshOverrides) {
      return { ...device, ssh_profile_id: sshOverrides[device.id] };
    }
    return device;
  }, [sshOverrides]);

  // Filters
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [typeFilter, setTypeFilter] = useState<string>('all');
  const [search, setSearch] = useState('');

  const filteredDevices = devices.filter((d) => {
    if (statusFilter !== 'all' && d.status !== statusFilter) return false;
    if (typeFilter !== 'all' && d.device_type !== typeFilter) return false;
    if (search) {
      const s = search.toLowerCase();
      const display = d.tags?.display_name || '';
      if (
        !d.hostname.toLowerCase().includes(s) &&
        !d.ip.toLowerCase().includes(s) &&
        !d.sys_name.toLowerCase().includes(s) &&
        !display.toLowerCase().includes(s)
      )
        return false;
    }
    return true;
  });

  const types = [...new Set(devices.map((d) => d.device_type))].sort();

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
    <div className="h-full pt-10 flex flex-col">
      {/* Filter bar */}
      <div className="flex items-center gap-3 px-4 py-3 border-b border-border-subtle bg-bg-surface/50">
        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          className="rounded-md border border-border-subtle bg-bg-elevated px-2 py-1.5 text-xs text-text-primary outline-none focus:border-accent"
        >
          <option value="all">All Status</option>
          <option value="up">Up</option>
          <option value="down">Down</option>
          <option value="probing">Probing</option>
          <option value="unknown">Unknown</option>
        </select>

        <select
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value)}
          className="rounded-md border border-border-subtle bg-bg-elevated px-2 py-1.5 text-xs text-text-primary outline-none focus:border-accent"
        >
          <option value="all">All Types</option>
          {types.map((t) => (
            <option key={t} value={t}>{t}</option>
          ))}
        </select>

        <input
          type="text"
          placeholder="Search devices..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="flex-1 rounded-md border border-border-subtle bg-bg-elevated px-3 py-1.5 text-xs text-text-primary placeholder-text-secondary outline-none focus:border-accent"
        />

        <button
          onClick={() => setPanel({ kind: 'bulk-backup' })}
          className="rounded-md border border-accent/30 bg-accent/10 px-2.5 py-1.5 text-xs text-accent hover:bg-accent/20 transition-colors"
        >
          Backup All
        </button>

        <button
          onClick={() => setPanel({ kind: 'vendor-settings' })}
          className="rounded-md border border-border-subtle bg-bg-elevated px-2.5 py-1.5 text-xs text-text-secondary hover:text-text-primary hover:bg-bg-surface transition-colors"
        >
          Vendor Settings
        </button>

        <span className="text-xs text-text-secondary">
          {filteredDevices.length} / {devices.length} devices
        </span>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto px-4 py-2">
        {devices.length === 0 ? (
          <div className="flex items-center justify-center h-40 text-text-secondary text-sm">
            Loading devices...
          </div>
        ) : (
          <DeviceTable
            devices={filteredDevices}
            onSSHCredentials={(device) => setPanel({ kind: 'ssh-credentials', device: applyOverrides(device) })}
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
            currentProfileId={panel.device.ssh_profile_id}
            onProfileChanged={(profileId) => {
              setSSHOverrides((prev) => ({ ...prev, [panel.device.id]: profileId }));
              setPanel({ kind: 'ssh-credentials', device: { ...panel.device, ssh_profile_id: profileId } });
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
