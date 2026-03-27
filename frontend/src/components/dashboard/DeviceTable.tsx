import { useState } from 'react';
import type { Device, Area } from '../../types/api';
import type { ResolvedTheme } from '../../contexts/ThemeContext';
import type { SnapshotPayload } from '../../types/metrics';
import { DeviceRow } from './DeviceRow';

type SortKey = 'hostname' | 'ip' | 'status' | 'area' | 'hardware_model' | 'vendor' | 'uptime' | 'os_version';
type SortDir = 'asc' | 'desc';

interface DeviceTableProps {
  devices: Device[];
  areaMap: Map<string, Area>;
  resolvedTheme: ResolvedTheme;
  snapshot: SnapshotPayload | null;
  onSSHCredentials: (device: Device) => void;
  onBackup: (device: Device) => void;
  onBackupHistory: (device: Device) => void;
  onViewConfig: (device: Device) => void;
}

function parseOsVersion(sysDescr: string): string {
  if (!sysDescr) return '';
  // Match patterns like "RouterOS 7.14.3", "Version 6.49.10", "IOS-XE 17.3.4"
  const match = sysDescr.match(/(?:RouterOS|Version|IOS(?:-XE)?|JunOS|EOS)\s*([\d.]+\S*)/i);
  return match ? match[0] : '';
}

export function DeviceTable({
  devices,
  areaMap,
  resolvedTheme,
  snapshot,
  onSSHCredentials,
  onBackup,
  onBackupHistory,
  onViewConfig,
}: DeviceTableProps) {
  const [sortKey, setSortKey] = useState<SortKey>('hostname');
  const [sortDir, setSortDir] = useState<SortDir>('asc');

  const handleSort = (key: SortKey) => {
    if (sortKey === key) {
      setSortDir(sortDir === 'asc' ? 'desc' : 'asc');
    } else {
      setSortKey(key);
      setSortDir('asc');
    }
  };

  const sorted = [...devices].sort((a, b) => {
    let aVal: string | number;
    let bVal: string | number;
    if (sortKey === 'area') {
      aVal = (a.area_id ? areaMap.get(a.area_id)?.name : '') ?? '';
      bVal = (b.area_id ? areaMap.get(b.area_id)?.name : '') ?? '';
    } else if (sortKey === 'uptime') {
      // Numeric sort by uptime seconds (null = -1 so they sort last)
      aVal = snapshot?.device_metrics[a.id]?.uptime_secs ?? -1;
      bVal = snapshot?.device_metrics[b.id]?.uptime_secs ?? -1;
      const cmp = (aVal as number) - (bVal as number);
      return sortDir === 'asc' ? cmp : -cmp;
    } else if (sortKey === 'os_version') {
      aVal = parseOsVersion(a.sys_descr);
      bVal = parseOsVersion(b.sys_descr);
    } else {
      aVal = (a[sortKey] ?? '').toString().toLowerCase();
      bVal = (b[sortKey] ?? '').toString().toLowerCase();
    }
    const cmp = String(aVal).localeCompare(String(bVal));
    return sortDir === 'asc' ? cmp : -cmp;
  });

  const columns: { key: SortKey; label: string; className?: string }[] = [
    { key: 'hostname', label: 'Name', className: 'sticky left-0 z-[5] bg-bg' },
    { key: 'ip', label: 'IP Address' },
    { key: 'status', label: 'Status' },
    { key: 'area', label: 'Area' },
    { key: 'vendor', label: 'Vendor' },
    { key: 'hardware_model', label: 'Model' },
    { key: 'os_version', label: 'OS Version' },
    { key: 'uptime', label: 'Uptime' },
  ];

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-xs min-w-[900px]">
        <thead className="sticky top-0 z-10 bg-bg">
          <tr className="text-left text-on-bg-secondary">
            {columns.map((col) => (
              <th
                key={col.key}
                className={`px-3 py-2 text-[12px] font-normal uppercase tracking-[0.16em] cursor-pointer select-none hover:text-on-bg transition-colors whitespace-nowrap ${col.className ?? ''}`}
                onClick={() => handleSort(col.key)}
              >
                {col.label}
                {sortKey === col.key && (
                  <span className="ml-1 text-primary">{sortDir === 'asc' ? '\u2191' : '\u2193'}</span>
                )}
              </th>
            ))}
            <th className="px-3 py-2 text-[12px] font-normal uppercase tracking-[0.16em] text-right">Actions</th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((device) => (
            <DeviceRow
              key={device.id}
              device={device}
              areaMap={areaMap}
              resolvedTheme={resolvedTheme}
              deviceMetrics={snapshot?.device_metrics[device.id] ?? null}
              onSSHCredentials={() => onSSHCredentials(device)}
              onBackup={() => onBackup(device)}
              onBackupHistory={() => onBackupHistory(device)}
              onViewConfig={() => onViewConfig(device)}
            />
          ))}
        </tbody>
      </table>
    </div>
  );
}
