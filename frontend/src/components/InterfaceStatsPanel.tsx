import type { Device, Link } from '../types/api';
import type { SnapshotPayload } from '../types/metrics';
import { formatThroughput, utilizationColor } from '../types/metrics';
import { formatBandwidth } from './LinkEdge';

interface InterfaceStatsSectionProps {
  device: Device;
  ifName: string;
  snapshot: SnapshotPayload | null;
}

function InterfaceStatsSection({ device, ifName, snapshot }: InterfaceStatsSectionProps) {
  const iface = device.interfaces.find(
    (i) => i.if_name.trim().toLowerCase() === ifName.trim().toLowerCase(),
  );

  const linkMetrics = snapshot?.link_metrics[device.id];
  const metrics = linkMetrics?.find(
    (m) => m.if_name.trim().toLowerCase() === ifName.trim().toLowerCase(),
  ) ?? null;

  const speedLabel = iface && iface.speed > 0 ? formatBandwidth(iface.speed) : null;
  const txLabel = metrics?.tx_bps != null ? formatThroughput(metrics.tx_bps) : '--';
  const rxLabel = metrics?.rx_bps != null ? formatThroughput(metrics.rx_bps) : '--';
  const utilPct =
    metrics?.utilization != null ? Math.round(metrics.utilization * 100) : null;
  const utilColor =
    metrics?.utilization != null ? utilizationColor(metrics.utilization) : '#657786';

  return (
    <div className="rounded-xl border border-border-subtle bg-bg-elevated p-4 space-y-3">
      <div>
        <p className="text-xs uppercase tracking-widest text-text-secondary">Device</p>
        <p className="mt-0.5 text-sm font-medium text-text-primary">
          {device.tags?.display_name || device.sys_name || device.ip}
        </p>
      </div>

      <div>
        <p className="text-xs uppercase tracking-widest text-text-secondary">Interface</p>
        <p className="mt-0.5 text-sm font-mono text-text-primary">{ifName}</p>
        {iface?.if_descr && (
          <p className="text-xs text-text-secondary">{iface.if_descr}</p>
        )}
      </div>

      <div className="grid grid-cols-2 gap-3">
        {speedLabel && (
          <div>
            <p className="text-xs uppercase tracking-widest text-text-secondary">Speed</p>
            <p className="mt-0.5 text-sm text-text-primary">{speedLabel}</p>
          </div>
        )}
        {iface && (
          <div>
            <p className="text-xs uppercase tracking-widest text-text-secondary">Status</p>
            <p
              className={`mt-0.5 text-sm ${iface.oper_status === 'up' ? 'text-status-up' : 'text-status-down'}`}
            >
              {iface.oper_status}
            </p>
          </div>
        )}
      </div>

      <div className="grid grid-cols-2 gap-3 border-t border-border-subtle pt-3">
        <div>
          <p className="text-xs uppercase tracking-widest text-text-secondary">TX</p>
          <p className="mt-0.5 text-sm font-mono text-text-primary">{txLabel}</p>
        </div>
        <div>
          <p className="text-xs uppercase tracking-widest text-text-secondary">RX</p>
          <p className="mt-0.5 text-sm font-mono text-text-primary">{rxLabel}</p>
        </div>
      </div>

      {utilPct !== null && (
        <div>
          <div className="flex items-center justify-between">
            <p className="text-xs uppercase tracking-widest text-text-secondary">Utilization</p>
            <p className="text-xs font-mono" style={{ color: utilColor }}>
              {utilPct}%
            </p>
          </div>
          <div className="mt-1 h-1.5 w-full rounded-full bg-bg-surface">
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
}

export function InterfaceStatsPanel({
  link,
  sourceDevice,
  targetDevice,
  snapshot,
}: InterfaceStatsPanelProps) {
  return (
    <div className="space-y-3 p-4">
      <InterfaceStatsSection
        device={sourceDevice}
        ifName={link.source_if_name}
        snapshot={snapshot}
      />
      <InterfaceStatsSection
        device={targetDevice}
        ifName={link.target_if_name}
        snapshot={snapshot}
      />
    </div>
  );
}

interface DeviceInterfaceStatsPanelProps {
  device: Device;
  snapshot: SnapshotPayload | null;
}

export function DeviceInterfaceStatsPanel({
  device,
  snapshot,
}: DeviceInterfaceStatsPanelProps) {
  const interfaces = device.interfaces
    .filter((i) => {
      const lower = i.if_name.toLowerCase();
      return !lower.startsWith('lo') && lower !== 'null' && !lower.startsWith('null');
    })
    .sort((a, b) => {
      const aUp = a.oper_status === 'up';
      const bUp = b.oper_status === 'up';
      if (aUp !== bUp) return aUp ? -1 : 1;
      return a.if_name.localeCompare(b.if_name);
    });

  if (interfaces.length === 0) {
    return (
      <div className="p-4 text-sm text-text-secondary">
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
          snapshot={snapshot}
        />
      ))}
    </div>
  );
}
