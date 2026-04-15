import { useEffect, useState } from 'react';
import type { Device, InterfaceInfo, Link } from '../types/api';
import { formatThroughput, isPrometheusUnavailable, type PrometheusStatusPayload, type SnapshotPayload, utilizationColor } from '../types/metrics';
import { fetchDeviceInterfaces } from '../api/client';
import { formatBandwidth } from './LinkEdge';

interface InterfaceStatsSectionProps {
  device: Device;
  ifName: string;
  interfaceInfo?: InterfaceInfo | null;
  snapshot: SnapshotPayload | null;
  prometheusStatus: PrometheusStatusPayload | null;
}

function InterfaceStatsSection({ device, ifName, interfaceInfo, snapshot, prometheusStatus }: InterfaceStatsSectionProps) {
  const iface = interfaceInfo;

  const isDown = device.status === 'down';
  const src = device.metrics_source || 'prometheus';
  const promDown = isPrometheusUnavailable(prometheusStatus)
    && (src === 'prometheus' || src === 'prometheus_snmp_fallback');
  const linkMetrics = (isDown || promDown) ? null : snapshot?.link_metrics[device.id];
  const metrics = linkMetrics?.find(
    (m) => m.if_name.trim().toLowerCase() === ifName.trim().toLowerCase(),
  ) ?? null;

  const speedLabel = iface && iface.speed > 0 ? formatBandwidth(iface.speed) : null;
  const txLabel = metrics?.tx_bps != null ? formatThroughput(metrics.tx_bps) : '--';
  const rxLabel = metrics?.rx_bps != null ? formatThroughput(metrics.rx_bps) : '--';
  const utilPct =
    metrics?.utilization != null ? Math.round(metrics.utilization * 100) : null;
  const utilColor =
    metrics?.utilization != null ? utilizationColor(metrics.utilization) : 'var(--color-status-unknown)';

  // Combine device-down and prometheus-down into a single "metrics unavailable" flag
  // for both data clearing and visual styling.
  const metricsUnavailable = isDown || promDown;

  return (
    <div className={`rounded-xl p-4 space-y-3 transition-colors duration-200 ${metricsUnavailable ? 'bg-status-down/10' : 'bg-surface-high'}`}>
      <div>
        <p className="text-[12px] uppercase tracking-[0.16em] text-on-bg-secondary">Device</p>
        <p className="mt-0.5 text-sm font-medium text-on-bg">
          {device.tags?.display_name || device.sys_name || device.ip}
        </p>
        {isDown && (
          <p className="mt-1 text-xs font-medium text-status-down">Device unreachable</p>
        )}
        {promDown && !isDown && (
          <p className="mt-1 text-xs font-medium text-status-down">Prometheus unavailable</p>
        )}
      </div>

      <div>
        <p className="text-[12px] uppercase tracking-[0.16em] text-on-bg-secondary">Interface</p>
        <p className="mt-0.5 text-sm font-mono text-on-bg">{ifName}</p>
        {iface?.if_descr && (
          <p className="text-xs text-on-bg-secondary">{iface.if_descr}</p>
        )}
      </div>

      <div className="grid grid-cols-2 gap-3">
        {speedLabel && (
          <div>
            <p className="text-[12px] uppercase tracking-[0.16em] text-on-bg-secondary">Speed</p>
            <p className="mt-0.5 font-mono text-[11px] font-semibold text-on-bg">{speedLabel}</p>
          </div>
        )}
        {iface && (
          <div>
            <p className="text-[12px] uppercase tracking-[0.16em] text-on-bg-secondary">Status</p>
            <p
              className={`mt-0.5 text-sm ${isDown ? 'text-status-down' : iface.oper_status === 'up' ? 'text-status-up' : 'text-status-down'}`}
            >
              {isDown ? 'down' : iface.oper_status}
            </p>
          </div>
        )}
      </div>

      <div className="grid grid-cols-2 gap-3 mt-3 pt-3">
        <div>
          <p className="text-[12px] uppercase tracking-[0.16em] text-on-bg-secondary">TX</p>
          <p className={`mt-0.5 font-mono text-[11px] font-semibold ${metricsUnavailable ? 'text-status-down/70' : 'text-on-bg'}`}>{txLabel}</p>
        </div>
        <div>
          <p className="text-[12px] uppercase tracking-[0.16em] text-on-bg-secondary">RX</p>
          <p className={`mt-0.5 font-mono text-[11px] font-semibold ${metricsUnavailable ? 'text-status-down/70' : 'text-on-bg'}`}>{rxLabel}</p>
        </div>
      </div>

      {utilPct !== null && (
        <div>
          <div className="flex items-center justify-between">
            <p className="text-[12px] uppercase tracking-[0.16em] text-on-bg-secondary">Utilization</p>
            <p className="text-xs font-mono" style={{ color: utilColor }}>
              {utilPct}%
            </p>
          </div>
          <div className="mt-1 h-1.5 w-full rounded-full bg-surface">
            <div
              className="h-1.5 rounded-full transition-all duration-500"
              style={{ width: `${Math.min(utilPct, 100)}%`, backgroundColor: utilColor }}
            />
          </div>
        </div>
      )}
    </div>
  );
}

