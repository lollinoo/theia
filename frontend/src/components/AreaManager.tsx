import { useEffect, useState } from 'react';
import {
  createArea,
  createCanvasMapArea,
  deleteArea,
  deleteCanvasMapArea,
  fetchAreas,
  fetchDevices,
  updateArea,
  updateCanvasMapArea,
  updateCanvasMapDeviceAreas,
  updateDevice,
} from '../api/client';
import { adaptAreaColor, useTheme } from '../contexts/ThemeContext';
import type { Area, Device } from '../types/api';
import { MaterialIcon } from './MaterialIcon';

// Per D-01: curated palette of 7 swatches
const AREA_COLORS = [
  '#00E676', // green (default per D-03)
  '#2979FF', // blue
  '#E040FB', // purple
  '#FFEA00', // amber
  '#FF6D00', // orange
  '#00BCD4', // cyan
  '#FF1744', // red
] as const;
const defaultAreaColor = AREA_COLORS[0];

const inputClass =
  'w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none';
const labelClass = 'text-xs font-medium uppercase tracking-widest text-on-bg-secondary';

function normalizeAreaColor(color: string): string {
  const trimmed = color.trim();
  if (/^#[0-9a-fA-F]{6}$/.test(trimmed)) {
    return trimmed.toUpperCase();
  }
  return defaultAreaColor;
}

// --- AreaForm child component ---

interface AreaFormProps {
  initial: { name: string; description: string; color: string };
  onSave: (form: { name: string; description: string; color: string }) => Promise<void>;
  onCancel: () => void;
  saveLabel: string;
}

function AreaForm({ initial, onSave, onCancel, saveLabel }: AreaFormProps) {
  const [form, setForm] = useState(() => ({
    ...initial,
    color: normalizeAreaColor(initial.color),
  }));
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const selectedColor = normalizeAreaColor(form.color);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      await onSave({ ...form, color: selectedColor });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save area.');
    } finally {
      setLoading(false);
    }
  }

  return (
    <form
      onSubmit={(e) => {
        void handleSubmit(e);
      }}
      className="space-y-3"
    >
      <div className="space-y-1">
        <label className={labelClass}>
          Name <span className="text-status-down">*</span>
        </label>
        <input
          type="text"
          value={form.name}
          onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
          placeholder="e.g. Backbone Core"
          required
          className={inputClass}
        />
      </div>

      <div className="space-y-1">
        <label className={labelClass}>Description</label>
        <input
          type="text"
          value={form.description}
          onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
          placeholder="Optional description"
          className={inputClass}
        />
      </div>

      <div className="space-y-1">
        <label className={labelClass}>Color</label>
        <div className="flex flex-wrap items-center gap-2">
          {AREA_COLORS.map((c) => (
            <button
              key={c}
              type="button"
              onClick={() => setForm((f) => ({ ...f, color: c }))}
              className={`h-6 w-6 rounded-full border-2 transition-all ${
                selectedColor === c
                  ? 'border-primary scale-110'
                  : 'border-transparent hover:scale-105'
              }`}
              style={{ backgroundColor: c }}
              title={c}
            />
          ))}
          <label className="ml-1 flex items-center gap-2 rounded-lg border border-outline-subtle bg-elevated px-2 py-1">
            <span className="sr-only">Custom color</span>
            <input
              type="color"
              aria-label="Custom color"
              value={selectedColor}
              onChange={(e) =>
                setForm((current) => ({
                  ...current,
                  color: normalizeAreaColor(e.target.value),
                }))
              }
              className="h-6 w-8 cursor-pointer border-0 bg-transparent p-0"
            />
            <span className="font-mono text-xs text-on-bg-secondary">{selectedColor}</span>
          </label>
        </div>
      </div>

      {error && (
        <p className="rounded-lg border border-status-down/30 bg-status-down/10 px-3 py-2 text-xs text-status-down">
          {error}
        </p>
      )}

      <div className="flex gap-2">
        <button
          type="button"
          onClick={onCancel}
          className="flex-1 rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg hover:bg-surface-high"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={loading}
          className="flex-1 rounded-lg bg-primary px-3 py-2 text-sm font-medium text-white hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {loading ? 'Saving...' : saveLabel}
        </button>
      </div>
    </form>
  );
}

