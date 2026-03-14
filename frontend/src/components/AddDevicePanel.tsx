import { useEffect, useState } from 'react';
import { checkPrometheusHealth, createDevice, fetchSNMPProfiles } from '../api/client';
import type { SNMPProfile } from '../types/api';

interface AddDevicePanelProps {
  onDeviceAdded: () => void;
}

type MetricsMode = 'snmp' | 'prometheus' | 'prometheus_snmp_fallback';

export function AddDevicePanel({ onDeviceAdded }: AddDevicePanelProps) {
  const [hostname, setHostname] = useState('');
  const [version, setVersion] = useState('2c');
  const [displayName, setDisplayName] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Prometheus availability
  const [prometheusAvailable, setPrometheusAvailable] = useState<boolean | null>(null);
  const [prometheusCheckDone, setPrometheusCheckDone] = useState(false);

  // Metrics mode (unified dropdown)
  const [metricsMode, setMetricsMode] = useState<MetricsMode>('snmp');
  const [prometheusLabelName, setPrometheusLabelName] = useState('instance');
  const [prometheusLabelValue, setPrometheusLabelValue] = useState('');

  // v2c
  const [community, setCommunity] = useState('public');

  // v3
  const [username, setUsername] = useState('');
  const [securityLevel, setSecurityLevel] = useState('authPriv');
  const [authProtocol, setAuthProtocol] = useState('SHA');
  const [authPassword, setAuthPassword] = useState('');
  const [privProtocol, setPrivProtocol] = useState('AES');
  const [privPassword, setPrivPassword] = useState('');

  // profiles
  const [profiles, setProfiles] = useState<SNMPProfile[]>([]);

  useEffect(() => {
    fetchSNMPProfiles().then(setProfiles).catch(() => {/* non-fatal */});
    checkPrometheusHealth().then((result) => {
      setPrometheusAvailable(result.available);
      setPrometheusCheckDone(true);
      // If prometheus is unavailable, force SNMP mode
      if (!result.available) {
        setMetricsMode('snmp');
      }
    }).catch(() => {
      setPrometheusAvailable(false);
      setPrometheusCheckDone(true);
      setMetricsMode('snmp');
    });
  }, []);

  function applyProfile(profileId: string) {
    const profile = profiles.find((p) => p.id === profileId);
    if (!profile) return;
    setVersion(profile.snmp.version);
    setCommunity(profile.snmp.community ?? 'public');
    setUsername(profile.snmp.username ?? '');
    setSecurityLevel(profile.snmp.security_level ?? 'authPriv');
    setAuthProtocol(profile.snmp.auth_protocol ?? 'SHA');
    setAuthPassword(profile.snmp.auth_password ?? '');
    setPrivProtocol(profile.snmp.priv_protocol ?? 'AES');
    setPrivPassword(profile.snmp.priv_password ?? '');
  }

  function handleMetricsModeChange(value: MetricsMode) {
    if ((value === 'prometheus' || value === 'prometheus_snmp_fallback') && !prometheusAvailable) {
      return; // guard against selecting unavailable option
    }
    setMetricsMode(value);
  }

  const isV3 = version === '3';
  const needsAuth = securityLevel === 'authNoPriv' || securityLevel === 'authPriv';
  const needsPriv = securityLevel === 'authPriv';
  const usesPrometheus = metricsMode === 'prometheus' || metricsMode === 'prometheus_snmp_fallback';

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!hostname.trim()) {
      setError('Hostname or IP is required.');
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const effectiveLabelValue = prometheusLabelValue.trim() || hostname.trim();
      await createDevice({
        hostname: hostname.trim(),
        ip: hostname.trim(),
        snmp: isV3
          ? {
              version: '3',
              username: username.trim(),
              security_level: securityLevel,
              ...(needsAuth ? { auth_protocol: authProtocol, auth_password: authPassword } : {}),
              ...(needsPriv ? { priv_protocol: privProtocol, priv_password: privPassword } : {}),
            }
          : {
              version: version,
              community: community.trim() || 'public',
            },
        tags: displayName.trim() ? { display_name: displayName.trim() } : undefined,
        metrics_source: metricsMode,
        prometheus_label_name: usesPrometheus ? prometheusLabelName : undefined,
        prometheus_label_value: usesPrometheus ? effectiveLabelValue : undefined,
      });
      onDeviceAdded();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add device.');
    } finally {
      setLoading(false);
    }
  }

  const inputClass =
    'w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder-text-secondary/40 focus:border-accent focus:outline-none';
  const selectClass =
    'w-full rounded-lg border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary focus:border-accent focus:outline-none';
  const labelClass = 'text-xs font-medium uppercase tracking-widest text-text-secondary';

  return (
    <form onSubmit={(e) => { void handleSubmit(e); }} className="space-y-4 p-4">
      {/* Prometheus unavailable warning */}
      {prometheusCheckDone && !prometheusAvailable && (
        <div className="rounded-lg border border-yellow-500/30 bg-yellow-500/10 px-3 py-2 text-xs text-yellow-400">
          Prometheus is not configured or unreachable. Only SNMP Direct is available.
        </div>
      )}

      <div className="space-y-2">
        <label className={labelClass}>
          IP Address <span className="text-status-down">*</span>
        </label>
        <input
          type="text"
          value={hostname}
          onChange={(e) => setHostname(e.target.value)}
          placeholder="192.168.1.1"
          required
          className={inputClass}
        />
      </div>

      {/* Metrics & Collection Mode */}
      <div className="space-y-2">
        <label className={labelClass}>Metrics Source</label>
        <select
          value={metricsMode}
          onChange={(e) => handleMetricsModeChange(e.target.value as MetricsMode)}
          className={selectClass}
        >
          <option value="snmp">SNMP Direct</option>
          <option value="prometheus" disabled={!prometheusAvailable}>
            Prometheus{!prometheusAvailable ? ' (unavailable)' : ''}
          </option>
          <option value="prometheus_snmp_fallback" disabled={!prometheusAvailable}>
            Prometheus + SNMP Fallback{!prometheusAvailable ? ' (unavailable)' : ''}
          </option>
        </select>
        {metricsMode === 'prometheus' && (
          <p className="text-xs text-text-secondary/70">
            Metrics from Prometheus only. No fallback if Prometheus is unreachable.
          </p>
        )}
        {metricsMode === 'prometheus_snmp_fallback' && (
          <p className="text-xs text-text-secondary/70">
            Metrics from Prometheus. Falls back to SNMP if Prometheus is unavailable or has no data.
          </p>
        )}
      </div>

      {/* Prometheus label config */}
      {usesPrometheus && (
        <div className="space-y-2 rounded-lg border border-border-subtle p-3">
          <p className={labelClass}>Prometheus Target</p>
          <div className="space-y-1">
            <label className="text-xs text-text-secondary">Label</label>
            <select
              value={prometheusLabelName}
              onChange={(e) => setPrometheusLabelName(e.target.value)}
              className={selectClass}
            >
              <option value="instance">instance (IP address)</option>
              <option value="identity">identity</option>
              <option value="vendor">vendor</option>
            </select>
          </div>
          <div className="space-y-1">
            <label className="text-xs text-text-secondary">
              Value{prometheusLabelName === 'instance' ? ' (defaults to IP if blank)' : ''}
            </label>
            <input
              type="text"
              value={prometheusLabelValue}
              onChange={(e) => setPrometheusLabelValue(e.target.value)}
              placeholder={prometheusLabelName === 'instance' ? hostname || '192.168.1.1' : `e.g. my-router`}
              className={inputClass}
            />
          </div>
        </div>
      )}

      {/* SNMP Credentials */}
      <div className="space-y-3 rounded-lg border border-border-subtle p-3">
        <p className={labelClass}>SNMP Credentials</p>

        {profiles.length > 0 && (
          <div className="space-y-1">
            <label className="text-xs text-text-secondary">Load from Profile</label>
            <select
              defaultValue=""
              onChange={(e) => { applyProfile(e.target.value); e.target.value = ''; }}
              className={selectClass}
            >
              <option value="" disabled>Select a credential profile...</option>
              {profiles.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name} (SNMP {p.snmp.version})
                </option>
              ))}
            </select>
          </div>
        )}

        <div className="space-y-1">
          <label className="text-xs text-text-secondary">Version</label>
          <select
            value={version}
            onChange={(e) => setVersion(e.target.value)}
            className={selectClass}
          >
            <option value="2c">v2c</option>
            <option value="3">v3</option>
          </select>
        </div>

        {!isV3 && (
          <div className="space-y-1">
            <label className="text-xs text-text-secondary">Community</label>
            <input
              type="text"
              value={community}
              onChange={(e) => setCommunity(e.target.value)}
              placeholder="public"
              className={inputClass}
            />
          </div>
        )}

        {isV3 && (
          <div className="space-y-2">
            <div className="space-y-1">
              <label className="text-xs text-text-secondary">Username</label>
              <input
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="snmpv3user"
                className={inputClass}
              />
            </div>

            <div className="space-y-1">
              <label className="text-xs text-text-secondary">Security Level</label>
              <select
                value={securityLevel}
                onChange={(e) => setSecurityLevel(e.target.value)}
                className={selectClass}
              >
                <option value="noAuthNoPriv">No Auth, No Privacy</option>
                <option value="authNoPriv">Auth, No Privacy</option>
                <option value="authPriv">Auth + Privacy</option>
              </select>
            </div>

            {needsAuth && (
              <>
                <div className="space-y-1">
                  <label className="text-xs text-text-secondary">Auth Protocol</label>
                  <select
                    value={authProtocol}
                    onChange={(e) => setAuthProtocol(e.target.value)}
                    className={selectClass}
                  >
                    <option value="SHA">SHA</option>
                    <option value="MD5">MD5</option>
                  </select>
                </div>
                <div className="space-y-1">
                  <label className="text-xs text-text-secondary">Auth Key</label>
                  <input
                    type="password"
                    value={authPassword}
                    onChange={(e) => setAuthPassword(e.target.value)}
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
                  <label className="text-xs text-text-secondary">Encryption Protocol</label>
                  <select
                    value={privProtocol}
                    onChange={(e) => setPrivProtocol(e.target.value)}
                    className={selectClass}
                  >
                    <option value="AES">AES</option>
                    <option value="DES">DES</option>
                  </select>
                </div>
                <div className="space-y-1">
                  <label className="text-xs text-text-secondary">Encryption Key</label>
                  <input
                    type="password"
                    value={privPassword}
                    onChange={(e) => setPrivPassword(e.target.value)}
                    placeholder="Privacy passphrase"
                    autoComplete="new-password"
                    className={inputClass}
                  />
                </div>
              </>
            )}
          </div>
        )}
      </div>

      <div className="space-y-2">
        <label className={labelClass}>
          Custom Name <span className="text-text-secondary/50">(optional)</span>
        </label>
        <input
          type="text"
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
          placeholder="Auto-discovered from SNMP / Prometheus"
          className={inputClass}
        />
      </div>

      {error && (
        <p className="rounded-lg border border-status-down/30 bg-status-down/10 px-3 py-2 text-xs text-status-down">
          {error}
        </p>
      )}

      <button
        type="submit"
        disabled={loading}
        className="w-full rounded-lg bg-accent px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {loading ? 'Adding...' : 'Add Device'}
      </button>
    </form>
  );
}