interface InterfaceStatsPanelProps {
  link: Link;
  sourceDevice: Device;
  targetDevice: Device;
  snapshot: SnapshotPayload | null;
  prometheusStatus: PrometheusStatusPayload | null;
}

interface NegotiationSummaryProps {
  sourceInterfaceInfo: InterfaceInfo | null;
  targetInterfaceInfo: InterfaceInfo | null;
}

function NegotiationSummary({
  sourceInterfaceInfo,
  targetInterfaceInfo,
}: NegotiationSummaryProps) {
  const sourceSpeed = sourceInterfaceInfo?.speed ?? 0;
  const targetSpeed = targetInterfaceInfo?.speed ?? 0;
  const sourceLabel = sourceSpeed > 0 ? formatBandwidth(sourceSpeed) : 'Unknown';
  const targetLabel = targetSpeed > 0 ? formatBandwidth(targetSpeed) : 'Unknown';

  let toneClass = 'border-outline-subtle bg-surface-high text-on-bg';
  let summaryLabel = 'Autonegotiation';
  let detailLabel = 'Waiting for interface speed data from one or both ends.';

  if (sourceSpeed > 0 && targetSpeed > 0 && sourceSpeed === targetSpeed) {
    toneClass = 'border-status-up/30 bg-status-up/10 text-status-up';
    summaryLabel = `Matched at ${formatBandwidth(sourceSpeed)}`;
    detailLabel = 'Both interfaces report the same negotiated speed.';
  } else if (sourceSpeed > 0 && targetSpeed > 0) {
    toneClass = 'border-status-probing/30 bg-status-probing/10 text-status-probing';
    summaryLabel = `${formatBandwidth(sourceSpeed)} vs ${formatBandwidth(targetSpeed)}`;
    detailLabel = 'The two ends report different negotiated speeds.';
  } else if (sourceSpeed > 0 || targetSpeed > 0) {
    toneClass = 'border-status-probing/30 bg-status-probing/10 text-status-probing';
    summaryLabel = sourceSpeed > 0 ? sourceLabel : targetLabel;
    detailLabel = 'Only one side exposed a negotiated speed.';
  }

  return (
    <div className={`rounded-xl border px-4 py-3 transition-colors duration-200 ${toneClass}`}>
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="text-[12px] uppercase tracking-[0.16em] text-on-bg-secondary">
            Autonegotiation
          </p>
          <p className="mt-1 text-sm font-semibold">{summaryLabel}</p>
          <p className="mt-1 text-xs text-on-bg-secondary">{detailLabel}</p>
        </div>
      </div>

      <div className="mt-3 grid grid-cols-2 gap-3 border-t border-outline-subtle/70 pt-3">
        <div>
          <p className="text-[12px] uppercase tracking-[0.16em] text-on-bg-secondary">Source</p>
          <p className="mt-0.5 font-mono text-[11px] font-semibold text-on-bg">{sourceLabel}</p>
        </div>
        <div>
          <p className="text-[12px] uppercase tracking-[0.16em] text-on-bg-secondary">Target</p>
          <p className="mt-0.5 font-mono text-[11px] font-semibold text-on-bg">{targetLabel}</p>
        </div>
      </div>
    </div>
  );
}

