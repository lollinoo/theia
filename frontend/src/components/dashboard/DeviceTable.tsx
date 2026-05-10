import { useState } from 'react';
import type { ResolvedTheme } from '../../contexts/ThemeContext';
import type { Area, Device } from '../../types/api';
import { DeviceRow } from './DeviceRow';
import type { RuntimeDeviceRow } from './runtimeDeviceRows';

type SortKey =
  | 'hostname'
  | 'ip'
  | 'status'
  | 'area'
  | 'hardware_model'
  | 'vendor'
  | 'uptime'
  | 'os_version';
type SortDir = 'asc' | 'desc';

interface DeviceTableProps {
  rows: RuntimeDeviceRow[];
  areaMap: Map<string, Area>;
  resolvedTheme: ResolvedTheme;
  onSSHCredentials: (device: Device) => void;
  onBackup: (device: Device) => void;
  onBackupHistory: (device: Device) => void;
  onViewConfig: (device: Device) => void;
  onDeletePermanently?: (device: Device) => void;
}

export function DeviceTable({
  rows,
  areaMap,
  resolvedTheme,
  onSSHCredentials,
  onBackup,
  onBackupHistory,
  onViewConfig,
  onDeletePermanently,
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

  const sorted = [...rows].sort((a, b) => {
    let aVal: string | number;
    let bVal: string | number;
    if (sortKey === 'area') {
      aVal = a.areaSortName;
      bVal = b.areaSortName;
    } else if (sortKey === 'status') {
      aVal = a.statusSortLabel;
      bVal = b.statusSortLabel;
    } else if (sortKey === 'uptime') {
      aVal = a.uptimeSecs ?? -1;
      bVal = b.uptimeSecs ?? -1;
      const cmp = (aVal as number) - (bVal as number);
      return sortDir === 'asc' ? cmp : -cmp;
    } else if (sortKey === 'os_version') {
      aVal = a.osVersion;
      bVal = b.osVersion;
    } else if (sortKey === 'hostname') {
      aVal = a.hostname.toLowerCase();
      bVal = b.hostname.toLowerCase();
    } else if (sortKey === 'ip') {
      aVal = a.ip.toLowerCase();
      bVal = b.ip.toLowerCase();
    } else if (sortKey === 'vendor') {
      aVal = a.vendor.toLowerCase();
      bVal = b.vendor.toLowerCase();
    } else if (sortKey === 'hardware_model') {
      aVal = a.modelLabel.toLowerCase();
      bVal = b.modelLabel.toLowerCase();
    } else {
      aVal = '';
      bVal = '';
    }
    const cmp = String(aVal).localeCompare(String(bVal));
    return sortDir === 'asc' ? cmp : -cmp;
  });

  const columns: { key: SortKey; label: string; className?: string }[] = [
    { key: 'hostname', label: 'Name' },
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
                className={`px-3 py-2 text-[12px] font-normal uppercase tracking-[0.16em] cursor-pointer whitespace-nowrap ${col.className ?? ''}`}
              >
                <button
                  type="button"
                  className="cursor-pointer select-none transition-colors hover:text-on-bg"
                  onClick={() => handleSort(col.key)}
                >
                  {col.label}
                  {sortKey === col.key && (
                    <span className="ml-1 text-primary">
                      {sortDir === 'asc' ? '\u2191' : '\u2193'}
                    </span>
                  )}
                </button>
              </th>
            ))}
            <th className="px-3 py-2 text-[12px] font-normal uppercase tracking-[0.16em] text-right">
              Actions
            </th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((row) => (
            <DeviceRow
              key={row.id}
              row={row}
              areaMap={areaMap}
              resolvedTheme={resolvedTheme}
              onSSHCredentials={() => onSSHCredentials(row.device)}
              onBackup={() => onBackup(row.device)}
              onBackupHistory={() => onBackupHistory(row.device)}
              onViewConfig={() => onViewConfig(row.device)}
              onDeletePermanently={
                onDeletePermanently ? () => onDeletePermanently(row.device) : undefined
              }
            />
          ))}
        </tbody>
      </table>
    </div>
  );
}
