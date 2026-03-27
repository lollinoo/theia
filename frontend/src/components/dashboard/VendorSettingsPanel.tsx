import { useState, useEffect } from 'react';
import { type VendorConfig } from '../../types/api';
import { fetchVendorConfig, updateVendorConfig } from '../../api/client';

interface VendorSettingsPanelProps {
  vendorName?: string;
}

export function VendorSettingsPanel({ vendorName = 'mikrotik' }: VendorSettingsPanelProps) {
  const [vendor, setVendor] = useState<VendorConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState('');

  // Prometheus queries
  const [promCpu, setPromCpu] = useState('');
  const [promMemory, setPromMemory] = useState('');
  const [promTemp, setPromTemp] = useState('');
  const [promUptime, setPromUptime] = useState('');

  // SNMP OIDs
  const [snmpTempOid, setSnmpTempOid] = useState('');
  const [snmpTempScale, setSnmpTempScale] = useState(0.1);
  const [snmpCpuOid, setSnmpCpuOid] = useState('');
  const [snmpMemUsedOid, setSnmpMemUsedOid] = useState('');
  const [snmpMemTotalOid, setSnmpMemTotalOid] = useState('');

  useEffect(() => {
    fetchVendorConfig(vendorName)
      .then((v) => {
        setVendor(v);
        const c = v.config;
        // Prometheus
        setPromCpu(c.metrics.prometheus.cpu || '');
        setPromMemory(c.metrics.prometheus.memory || '');
        setPromTemp(c.metrics.prometheus.temperature || '');
        setPromUptime(c.metrics.prometheus.uptime || '');
        // SNMP
        setSnmpTempOid(c.snmp.temperature_oid || '');
        setSnmpTempScale(c.snmp.temperature_scale || 0);
        setSnmpCpuOid(c.snmp.cpu_oid || '');
        setSnmpMemUsedOid(c.snmp.memory_used_oid || '');
        setSnmpMemTotalOid(c.snmp.memory_total_oid || '');
      })
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load vendor config'))
      .finally(() => setLoading(false));
  }, [vendorName]);

  const handleSave = async () => {
    if (!vendor) return;
    setSaving(true);
    setError('');
    setSaved(false);
    try {
      const updated: VendorConfig['config'] = {
        ...vendor.config,
        metrics: {
          prometheus: {
            cpu: promCpu,
            memory: promMemory,
            temperature: promTemp,
            uptime: promUptime,
          },
        },
        snmp: {
          temperature_oid: snmpTempOid,
          temperature_scale: snmpTempScale,
          cpu_oid: snmpCpuOid,
          memory_used_oid: snmpMemUsedOid,
          memory_total_oid: snmpMemTotalOid,
        },
        backup: vendor.config.backup,
      };
      const result = await updateVendorConfig(vendorName, updated);
      setVendor(result);
      setSaved(true);
      setTimeout(() => setSaved(false), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  const inputClass =
    'w-full rounded-md border border-outline-subtle bg-elevated px-2 py-1.5 text-xs text-on-bg font-mono placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none transition-colors';
  const textareaClass = inputClass + ' min-h-[60px] resize-y';
  const labelClass = 'text-[10px] text-on-bg-secondary font-medium';

  if (loading) {
    return <div className="text-xs text-on-bg-secondary p-4">Loading vendor settings...</div>;
  }

  if (!vendor) {
    return <div className="text-xs text-on-bg-secondary p-4">Vendor not found</div>;
  }

  return (
    <div className="space-y-5 p-4 transition-colors duration-200">
      <div className="text-sm font-medium text-on-bg">
        {vendor.display_name} Settings
      </div>

      {/* Prometheus Queries */}
      <div className="space-y-3">
        <div className="text-xs font-medium text-on-bg-secondary uppercase tracking-[0.12em]">
          Prometheus Queries
        </div>
        <p className="text-[10px] text-on-bg-secondary/70">
          Use <code className="bg-elevated px-1 rounded">%[1]s</code> for label name and <code className="bg-elevated px-1 rounded">%[2]s</code> for label value.
        </p>
        <div className="space-y-2">
          <div>
            <label className={labelClass}>CPU</label>
            <textarea value={promCpu} onChange={(e) => setPromCpu(e.target.value)} className={textareaClass} />
          </div>
          <div>
            <label className={labelClass}>Memory</label>
            <textarea value={promMemory} onChange={(e) => setPromMemory(e.target.value)} className={textareaClass} />
          </div>
          <div>
            <label className={labelClass}>Temperature</label>
            <textarea value={promTemp} onChange={(e) => setPromTemp(e.target.value)} className={textareaClass} />
          </div>
          <div>
            <label className={labelClass}>Uptime</label>
            <textarea value={promUptime} onChange={(e) => setPromUptime(e.target.value)} className={textareaClass} />
          </div>
        </div>
      </div>

      {/* SNMP OIDs */}
      <div className="space-y-3">
        <div className="text-xs font-medium text-on-bg-secondary uppercase tracking-[0.12em]">
          SNMP OIDs
        </div>
        <div className="space-y-2">
          <div>
            <label className={labelClass}>Temperature OID</label>
            <input type="text" value={snmpTempOid} onChange={(e) => setSnmpTempOid(e.target.value)} className={inputClass} />
          </div>
          <div>
            <label className={labelClass}>Temperature Scale</label>
            <input type="number" step="0.01" value={snmpTempScale} onChange={(e) => setSnmpTempScale(parseFloat(e.target.value) || 0)} className={inputClass} />
          </div>
          <div>
            <label className={labelClass}>CPU OID</label>
            <input type="text" value={snmpCpuOid} onChange={(e) => setSnmpCpuOid(e.target.value)} className={inputClass} />
          </div>
          <div>
            <label className={labelClass}>Memory Used OID</label>
            <input type="text" value={snmpMemUsedOid} onChange={(e) => setSnmpMemUsedOid(e.target.value)} className={inputClass} />
          </div>
          <div>
            <label className={labelClass}>Memory Total OID</label>
            <input type="text" value={snmpMemTotalOid} onChange={(e) => setSnmpMemTotalOid(e.target.value)} className={inputClass} />
          </div>
        </div>
      </div>

      {/* Detection (read-only) */}
      <div className="space-y-2">
        <div className="text-xs font-medium text-on-bg-secondary uppercase tracking-[0.12em]">
          Detection (read-only)
        </div>
        <div className="rounded-lg bg-surface-high p-3 text-[10px] text-on-bg-secondary space-y-1">
          <div>
            <span className="font-medium">SysObjectID Prefixes:</span>{' '}
            {vendor.config.detection.sys_object_id_prefixes?.join(', ') || 'none'}
          </div>
          <div>
            <span className="font-medium">SysDescr Patterns:</span>{' '}
            {vendor.config.detection.sys_descr_patterns?.join(', ') || 'none'}
          </div>
        </div>
      </div>

      {/* Actions */}
      {error && (
        <div className="rounded-md border border-status-down/20 bg-status-down/5 p-3 text-xs text-status-down">
          {error}
        </div>
      )}

      <div className="flex gap-2">
        <button
          onClick={handleSave}
          disabled={saving}
          className="flex-1 rounded-md bg-primary px-3 py-2 text-xs font-medium text-white hover:bg-primary/90 disabled:opacity-50 transition-colors"
        >
          {saving ? 'Saving...' : saved ? 'Saved!' : 'Save Changes'}
        </button>
      </div>
    </div>
  );
}
