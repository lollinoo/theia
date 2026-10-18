/**
 * Renders a one-time onboarding modal when Theia auto-generates encryption keys at bootstrap.
 * Guides the administrator to copy or download the key before acknowledging and hiding the secret.
 */
import { useEffect, useState } from 'react';
import { acknowledgeEncryptionKey, fetchEncryptionKeyStatus } from '../api/client';
import type { EncryptionKeyStatus } from '../types/api';

interface EncryptionKeyOnboardingModalProps {
  enabled: boolean;
  onStatusChange?: (status: EncryptionKeyStatus) => void;
}

/** Displays a one-time modal prompting administrators to save auto-generated encryption keys. */
export function EncryptionKeyOnboardingModal({
  enabled,
  onStatusChange,
}: EncryptionKeyOnboardingModalProps) {
  const [status, setStatus] = useState<EncryptionKeyStatus | null>(null);
  const [confirmedSaved, setConfirmedSaved] = useState(false);
  const [copied, setCopied] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!enabled || typeof fetchEncryptionKeyStatus !== 'function') return;
    let cancelled = false;
    void fetchEncryptionKeyStatus()
      .then((res) => {
        if (cancelled) return;
        setStatus(res);
        onStatusChange?.(res);
      })
      .catch(() => {
        // Non-fatal if endpoint is unavailable
      });
    return () => {
      cancelled = true;
    };
  }, [enabled, onStatusChange]);

  if (!enabled || !status || status.acknowledged || !status.pending_secret) {
    return null;
  }

  const keyAssignment = `${status.active_key_id}=${status.pending_secret}`;
  const envSnippet =
    status.env_snippet ||
    `THEIA_ENCRYPTION_KEY_ID=${status.active_key_id}\nTHEIA_ENCRYPTION_KEYS=${keyAssignment}`;

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(envSnippet);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Fallback ignored
    }
  };

  const handleDownload = () => {
    const content = [
      '# Theia Instance Encryption Key Recovery File',
      '# Store this file in a secure password manager or vault.',
      '# You will need this key to restore backups onto a new Theia instance.',
      '',
      envSnippet,
      '',
    ].join('\n');
    const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `theia-encryption-key-${status.active_key_id}.txt`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  };

  const handleAcknowledge = async () => {
    if (!confirmedSaved || submitting) return;
    setSubmitting(true);
    setError('');
    try {
      const updated = await acknowledgeEncryptionKey();
      setStatus(updated);
      onStatusChange?.(updated);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not confirm encryption key');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60"
      role="dialog"
      aria-modal="true"
      aria-labelledby="encryption-key-onboarding-title"
    >
      <div className="mx-4 w-full max-w-lg rounded-xl bg-surface-high p-5 shadow-xl space-y-4">
        <div className="space-y-1">
          <h3 id="encryption-key-onboarding-title" className="text-sm font-semibold text-on-bg">
            Save Your Instance Encryption Key
          </h3>
          <p className="text-xs text-on-bg-secondary">
            Theia automatically generated an encryption key for device credentials and secrets. Save
            this key in a password manager or secure vault now: you will need it if you ever restore
            a database backup onto a new Theia instance.
          </p>
        </div>

        <div className="rounded-lg border border-outline-subtle bg-bg/60 p-3 space-y-2">
          <div className="flex items-center justify-between text-[11px] text-on-bg-secondary">
            <span>
              Key ID: <strong className="font-mono text-on-bg">{status.active_key_id}</strong>
            </span>
            <span className="text-warning font-medium">Shown only once</span>
          </div>
          <pre className="overflow-x-auto rounded bg-elevated p-2.5 text-[11px] font-mono text-on-bg select-all">
            {envSnippet}
          </pre>
          <div className="flex items-center gap-2 pt-1">
            <button
              type="button"
              onClick={() => {
                void handleCopy();
              }}
              className="rounded-lg border border-primary/30 bg-transparent px-2.5 py-1 text-xs font-medium text-primary hover:bg-primary/10 transition-colors"
            >
              {copied ? 'Copied!' : 'Copy Key'}
            </button>
            <button
              type="button"
              onClick={handleDownload}
              className="rounded-lg border border-outline-subtle bg-elevated px-2.5 py-1 text-xs font-medium text-on-bg hover:bg-elevated/80 transition-colors"
            >
              Download Recovery File (.txt)
            </button>
          </div>
        </div>

        <label className="flex items-start gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={confirmedSaved}
            onChange={(e) => setConfirmedSaved(e.target.checked)}
            className="mt-0.5 rounded border-border"
          />
          <span className="text-xs text-on-bg-secondary">
            I have saved this encryption key in a secure place and understand it will be permanently
            hidden after confirmation.
          </span>
        </label>

        {error && (
          <div className="rounded-md border border-status-down/20 bg-status-down/5 p-2 text-xs text-status-down">
            {error}
          </div>
        )}

        <div className="flex justify-end">
          <button
            type="button"
            onClick={() => {
              void handleAcknowledge();
            }}
            disabled={!confirmedSaved || submitting}
            className="rounded-lg bg-primary px-3.5 py-1.5 text-xs font-medium text-white hover:bg-primary/90 disabled:opacity-50 transition-colors"
          >
            {submitting ? 'Saving...' : 'I Saved the Key'}
          </button>
        </div>
      </div>
    </div>
  );
}
