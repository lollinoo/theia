import type { Node } from '@xyflow/react';

import type { Device, Link } from '../../types/api';
import type { LinkMetricsDTO } from '../../types/metrics';
import { formatThroughput } from '../../types/metrics';
import type { DeviceNodeData } from '../DeviceCard';
import type { PositionPayload } from '../../hooks/usePositions';
import { formatBandwidth } from '../LinkEdge';

export type HandleSide = 'top' | 'right' | 'bottom' | 'left';

export const manualEdgeStorageKey = 'theia-manual-edges';

export const defaultPollingIntervalMs = 60_000;
export const staleThresholdMs = defaultPollingIntervalMs * 2;

export function buildPositionPayload(nodes: Node<DeviceNodeData>[]): PositionPayload[] {
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
  const deviceMetrics = snapshotMetrics[link.source_device_id];
  if (!deviceMetrics) {
    return null;
  }

  const sourceIfName = normalizeInterfaceName(link.source_if_name);
  return (
    deviceMetrics.find(
      (metric) => normalizeInterfaceName(metric.if_name) === sourceIfName,
    ) ?? null
  );
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
