/**
 * Renders credential profile manager UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import { useEffect, useState } from 'react';
import {
  createCredentialProfile,
  deleteCredentialProfile,
  fetchCredentialProfiles,
  updateCredentialProfile,
} from '../api/client';
import { ServerError, ValidationError } from '../api/errors';
import type { CredentialProfile } from '../types/api';
import {
  MAX_STRING_LENGTH,
  validateMaxLength,
  validatePort,
  validateRequired,
} from '../utils/validation';

const inputClass =
  'w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none';
const labelClass = 'text-xs font-medium uppercase tracking-widest text-on-bg-secondary';

type FormState = {
  name: string;
  description: string;
  username: string;
  port: string;
  authMethod: 'password' | 'key';
  secret: string;
  role: string;
};

function emptyForm(): FormState {
  return {
    name: '',
    description: '',
    username: 'admin',
    port: '22',
    authMethod: 'password',
    secret: '',
    role: 'Admin',
  };
}

function profileToForm(p: CredentialProfile): FormState {
  return {
    name: p.name,
    description: p.description,
    username: p.username,
    port: String(p.port),
    authMethod: p.auth_method,
    secret: '',
    role: p.role,
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
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  function setFieldError(field: string, err: string | null) {
    setFieldErrors((prev) => {
      if (err) return { ...prev, [field]: err };
      const next = { ...prev };
      delete next[field];
      return next;
    });
  }

  function handleBlur(field: string, validator: () => string | null) {
    return () => {
      const err = validator();
      setFieldError(field, err);
    };
  }

  function set<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((f) => ({ ...f, [key]: value }));
    setFieldError(key, null);
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();

    // Validate all fields before calling onSave
    const errors: Record<string, string> = {};
    const nameErr =
      validateRequired(form.name, 'Profile name') ??
      validateMaxLength(form.name, MAX_STRING_LENGTH, 'Profile name');
    if (nameErr) errors['name'] = nameErr;
    const descErr = validateMaxLength(form.description, MAX_STRING_LENGTH, 'Description');
    if (descErr) errors['description'] = descErr;
    const usernameErr =
      validateRequired(form.username, 'Username') ??
      validateMaxLength(form.username, MAX_STRING_LENGTH, 'Username');
    if (usernameErr) errors['username'] = usernameErr;
    const portErr = validatePort(form.port, 'Port');
    if (portErr) errors['port'] = portErr;
    if (!isEdit) {
      const secretErr = validateRequired(
        form.secret,
        form.authMethod === 'password' ? 'Password' : 'Private key',
      );
      if (secretErr) errors['secret'] = secretErr;
    }
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors);
      return;
    }

    setError(null);
    setLoading(true);
    try {
      await onSave(form);
    } catch (err) {
      if (err instanceof ServerError) {
        setError(
          err.correlationId
            ? `Something went wrong (ref: ${err.correlationId})`
            : 'Something went wrong',
        );
      } else if (err instanceof ValidationError) {
        setError(err.message);
      } else {
        setError(err instanceof Error ? err.message : 'Failed to save profile.');
      }
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
          Profile Name <span className="text-status-down">*</span>
        </label>
        <input
          type="text"
          value={form.name}
          onChange={(e) => set('name', e.target.value)}
          onBlur={handleBlur(
            'name',
            () =>
              validateRequired(form.name, 'Profile name') ??
              validateMaxLength(form.name, MAX_STRING_LENGTH, 'Profile name'),
          )}
          placeholder="e.g. MikroTik Admin"
          required
          className={`${inputClass}${fieldErrors['name'] ? ' border-status-down' : ''}`}
        />
        {fieldErrors['name'] && (
          <p className="mt-1 text-xs text-status-down">{fieldErrors['name']}</p>
        )}
      </div>

      <div className="space-y-1">
        <label className={labelClass}>Description</label>
        <input
          type="text"
          value={form.description}
          onChange={(e) => set('description', e.target.value)}
          onBlur={handleBlur('description', () =>
            validateMaxLength(form.description, MAX_STRING_LENGTH, 'Description'),
          )}
          placeholder="Optional description"
          className={`${inputClass}${fieldErrors['description'] ? ' border-status-down' : ''}`}
        />
        {fieldErrors['description'] && (
          <p className="mt-1 text-xs text-status-down">{fieldErrors['description']}</p>
        )}
      </div>

      <div className="space-y-1">
        <label className={labelClass}>Role</label>
        <input
          type="text"
          value={form.role}
          onChange={(e) => set('role', e.target.value)}
          placeholder="e.g. Admin"
          className={inputClass}
        />
      </div>

      <div className="space-y-1">
        <label className={labelClass}>Username</label>
        <input
          type="text"
          value={form.username}
          onChange={(e) => set('username', e.target.value)}
          onBlur={handleBlur(
            'username',
            () =>
              validateRequired(form.username, 'Username') ??
              validateMaxLength(form.username, MAX_STRING_LENGTH, 'Username'),
          )}
          placeholder="admin"
          className={`${inputClass}${fieldErrors['username'] ? ' border-status-down' : ''}`}
        />
        {fieldErrors['username'] && (
          <p className="mt-1 text-xs text-status-down">{fieldErrors['username']}</p>
        )}
      </div>

      <div className="space-y-1">
        <label className={labelClass}>Port</label>
        <input
          type="number"
          value={form.port}
          onChange={(e) => set('port', e.target.value)}
          onBlur={handleBlur('port', () => validatePort(form.port, 'Port'))}
          placeholder="22"
          className={`${inputClass}${fieldErrors['port'] ? ' border-status-down' : ''}`}
        />
        {fieldErrors['port'] && (
          <p className="mt-1 text-xs text-status-down">{fieldErrors['port']}</p>
        )}
      </div>

      <div className="space-y-1">
        <label className={labelClass}>Auth Method</label>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => set('authMethod', 'password')}
            className={`flex-1 rounded-lg border px-3 py-2 text-xs font-medium transition-colors ${
              form.authMethod === 'password'
                ? 'border-primary bg-primary/15 text-primary'
                : 'border-outline-subtle text-on-bg-secondary hover:text-on-bg'
            }`}
          >
            Password
          </button>
          <button
            type="button"
            onClick={() => set('authMethod', 'key')}
            className={`flex-1 rounded-lg border px-3 py-2 text-xs font-medium transition-colors ${
              form.authMethod === 'key'
                ? 'border-primary bg-primary/15 text-primary'
                : 'border-outline-subtle text-on-bg-secondary hover:text-on-bg'
            }`}
          >
            Private Key
          </button>
        </div>
      </div>

      <div className="space-y-1">
        <label className={labelClass}>
          {form.authMethod === 'password' ? 'Password' : 'Private Key'}
          {!isEdit && <span className="text-status-down"> *</span>}
        </label>
        {form.authMethod === 'password' ? (
          <input
            type="password"
            value={form.secret}
            onChange={(e) => set('secret', e.target.value)}
            onBlur={
              !isEdit
                ? handleBlur('secret', () => validateRequired(form.secret, 'Password'))
                : undefined
            }
            placeholder={isEdit ? '(unchanged if blank)' : 'Enter password'}
            autoComplete="new-password"
            className={`${inputClass}${fieldErrors['secret'] ? ' border-status-down' : ''}`}
          />
        ) : (
          <textarea
            value={form.secret}
            onChange={(e) => set('secret', e.target.value)}
            onBlur={
              !isEdit
                ? handleBlur('secret', () => validateRequired(form.secret, 'Private key'))
                : undefined
            }
            placeholder={isEdit ? '(unchanged if blank)' : 'Paste private key'}
            rows={4}
            className={`${inputClass} font-mono text-xs${fieldErrors['secret'] ? ' border-status-down' : ''}`}
          />
        )}
        {fieldErrors['secret'] && (
          <p className="mt-1 text-xs text-status-down">{fieldErrors['secret']}</p>
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
          className="flex-1 rounded-lg bg-surface-high px-3 py-2 text-sm text-on-bg hover:bg-elevated"
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

/** Renders the CredentialProfileManager component within the UI component boundary. */
export function CredentialProfileManager() {
  const [profiles, setProfiles] = useState<CredentialProfile[]>([]);
  const [loading, setLoading] = useState(true);
  const [mode, setMode] = useState<'list' | 'create' | 'edit'>('list');
  const [editing, setEditing] = useState<CredentialProfile | null>(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  async function load() {
    setLoading(true);
    try {
      setProfiles(await fetchCredentialProfiles());
    } catch {
      // non-fatal
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  function formToPayload(form: FormState) {
    return {
      name: form.name.trim(),
      description: form.description.trim(),
      username: form.username.trim() || 'admin',
      port: parseInt(form.port, 10) || 22,
      auth_method: form.authMethod,
      secret: form.secret,
      role: form.role.trim(),
    };
  }

  async function handleCreate(form: FormState) {
    await createCredentialProfile(formToPayload(form));
    setMode('list');
    void load();
  }

  async function handleUpdate(form: FormState) {
    if (!editing) return;
    await updateCredentialProfile(editing.id, formToPayload(form));
    setMode('list');
    setEditing(null);
    void load();
  }

  async function handleDelete(id: string) {
    setDeleteLoading(true);
    setDeleteError(null);
    try {
      await deleteCredentialProfile(id);
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
      <div className="space-y-3 transition-colors duration-200">
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
          <p className={labelClass}>New Credential Profile</p>
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
      <div className="space-y-3 transition-colors duration-200">
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
          <p className={labelClass}>Edit Credential Profile</p>
        </div>
        <ProfileForm
          initial={profileToForm(editing)}
          onSave={handleUpdate}
          onCancel={() => {
            setMode('list');
            setEditing(null);
          }}
          saveLabel="Save Changes"
          isEdit
        />
      </div>
    );
  }

  return (
    <div className="space-y-3 transition-colors duration-200">
      <div className="flex items-center justify-between">
        <p className={labelClass}>Credential Profiles</p>
        <button
          type="button"
          onClick={() => setMode('create')}
          className="flex items-center gap-1 rounded-lg bg-surface-high px-2 py-1 text-xs text-on-bg hover:bg-elevated"
        >
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          New
        </button>
      </div>

      {loading && <p className="text-xs text-on-bg-secondary">Loading profiles...</p>}

      {!loading && profiles.length === 0 && (
        <p className="text-xs text-on-bg-secondary">
          No credential profiles yet. Create one to reuse credentials across devices.
        </p>
      )}

      {!loading &&
        profiles.map((profile) => (
          <div key={profile.id} className="rounded-lg bg-surface-high p-3 space-y-1">
            <div className="flex items-start justify-between gap-2">
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium text-on-bg truncate">{profile.name}</p>
                {profile.description && (
                  <p className="text-xs text-on-bg-secondary truncate">{profile.description}</p>
                )}
                {profile.role && (
                  <span className="text-xs text-on-bg-secondary">Role: {profile.role}</span>
                )}
                <p className="text-xs text-on-bg-secondary/60 mt-1">
                  {profile.username}:{profile.port} ({profile.auth_method})
                </p>
              </div>
              <div className="flex items-center gap-1 shrink-0">
                <button
                  type="button"
                  onClick={() => {
                    setEditing(profile);
                    setMode('edit');
                  }}
                  className="p-1 text-on-bg-secondary hover:text-on-bg rounded"
                  title="Edit profile"
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
                  onClick={() => {
                    setConfirmDeleteId(profile.id);
                    setDeleteError(null);
                  }}
                  className="p-1 text-on-bg-secondary hover:text-status-down rounded"
                  title="Delete profile"
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

            {confirmDeleteId === profile.id && (
              <div className="mt-2 rounded-lg border border-status-down/30 bg-status-down/10 p-2 space-y-2">
                <p className="text-xs text-status-down">Delete this profile?</p>
                {deleteError && <p className="text-xs text-status-down">{deleteError}</p>}
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => {
                      setConfirmDeleteId(null);
                      setDeleteError(null);
                    }}
                    className="flex-1 rounded bg-surface-high px-2 py-1 text-xs text-on-bg hover:bg-elevated"
                  >
                    Cancel
                  </button>
                  <button
                    type="button"
                    disabled={deleteLoading}
                    onClick={() => {
                      void handleDelete(profile.id);
                    }}
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
