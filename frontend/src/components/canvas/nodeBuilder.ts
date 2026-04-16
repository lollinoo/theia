import type { Device } from '../../types/api';
import type { SnapshotPayload } from '../../types/metrics';
import { alertStatusForDevice } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import {
  resolveDeviceMonitoringState,
  sanitizeDeviceMetricsForDisplay,
} from '../deviceVisualState';

export function buildTopologyNodes(
  devices: Device[],
  savedPositions: Map<string, { x: number; y: number; pinned?: boolean }>,
  computedPositions: Map<string, { x: number; y: number }>,
  defaultPosition: { x: number; y: number } | undefined,
  editMode: boolean,
  openDeviceMenu: (event: React.MouseEvent, deviceId: string) => void,
  pendingSnapshot: SnapshotPayload | null,
): DeviceNode[] {
  return devices.map((device) => {
    const saved = savedPositions.get(device.id);
    const position = saved ?? defaultPosition ?? computedPositions.get(device.id) ?? { x: 0, y: 0 };

    // Merge snapshot status/hostname/model into device if available
    let deviceData = device;
    if (pendingSnapshot) {
      const snapStatus = pendingSnapshot.device_statuses[device.id];
      const snapHostname = pendingSnapshot.device_hostnames[device.id];
      const snapModel = pendingSnapshot.device_models?.[device.id];
      if (snapStatus || snapHostname || snapModel) {
        deviceData = {
          ...device,
          ...(snapStatus ? { status: snapStatus as Device['status'] } : {}),
          ...(snapHostname ? { sys_name: snapHostname } : {}),
          ...(snapModel ? { hardware_model: snapModel } : {}),
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
        x: position.x,
        y: position.y,
      },
      data: {
        device: deviceData,
        pinned: saved?.pinned ?? false,
        highlighted: false,
        editMode,
        onContextMenu: openDeviceMenu,
        metrics: nodeMetrics,
        alertStatus: pendingSnapshot
          ? alertStatusForDevice(device.id, pendingSnapshot.alerts)
          : undefined,
        isVirtual,
        monitoringState,
        subtype: isVirtual ? (deviceData.tags?.virtual_subtype ?? 'generic') : undefined,
      },
    };
  });
}
