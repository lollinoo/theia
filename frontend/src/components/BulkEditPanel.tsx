import { useEffect, useState } from 'react';
import type { Area, Device } from '../types/api';
import { deleteDevice, fetchAreas, updateDevice } from '../api/client';
import { ValidationError, ServerError } from '../api/errors';

interface BulkEditPanelProps {
  devices: Device[];
  onDevicesUpdated: (updated: Device[]) => void;
  onDevicesDeleted: () => void;
}

/** Compute the common value across devices for a given key, or 'mixed' if they differ. */
function commonValue<T>(devices: Device[], extract: (d: Device) => T): T | 'mixed' {
  if (devices.length === 0) return 'mixed';
  const first = extract(devices[0]);
  const firstJSON = JSON.stringify(first);
  for (let i = 1; i < devices.length; i++) {
    if (JSON.stringify(extract(devices[i])) !== firstJSON) return 'mixed';
  }
  return first;
}

export function BulkEditPanel({ devices, onDevicesUpdated, onDevicesDeleted }: BulkEditPanelProps) {
  const [areas, setAreas] = useState<Area[]>([]);

  // Bulk field state -- undefined means "no change"
  const [areaIds, setAreaIds] = useState<string[] | undefined>(undefined);
  const [metricsSource, setMetricsSource] = useState<string | undefined>(undefined);
  const [vendor, setVendor] = useState<string | undefined>(undefined);

  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [deleteProgress, setDeleteProgress] = useState(0);

  // Load reference data
  useEffect(() => {
    fetchAreas().then(setAreas).catch(() => {/* non-fatal */});
  }, []);

  // Compute current common values for display
  const commonAreaIds = commonValue(devices, (d) => [...(d.area_ids ?? [])].sort());
  const commonMetricsSource = commonValue(devices, (d) => d.metrics_source || 'snmp');
  const commonVendor = commonValue(devices, (d) => d.vendor || '');

  // The effective values shown in the UI: user override or current common
  const displayAreaIds = areaIds ?? (commonAreaIds === 'mixed' ? [] : commonAreaIds);
  const displayMetricsSource = metricsSource ?? (commonMetricsSource === 'mixed' ? '' : commonMetricsSource);
  const displayVendor = vendor ?? (commonVendor === 'mixed' ? '' : commonVendor);

  const hasChanges = areaIds !== undefined || metricsSource !== undefined || vendor !== undefined;

  async function handleSave() {
    if (!hasChanges) return;
    setSaving(true);
    setSaveError(null);
    setSaved(false);

    try {
      const results = await Promise.allSettled(
        devices.map((d) =>
          updateDevice(d.id, {
            hostname: d.hostname,
            ...(areaIds !== undefined ? { area_ids: areaIds } : {}),
            ...(metricsSource !== undefined ? { metrics_source: metricsSource } : {}),
            ...(vendor !== undefined ? { vendor: vendor || undefined } : {}),
          }),
        ),
      );

      const updatedDevices: Device[] = [];
      const errors: string[] = [];
      for (let i = 0; i < results.length; i++) {
        const result = results[i];
        if (result.status === 'fulfilled') {
          updatedDevices.push(result.value);
        } else {
          const reason = result.reason;
          let errMsg: string;
          if (reason instanceof ServerError) {
            errMsg = reason.correlationId
              ? `server error (ref: ${reason.correlationId})`
              : 'server error';
          } else if (reason instanceof ValidationError) {
            errMsg = reason.message;
          } else {
            errMsg = reason instanceof Error ? reason.message : 'failed';
          }
          errors.push(`${devices[i].hostname || devices[i].ip}: ${errMsg}`);
        }
      }

      if (errors.length > 0) {
        setSaveError(`${errors.length} of ${devices.length} updates failed:\n${errors.join('\n')}`);
      }

      if (updatedDevices.length > 0) {
        onDevicesUpdated(updatedDevices);
        setSaved(true);
        // Reset change tracking
        setAreaIds(undefined);
        setMetricsSource(undefined);
        setVendor(undefined);
        setTimeout(() => setSaved(false), 2000);
      }
    } catch (err) {
      if (err instanceof ServerError) {
        setSaveError(err.correlationId
          ? `Something went wrong (ref: ${err.correlationId})`
          : 'Something went wrong');
      } else if (err instanceof ValidationError) {
        setSaveError(err.message);
      } else {
        setSaveError(err instanceof Error ? err.message : 'Bulk update failed');
      }
    } finally {
      setSaving(false);
    }
  }

  async function handleBulkDelete() {
    setDeleteLoading(true);
    setDeleteProgress(0);

    let completed = 0;
    const results = await Promise.allSettled(
      devices.map((d) =>
        deleteDevice(d.id).then(() => {
          completed++;
          setDeleteProgress(completed);
        }),
      ),
    );

    const failures = results.filter((r) => r.status === 'rejected');
    if (failures.length > 0) {
      setSaveError(`${failures.length} of ${devices.length} deletes failed`);
      setDeleteLoading(false);
      setConfirmDelete(false);
      return;
    }

    onDevicesDeleted();
  }

  return (
    <div className="space-y-6 p-4 transition-colors duration-200">
      {/* Selection summary */}
      <div className="flex items-center gap-3 rounded-lg bg-primary/10 border border-primary/20 px-4 py-3">
        <span className="flex h-8 min-w-[32px] items-center justify-center rounded-full bg-primary/20 px-2 text-sm font-bold text-primary">
          {devices.length}
        </span>
        <div>
          <p className="text-sm font-medium text-on-bg">devices selected</p>
          <p className="text-xs text-on-bg-secondary mt-0.5">
            Changes apply to all selected devices
          </p>
        </div>
      </div>

      {/* Area assignment */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">Areas</p>
          {commonAreaIds === 'mixed' && areaIds === undefined && (
            <span className="text-xs text-on-bg-muted italic">Mixed</span>
          )}
        </div>

        {displayAreaIds.length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            {displayAreaIds.map((id) => {
              const area = areas.find((a) => a.id === id);
              if (!area) return null;
              return (
                <span
                  key={id}
                  className="inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium text-on-bg"
                  style={{ backgroundColor: `${area.color}25`, border: `1px solid ${area.color}60` }}
                >
                  <span className="inline-block h-2 w-2 rounded-full" style={{ backgroundColor: area.color }} />
                  {area.name}
                  <button
                    type="button"
                    onClick={() => {
                      const next = (areaIds ?? (commonAreaIds === 'mixed' ? [] : commonAreaIds)).filter((a) => a !== id);
                      setAreaIds(next);
                    }}
                    className="ml-0.5 text-on-bg-secondary hover:text-on-bg"
                  >
                    <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                </span>
              );
            })}
          </div>
        )}
        {areas.filter((a) => !displayAreaIds.includes(a.id)).length > 0 && (
          <select
            value=""
            onChange={(e) => {
              if (e.target.value) {
                const current = areaIds ?? (commonAreaIds === 'mixed' ? [] : commonAreaIds);
                setAreaIds([...current, e.target.value]);
              }
            }}
            className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
          >
            <option value="">{displayAreaIds.length === 0 ? 'Unassigned - select area...' : 'Add another area...'}</option>
            {areas.filter((a) => !displayAreaIds.includes(a.id)).map((a) => (
              <option key={a.id} value={a.id}>{a.name}</option>
            ))}
          </select>
        )}
      </div>

      {/* Vendor */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">Vendor</p>
          {commonVendor === 'mixed' && vendor === undefined && (
            <span className="text-xs text-on-bg-muted italic">Mixed</span>
          )}
        </div>
        <select
          value={displayVendor}
          onChange={(e) => setVendor(e.target.value)}
          className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
        >
          <option value="">-- Select vendor --</option>
          <option value="mikrotik">MikroTik</option>
        </select>
      </div>

      {/* Metrics Source */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">Metrics Source</p>
          {commonMetricsSource === 'mixed' && metricsSource === undefined && (
            <span className="text-xs text-on-bg-muted italic">Mixed</span>
          )}
        </div>
        <select
          value={displayMetricsSource}
          onChange={(e) => setMetricsSource(e.target.value)}
          className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
        >
          <option value="">-- Keep current --</option>
          <option value="snmp">SNMP Direct</option>
          <option value="prometheus">Prometheus</option>
          <option value="prometheus_snmp_fallback">Prometheus + SNMP Fallback</option>
        </select>
      </div>

      {/* Save */}
      {saveError && (
        <p className="rounded-lg border border-status-down/30 bg-status-down/10 px-3 py-2 text-xs text-status-down whitespace-pre-line">
          {saveError}
        </p>
      )}

      <button
        type="button"
        disabled={saving || !hasChanges}
        onClick={() => { void handleSave(); }}
        className="w-full rounded-lg bg-surface-high px-4 py-2 text-sm font-medium text-on-bg transition-colors hover:bg-elevated disabled:cursor-not-allowed disabled:opacity-50"
      >
        {saving ? 'Applying...' : saved ? 'Saved' : `Apply to ${devices.length} Devices`}
      </button>

      {/* Bulk Delete */}
      <div className="mt-6 space-y-3">
        {!confirmDelete ? (
          <button
            type="button"
            onClick={() => setConfirmDelete(true)}
            className="w-full rounded-lg border border-status-down/30 bg-status-down/10 px-4 py-2 text-sm font-medium text-status-down transition-colors hover:bg-status-down/20"
          >
            Delete {devices.length} Devices
          </button>
        ) : (
          <div className="space-y-2 rounded-lg border border-status-down/30 bg-status-down/10 p-3">
            <p className="text-sm text-status-down">
              Delete {devices.length} devices? This cannot be undone.
            </p>
            {deleteLoading && (
              <div className="w-full rounded-full bg-status-down/20 h-1.5">
                <div
                  className="h-1.5 rounded-full bg-status-down transition-all duration-300"
                  style={{ width: `${(deleteProgress / devices.length) * 100}%` }}
                />
              </div>
            )}
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => setConfirmDelete(false)}
                disabled={deleteLoading}
                className="flex-1 rounded-lg bg-surface-high px-3 py-1.5 text-xs text-on-bg hover:bg-elevated disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                type="button"
                disabled={deleteLoading}
                onClick={() => { void handleBulkDelete(); }}
                className="flex-1 rounded-lg bg-status-down px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {deleteLoading ? `Deleting ${deleteProgress}/${devices.length}...` : 'Confirm Delete All'}
              </button>
            </div>
          </div>
        )}
      </div>

      {/* Selected devices list */}
      <div className="space-y-2">
        <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">Selected Devices</p>
        <div className="space-y-1 max-h-48 overflow-y-auto">
          {devices.map((d) => (
            <div key={d.id} className="flex items-center gap-2 rounded-lg bg-surface-high px-3 py-1.5 text-xs">
              <span className={`h-1.5 w-1.5 rounded-full flex-none ${d.status === 'up' ? 'bg-status-up' : d.status === 'down' ? 'bg-status-down' : 'bg-on-bg-muted'}`} />
              <span className="text-on-bg truncate">{d.tags?.display_name || d.sys_name || d.hostname || d.ip}</span>
              <span className="text-on-bg-muted ml-auto flex-none">{d.ip}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
