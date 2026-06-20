/**
 * Defines SNMP debug settings section behavior for settings screens.
 * Keeps runtime tuning controls grouped for production troubleshooting.
 */
import { MaterialIcon } from '../MaterialIcon';
import { SavedIndicator } from './SavedIndicator';
import {
  SNMP_DEBUG_SETTING_GROUPS,
  SNMP_MAX_REPETITIONS,
  type SNMPDebugSetting,
  type SNMPDebugSettingKey,
} from './settingsConstants';
import { controlClass, fieldLabelClass } from './settingsPanelStyles';

interface SNMPDebugSettingsSectionProps {
  open: boolean;
  settings: Record<SNMPDebugSettingKey, string>;
  savedSettings: Record<SNMPDebugSettingKey, boolean>;
  fieldErrors: Record<string, string>;
  onToggle: () => void;
  onSettingChange: (key: SNMPDebugSettingKey, value: string) => void;
  onSettingBlur: (setting: SNMPDebugSetting) => void;
}

/** Renders the SNMPDebugSettingsSection component within the settings workflow. */
export function SNMPDebugSettingsSection({
  open,
  settings,
  savedSettings,
  fieldErrors,
  onToggle,
  onSettingChange,
  onSettingBlur,
}: SNMPDebugSettingsSectionProps) {
  return (
    <div className="rounded-lg bg-surface-container-high p-3">
      <button
        type="button"
        aria-expanded={open}
        onClick={onToggle}
        className="flex w-full items-center justify-between gap-3 rounded-md px-1 py-1 text-left transition-colors hover:text-on-bg"
      >
        <span>
          <span className="block text-sm font-semibold text-on-bg">SNMP Debug Parameters</span>
          <span className="block text-xs text-on-bg-secondary">
            Runtime SNMP timing, workers, and isolation.
          </span>
        </span>
        <MaterialIcon
          name={open ? 'expand_less' : 'expand_more'}
          className="text-on-bg-secondary"
        />
      </button>
      {open && (
        <div className="mt-4 grid gap-4">
          <div className="grid gap-3 rounded-md border border-outline-subtle bg-surface px-3 py-3">
            <div className="flex items-center justify-between gap-3">
              <span className={fieldLabelClass}>GETBULK Max Repetitions</span>
              <span className="rounded bg-bg px-2 py-1 font-mono text-xs text-on-bg">
                {SNMP_MAX_REPETITIONS}
              </span>
            </div>
            <p
              data-testid="snmp-debug-max-repetitions-key"
              className="break-all font-mono text-[10px] leading-relaxed text-on-bg-muted"
            >
              snmp_max_repetitions
            </p>
          </div>

          {SNMP_DEBUG_SETTING_GROUPS.map((group) => (
            <div key={group.title} className="grid gap-3">
              <h3 className="text-xs font-semibold uppercase text-on-bg-muted">{group.title}</h3>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                {group.settings.map((setting) => (
                  <SNMPDebugSettingField
                    key={setting.key}
                    setting={setting}
                    value={settings[setting.key]}
                    saved={savedSettings[setting.key]}
                    error={fieldErrors[setting.key]}
                    onChange={onSettingChange}
                    onBlur={onSettingBlur}
                  />
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

interface SNMPDebugSettingFieldProps {
  setting: SNMPDebugSetting;
  value: string;
  saved: boolean;
  error?: string;
  onChange: (key: SNMPDebugSettingKey, value: string) => void;
  onBlur: (setting: SNMPDebugSetting) => void;
}

function SNMPDebugSettingField({
  setting,
  value,
  saved,
  error,
  onChange,
  onBlur,
}: SNMPDebugSettingFieldProps) {
  const inputId = `snmp-debug-setting-${setting.key}`;
  return (
    <div className="space-y-1">
      <div className="flex min-h-8 items-start justify-between gap-3">
        <label htmlFor={inputId} className={fieldLabelClass}>
          {setting.label}
        </label>
        <SavedIndicator visible={saved} />
      </div>
      <div className="flex items-center gap-2">
        <input
          id={inputId}
          type="number"
          min={setting.min}
          max={setting.max}
          step={1}
          value={value}
          onChange={(e) => onChange(setting.key, e.target.value)}
          onBlur={() => onBlur(setting)}
          className={controlClass(Boolean(error))}
        />
        {setting.unit && <span className="text-xs text-on-bg-secondary">{setting.unit}</span>}
      </div>
      <p className="break-all font-mono text-[10px] leading-relaxed text-on-bg-muted">
        {setting.key}
      </p>
      {error && <p className="text-[10px] text-status-down">{error}</p>}
    </div>
  );
}