export function InterfaceStatsPanel({
  link,
  sourceDevice,
  targetDevice,
  snapshot,
  prometheusStatus,
}: InterfaceStatsPanelProps) {
  const [sourceInterfaceInfo, setSourceInterfaceInfo] = useState<InterfaceInfo | null>(null);
  const [targetInterfaceInfo, setTargetInterfaceInfo] = useState<InterfaceInfo | null>(null);

  useEffect(() => {
    let stale = false;
    setSourceInterfaceInfo(null);
    setTargetInterfaceInfo(null);
    fetchDeviceInterfaces(sourceDevice.id)
      .then((ifaces) => {
        if (stale) return;
        const match = ifaces.find((i) => i.if_name.trim().toLowerCase() === link.source_if_name.trim().toLowerCase());
        setSourceInterfaceInfo(match ?? null);
      })
      .catch(() => { if (!stale) setSourceInterfaceInfo(null); });
    fetchDeviceInterfaces(targetDevice.id)
      .then((ifaces) => {
        if (stale) return;
        const match = ifaces.find((i) => i.if_name.trim().toLowerCase() === link.target_if_name.trim().toLowerCase());
        setTargetInterfaceInfo(match ?? null);
      })
      .catch(() => { if (!stale) setTargetInterfaceInfo(null); });
    return () => { stale = true; };
  }, [sourceDevice.id, targetDevice.id, link.source_if_name, link.target_if_name]);

  return (
    <div className="space-y-3 p-4">
      <NegotiationSummary
        sourceInterfaceInfo={sourceInterfaceInfo}
        targetInterfaceInfo={targetInterfaceInfo}
      />
      <InterfaceStatsSection
        device={sourceDevice}
        ifName={link.source_if_name}
        interfaceInfo={sourceInterfaceInfo}
        snapshot={snapshot}
        prometheusStatus={prometheusStatus}
      />
      <InterfaceStatsSection
        device={targetDevice}
        ifName={link.target_if_name}
        interfaceInfo={targetInterfaceInfo}
        snapshot={snapshot}
        prometheusStatus={prometheusStatus}
      />
    </div>
  );
}

interface DeviceInterfaceStatsPanelProps {
  device: Device;
  snapshot: SnapshotPayload | null;
  prometheusStatus: PrometheusStatusPayload | null;
}

export function DeviceInterfaceStatsPanel({
  device,
  snapshot,
  prometheusStatus,
}: DeviceInterfaceStatsPanelProps) {
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let stale = false;
    setLoading(true);
    fetchDeviceInterfaces(device.id)
      .then((ifaces) => {
        if (stale) return;
        const filtered = ifaces.filter((i) => {
          const lower = i.if_name.toLowerCase();
          return !lower.startsWith('lo') && lower !== 'null' && !lower.startsWith('null');
        });
        filtered.sort((a, b) => {
          const aUp = a.oper_status === 'up';
          const bUp = b.oper_status === 'up';
          if (aUp !== bUp) return aUp ? -1 : 1;
          return a.if_name.localeCompare(b.if_name);
        });
        setInterfaces(filtered);
      })
      .catch(() => { if (!stale) setInterfaces([]); })
      .finally(() => { if (!stale) setLoading(false); });
    return () => { stale = true; };
  }, [device.id]);

  if (!loading && interfaces.length === 0) {
    return (
      <div className="p-4 text-sm text-on-bg-secondary">
        No interfaces discovered for this device.
      </div>
    );
  }

  return (
    <div className="space-y-3 p-4">
      {interfaces.map((iface) => (
        <InterfaceStatsSection
          key={iface.if_name}
          device={device}
          ifName={iface.if_name}
          interfaceInfo={iface}
          snapshot={snapshot}
          prometheusStatus={prometheusStatus}
        />
      ))}
    </div>
  );
}
