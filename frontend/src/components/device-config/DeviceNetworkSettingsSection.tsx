/**
 * Renders device network settings section controls within the device configuration workflow.
 * Keeps this section focused on one editable device responsibility.
 */
import type { ReactNode } from 'react';
import { useEffect, useState } from 'react';
import { checkPrometheusHealth, fetchSNMPProfiles } from '../../api/client';
import type { Device, MetricsSource, SNMPProfile } from '../../types/api';
import { createAsyncStaleGuard } from '../../utils/asyncStaleGuard';
import { MAX_STRING_LENGTH, validateMaxLength } from '../../utils/validation';
import type { DeviceFormModel } from '../forms/deviceFormModels';

interface DeviceNetworkSettingsSectionProps {
  device: Device;
  form: DeviceFormModel;
  readOnly?: boolean;
  isVirtual?: boolean;
  fieldErrors: Record<string, string>;
  children?: ReactNode;
  onFormChange: (update: Partial<DeviceFormModel>) => void;
  onPrometheusChange: (update: Partial<DeviceFormModel['prometheus']>) => void;
  onSnmpChange: (update: Partial<DeviceFormModel['snmp']>) => void;
  onFieldError: (field: string, err: string | null) => void;
  onSNMPProfileSelected: (profileId: string) => void;
}

