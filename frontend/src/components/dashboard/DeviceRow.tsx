import { type Device } from '../../types/api';

interface DeviceRowProps {
  device: Device;
  onSSHCredentials: () => void;
  onBackup: () => void;
  onBackupHistory: () => void;
  onViewConfig: () => void;
}

const statusColors: Record<string, string> = {
  up: 'bg-status-up',
  down: 'bg-status-down',
  probing: 'bg-status-probing',
  unknown: 'bg-status-unknown',
};

export function DeviceRow({
  device,
  onSSHCredentials,
  onBackup,
  onBackupHistory,
  onViewConfig,
}: DeviceRowProps) {
  const displayName = device.tags?.display_name || device.sys_name || device.hostname || device.ip;

  return (
    <tr className="border-b border-border-subtle/50 hover:bg-bg-elevated/50 transition-colors">
      <td className="px-3 py-2.5">
        <div className="font-medium text-text-primary">{displayName}</div>
        {device.sys_name && device.sys_name !== displayName && (
          <div className="text-text-secondary mt-0.5">{device.sys_name}</div>
        )}
      </td>
      <td className="px-3 py-2.5 text-text-secondary font-mono">{device.ip}</td>
      <td className="px-3 py-2.5">
        <div className="flex items-center gap-1.5">
          <span className={`h-2 w-2 rounded-full ${statusColors[device.status] ?? statusColors.unknown}`} />
          <span className="text-text-secondary capitalize">{device.status}</span>
        </div>
      </td>
      <td className="px-3 py-2.5 text-text-secondary">
        {device.hardware_model && device.hardware_model !== 'Unknown'
          ? device.hardware_model
          : device.sys_descr
            ? device.sys_descr.length > 30 ? `${device.sys_descr.slice(0, 29)}\u2026` : device.sys_descr
            : '\u2014'}
      </td>
      <td className="px-3 py-2.5">
        <div className="flex items-center justify-end gap-1">
          <ActionButton label="SSH" onClick={onSSHCredentials} title="SSH Credentials" />
          <ActionButton label="Backup" onClick={onBackup} title="Backup Now" />
          <ActionButton label="History" onClick={onBackupHistory} title="Backup History" />
          <ActionButton label="Config" onClick={onViewConfig} title="View Config" />
        </div>
      </td>
    </tr>
  );
}

function ActionButton({ label, onClick, title }: { label: string; onClick: () => void; title: string }) {
  return (
    <button
      onClick={onClick}
      title={title}
      className="rounded px-2 py-1 text-[10px] font-medium text-text-secondary border border-border-subtle hover:bg-bg-elevated hover:text-text-primary transition-colors"
    >
      {label}
    </button>
  );
}
