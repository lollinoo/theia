import { useState } from 'react';
import { type Device } from '../../types/api';
import { DeviceRow } from './DeviceRow';

type SortKey = 'hostname' | 'ip' | 'status' | 'hardware_model';
type SortDir = 'asc' | 'desc';

interface DeviceTableProps {
  devices: Device[];
  onSSHCredentials: (device: Device) => void;
  onBackup: (device: Device) => void;
  onBackupHistory: (device: Device) => void;
  onViewConfig: (device: Device) => void;
}

export function DeviceTable({
  devices,
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
    const aVal = (a[sortKey] ?? '').toString().toLowerCase();
    const bVal = (b[sortKey] ?? '').toString().toLowerCase();
    const cmp = aVal.localeCompare(bVal);
    return sortDir === 'asc' ? cmp : -cmp;
  });

  const columns: { key: SortKey; label: string }[] = [
    { key: 'hostname', label: 'Name' },
    { key: 'ip', label: 'IP Address' },
    { key: 'status', label: 'Status' },
    { key: 'hardware_model', label: 'Model' },
  ];

  if (devices.length === 0) {
    return (
      <div className="flex items-center justify-center h-40 text-text-secondary text-sm">
        No devices found
      </div>
    );
  }

  return (
    <table className="w-full text-xs">
      <thead>
        <tr className="border-b border-border-subtle text-left text-text-secondary">
          {columns.map((col) => (
            <th
              key={col.key}
              className="px-3 py-2 font-medium cursor-pointer select-none hover:text-text-primary transition-colors"
              onClick={() => handleSort(col.key)}
            >
              {col.label}
              {sortKey === col.key && (
                <span className="ml-1">{sortDir === 'asc' ? '\u2191' : '\u2193'}</span>
              )}
            </th>
          ))}
          <th className="px-3 py-2 font-medium text-right">Actions</th>
        </tr>
      </thead>
      <tbody>
        {sorted.map((device) => (
          <DeviceRow
            key={device.id}
            device={device}
            onSSHCredentials={() => onSSHCredentials(device)}
            onBackup={() => onBackup(device)}
            onBackupHistory={() => onBackupHistory(device)}
            onViewConfig={() => onViewConfig(device)}
          />
        ))}
      </tbody>
    </table>
  );
}