/** Renders the DeviceNetworkSettingsSection component within the device configuration workflow. */
export function DeviceNetworkSettingsSection({
  device,
  form,
  readOnly = false,
  isVirtual,
  fieldErrors,
  children,
  onFormChange,
  onPrometheusChange,
  onSnmpChange,
  onFieldError,
  onSNMPProfileSelected,
}: DeviceNetworkSettingsSectionProps) {
  const [profiles, setProfiles] = useState<SNMPProfile[]>([]);
  const [prometheusAvailable, setPrometheusAvailable] = useState<boolean | null>(null);
  const usesPrometheus =
    form.metricsMode === 'prometheus' || form.metricsMode === 'prometheus_snmp_fallback';
  const usesSNMP = form.metricsMode === 'snmp' || form.metricsMode === 'prometheus_snmp_fallback';

  useEffect(() => {
    if (isVirtual) {
      setProfiles([]);
      setPrometheusAvailable(null);
      return;
    }

    const staleGuard = createAsyncStaleGuard();
    fetchSNMPProfiles()
      .then((nextProfiles) => {
        staleGuard.run(() => setProfiles(nextProfiles));
      })
      .catch(() => {
        /* non-fatal */
      });
    checkPrometheusHealth()
      .then((result) => {
        staleGuard.run(() => setPrometheusAvailable(result.enabled !== false && result.available));
      })
      .catch(() => {
        staleGuard.run(() => setPrometheusAvailable(false));
      });

    return () => {
      staleGuard.cancel();
    };
  }, [isVirtual]);

  if (isVirtual) {
    return <>{children}</>;
  }

  function handlePrometheusLabelValueBlur() {
    onFieldError(
      'prometheusLabelValue',
      validateMaxLength(form.prometheus.labelValue, MAX_STRING_LENGTH, 'Label value'),
    );
  }

  function handleCommunityBlur() {
    onFieldError(
      'community',
      validateMaxLength(form.snmp.community, MAX_STRING_LENGTH, 'Community string'),
    );
  }

  function handleUsernameBlur() {
    onFieldError('username', validateMaxLength(form.snmp.username, MAX_STRING_LENGTH, 'Username'));
  }

  return (
    <>
      <div className="space-y-1">
        <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
          Vendor
        </label>
        <select
          value={form.vendor}
          disabled={readOnly}
          onChange={(e) => onFormChange({ vendor: e.target.value })}
          className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
        >
          <option value="">— Select vendor —</option>
          <option value="mikrotik">MikroTik</option>
        </select>
        <p className="text-xs text-on-bg-secondary">
          Vendor tag determines backup commands and metric queries.
        </p>
      </div>

      {children}

      {prometheusAvailable === false && (
        <p className="rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning">
          Prometheus is not configured or unreachable. Only SNMP Direct is available.
        </p>
      )}

      <div className="space-y-1">
        <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
          Metrics Source
        </label>
        <select
          value={form.metricsMode}
          disabled={readOnly}
          onChange={(e) => {
            const val = e.target.value as 'prometheus' | 'snmp' | 'prometheus_snmp_fallback';
            if (
              (val === 'prometheus' || val === 'prometheus_snmp_fallback') &&
              !prometheusAvailable
            ) {
              return;
            }
            onFormChange({ metricsMode: val as MetricsSource });
          }}
          className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
        >
          <option value="snmp">SNMP Direct</option>
          <option value="prometheus" disabled={!prometheusAvailable}>
            Prometheus{!prometheusAvailable ? ' (unavailable)' : ''}
          </option>
          <option value="prometheus_snmp_fallback" disabled={!prometheusAvailable}>
            Prometheus + SNMP Fallback{!prometheusAvailable ? ' (unavailable)' : ''}
          </option>
        </select>
        {form.metricsMode === 'prometheus' && (
          <p className="text-xs text-on-bg-secondary">
            Metrics from Prometheus only. No fallback if Prometheus is unreachable.
          </p>
        )}
        {form.metricsMode === 'prometheus_snmp_fallback' && (
          <p className="text-xs text-on-bg-secondary">
            Falls back to SNMP if Prometheus is unavailable or has no data for this device.
          </p>
        )}
      </div>

      {usesPrometheus && (
        <div className="space-y-2 bg-surface-high rounded-lg p-3">
          <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            Prometheus Target
          </p>
          <div className="space-y-1">
            <label className="text-xs text-on-bg-secondary">Label</label>
            <select
              value={form.prometheus.labelName}
              disabled={readOnly}
              onChange={(e) => onPrometheusChange({ labelName: e.target.value })}
              className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
            >
              <option value="instance">instance (IP address)</option>
              <option value="identity">identity</option>
              <option value="vendor">vendor</option>
            </select>
          </div>
          <div className="space-y-1">
            <label className="text-xs text-on-bg-secondary">
              Value
              {form.prometheus.labelName === 'instance' ? ' (defaults to IP if blank)' : ''}
            </label>
            <input
              type="text"
              value={form.prometheus.labelValue}
              disabled={readOnly}
              onChange={(e) => {
                onPrometheusChange({ labelValue: e.target.value });
                onFieldError('prometheusLabelValue', null);
              }}
              onBlur={handlePrometheusLabelValueBlur}
              placeholder={
                form.prometheus.labelName === 'instance' ? form.ip || device.ip : 'e.g. my-router'
              }
              className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60${fieldErrors['prometheusLabelValue'] ? ' border-status-down' : ' border-outline-subtle'}`}
            />
            {fieldErrors['prometheusLabelValue'] && (
              <p className="mt-1 text-xs text-status-down">{fieldErrors['prometheusLabelValue']}</p>
            )}
          </div>
        </div>
      )}

      {usesSNMP && (
        <>
          {profiles.length > 0 && (
            <select
              defaultValue=""
              disabled={readOnly}
              onChange={(e) => {
                if (e.target.value) {
                  onSNMPProfileSelected(e.target.value);
                  e.target.value = '';
                }
              }}
              className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
            >
              <option value="" disabled>
                Load credentials from profile...
              </option>
              {profiles.map((profile) => (
                <option key={profile.id} value={profile.id}>
                  {profile.name} (SNMP {profile.snmp.version})
                </option>
              ))}
            </select>
          )}

          <select
            value={form.snmp.version}
            disabled={readOnly}
            onChange={(e) =>
              onSnmpChange({ version: e.target.value as DeviceFormModel['snmp']['version'] })
            }
            className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
          >
            <option value="2c">SNMP v2c</option>
            <option value="3">SNMP v3</option>
          </select>

          {form.snmp.version !== '3' && (
            <>
              <input
                type="text"
                value={form.snmp.community}
                disabled={readOnly}
                onChange={(e) => {
                  onSnmpChange({ community: e.target.value });
                  onFieldError('community', null);
                }}
                onBlur={handleCommunityBlur}
                placeholder="SNMP Community (leave blank to keep current)"
                className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60${fieldErrors['community'] ? ' border-status-down' : ' border-outline-subtle'}`}
              />
              {fieldErrors['community'] && (
                <p className="mt-1 text-xs text-status-down">{fieldErrors['community']}</p>
              )}
            </>
          )}

          {form.snmp.version === '3' && (
            <div className="space-y-2 bg-surface-high rounded-lg p-3">
              <p className="text-xs text-on-bg-secondary">
                SNMPv3 Credentials (leave blank to keep current)
              </p>
              <input
                type="text"
                value={form.snmp.username}
                disabled={readOnly}
                onChange={(e) => {
                  onSnmpChange({ username: e.target.value });
                  onFieldError('username', null);
                }}
                onBlur={handleUsernameBlur}
                placeholder="Username"
                className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60${fieldErrors['username'] ? ' border-status-down' : ' border-outline-subtle'}`}
              />
              {fieldErrors['username'] && (
                <p className="mt-1 text-xs text-status-down">{fieldErrors['username']}</p>
              )}
              <select
                value={form.snmp.securityLevel}
                disabled={readOnly}
                onChange={(e) => onSnmpChange({ securityLevel: e.target.value })}
                className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
              >
                <option value="noAuthNoPriv">No Auth, No Privacy</option>
                <option value="authNoPriv">Auth, No Privacy</option>
                <option value="authPriv">Auth + Privacy</option>
              </select>
              {(form.snmp.securityLevel === 'authNoPriv' ||
                form.snmp.securityLevel === 'authPriv') && (
                <>
                  <select
                    value={form.snmp.authProtocol}
                    disabled={readOnly}
                    onChange={(e) => onSnmpChange({ authProtocol: e.target.value })}
                    className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    <option value="SHA">SHA</option>
                    <option value="MD5">MD5</option>
                    <option value="SHA-224">SHA-224</option>
                    <option value="SHA-256">SHA-256</option>
                    <option value="SHA-384">SHA-384</option>
                    <option value="SHA-512">SHA-512</option>
                  </select>
                  <input
                    type="password"
                    value={form.snmp.authPassword}
                    disabled={readOnly}
                    onChange={(e) => onSnmpChange({ authPassword: e.target.value })}
                    placeholder="Auth Key"
                    autoComplete="new-password"
                    className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
                  />
                </>
              )}
              {form.snmp.securityLevel === 'authPriv' && (
                <>
                  <select
                    value={form.snmp.privProtocol}
                    disabled={readOnly}
                    onChange={(e) => onSnmpChange({ privProtocol: e.target.value })}
                    className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    <option value="AES">AES</option>
                    <option value="DES">DES</option>
                  </select>
                  <input
                    type="password"
                    value={form.snmp.privPassword}
                    disabled={readOnly}
                    onChange={(e) => onSnmpChange({ privPassword: e.target.value })}
                    placeholder="Encryption Key"
                    autoComplete="new-password"
                    className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
                  />
                </>
              )}
            </div>
          )}
        </>
      )}
    </>
  );
}