// --- AreaManager main component ---

interface AreaManagerProps {
  onAreasChange?: () => void | Promise<void>;
  mapContext?: { mapId: string; mapName: string };
  areas?: Area[];
  devices?: Device[];
  title?: string;
  titleId?: string;
  areaMetrics?: Record<
    string,
    {
      healthLabel: string;
      activeLinkCount: number;
      degradedDeviceCount: number;
    }
  >;
  onOpenArea?: (areaId: string) => void;
  onCreateMapFromArea?: (area: Area) => void;
}

export function AreaManager({
  onAreasChange,
  mapContext,
  areas: providedAreas,
  devices: providedDevices,
  title = 'Areas',
  titleId,
  areaMetrics,
  onOpenArea,
  onCreateMapFromArea,
}: AreaManagerProps) {
  const { resolvedTheme } = useTheme();
  const [loadedAreas, setLoadedAreas] = useState<Area[]>([]);
  const [loadedDevices, setLoadedDevices] = useState<Device[]>([]);
  const [loading, setLoading] = useState(
    providedAreas === undefined || providedDevices === undefined,
  );
  const [mode, setMode] = useState<'list' | 'create' | 'edit'>('list');
  const [editing, setEditing] = useState<Area | null>(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const areas = providedAreas ?? loadedAreas;
  const allDevices = providedDevices ?? loadedDevices;
  const mapScoped = Boolean(mapContext);

  async function load() {
    if (mapScoped) {
      setLoading(false);
      return;
    }
    setLoading(true);
    try {
      const [areasData, devicesData] = await Promise.all([fetchAreas(), fetchDevices()]);
      setLoadedAreas(areasData);
      setLoadedDevices(devicesData);
    } catch {
      // non-fatal
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, [mapScoped]);

  useEffect(() => {
    setMode('list');
    setEditing(null);
    setConfirmDeleteId(null);
  }, [mapContext?.mapId]);

  async function refreshAreas() {
    if (mapScoped) {
      await onAreasChange?.();
      return;
    }
    await load();
    await onAreasChange?.();
  }

  async function handleCreate(form: { name: string; description: string; color: string }) {
    const payload = {
      name: form.name.trim(),
      description: form.description.trim(),
      color: normalizeAreaColor(form.color),
    };
    if (mapContext) {
      await createCanvasMapArea(mapContext.mapId, payload);
    } else {
      await createArea(payload);
    }
    setMode('list');
    await refreshAreas();
  }

  async function handleUpdate(form: { name: string; description: string; color: string }) {
    if (!editing) return;
    const payload = {
      name: form.name.trim(),
      description: form.description.trim(),
      color: normalizeAreaColor(form.color),
    };
    if (mapContext) {
      await updateCanvasMapArea(mapContext.mapId, editing.id, payload);
    } else {
      await updateArea(editing.id, payload);
    }
    setMode('list');
    setEditing(null);
    await refreshAreas();
  }

  async function handleDelete(id: string) {
    setDeleteLoading(true);
    try {
      if (mapContext) {
        await deleteCanvasMapArea(mapContext.mapId, id);
      } else {
        await deleteArea(id);
      }
      setConfirmDeleteId(null);
      await refreshAreas();
    } finally {
      setDeleteLoading(false);
    }
  }

  async function handleRemoveDevice(deviceId: string) {
    if (!editing) return;
    const device = allDevices.find((d) => d.id === deviceId);
    const newIds = (device?.area_ids ?? []).filter((id) => id !== editing.id);
    if (mapContext) {
      await updateCanvasMapDeviceAreas(mapContext.mapId, {
        device_ids: [deviceId],
        area_ids: newIds,
      });
    } else {
      await updateDevice(deviceId, { area_ids: newIds });
    }
    await refreshAreas();
  }

  async function handleAssignDevice(deviceId: string) {
    if (!editing) return;
    const device = allDevices.find((d) => d.id === deviceId);
    const newIds = [...(device?.area_ids ?? []), editing.id];
    if (mapContext) {
      await updateCanvasMapDeviceAreas(mapContext.mapId, {
        device_ids: [deviceId],
        area_ids: newIds,
      });
    } else {
      await updateDevice(deviceId, { area_ids: newIds });
    }
    await refreshAreas();
  }

  // --- Create mode ---
  if (mode === 'create') {
    return (
      <div className="space-y-3">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setMode('list')}
            className="text-on-bg-secondary hover:text-on-bg"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M15 19l-7-7 7-7"
              />
            </svg>
          </button>
          <p className={labelClass}>New Area</p>
        </div>
        <AreaForm
          initial={{ name: '', description: '', color: '#00E676' }}
          onSave={handleCreate}
          onCancel={() => setMode('list')}
          saveLabel="Create Area"
        />
      </div>
    );
  }

  // --- Edit mode ---
  if (mode === 'edit' && editing) {
    const assignedDevices = allDevices.filter((d) => d.area_ids?.includes(editing.id));
    const unassignedDevices = allDevices.filter((d) => !d.area_ids?.includes(editing.id));

    return (
      <div className="space-y-3">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => {
              setMode('list');
              setEditing(null);
            }}
            className="text-on-bg-secondary hover:text-on-bg"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M15 19l-7-7 7-7"
              />
            </svg>
          </button>
          <p className={labelClass}>Edit Area</p>
        </div>
        <AreaForm
          initial={{
            name: editing.name,
            description: editing.description,
            color: normalizeAreaColor(editing.color),
          }}
          onSave={handleUpdate}
          onCancel={() => {
            setMode('list');
            setEditing(null);
          }}
          saveLabel="Save Changes"
        />

        {/* Assigned Devices section */}
        <div className="space-y-2 pt-2">
          <p className={labelClass}>Assigned Devices ({assignedDevices.length})</p>
          {assignedDevices.length === 0 && (
            <p className="text-xs text-on-bg-secondary">No devices assigned to this area.</p>
          )}
          {assignedDevices.map((d) => (
            <div
              key={d.id}
              className="flex items-center justify-between rounded-lg border border-outline-subtle bg-elevated p-2"
            >
              <span className="text-sm text-on-bg truncate">{d.hostname || d.ip}</span>
              <button
                type="button"
                onClick={() => {
                  void handleRemoveDevice(d.id);
                }}
                className="p-1 text-on-bg-secondary hover:text-status-down rounded shrink-0"
                title="Remove from area"
                aria-label="remove device"
              >
                <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M6 18L18 6M6 6l12 12"
                  />
                </svg>
              </button>
            </div>
          ))}

          {/* Add device dropdown */}
          {unassignedDevices.length > 0 && (
            <select
              defaultValue=""
              onChange={(e) => {
                if (e.target.value) {
                  void handleAssignDevice(e.target.value);
                  e.target.value = '';
                }
              }}
              className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:outline-none"
            >
              <option value="" disabled>
                Add device to area...
              </option>
              {unassignedDevices.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.hostname || d.ip}
                </option>
              ))}
            </select>
          )}
        </div>
      </div>
    );
  }

  // --- List mode ---
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        {titleId ? (
          <h3 id={titleId} className="text-base font-semibold text-on-bg">
            {title}
          </h3>
        ) : (
          <p className={labelClass}>{title}</p>
        )}
        <button
          type="button"
          onClick={() => setMode('create')}
          className="flex items-center gap-1 rounded-lg border border-outline-subtle bg-elevated px-2 py-1 text-xs text-on-bg hover:bg-surface-high"
        >
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          New area
        </button>
      </div>

      {loading && <p className="text-xs text-on-bg-secondary">Loading areas...</p>}

      {!loading && areas.length === 0 && (
        <p className="text-xs text-on-bg-secondary">No areas yet. Create one to group devices.</p>
      )}

      {!loading &&
        areas.map((area) => {
          const metrics = areaMetrics?.[area.id];

          return (
            <div
              key={area.id}
              className="rounded-lg border border-outline-subtle bg-elevated p-3 space-y-3"
            >
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span
                      className="inline-block h-3 w-3 rounded-full shrink-0"
                      style={{ backgroundColor: adaptAreaColor(area.color, resolvedTheme) }}
                    />
                    <p className="text-sm font-medium text-on-bg truncate">{area.name}</p>
                  </div>
                  {area.description && (
                    <p className="text-xs text-on-bg-secondary truncate mt-0.5 ml-5">
                      {area.description}
                    </p>
                  )}
                  <dl className="mt-3 ml-5 grid grid-cols-3 gap-2 text-xs">
                    {metrics && (
                      <div>
                        <dt className="text-on-bg-secondary">Health</dt>
                        <dd
                          className={
                            metrics.degradedDeviceCount > 0
                              ? 'font-medium text-critical'
                              : 'font-medium text-status-up'
                          }
                        >
                          {metrics.healthLabel}
                        </dd>
                      </div>
                    )}
                    <div>
                      <dt className="text-on-bg-secondary">Devices</dt>
                      <dd className="font-mono text-sm text-on-bg">
                        {area.device_count}{' '}
                        <span className="font-sans text-xs text-on-bg-secondary">
                          {area.device_count === 1 ? 'device' : 'devices'}
                        </span>
                      </dd>
                    </div>
                    {metrics && (
                      <div>
                        <dt className="text-on-bg-secondary">Links</dt>
                        <dd className="font-mono text-sm text-on-bg">{metrics.activeLinkCount}</dd>
                      </div>
                    )}
                  </dl>
                  <p className="sr-only">
                    {area.device_count} {area.device_count === 1 ? 'device' : 'devices'}
                  </p>
                </div>
                <div className="flex items-center gap-1 shrink-0">
                  {onOpenArea && (
                    <button
                      type="button"
                      onClick={() => onOpenArea(area.id)}
                      className="p-1 text-on-bg-secondary hover:text-on-bg rounded"
                      title="Open area"
                      aria-label={`Open area ${area.name}`}
                    >
                      <MaterialIcon name="hub" size={16} />
                    </button>
                  )}
                  {onCreateMapFromArea && (
                    <button
                      type="button"
                      onClick={() => onCreateMapFromArea(area)}
                      className="p-1 text-on-bg-secondary hover:text-on-bg rounded"
                      title="Create map from area"
                      aria-label={`Create map from area ${area.name}`}
                    >
                      <MaterialIcon name="add_location_alt" size={16} />
                    </button>
                  )}
                  <button
                    type="button"
                    onClick={() => {
                      setEditing(area);
                      setMode('edit');
                    }}
                    className="p-1 text-on-bg-secondary hover:text-on-bg rounded"
                    title="Edit area"
                    aria-label="edit area"
                  >
                    <svg
                      className="w-3.5 h-3.5"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
                      />
                    </svg>
                  </button>
                  <button
                    type="button"
                    onClick={() => setConfirmDeleteId(area.id)}
                    className="p-1 text-on-bg-secondary hover:text-status-down rounded"
                    title="Delete area"
                    aria-label="delete area"
                  >
                    <svg
                      className="w-3.5 h-3.5"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                      />
                    </svg>
                  </button>
                </div>
              </div>

              {confirmDeleteId === area.id && (
                <div className="mt-2 rounded-lg border border-status-down/30 bg-status-down/10 p-2 space-y-2">
                  <p className="text-xs text-status-down">
                    Delete this area? {area.device_count}{' '}
                    {area.device_count === 1 ? 'device' : 'devices'} will be unassigned.
                  </p>
                  <div className="flex gap-2">
                    <button
                      type="button"
                      onClick={() => setConfirmDeleteId(null)}
                      className="flex-1 rounded border border-outline-subtle bg-elevated px-2 py-1 text-xs text-on-bg hover:bg-surface-high"
                    >
                      Cancel
                    </button>
                    <button
                      type="button"
                      disabled={deleteLoading}
                      onClick={() => {
                        void handleDelete(area.id);
                      }}
                      className="flex-1 rounded bg-status-down px-2 py-1 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
                    >
                      {deleteLoading ? 'Deleting...' : 'Delete'}
                    </button>
                  </div>
                </div>
              )}
            </div>
          );
        })}
    </div>
  );
}
