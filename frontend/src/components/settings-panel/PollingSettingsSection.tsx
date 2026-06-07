/**
 * Defines polling settings section behavior for settings screens.
 * Keeps validation, saved-state display, and defaults close to the controls that use them.
 */
import { SavedIndicator } from './SavedIndicator';
import type { WorkerSetting, WorkerSettingKey } from './settingsConstants';
import { POLLING_PRESETS } from './settingsConstants';
import { controlClass, fieldLabelClass } from './settingsPanelStyles';
import { WorkerSettingsSection } from './WorkerSettingsSection';

interface PollingSettingsSectionProps {
  pollingValue: string;
  customPolling: string;
  savedPolling: boolean;
  customPollingError?: string;
  workerSectionOpen: boolean;
  workerSettings: Record<WorkerSettingKey, string>;
  savedWorkerSettings: Record<WorkerSettingKey, boolean>;
  fieldErrors: Record<string, string>;
  onPollingPresetChange: (value: string) => void;
  onCustomPollingChange: (value: string) => void;
  onCustomPollingBlur: () => void;
  onWorkerSectionToggle: () => void;
  onWorkerSettingChange: (key: WorkerSettingKey, value: string) => void;
  onWorkerSettingBlur: (setting: WorkerSetting) => void;
}

/** Renders the PollingSettingsSection component within the settings workflow. */
export function PollingSettingsSection({
  pollingValue,
  customPolling,
  savedPolling,
  customPollingError,
  workerSectionOpen,
  workerSettings,
  savedWorkerSettings,
  fieldErrors,
  onPollingPresetChange,
  onCustomPollingChange,
  onCustomPollingBlur,
  onWorkerSectionToggle,
  onWorkerSettingChange,
  onWorkerSettingBlur,
}: PollingSettingsSectionProps) {
  return (
    <div className="grid gap-4">
      <label className="grid gap-1 text-sm">
        <span className="flex items-center justify-between gap-3">
          <span className={fieldLabelClass}>Polling Interval</span>
          <SavedIndicator visible={savedPolling} />
        </span>
        <select
          value={pollingValue}
          onChange={(e) => onPollingPresetChange(e.target.value)}
          className={controlClass()}
        >
          {POLLING_PRESETS.map((preset) => (
            <option key={preset.value} value={preset.value}>
              {preset.label}
            </option>
          ))}
        </select>
      </label>
      {pollingValue === 'custom' && (
        <div className="grid gap-1 text-sm">
          <label htmlFor="custom-polling-seconds" className={fieldLabelClass}>
            Custom interval
          </label>
          <div className="flex items-center gap-2">
            <div className="min-w-0 flex-1">
              <input
                id="custom-polling-seconds"
                type="number"
                min={5}
                max={3600}
                value={customPolling}
                placeholder="Seconds (5-3600)"
                onChange={(e) => onCustomPollingChange(e.target.value)}
                onBlur={onCustomPollingBlur}
                className={controlClass(Boolean(customPollingError))}
              />
              {customPollingError && (
                <p className="mt-1 text-xs text-status-down">{customPollingError}</p>
              )}
            </div>
            <span className="text-xs text-on-bg-secondary">sec</span>
          </div>
        </div>
      )}

      <WorkerSettingsSection
        open={workerSectionOpen}
        workerSettings={workerSettings}
        savedWorkerSettings={savedWorkerSettings}
        fieldErrors={fieldErrors}
        onToggle={onWorkerSectionToggle}
        onSettingChange={onWorkerSettingChange}
        onSettingBlur={onWorkerSettingBlur}
      />
    </div>
  );
}
