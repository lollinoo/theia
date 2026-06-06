import { SavedIndicator } from './SavedIndicator';
import { TIMEZONES } from './settingsConstants';
import { controlClass, fieldLabelClass } from './settingsPanelStyles';

interface BridgeSettingsSectionProps {
  timezone: string;
  bridgePort: string;
  savedTimezone: boolean;
  savedBridgePort: boolean;
  bridgePortError?: string;
  onTimezoneChange: (value: string) => void;
  onBridgePortChange: (value: string) => void;
  onBridgePortBlur: () => void;
}

export function BridgeSettingsSection({
  timezone,
  bridgePort,
  savedTimezone,
  savedBridgePort,
  bridgePortError,
  onTimezoneChange,
  onBridgePortChange,
  onBridgePortBlur,
}: BridgeSettingsSectionProps) {
  return (
    <div className="grid gap-4">
      <label className="grid gap-1 text-sm">
        <span className="flex items-center justify-between gap-3">
          <span className={fieldLabelClass}>Timezone</span>
          <SavedIndicator visible={savedTimezone} />
        </span>
        <select
          value={timezone}
          onChange={(e) => onTimezoneChange(e.target.value)}
          className={controlClass()}
        >
          {TIMEZONES.map((tz) => (
            <option key={tz.value} value={tz.value}>
              {tz.label}
            </option>
          ))}
        </select>
        <span className="text-xs text-on-bg-secondary">
          Affects backup filenames and zip timestamps.
        </span>
      </label>

      <label className="grid gap-1 text-sm">
        <span className="flex items-center justify-between gap-3">
          <span className={fieldLabelClass}>WinBox Bridge Port</span>
          <SavedIndicator visible={savedBridgePort} />
        </span>
        <input
          type="number"
          min={1}
          max={65535}
          value={bridgePort}
          placeholder="1337"
          onChange={(e) => onBridgePortChange(e.target.value)}
          onBlur={onBridgePortBlur}
          className={controlClass(Boolean(bridgePortError), 'font-mono')}
        />
        {bridgePortError && <span className="text-xs text-status-down">{bridgePortError}</span>}
        <span className="text-xs text-on-bg-secondary">
          Default is <span className="font-mono">1337</span>. Must match{' '}
          <span className="font-mono">ListenPort</span> in the bridge config.
        </span>
      </label>
    </div>
  );
}
