import { SavedIndicator } from './SavedIndicator';
import { controlClass, fieldLabelClass } from './settingsPanelStyles';

interface PrometheusSettingsSectionProps {
  prometheusUrl: string;
  savedPrometheus: boolean;
  prometheusError?: string;
  onPrometheusChange: (value: string) => void;
  onPrometheusBlur: () => void;
}

export function PrometheusSettingsSection({
  prometheusUrl,
  savedPrometheus,
  prometheusError,
  onPrometheusChange,
  onPrometheusBlur,
}: PrometheusSettingsSectionProps) {
  return (
    <div className="grid gap-4">
      <label className="grid gap-1 text-sm">
        <span className="flex items-center justify-between gap-3">
          <span className={fieldLabelClass}>Prometheus URL</span>
          <SavedIndicator visible={savedPrometheus} />
        </span>
        <input
          type="url"
          value={prometheusUrl}
          placeholder="http://localhost:9090"
          onChange={(e) => onPrometheusChange(e.target.value)}
          onBlur={onPrometheusBlur}
          className={controlClass(Boolean(prometheusError))}
        />
        {prometheusError && <span className="text-xs text-status-down">{prometheusError}</span>}
      </label>
    </div>
  );
}
