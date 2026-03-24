import { useEffect, useState } from 'react';
import type { SSHProfile } from '../types/api';
import {
  createSSHProfile,
  deleteSSHProfile,
  fetchSSHProfiles,
  updateSSHProfile,
} from '../api/client';

const inputClass =
  'w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none';
const labelClass = 'text-xs font-medium uppercase tracking-widest text-text-secondary';

type FormState = {
  name: string;
  description: string;
  username: string;
  port: string;
  authMethod: 'password' | 'key';
  secret: string;
};

function emptyForm(): FormState {
  return {
    name: '',
    description: '',
    username: 'admin',
    port: '22',
    authMethod: 'password',
    secret: '',
  };
}

function profileToForm(p: SSHProfile): FormState {
  return {
    name: p.name,
    description: p.description,
    username: p.username,
    port: String(p.port),
    authMethod: p.auth_method,
    secret: '',
  };
}

interface ProfileFormProps {
  initial: FormState;
  onSave: (form: FormState) => Promise<void>;
  onCancel: () => void;
  saveLabel: string;
  isEdit?: boolean;
}

function ProfileForm({ initial, onSave, onCancel, saveLabel, isEdit }: ProfileFormProps) {
  const [form, setForm] = useState<FormState>(initial);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function set<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((f) => ({ ...f, [key]: value }));
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      await onSave(form);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save profile.');
    } finally {
      setLoading(false);
    }
  }

  return (
    <form onSubmit={(e) => { void handleSubmit(e); }} className="space-y-3">
      <div className="space-y-1">
        <label className={labelClass}>Profile Name <span className="text-status-down">*</span></label>
        <input
          type="text"
          value={form.name}
          onChange={(e) => set('name', e.target.value)}
          placeholder="e.g. MikroTik Admin"
          required
          className={inputClass}
        />
      </div>

      <div className="space-y-1">
        <label className={labelClass}>Description</label>
        <input
          type="text"
          value={form.description}
          onChange={(e) => set('description', e.target.value)}
          placeholder="Optional description"
          className={inputClass}
        />
      </div>

      <div className="space-y-1">
        <label className={labelClass}>Username</label>
        <input
          type="text"
          value={form.username}
          onChange={(e) => set('username', e.target.value)}
          placeholder="admin"
          className={inputClass}
        />
      </div>

      <div className="space-y-1">
        <label className={labelClass}>Port</label>
        <input
          type="number"
          value={form.port}
          onChange={(e) => set('port', e.target.value)}
          placeholder="22"
          className={inputClass}
        />
      </div>

      <div className="space-y-1">
        <label className={labelClass}>Auth Method</label>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => set('authMethod', 'password')}
            className={`flex-1 rounded-lg border px-3 py-2 text-xs font-medium transition-colors ${
              form.authMethod === 'password'
                ? 'border-accent bg-accent/15 text-accent'
                : 'border-border-subtle text-text-secondary hover:text-text-primary'
            }`}
          >
            Password
          </button>
          <button
            type="button"
            onClick={() => set('authMethod', 'key')}
            className={`flex-1 rounded-lg border px-3 py-2 text-xs font-medium transition-colors ${
              form.authMethod === 'key'
                ? 'border-accent bg-accent/15 text-accent'
                : 'border-border-subtle text-text-secondary hover:text-text-primary'
            }`}
          >
            Private Key
          </button>
        </div>
      </div>

      <div className="space-y-1">
        <label className={labelClass}>
          {form.authMethod === 'password' ? 'Password' : 'Private Key'}
        </label>
        {form.authMethod === 'password' ? (
          <input
            type="password"
            value={form.secret}
            onChange={(e) => set('secret', e.target.value)}
            placeholder={isEdit ? '(unchanged if blank)' : 'Enter password'}
            autoComplete="new-password"
            className={inputClass}
          />
        ) : (
          <textarea
            value={form.secret}
            onChange={(e) => set('secret', e.target.value)}
            placeholder={isEdit ? '(unchanged if blank)' : 'Paste private key'}
            rows={4}
            className={`${inputClass} font-mono text-xs`}
          />
        )}
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
          className="flex-1 rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary hover:bg-bg-surface"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={loading}
          className="flex-1 rounded-lg bg-accent px-3 py-2 text-sm font-medium text-white hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {loading ? 'Saving...' : saveLabel}
        </button>
      </div>
    </form>
  );
}

