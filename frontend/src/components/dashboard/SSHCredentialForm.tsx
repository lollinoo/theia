import { useState, useEffect, useCallback } from 'react';
import type { SSHProfile } from '../../types/api';
import {
  fetchSSHProfiles,
  createSSHProfile,
  updateDevice,
  testSSHConnection,
} from '../../api/client';

interface SSHCredentialFormProps {
  deviceId: string;
  currentProfileId?: string;
  onProfileChanged?: (profileId: string | undefined) => void;
}

const inputClass =
  'w-full rounded-md border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 outline-none focus:border-accent';
const selectClass =
  'w-full rounded-md border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary outline-none focus:border-accent';

export function SSHCredentialForm({ deviceId, currentProfileId, onProfileChanged }: SSHCredentialFormProps) {
  const [profiles, setProfiles] = useState<SSHProfile[]>([]);
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
      const profs = await fetchSSHProfiles();
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
      await updateDevice(deviceId, {
        ssh_profile_id: selectedProfileId || '',
      });
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
      const profile = await createSSHProfile({
        name: newName.trim(),
        description: newDescription.trim(),
        username: newUsername.trim() || 'admin',
        port: parseInt(newPort, 10) || 22,
        auth_method: newAuthMethod,
        secret: newSecret,
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
      <div className="space-y-4">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => { setShowCreate(false); resetCreateForm(); }}
            className="text-text-secondary hover:text-text-primary"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
          </button>
          <p className="text-xs font-medium uppercase tracking-widest text-text-secondary">New SSH Profile</p>
        </div>

        <form onSubmit={(e) => { void handleCreate(e); }} className="space-y-3">
          <div className="space-y-1">
            <label className="text-xs text-text-secondary">Name <span className="text-status-down">*</span></label>
            <input type="text" value={newName} onChange={(e) => setNewName(e.target.value)} placeholder="e.g. MikroTik Admin" required className={inputClass} />
          </div>
          <div className="space-y-1">
            <label className="text-xs text-text-secondary">Description</label>
            <input type="text" value={newDescription} onChange={(e) => setNewDescription(e.target.value)} placeholder="Optional" className={inputClass} />
          </div>
          <div className="space-y-1">
            <label className="text-xs text-text-secondary">Username</label>
            <input type="text" value={newUsername} onChange={(e) => setNewUsername(e.target.value)} placeholder="admin" className={inputClass} />
          </div>
          <div className="space-y-1">
            <label className="text-xs text-text-secondary">Port</label>
            <input type="number" value={newPort} onChange={(e) => setNewPort(e.target.value)} placeholder="22" className={inputClass} />
          </div>
          <div className="space-y-1">
            <label className="text-xs text-text-secondary">Auth Method</label>
            <div className="flex gap-2">
              <button type="button" onClick={() => setNewAuthMethod('password')}
                className={`flex-1 rounded-md border px-3 py-2 text-xs font-medium transition-colors ${newAuthMethod === 'password' ? 'border-accent bg-accent/15 text-accent' : 'border-border-subtle text-text-secondary hover:text-text-primary'}`}>
                Password
              </button>
              <button type="button" onClick={() => setNewAuthMethod('key')}
                className={`flex-1 rounded-md border px-3 py-2 text-xs font-medium transition-colors ${newAuthMethod === 'key' ? 'border-accent bg-accent/15 text-accent' : 'border-border-subtle text-text-secondary hover:text-text-primary'}`}>
                Private Key
              </button>
            </div>
          </div>
          <div className="space-y-1">
            <label className="text-xs text-text-secondary">{newAuthMethod === 'password' ? 'Password' : 'Private Key'}</label>
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
              className="flex-1 rounded-md border border-border-subtle bg-bg-elevated px-3 py-2 text-xs text-text-primary hover:bg-bg-surface transition-colors">
              Cancel
            </button>
            <button type="submit" disabled={createLoading}
              className="flex-1 rounded-md bg-accent px-3 py-2 text-xs font-medium text-white hover:bg-accent/90 disabled:opacity-50 transition-colors">
              {createLoading ? 'Creating...' : 'Create Profile'}
            </button>
          </div>
        </form>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div>
        <label className="block text-xs text-text-secondary mb-1">SSH Profile</label>
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
        className="flex items-center gap-1.5 text-xs text-accent hover:text-accent/80 transition-colors"
      >
        <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
        </svg>
        Create new profile
      </button>

      {selectedProfile && (
        <div className="rounded-md border border-border-subtle bg-bg-elevated/40 px-3 py-2 text-xs text-text-secondary">
          <span className="font-medium text-text-primary">{selectedProfile.username}</span>
          :{selectedProfile.port} ({selectedProfile.auth_method})
          {selectedProfile.description && (
            <span className="block mt-0.5 text-text-secondary/60">{selectedProfile.description}</span>
          )}
        </div>
      )}

      {/* Actions */}
      <div className="flex gap-2">
        <button
          onClick={() => { void handleSave(); }}
          disabled={saving || !hasChanged}
          className="flex-1 rounded-md bg-accent px-3 py-2 text-xs font-medium text-white hover:bg-accent/90 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {saving ? 'Saving...' : 'Save'}
        </button>
        {hasProfile && currentProfileId && (
          <button
            onClick={() => { void handleTest(); }}
            disabled={testing}
            className="rounded-md border border-border-subtle px-3 py-2 text-xs font-medium text-text-secondary hover:text-text-primary hover:bg-bg-elevated disabled:opacity-50 transition-colors"
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
        <div className="text-xs text-text-secondary">{message}</div>
      )}
    </div>
  );
}
