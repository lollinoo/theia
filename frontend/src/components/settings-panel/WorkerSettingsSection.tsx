/**
 * Defines worker settings section behavior for settings screens.
 * Keeps validation, saved-state display, and defaults close to the controls that use them.
 */
import { MaterialIcon } from '../MaterialIcon';
import {
  WORKER_SETTING_GROUPS,
  type WorkerSetting,
  type WorkerSettingKey,
} from './settingsConstants';
import { controlClass, fieldLabelClass } from './settingsPanelStyles';

interface WorkerSettingsSectionProps {
  open: boolean;
  workerSettings: Record<WorkerSettingKey, string>;
  savedWorkerSettings: Record<WorkerSettingKey, boolean>;
  fieldErrors: Record<string, string>;
  onToggle: () => void;
  onSettingChange: (key: WorkerSettingKey, value: string) => void;
  onSettingBlur: (setting: WorkerSetting) => void;
}

/** Renders the WorkerSettingsSection component within the settings workflow. */
export function WorkerSettingsSection({
  open,
  workerSettings,
  savedWorkerSettings,
  fieldErrors,
  onToggle,
  onSettingChange,
  onSettingBlur,
}: WorkerSettingsSectionProps) {
  return (
    <div className="rounded-lg bg-surface-container-high p-3">
      <button
        type="button"
        aria-expanded={open}
        onClick={onToggle}
        className="flex w-full items-center justify-between gap-3 rounded-md px-1 py-1 text-left transition-colors hover:text-on-bg"
      >
        <span>
          <span className="block text-sm font-semibold text-on-bg">Polling Workers</span>
          <span className="block text-xs text-on-bg-secondary">
            Tune pool sizes and isolation limits.
          </span>
        </span>
        <MaterialIcon
          name={open ? 'expand_less' : 'expand_more'}
          className="text-on-bg-secondary"
        />
      </button>
      {open && (
        <div className="mt-4 grid gap-4">
          {WORKER_SETTING_GROUPS.map((group) => (
            <div key={group.title} className="grid gap-3">
              <h3 className="text-xs font-semibold uppercase text-on-bg-muted">{group.title}</h3>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                {group.settings.map((setting) => (
                  <WorkerSettingField
                    key={setting.key}
                    setting={setting}
                    value={workerSettings[setting.key]}
                    saved={savedWorkerSettings[setting.key]}
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

interface WorkerSettingFieldProps {
  setting: WorkerSetting;
  value: string;
  saved: boolean;
  error?: string;
  onChange: (key: WorkerSettingKey, value: string) => void;
  onBlur: (setting: WorkerSetting) => void;
}

function WorkerSettingField({
  setting,
  value,
  saved,
  error,
  onChange,
  onBlur,
}: WorkerSettingFieldProps) {
  const inputId = `worker-setting-${setting.key}`;
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between gap-3">
        <label htmlFor={inputId} className={fieldLabelClass}>
          {setting.label}
        </label>
        {saved && <span className="text-xs font-medium text-status-up">Saved</span>}
      </div>
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
      <p className="break-all font-mono text-[10px] leading-relaxed text-on-bg-muted">
        {setting.key}
      </p>
      {error && <p className="text-[10px] text-status-down">{error}</p>}
    </div>
  );
}
