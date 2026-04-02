import { useEffect, useState } from 'react';
import { checkPrometheusHealth, createDevice, fetchAreas, fetchSNMPProfiles, fetchSSHProfiles } from '../api/client';
import type { Area, SNMPProfile, SSHProfile } from '../types/api';
import { MaterialIcon } from './MaterialIcon';

interface AddDevicePanelProps {
  onDeviceAdded: () => void;
}

type MetricsMode = 'snmp' | 'prometheus' | 'prometheus_snmp_fallback';
type DeviceMode = 'physical' | 'virtual';

export function AddDevicePanel({ onDeviceAdded }: AddDevicePanelProps) {
  const [hostname, setHostname] = useState('');
  const [version, setVersion] = useState('2c');
  const [displayName, setDisplayName] = useState('');
  const [vendorOverride, setVendorOverride] = useState('');
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
  const [sshProfiles, setSSHProfiles] = useState<SSHProfile[]>([]);
  const [sshProfileId, setSSHProfileId] = useState('');

  // areas
  const [areas, setAreas] = useState<Area[]>([]);
  const [areaIds, setAreaIds] = useState<string[]>([]);

  // Device mode (physical vs virtual)
  const [deviceMode, setDeviceMode] = useState<DeviceMode>('physical');
  const isVirtual = deviceMode === 'virtual';

  // Virtual-specific state
  const [virtualSubtype, setVirtualSubtype] = useState('internet');
  const [virtualIp, setVirtualIp] = useState('');

  function handleModeSwitch(mode: DeviceMode) {
    setDeviceMode(mode);
    setError(null);
    // Reset physical fields
    setHostname('');
    setDisplayName('');
    setVendorOverride('');
    setVersion('2c');
    setCommunity('public');
    setUsername('');
    setSecurityLevel('authPriv');
    setAuthProtocol('SHA');
    setAuthPassword('');
    setPrivProtocol('AES');
    setPrivPassword('');
    setMetricsMode('snmp');
    setPrometheusLabelName('instance');
    setPrometheusLabelValue('');
    setSSHProfileId('');
    setAreaIds([]);
    // Reset virtual fields
    setVirtualSubtype('internet');
    setVirtualIp('');
  }

  useEffect(() => {
    fetchSNMPProfiles().then(setProfiles).catch(() => {/* non-fatal */});
    fetchSSHProfiles().then(setSSHProfiles).catch(() => {/* non-fatal */});
    fetchAreas().then(setAreas).catch(() => {/* non-fatal */});
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
  const usesSNMP = metricsMode === 'snmp' || metricsMode === 'prometheus_snmp_fallback';

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (isVirtual) {
      if (!displayName.trim()) {
        setError('Display Name is required.');
        return;
      }
      setLoading(true);
      setError(null);
      try {
        await createDevice({
          hostname: displayName.trim(),
          ip: virtualIp.trim() || undefined,
          device_type: 'virtual',
          tags: {
            display_name: displayName.trim(),
            virtual_subtype: virtualSubtype,
          },
          area_ids: areaIds.length > 0 ? areaIds : undefined,
        });
        onDeviceAdded();
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to add virtual node.');
      } finally {
        setLoading(false);
      }
      return;
    }
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
        vendor: vendorOverride || undefined,
        metrics_source: metricsMode,
        prometheus_label_name: usesPrometheus ? prometheusLabelName : undefined,
        prometheus_label_value: usesPrometheus ? effectiveLabelValue : undefined,
        ssh_profile_id: sshProfileId || undefined,
        area_ids: areaIds.length > 0 ? areaIds : undefined,
      });
      onDeviceAdded();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add device.');
    } finally {
      setLoading(false);
    }
  }

  const inputClass =
    'w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none';
  const selectClass =
    'w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none';
  const labelClass = 'text-xs font-medium uppercase tracking-widest text-on-bg-secondary';

  return (
    <form onSubmit={(e) => { void handleSubmit(e); }} className="space-y-4 p-4 transition-colors duration-200">
      {/* Device mode toggle */}
      <div className="flex rounded-lg bg-surface p-0.5">
        <button
          type="button"
          className={`flex-1 rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
            !isVirtual ? 'bg-primary text-white' : 'text-on-bg-secondary hover:text-on-bg'
          }`}
          onClick={() => handleModeSwitch('physical')}
        >
          Physical Device
        </button>
        <button
          type="button"
          className={`flex-1 rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
            isVirtual ? 'bg-primary text-white' : 'text-on-bg-secondary hover:text-on-bg'
          }`}
          onClick={() => handleModeSwitch('virtual')}
        >
          Virtual Node
        </button>
      </div>

      {isVirtual ? (
        <div className="space-y-4">
          {/* Display Name (required) */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
              Display Name <span className="text-status-down">*</span>
            </label>
            <input
              type="text"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="e.g. ISP Gateway"
              className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder:text-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
              required
            />
          </div>

          {/* Subtype 2x2 grid */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
              Type
            </label>
            <div className="grid grid-cols-2 gap-2">
              {([
                { value: 'internet', label: 'Internet', icon: 'language' },
                { value: 'cloud', label: 'Cloud', icon: 'cloud' },
                { value: 'server', label: 'Server', icon: 'dns' },
                { value: 'generic', label: 'Generic', icon: 'hub' },
              ] as const).map((st) => (
                <button
                  key={st.value}
                  type="button"
                  onClick={() => setVirtualSubtype(st.value)}
                  className={`flex flex-col items-center gap-1.5 rounded-lg border-2 px-3 py-3 transition-colors ${
                    virtualSubtype === st.value
                      ? 'border-primary bg-primary/10'
                      : 'border-outline-subtle bg-elevated hover:border-outline'
                  }`}
                >
                  <MaterialIcon name={st.icon} size={24} className={
                    virtualSubtype === st.value ? 'text-primary' : 'text-on-bg-secondary'
                  } />
                  <span className={`text-xs font-medium ${
                    virtualSubtype === st.value ? 'text-primary' : 'text-on-bg-secondary'
                  }`}>
                    {st.label}
                  </span>
                </button>
              ))}
            </div>
          </div>

          {/* IP Address (optional) */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
              IP Address <span className="normal-case tracking-normal text-on-bg-muted">(optional)</span>
            </label>
            <input
              type="text"
              value={virtualIp}
              onChange={(e) => setVirtualIp(e.target.value)}
              placeholder="e.g. 203.0.113.1"
              className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder:text-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
            />
          </div>
        </div>
      ) : (
        <>
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
              <p className="text-xs text-on-bg-secondary/70">
                Metrics from Prometheus only. No fallback if Prometheus is unreachable.
              </p>
            )}
            {metricsMode === 'prometheus_snmp_fallback' && (
              <p className="text-xs text-on-bg-secondary/70">
                Metrics from Prometheus. Falls back to SNMP if Prometheus is unavailable or has no data.
              </p>
            )}
          </div>

          {/* Prometheus label config */}
          {usesPrometheus && (
            <div className="space-y-2 bg-surface-high rounded-lg p-3">
              <p className={labelClass}>Prometheus Target</p>
              <div className="space-y-1">
                <label className="text-xs text-on-bg-secondary">Label</label>
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
                <label className="text-xs text-on-bg-secondary">
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

          {/* SNMP Credentials — visible when metrics source uses SNMP */}
          {usesSNMP && (
            <div className="space-y-3 bg-surface-high rounded-lg p-3">
              <p className={labelClass}>SNMP Credentials</p>

              {profiles.length > 0 && (
                <div className="space-y-1">
                  <label className="text-xs text-on-bg-secondary">Load from Profile</label>
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
                <label className="text-xs text-on-bg-secondary">Version</label>
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
                  <label className="text-xs text-on-bg-secondary">Community</label>
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
                    <label className="text-xs text-on-bg-secondary">Username</label>
                    <input
                      type="text"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                      placeholder="snmpv3user"
                      className={inputClass}
                    />
                  </div>

                  <div className="space-y-1">
                    <label className="text-xs text-on-bg-secondary">Security Level</label>
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
                        <label className="text-xs text-on-bg-secondary">Auth Protocol</label>
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
                        <label className="text-xs text-on-bg-secondary">Auth Key</label>
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
                        <label className="text-xs text-on-bg-secondary">Encryption Protocol</label>
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
                        <label className="text-xs text-on-bg-secondary">Encryption Key</label>
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
          )}

          <div className="space-y-2">
            <label className={labelClass}>
              Custom Name <span className="text-on-bg-secondary/50">(optional)</span>
            </label>
            <input
              type="text"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="Auto-discovered from SNMP / Prometheus"
              className={inputClass}
            />
          </div>

          <div className="space-y-2">
            <label className={labelClass}>
              Vendor <span className="text-on-bg-secondary/50">(optional)</span>
            </label>
            <select
              value={vendorOverride}
              onChange={(e) => setVendorOverride(e.target.value)}
              className={selectClass}
            >
              <option value="">— Select vendor —</option>
              <option value="mikrotik">MikroTik</option>
            </select>
            <p className="text-xs text-on-bg-secondary/70">
              Vendor tag determines backup commands and metric queries.
            </p>
          </div>
        </>
      )}

      {/* Areas multi-select -- shared between both modes */}
      {areas.length > 0 && (
        <div className="space-y-2">
          <label className={labelClass}>
            Area <span className="text-on-bg-secondary/50">(optional)</span>
          </label>
          {areaIds.length > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {areaIds.map((id) => {
                const area = areas.find((a) => a.id === id);
                if (!area) return null;
                return (
                  <span
                    key={id}
                    className="inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium text-on-bg"
                    style={{ backgroundColor: `${area.color}25`, border: `1px solid ${area.color}60` }}
                  >
                    <span className="inline-block h-2 w-2 rounded-full" style={{ backgroundColor: area.color }} />
                    {area.name}
                    <button
                      type="button"
                      onClick={() => setAreaIds((prev) => prev.filter((a) => a !== id))}
                      className="ml-0.5 text-on-bg-secondary hover:text-on-bg"
                    >
                      <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                      </svg>
                    </button>
                  </span>
                );
              })}
            </div>
          )}
          {areas.filter((a) => !areaIds.includes(a.id)).length > 0 && (
            <select
              value=""
              onChange={(e) => {
                if (e.target.value) {
                  setAreaIds((prev) => [...prev, e.target.value]);
                }
              }}
              className={selectClass}
            >
              <option value="">{areaIds.length === 0 ? 'Unassigned - select area...' : 'Add another area...'}</option>
              {areas.filter((a) => !areaIds.includes(a.id)).map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
          )}
        </div>
      )}

      {/* SSH Profile -- only for physical devices */}
      {!isVirtual && sshProfiles.length > 0 && (
        <div className="space-y-2">
          <label className={labelClass}>
            SSH Profile <span className="text-on-bg-secondary/50">(optional)</span>
          </label>
          <select
            value={sshProfileId}
            onChange={(e) => setSSHProfileId(e.target.value)}
            className={selectClass}
          >
            <option value="">-- No SSH Profile --</option>
            {sshProfiles.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name} ({p.username}:{p.port})
              </option>
            ))}
          </select>
          <p className="text-xs text-on-bg-secondary/70">
            SSH profile is used for config backups.
          </p>
        </div>
      )}

      {error && (
        <p className="rounded-lg border border-status-down/30 bg-status-down/10 px-3 py-2 text-xs text-status-down">
          {error}
        </p>
      )}

      <button
        type="submit"
        disabled={loading}
        className="w-full rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {loading ? 'Adding...' : (isVirtual ? 'Add Virtual Node' : 'Add Device')}
      </button>
    </form>
  );
}
