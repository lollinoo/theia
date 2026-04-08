import { useState, useEffect, useCallback } from 'react';
import type { CredentialProfile } from '../../types/api';
import {
  fetchCredentialProfiles,
  createCredentialProfile,
  assignCredentialProfile,
  unassignCredentialProfile,
  testSSHConnection,
} from '../../api/client';

interface SSHCredentialFormProps {
  deviceId: string;
  currentProfileId?: string;
  onProfileChanged?: (profileId: string | undefined) => void;
}

const inputClass =
  'w-full rounded-md border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted outline-none focus:border-primary focus:ring-1 focus:ring-primary/30 transition-colors';
const selectClass =
  'w-full rounded-md border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg outline-none focus:border-primary focus:ring-1 focus:ring-primary/30 transition-colors';

export function SSHCredentialForm({ deviceId, currentProfileId, onProfileChanged }: SSHCredentialFormProps) {
  const [profiles, setProfiles] = useState<CredentialProfile[]>([]);
  const [selectedProfileId, setSelectedProfileId] = useState(currentProfileId || '');
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ success: boolean; error?: string } | null>(null);
  const [message, setMessage] = useState('');
  const [showCreate, setShowCreate] = useState(false);

  // Create form state
  const [newName, setNewName] = useState('');
  const [newDescription, setNewDescription] = useState('');
  const [newUsername, setNewUsername] = useState('admin');
  const [newPort, setNewPort] = useState('22');
  const [newAuthMethod, setNewAuthMethod] = useState<'password' | 'key'>('password');
  const [newSecret, setNewSecret] = useState('');
  const [createLoading, setCreateLoading] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const profs = await fetchCredentialProfiles();
      setProfiles(profs);
    } catch {
      // non-fatal
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    setSelectedProfileId(currentProfileId || '');
  }, [currentProfileId]);

  const hasChanged = selectedProfileId !== (currentProfileId || '');
  const hasProfile = selectedProfileId !== '';

  const handleSave = async () => {
    if (!hasChanged) return;
    setSaving(true);
    setMessage('');
    try {
      // Use the dedicated credential profile assignment API (T-27-07 mitigation:
      // avoids exposing profile IDs via updateDevice request logs).
      if (selectedProfileId) {
        // Unassign previous profile first if one was set
        if (currentProfileId) {
          await unassignCredentialProfile(deviceId, currentProfileId);
        }
        await assignCredentialProfile(deviceId, selectedProfileId);
      } else if (currentProfileId) {
        // Unassign — no new profile selected
        await unassignCredentialProfile(deviceId, currentProfileId);
      }
      onProfileChanged?.(selectedProfileId || undefined);
      setMessage(hasProfile ? 'SSH profile assigned' : 'SSH profile unassigned');
    } catch (err) {
      setMessage(err instanceof Error ? err.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  };

  const handleTest = async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const result = await testSSHConnection(deviceId);
      setTestResult(result);
    } catch (err) {
      setTestResult({ success: false, error: err instanceof Error ? err.message : 'Test failed' });
    } finally {
      setTesting(false);
    }
  };

  function resetCreateForm() {
    setNewName('');
    setNewDescription('');
    setNewUsername('admin');
    setNewPort('22');
    setNewAuthMethod('password');
    setNewSecret('');
    setCreateError(null);
  }

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newName.trim()) {
      setCreateError('Profile name is required.');
      return;
    }
    setCreateLoading(true);
    setCreateError(null);
    try {
      const profile = await createCredentialProfile({
        name: newName.trim(),
        description: newDescription.trim(),
        username: newUsername.trim() || 'admin',
        port: parseInt(newPort, 10) || 22,
        auth_method: newAuthMethod,
        secret: newSecret,
        role: 'Admin',
      });
      await load();
      setSelectedProfileId(profile.id);
      setShowCreate(false);
      resetCreateForm();
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : 'Failed to create profile.');
    } finally {
      setCreateLoading(false);
    }
  };

  const selectedProfile = profiles.find((p) => p.id === selectedProfileId);

  if (showCreate) {
    return (
      <div className="space-y-4 transition-colors duration-200">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => { setShowCreate(false); resetCreateForm(); }}
            className="text-on-bg-secondary hover:text-on-bg"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
          </button>
          <p className="text-xs font-medium text-on-bg-secondary uppercase tracking-[0.12em]">New SSH Profile</p>
        </div>

        <form onSubmit={(e) => { void handleCreate(e); }} className="space-y-3">
          <div className="space-y-1">
            <label className="text-xs text-on-bg-secondary">Name <span className="text-status-down">*</span></label>
            <input type="text" value={newName} onChange={(e) => setNewName(e.target.value)} placeholder="e.g. MikroTik Admin" required className={inputClass} />
          </div>
          <div className="space-y-1">
            <label className="text-xs text-on-bg-secondary">Description</label>
            <input type="text" value={newDescription} onChange={(e) => setNewDescription(e.target.value)} placeholder="Optional" className={inputClass} />
          </div>
          <div className="space-y-1">
            <label className="text-xs text-on-bg-secondary">Username</label>
            <input type="text" value={newUsername} onChange={(e) => setNewUsername(e.target.value)} placeholder="admin" className={inputClass} />
          </div>
          <div className="space-y-1">
            <label className="text-xs text-on-bg-secondary">Port</label>
            <input type="number" value={newPort} onChange={(e) => setNewPort(e.target.value)} placeholder="22" className={`${inputClass} font-mono`} />
          </div>
          <div className="space-y-1">
            <label className="text-xs text-on-bg-secondary">Auth Method</label>
            <div className="flex gap-2">
              <button type="button" onClick={() => setNewAuthMethod('password')}
                className={`flex-1 rounded-md border px-3 py-2 text-xs font-medium transition-colors ${newAuthMethod === 'password' ? 'border-primary bg-primary/15 text-primary' : 'border-outline-subtle text-on-bg-secondary hover:text-on-bg'}`}>
                Password
              </button>
              <button type="button" onClick={() => setNewAuthMethod('key')}
                className={`flex-1 rounded-md border px-3 py-2 text-xs font-medium transition-colors ${newAuthMethod === 'key' ? 'border-primary bg-primary/15 text-primary' : 'border-outline-subtle text-on-bg-secondary hover:text-on-bg'}`}>
                Private Key
              </button>
            </div>
          </div>
          <div className="space-y-1">
            <label className="text-xs text-on-bg-secondary">{newAuthMethod === 'password' ? 'Password' : 'Private Key'}</label>
            {newAuthMethod === 'password' ? (
              <input type="password" value={newSecret} onChange={(e) => setNewSecret(e.target.value)} placeholder="Enter password" autoComplete="new-password" className={inputClass} />
            ) : (
              <textarea value={newSecret} onChange={(e) => setNewSecret(e.target.value)} placeholder="Paste private key" rows={4} className={`${inputClass} font-mono text-xs`} />
            )}
          </div>

          {createError && (
            <p className="rounded-md border border-status-down/30 bg-status-down/10 px-3 py-2 text-xs text-status-down">{createError}</p>
          )}

          <div className="flex gap-2">
            <button type="button" onClick={() => { setShowCreate(false); resetCreateForm(); }}
              className="flex-1 rounded-md bg-surface-high px-3 py-2 text-xs text-on-bg hover:bg-elevated transition-colors">
              Cancel
            </button>
            <button type="submit" disabled={createLoading}
              className="flex-1 rounded-md bg-primary px-3 py-2 text-xs font-medium text-white hover:bg-primary/90 disabled:opacity-50 transition-colors">
              {createLoading ? 'Creating...' : 'Create Profile'}
            </button>
          </div>
        </form>
      </div>
    );
  }

  return (
    <div className="space-y-4 transition-colors duration-200">
      <div>
        <label className="block text-xs text-on-bg-secondary mb-1">SSH Profile</label>
        <select
          value={selectedProfileId}
          onChange={(e) => setSelectedProfileId(e.target.value)}
          className={selectClass}
        >
          <option value="">-- No SSH Profile --</option>
          {profiles.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name} ({p.username}:{p.port})
            </option>
          ))}
        </select>
      </div>

      <button
        type="button"
        onClick={() => setShowCreate(true)}
        className="flex items-center gap-1.5 text-xs text-primary hover:text-primary/80 transition-colors"
      >
        <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
        </svg>
        Create new profile
      </button>

      {selectedProfile && (
        <div className="rounded-md bg-surface-high px-3 py-2 text-xs text-on-bg-secondary">
          <span className="font-medium text-on-bg">{selectedProfile.username}</span>
          :{selectedProfile.port} ({selectedProfile.auth_method})
          {selectedProfile.description && (
            <span className="block mt-0.5 text-on-bg-secondary/60">{selectedProfile.description}</span>
          )}
        </div>
      )}

      {/* Actions */}
      <div className="flex gap-2">
        <button
          onClick={() => { void handleSave(); }}
          disabled={saving || !hasChanged}
          className="flex-1 rounded-md bg-primary px-3 py-2 text-xs font-medium text-white hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {saving ? 'Saving...' : 'Save'}
        </button>
        {hasProfile && currentProfileId && (
          <button
            onClick={() => { void handleTest(); }}
            disabled={testing}
            className="rounded-md bg-surface-high px-3 py-2 text-xs font-medium text-on-bg-secondary hover:text-on-bg hover:bg-elevated disabled:opacity-50 transition-colors"
          >
            {testing ? 'Testing...' : 'Test'}
          </button>
        )}
      </div>

      {/* Test result */}
      {testResult && (
        <div
          className={`rounded-md p-3 text-xs ${
            testResult.success
              ? 'bg-status-up/10 text-status-up border border-status-up/20'
              : 'bg-status-down/10 text-status-down border border-status-down/20'
          }`}
        >
          {testResult.success ? 'Connection successful' : `Connection failed: ${testResult.error}`}
        </div>
      )}

      {/* Status message */}
      {message && (
        <div className="text-xs text-on-bg-secondary">{message}</div>
      )}
    </div>
  );
}
