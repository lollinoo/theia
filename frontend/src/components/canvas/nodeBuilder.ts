import type { Device, Link } from '../../types/api';
import { alertStatusForDevice, type AlertDTO, type SnapshotPayload } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import {
  resolveDeviceMonitoringState,
  sanitizeDeviceMetricsForDisplay,
} from '../deviceVisualState';
import { preferVisibleLinks } from './edgeBuilder';

function hasUsablePosition(
  position: { x: number; y: number; pinned?: boolean } | undefined,
): boolean {
  return position !== undefined
    && Number.isFinite(position.x)
    && Number.isFinite(position.y);
}
function selfLinkScore(link: Link): number {
  let score = 0;
  if (link.discovery_protocol === 'lldp') score += 4;
  if (link.source_if_name) score += 2;
  if (link.target_if_name) score += 2;
  return score;
}

export function buildTopologyNodes(
  devices: Device[],
  savedPositions: Map<string, { x: number; y: number; pinned?: boolean }>,
  computedPositions: Map<string, { x: number; y: number }>,
  defaultPosition: { x: number; y: number } | undefined,
  editMode: boolean,
  openDeviceMenu: (event: React.MouseEvent, deviceId: string) => void,
  pendingSnapshot: SnapshotPayload | null,
  alerts: AlertDTO[] = [],
  links: Link[] = [],
  onSelfLinkClick?: (link: Link) => void,
  currentPositions: Map<string, { x: number; y: number; pinned?: boolean }> = new Map(),
  placementDeviceIds: Set<string> = new Set(devices.map((device) => device.id)),
): DeviceNode[] {
  const selfLinksByDeviceId = new Map<string, Link[]>();
  for (const link of preferVisibleLinks(links)) {
    if (link.source_device_id !== link.target_device_id) continue;
    const deviceLinks = selfLinksByDeviceId.get(link.source_device_id) ?? [];
    deviceLinks.push(link);
    selfLinksByDeviceId.set(link.source_device_id, deviceLinks);
  }

  for (const deviceLinks of selfLinksByDeviceId.values()) {
    deviceLinks.sort((left, right) => {
      const scoreDelta = selfLinkScore(right) - selfLinkScore(left);
      if (scoreDelta !== 0) return scoreDelta;
      return left.id.localeCompare(right.id);
    });
  }

  return devices.map((device) => {
    const current = currentPositions.get(device.id);
    const saved = savedPositions.get(device.id);
    const canPlaceDevice = placementDeviceIds.has(device.id);
    const placementPosition = canPlaceDevice
      ? defaultPosition ?? computedPositions.get(device.id)
      : undefined;
    const position = hasUsablePosition(current)
      ? current
      : hasUsablePosition(saved)
        ? saved
        : placementPosition;
    const resolvedPosition = position ?? { x: 0, y: 0 };
    const selfLinks = selfLinksByDeviceId.get(device.id);

    // Merge runtime status into fetched topology data when available.
    let deviceData = device;
    if (pendingSnapshot) {
      const snapStatus = pendingSnapshot.device_statuses[device.id];
      if (snapStatus) {
        deviceData = {
          ...device,
          ...(snapStatus ? { status: snapStatus as Device['status'] } : {}),
        };
      }
    }

    const nodeMetrics = sanitizeDeviceMetricsForDisplay(
      deviceData,
      pendingSnapshot?.device_metrics[device.id] ?? null,
    );

    // Virtual devices have no SNMP metrics; detect and propagate flags
    const isVirtual = device.device_type === 'virtual';
    const monitoringState = resolveDeviceMonitoringState(deviceData);

    return {
      id: device.id,
      type: 'device',
      position: {
        x: resolvedPosition.x,
        y: resolvedPosition.y,
      },
      data: {
        device: deviceData,
        pinned: current?.pinned ?? saved?.pinned ?? false,
        highlighted: false,
        editMode,
        onContextMenu: openDeviceMenu,
        metrics: nodeMetrics,
        alertStatus: alertStatusForDevice(device.id, alerts),
        isVirtual,
        monitoringState,
        subtype: isVirtual ? (deviceData.tags?.virtual_subtype ?? 'generic') : undefined,
        selfLinks,
        onSelfLinkClick,
      },
    };
  });
}
