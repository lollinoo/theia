import { useEffect, useState } from 'react';
import {
  createSNMPProfile,
  deleteSNMPProfile,
  fetchSNMPProfiles,
  updateSNMPProfile,
} from '../api/client';
import { ServerError, ValidationError } from '../api/errors';
import type { SNMPProfile } from '../types/api';
import {
  MAX_STRING_LENGTH,
  validateMaxLength,
  validateRequired,
  validateSNMPv3Auth,
  validateSNMPv3Priv,
  validateSNMPv3SecurityLevel,
} from '../utils/validation';

const inputClass =
  'w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none';
const selectClass =
  'w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none';
const labelClass = 'text-xs font-medium uppercase tracking-widest text-on-bg-secondary';

type FormState = {
  name: string;
  description: string;
  version: string;
  community: string;
  username: string;
  securityLevel: string;
  authProtocol: string;
  authPassword: string;
  privProtocol: string;
  privPassword: string;
};

function emptyForm(): FormState {
  return {
    name: '',
    description: '',
    version: '2c',
    community: 'public',
    username: '',
    securityLevel: 'authPriv',
    authProtocol: 'SHA',
    authPassword: '',
    privProtocol: 'AES',
    privPassword: '',
  };
}

function profileToForm(p: SNMPProfile): FormState {
  return {
    name: p.name,
    description: p.description,
    version: p.snmp.version,
    community: p.snmp.community ?? 'public',
    username: p.snmp.username ?? '',
    securityLevel: p.snmp.security_level ?? 'authPriv',
    authProtocol: p.snmp.auth_protocol ?? 'SHA',
    authPassword: p.snmp.auth_password ?? '',
    privProtocol: p.snmp.priv_protocol ?? 'AES',
    privPassword: p.snmp.priv_password ?? '',
  };
}

interface ProfileFormProps {
  initial: FormState;
  onSave: (form: FormState) => Promise<void>;
  onCancel: () => void;
  saveLabel: string;
}

