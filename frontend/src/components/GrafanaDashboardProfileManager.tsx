import { useEffect, useState } from 'react';
import {
  createGrafanaDashboardProfile,
  deleteGrafanaDashboardProfile,
  fetchGrafanaDashboardConfig,
  updateGrafanaDashboardProfile,
} from '../api/client';
import type {
  GrafanaDashboardConfig,
  GrafanaDashboardProfile,
  GrafanaVariableSource,
} from '../types/api';
import { validateRequired, validateURL } from '../utils/validation';
import { MaterialIcon } from './MaterialIcon';

const inputClass =
  'w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none';
const labelClass = 'text-xs font-medium uppercase tracking-widest text-on-bg-secondary';

type FormState = {
  name: string;
  urlTemplate: string;
  variableSource: GrafanaVariableSource;
  isDefault: boolean;
};

const variableSourceLabels: Record<GrafanaVariableSource, string> = {
  hostname: 'Hostname',
  ip: 'IP Address',
  map_name: 'Map Name',
  map_id: 'Map ID',
};

function emptyForm(): FormState {
  return {
    name: '',
    urlTemplate: 'https://grafana.example/d/router?var-device={{hostname}}',
    variableSource: 'hostname',
    isDefault: false,
  };
}

function profileToForm(
  profile: GrafanaDashboardProfile,
  config: GrafanaDashboardConfig,
): FormState {
  return {
    name: profile.name,
    urlTemplate: profile.url_template,
    variableSource: profile.variable_source,
    isDefault: config.default_profile_id === profile.id,
  };
}

function validateTemplateURL(value: string): string | null {
  const probe = value.replace(/\{\{\s*(hostname|ip|map_name|map_id)\s*\}\}/g, 'theia-placeholder');
  if (probe.includes('{{') || probe.includes('}}')) {
    return 'URL template contains an unsupported placeholder';
  }
  return validateURL(probe, 'Grafana URL template');
}

interface ProfileFormProps {
  initial: FormState;
  saveLabel: string;
  onSave: (form: FormState) => Promise<void>;
  onCancel: () => void;
}

function ProfileForm({ initial, saveLabel, onSave, onCancel }: ProfileFormProps) {
  const [form, setForm] = useState<FormState>(initial);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  function set<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((current) => ({ ...current, [key]: value }));
    setFieldErrors((current) => {
      const next = { ...current };
      delete next[key];
      return next;
    });
  }

  async function handleSubmit(event: React.FormEvent) {
    event.preventDefault();
    const errors: Record<string, string> = {};
    const nameError = validateRequired(form.name, 'Profile name');
    if (nameError) errors.name = nameError;
    const urlError = validateTemplateURL(form.urlTemplate);
    if (urlError) errors.urlTemplate = urlError;
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors);
      return;
    }

    setSaving(true);
    setError(null);
    try {
      await onSave(form);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save Grafana dashboard profile');
    } finally {
      setSaving(false);
    }
  }

  return (
    <form
      onSubmit={(event) => {
        void handleSubmit(event);
      }}
      className="space-y-3"
    >
      <label className="grid gap-1">
        <span className={labelClass}>Profile Name</span>
        <input
          value={form.name}
          onChange={(event) => set('name', event.target.value)}
          className={`${inputClass}${fieldErrors.name ? ' border-status-down' : ''}`}
          placeholder="RouterBoard shared"
        />
        {fieldErrors.name && <span className="text-xs text-status-down">{fieldErrors.name}</span>}
      </label>

      <label className="grid gap-1">
        <span className={labelClass}>Dashboard URL Template</span>
        <input
          type="url"
          value={form.urlTemplate}
          onChange={(event) => set('urlTemplate', event.target.value)}
          className={`${inputClass}${fieldErrors.urlTemplate ? ' border-status-down' : ''}`}
          placeholder="https://grafana.example/d/router?var-device={{hostname}}"
        />
        {fieldErrors.urlTemplate ? (
          <span className="text-xs text-status-down">{fieldErrors.urlTemplate}</span>
        ) : (
          <span className="text-xs text-on-bg-secondary">
            Supports {'{{hostname}}'}, {'{{ip}}'}, {'{{map_name}}'}, {'{{map_id}}'}.
          </span>
        )}
      </label>

      <label className="grid gap-1">
        <span className={labelClass}>Primary Variable</span>
        <select
          value={form.variableSource}
          onChange={(event) => set('variableSource', event.target.value as GrafanaVariableSource)}
          className={inputClass}
        >
          {Object.entries(variableSourceLabels).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </select>
      </label>

      <label className="flex items-center gap-2 text-sm text-on-bg-secondary">
        <input
          type="checkbox"
          checked={form.isDefault}
          onChange={(event) => set('isDefault', event.target.checked)}
          className="h-4 w-4 rounded border-outline-subtle bg-elevated text-primary"
        />
        Use as default dashboard profile
      </label>

      {error && (
        <p className="rounded-lg border border-status-down/30 bg-status-down/10 px-3 py-2 text-xs text-status-down">
          {error}
        </p>
      )}

      <div className="flex gap-2">
        <button
          type="button"
          onClick={onCancel}
          className="flex-1 rounded-lg bg-surface-high px-3 py-2 text-sm text-on-bg hover:bg-elevated"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={saving}
          className="flex-1 rounded-lg bg-primary px-3 py-2 text-sm font-medium text-white hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {saving ? 'Saving...' : saveLabel}
        </button>
      </div>
    </form>
  );
}

