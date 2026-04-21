import type { Device, Link } from '../../types/api';
import type { LinkMetricsDTO } from '../../types/metrics';
import { formatThroughput } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { PositionPayload } from '../../hooks/usePositions';
import type { ContextMenuItem } from '../ContextMenu';
import { formatBandwidth } from '../linkSemantics';

export type HandleSide = 'top' | 'right' | 'bottom' | 'left';

export const manualEdgeStorageKey = 'theia-manual-edges';

export const defaultPollingIntervalMs = 60_000;
export const staleThresholdMs = defaultPollingIntervalMs * 2;

export function buildPositionPayload(nodes: DeviceNode[]): PositionPayload[] {
  return nodes.map((node) => ({
    device_id: node.id,
    x: node.position.x,
    y: node.position.y,
    pinned: node.data.pinned,
  }));
}

export function inferSpeedLabel(
  sourceDevice: Device | undefined,
  targetDevice?: Device,
): string | undefined {
  const speeds = [
    ...(sourceDevice?.interfaces ?? []).map((iface) => iface.speed),
    ...(targetDevice?.interfaces ?? []).map((iface) => iface.speed),
  ].filter((speed) => speed > 0);

  if (speeds.length === 0) {
    return undefined;
  }

  return formatBandwidth(Math.max(...speeds));
}

export function compactThroughput(bps: number): string {
  return formatThroughput(bps)
    .replace(' Gbps', 'G')
    .replace(' Mbps', 'M')
    .replace(' Kbps', 'K')
    .replace(' bps', 'b');
}

export function normalizeInterfaceName(name: string | undefined): string {
  return (name ?? '').trim().toLowerCase();
}

export function buildThroughputLabel(metrics: LinkMetricsDTO): string | undefined {
  if (metrics.tx_bps === null && metrics.rx_bps === null) {
    return undefined;
  }

  const txLabel = metrics.tx_bps === null ? '--' : compactThroughput(metrics.tx_bps);
  const rxLabel = metrics.rx_bps === null ? '--' : compactThroughput(metrics.rx_bps);
  return `TX: ${txLabel} / RX: ${rxLabel}`;
}

export function findLinkMetrics(
  snapshotMetrics: Record<string, LinkMetricsDTO[]>,
  link: Link,
): LinkMetricsDTO | null {
  // Try source device first (standard case: source has the physical interface)
  const sourceDeviceMetrics = snapshotMetrics[link.source_device_id];
  if (sourceDeviceMetrics) {
    const sourceIfName = normalizeInterfaceName(link.source_if_name);
    const found = sourceDeviceMetrics.find(
      (metric) => normalizeInterfaceName(metric.source_if_name) === sourceIfName,
    );
    if (found) return found;
  }

  // Fallback: try target device (virtual-source links where metrics are keyed by real device)
  const targetDeviceMetrics = snapshotMetrics[link.target_device_id];
  if (targetDeviceMetrics) {
    const targetIfName = normalizeInterfaceName(link.target_if_name);
    const found = targetDeviceMetrics.find(
      (metric) => normalizeInterfaceName(metric.target_if_name) === targetIfName,
    );
    if (found) return found;
  }

  return null;
}

export function statusColor(status: Device['status']): string {
  switch (status) {
    case 'up':
      return 'var(--color-status-up)';
    case 'down':
      return 'var(--color-status-down)';
    case 'probing':
      return 'var(--color-status-probing)';
    default:
      return 'var(--color-status-unknown)';
  }
}

export function viewportSize(): { width: number; height: number } {
  return {
    width: typeof window === 'undefined' ? 1440 : window.innerWidth,
    height: typeof window === 'undefined' ? 900 : window.innerHeight,
  };
}

type DeviceContextMenuItemId = 'winbox' | 'grafana' | 'interface-stats' | 'configure';

export type DeviceContextMenuItem = ContextMenuItem & { id: DeviceContextMenuItemId };

interface BuildDeviceContextMenuItemsOptions {
  isVirtual: boolean;
  grafanaEnabled: boolean;
  winboxDisabled: boolean;
  winboxTitle?: string;
  onOpenWinbox: () => void;
  onOpenGrafana: () => void;
  onOpenInterfaceStats: () => void;
  onConfigure: () => void;
}

export function buildDeviceContextMenuItems({
  isVirtual,
  grafanaEnabled,
  winboxDisabled,
  winboxTitle,
  onOpenWinbox,
  onOpenGrafana,
  onOpenInterfaceStats,
  onConfigure,
}: BuildDeviceContextMenuItemsOptions): DeviceContextMenuItem[] {
  const allItems: DeviceContextMenuItem[] = [
    {
      id: 'winbox',
      label: 'Open in WinBox',
      icon: 'open_in_new',
      disabled: winboxDisabled,
      title: winboxTitle,
      onClick: onOpenWinbox,
    },
    {
      id: 'grafana',
      label: grafanaEnabled ? 'Open in Grafana' : 'Open in Grafana (not configured)',
      icon: 'hub',
      onClick: onOpenGrafana,
    },
    {
      id: 'interface-stats',
      label: 'Per-Interface Stats',
      icon: 'devices',
      onClick: onOpenInterfaceStats,
    },
    {
      id: 'configure',
      label: 'Configure',
      icon: 'settings',
      onClick: onConfigure,
    },
  ];

  return isVirtual
    ? allItems.filter((item) => item.id === 'configure')
    : allItems;
}