function ProfileForm({ initial, onSave, onCancel, saveLabel }: ProfileFormProps) {
  const [form, setForm] = useState<FormState>(initial);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  const isV3 = form.version === '3';
  const needsAuth = form.securityLevel === 'authNoPriv' || form.securityLevel === 'authPriv';
  const needsPriv = form.securityLevel === 'authPriv';

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

  function set(key: keyof FormState, value: string) {
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
    if (!isV3) {
      const communityErr = validateMaxLength(form.community, MAX_STRING_LENGTH, 'Community string');
      if (communityErr) errors['community'] = communityErr;
    } else {
      const usernameErr = validateMaxLength(form.username, MAX_STRING_LENGTH, 'Username');
      if (usernameErr) errors['username'] = usernameErr;
      const secLevelErr = validateSNMPv3SecurityLevel(form.securityLevel);
      if (secLevelErr) errors['securityLevel'] = secLevelErr;
      if (needsAuth) {
        const authErr = validateSNMPv3Auth(form.authProtocol);
        if (authErr) errors['authProtocol'] = authErr;
      }
      if (needsPriv) {
        const privErr = validateSNMPv3Priv(form.privProtocol);
        if (privErr) errors['privProtocol'] = privErr;
      }
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
          placeholder="e.g. Office SNMPv3"
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
        <label className={labelClass}>SNMP Version</label>
        <select
          value={form.version}
          onChange={(e) => set('version', e.target.value)}
          className={selectClass}
        >
          <option value="2c">v2c</option>
          <option value="3">v3</option>
        </select>
      </div>

      {!isV3 && (
        <div className="space-y-1">
          <label className={labelClass}>Community String</label>
          <input
            type="text"
            value={form.community}
            onChange={(e) => set('community', e.target.value)}
            onBlur={handleBlur('community', () =>
              validateMaxLength(form.community, MAX_STRING_LENGTH, 'Community string'),
            )}
            placeholder="public"
            className={`${inputClass}${fieldErrors['community'] ? ' border-status-down' : ''}`}
          />
          {fieldErrors['community'] && (
            <p className="mt-1 text-xs text-status-down">{fieldErrors['community']}</p>
          )}
        </div>
      )}

      {isV3 && (
        <div className="space-y-3 bg-surface-high rounded-lg p-3">
          <p className={labelClass}>SNMPv3 Credentials</p>

          <div className="space-y-1">
            <label className="text-xs text-on-bg-secondary">Username</label>
            <input
              type="text"
              value={form.username}
              onChange={(e) => set('username', e.target.value)}
              onBlur={handleBlur('username', () =>
                validateMaxLength(form.username, MAX_STRING_LENGTH, 'Username'),
              )}
              placeholder="snmpv3user"
              className={`${inputClass}${fieldErrors['username'] ? ' border-status-down' : ''}`}
            />
            {fieldErrors['username'] && (
              <p className="mt-1 text-xs text-status-down">{fieldErrors['username']}</p>
            )}
          </div>

          <div className="space-y-1">
            <label className="text-xs text-on-bg-secondary">Security Level</label>
            <select
              value={form.securityLevel}
              onChange={(e) => set('securityLevel', e.target.value)}
              className={`${selectClass}${fieldErrors['securityLevel'] ? ' border-status-down' : ''}`}
            >
              <option value="noAuthNoPriv">No Auth, No Privacy</option>
              <option value="authNoPriv">Auth, No Privacy</option>
              <option value="authPriv">Auth + Privacy</option>
            </select>
            {fieldErrors['securityLevel'] && (
              <p className="mt-1 text-xs text-status-down">{fieldErrors['securityLevel']}</p>
            )}
          </div>

          {needsAuth && (
            <>
              <div className="space-y-1">
                <label className="text-xs text-on-bg-secondary">Auth Protocol</label>
                <select
                  value={form.authProtocol}
                  onChange={(e) => set('authProtocol', e.target.value)}
                  className={`${selectClass}${fieldErrors['authProtocol'] ? ' border-status-down' : ''}`}
                >
                  <option value="SHA">SHA</option>
                  <option value="MD5">MD5</option>
                  <option value="SHA-224">SHA-224</option>
                  <option value="SHA-256">SHA-256</option>
                  <option value="SHA-384">SHA-384</option>
                  <option value="SHA-512">SHA-512</option>
                </select>
                {fieldErrors['authProtocol'] && (
                  <p className="mt-1 text-xs text-status-down">{fieldErrors['authProtocol']}</p>
                )}
              </div>
              <div className="space-y-1">
                <label className="text-xs text-on-bg-secondary">Auth Key</label>
                <input
                  type="password"
                  value={form.authPassword}
                  onChange={(e) => set('authPassword', e.target.value)}
                  placeholder="Authentication passphrase"
                  autoComplete="new-password"
                  className={inputClass}
                />
              </div>
            </>
          )}

          {needsPriv && (
            <>
              <div className="space-y-1">
                <label className="text-xs text-on-bg-secondary">Encryption Protocol</label>
                <select
                  value={form.privProtocol}
                  onChange={(e) => set('privProtocol', e.target.value)}
                  className={`${selectClass}${fieldErrors['privProtocol'] ? ' border-status-down' : ''}`}
                >
                  <option value="AES">AES</option>
                  <option value="DES">DES</option>
                </select>
                {fieldErrors['privProtocol'] && (
                  <p className="mt-1 text-xs text-status-down">{fieldErrors['privProtocol']}</p>
                )}
              </div>
              <div className="space-y-1">
                <label className="text-xs text-on-bg-secondary">Encryption Key</label>
                <input
                  type="password"
                  value={form.privPassword}
                  onChange={(e) => set('privPassword', e.target.value)}
                  placeholder="Privacy passphrase"
                  autoComplete="new-password"
                  className={inputClass}
                />
              </div>
            </>
          )}
        </div>
      )}

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

export function SNMPProfileManager() {
  const [profiles, setProfiles] = useState<SNMPProfile[]>([]);
  const [loading, setLoading] = useState(true);
  const [mode, setMode] = useState<'list' | 'create' | 'edit'>('list');
  const [editing, setEditing] = useState<SNMPProfile | null>(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);

  async function load() {
    setLoading(true);
    try {
      setProfiles(await fetchSNMPProfiles());
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
    const isV3 = form.version === '3';
    const needsAuth = form.securityLevel === 'authNoPriv' || form.securityLevel === 'authPriv';
    const needsPriv = form.securityLevel === 'authPriv';
    return {
      name: form.name.trim(),
      description: form.description.trim(),
      snmp: isV3
        ? {
            version: '3' as const,
            username: form.username.trim(),
            security_level: form.securityLevel,
            ...(needsAuth
              ? { auth_protocol: form.authProtocol, auth_password: form.authPassword }
              : {}),
            ...(needsPriv
              ? { priv_protocol: form.privProtocol, priv_password: form.privPassword }
              : {}),
          }
        : {
            version: form.version,
            community: form.community.trim() || 'public',
          },
    };
  }

  async function handleCreate(form: FormState) {
    await createSNMPProfile(formToPayload(form));
    setMode('list');
    void load();
  }

  async function handleUpdate(form: FormState) {
    if (!editing) return;
    await updateSNMPProfile(editing.id, formToPayload(form));
    setMode('list');
    setEditing(null);
    void load();
  }

  async function handleDelete(id: string) {
    setDeleteLoading(true);
    try {
      await deleteSNMPProfile(id);
      setConfirmDeleteId(null);
      void load();
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
          <p className={labelClass}>New Profile</p>
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
          <p className={labelClass}>Edit Profile</p>
        </div>
        <ProfileForm
          initial={profileToForm(editing)}
          onSave={handleUpdate}
          onCancel={() => {
            setMode('list');
            setEditing(null);
          }}
          saveLabel="Save Changes"
        />
      </div>
    );
  }

  return (
    <div className="space-y-3 transition-colors duration-200">
      <div className="flex items-center justify-between">
        <p className={labelClass}>SNMP Profiles</p>
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
          No profiles yet. Create one to reuse credentials across devices.
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
                <p className="text-xs text-on-bg-secondary/60 mt-1">
                  SNMP {profile.snmp.version}
                  {profile.snmp.version === '2c' &&
                    profile.snmp.community &&
                    ` · ${profile.snmp.community}`}
                  {profile.snmp.version === '3' &&
                    profile.snmp.username &&
                    ` · ${profile.snmp.username}`}
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
                  onClick={() => setConfirmDeleteId(profile.id)}
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
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => setConfirmDeleteId(null)}
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
