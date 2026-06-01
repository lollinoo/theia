import { useEffect, useRef, useState } from 'react';
import { fetchSettings, runTopologyDiscovery } from '../../api/client';
import { ServerError, ValidationError } from '../../api/errors';
import type { Device, MetricsSource, TopologyDiscoveryMode } from '../../types/api';
import { createAsyncStaleGuard } from '../../utils/asyncStaleGuard';
import {
  TOPOLOGY_DISCOVERY_MODE_OPTIONS,
  formatTopologyBootstrapState,
  formatTopologyDiscoveryMode,
  formatTopologyDiscoveryResult,
  formatTopologyDiscoveryTimestamp,
  formatTopologyFollowupExpectation,
} from '../../utils/topologyDiscovery';

interface DeviceTopologyDiscoverySectionProps {
  device: Device;
  topologyDiscoveryMode: TopologyDiscoveryMode;
  metricsMode: MetricsSource;
  ip: string;
  readOnly?: boolean;
  resetKey?: string;
  isVirtual?: boolean;
  onTopologyDiscoveryModeChange: (mode: TopologyDiscoveryMode) => void;
}

export function DeviceTopologyDiscoverySection({
  device,
  topologyDiscoveryMode,
  metricsMode,
  ip,
  readOnly = false,
  resetKey,
  isVirtual,
  onTopologyDiscoveryModeChange,
}: DeviceTopologyDiscoverySectionProps) {
  const [topologyDiscoveryDefaultMode, setTopologyDiscoveryDefaultMode] =
    useState<TopologyDiscoveryMode>('lldp_cdp');
  const [topologyDiscoveryMessage, setTopologyDiscoveryMessage] = useState<string | null>(null);
  const [topologyDiscoveryError, setTopologyDiscoveryError] = useState<string | null>(null);
  const [topologyDiscoveryRunning, setTopologyDiscoveryRunning] = useState(false);
  const discoveryGenerationRef = useRef(0);
  const discoveryContextKeyRef = useRef('');
  const discoveryContextKey = `${device.id}\u0000${resetKey ?? ''}\u0000${isVirtual ? '1' : '0'}\u0000${
    readOnly ? '1' : '0'
  }`;

  if (discoveryContextKeyRef.current !== discoveryContextKey) {
    discoveryContextKeyRef.current = discoveryContextKey;
    discoveryGenerationRef.current += 1;
  }

  useEffect(() => {
    const staleGuard = createAsyncStaleGuard();
    fetchSettings()
      .then((rawSettings) => {
        staleGuard.run(() =>
          setTopologyDiscoveryDefaultMode(
            (rawSettings['topology_discovery_default_mode'] as TopologyDiscoveryMode | undefined) ??
              'lldp_cdp',
          ),
        );
      })
      .catch(() => {
        staleGuard.run(() => setTopologyDiscoveryDefaultMode('lldp_cdp'));
      });
    return () => {
      staleGuard.cancel();
    };
  }, [device.id]);

  useEffect(() => {
    setTopologyDiscoveryMessage(null);
    setTopologyDiscoveryError(null);
    setTopologyDiscoveryRunning(false);
  }, [resetKey, device.id, isVirtual, readOnly]);

  useEffect(() => {
    return () => {
      discoveryGenerationRef.current += 1;
    };
  }, []);

  if (isVirtual) {
    return null;
  }

  async function handleRunTopologyDiscovery() {
    if (readOnly) return;
    const discoveryGeneration = discoveryGenerationRef.current;
    setTopologyDiscoveryRunning(true);
    setTopologyDiscoveryError(null);
    setTopologyDiscoveryMessage(null);
    try {
      await runTopologyDiscovery(device.id);
      if (discoveryGenerationRef.current !== discoveryGeneration) return;
      setTopologyDiscoveryMessage(
        'Topology discovery started. Links and ports will refresh when the SNMP pass completes.',
      );
    } catch (err) {
      if (discoveryGenerationRef.current !== discoveryGeneration) return;
      if (err instanceof ServerError || err instanceof ValidationError) {
        setTopologyDiscoveryError(err.message);
      } else {
        setTopologyDiscoveryError(
          err instanceof Error ? err.message : 'Failed to start topology discovery.',
        );
      }
    } finally {
      if (discoveryGenerationRef.current === discoveryGeneration) {
        setTopologyDiscoveryRunning(false);
      }
    }
  }

  const discoveryState = device.topology_bootstrap_state || 'idle';
  const discoveryBusy =
    topologyDiscoveryRunning ||
    discoveryState === 'pending' ||
    discoveryState === 'followup_scheduled';
  const discoveryRunDisabled =
    readOnly || discoveryBusy || metricsMode === 'prometheus' || ip.trim() === '';
  const effectiveTopologyDiscoveryMode = device.effective_topology_discovery_mode || 'off';
  const configuredTopologyDiscoveryMode =
    topologyDiscoveryMode === 'inherit'
      ? `Use global default (${formatTopologyDiscoveryMode(topologyDiscoveryDefaultMode)})`
      : formatTopologyDiscoveryMode(topologyDiscoveryMode);
  const nextTopologyFollowup = formatTopologyFollowupExpectation(
    discoveryState,
    device.last_topology_discovery_at,
  );

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
          Topology Discovery
        </p>
        <span className="text-xs text-on-bg-secondary">
          Effective: {formatTopologyDiscoveryMode(effectiveTopologyDiscoveryMode)}
        </span>
      </div>
      <select
        id="device-topology-discovery-mode"
        aria-label="Topology Discovery"
        value={topologyDiscoveryMode}
        disabled={readOnly}
        onChange={(e) => onTopologyDiscoveryModeChange(e.target.value as TopologyDiscoveryMode)}
        className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
      >
        {TOPOLOGY_DISCOVERY_MODE_OPTIONS.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
      <div className="space-y-2 rounded-lg bg-surface-high p-3">
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs uppercase tracking-widest text-on-bg-secondary">
            Device Setting
          </span>
          <span className="text-sm text-on-bg">{configuredTopologyDiscoveryMode}</span>
        </div>
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs uppercase tracking-widest text-on-bg-secondary">
            Bootstrap State
          </span>
          <span className="text-sm text-on-bg">
            {formatTopologyBootstrapState(device.topology_bootstrap_state)}
          </span>
        </div>
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs uppercase tracking-widest text-on-bg-secondary">
            Last Discovery
          </span>
          <span className="text-sm text-on-bg">
            {formatTopologyDiscoveryTimestamp(device.last_topology_discovery_at)}
          </span>
        </div>
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs uppercase tracking-widest text-on-bg-secondary">
            Last Result
          </span>
          <span className="text-sm text-on-bg">
            {formatTopologyDiscoveryResult(device.last_topology_discovery_result)}
          </span>
        </div>
        {nextTopologyFollowup && (
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs uppercase tracking-widest text-on-bg-secondary">
              Next Follow-up
            </span>
            <span className="text-sm text-on-bg">{nextTopologyFollowup}</span>
          </div>
        )}
      </div>
      <button
        type="button"
        onClick={() => {
          void handleRunTopologyDiscovery();
        }}
        disabled={discoveryRunDisabled}
        className="w-full rounded-lg bg-surface-high px-4 py-2 text-sm font-medium text-on-bg transition-colors hover:bg-elevated disabled:cursor-not-allowed disabled:opacity-50"
      >
        {discoveryBusy ? 'Topology discovery running...' : 'Run Topology Discovery Now'}
      </button>
      <p className="text-xs text-on-bg-secondary">
        {metricsMode === 'prometheus'
          ? 'Prometheus-only devices cannot run SNMP topology discovery until SNMP or fallback mode is enabled.'
          : ip.trim() === ''
            ? 'Topology discovery requires a device IP.'
            : 'Bootstrap once opens a short discovery window, may queue one follow-up to fill missing ports, then returns the device to Off.'}
      </p>
      {topologyDiscoveryMessage && (
        <p className="rounded-lg border border-status-up/30 bg-status-up/10 px-3 py-2 text-xs text-status-up">
          {topologyDiscoveryMessage}
        </p>
      )}
      {topologyDiscoveryError && (
        <p className="rounded-lg border border-status-down/30 bg-status-down/10 px-3 py-2 text-xs text-status-down">
          {topologyDiscoveryError}
        </p>
      )}
    </div>
  );
}
