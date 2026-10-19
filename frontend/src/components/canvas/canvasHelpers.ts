/**
 * Defines canvas helpers behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { FitViewOptions } from '@xyflow/react';
import type { PositionPayload } from '../../hooks/usePositions';
import type { LinkMetricsDTO } from '../../types/metrics';
import { formatThroughput } from '../../types/metrics';
import type { ContextMenuItem } from '../ContextMenu';
import type { DeviceNode } from '../DeviceCard';

/** Describes the handle side contract used by the topology canvas. */
export type HandleSide = 'top' | 'right' | 'bottom' | 'left';

/** Defines manual edge storage key constants and helper contracts for the topology canvas. */
export const manualEdgeStorageKey = 'theia-manual-edges';
/** Defines manual edge migration storage key constants and helper contracts for the topology canvas. */
export const manualEdgeMigrationStorageKey = 'theia-manual-edge-migration-v1';

/** Defines default polling interval ms constants and helper contracts for the topology canvas. */
export const defaultPollingIntervalMs = 60_000;
/** Defines topology fit view padding constants and helper contracts for the topology canvas. */
export const topologyFitViewPadding: NonNullable<FitViewOptions['padding']> = {
  top: '96px',
  right: 0.08,
  bottom: 0.08,
  left: 0.08,
};

/** Builds position payload for the topology canvas. */
export function buildPositionPayload(nodes: DeviceNode[]): PositionPayload[] {
  return nodes
    .filter((node) => !isGhostDeviceNode(node))
    .map((node) => ({
      device_id: node.id,
      x: node.position.x,
      y: node.position.y,
      pinned: node.data.pinned,
    }));
}

/** Identifies ghost device node for the topology canvas. */
export function isGhostDeviceNode(node: DeviceNode): boolean {
  return node.data.kind === 'ghost-device' || node.data.isGhost === true;
}

/** Compacts throughput for the topology canvas. */
export function compactThroughput(bps: number): string {
  return formatThroughput(bps)
    .replace(' Gbps', 'G')
    .replace(' Mbps', 'M')
    .replace(' Kbps', 'K')
    .replace(' bps', 'b');
}

/** Normalizes interface name for the topology canvas. */
export function normalizeInterfaceName(name: string | undefined): string {
  return (name ?? '').trim().toLowerCase();
}

/** Builds throughput label for the topology canvas. */
export function buildThroughputLabel(metrics: LinkMetricsDTO): string | undefined {
  if (metrics.tx_bps === null && metrics.rx_bps === null) {
    return undefined;
  }

  const txLabel = metrics.tx_bps === null ? '--' : compactThroughput(metrics.tx_bps);
  const rxLabel = metrics.rx_bps === null ? '--' : compactThroughput(metrics.rx_bps);
  return `TX: ${txLabel} / RX: ${rxLabel}`;
}

/** Viewport size for the topology canvas. */
export function viewportSize(): { width: number; height: number } {
  return {
    width: typeof window === 'undefined' ? 1440 : window.innerWidth,
    height: typeof window === 'undefined' ? 900 : window.innerHeight,
  };
}

type DeviceContextMenuItemId = 'winbox' | 'grafana' | 'configure';

/** Describes the device context menu item contract used by the topology canvas. */
export type DeviceContextMenuItem = ContextMenuItem & { id: DeviceContextMenuItemId };

interface BuildDeviceContextMenuItemsOptions {
  isVirtual: boolean;
  grafanaEnabled: boolean;
  winboxDisabled: boolean;
  winboxTitle?: string;
  onOpenWinbox: () => void;
  onOpenGrafana: () => void;
  onConfigure: () => void;
}

/** Builds device context menu items for the topology canvas. */
export function buildDeviceContextMenuItems({
  isVirtual,
  grafanaEnabled,
  winboxDisabled,
  winboxTitle,
  onOpenWinbox,
  onOpenGrafana,
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
      id: 'configure',
      label: 'Configure',
      icon: 'settings',
      onClick: onConfigure,
    },
  ];

  return isVirtual ? allItems.filter((item) => item.id === 'configure') : allItems;
}
