import type { Area } from '../../types/api';
import type { ResolvedTheme } from '../../contexts/ThemeContext';
import { adaptAreaColor } from '../../contexts/ThemeContext';
import { StatusDot } from '../StatusDot';
import { MaterialIcon } from '../MaterialIcon';
import type { RuntimeDeviceRow } from './runtimeDeviceRows';


interface DeviceRowProps {
  row: RuntimeDeviceRow;
  areaMap: Map<string, Area>;
  resolvedTheme: ResolvedTheme;
  onSSHCredentials: () => void;
  onBackup: () => void;
  onBackupHistory: () => void;
  onViewConfig: () => void;
}

export function DeviceRow({
  row, areaMap, resolvedTheme,
  onSSHCredentials, onBackup, onBackupHistory, onViewConfig,
}: DeviceRowProps) {
  const deviceAreas = row.areaIds.map((id) => areaMap.get(id)).filter((a): a is Area => !!a);

  return (
    <tr className="[&:nth-child(even)]:bg-surface-high/30 hover:bg-elevated/50 transition-colors duration-150">
      {/* Name -- sticky first column per D-20 */}
      <td className="px-3 py-2.5 sticky left-0 z-[4] bg-inherit">
        <div className="font-medium text-on-bg">{row.displayName}</div>
        {row.sysName && row.sysName !== row.displayName && (
          <div className="text-on-bg-secondary text-[11px] mt-0.5">{row.sysName}</div>
        )}
      </td>
      {/* IP Address -- monospace per design spec */}
      <td className="px-3 py-2.5 font-mono text-[11px] font-semibold text-on-bg-secondary whitespace-nowrap">{row.ip}</td>
      {/* Status -- StatusDot component per D-02 */}
      <td className="px-3 py-2.5">
        <div className="flex items-center gap-1.5">
          <StatusDot status={row.statusState.dotStatus} />
          <span className="text-on-bg-secondary text-[11px]">{row.statusState.label}</span>
        </div>
      </td>
      {/* Area -- color dot(s) + name per D-02/D-11 */}
      <td className="px-3 py-2.5">
        {deviceAreas.length > 0 ? (
          <div className="flex items-center gap-1.5 flex-wrap">
            {deviceAreas.map((area) => (
              <span key={area.id} className="inline-flex items-center gap-1">
                <span
                  className="w-2 h-2 rounded-full flex-shrink-0"
                  style={{ backgroundColor: adaptAreaColor(area.color, resolvedTheme) }}
                />
                <span className="text-on-bg-secondary text-[11px]">{area.name}</span>
              </span>
            ))}
          </div>
        ) : (
          <span className="text-on-bg-muted text-[11px]">{'\u2014'}</span>
        )}
      </td>
      {/* Vendor */}
      <td className="px-3 py-2.5 text-on-bg-secondary text-[11px] capitalize">
        {row.vendor || '\u2014'}
      </td>
      {/* Model */}
      <td className="px-3 py-2.5 text-on-bg-secondary text-[11px] font-mono">
        {row.modelLabel
          ? row.modelLabel.length > 30 ? `${row.modelLabel.slice(0, 29)}\u2026` : row.modelLabel
          : '\u2014'}
      </td>
      {/* OS Version -- parsed from sys_descr, font-mono per D-02 */}
      <td className="px-3 py-2.5 font-mono text-[11px] text-on-bg-secondary whitespace-nowrap">
        {row.osVersion || '\u2014'}
      </td>
      {/* Uptime -- live from WebSocket snapshot, font-mono per D-02 */}
      <td className="px-3 py-2.5 font-mono text-[11px] text-on-bg-secondary whitespace-nowrap">
        {row.uptimeLabel || '\u2014'}
      </td>
      {/* Actions -- icon buttons per D-08; virtual nodes have no SSH/backup */}
      <td className="px-3 py-2.5">
        {row.deviceType !== 'virtual' && (
          <div className="flex items-center justify-end gap-0.5">
            <IconAction icon="terminal" title="SSH Credentials" onClick={onSSHCredentials} />
            <IconAction icon="backup" title="Backup Now" onClick={onBackup} />
            <IconAction icon="history" title="Backup History" onClick={onBackupHistory} />
            <IconAction icon="description" title="View Config" onClick={onViewConfig} />
          </div>
        )}
      </td>
    </tr>
  );
}

function IconAction({ icon, title, onClick, disabled }: {
  icon: string; title: string; onClick: () => void; disabled?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={(e) => { e.stopPropagation(); if (!disabled) onClick(); }}
      title={title}
      disabled={disabled}
      className={`p-1.5 rounded-md transition-colors ${
        disabled
          ? 'text-on-bg-muted cursor-not-allowed opacity-40'
          : 'text-on-bg-secondary hover:text-on-bg hover:bg-surface-high'
      }`}
    >
      <MaterialIcon name={icon} size={16} />
    </button>
  );
}