export function GrafanaDashboardProfileManager() {
  const [config, setConfig] = useState<GrafanaDashboardConfig>({
    profiles: [],
    default_profile_id: '',
    device_overrides: {},
  });
  const [loading, setLoading] = useState(true);
  const [mode, setMode] = useState<'list' | 'create' | 'edit'>('list');
  const [editing, setEditing] = useState<GrafanaDashboardProfile | null>(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);

  async function load() {
    setLoading(true);
    try {
      setConfig(await fetchGrafanaDashboardConfig());
    } catch {
      // non-fatal
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  function toPayload(form: FormState) {
    return {
      name: form.name.trim(),
      url_template: form.urlTemplate.trim(),
      variable_source: form.variableSource,
      is_default: form.isDefault,
    };
  }

  async function handleCreate(form: FormState) {
    setConfig(await createGrafanaDashboardProfile(toPayload(form)));
    setMode('list');
  }

  async function handleUpdate(form: FormState) {
    if (!editing) return;
    setConfig(await updateGrafanaDashboardProfile(editing.id, toPayload(form)));
    setEditing(null);
    setMode('list');
  }

  async function handleDelete(id: string) {
    await deleteGrafanaDashboardProfile(id);
    setConfirmDeleteId(null);
    void load();
  }

  if (mode === 'create') {
    return (
      <div className="min-w-0 space-y-3">
        <button
          type="button"
          onClick={() => setMode('list')}
          className="flex items-center gap-1 text-xs font-medium uppercase tracking-widest text-on-bg-secondary hover:text-on-bg"
        >
          <MaterialIcon name="close" className="text-base" />
          Grafana Profiles
        </button>
        <ProfileForm
          initial={emptyForm()}
          saveLabel="Create Profile"
          onSave={handleCreate}
          onCancel={() => setMode('list')}
        />
      </div>
    );
  }

  if (mode === 'edit' && editing) {
    return (
      <div className="min-w-0 space-y-3">
        <button
          type="button"
          onClick={() => {
            setEditing(null);
            setMode('list');
          }}
          className="flex items-center gap-1 text-xs font-medium uppercase tracking-widest text-on-bg-secondary hover:text-on-bg"
        >
          <MaterialIcon name="close" className="text-base" />
          Grafana Profiles
        </button>
        <ProfileForm
          initial={profileToForm(editing, config)}
          saveLabel="Save Changes"
          onSave={handleUpdate}
          onCancel={() => {
            setEditing(null);
            setMode('list');
          }}
        />
      </div>
    );
  }

  return (
    <div className="min-w-0 space-y-3">
      <div className="flex items-center justify-between gap-3">
        <p className={labelClass}>Grafana Dashboard Profiles</p>
        <button
          type="button"
          onClick={() => setMode('create')}
          className="flex items-center gap-1 rounded-lg bg-surface-high px-2 py-1 text-xs text-on-bg hover:bg-elevated"
        >
          <MaterialIcon name="add" className="text-sm" />
          New
        </button>
      </div>

      {loading && <p className="text-xs text-on-bg-secondary">Loading profiles...</p>}
      {!loading && config.profiles.length === 0 && (
        <p className="text-xs text-on-bg-secondary">
          No Grafana dashboard profiles yet. Create one to share dashboard templates.
        </p>
      )}
      {!loading &&
        config.profiles.map((profile) => (
          <div
            key={profile.id}
            data-testid={`grafana-profile-card-${profile.id}`}
            className="max-w-full min-w-0 space-y-2 overflow-hidden rounded-lg bg-surface-high p-3"
          >
            <div className="flex min-w-0 items-start justify-between gap-2">
              <div className="min-w-0 flex-1 overflow-hidden">
                <p className="truncate text-sm font-medium text-on-bg">{profile.name}</p>
                <p className="block max-w-full truncate font-mono text-xs text-on-bg-secondary">
                  {profile.url_template}
                </p>
                <p className="mt-1 text-xs text-on-bg-secondary">
                  {variableSourceLabels[profile.variable_source]}
                  {config.default_profile_id === profile.id ? ' · Default' : ''}
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <button
                  type="button"
                  title="Edit profile"
                  onClick={() => {
                    setEditing(profile);
                    setMode('edit');
                  }}
                  className="rounded p-1 text-on-bg-secondary hover:text-on-bg"
                >
                  <MaterialIcon name="edit" className="text-base" />
                </button>
                <button
                  type="button"
                  title="Delete profile"
                  onClick={() => setConfirmDeleteId(profile.id)}
                  className="rounded p-1 text-on-bg-secondary hover:text-status-down"
                >
                  <MaterialIcon name="delete" className="text-base" />
                </button>
              </div>
            </div>
            {confirmDeleteId === profile.id && (
              <div className="flex items-center justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setConfirmDeleteId(null)}
                  className="rounded-md px-2 py-1 text-xs text-on-bg-secondary hover:text-on-bg"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={() => {
                    void handleDelete(profile.id);
                  }}
                  className="rounded-md bg-status-down px-2 py-1 text-xs font-medium text-white"
                >
                  Delete
                </button>
              </div>
            )}
          </div>
        ))}
    </div>
  );
}
