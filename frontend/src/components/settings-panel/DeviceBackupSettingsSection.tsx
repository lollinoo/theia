/**
 * Defines device backup settings section behavior for settings screens.
 * Keeps validation, saved-state display, and defaults close to the controls that use them.
 */
import { MaterialIcon } from '../MaterialIcon';
import { compactControlClass, fieldLabelClass } from './settingsPanelStyles';

/** Formats device interval for the settings workflow. */
export function formatDeviceInterval(hours: number): string {
  if (hours >= 168) return '7 days';
  if (hours >= 48) return '48 hours';
  if (hours >= 24) return '24 hours';
  return hours + ' hours';
}

/** Device backup next backup text for the settings workflow. */
export function deviceBackupNextBackupText(intervalValue: string): string {
  const intervalHours = parseInt(intervalValue, 10);
  if (!intervalHours || intervalHours <= 0) return 'Scheduling disabled';
  return 'Backups run every ' + formatDeviceInterval(intervalHours);
}

interface DeviceBackupSettingsSectionProps {
  open: boolean;
  deviceBackupInterval: string;
  deviceBackupRetention: string;
  savedDeviceInterval: boolean;
  savedDeviceRetention: boolean;
  retentionError?: string;
  onToggle: () => void;
  onDeviceIntervalChange: (value: string) => void;
  onDeviceRetentionChange: (value: string) => void;
}

/** Renders the DeviceBackupSettingsSection component within the settings workflow. */
export function DeviceBackupSettingsSection({
  open,
  deviceBackupInterval,
  deviceBackupRetention,
  savedDeviceInterval,
  savedDeviceRetention,
  retentionError,
  onToggle,
  onDeviceIntervalChange,
  onDeviceRetentionChange,
}: DeviceBackupSettingsSectionProps) {
  return (
    <div className="rounded-lg bg-surface-container-high p-3">
      <button
        type="button"
        aria-expanded={open}
        onClick={onToggle}
        className="flex w-full items-center justify-between gap-3 rounded-md px-1 py-1 text-left transition-colors hover:text-on-bg"
      >
        <span>
          <span className="block text-sm font-semibold text-on-bg">Device Backups</span>
          <span className="block text-xs text-on-bg-secondary">
            Schedule automatic config snapshots.
          </span>
        </span>
        <MaterialIcon
          name={open ? 'expand_less' : 'expand_more'}
          className="text-on-bg-secondary"
        />
      </button>
      {open && (
        <div className="mt-4 grid items-start gap-4 sm:grid-cols-2">
          <label className="grid grid-rows-[2.5rem_auto_1rem] gap-1 text-sm">
            <span
              data-testid="device-backup-schedule-label-row"
              className="flex min-h-10 items-start justify-between gap-3"
            >
              <span className={fieldLabelClass}>Automatic Backup Schedule</span>
              {savedDeviceInterval && (
                <span className="text-xs font-medium text-status-up">Saved</span>
              )}
            </span>
            <select
              value={deviceBackupInterval}
              onChange={(e) => onDeviceIntervalChange(e.target.value)}
              className={compactControlClass()}
            >
              <option value="0">Disabled</option>
              <option value="6">Every 6 hours</option>
              <option value="12">Every 12 hours</option>
              <option value="24">Every 24 hours</option>
              <option value="48">Every 48 hours</option>
              <option value="168">Every 7 days</option>
            </select>
            <span
              data-testid="device-backup-schedule-helper-row"
              className="min-h-4 text-xs text-on-bg-muted"
            >
              {deviceBackupNextBackupText(deviceBackupInterval)}
            </span>
          </label>

          <label className="grid grid-rows-[2.5rem_auto_1rem] gap-1 text-sm">
            <span
              data-testid="device-backup-retention-label-row"
              className="flex min-h-10 items-start justify-between gap-3"
            >
              <span className={fieldLabelClass}>Keep last N backups per device</span>
              {savedDeviceRetention && (
                <span className="text-xs font-medium text-status-up">Saved</span>
              )}
            </span>
            <input
              type="number"
              min={1}
              max={365}
              value={deviceBackupRetention}
              onChange={(e) => onDeviceRetentionChange(e.target.value)}
              className={compactControlClass(Boolean(retentionError))}
            />
            <span
              data-testid="device-backup-retention-helper-row"
              className="min-h-4 text-xs text-status-down"
            >
              {retentionError ?? ''}
            </span>
          </label>
        </div>
      )}
    </div>
  );
}