export function SSHProfileManager() {
  const [profiles, setProfiles] = useState<SSHProfile[]>([]);
  const [loading, setLoading] = useState(true);
  const [mode, setMode] = useState<'list' | 'create' | 'edit'>('list');
  const [editing, setEditing] = useState<SSHProfile | null>(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  async function load() {
    setLoading(true);
    try {
      setProfiles(await fetchSSHProfiles());
    } catch {
      // non-fatal
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void load(); }, []);

  function formToPayload(form: FormState) {
    return {
      name: form.name.trim(),
      description: form.description.trim(),
      username: form.username.trim() || 'admin',
      port: parseInt(form.port, 10) || 22,
      auth_method: form.authMethod,
      secret: form.secret,
    };
  }

  async function handleCreate(form: FormState) {
    await createSSHProfile(formToPayload(form));
    setMode('list');
    void load();
  }

  async function handleUpdate(form: FormState) {
    if (!editing) return;
    await updateSSHProfile(editing.id, formToPayload(form));
    setMode('list');
    setEditing(null);
    void load();
  }

  async function handleDelete(id: string) {
    setDeleteLoading(true);
    setDeleteError(null);
    try {
      await deleteSSHProfile(id);
      setConfirmDeleteId(null);
      void load();
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed');
    } finally {
      setDeleteLoading(false);
    }
  }

  if (mode === 'create') {
    return (
      <div className="space-y-3">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setMode('list')}
            className="text-text-secondary hover:text-text-primary"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
          </button>
          <p className={labelClass}>New SSH Profile</p>
        </div>
        <ProfileForm
          initial={emptyForm()}
          onSave={handleCreate}
          onCancel={() => setMode('list')}
          saveLabel="Create Profile"
        />
      </div>
    );
  }

  if (mode === 'edit' && editing) {
    return (
      <div className="space-y-3">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => { setMode('list'); setEditing(null); }}
            className="text-text-secondary hover:text-text-primary"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
          </button>
          <p className={labelClass}>Edit SSH Profile</p>
        </div>
        <ProfileForm
          initial={profileToForm(editing)}
          onSave={handleUpdate}
          onCancel={() => { setMode('list'); setEditing(null); }}
          saveLabel="Save Changes"
          isEdit
        />
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <p className={labelClass}>SSH Profiles</p>
        <button
          type="button"
          onClick={() => setMode('create')}
          className="flex items-center gap-1 rounded-lg border border-border-subtle bg-bg-elevated px-2 py-1 text-xs text-text-primary hover:bg-bg-surface"
        >
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          New
        </button>
      </div>

      {loading && (
        <p className="text-xs text-text-secondary">Loading profiles...</p>
      )}

      {!loading && profiles.length === 0 && (
        <p className="text-xs text-text-secondary">
          No SSH profiles yet. Create one to reuse credentials across devices.
        </p>
      )}

      {!loading && profiles.map((profile) => (
        <div
          key={profile.id}
          className="rounded-lg border border-border-subtle bg-bg-elevated p-3 space-y-1"
        >
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium text-text-primary truncate">{profile.name}</p>
              {profile.description && (
                <p className="text-xs text-text-secondary truncate">{profile.description}</p>
              )}
              <p className="text-xs text-text-secondary/60 mt-1">
                {profile.username}:{profile.port} ({profile.auth_method})
              </p>
            </div>
            <div className="flex items-center gap-1 shrink-0">
              <button
                type="button"
                onClick={() => { setEditing(profile); setMode('edit'); }}
                className="p-1 text-text-secondary hover:text-text-primary rounded"
                title="Edit profile"
              >
                <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                </svg>
              </button>
              <button
                type="button"
                onClick={() => { setConfirmDeleteId(profile.id); setDeleteError(null); }}
                className="p-1 text-text-secondary hover:text-status-down rounded"
                title="Delete profile"
              >
                <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                </svg>
              </button>
            </div>
          </div>

          {confirmDeleteId === profile.id && (
            <div className="mt-2 rounded-lg border border-status-down/30 bg-status-down/10 p-2 space-y-2">
              <p className="text-xs text-status-down">Delete this profile?</p>
              {deleteError && (
                <p className="text-xs text-status-down">{deleteError}</p>
              )}
              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={() => { setConfirmDeleteId(null); setDeleteError(null); }}
                  className="flex-1 rounded border border-border-subtle bg-bg-elevated px-2 py-1 text-xs text-text-primary hover:bg-bg-surface"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  disabled={deleteLoading}
                  onClick={() => { void handleDelete(profile.id); }}
                  className="flex-1 rounded bg-status-down px-2 py-1 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
                >
                  {deleteLoading ? 'Deleting...' : 'Delete'}
                </button>
              </div>
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
